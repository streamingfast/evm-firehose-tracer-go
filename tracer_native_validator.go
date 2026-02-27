package firehose

import (
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/tracers"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
)

type nativeValidator struct {
	stateDB *mockStateDB
	tracer  *tracers.Firehose
	t       *testing.T
}

// newNativeValidator creates a native validator with go-ethereum tracer
// Backward compatibility is disabled to test against correct behavior
func newNativeValidator(nativeJSONConfig string) (*nativeValidator, error) {
	var nativeConfig json.RawMessage
	if nativeJSONConfig == "" {
		// Disable backward compatibility for testing correct code paths
		nativeConfig = json.RawMessage(`{"applyBackwardCompatibility": false, "_private": {"flushToTestBuffer": true}}`)
	} else {
		nativeConfig = json.RawMessage(`{` + nativeJSONConfig + `, "applyBackwardCompatibility": false, "_private": {"flushToTestBuffer": true}}`)
	}

	nativeTracer, err := tracers.NewFirehoseFromRawJSON(nativeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create native tracer: %w", err)
	}

	return &nativeValidator{
		tracer:  nativeTracer,
		stateDB: newMockStateDB(),
	}, nil
}

func (v *nativeValidator) OnBlockchainInit(chainConfig *ChainConfig) {
	if v == nil {
		return
	}

	nativeConfig := convertToNativeChainConfig(chainConfig)
	v.tracer.OnBlockchainInit(nativeConfig)
}

func (v *nativeValidator) OnGenesisBlock(event BlockEvent, alloc GenesisAlloc) {
	if v == nil {
		return
	}

	// Convert BlockData to native types.Block
	nativeBlock := convertBlockDataToNativeBlock(&event.Block)

	// Convert GenesisAlloc to native format
	nativeAlloc := convertToNativeGenesisAlloc(alloc)

	v.tracer.OnGenesisBlock(nativeBlock, nativeAlloc)
}

// convertBlockDataToNativeBlock converts our BlockData to go-ethereum's types.Block
func convertBlockDataToNativeBlock(data *BlockData) *types.Block {
	header := &types.Header{
		ParentHash:  common.Hash(data.ParentHash),
		UncleHash:   common.Hash(data.UncleHash),
		Coinbase:    common.Address(data.Coinbase),
		Root:        common.Hash(data.Root),
		TxHash:      common.Hash(data.TxHash),
		ReceiptHash: common.Hash(data.ReceiptHash),
		Bloom:       types.BytesToBloom(data.Bloom),
		Difficulty:  data.Difficulty,
		Number:      big.NewInt(int64(data.Number)),
		GasLimit:    data.GasLimit,
		GasUsed:     data.GasUsed,
		Time:        data.Time,
		Extra:       data.Extra,
		MixDigest:   common.Hash(data.MixDigest),
		Nonce:       types.EncodeNonce(data.Nonce),
		BaseFee:     data.BaseFee,
		// Note: Block hash is computed from header, not set directly
		// The hash in BlockData is informational only
	}

	// For PoS blocks, set difficulty to zero if nil
	if header.Difficulty == nil {
		header.Difficulty = big.NewInt(0)
	}

	// Create block with just the header (genesis block has no transactions)
	block := types.NewBlockWithHeader(header)

	// Return the block - its hash will be computed from the header
	return block
}

// convertToNativeGenesisAlloc converts our GenesisAlloc to go-ethereum's types.GenesisAlloc
func convertToNativeGenesisAlloc(alloc GenesisAlloc) types.GenesisAlloc {
	nativeAlloc := make(types.GenesisAlloc, len(alloc))

	for addr, account := range alloc {
		nativeAddr := common.Address(addr)

		// Convert storage map from [32]byte keys to common.Hash keys
		var nativeStorage map[common.Hash]common.Hash
		if len(account.Storage) > 0 {
			nativeStorage = make(map[common.Hash]common.Hash, len(account.Storage))
			for key, value := range account.Storage {
				nativeStorage[common.Hash(key)] = common.Hash(value)
			}
		}

		nativeAlloc[nativeAddr] = types.Account{
			Code:    account.Code,
			Storage: nativeStorage,
			Balance: account.Balance,
			Nonce:   account.Nonce,
		}
	}

	return nativeAlloc
}

