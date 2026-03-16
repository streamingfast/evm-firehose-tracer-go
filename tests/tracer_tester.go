package tests

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"math/big"
	"strconv"
	"strings"
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go/v5"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// Common test addresses (for readability in tests)
// These are derived from deterministic private keys defined in testing_helpers.go
var (
	Alice   = "0x7e5f4552091a69125d5dfcb7b8c2659029395bdf"
	Bob     = "0x2b5ad5c4795c026514f8317c7a215e218dccd6cf"
	Charlie = "0x6813eb9362372eef6200f3b1dbc3f819671cba69"
	Miner   = "0x1eff47bc3a10a45d4b230b5d10e37751fe6aa718"
)

// Test error constants matching VM errors text
// The shared tracer uses errorIsString which matches on error text
// These error messages match go-ethereum's VM errors exactly
var (
	testErrExecutionReverted           = errors.New("execution reverted")
	testErrInsufficientBalanceTransfer = errors.New("insufficient balance for transfer")
	testErrMaxCallDepth                = errors.New("max call depth exceeded")
	testErrOutOfGas                    = errors.New("out of gas")
	testErrCodeStoreOutOfGas           = errors.New("contract creation code storage out of gas")
)

// TestBlock provides a standard test block with reasonable defaults
// This block represents block #100 with typical Ethereum mainnet settings
//
// IMPORTANT: Until native validator code is removed, this block MUST produce
// the exact same hash and size as the native Geth tracer would compute.
// The hash below is the real Keccak256 hash of the block header with these exact parameters.
// The size is the RLP-encoded size that Geth computes for this block.
// If you change any block parameters (timestamp, coinbase, etc.), you MUST recompute
// the hash and size by running the test and copying values from the native tracer output.
var TestBlock = (&firehose.BlockEventBuilder{}).
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
var TestLegacyTrx = new(firehose.TxEventBuilder).
	Type(firehose.TxTypeLegacy).
	Hash("0x0000000000000000000000000000000000000000000000000000000000000000"). // Placeholder, computed by native validator
	From(Alice).
	To(Bob).
	Value(bigInt(100)).   // 100 wei
	Gas(21000).           // Standard gas for simple transfer
	GasPrice(bigInt(10)). // 10 wei per gas
	Nonce(0).
	Build()

// TestAccessListTrx provides an EIP-2930 access list (type 1) test transaction
var TestAccessListTrx = new(firehose.TxEventBuilder).
	Type(firehose.TxTypeAccessList).
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
var TestDynamicFeeTrx = new(firehose.TxEventBuilder).
	Type(firehose.TxTypeDynamicFee).
	Hash("0x0000000000000000000000000000000000000000000000000000000000000002"). // Placeholder
	From(Alice).
	To(Bob).
	Value(bigInt(100)).
	Gas(21000).
	GasPrice(bigInt(10)).            // Effective gas price
	MaxFeePerGas(bigInt(20)).        // Max fee willing to pay
	MaxPriorityFeePerGas(bigInt(2)). // Priority fee (tip to miner)
	AccessList(firehose.AccessList{
		{Address: BobAddr, StorageKeys: [][32]byte{hashFromHex("0x01")}},
	}).
	Nonce(0).
	Build()

// TestBlobTrx provides an EIP-4844 blob (type 3) test transaction
var TestBlobTrx = new(firehose.TxEventBuilder).
	Type(firehose.TxTypeBlob).
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
var TestSetCodeTrx = new(firehose.TxEventBuilder).
	Type(firehose.TxTypeSetCode).
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

// TracerTester provides a fluent API for building test testers
type TracerTester struct {
	t *testing.T

	tracer *firehose.Tracer

	// mockStateDB for providing blockchain state to the tracer
	// Wraps the native validator's mockStateDB
	mockStateDB *mockStateDB

	// Current call depth (0 = root call, 1 = first nested call, etc.)
	// Automatically managed by StartCall*/EndCall methods
	depth int

	// Block-wide log index counter (0-based)
	// Automatically incremented by Log() and used to populate BlockIndex in receipt LogData
	blockLogIndex uint32
}

