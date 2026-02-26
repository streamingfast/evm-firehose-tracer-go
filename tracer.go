package firehose

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"math/big"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"golang.org/x/crypto/sha3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const ProtocolVersion = "3.0"

// Tracer is the main Firehose tracer that captures EVM execution and produces
// protobuf blocks for indexing. It can operate in two modes:
// - Coordinator mode (default): Manages block-level state and transaction traces
// - Isolated mode: Used for parallel per-transaction tracing
type Tracer struct {
	// Global state
	outputWriter              io.Writer
	initSent                  *atomic.Bool
	config                    *Config
	chainConfig               *ChainConfig
	hasher                    hash.Hash // Keccak256 hasher instance (non-concurrent safe)
	hasherBuf                 [32]byte  // Keccak256 hasher result buffer (non-concurrent safe)
	tracerID                  string
	concurrentFlushQueue      *ConcurrentFlushQueue
	concurrentFlushBufferSize int

	// Block state
	block                       *pbeth.Block
	blockBaseFee                *big.Int
	blockOrdinal                *Ordinal
	blockFinality               *FinalityStatus
	blockRules                  Rules // Fork rules for current block (computed once per block)
	blockIsPrecompiledAddr      func(addr [20]byte) bool
	blockReorderOrdinal         bool
	blockReorderOrdinalSnapshot uint64
	blockReorderOrdinalOnce     sync.Once
	blockIsGenesis              bool

	// Transaction state
	transaction          *pbeth.TransactionTrace
	transactionLogIndex  uint32
	inSystemCall         bool
	transactionIsolated  bool                    // true = isolated mode, false = coordinator mode
	transactionTransient *pbeth.TransactionTrace // Only used in isolated mode

	// Call state
	callStack               *CallStack
	deferredCallState       *DeferredCallState
	latestCallEnterSuicided bool

	// Chain-specific state (used via optional hooks)
	// These fields are included in the shared Tracer to support chains that need them:
	// - BNB: inSystemTx for system transaction tracking
	// - Optimism/Katana: flashBlockIndex for flash block execution
	// - Polygon: State sync receipt handling
	// They remain zero/nil if the hooks are never called.
	flashBlockIndex int // Flash block index (Optimism/Katana)

	// System calls tracking (used in some chains via OnSystemCallStart/End hooks)
	systemCalls []*pbeth.Call

	// Testing state (only used in tests)
	testingBuffer             *bytes.Buffer
	testingIgnoreGenesisBlock bool

	// Validation state (temporary - only used during validation phase)
	// This field is nil unless validation mode is enabled via test framework.
	// When non-nil, all tracer entrypoints also call the native tracer for comparison.
	// This will be removed once validation is complete.
	nativeValidator *nativeValidator
}

// NewTracer creates a new Firehose tracer with the given configuration.
func NewTracer(config *Config) *Tracer {
	if config == nil {
		config = &Config{}
	}

	if config.OutputWriter == nil {
		config.OutputWriter = os.Stdout
	}

	tracer := &Tracer{
		// Global state
		outputWriter:              config.OutputWriter,
		initSent:                  new(atomic.Bool),
		config:                    config,
		chainConfig:               config.ChainConfig,
		hasher:                    sha3.NewLegacyKeccak256(),
		tracerID:                  "global",
		concurrentFlushBufferSize: 100,

		// Block state
		blockOrdinal:  &Ordinal{},
		blockFinality: &FinalityStatus{},

		// Transaction state
		transactionLogIndex: 0,

		// Call state
		callStack:               NewCallStack(),
		deferredCallState:       NewDeferredCallState(),
		latestCallEnterSuicided: false,

		// Validation state (set explicitly in tests via newNativeValidator)
		nativeValidator: nil,
	}

	// Set up concurrent flushing if enabled
	if config.EnableConcurrentFlushing && config.ConcurrentBufferSize > 0 {
		tracer.concurrentFlushQueue = NewConcurrentFlushQueue(
			config.ConcurrentBufferSize,
			func(block *pbeth.Block) {
				bytes, err := tracer.printBlockToFirehose(block)
				if err == nil {
					tracer.flushToFirehose(bytes)
				}
			},
			func() {}, // No additional flush needed
		)
		tracer.concurrentFlushQueue.Start()
	}

	return tracer
}

// newIsolatedTransactionTracer creates an isolated tracer for parallel per-transaction execution.
// The isolated tracer shares block-level state but has its own transaction-specific state.
// Results are stored in transactionTransient and merged back to the coordinator later.
func (t *Tracer) newIsolatedTransactionTracer(tracerID string) *Tracer {
	return &Tracer{
		// Global state (shared from coordinator)
		initSent:    t.initSent,
		config:      t.config,
		chainConfig: t.chainConfig,
		hasher:      sha3.NewLegacyKeccak256(),
		tracerID:    tracerID,

		// Block state (shared from coordinator)
		block:                  t.block,
		blockBaseFee:           t.blockBaseFee,
		blockOrdinal:           &Ordinal{},
		blockFinality:          t.blockFinality,
		blockIsPrecompiledAddr: t.blockIsPrecompiledAddr,
		blockRules:             t.blockRules,

		// Transaction state (fresh for this isolated tracer)
		transactionLogIndex: 0,
		transactionIsolated: true,

		// Call state (fresh for this isolated tracer)
		callStack:               NewCallStack(),
		deferredCallState:       NewDeferredCallState(),
		latestCallEnterSuicided: false,
	}
}

// resetBlock resets the block state only (not transaction or call state)
func (t *Tracer) resetBlock() {
	t.block = nil
	t.blockBaseFee = nil
	t.blockOrdinal.Reset()
	t.blockFinality.Reset()
	t.blockIsPrecompiledAddr = nil
	t.blockRules = Rules{}
	t.blockReorderOrdinal = false
	t.blockReorderOrdinalSnapshot = 0
	t.blockReorderOrdinalOnce = sync.Once{}
	t.blockIsGenesis = false
}

// resetTransaction resets the transaction state and call state in one shot
func (t *Tracer) resetTransaction() {
	t.transaction = nil
	t.transactionLogIndex = 0
	t.inSystemCall = false
	t.transactionTransient = nil

	t.callStack.Reset()
	t.latestCallEnterSuicided = false
	t.deferredCallState.Reset()
}

