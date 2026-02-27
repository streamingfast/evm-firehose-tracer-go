package tests

import (
	firehose "github.com/streamingfast/evm-firehose-tracer-go"
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// Test error constants matching VM errors from go-ethereum
// We use the actual VM errors so that both the native validator (which uses errors.Is)
// and the shared tracer (which uses errorIsString) can recognize them correctly
var (
	testErrExecutionReverted           = vm.ErrExecutionReverted
	testErrInsufficientBalanceTransfer = vm.ErrInsufficientBalance
	testErrMaxCallDepth                = vm.ErrDepth
	testErrOutOfGas                    = vm.ErrOutOfGas
	testErrInvalidInput                = errors.New("invalid input")
)

// wrapError wraps an error with a custom reason message
// The wrapped error is checked by errorIs in tracer.go
type wrapError struct {
	reason  string
	wrapped error
}

func (e *wrapError) Error() string {
	return e.reason
}

func (e *wrapError) Unwrap() error {
	return e.wrapped
}

// TestBlock provides a standard test block with reasonable defaults
// This block represents block #100 with typical Ethereum mainnet settings
//
// IMPORTANT: Until native validator code is removed, this block MUST produce
// the exact same hash and size as the native Geth tracer would compute.
// The hash below is the real Keccak256 hash of the block header with these exact parameters.
// The size is the RLP-encoded size that Geth computes for this block.
// If you change any block parameters (timestamp, coinbase, etc.), you MUST recompute
// the hash and size by running the test and copying values from the native tracer output.
var TestBlock = (&BlockEventBuilder{}).
	Number(100).
	Hash("0xe74fcc728df762055c71a999736bb89dd47c541807c3021a1b94de6761afaf25"). // Computed by native Geth tracer with new addresses
	ParentHash("0x0000000000000000000000000000000000000000000000000000000000000063").
	Timestamp(1704067200).
	Coinbase(Miner).
	GasLimit(30_000_000). // 30M gas (standard Ethereum mainnet)
	Difficulty(big.NewInt(0)).
	Size(509).                // RLP-encoded block size computed by Geth
	Bloom(make([]byte, 256)). // Empty 256-byte logs bloom filter
	Build()

// TestLegacyTrx provides a legacy (type 0) test transaction
// The hash is computed at runtime by the native validator in OnTxStart
var TestLegacyTrx = new(TxEventBuilder).
	Type(TxTypeLegacy).
	Hash("0x0000000000000000000000000000000000000000000000000000000000000000"). // Placeholder, computed by native validator
	From(Alice).
	To(Bob).
	Value(bigInt(100)).   // 100 wei
	Gas(21000).           // Standard gas for simple transfer
	GasPrice(bigInt(10)). // 10 wei per gas
	Nonce(0).
	Build()

// TestAccessListTrx provides an EIP-2930 access list (type 1) test transaction
var TestAccessListTrx = new(TxEventBuilder).
	Type(TxTypeAccessList).
	Hash("0x0000000000000000000000000000000000000000000000000000000000000001"). // Placeholder
	From(Alice).
	To(Bob).
	Value(bigInt(100)).
	Gas(21000).
	GasPrice(bigInt(10)).
	Nonce(0).
	AccessList(firehose.AccessList{
		{Address: BobAddr, StorageKeys: [][32]byte{hashFromHex("0x01")}},
	}).
	Build()

// TestDynamicFeeTrx provides an EIP-1559 dynamic fee (type 2) test transaction
var TestDynamicFeeTrx = new(TxEventBuilder).
	Type(TxTypeDynamicFee).
	Hash("0x0000000000000000000000000000000000000000000000000000000000000002"). // Placeholder
	From(Alice).
	To(Bob).
	Value(bigInt(100)).
	Gas(21000).
	GasPrice(bigInt(10)).             // Effective gas price
	MaxFeePerGas(bigInt(20)).         // Max fee willing to pay
	MaxPriorityFeePerGas(bigInt(2)). // Priority fee (tip to miner)
	AccessList(firehose.AccessList{
		{Address: BobAddr, StorageKeys: [][32]byte{hashFromHex("0x01")}},
	}).
	Nonce(0).
	Build()

// TestBlobTrx provides an EIP-4844 blob (type 3) test transaction
var TestBlobTrx = new(TxEventBuilder).
	Type(TxTypeBlob).
	Hash("0x0000000000000000000000000000000000000000000000000000000000000003"). // Placeholder
	From(Alice).
	To(Bob).
	Value(bigInt(100)).
	Gas(21000).
	GasPrice(bigInt(10)).
	MaxFeePerGas(bigInt(20)).
	MaxPriorityFeePerGas(bigInt(2)).
	BlobGasFeeCap(bigInt(5)).
	BlobHashes([][32]byte{
		hashFromHex("0x0100000000000000000000000000000000000000000000000000000000000000"),
	}).
	Nonce(0).
	Build()

// TestSetCodeTrx provides an EIP-7702 set code (type 4) test transaction
// NOTE: This uses placeholder signatures. For proper validation tests,
// use CreateValidSetCodeTrxEvent() from eip7702_test.go
var TestSetCodeTrx = new(TxEventBuilder).
	Type(TxTypeSetCode).
	Hash("0x0000000000000000000000000000000000000000000000000000000000000004"). // Placeholder
	From(Alice).
	To(Bob).
	Value(bigInt(100)).
	Gas(21000).
	GasPrice(bigInt(10)).
	MaxFeePerGas(bigInt(20)).
	MaxPriorityFeePerGas(bigInt(2)).
	SetCodeAuthorizations([]firehose.SetCodeAuthorization{
		{
			ChainID: hashFromHex("0x01"),
			Address: CharlieAddr,
			Nonce:   0,
			V:       27,
			R:       hashFromHex("0x01"),
			S:       hashFromHex("0x01"),
		},
	}).
	Nonce(0).
	Build()

// TestTrx is the default test transaction (legacy type for backward compatibility)
var TestTrx = TestLegacyTrx

// TracerTester provides a fluent API for building test testers
type TracerTester struct {
	t *testing.T

	Block  *BlockEventBuilder
	Tracer *firehose.Tracer

	// Mock state reader for providing blockchain state to the tracer
	// Wraps the native validator's mockStateDB
	stateReader firehose.StateReader
}

// NewTracerTester creates a new tester builder with native validator
func NewTracerTester(t *testing.T) *TracerTester {
	return newTracerTesterWithConfig(t, &firehose.ChainConfig{
		ChainID: big.NewInt(1),
		// Use Ethereum-style signature recovery for SetCode authorizations
		SetCodeAuthRecovery: EthereumSetCodeAuthRecovery,
	})
}

// firehose.NewTracerTesterPrague creates a tracer tester with Prague fork enabled (for EIP-7702 testing)
func NewTracerTesterPrague(t *testing.T) *TracerTester {
	pragueTime := uint64(0) // Prague activated at genesis
	return newTracerTesterWithConfig(t, &firehose.ChainConfig{
		ChainID:             big.NewInt(1),
		PragueTime:          &pragueTime,
		SetCodeAuthRecovery: EthereumSetCodeAuthRecovery,
	})
}

// newTracerTesterWithConfig creates a tester with a specific chain config
func newTracerTesterWithConfig(t *testing.T, chainConfig *firehose.ChainConfig) *TracerTester {
	tester := &TracerTester{
		t: t,
		Tracer: firehose.NewTracer(&firehose.Config{
			ChainConfig:  chainConfig,
			OutputWriter: &bytes.Buffer{},
		}),
	}

	var err error
	nv, err := firehose.NewTestingNativeValidator("")
	require.NoError(t, err, "creating native validator")
	tester.Tracer.SetTestingNativeValidator(nv)

	// Create state reader wrapper around the native validator's mockStateDB
	stateDB := firehose.GetTestingStateDB(nv)
	tester.stateReader = firehose.NewTestingMockStateReader(stateDB)

	tester.Tracer.OnBlockchainInit("test", "1.0.0", chainConfig)

	return tester
}

// toCommonAddress converts a [20]byte address to common.Address
func toCommonAddress(addr [20]byte) common.Address {
	return common.Address(addr)
}

// toCommonHash converts a [32]byte hash to common.Hash
func toCommonHash(hash [32]byte) common.Hash {
	return common.Hash(hash)
}

// SetMockStateCode sets the code for an address in the mock StateDB
// This allows testing code paths that depend on StateDB.GetCode()
func (s *TracerTester) SetMockStateCode(addr [20]byte, code []byte) *TracerTester {
	nv := s.Tracer.GetTestingNativeValidator()
	if nv != nil {
		firehose.SetTestingMockStateCode(nv, toCommonAddress(addr), code)
	}
	return s
}

// SetMockStateNonce sets the nonce for an address in the mock StateDB
// This allows testing code paths that depend on StateDB.GetNonce()
func (s *TracerTester) SetMockStateNonce(addr [20]byte, nonce uint64) *TracerTester {
	nv := s.Tracer.GetTestingNativeValidator()
	if nv != nil {
		firehose.SetTestingMockStateNonce(nv, toCommonAddress(addr), nonce)
	}
	return s
}

// SetMockStateExist sets whether an address exists in the mock StateDB
// This allows testing code paths that depend on StateDB.Exist()
func (s *TracerTester) SetMockStateExist(addr [20]byte, exists bool) *TracerTester {
	nv := s.Tracer.GetTestingNativeValidator()
	if nv != nil {
		firehose.SetTestingMockStateExist(nv, toCommonAddress(addr), exists)
	}
	return s
}

func (s *TracerTester) StartBlock() *TracerTester {
	s.Tracer.OnBlockStart(TestBlock)
	return s
}

// StartBlockTrx starts a block and a transaction with standard Ethereum initialization
// This emits the state changes that happen at transaction start:
// - Nonce increment
// - Gas buying (balance decrease)
// - Gas initialization
// Uses TestTrx (legacy transaction) by default
func (s *TracerTester) StartBlockTrx() *TracerTester {
	return s.startBlockTrxWithEvent(TestTrx)
}

// StartBlockLegacyTrx starts a block and a legacy (type 0) transaction
func (s *TracerTester) StartBlockLegacyTrx() *TracerTester {
	return s.startBlockTrxWithEvent(TestLegacyTrx)
}

// StartBlockAccessListTrx starts a block and an EIP-2930 access list (type 1) transaction
func (s *TracerTester) StartBlockAccessListTrx() *TracerTester {
	return s.startBlockTrxWithEvent(TestAccessListTrx)
}

// StartBlockDynamicFeeTrx starts a block and an EIP-1559 dynamic fee (type 2) transaction
func (s *TracerTester) StartBlockDynamicFeeTrx() *TracerTester {
	return s.startBlockTrxWithEvent(TestDynamicFeeTrx)
}

// StartBlockBlobTrx starts a block and an EIP-4844 blob (type 3) transaction
func (s *TracerTester) StartBlockBlobTrx() *TracerTester {
	return s.startBlockTrxWithEvent(TestBlobTrx)
}

// StartBlockSetCodeTrx starts a block and an EIP-7702 set code (type 4) transaction
func (s *TracerTester) StartBlockSetCodeTrx() *TracerTester {
	return s.startBlockTrxWithEvent(TestSetCodeTrx)
}

// StartBlockTrxNoHooks starts a block and transaction WITHOUT automatic hooks
// This is useful for testing specific state changes in isolation without the
// automatic nonce increment, gas buy, and gas initialization that StartBlockTrx adds.
// Uses TestTrx (legacy transaction) by default.
func (s *TracerTester) StartBlockTrxNoHooks() *TracerTester {
	s.Tracer.OnBlockStart(TestBlock)
	tx := TestTrx
	tx.StateReader = s.stateReader
	s.Tracer.OnTxStart(tx)
	return s
}

// StartTrxNoHooks starts a transaction WITHOUT starting a block or automatic hooks
// Use this when you've already started a block (e.g., after system calls)
func (s *TracerTester) StartTrxNoHooks() *TracerTester {
	tx := TestTrx
	tx.StateReader = s.stateReader
	s.Tracer.OnTxStart(tx)
	return s
}

// startBlockTrxWithEvent is the internal implementation for starting a block with a specific transaction
func (s *TracerTester) startBlockTrxWithEvent(tx firehose.TxEvent) *TracerTester {
	s.Tracer.OnBlockStart(TestBlock)
	tx.StateReader = s.stateReader
	s.Tracer.OnTxStart(tx)

	// Standard Ethereum transaction initialization hooks
	from := tx.From

	// 1. Nonce change: increment sender's nonce
	s.Tracer.OnNonceChange(from, tx.Nonce, tx.Nonce+1)

	// 2. Gas buy: sender pays upfront gas cost (gas * gasPrice)
	gasCost := new(big.Int).Mul(new(big.Int).SetUint64(tx.Gas), tx.GasPrice)
	oldBalance := bigInt(1000000) // Assume sender has 1M wei
	newBalance := new(big.Int).Sub(oldBalance, gasCost)
	s.Tracer.OnBalanceChange(from, oldBalance, newBalance, pbeth.BalanceChange_REASON_GAS_BUY)

	// 3. Gas initialization: set initial gas for transaction
	s.Tracer.OnGasChange(0, tx.Gas, pbeth.GasChange_REASON_TX_INITIAL_BALANCE)

	return s
}

// StartCallRaw begins a call context with explicit depth and parameters
// Use this for nested calls or when you need full control over call depth
func (s *TracerTester) StartCallRaw(depth int, typ byte, from, to [20]byte, input []byte, gas uint64, value *big.Int) *TracerTester {
	s.Tracer.OnCallEnter(depth, typ, from, to, input, gas, value)
	return s
}

func (s *TracerTester) StartRootCall(from, to [20]byte, value *big.Int, gas uint64, input []byte) *TracerTester {
	return s.StartCallRaw(0, byte(firehose.CallTypeCall), from, to, input, gas, value)
}

func (s *TracerTester) StartRootCreateCall(from, to [20]byte, value *big.Int, gas uint64, input []byte) *TracerTester {
	return s.StartCallRaw(0, byte(firehose.CallTypeCreate), from, to, input, gas, value)
}

func (s *TracerTester) StartCall(depth int, from, to [20]byte, value *big.Int, gas uint64, input []byte) *TracerTester {
	return s.StartCallRaw(depth, byte(firehose.CallTypeCall), from, to, input, gas, value)
}

func (s *TracerTester) StartStaticCall(depth int, from, to [20]byte, gas uint64, input []byte) *TracerTester {
	return s.StartCallRaw(depth, byte(firehose.CallTypeStaticCall), from, to, input, gas, nil)
}

func (s *TracerTester) StartCreateCall(depth int, from, to [20]byte, value *big.Int, gas uint64, input []byte) *TracerTester {
	return s.StartCallRaw(depth, byte(firehose.CallTypeCreate), from, to, input, gas, value)
}

func (s *TracerTester) StartCreate2Call(depth int, from, to [20]byte, value *big.Int, gas uint64, input []byte) *TracerTester {
	return s.StartCallRaw(depth, byte(firehose.CallTypeCreate2), from, to, input, gas, value)
}

func (s *TracerTester) StartDelegateCall(depth int, from, to [20]byte, value *big.Int, gas uint64, input []byte) *TracerTester {
	return s.StartCallRaw(depth, byte(firehose.CallTypeDelegateCall), from, to, input, gas, value)
}

func (s *TracerTester) StartCallCode(depth int, from, to [20]byte, value *big.Int, gas uint64, input []byte) *TracerTester {
	return s.StartCallRaw(depth, byte(firehose.CallTypeCallCode), from, to, input, gas, value)
}

// EndCall ends a call context successfully
func (s *TracerTester) EndCall(output []byte, gasUsed uint64, err error) *TracerTester {
	s.Tracer.OnCallExit(output, gasUsed, err)
	return s
}

// firehose.OpCode simulates an opcode execution to trigger ExecutedCode setting
// This ensures both shared and native tracers set ExecutedCode correctly
func (s *TracerTester) OpCode(pc uint64, op byte, gas, cost uint64) *TracerTester {
	// Call shared tracer's OnOpcode with empty scope data
	emptyScope := firehose.OpcodeScopeData{}
	s.Tracer.OnOpcode(pc, op, gas, cost, emptyScope, nil, 0, nil)

	// Also call native validator's OnOpcode if present (different signature)
	nv := s.Tracer.GetTestingNativeValidator()
	if nv != nil {
		firehose.CallTestingNativeValidatorOnOpcode(nv, pc, op, gas, cost, 0)
	}

	return s
}

// Keccak simulates a KECCAK256 opcode execution with preimage capture
// This ensures both shared and native tracers store keccak preimages correctly
func (s *TracerTester) Keccak(hash [32]byte, preimage []byte) *TracerTester {
	// Call shared tracer's OnKeccakPreimage
	s.Tracer.OnKeccakPreimage(hash, preimage)

	// Also call native validator's OnKeccakPreimage if present
	nv := s.Tracer.GetTestingNativeValidator()
	if nv != nil {
		firehose.CallTestingNativeValidatorOnKeccakPreimage(nv, hash, preimage)
	}

	return s
}

// EndCallReverted ends a call context with a revert error
// Wraps testErrExecutionReverted so tracer recognizes it as reverted
func (s *TracerTester) EndCallReverted(output []byte, gasUsed uint64, reason string) *TracerTester {
	err := &wrapError{
		reason:  reason,
		wrapped: testErrExecutionReverted,
	}
	s.Tracer.OnCallExit(output, gasUsed, err)
	return s
}

// EndCallFailed ends a call context with a non-revert failure
// Uses errors that are recognized as failures but not reverts
func (s *TracerTester) EndCallFailed(output []byte, gasUsed uint64, reason string) *TracerTester {
	// Map common failure reasons to their appropriate errors
	var wrapped error
	switch reason {
	case "out of gas":
		wrapped = testErrOutOfGas
	default:
		// Generic error - not recognized as revert
		wrapped = errors.New(reason)
	}

	err := &wrapError{
		reason:  reason,
		wrapped: wrapped,
	}
	s.Tracer.OnCallExit(output, gasUsed, err)
	return s
}

// BalanceChange records a balance change
func (s *TracerTester) BalanceChange(addr [20]byte, oldBalance, newBalance *big.Int, reason pbeth.BalanceChange_Reason) *TracerTester {
	s.Tracer.OnBalanceChange(addr, oldBalance, newBalance, reason)
	return s
}

// NonceChange records a nonce change
func (s *TracerTester) NonceChange(addr [20]byte, oldNonce, newNonce uint64) *TracerTester {
	s.Tracer.OnNonceChange(addr, oldNonce, newNonce)
	return s
}

// CodeChange records a code change
func (s *TracerTester) CodeChange(addr [20]byte, prevCodeHash, newCodeHash [32]byte, oldCode, newCode []byte) *TracerTester {
	s.Tracer.OnCodeChange(addr, prevCodeHash, newCodeHash, oldCode, newCode)
	return s
}

// StorageChange records a storage change
func (s *TracerTester) StorageChange(addr [20]byte, slot, oldValue, newValue [32]byte) *TracerTester {
	s.Tracer.OnStorageChange(addr, slot, oldValue, newValue)
	return s
}

// GasChange records a gas change
func (s *TracerTester) GasChange(oldGas, newGas uint64, reason pbeth.GasChange_Reason) *TracerTester {
	s.Tracer.OnGasChange(oldGas, newGas, reason)
	return s
}

// Log records a log event
func (s *TracerTester) Log(addr [20]byte, topics [][32]byte, data []byte, blockIndex uint32) *TracerTester {
	s.Tracer.OnLog(addr, topics, data, blockIndex)
	return s
}

// Suicide simulates a SELFDESTRUCT operation with proper Ethereum state changes
// This follows the Geth native tracer's behavior where SELFDESTRUCT triggers:
// 1. OnOpcode(SELFDESTRUCT) - marks call.Suicide = true
// 2. OnCallEnter(SELFDESTRUCT) - sets latestCallEnterSuicided flag
// 3. Balance changes (SUICIDE_WITHDRAW then SUICIDE_REFUND)
// 4. OnCallExit - clears the latestCallEnterSuicided flag
//
// Parameters:
// - contractAddr: the address of the contract being destructed
// - beneficiaryAddr: the address receiving the contract's balance
// - contractBalance: the balance of the contract before suicide
//
// Note: Since OnOpcode isn't exposed in tests, we manually mark the call as suicided
func (s *TracerTester) Suicide(contractAddr, beneficiaryAddr [20]byte, contractBalance *big.Int) *TracerTester {
	// Step 1: Simulate OnOpcode(SELFDESTRUCT) - marks active call as suicided
	// (matching native tracer firehose.go:1191-1193)
	activeCallDepth := s.Tracer.GetTestingCallStackDepth() - 1 // Depth of the active call (0 for root call)

	// Mark the shared tracer's active call as suicided and executed
	// OnOpcode in the native tracer sets both Suicide and ExecutedCode
	activeCall := s.Tracer.GetTestingCallStackPeek()
	if activeCall != nil {
		firehose.SetTestingCallSuicide(activeCall, true)
		firehose.SetTestingCallExecutedCode(activeCall, true) // Set by captureInterpreterStep in native tracer
	}

	// Call OnOpcode for native validator to mark its call as suicided and executed
	nv := s.Tracer.GetTestingNativeValidator()
	if nv != nil {
		firehose.CallTestingNativeValidatorOnOpcode(nv, 0, 0xff, 0, 0, activeCallDepth) // op=0xff is SELFDESTRUCT
	}

	// Step 2: Trigger OnCallEnter(SELFDESTRUCT) at depth = active_call_depth + 1
	// This sets latestCallEnterSuicided flag (matching firehose.go:1040-1041)
	selfDestructDepth := activeCallDepth + 1 // SELFDESTRUCT is signaled as a nested operation

	s.Tracer.OnCallEnter(
		selfDestructDepth,
		byte(firehose.CallTypeSelfDestruct),
		contractAddr,      // from: contract being destructed
		beneficiaryAddr,   // to: beneficiary receiving balance
		[]byte{},          // input: empty for SELFDESTRUCT
		0,                 // gas: not relevant for SELFDESTRUCT
		contractBalance,   // value: contract balance being transferred
	)

	// Apply balance changes in the order Ethereum emits them:
	// 1. SUICIDE_WITHDRAW: Contract balance goes to 0
	s.Tracer.OnBalanceChange(
		contractAddr,
		contractBalance,
		big.NewInt(0),
		pbeth.BalanceChange_REASON_SUICIDE_WITHDRAW,
	)

	// 2. SUICIDE_REFUND: Beneficiary receives the balance
	var beneficiaryOldBalance *big.Int
	if contractAddr == beneficiaryAddr {
		// Special case: suicide to self
		beneficiaryOldBalance = big.NewInt(0)
	} else {
		// Normal case: assume beneficiary starts at 0 for simplicity
		beneficiaryOldBalance = big.NewInt(0)
	}

	s.Tracer.OnBalanceChange(
		beneficiaryAddr,
		beneficiaryOldBalance,
		new(big.Int).Add(beneficiaryOldBalance, contractBalance),
		pbeth.BalanceChange_REASON_SUICIDE_REFUND,
	)

	// Geth calls OnCallExit for SELFDESTRUCT at the same depth as OnCallEnter
	// The Firehose tracer will check latestCallEnterSuicided and skip processing
	// This clears the flag so subsequent OnCallExit (for the real call) works correctly
	//
	// We need to call the native validator directly with the correct depth,
	// because the shared tracer's OnCallExit computes depth from callStack
	if nv != nil {
		firehose.CallTestingNativeValidatorOnCallExit(nv, selfDestructDepth, []byte{}, 0, nil, false)
	}

	// Also need to clear the flag in the shared tracer
	if s.Tracer.GetTestingLatestCallEnterSuicided() {
		s.Tracer.SetTestingLatestCallEnterSuicided(false)
	}

	return s
}

// StartSystemCall starts a system call
// System calls are special protocol-level calls that happen outside of regular transactions
// Examples: Beacon root updates (EIP-4788), parent hash storage (EIP-2935), withdrawal queue (EIP-7002)
func (s *TracerTester) StartSystemCall() *TracerTester {
	s.Tracer.OnSystemCallStart()
	return s
}

// EndSystemCall ends a system call
func (s *TracerTester) EndSystemCall() *TracerTester {
	s.Tracer.OnSystemCallEnd()
	return s
}

// SystemCall simulates a complete system call with a single CALL operation
// This is a convenience helper for common system calls that make one contract call
//
// Parameters:
// - from: caller address (typically SystemAddress 0xff...fe)
// - to: target contract address (e.g., BeaconRootsAddress, HistoryStorageAddress)
// - input: call data
// - gas: gas limit for the call
// - output: return data from the call
// - gasUsed: gas consumed by the call
//
// Example: Beacon root system call (EIP-4788)
//   SystemCall(SystemAddress, BeaconRootsAddress, beaconRoot[:], 30000000, []byte{}, 50000)
func (s *TracerTester) SystemCall(from, to [20]byte, input []byte, gas uint64, output []byte, gasUsed uint64) *TracerTester {
	s.Tracer.OnSystemCallStart()
	s.Tracer.OnCallEnter(0, byte(firehose.CallTypeCall), from, to, input, gas, big.NewInt(0))
	s.Tracer.OnCallExit(output, gasUsed, nil)
	s.Tracer.OnSystemCallEnd()
	return s
}

// EndTrx ends the current transaction without ending the block
// Use this when you have multiple transactions in the same block
func (s *TracerTester) EndTrx(receipt *firehose.ReceiptData, txErr error) *TracerTester {
	s.Tracer.OnTxEnd(receipt, txErr)
	return s
}

// EndBlockTrx ends the transaction and block with an optional error
func (s *TracerTester) EndBlockTrx(receipt *firehose.ReceiptData, txErr, blockErr error) *TracerTester {
	s.Tracer.OnTxEnd(receipt, txErr)
	s.Tracer.OnBlockEnd(blockErr)
	return s
}

func (s *TracerTester) EndBlock(err error) *TracerTester {
	s.Tracer.OnBlockEnd(err)
	return s
}

// GenesisBlock processes a genesis block with the given allocation
// This creates a complete genesis block trace with deterministic ordering
func (s *TracerTester) GenesisBlock(blockNumber uint64, blockHash [32]byte, alloc firehose.GenesisAlloc) *TracerTester {
	// Standard genesis block header values
	// EmptyUncleHash = 1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347
	emptyUncleHash := mustHash32FromHex("1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347")
	// EmptyTxsHash = EmptyReceiptsHash = 56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421
	emptyTxsHash := mustHash32FromHex("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")

	// Create a properly formed genesis block header that will hash deterministically
	// We use the provided blockHash as the state root, and let go-ethereum compute the block hash from the header
	header := &types.Header{
		ParentHash:  common.Hash{},                          // Genesis has no parent
		UncleHash:   common.Hash(emptyUncleHash),            // Standard empty uncle hash
		Coinbase:    common.Address{},                       // Zero address
		Root:        common.Hash(blockHash),                 // Use provided hash as state root (for testing)
		TxHash:      common.Hash(emptyTxsHash),              // Standard empty transactions hash
		ReceiptHash: common.Hash(emptyTxsHash),              // Standard empty receipts hash
		Bloom:       types.Bloom{},                          // Empty bloom filter
		Difficulty:  big.NewInt(0),                          // PoS blocks have zero difficulty
		Number:      big.NewInt(int64(blockNumber)),         // Block number
		GasLimit:    8000000,                                // Default gas limit
		GasUsed:     0,                                      // Genesis has no gas used
		Time:        0,                                      // Genesis time
		Extra:       nil,                                    // No extra data
		MixDigest:   common.Hash{},                          // Empty mix digest
		Nonce:       types.BlockNonce{},                     // Empty nonce
		BaseFee:     nil,                                    // No base fee for genesis
	}

	// Compute the actual block hash from the header using go-ethereum's native hash function
	// This ensures both the shared tracer and native validator use the same hash
	computedHash := header.Hash()

	// Create a types.Block to compute the block size (RLP-encoded size)
	block := types.NewBlockWithHeader(header)
	blockSize := block.Size()

	event := firehose.BlockEvent{
		Block: firehose.BlockData{
			Number:      blockNumber,
			Hash:        [32]byte(computedHash),     // Use computed hash
			ParentHash:  [32]byte{},                 // Genesis has no parent
			UncleHash:   emptyUncleHash,             // Standard empty uncle hash
			Coinbase:    [20]byte{},                 // Zero address
			Root:        blockHash,                  // State root (provided by test)
			TxHash:      emptyTxsHash,               // Standard empty transactions hash
			ReceiptHash: emptyTxsHash,               // Standard empty receipts hash
			Bloom:       make([]byte, 256),          // Empty 256-byte logs bloom filter
			Difficulty:  big.NewInt(0),              // PoS blocks have zero difficulty
			GasLimit:    8000000,                    // Default gas limit
			GasUsed:     0,                          // Genesis has no gas used
			Time:        0,
			Extra:       nil,
			MixDigest:   [32]byte{},
			Nonce:       0,
			BaseFee:     nil,
			Size:        blockSize,                  // Computed RLP-encoded block size
		},
	}

	s.Tracer.OnGenesisBlock(event, alloc)

	return s
}

func (s *TracerTester) Validate(validateFunc func(block *pbeth.Block)) {
	block := ParseFirehoseBlock(s.t, "shared tracer", s.Tracer.GetTestingOutputWriter())

	nv := s.Tracer.GetTestingNativeValidator()
	require.NotNil(s.t, nv, "native validator should be configured for testing")

	nativeBlock := ParseFirehoseBlock(s.t, "native tracer", firehose.GetTestingNativeValidatorBuffer(nv))

	if !proto.Equal(block, nativeBlock) {
		require.EqualExportedValues(s.t, nativeBlock, block)
	}

	validateFunc(block)
}

// ParseFirehoseBlock parses a block from FIRE BLOCK output format
func ParseFirehoseBlock(t *testing.T, tag string, buffer *bytes.Buffer) *pbeth.Block {
	scanner := bufio.NewScanner(buffer)

	var initSeen bool
	var block *pbeth.Block

	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines
		if len(line) == 0 {
			continue
		}

		// Parse FIRE INIT
		if strings.HasPrefix(line, "FIRE INIT ") {
			parts := strings.SplitN(line, " ", 4)
			require.GreaterOrEqual(t, len(parts), 4, "For %s: FIRE INIT line should have at least 4 parts", tag)

			version := parts[2]
			require.Contains(t, []string{"3.0", "3.1"}, version, "For %s: protocol version should be 3.0 or 3.1", tag)

			initSeen = true
			continue
		}

		// Parse FIRE BLOCK
		if strings.HasPrefix(line, "FIRE BLOCK ") {
			require.True(t, initSeen, "For %s: FIRE INIT must appear before FIRE BLOCK", tag)

			// FIRE BLOCK <block_num> <block_hash> <parent_num> <parent_hash> <lib_num> <timestamp_unix_nano> <payload_base64>
			parts := strings.SplitN(line, " ", 9)
			require.GreaterOrEqual(t, len(parts), 9, "For %s: FIRE BLOCK line should have 9 parts", tag)

			// Extract base64-encoded payload (last field)
			payloadBase64 := parts[8]

			// Decode base64
			payloadBytes, err := base64.StdEncoding.DecodeString(payloadBase64)
			require.NoError(t, err, "For %s: base64 payload decode", tag)

			// Unmarshal protobuf
			block = &pbeth.Block{}
			err = proto.Unmarshal(payloadBytes, block)
			require.NoError(t, err, "For %s: protobuf unmarshal", tag)

			// Validate fields match (for integrity)
			blockNum, err := strconv.ParseUint(parts[2], 10, 64)
			require.NoError(t, err, "For %s: parse block number from FIRE BLOCK header", tag)
			require.Equal(t, blockNum, block.Number, "For %s: block number in header should match protobuf", tag)

			// We found the block, return it
			return block
		}
	}

	require.NoError(t, scanner.Err(), "For %s: reading buffer", tag)
	require.Fail(t, "For %s: no FIRE BLOCK found in buffer", tag)
	return nil
}