// NewTracerTester creates a new tester builder with native validator
func NewTracerTester(t *testing.T) *TracerTester {
	return newTracerTesterWithConfig(t, &firehose.ChainConfig{
		ChainID: big.NewInt(1),
	})
}

// firehose.NewTracerTesterPrague creates a tracer tester with Prague fork enabled (for EIP-7702 testing)
func NewTracerTesterPrague(t *testing.T) *TracerTester {
	pragueTime := uint64(0) // Prague activated at genesis

	return newTracerTesterWithConfig(t, &firehose.ChainConfig{
		ChainID:    big.NewInt(1),
		PragueTime: &pragueTime,
	})
}

// newTracerTesterWithConfig creates a tester with a specific chain config
func newTracerTesterWithConfig(t *testing.T, chainConfig *firehose.ChainConfig) *TracerTester {
	tester := &TracerTester{
		t: t,
		tracer: firehose.NewTracer(&firehose.Config{
			ChainConfig:  chainConfig,
			OutputWriter: &bytes.Buffer{},
		}),
		mockStateDB: newMockStateDB(),
	}

	tester.tracer.OnBlockchainInit("test", "1.0.0", chainConfig)

	return tester
}

// SetMockStateCode sets the code for an address in the mock StateDB
// This allows testing code paths that depend on StateDB.GetCode()
func (s *TracerTester) SetMockStateCode(addr [20]byte, code []byte) *TracerTester {
	s.mockStateDB.SetCode(addr, code)
	return s
}

// SetMockStateNonce sets the nonce for an address in the mock StateDB
// This allows testing code paths that depend on StateDB.GetNonce()
func (s *TracerTester) SetMockStateNonce(addr [20]byte, nonce uint64) *TracerTester {
	s.mockStateDB.SetNonce(addr, nonce)
	return s
}

// SetMockStateExist sets whether an address exists in the mock StateDB
// This allows testing code paths that depend on StateDB.Exist()
func (s *TracerTester) SetMockStateExist(addr [20]byte, exists bool) *TracerTester {
	s.mockStateDB.SetExist(addr, exists)
	return s
}

func (s *TracerTester) StartBlock() *TracerTester {
	s.tracer.OnBlockStart(TestBlock)
	s.blockLogIndex = 0 // Reset log counter for new block
	return s
}

// StartBlockTrx starts a block and a transaction
func (s *TracerTester) StartBlockTrx(tx firehose.TxEvent) *TracerTester {
	s.tracer.OnBlockStart(TestBlock)
	s.blockLogIndex = 0 // Reset log counter for new block
	s.tracer.OnTxStart(tx, s.mockStateDB)
	return s
}

// StartTrx starts a transaction without starting a block. Use this for testing transaction tracing in isolation.
func (s *TracerTester) StartTrx(tx firehose.TxEvent) *TracerTester {
	s.tracer.OnTxStart(tx, s.mockStateDB)
	return s
}

func (s *TracerTester) StartCall(from, to [20]byte, value *big.Int, gas uint64, input []byte) *TracerTester {
	return s.startCallRaw(byte(firehose.CallTypeCall), from, to, input, gas, value)
}

func (s *TracerTester) StartStaticCall(from, to [20]byte, gas uint64, input []byte) *TracerTester {
	return s.startCallRaw(byte(firehose.CallTypeStaticCall), from, to, input, gas, nil)
}

func (s *TracerTester) StartCreateCall(from, to [20]byte, value *big.Int, gas uint64, input []byte) *TracerTester {
	return s.startCallRaw(byte(firehose.CallTypeCreate), from, to, input, gas, value)
}