// ============================================================================
// Output Functions
// ============================================================================

// printToFirehose writes a message to the Firehose output stream
func (t *Tracer) printToFirehose(args ...interface{}) {
	line := fmt.Sprintln(args...)
	if t.testingBuffer != nil {
		t.testingBuffer.WriteString(line)
	} else {
		t.outputWriter.Write([]byte(line))
	}
}

// flushToFirehose writes bytes directly to the output stream
func (t *Tracer) flushToFirehose(bytes []byte) error {
	if t.testingBuffer != nil {
		t.testingBuffer.Write(bytes)
		return nil
	}

	_, err := t.outputWriter.Write(bytes)
	return err
}

// printBlockToFirehose serializes and writes a block to the output stream
func (t *Tracer) printBlockToFirehose(block *pbeth.Block) ([]byte, error) {
	marshalled, err := proto.Marshal(block)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal block: %w", err)
	}

	// Encode as base64 for Firehose protocol
	encoded := base64.StdEncoding.EncodeToString(marshalled)

	// Format: "FIRE BLOCK <block_num> <block_hash> <parent_num> <parent_hash> <lib_num> <timestamp> <payload>"
	blockHash := hex.EncodeToString(block.Hash)
	parentHash := hex.EncodeToString(block.Header.ParentHash)
	line := fmt.Sprintf("FIRE BLOCK %d %s %d %s 0 %d %s\n",
		block.Number,
		blockHash,
		block.Number-1, // parent number
		parentHash,
		block.Header.Timestamp.AsTime().UnixNano(),
		encoded)
	return []byte(line), nil
}

// Helper to create a pointer to a value
func ptr[T any](v T) *T {
	return &v
}

// computeEffectiveGasPrice computes the effective gas price for a transaction
// following the same logic as go-ethereum's gasPrice function:
// - For legacy/access list transactions: use GasPrice
// - For EIP-1559 transactions (dynamic fee, blob, set code):
//   - If baseFee is nil: use MaxFeePerGas (GasFeeCap)
//   - If baseFee is set: use min(MaxPriorityFeePerGas + baseFee, MaxFeePerGas)
func computeEffectiveGasPrice(event TxEvent, baseFee *big.Int) *big.Int {
	switch event.Type {
	case 0, 1: // Legacy, AccessList
		return event.GasPrice

	case 2, 3, 4: // DynamicFee, Blob, SetCode
		// For EIP-1559 transactions, if baseFee is nil, use MaxFeePerGas
		if baseFee == nil {
			if event.MaxFeePerGas != nil {
				return event.MaxFeePerGas
			}
			// Fallback to GasPrice if MaxFeePerGas is not set
			return event.GasPrice
		}

		// Compute: min(MaxPriorityFeePerGas + baseFee, MaxFeePerGas)
		if event.MaxPriorityFeePerGas != nil && event.MaxFeePerGas != nil {
			effectivePrice := new(big.Int).Add(event.MaxPriorityFeePerGas, baseFee)
			if effectivePrice.Cmp(event.MaxFeePerGas) > 0 {
				return event.MaxFeePerGas
			}
			return effectivePrice
		}

		// Fallback to GasPrice if EIP-1559 fields are not set
		return event.GasPrice

	default:
		// Unknown type, use GasPrice
		return event.GasPrice
	}
}

// bigIntToProtobuf converts a big.Int to protobuf BigInt
// Matches the semantics of firehoseBigIntFromNative in go-ethereum:
// - Returns nil for both nil and zero values
// - Only non-zero values get encoded
func bigIntToProtobuf(i *big.Int) *pbeth.BigInt {
	if i == nil || i.Sign() == 0 {
		return nil
	}
	return &pbeth.BigInt{Bytes: i.Bytes()}
}

// ============================================================================
// Hook Parameter Types
// ============================================================================
// These types define the minimal data needed for each hook method.
// Chain-specific implementations will convert from go-ethereum types to these.

// ============================================================================
// Lifecycle Hooks
// ============================================================================

// OnBlockchainInit is called once when the blockchain is initialized
func (t *Tracer) OnBlockchainInit(nodeName string, nodeVersion string, chainConfig *ChainConfig) {
	if t.nativeValidator != nil {
		t.nativeValidator.OnBlockchainInit(chainConfig)
	}

	t.chainConfig = chainConfig

	if wasNeverSent := t.initSent.CompareAndSwap(false, true); wasNeverSent {
		t.printToFirehose("FIRE INIT", ProtocolVersion, "firehose-evm-tracer/"+nodeName, nodeVersion)
	} else {
		panic("OnBlockchainInit was called more than once")
	}

	firehoseInfo("tracer initialized (chain_id=%d)", chainConfig.ChainID)
}

// OnGenesisBlock is called for the genesis block
func (t *Tracer) OnGenesisBlock(event BlockEvent) {
	if t.testingIgnoreGenesisBlock {
		return
	}

	firehoseInfo("genesis block (number=%d)", event.Block.Number)

	// Trace genesis as a normal block
	t.blockIsGenesis = true
	t.OnBlockStart(event)
	t.OnBlockEnd(nil)
	t.blockIsGenesis = false
}