// convertToNativeChainConfig converts our ChainConfig to go-ethereum's params.ChainConfig
func convertToNativeChainConfig(cfg *ChainConfig) *params.ChainConfig {
	if cfg == nil {
		return nil
	}

	// For merge blocks (PoS) to work properly, London and all prerequisite forks
	// must be activated. We set them to block 0 (genesis activation) since our
	// simplified ChainConfig doesn't track historical block-based forks.
	return &params.ChainConfig{
		ChainID:      cfg.ChainID,
		LondonBlock:  big.NewInt(0), // Required for merge validation
		ShanghaiTime: cfg.ShanghaiTime,
		CancunTime:   cfg.CancunTime,
		PragueTime:   cfg.PragueTime,
		VerkleTime:   cfg.VerkleTime,
	}
}

func (v *nativeValidator) OnBlockStart(blockEvent BlockEvent) {
	if v == nil {
		return
	}

	nativeEvent := convertToNativeBlockEvent(blockEvent)
	v.tracer.OnBlockStart(nativeEvent)
}

// convertToNativeBlockEvent converts our BlockEvent to go-ethereum's tracing.BlockEvent
func convertToNativeBlockEvent(event BlockEvent) tracing.BlockEvent {
	block := convertToNativeBlock(event.Block)

	var finalized *types.Header
	if event.Finalized != nil {
		finalized = &types.Header{
			Number: new(big.Int).SetUint64(event.Finalized.Number),
		}
		copy(finalized.ParentHash[:], event.Finalized.Hash[:])
	}

	return tracing.BlockEvent{
		Block:     block,
		Finalized: finalized,
		Safe:      nil, // We don't track Safe separately
	}
}

// convertToNativeBlock converts our BlockData to go-ethereum's types.Block
func convertToNativeBlock(data BlockData) *types.Block {
	header := &types.Header{
		ParentHash:  common.Hash(data.ParentHash),
		UncleHash:   common.Hash(data.UncleHash),
		Coinbase:    common.Address(data.Coinbase),
		Root:        common.Hash(data.Root),
		TxHash:      common.Hash(data.TxHash),
		ReceiptHash: common.Hash(data.ReceiptHash),
		Bloom:       types.BytesToBloom(data.Bloom),
		Difficulty:  data.Difficulty,
		Number:      new(big.Int).SetUint64(data.Number),
		GasLimit:    data.GasLimit,
		GasUsed:     data.GasUsed,
		Time:        data.Time,
		Extra:       data.Extra,
		MixDigest:   common.Hash(data.MixDigest),
		Nonce:       types.EncodeNonce(data.Nonce),
		BaseFee:     data.BaseFee,
	}

	// Convert withdrawals if any
	var withdrawals []*types.Withdrawal
	if len(data.Withdrawals) > 0 {
		withdrawals = make([]*types.Withdrawal, len(data.Withdrawals))
		for i, w := range data.Withdrawals {
			withdrawals[i] = &types.Withdrawal{
				Index:     w.Index,
				Validator: w.ValidatorIndex,
				Address:   common.Address(w.Address),
				Amount:    w.Amount,
			}
		}
	}

	// Convert uncles if any
	var uncles []*types.Header
	if len(data.Uncles) > 0 {
		uncles = make([]*types.Header, len(data.Uncles))
		for i, u := range data.Uncles {
			uncles[i] = &types.Header{
				ParentHash:  common.Hash(u.ParentHash),
				UncleHash:   common.Hash(u.UncleHash),
				Coinbase:    common.Address(u.Coinbase),
				Root:        common.Hash(u.Root),
				TxHash:      common.Hash(u.TxHash),
				ReceiptHash: common.Hash(u.ReceiptHash),
				Bloom:       types.BytesToBloom(u.Bloom),
				Difficulty:  u.Difficulty,
				Number:      new(big.Int).SetUint64(u.Number),
				GasLimit:    u.GasLimit,
				GasUsed:     u.GasUsed,
				Time:        u.Time,
				Extra:       u.Extra,
				MixDigest:   common.Hash(u.MixDigest),
				Nonce:       types.EncodeNonce(u.Nonce),
			}
		}
	}

	// Create block with empty transactions (we don't need them for block-level tracing)
	return types.NewBlockWithHeader(header).WithBody(types.Body{
		Transactions: nil, // Transactions are traced separately
		Uncles:       uncles,
		Withdrawals:  withdrawals,
	})
}