func (s *TracerTester) StartCreate2Call(from, to [20]byte, value *big.Int, gas uint64, input []byte) *TracerTester {
	return s.startCallRaw(byte(firehose.CallTypeCreate2), from, to, input, gas, value)
}

func (s *TracerTester) StartDelegateCall(from, to [20]byte, value *big.Int, gas uint64, input []byte) *TracerTester {
	return s.startCallRaw(byte(firehose.CallTypeDelegateCall), from, to, input, gas, value)
}

func (s *TracerTester) StartCallCode(from, to [20]byte, value *big.Int, gas uint64, input []byte) *TracerTester {
	return s.startCallRaw(byte(firehose.CallTypeCallCode), from, to, input, gas, value)
}

func (s *TracerTester) startCallRaw(typ byte, from, to [20]byte, input []byte, gas uint64, value *big.Int) *TracerTester {
	s.tracer.OnCallEnter(s.depth, typ, from, to, input, gas, value)
	s.depth++
	return s
}

// EndCall ends a call context successfully
// Automatically manages depth: decrements depth and passes it to OnCallExit
func (s *TracerTester) EndCall(output []byte, gasUsed uint64) *TracerTester {
	s.depth--
	s.tracer.OnCallExit(s.depth, output, gasUsed, nil, false)
	return s
}

// EndCallFailed ends a call context with an error
// Automatically manages depth: decrements depth and passes it to OnCallExit
func (s *TracerTester) EndCallFailed(output []byte, gasUsed uint64, err error, reverted bool) *TracerTester {
	s.depth--
	s.tracer.OnCallExit(s.depth, output, gasUsed, err, reverted)
	return s
}

// firehose.OpCode simulates an opcode execution to trigger ExecutedCode setting
// This ensures both shared and native tracers set ExecutedCode correctly
func (s *TracerTester) OpCode(pc uint64, op byte, gas, cost uint64) *TracerTester {
	// Call shared tracer's OnOpcode
	// This will call the native validator internally if present
	activeCallDepth := s.depth - 1 // Depth of the active call
	s.tracer.OnOpcode(pc, op, gas, cost, []byte{}, activeCallDepth, nil)

	return s
}

// Keccak simulates a KECCAK256 opcode execution with preimage capture
// This ensures both shared and native tracers store keccak preimages correctly
func (s *TracerTester) Keccak(hash [32]byte, preimage []byte) *TracerTester {
	// Call shared tracer's OnKeccakPreimage
	// This will call the native validator internally if present
	s.tracer.OnKeccakPreimage(hash, preimage)

	return s
}

// OpCodeFault simulates an opcode fault during execution
// This ensures both shared and native tracers handle opcode faults correctly
func (s *TracerTester) OpCodeFault(pc uint64, op byte, gas, cost uint64, err error) *TracerTester {
	// Call shared tracer's OnOpcodeFault
	// This will call the native validator internally if present
	activeCallDepth := s.depth - 1 // Depth of the active call
	s.tracer.OnOpcodeFault(pc, op, gas, cost, activeCallDepth, err)

	return s
}

// BalanceChange records a balance change
func (s *TracerTester) BalanceChange(addr [20]byte, oldBalance, newBalance *big.Int, reason pbeth.BalanceChange_Reason) *TracerTester {
	s.tracer.OnBalanceChange(addr, oldBalance, newBalance, reason)
	return s
}

// NonceChange records a nonce change
func (s *TracerTester) NonceChange(addr [20]byte, oldNonce, newNonce uint64) *TracerTester {
	s.tracer.OnNonceChange(addr, oldNonce, newNonce)
	return s
}

// CodeChange records a code change
func (s *TracerTester) CodeChange(addr [20]byte, prevCodeHash, newCodeHash [32]byte, oldCode, newCode []byte) *TracerTester {
	s.tracer.OnCodeChange(addr, prevCodeHash, newCodeHash, oldCode, newCode)
	return s
}