// OnBlockStart is called at the beginning of block processing
func (t *Tracer) OnBlockStart(event BlockEvent) {
	if t.nativeValidator != nil {
		t.nativeValidator.OnBlockStart(event)
	}

	block := event.Block

	// Compute block rules for this block (block-scoped fork flags)
	t.blockRules = t.chainConfig.Rules(new(big.Int).SetUint64(block.Number), block.IsMerge, block.Time)

	// Use provided precompile checker, or build one from the list, or use a no-op
	if event.IsPrecompiledAddr != nil {
		t.blockIsPrecompiledAddr = event.IsPrecompiledAddr
	} else if len(event.ActivePrecompiles) > 0 {
		t.blockIsPrecompiledAddr = t.buildPrecompileChecker(event.ActivePrecompiles)
	} else {
		// No precompiles provided - use a no-op checker (always returns false)
		t.blockIsPrecompiledAddr = func(addr [20]byte) bool { return false }
	}

	firehoseInfo("block start (number=%d hash=%x)", block.Number, block.Hash)

	// Create protobuf block
	t.block = &pbeth.Block{
		Hash:   block.Hash[:],
		Number: block.Number,
		Header: t.newBlockHeaderFromBlockData(block),
		Ver:    4, // Protocol version 4 (without backward compatibility)
		Size:   block.Size,
	}

	// Add uncles
	for _, uncle := range block.Uncles {
		t.block.Uncles = append(t.block.Uncles, t.newBlockHeaderFromUncleData(uncle))
	}

	// Set base fee
	if block.BaseFee != nil {
		t.blockBaseFee = new(big.Int).Set(block.BaseFee)
	}

	// Add withdrawals if present
	if len(block.Withdrawals) > 0 {
		t.block.Withdrawals = make([]*pbeth.Withdrawal, len(block.Withdrawals))
		for i, w := range block.Withdrawals {
			t.block.Withdrawals[i] = &pbeth.Withdrawal{
				Index:          w.Index,
				ValidatorIndex: w.ValidatorIndex,
				Address:        w.Address[:],
				Amount:         w.Amount,
			}
		}
	}

	// Populate finality status
	if event.Finalized != nil {
		t.blockFinality.SetLastFinalizedBlock(event.Finalized.Number)
	}
}

// OnBlockEnd is called at the end of block processing
func (t *Tracer) OnBlockEnd(err error) {
	if t.nativeValidator != nil {
		t.nativeValidator.OnBlockEnd(err)
	}

	firehoseInfo("block ending (err=%v)", err)

	if err == nil {
		// Reorder isolated transactions if needed
		if t.blockReorderOrdinal {
			t.reorderIsolatedTransactionsAndOrdinals()
		}

		// Flush block to firehose
		if t.concurrentFlushQueue != nil {
			t.concurrentFlushQueue.Push(t.block)
		} else {
			bytes, err := t.printBlockToFirehose(t.block)
			if err == nil {
				t.flushToFirehose(bytes)
			}
		}
	}

	t.resetBlock()
	t.resetTransaction()

	firehoseInfo("block end")
}

// OnSkippedBlock is called for blocks that are skipped
func (t *Tracer) OnSkippedBlock(event BlockEvent) {
	// Trace the block as normal, the Firehose system will discard it if needed
	t.OnBlockStart(event)
	t.OnBlockEnd(nil)
}

// OnClose is called when the tracer is being shut down
func (t *Tracer) OnClose() {
	if t.concurrentFlushQueue != nil {
		t.concurrentFlushQueue.Close()
	}
}

// ============================================================================
// Helper methods for block data conversion
// ============================================================================

func (t *Tracer) newBlockHeaderFromBlockData(block BlockData) *pbeth.BlockHeader {
	header := &pbeth.BlockHeader{
		ParentHash:       block.ParentHash[:],
		UncleHash:        block.UncleHash[:],
		Coinbase:         block.Coinbase[:],
		StateRoot:        block.Root[:],
		TransactionsRoot: block.TxHash[:],
		ReceiptRoot:      block.ReceiptHash[:],
		LogsBloom:        block.Bloom,
		Difficulty:       bigIntToProtobuf(block.Difficulty),
		TotalDifficulty:  nil, // Set to nil for PoS blocks (will be properly implemented later)
		Number:           block.Number,
		GasLimit:         block.GasLimit,
		GasUsed:          block.GasUsed,
		Timestamp:        timestamppb.New(toTime(block.Time)),
		ExtraData:        block.Extra,
		MixHash:          block.MixDigest[:],
		Nonce:            block.Nonce,
		Hash:             block.Hash[:],
	}

	// BaseFee uses the same conversion as other BigInt fields
	header.BaseFeePerGas = bigIntToProtobuf(block.BaseFee)

	// Special case: Difficulty must always be set, even for PoS (zero difficulty)
	// This matches the native tracer's behavior in firehose.go:2089-2091
	if header.Difficulty == nil {
		header.Difficulty = &pbeth.BigInt{Bytes: []byte{0}}
	}

	return header
}

func (t *Tracer) newBlockHeaderFromUncleData(uncle UncleData) *pbeth.BlockHeader {
	header := &pbeth.BlockHeader{
		ParentHash:       uncle.ParentHash[:],
		UncleHash:        uncle.UncleHash[:],
		Coinbase:         uncle.Coinbase[:],
		StateRoot:        uncle.Root[:],
		TransactionsRoot: uncle.TxHash[:],
		ReceiptRoot:      uncle.ReceiptHash[:],
		LogsBloom:        uncle.Bloom,
		Difficulty:       bigIntToProtobuf(uncle.Difficulty),
		TotalDifficulty:  nil, // Set to nil for consistency
		Number:           uncle.Number,
		GasLimit:         uncle.GasLimit,
		GasUsed:          uncle.GasUsed,
		Timestamp:        timestamppb.New(toTime(uncle.Time)),
		ExtraData:        uncle.Extra,
		MixHash:          uncle.MixDigest[:],
		Nonce:            uncle.Nonce,
		Hash:             uncle.Hash[:],
	}

	// BaseFee uses the same conversion as other BigInt fields
	header.BaseFeePerGas = bigIntToProtobuf(uncle.BaseFee)

	// Special case: Difficulty must always be set, even for PoS (zero difficulty)
	// This matches the native tracer's behavior in firehose.go:2089-2091
	if header.Difficulty == nil {
		header.Difficulty = &pbeth.BigInt{Bytes: []byte{0}}
	}

	return header
}

// buildPrecompileChecker creates a checker function from a list of precompile addresses
// This is used when the caller provides a list of active precompiles for the block
func (t *Tracer) buildPrecompileChecker(activePrecompiles [][20]byte) func(addr [20]byte) bool {
	activeMap := make(map[[20]byte]bool, len(activePrecompiles))
	for _, addr := range activePrecompiles {
		activeMap[addr] = true
	}

	return func(addr [20]byte) bool {
		return activeMap[addr]
	}
}

func toTime(timestamp uint64) time.Time {
	return time.Unix(int64(timestamp), 0)
}