func (v *nativeValidator) OnBlockEnd(err error) {
	if v == nil {
		return
	}

	v.tracer.OnBlockEnd(err)
}

func (v *nativeValidator) OnTxStart(tx interface{}, from [20]byte) [32]byte {
	if v == nil {
		return [32]byte{}
	}

	txEvent, ok := tx.(TxEvent)
	if !ok {
		return [32]byte{}
	}

	nativeTx := convertToNativeTransaction(txEvent)
	nativeFrom := common.Address(from)

	// Pass minimal mock StateDB - the native tracer only needs it for
	// getExecutedCode checks (GetNonce, GetCode, Exist methods)
	vmContext := &tracing.VMContext{
		StateDB: v.stateDB,
	}

	v.tracer.OnTxStart(vmContext, nativeTx, nativeFrom)

	// Return the transaction hash computed by go-ethereum
	return [32]byte(nativeTx.Hash())
}

func (v *nativeValidator) OnTxEnd(receipt interface{}, err error) {
	if v == nil {
		return
	}

	receiptData, ok := receipt.(*ReceiptData)
	if !ok {
		return
	}

	nativeReceipt := convertToNativeReceipt(receiptData)
	v.tracer.OnTxEnd(nativeReceipt, err)
}

func (v *nativeValidator) OnCallEnter(depth int, typ byte, from, to [20]byte, input []byte, gas uint64, value *big.Int) {
	if v == nil {
		return
	}

	nativeFrom := common.Address(from)
	nativeTo := common.Address(to)

	v.tracer.OnCallEnter(depth, typ, nativeFrom, nativeTo, input, gas, value)
}

func (v *nativeValidator) OnCallExit(depth int, output []byte, gasUsed uint64, err error, reverted bool) {
	if v == nil {
		return
	}

	v.tracer.OnCallExit(depth, output, gasUsed, err, reverted)
}

func (v *nativeValidator) OnBalanceChange(addr [20]byte, prev, new *big.Int, reason interface{}) {
	if v == nil {
		return
	}

	pbReason, ok := reason.(pbeth.BalanceChange_Reason)
	if !ok {
		return
	}

	nativeAddr := common.Address(addr)
	nativeReason := convertToNativeBalanceChangeReason(pbReason)

	v.tracer.OnBalanceChange(nativeAddr, prev, new, nativeReason)
}

func (v *nativeValidator) OnNonceChange(addr [20]byte, prev, new uint64) {
	if v == nil {
		return
	}

	nativeAddr := common.Address(addr)
	v.tracer.OnNonceChange(nativeAddr, prev, new)
}

func (v *nativeValidator) OnCodeChange(addr [20]byte, prevCodeHash, codeHash [32]byte, prevCode, code []byte) {
	if v == nil {
		return
	}

	nativeAddr := common.Address(addr)
	nativePrevHash := common.Hash(prevCodeHash)
	nativeNewHash := common.Hash(codeHash)

	v.tracer.OnCodeChange(nativeAddr, nativePrevHash, prevCode, nativeNewHash, code)
}