// StorageChange records a storage change
func (s *TracerTester) StorageChange(addr [20]byte, slot, oldValue, newValue [32]byte) *TracerTester {
	s.tracer.OnStorageChange(addr, slot, oldValue, newValue)
	return s
}

// Log records a log event
// The blockIndex parameter is used for OnLog (call logs), while s.blockLogIndex
// tracks the index for receipt logs (auto-populated in EndBlockTrx/EndTrx)
func (s *TracerTester) Log(addr [20]byte, topics [][32]byte, data []byte, blockIndex uint32) *TracerTester {
	s.tracer.OnLog(addr, topics, data, blockIndex)
	s.blockLogIndex++ // Increment for next log
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
	// Step 1: Call OnOpcode(SELFDESTRUCT) - marks active call as suicided and executed
	// (matching native tracer firehose.go:1191-1193)
	activeCallDepth := s.depth - 1 // Depth of the active call (depth-1 since we incremented after StartCall)

	// Call shared tracer's OnOpcode to mark the call as suicided and executed
	// This will:
	// - Set call.Suicide = true (for SELFDESTRUCT opcode)
	// - Set call.ExecutedCode = true (always set by OnOpcode)
	// - Call the native validator internally if present
	s.tracer.OnOpcode(0, 0xff, 0, 0, []byte{}, activeCallDepth, nil) // op=0xff is SELFDESTRUCT

	// Step 2: Trigger OnCallEnter(SELFDESTRUCT) at depth = active_call_depth + 1
	// This sets latestCallEnterSuicided flag (matching firehose.go:1040-1041)
	selfDestructDepth := activeCallDepth + 1 // SELFDESTRUCT is signaled as a nested operation

	s.tracer.OnCallEnter(
		selfDestructDepth,
		byte(firehose.CallTypeSelfDestruct),
		contractAddr,    // from: contract being destructed
		beneficiaryAddr, // to: beneficiary receiving balance
		[]byte{},        // input: empty for SELFDESTRUCT
		0,               // gas: not relevant for SELFDESTRUCT
		contractBalance, // value: contract balance being transferred
	)

	// Apply balance changes in the order Ethereum emits them:
	// 1. SUICIDE_WITHDRAW: Contract balance goes to 0
	s.tracer.OnBalanceChange(
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

	s.tracer.OnBalanceChange(
		beneficiaryAddr,
		beneficiaryOldBalance,
		new(big.Int).Add(beneficiaryOldBalance, contractBalance),
		pbeth.BalanceChange_REASON_SUICIDE_REFUND,
	)

	// Geth calls OnCallExit for SELFDESTRUCT at the same depth as OnCallEnter
	// The shared tracer's OnCallExit will:
	// - Call the native validator with the correct depth
	// - Check latestCallEnterSuicided and skip processing
	// - Clear the flag so subsequent OnCallExit (for the real call) works correctly
	s.tracer.OnCallExit(selfDestructDepth, []byte{}, 0, nil, false)

	return s
}

// StartSystemCall starts a system call
// System calls are special protocol-level calls that happen outside of regular transactions
// Examples: Beacon root updates (EIP-4788), parent hash storage (EIP-2935), withdrawal queue (EIP-7002)
func (s *TracerTester) StartSystemCall() *TracerTester {
	s.tracer.OnSystemCallStart()
	return s
}

// EndSystemCall ends a system call
func (s *TracerTester) EndSystemCall() *TracerTester {
	s.tracer.OnSystemCallEnd()
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
//
//	SystemCall(SystemAddress, BeaconRootsAddress, beaconRoot[:], 30000000, []byte{}, 50000)
func (s *TracerTester) SystemCall(from, to [20]byte, input []byte, gas uint64, output []byte, gasUsed uint64) *TracerTester {
	s.tracer.OnSystemCallStart()
	s.tracer.OnCallEnter(0, byte(firehose.CallTypeCall), from, to, input, gas, big.NewInt(0))
	s.tracer.OnCallExit(0, output, gasUsed, nil, false) // System calls are at depth 0
	s.tracer.OnSystemCallEnd()
	return s
}

// EndTrx ends the current transaction without ending the block
// Use this when you have multiple transactions in the same block
func (s *TracerTester) EndTrx(receipt *firehose.ReceiptData, txErr error) *TracerTester {
	s.populateReceiptLogBlockIndex(receipt)
	s.tracer.OnTxEnd(receipt, txErr)
	return s
}

// EndBlockTrx ends the transaction and block with an optional error
func (s *TracerTester) EndBlockTrx(receipt *firehose.ReceiptData, txErr, blockErr error) *TracerTester {
	s.populateReceiptLogBlockIndex(receipt)
	s.tracer.OnTxEnd(receipt, txErr)
	s.tracer.OnBlockEnd(blockErr)
	return s
}

// populateReceiptLogBlockIndex automatically populates BlockIndex in receipt LogData
// This matches go-ethereum behavior where BlockIndex is prepopulated in receipt logs
func (s *TracerTester) populateReceiptLogBlockIndex(receipt *firehose.ReceiptData) {
	if receipt == nil {
		return
	}

	// Populate BlockIndex in receipt logs (0-based, matching the order logs were added)
	for i := range receipt.Logs {
		receipt.Logs[i].BlockIndex = uint32(i)
	}
}

func (s *TracerTester) EndBlock(err error) *TracerTester {
	s.tracer.OnBlockEnd(err)
	return s
}

// GenesisBlock processes a genesis block with the given allocation
// This creates a complete genesis block trace with deterministic ordering
func (s *TracerTester) GenesisBlock(blockNumber uint64, stateRoot [32]byte, alloc firehose.GenesisAlloc) *TracerTester {
	// Standard genesis block header values
	// EmptyUncleHash = 1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347
	emptyUncleHash := mustHash32FromHex("1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347")
	// EmptyTxsHash = EmptyReceiptsHash = 56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421
	emptyTxsHash := mustHash32FromHex("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")

	// For genesis blocks, compute a deterministic block hash from the state root
	// This is sufficient for testing purposes
	blockHash := computeGenesisBlockHash(blockNumber, stateRoot, emptyUncleHash, emptyTxsHash)

	// Genesis blocks typically RLP-encode to ~500-600 bytes
	// This is a reasonable estimate for testing
	blockSize := uint64(539)

	event := firehose.BlockEvent{
		Block: firehose.BlockData{
			Number:      blockNumber,
			Hash:        blockHash,         // Computed hash
			ParentHash:  [32]byte{},        // Genesis has no parent
			UncleHash:   emptyUncleHash,    // Standard empty uncle hash
			Coinbase:    [20]byte{},        // Zero address
			Root:        stateRoot,         // State root (provided by test)
			TxHash:      emptyTxsHash,      // Standard empty transactions hash
			ReceiptHash: emptyTxsHash,      // Standard empty receipts hash
			Bloom:       make([]byte, 256), // Empty 256-byte logs bloom filter
			Difficulty:  big.NewInt(0),     // PoS blocks have zero difficulty
			GasLimit:    8000000,           // Default gas limit
			GasUsed:     0,                 // Genesis has no gas used
			Time:        0,
			Extra:       nil,
			MixDigest:   [32]byte{},
			Nonce:       0,
			BaseFee:     nil,
			Size:        blockSize, // Standard size for genesis blocks
		},
	}

	s.tracer.OnGenesisBlock(event, alloc)

	return s
}

// computeGenesisBlockHash computes a deterministic hash for a genesis block
// This is a simplified version for testing that doesn't match Ethereum's exact RLP encoding
// but provides deterministic hashes for test validation
func computeGenesisBlockHash(blockNumber uint64, stateRoot, uncleHash, txHash [32]byte) [32]byte {
	// Create a simple deterministic hash by hashing key block components
	// This doesn't match Ethereum's exact RLP encoding but is sufficient for tests
	data := make([]byte, 0, 32+8+32+32)

	// Add state root
	data = append(data, stateRoot[:]...)

	// Add block number (8 bytes, big-endian)
	blockNumBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		blockNumBytes[7-i] = byte(blockNumber >> (i * 8))
	}
	data = append(data, blockNumBytes...)

	// Add uncle hash
	data = append(data, uncleHash[:]...)

	// Add tx hash
	data = append(data, txHash[:]...)

	// Compute hash using eth-go's Keccak256
	hash := hashBytes(data)
	return hash
}

func (s *TracerTester) Validate(validateFunc func(block *pbeth.Block)) {
	block := ParseFirehoseBlock(s.t, "shared tracer", s.tracer.GetTestingOutputBuffer())

	validateFunc(block)
}

// ValidateWithCustomBlock validates using a custom BlockEvent instead of TestBlock
// This is useful for testing specific block header fields that aren't in TestBlock
// NOTE: This method bypasses native validator comparison since custom blocks may not
// be compatible with native tracer validation
//
// Usage: Don't call StartBlock() before this - it handles the block lifecycle itself
func (s *TracerTester) ValidateWithCustomBlock(blockEvent firehose.BlockEvent, validateFunc func(block *pbeth.Block)) {
	// Start block with custom event
	s.tracer.OnBlockStart(blockEvent)
	s.blockLogIndex = 0 // Reset log counter for new block

	// End the block
	s.tracer.OnBlockEnd(nil)

	// Parse and validate
	block := ParseFirehoseBlock(s.t, "shared tracer", s.tracer.GetTestingOutputBuffer())
	validateFunc(block)
}

// ============================================================================
// Parallel Execution Helpers
// ============================================================================

// Spawn creates an isolated tracer for parallel transaction execution.
//
// This method MUST be called on a coordinator TracerTester (after StartBlock).
// It returns a new TracerTester wrapping an isolated tracer that can execute
// transactions in parallel.
//
// Example:
//
//	coordinator := NewTracerTester(t)
//	coordinator.StartBlock()
//
//	isolated0 := coordinator.Spawn(0)
//	isolated1 := coordinator.Spawn(1)
//
//	// Execute in parallel goroutines
//	go func() {
//	    isolated0.StartTrx(tx0).
//	        StartCall(...).EndCall(...).
//	        EndTrx(receipt0, nil)
//	}()
//
//	// Commit in order
//	coordinator.Commit(isolated0)
//	coordinator.Commit(isolated1)
func (s *TracerTester) Spawn(txIndex int) *TracerTester {
	isolatedTracer := s.tracer.OnTxSpawn(txIndex)

	return &TracerTester{
		t:           s.t,
		tracer:      isolatedTracer,
		mockStateDB: s.mockStateDB, // Share mockStateDB with coordinator
		depth:       0,
		// Note: blockLogIndex is NOT shared - each isolated tracer has its own
		blockLogIndex: 0,
	}
}

// Commit commits an isolated tracer's transaction to the coordinator.
//
// This method MUST be called on the coordinator TracerTester with an isolated
// TracerTester as the parameter.
//
// Example:
//
//	isolated := coordinator.Spawn(0)
//	// ... execute transaction in isolated ...
//	coordinator.Commit(isolated)
func (s *TracerTester) Commit(isolated *TracerTester) *TracerTester {
	err := s.tracer.OnTxCommit(isolated.tracer)
	require.NoError(s.t, err, "OnTxCommit should succeed")
	return s
}

// CommitWithError commits an isolated tracer and returns the error (if any).
//
// This is useful for testing error cases where commit should fail.
//
// Example:
//
//	err := coordinator.CommitWithError(isolated)
//	assert.Error(t, err)
func (s *TracerTester) CommitWithError(isolated *TracerTester) error {
	return s.tracer.OnTxCommit(isolated.tracer)
}

// Reset resets an isolated tracer for retry.
//
// This method MUST be called on an isolated TracerTester.
//
// Example:
//
//	isolated := coordinator.Spawn(0)
//	isolated.StartTrx(tx0)
//	// ... execution fails ...
//	isolated.Reset()
//	// ... can execute again ...
func (s *TracerTester) Reset() *TracerTester {
	s.tracer.OnTxReset()
	s.depth = 0
	s.blockLogIndex = 0
	return s
}

// ParseFirehoseBlock parses a single block from FIRE BLOCK output format
// This is a convenience wrapper around ParseFirehoseBlocks that returns the first block
func ParseFirehoseBlock(t *testing.T, tag string, buffer *bytes.Buffer) *pbeth.Block {
	blocks := ParseFirehoseBlocks(t, tag, buffer)
	require.NotEmpty(t, blocks, "For %s: no FIRE BLOCK found in buffer", tag)
	return blocks[0]
}

// ParseFirehoseBlocks parses all blocks from FIRE BLOCK output format
// Returns a slice of blocks in the order they appear in the output
func ParseFirehoseBlocks(t *testing.T, tag string, buffer *bytes.Buffer) []*pbeth.Block {
	scanner := bufio.NewScanner(buffer)

	var initSeen bool
	var blocks []*pbeth.Block

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
			block := &pbeth.Block{}
			err = proto.Unmarshal(payloadBytes, block)
			require.NoError(t, err, "For %s: protobuf unmarshal", tag)

			// Validate fields match (for integrity)
			blockNum, err := strconv.ParseUint(parts[2], 10, 64)
			require.NoError(t, err, "For %s: parse block number from FIRE BLOCK header", tag)
			require.Equal(t, blockNum, block.Number, "For %s: block number in header should match protobuf", tag)

			blocks = append(blocks, block)
		}
	}

	require.NoError(t, scanner.Err(), "For %s: reading buffer", tag)
	return blocks
}