// reorderIsolatedTransactionsAndOrdinals reorders transactions and ordinals after parallel execution
func (t *Tracer) reorderIsolatedTransactionsAndOrdinals() {
	// TODO: Implement ordinal reordering for parallel execution
	// This is part of Phase 2.6 (Parallel Execution Model)
}

// ============================================================================
// Transaction Lifecycle Hooks
// ============================================================================

// OnTxStart is called at the beginning of transaction execution
func (t *Tracer) OnTxStart(event TxEvent) {
	if t.nativeValidator != nil {
		// Get the transaction hash computed by the native go-ethereum tracer
		// This ensures we use the correct hash for all transaction types
		nativeHash := t.nativeValidator.OnTxStart(event, event.From)
		event.Hash = nativeHash
	}

	firehoseInfo("trx start (hash=%x type=%d gas=%d isolated=%t)", event.Hash, event.Type, event.Gas, t.transactionIsolated)

	// Compute effective gas price based on transaction type
	effectiveGasPrice := computeEffectiveGasPrice(event, t.blockBaseFee)

	// Create transaction trace
	trx := &pbeth.TransactionTrace{
		BeginOrdinal: t.blockOrdinal.Next(),
		Hash:         event.Hash[:],
		From:         event.From[:],
		Nonce:        event.Nonce,
		GasLimit:     event.Gas,
		GasPrice:     bigIntToProtobuf(effectiveGasPrice),
		Value:        bigIntToProtobuf(event.Value),
		Input:        event.Input,
		Type:         pbeth.TransactionTrace_Type(event.Type),
	}

	// Set To address (nil for contract creation)
	if event.To != nil {
		trx.To = event.To[:]
	}

	// Set EIP-1559 fields (type 2, 3, 4)
	if event.MaxFeePerGas != nil {
		trx.MaxFeePerGas = bigIntToProtobuf(event.MaxFeePerGas)
	}
	if event.MaxPriorityFeePerGas != nil {
		trx.MaxPriorityFeePerGas = bigIntToProtobuf(event.MaxPriorityFeePerGas)
	}

	// Set access list (type 1, 2)
	if len(event.AccessList) > 0 {
		trx.AccessList = make([]*pbeth.AccessTuple, len(event.AccessList))
		for i, tuple := range event.AccessList {
			pbTuple := &pbeth.AccessTuple{
				Address: tuple.Address[:],
			}
			if len(tuple.StorageKeys) > 0 {
				pbTuple.StorageKeys = make([][]byte, len(tuple.StorageKeys))
				for j, key := range tuple.StorageKeys {
					pbTuple.StorageKeys[j] = key[:]
				}
			}
			trx.AccessList[i] = pbTuple
		}
	}

	// Set EIP-4844 blob fields (type 3)
	if event.BlobGasFeeCap != nil {
		trx.BlobGasFeeCap = bigIntToProtobuf(event.BlobGasFeeCap)
	}
	if len(event.BlobHashes) > 0 {
		trx.BlobHashes = make([][]byte, len(event.BlobHashes))
		for i, hash := range event.BlobHashes {
			trx.BlobHashes[i] = hash[:]
		}

		// Compute BlobGas: each blob consumes 131072 gas (DATA_GAS_PER_BLOB)
		const blobGasPerBlob = 131072 // 1 << 17
		blobGas := uint64(len(event.BlobHashes)) * blobGasPerBlob
		trx.BlobGas = &blobGas
	}

	// Set EIP-7702 set code authorization list (type 4)
	if len(event.SetCodeAuthorizations) > 0 {
		trx.SetCodeAuthorizations = make([]*pbeth.SetCodeAuthorization, len(event.SetCodeAuthorizations))
		for i, auth := range event.SetCodeAuthorizations {
			trx.SetCodeAuthorizations[i] = &pbeth.SetCodeAuthorization{
				ChainId: auth.ChainID[:],
				Address: auth.Address[:],
				Nonce:   auth.Nonce,
				V:       auth.V,
				R:       auth.R[:],
				S:       auth.S[:],
			}
		}
	}

	t.transaction = trx
}

// OnTxEnd is called at the end of transaction execution
func (t *Tracer) OnTxEnd(receipt *ReceiptData, err error) {
	if t.nativeValidator != nil {
		t.nativeValidator.OnTxEnd(receipt, err)
	}

	firehoseInfo("trx ending (isolated=%t, err=%v)", t.transactionIsolated, err)

	trxTrace := t.completeTransaction(receipt, err)

	// In isolated mode, store in transient storage for later merge
	if t.transactionIsolated {
		t.transactionTransient = trxTrace
		// Don't reset transaction in isolated mode - will be reset by coordinator
	} else {
		t.block.TransactionTraces = append(t.block.TransactionTraces, trxTrace)
		t.resetTransaction()
	}

	firehoseInfo("trx end")
}

// completeTransaction finalizes a transaction trace with receipt data
func (t *Tracer) completeTransaction(receipt *ReceiptData, err error) *pbeth.TransactionTrace {
	firehoseInfo("completing transaction (call_count=%d)", len(t.transaction.Calls))

	if len(t.transaction.Calls) == 0 {
		// Bad block or misconfigured - terminate immediately
		t.transaction.EndOrdinal = t.blockOrdinal.Next()
		return t.transaction
	}

	// Get root call
	rootCall := t.transaction.Calls[0]

	// Move any remaining deferred state to root call
	if !t.deferredCallState.IsEmpty() {
		t.deferredCallState.MaybePopulateCallAndReset("root", rootCall)
	}

	// Populate receipt data
	if receipt != nil {
		t.transaction.Index = receipt.TransactionIndex
		t.transaction.GasUsed = receipt.GasUsed
		t.transaction.Receipt = t.newReceiptFromData(receipt)

		if receipt.Status == 1 {
			t.transaction.Status = pbeth.TransactionTraceStatus_SUCCEEDED
		} else {
			t.transaction.Status = pbeth.TransactionTraceStatus_FAILED
		}
	}

	// Check if root call reverted
	if rootCall.StatusReverted {
		t.transaction.Status = pbeth.TransactionTraceStatus_REVERTED
	}

	// Populate state reverted flags
	t.populateStateReverted()

	// Set end ordinal
	t.transaction.EndOrdinal = t.blockOrdinal.Next()

	return t.transaction
}