func (v *nativeValidator) OnStorageChange(addr [20]byte, slot, prev, new [32]byte) {
	if v == nil {
		return
	}

	nativeAddr := common.Address(addr)
	nativeSlot := common.Hash(slot)
	nativePrev := common.Hash(prev)
	nativeNew := common.Hash(new)

	v.tracer.OnStorageChange(nativeAddr, nativeSlot, nativePrev, nativeNew)
}

func (v *nativeValidator) OnGasChange(old, new uint64, reason interface{}) {
	if v == nil {
		return
	}

	pbReason, ok := reason.(pbeth.GasChange_Reason)
	if !ok {
		return
	}

	nativeReason := convertToNativeGasChangeReason(pbReason)
	v.tracer.OnGasChange(old, new, nativeReason)
}

func (v *nativeValidator) OnLog(addr [20]byte, topics [][32]byte, data []byte, blockIndex uint32) {
	if v == nil {
		return
	}

	nativeLog := convertToNativeLog(addr, topics, data, blockIndex)
	v.tracer.OnLog(nativeLog)
}

func (v *nativeValidator) OnOpcode(pc uint64, op byte, gas, cost uint64, depth int) {
	if v == nil {
		return
	}

	// Call native tracer's OnOpcode with minimal parameters
	// The scope and rData parameters are only used for some opcodes (like KECCAK256),
	// for SELFDESTRUCT we only need the opcode byte
	v.tracer.OnOpcode(pc, op, gas, cost, nil, nil, depth, nil)
}

func (v *nativeValidator) OnKeccakPreimage(hash [32]byte, preimage []byte) {
	if v == nil {
		return
	}

	// Call native tracer's OnKeccakPreimage
	v.tracer.OnKeccakPreimage(toCommonHash(hash), preimage)
}

func (v *nativeValidator) OnSystemCallStart() {
	if v == nil {
		return
	}

	v.tracer.OnSystemCallStart()
}

func (v *nativeValidator) OnSystemCallEnd() {
	if v == nil {
		return
	}

	v.tracer.OnSystemCallEnd()
}

// convertToNativeTransaction converts our TxEvent to go-ethereum's types.Transaction
func convertToNativeTransaction(event TxEvent) *types.Transaction {
	var to *common.Address
	if event.To != nil {
		addr := common.Address(*event.To)
		to = &addr
	}

	// Convert based on transaction type
	switch event.Type {
	case 0: // Legacy
		legacyTx := &types.LegacyTx{
			Nonce:    event.Nonce,
			GasPrice: event.GasPrice,
			Gas:      event.Gas,
			To:       to,
			Value:    event.Value,
			Data:     event.Input,
			V:        new(big.Int),
			R:        new(big.Int),
			S:        new(big.Int),
		}
		return types.NewTx(legacyTx)

	case 1: // EIP-2930 Access List
		accessListTx := &types.AccessListTx{
			ChainID:    big.NewInt(1),
			Nonce:      event.Nonce,
			GasPrice:   event.GasPrice,
			Gas:        event.Gas,
			To:         to,
			Value:      event.Value,
			Data:       event.Input,
			AccessList: convertToNativeAccessList(event.AccessList),
			V:          new(big.Int),
			R:          new(big.Int),
			S:          new(big.Int),
		}
		return types.NewTx(accessListTx)

	case 2: // EIP-1559 Dynamic Fee
		dynamicFeeTx := &types.DynamicFeeTx{
			ChainID:    big.NewInt(1),
			Nonce:      event.Nonce,
			GasTipCap:  event.MaxPriorityFeePerGas,
			GasFeeCap:  event.MaxFeePerGas,
			Gas:        event.Gas,
			To:         to,
			Value:      event.Value,
			Data:       event.Input,
			AccessList: convertToNativeAccessList(event.AccessList),
			V:          new(big.Int),
			R:          new(big.Int),
			S:          new(big.Int),
		}
		return types.NewTx(dynamicFeeTx)

	case 3: // EIP-4844 Blob
		blobTx := &types.BlobTx{
			ChainID:    bigToUint256(big.NewInt(1)),
			Nonce:      event.Nonce,
			GasTipCap:  bigToUint256(event.MaxPriorityFeePerGas),
			GasFeeCap:  bigToUint256(event.MaxFeePerGas),
			Gas:        event.Gas,
			To:         common.Address(*event.To),
			Value:      bigToUint256(event.Value),
			Data:       event.Input,
			AccessList: convertToNativeAccessList(event.AccessList),
			BlobFeeCap: bigToUint256(event.BlobGasFeeCap),
			BlobHashes: convertToBlobHashes(event.BlobHashes),
			V:          uint256.NewInt(0),
			R:          uint256.NewInt(0),
			S:          uint256.NewInt(0),
		}
		return types.NewTx(blobTx)

	case 4: // EIP-7702 Set Code
		setCodeTx := &types.SetCodeTx{
			ChainID:    bigToUint256(big.NewInt(1)),
			Nonce:      event.Nonce,
			GasTipCap:  bigToUint256(event.MaxPriorityFeePerGas),
			GasFeeCap:  bigToUint256(event.MaxFeePerGas),
			Gas:        event.Gas,
			To:         common.Address(*event.To),
			Value:      bigToUint256(event.Value),
			Data:       event.Input,
			AccessList: convertToNativeAccessList(event.AccessList),
			AuthList:   convertToNativeAuthList(event.SetCodeAuthorizations),
			V:          uint256.NewInt(0),
			R:          uint256.NewInt(0),
			S:          uint256.NewInt(0),
		}
		return types.NewTx(setCodeTx)

	default:
		// Fallback to legacy for unknown types
		legacyTx := &types.LegacyTx{
			Nonce:    event.Nonce,
			GasPrice: event.GasPrice,
			Gas:      event.Gas,
			To:       to,
			Value:    event.Value,
			Data:     event.Input,
			V:        new(big.Int),
			R:        new(big.Int),
			S:        new(big.Int),
		}
		return types.NewTx(legacyTx)
	}
}