// mockStateDB is a minimal StateDB stub for testing
// It only implements the methods called by the native Firehose tracer (firehose.go)
// The tracer primarily uses GetNonce, GetCode, and Exist for getExecutedCode checks
// All other methods are no-op stubs since actual state is tracked via tracer hooks
type mockStateDB struct {
	// Configurable state for testing
	nonces map[[20]byte]uint64
	codes  map[[20]byte][]byte
	exists map[[20]byte]bool
}

func newMockStateDB() *mockStateDB {
	return &mockStateDB{
		nonces: make(map[[20]byte]uint64),
		codes:  make(map[[20]byte][]byte),
		exists: make(map[[20]byte]bool),
	}
}

// SetNonce sets the nonce for a specific address (for testing)
func (s *mockStateDB) SetNonce(addr [20]byte, nonce uint64) {
	s.nonces[addr] = nonce
}

// SetCode sets the code for a specific address (for testing)
func (s *mockStateDB) SetCode(addr [20]byte, code []byte) {
	s.codes[addr] = code
	s.exists[addr] = true // Setting code implies account exists
}

// SetExist sets whether an address exists (for testing)
func (s *mockStateDB) SetExist(addr [20]byte, exists bool) {
	s.exists[addr] = exists
}

// Methods used by native firehose.go tracer (takes [20]byte)
func (s *mockStateDB) GetNonce(addr [20]byte) uint64 {
	if nonce, ok := s.nonces[addr]; ok {
		return nonce
	}
	return 0 // Default: nonces start at 0
}

func (s *mockStateDB) GetCode(addr [20]byte) []byte {
	if code, ok := s.codes[addr]; ok {
		return code
	}
	return nil // Default: no code (EOA or non-existent)
}

func (s *mockStateDB) Exist(addr [20]byte) bool {
	if exists, ok := s.exists[addr]; ok {
		return exists
	}
	return true // Default: assume addresses exist unless explicitly set to false
}