// ReceiptData contains the minimal receipt data needed
type ReceiptData struct {
	TransactionIndex  uint32
	GasUsed           uint64
	Status            uint64
	Logs              []LogData
	CumulativeGasUsed uint64
	BlobGasUsed       uint64 // EIP-4844: Gas used for blob data
	BlobGasPrice      *big.Int // EIP-4844: Price per unit of blob gas
}

// LogData contains log event data
type LogData struct {
	Address [20]byte
	Topics  [][32]byte
	Data    []byte
}

func (t *Tracer) newReceiptFromData(receipt *ReceiptData) *pbeth.TransactionReceipt {
	r := &pbeth.TransactionReceipt{
		CumulativeGasUsed: receipt.CumulativeGasUsed,
		LogsBloom:         make([]byte, 256), // TODO: Compute logs bloom
	}

	// Add EIP-4844 blob fields for blob transactions (type 3)
	if t.transaction.Type == pbeth.TransactionTrace_TRX_TYPE_BLOB {
		r.BlobGasUsed = &receipt.BlobGasUsed
		if receipt.BlobGasPrice != nil {
			r.BlobGasPrice = bigIntToProtobuf(receipt.BlobGasPrice)
		}
	}

	// Add logs
	for _, log := range receipt.Logs {
		pbLog := &pbeth.Log{
			Address: log.Address[:],
			Data:    log.Data,
		}
		for _, topic := range log.Topics {
			pbLog.Topics = append(pbLog.Topics, topic[:])
		}
		r.Logs = append(r.Logs, pbLog)
	}

	return r
}

// populateStateReverted walks the call tree and marks reverted state
func (t *Tracer) populateStateReverted() {
	for _, call := range t.transaction.Calls {
		if call.StatusReverted {
			t.markStateReverted(call)
		}
	}
}

func (t *Tracer) markStateReverted(call *pbeth.Call) {
	call.StateReverted = true
	// Note: Individual state change objects don't have StateReverted field
	// The StateReverted flag on the call is sufficient to indicate all its changes are reverted
}

// ============================================================================
// Call Lifecycle Hooks
// ============================================================================

// OnCallEnter is called when entering a call
func (t *Tracer) OnCallEnter(depth int, typ byte, from, to [20]byte, input []byte, gas uint64, value *big.Int) {
	if t.nativeValidator != nil {
		t.nativeValidator.OnCallEnter(depth, typ, from, to, input, gas, value)
	}

	firehoseTrace("call enter (depth=%d type=%d from=%s to=%s value=%d gas=%d input=%s)",
		depth, typ, shortAddressView(&from), shortAddressView(&to),
		value, gas, inputView(input))

	call := &pbeth.Call{
		// Index, Depth, and ParentIndex are set by callStack.Push()
		CallType:     t.callTypeToProto(CallType(typ)),
		Caller:       from[:],
		Address:      to[:],
		Value:        &pbeth.BigInt{Bytes: value.Bytes()},
		GasLimit:     gas,
		GasConsumed:  0,
		Input:        input,
		BeginOrdinal: t.blockOrdinal.Next(),
	}

	// Handle DELEGATECALL code address
	// Note: CodeAddress field may not exist in all protobuf versions
	// if typ == byte(CallTypeDelegateCall) && codeAddress != nil {
	// 	call.CodeAddress = codeAddress[:]
	// }

	// Move deferred state to this call if it's the first call
	if depth == 0 {
		t.deferredCallState.MaybePopulateCallAndReset("enter", call)
	}

	t.transaction.Calls = append(t.transaction.Calls, call)
	t.callStack.Push(call)
}

// OnCallExit is called when exiting a call
func (t *Tracer) OnCallExit(output []byte, gasUsed uint64, err error) {
	call := t.callStack.Peek()
	if call == nil {
		firehoseDebug("call exit with no active call - ignoring")
		return
	}

	depth := t.callStack.Depth() - 1 // -1 because we haven't popped yet
	reverted := err != nil
	if t.nativeValidator != nil {
		t.nativeValidator.OnCallExit(depth, output, gasUsed, err, reverted)
	}

	firehoseTrace("call exit (depth=%d gas_used=%d err=%s output=%s)",
		call.Depth, gasUsed, errorView(err), outputView(output))

	call.GasConsumed = gasUsed
	call.ReturnData = output
	call.EndOrdinal = t.blockOrdinal.Next()

	if err != nil {
		call.StatusFailed = true
		call.StatusReverted = true
		call.FailureReason = err.Error()
	}

	t.callStack.Pop()
}

func (t *Tracer) callTypeToProto(ct CallType) pbeth.CallType {
	switch ct {
	case CallTypeCall:
		return pbeth.CallType_CALL
	case CallTypeCallCode:
		return pbeth.CallType_CALLCODE
	case CallTypeDelegateCall:
		return pbeth.CallType_DELEGATE
	case CallTypeStaticCall:
		return pbeth.CallType_STATIC
	case CallTypeCreate:
		return pbeth.CallType_CREATE
	case CallTypeCreate2:
		return pbeth.CallType_CREATE // CREATE2 may not be in protobuf, use CREATE
	case CallTypeSelfDestruct:
		return pbeth.CallType_UNSPECIFIED // SELFDESTRUCT may not be a call type
	default:
		return pbeth.CallType_UNSPECIFIED
	}
}

// ============================================================================
// State Change Hooks
// ============================================================================

// OnBalanceChange is called when an account balance changes
// Note: reason is pbeth.BalanceChange_Reason - the chain implementation converts from go-ethereum types
func (t *Tracer) OnBalanceChange(addr [20]byte, oldBalance, newBalance *big.Int, reason pbeth.BalanceChange_Reason) {
	if t.nativeValidator != nil {
		t.nativeValidator.OnBalanceChange(addr, oldBalance, newBalance, reason)
	}

	// Ignore unspecified reasons
	if reason == pbeth.BalanceChange_REASON_UNKNOWN {
		return
	}

	t.ensureInBlockOrTrx()

	change := t.newBalanceChange("tracer", addr, oldBalance, newBalance, reason)

	// In transaction context - attach to call or defer
	if t.transaction != nil {
		activeCall := t.callStack.Peek()

		// Initial transfer happens before call starts - defer it
		if activeCall == nil {
			t.deferredCallState.AddBalanceChange(change)
			return
		}

		activeCall.BalanceChanges = append(activeCall.BalanceChanges, change)
	} else {
		// Block-level balance change (e.g., block rewards, withdrawals)
		t.block.BalanceChanges = append(t.block.BalanceChanges, change)
	}
}