// convertToNativeAccessList converts our AccessList to go-ethereum's types.AccessList
func convertToNativeAccessList(accessList AccessList) types.AccessList {
	if len(accessList) == 0 {
		return nil
	}

	nativeAccessList := make(types.AccessList, len(accessList))
	for i, tuple := range accessList {
		nativeTuple := types.AccessTuple{
			Address: common.Address(tuple.Address),
		}
		if len(tuple.StorageKeys) > 0 {
			nativeTuple.StorageKeys = make([]common.Hash, len(tuple.StorageKeys))
			for j, key := range tuple.StorageKeys {
				nativeTuple.StorageKeys[j] = common.Hash(key)
			}
		}
		nativeAccessList[i] = nativeTuple
	}
	return nativeAccessList
}

// convertToNativeAuthList converts our SetCodeAuthorizations to go-ethereum's types.SetCodeAuthorization
func convertToNativeAuthList(authList []SetCodeAuthorization) []types.SetCodeAuthorization {
	if len(authList) == 0 {
		return nil
	}

	nativeAuthList := make([]types.SetCodeAuthorization, len(authList))
	for i, auth := range authList {
		// Convert ChainID from [32]byte to uint256.Int
		chainID := uint256.NewInt(0)
		chainID.SetBytes(auth.ChainID[:])

		// Convert R and S from [32]byte to uint256.Int
		r := uint256.NewInt(0)
		r.SetBytes(auth.R[:])

		s := uint256.NewInt(0)
		s.SetBytes(auth.S[:])

		nativeAuthList[i] = types.SetCodeAuthorization{
			ChainID: *chainID,
			Address: common.Address(auth.Address),
			Nonce:   auth.Nonce,
			V:       uint8(auth.V), // Convert uint32 to uint8
			R:       *r,
			S:       *s,
		}
	}
	return nativeAuthList
}

// bigToUint256 converts a *big.Int to *uint256.Int
func bigToUint256(b *big.Int) *uint256.Int {
	if b == nil {
		return uint256.NewInt(0)
	}
	val, overflow := uint256.FromBig(b)
	if overflow {
		return uint256.NewInt(0)
	}
	return val
}

// convertToBlobHashes converts [][32]byte to []common.Hash
func convertToBlobHashes(hashes [][32]byte) []common.Hash {
	if len(hashes) == 0 {
		return nil
	}
	blobHashes := make([]common.Hash, len(hashes))
	for i, hash := range hashes {
		blobHashes[i] = common.Hash(hash)
	}
	return blobHashes
}

// convertToNativeReceipt converts our ReceiptData to go-ethereum's types.Receipt
func convertToNativeReceipt(data *ReceiptData) *types.Receipt {
	if data == nil {
		return nil
	}

	// Convert logs
	var logs []*types.Log
	if len(data.Logs) > 0 {
		logs = make([]*types.Log, len(data.Logs))
		for i, logData := range data.Logs {
			topics := make([]common.Hash, len(logData.Topics))
			for j, topic := range logData.Topics {
				topics[j] = common.Hash(topic)
			}

			logs[i] = &types.Log{
				Address: common.Address(logData.Address),
				Topics:  topics,
				Data:    logData.Data,
				Index:   uint(i),
			}
		}
	}

	return &types.Receipt{
		Type:              uint8(0), // LegacyTx type
		Status:            data.Status,
		CumulativeGasUsed: data.CumulativeGasUsed,
		Logs:              logs,
		TxHash:            common.Hash{},
		GasUsed:           data.GasUsed,
		TransactionIndex:  uint(data.TransactionIndex),
	}
}

// convertToNativeBalanceChangeReason converts protobuf balance change reason to go-ethereum tracing reason
// This is the reverse mapping of balanceChangeReasonToPb in go-ethereum/eth/tracers/firehose.go
func convertToNativeBalanceChangeReason(pbReason pbeth.BalanceChange_Reason) tracing.BalanceChangeReason {
	switch pbReason {
	case pbeth.BalanceChange_REASON_REWARD_MINE_UNCLE:
		return tracing.BalanceIncreaseRewardMineUncle
	case pbeth.BalanceChange_REASON_REWARD_MINE_BLOCK:
		return tracing.BalanceIncreaseRewardMineBlock
	case pbeth.BalanceChange_REASON_DAO_REFUND_CONTRACT:
		return tracing.BalanceIncreaseDaoContract
	case pbeth.BalanceChange_REASON_DAO_ADJUST_BALANCE:
		return tracing.BalanceDecreaseDaoAccount
	case pbeth.BalanceChange_REASON_TRANSFER:
		return tracing.BalanceChangeTransfer
	case pbeth.BalanceChange_REASON_GENESIS_BALANCE:
		return tracing.BalanceIncreaseGenesisBalance
	case pbeth.BalanceChange_REASON_GAS_BUY:
		return tracing.BalanceDecreaseGasBuy
	case pbeth.BalanceChange_REASON_REWARD_TRANSACTION_FEE:
		return tracing.BalanceIncreaseRewardTransactionFee
	case pbeth.BalanceChange_REASON_GAS_REFUND:
		return tracing.BalanceIncreaseGasReturn
	case pbeth.BalanceChange_REASON_TOUCH_ACCOUNT:
		return tracing.BalanceChangeTouchAccount
	case pbeth.BalanceChange_REASON_SUICIDE_REFUND:
		return tracing.BalanceIncreaseSelfdestruct
	case pbeth.BalanceChange_REASON_SUICIDE_WITHDRAW:
		return tracing.BalanceDecreaseSelfdestruct
	case pbeth.BalanceChange_REASON_BURN:
		return tracing.BalanceDecreaseSelfdestructBurn
	case pbeth.BalanceChange_REASON_WITHDRAWAL:
		return tracing.BalanceIncreaseWithdrawal
	default:
		return tracing.BalanceChangeUnspecified
	}
}