func (t *Tracer) newBalanceChange(tag string, addr [20]byte, oldValue, newValue *big.Int, reason pbeth.BalanceChange_Reason) *pbeth.BalanceChange {
	firehoseTrace("balance changed (tag=%s address=%s before=%d after=%d reason=%s)",
		tag, shortAddressView(&addr), oldValue, newValue, reason)

	if reason == pbeth.BalanceChange_REASON_UNKNOWN {
		panic(fmt.Errorf("received unknown balance change reason %s", reason))
	}

	return &pbeth.BalanceChange{
		Ordinal:  t.blockOrdinal.Next(),
		Address:  addr[:],
		OldValue: &pbeth.BigInt{Bytes: oldValue.Bytes()},
		NewValue: &pbeth.BigInt{Bytes: newValue.Bytes()},
		Reason:   reason,
	}
}

// OnNonceChange is called when an account nonce changes
func (t *Tracer) OnNonceChange(addr [20]byte, oldNonce, newNonce uint64) {
	if t.nativeValidator != nil {
		t.nativeValidator.OnNonceChange(addr, oldNonce, newNonce)
	}

	t.ensureInBlockAndInTrx()

	activeCall := t.callStack.Peek()
	change := &pbeth.NonceChange{
		Address:  addr[:],
		OldValue: oldNonce,
		NewValue: newNonce,
		Ordinal:  t.blockOrdinal.Next(),
	}

	// Initial nonce change happens before call starts - defer it
	if activeCall == nil {
		t.deferredCallState.AddNonceChange(change)
		return
	}

	activeCall.NonceChanges = append(activeCall.NonceChanges, change)
}

// OnCodeChange is called when contract code changes
// Note: Includes code hashes for proper tracking
func (t *Tracer) OnCodeChange(addr [20]byte, prevCodeHash, newCodeHash [32]byte, oldCode, newCode []byte) {
	if t.nativeValidator != nil {
		t.nativeValidator.OnCodeChange(addr, prevCodeHash, newCodeHash, oldCode, newCode)
	}

	firehoseDebug("code changed (address=%s prev_hash=%x new_hash=%x)",
		shortAddressView(&addr), prevCodeHash, newCodeHash)

	t.ensureInBlockOrTrx()

	// In transaction context - attach to call or defer
	if t.transaction != nil {
		activeCall := t.callStack.Peek()

		// Code change before call starts (e.g., EIP-7702 SetCode) - defer it
		if activeCall == nil {
			t.deferredCallState.AddCodeChange(t.newCodeChange(addr, prevCodeHash, oldCode, newCodeHash, newCode))
			return
		}

		// Ignore code changes from suicide if there was previous code
		// Geth 1.14.12+ emits code change on suicide, but we ignore it for consistency
		// Exception: suicide in constructor (no previous code) is still tracked
		if activeCall.Suicide && len(oldCode) > 0 && len(newCode) == 0 {
			firehoseDebug("ignoring code change due to suicide (prev: %x (%d), new: %x (%d))",
				prevCodeHash, len(oldCode), newCodeHash, len(newCode))
			return
		}

		activeCall.CodeChanges = append(activeCall.CodeChanges, t.newCodeChange(addr, prevCodeHash, oldCode, newCodeHash, newCode))
	} else {
		// Block-level code change
		t.block.CodeChanges = append(t.block.CodeChanges, t.newCodeChange(addr, prevCodeHash, oldCode, newCodeHash, newCode))
	}
}

func (t *Tracer) newCodeChange(addr [20]byte, prevCodeHash [32]byte, oldCode []byte, newCodeHash [32]byte, newCode []byte) *pbeth.CodeChange {
	return &pbeth.CodeChange{
		Address: addr[:],
		OldHash: prevCodeHash[:],
		OldCode: oldCode,
		NewHash: newCodeHash[:],
		NewCode: newCode,
		Ordinal: t.blockOrdinal.Next(),
	}
}

// OnStorageChange is called when contract storage changes
func (t *Tracer) OnStorageChange(addr [20]byte, slot, oldValue, newValue [32]byte) {
	if t.nativeValidator != nil {
		t.nativeValidator.OnStorageChange(addr, slot, oldValue, newValue)
	}

	firehoseTrace("storage changed (address=%s key=%x, before=%x after=%x)",
		shortAddressView(&addr), slot, oldValue, newValue)

	t.ensureInBlockAndInTrxAndInCall()

	activeCall := t.callStack.Peek()
	activeCall.StorageChanges = append(activeCall.StorageChanges, &pbeth.StorageChange{
		Address:  addr[:],
		Key:      slot[:],
		OldValue: oldValue[:],
		NewValue: newValue[:],
		Ordinal:  t.blockOrdinal.Next(),
	})
}

// ============================================================================
// Other Hooks
// ============================================================================

// OnLog is called when a log event is emitted
// Note: blockIndex comes from the log itself (from go-ethereum types.Log.Index)
func (t *Tracer) OnLog(addr [20]byte, topics [][32]byte, data []byte, blockIndex uint32) {
	if t.nativeValidator != nil {
		t.nativeValidator.OnLog(addr, topics, data, blockIndex)
	}

	t.ensureInBlockAndInTrxAndInCall()

	activeCall := t.callStack.Peek()
	firehoseTrace("adding log to call (address=%s call=%d [has already %d logs])",
		shortAddressView(&addr), activeCall.Index, len(activeCall.Logs))

	pbLog := &pbeth.Log{
		Address:    addr[:],
		Data:       data,
		Index:      t.transactionLogIndex,
		BlockIndex: blockIndex,
		Ordinal:    t.blockOrdinal.Next(),
	}

	for _, topic := range topics {
		pbLog.Topics = append(pbLog.Topics, topic[:])
	}

	activeCall.Logs = append(activeCall.Logs, pbLog)
	t.transactionLogIndex++
}