// convertToNativeGasChangeReason converts protobuf gas change reason to go-ethereum tracing reason
// This is the reverse mapping of gasChangeReasonToPb in go-ethereum/eth/tracers/firehose.go
func convertToNativeGasChangeReason(pbReason pbeth.GasChange_Reason) tracing.GasChangeReason {
	switch pbReason {
	case pbeth.GasChange_REASON_TX_INITIAL_BALANCE:
		return tracing.GasChangeTxInitialBalance
	case pbeth.GasChange_REASON_TX_REFUNDS:
		return tracing.GasChangeTxRefunds
	case pbeth.GasChange_REASON_TX_LEFT_OVER_RETURNED:
		return tracing.GasChangeTxLeftOverReturned
	case pbeth.GasChange_REASON_CALL_INITIAL_BALANCE:
		return tracing.GasChangeCallInitialBalance
	case pbeth.GasChange_REASON_CALL_LEFT_OVER_RETURNED:
		return tracing.GasChangeCallLeftOverReturned
	case pbeth.GasChange_REASON_INTRINSIC_GAS:
		return tracing.GasChangeTxIntrinsicGas
	case pbeth.GasChange_REASON_CONTRACT_CREATION:
		return tracing.GasChangeCallContractCreation
	case pbeth.GasChange_REASON_CONTRACT_CREATION2:
		return tracing.GasChangeCallContractCreation2
	case pbeth.GasChange_REASON_CODE_STORAGE:
		return tracing.GasChangeCallCodeStorage
	case pbeth.GasChange_REASON_PRECOMPILED_CONTRACT:
		return tracing.GasChangeCallPrecompiledContract
	case pbeth.GasChange_REASON_STATE_COLD_ACCESS:
		return tracing.GasChangeCallStorageColdAccess
	case pbeth.GasChange_REASON_REFUND_AFTER_EXECUTION:
		return tracing.GasChangeCallLeftOverRefunded
	case pbeth.GasChange_REASON_FAILED_EXECUTION:
		return tracing.GasChangeCallFailedExecution
	default:
		return tracing.GasChangeUnspecified
	}
}

// convertToNativeLog converts our log parameters to go-ethereum's types.Log
func convertToNativeLog(addr [20]byte, topics [][32]byte, data []byte, blockIndex uint32) *types.Log {
	nativeTopics := make([]common.Hash, len(topics))
	for i, topic := range topics {
		nativeTopics[i] = common.Hash(topic)
	}

	return &types.Log{
		Address: common.Address(addr),
		Topics:  nativeTopics,
		Data:    data,
		Index:   uint(blockIndex),
	}
}

// mockStateDB is a minimal StateDB stub for testing
// It only implements the methods called by the native Firehose tracer (firehose.go)
// The tracer primarily uses GetNonce, GetCode, and Exist for getExecutedCode checks
// All other methods are no-op stubs since actual state is tracked via tracer hooks
type mockStateDB struct{
	// Configurable state for testing
	nonces map[common.Address]uint64
	codes  map[common.Address][]byte
	exists map[common.Address]bool
}

func newMockStateDB() *mockStateDB {
	return &mockStateDB{
		nonces: make(map[common.Address]uint64),
		codes:  make(map[common.Address][]byte),
		exists: make(map[common.Address]bool),
	}
}

// SetNonce sets the nonce for a specific address (for testing)
func (s *mockStateDB) SetNonce(addr common.Address, nonce uint64) {
	s.nonces[addr] = nonce
}

// SetCode sets the code for a specific address (for testing)
func (s *mockStateDB) SetCode(addr common.Address, code []byte) {
	s.codes[addr] = code
	s.exists[addr] = true // Setting code implies account exists
}

// SetExist sets whether an address exists (for testing)
func (s *mockStateDB) SetExist(addr common.Address, exists bool) {
	s.exists[addr] = exists
}

// Methods used by native firehose.go tracer (takes common.Address)
func (s *mockStateDB) GetNonce(addr common.Address) uint64 {
	if nonce, ok := s.nonces[addr]; ok {
		return nonce
	}
	return 0 // Default: nonces start at 0
}

func (s *mockStateDB) GetCode(addr common.Address) []byte {
	if code, ok := s.codes[addr]; ok {
		return code
	}
	return nil // Default: no code (EOA or non-existent)
}

func (s *mockStateDB) Exist(addr common.Address) bool {
	if exists, ok := s.exists[addr]; ok {
		return exists
	}
	return true // Default: assume addresses exist unless explicitly set to false
}

// mockStateReader wraps mockStateDB to implement StateReader interface
// This allows the same mock state to be used by both the native validator (via StateDB)
// and the shared tracer (via StateReader)
type mockStateReader struct {
	*mockStateDB
}

// GetCode implements StateReader interface (takes [20]byte instead of common.Address)
func (m *mockStateReader) GetCode(addr [20]byte) []byte {
	return m.mockStateDB.GetCode(common.Address(addr))
}

// GetNonce implements StateReader interface (takes [20]byte instead of common.Address)
func (m *mockStateReader) GetNonce(addr [20]byte) uint64 {
	return m.mockStateDB.GetNonce(common.Address(addr))
}

// Exist implements StateReader interface (takes [20]byte instead of common.Address)
func (m *mockStateReader) Exist(addr [20]byte) bool {
	return m.mockStateDB.Exist(common.Address(addr))
}

// No-op stub implementations for StateDB interface methods
// These are required by the interface but not used by the native firehose.go tracer
func (s *mockStateDB) CreateAccount(addr common.Address)                                          {}
func (s *mockStateDB) SubBalance(addr common.Address, amount *big.Int, reason tracing.BalanceChangeReason) {}
func (s *mockStateDB) AddBalance(addr common.Address, amount *big.Int, reason tracing.BalanceChangeReason) {}
func (s *mockStateDB) GetBalance(addr common.Address) *uint256.Int                               { return uint256.NewInt(0) }
func (s *mockStateDB) GetState(addr common.Address, hash common.Hash) common.Hash                { return common.Hash{} }
func (s *mockStateDB) GetTransientState(addr common.Address, hash common.Hash) common.Hash       { return common.Hash{} }
func (s *mockStateDB) SetState(addr common.Address, key, value common.Hash)                      {}
func (s *mockStateDB) GetCommittedState(addr common.Address, hash common.Hash) common.Hash       { return common.Hash{} }
func (s *mockStateDB) GetStorageRoot(addr common.Address) common.Hash                            { return common.Hash{} }
func (s *mockStateDB) GetCodeSize(addr common.Address) int                                       { return 0 }
func (s *mockStateDB) GetCodeHash(addr common.Address) common.Hash                               { return common.Hash{} }
func (s *mockStateDB) AddRefund(uint64)                                                          {}
func (s *mockStateDB) SubRefund(uint64)                                                          {}
func (s *mockStateDB) GetRefund() uint64                                                         { return 0 }
func (s *mockStateDB) HasSelfDestructed(addr common.Address) bool                                { return false }
func (s *mockStateDB) SelfDestruct(addr common.Address)                                          {}
func (s *mockStateDB) Selfdestruct6780(addr common.Address)                                      {}
func (s *mockStateDB) AddLog(*types.Log)                                                         {}
func (s *mockStateDB) AddPreimage(common.Hash, []byte)                                           {}
func (s *mockStateDB) AddAddressToAccessList(addr common.Address)                                {}
func (s *mockStateDB) AddSlotToAccessList(addr common.Address, slot common.Hash)                 {}
func (s *mockStateDB) AddressInAccessList(addr common.Address) bool                              { return false }
func (s *mockStateDB) SlotInAccessList(addr common.Address, slot common.Hash) (bool, bool)       { return false, false }
func (s *mockStateDB) Prepare(rules params.Rules, sender, coinbase common.Address, dest *common.Address, precompiles []common.Address, txAccesses types.AccessList) {}
func (s *mockStateDB) Snapshot() int                                                             { return 0 }
func (s *mockStateDB) RevertToSnapshot(int)                                                      {}