// OnGasChange is called when gas is consumed
// Note: reason is pbeth.GasChange_Reason - the chain implementation converts from go-ethereum types
func (t *Tracer) OnGasChange(oldGas, newGas uint64, reason pbeth.GasChange_Reason) {
	if t.nativeValidator != nil {
		t.nativeValidator.OnGasChange(oldGas, newGas, reason)
	}

	t.ensureInBlockAndInTrx()

	// No change in gas - ignore
	if oldGas == newGas {
		return
	}

	// Ignore UNKNOWN reasons (filtered by caller in chain implementation)
	if reason == pbeth.GasChange_REASON_UNKNOWN {
		return
	}

	activeCall := t.callStack.Peek()
	change := t.newGasChange("tracer", oldGas, newGas, reason)

	// Initial gas consumption happens before call starts - defer it
	if activeCall == nil {
		t.deferredCallState.AddGasChange(change)
		return
	}

	activeCall.GasChanges = append(activeCall.GasChanges, change)
}

func (t *Tracer) newGasChange(tag string, oldValue, newValue uint64, reason pbeth.GasChange_Reason) *pbeth.GasChange {
	firehoseTrace("gas consumed (tag=%s before=%d after=%d reason=%s)", tag, oldValue, newValue, reason)

	// Should already be checked by caller, but safety check
	if reason == pbeth.GasChange_REASON_UNKNOWN {
		panic(fmt.Errorf("received unknown gas change reason %s", reason))
	}

	return &pbeth.GasChange{
		OldValue: oldValue,
		NewValue: newValue,
		Reason:   reason,
		Ordinal:  t.blockOrdinal.Next(),
	}
}

// ============================================================================
// Optional/Chain-Specific Hooks
// ============================================================================

// OnSystemCallStart is called when a system call starts (chain-specific)
func (t *Tracer) OnSystemCallStart() {
	firehoseInfo("system call start")
	t.inSystemCall = true
}

// OnSystemCallEnd is called when a system call ends (chain-specific)
func (t *Tracer) OnSystemCallEnd() {
	firehoseInfo("system call end")
	t.inSystemCall = false

	// Move any calls created during system call to system calls list
	if len(t.transaction.Calls) > 0 {
		t.systemCalls = append(t.systemCalls, t.transaction.Calls...)
		t.transaction.Calls = nil
	}
}

// OnOpcode is called for each opcode (optional, for detailed tracing)
func (t *Tracer) OnOpcode(pc uint64, op byte, gas, cost uint64, scope OpcodeScopeData, rData []byte, depth int, err error) {
	// Only trace opcodes if enabled
	if !isTraceFullEnabled {
		return
	}

	call := t.callStack.Peek()
	if call == nil {
		return
	}

	// This would add detailed opcode-level tracing
	// For now, we skip it as it's very verbose
}

// OpcodeScopeData contains the execution scope for an opcode
type OpcodeScopeData struct {
	Memory   []byte
	Stack    [][]byte
	Contract []byte
	CodeAddr [20]byte
}

// OnKeccakPreimage is called when a keccak256 preimage is available
func (t *Tracer) OnKeccakPreimage(hash [32]byte, preimage []byte) {
	// Store keccak preimages for later lookup
	// This is used to map storage slot hashes back to their keys
	firehoseTrace("keccak preimage (hash=%x preimage_len=%d)", hash, len(preimage))
}

// OnNewAccount is called when a new account is created
// Note: This is a legacy hook that some chains may need
// Modern Firehose doesn't track this (use OnCodeChange instead)
// Set ignoreSystemAddress=true for Ethereum mainnet/testnets, false for BSC/Polygon
func (t *Tracer) OnNewAccount(addr [20]byte, ignoreSystemAddress bool) {
	t.ensureInBlockOrTrx()

	// If not in transaction, we're in block finalization
	// Old Firehose didn't track these, so we just advance ordinal
	if t.transaction == nil {
		t.blockOrdinal.Next()
		return
	}

	// Ignore account creations in static calls to precompiled contracts
	if call := t.callStack.Peek(); call != nil {
		if call.CallType == pbeth.CallType_STATIC {
			// Check if calling a precompiled address
			var callAddr [20]byte
			copy(callAddr[:], call.Address)
			if t.blockIsPrecompiledAddr(callAddr) {
				// Old Firehose ignored these
				return
			}
		}
	}

	// System address (0xfffffffffffffffffffffffffffffffffffffffe)
	// Ethereum mainnet/testnets ignore it, but BSC/Polygon track it
	systemAddr := [20]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfe}
	if addr == systemAddr && ignoreSystemAddress {
		return
	}

	accountCreation := &pbeth.AccountCreation{
		Account: addr[:],
		Ordinal: t.blockOrdinal.Next(),
	}

	activeCall := t.callStack.Peek()
	if activeCall == nil {
		t.deferredCallState.AddAccountCreation(accountCreation)
		return
	}

	activeCall.AccountCreations = append(activeCall.AccountCreations, accountCreation)
}

// OnOpcodeFault is called when an opcode execution fails
func (t *Tracer) OnOpcodeFault(pc uint64, op byte, gas, cost uint64, scope OpcodeScopeData, depth int, err error) {
	firehoseDebug("opcode fault (pc=%d op=%d err=%v)", pc, op, err)

	call := t.callStack.Peek()
	if call != nil {
		call.StatusFailed = true
		call.FailureReason = err.Error()
	}
}

// ============================================================================
// Isolated Transaction Support (for Parallel Execution)
// ============================================================================

// addIsolatedTransaction merges an isolated transaction trace back to the coordinator
func (t *Tracer) addIsolatedTransaction(isolatedTrace *pbeth.TransactionTrace) {
	baseOrdinal := t.blockOrdinal.Peek()
	t.blockOrdinal.Set(t.reorderTraceOrdinals(isolatedTrace, baseOrdinal))
	t.block.TransactionTraces = append(t.block.TransactionTraces, isolatedTrace)
}

// addIsolatedSystemCalls merges isolated system calls back to the coordinator
func (t *Tracer) addIsolatedSystemCalls(isolatedCalls []*pbeth.Call) {
	baseOrdinal := t.blockOrdinal.Peek()
	endOrdinal := baseOrdinal

	for _, call := range isolatedCalls {
		endOrdinal = t.reorderCallOrdinals(call, baseOrdinal)
	}

	t.blockOrdinal.Set(endOrdinal)
	t.block.SystemCalls = append(t.block.SystemCalls, isolatedCalls...)
}

// reorderTraceOrdinals recursively adjusts ordinals in a transaction trace
func (t *Tracer) reorderTraceOrdinals(trace *pbeth.TransactionTrace, baseOrdinal uint64) uint64 {
	trace.BeginOrdinal += baseOrdinal

	for _, call := range trace.Calls {
		baseOrdinal = t.reorderCallOrdinals(call, baseOrdinal)
	}

	trace.EndOrdinal += baseOrdinal
	return trace.EndOrdinal
}

// reorderCallOrdinals recursively adjusts ordinals in a call and its children
func (t *Tracer) reorderCallOrdinals(call *pbeth.Call, ordinalBase uint64) uint64 {
	call.BeginOrdinal += ordinalBase

	// Reorder all state changes
	for _, change := range call.BalanceChanges {
		change.Ordinal += ordinalBase
	}
	for _, change := range call.NonceChanges {
		change.Ordinal += ordinalBase
	}
	for _, change := range call.CodeChanges {
		change.Ordinal += ordinalBase
	}
	for _, change := range call.StorageChanges {
		change.Ordinal += ordinalBase
	}
	for _, change := range call.GasChanges {
		change.Ordinal += ordinalBase
	}
	for _, log := range call.Logs {
		log.Ordinal += ordinalBase
	}

	call.EndOrdinal += ordinalBase
	return call.EndOrdinal
}

// ============================================================================
// State Validation Methods
// ============================================================================

// ensureBlockChainInit checks that OnBlockchainInit was called
func (t *Tracer) ensureBlockChainInit() {
	if t.chainConfig == nil {
		t.panicInvalidState("the OnBlockchainInit hook should have been called at this point", 2)
	}
}

// ensureInBlock checks that we're currently processing a block
func (t *Tracer) ensureInBlock(callerSkip int) {
	if t.block == nil {
		t.panicInvalidState("caller expected to be in block state but we were not, this is a bug", callerSkip+1)
	}

	if t.chainConfig == nil {
		t.panicInvalidState("the OnBlockchainInit hook should have been called at this point", callerSkip+1)
	}
}

// ensureNotInBlock checks that we're not in a block
func (t *Tracer) ensureNotInBlock(callerSkip int) {
	if t.block != nil {
		t.panicInvalidState("caller expected to not be in block state but we were, this is a bug", callerSkip+1)
	}
}

// ensureInBlockAndInTrx checks that we're in a block and transaction
func (t *Tracer) ensureInBlockAndInTrx() {
	t.ensureInBlock(2)

	if t.transaction == nil {
		t.panicInvalidState("caller expected to be in transaction state but we were not, this is a bug", 2)
	}
}

// ensureInBlockAndNotInTrx checks that we're in a block but not in a transaction
func (t *Tracer) ensureInBlockAndNotInTrx() {
	t.ensureInBlock(2)

	if t.transaction != nil {
		t.panicInvalidState("caller expected to not be in transaction state but we were, this is a bug", 2)
	}
}

// ensureInBlockAndNotInTrxAndNotInCall checks state for starting a new transaction
func (t *Tracer) ensureInBlockAndNotInTrxAndNotInCall() {
	t.ensureInBlock(2)

	if t.transaction != nil {
		t.panicInvalidState("caller expected to not be in transaction state but we were, this is a bug", 2)
	}

	if t.callStack.HasActiveCall() {
		t.panicInvalidState("caller expected to not be in call state but we were, this is a bug", 2)
	}
}

// ensureInBlockOrTrx checks that we're in either a block or transaction
func (t *Tracer) ensureInBlockOrTrx() {
	if t.transaction == nil && t.block == nil {
		t.panicInvalidState("caller expected to be in either block or transaction state but we were not, this is a bug", 2)
	}
}

// ensureInBlockAndInTrxAndInCall checks that we're in a block, transaction, and call
func (t *Tracer) ensureInBlockAndInTrxAndInCall() {
	if t.transaction == nil || t.block == nil {
		t.panicInvalidState("caller expected to be in block and in transaction but we were not, this is a bug", 2)
	}

	if !t.callStack.HasActiveCall() {
		t.panicInvalidState("caller expected to be in call state but we were not, this is a bug", 2)
	}
}

// ensureInCall checks that we're in a call
func (t *Tracer) ensureInCall() {
	if !t.callStack.HasActiveCall() {
		t.panicInvalidState("caller expected to be in call state but we were not, this is a bug", 2)
	}
}

// ensureInSystemCall checks that we're in a system call
func (t *Tracer) ensureInSystemCall() {
	if !t.inSystemCall {
		t.panicInvalidState("caller expected to be in system call state but we were not, this is a bug", 2)
	}
}

// panicInvalidState panics with a detailed error message including:
// - The provided message
// - Caller location (file:line)
// - Current block number and hash (if in block)
// - Current transaction hash (if in transaction)
// - Current tracer state flags
func (t *Tracer) panicInvalidState(msg string, callerSkip int) {
	caller := "N/A"
	if _, file, line, ok := runtime.Caller(callerSkip); ok {
		caller = fmt.Sprintf("%s:%d", file, line)
	}

	if t.block != nil {
		msg += fmt.Sprintf(" at block #%d (%s)", t.block.Number, hex.EncodeToString(t.block.Hash))
	}

	if t.transaction != nil {
		msg += fmt.Sprintf(" in transaction %s", hex.EncodeToString(t.transaction.Hash))
	}

	panic(fmt.Errorf("%s (caller=%s, init=%t, inBlock=%t, inTransaction=%t, inCall=%t, isolated=%t)",
		msg, caller,
		t.chainConfig != nil,
		t.block != nil,
		t.transaction != nil,
		t.callStack.HasActiveCall(),
		t.transactionIsolated,
	))
}
