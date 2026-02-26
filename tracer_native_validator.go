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
)

type nativeValidator struct {
	tracer *tracers.Firehose
	t      *testing.T
}

// newNativeValidator creates a native validator with go-ethereum tracer
func newNativeValidator(nativeJSONConfig string) (*nativeValidator, error) {
	var nativeConfig json.RawMessage
	if nativeJSONConfig == "" {
		nativeConfig = json.RawMessage(`{"_private": {"flushToTestBuffer": true}}`)
	} else {
		nativeConfig = json.RawMessage(`{` + nativeJSONConfig + `, "_private": {"flushToTestBuffer": true}}`)
	}

	nativeTracer, err := tracers.NewFirehoseFromRawJSON(nativeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create native tracer: %w", err)
	}

	return &nativeValidator{
		tracer: nativeTracer,
	}, nil
}

func (v *nativeValidator) OnBlockchainInit(chainConfig *ChainConfig) {
	if v == nil {
		return
	}

	nativeConfig := convertToNativeChainConfig(chainConfig)
	v.tracer.OnBlockchainInit(nativeConfig)
}

// convertToNativeChainConfig converts our ChainConfig to go-ethereum's params.ChainConfig
func convertToNativeChainConfig(cfg *ChainConfig) *params.ChainConfig {
	if cfg == nil {
		return nil
	}

	return &params.ChainConfig{
		ChainID:      cfg.ChainID,
		ShanghaiTime: cfg.ShanghaiTime,
		CancunTime:   cfg.CancunTime,
		PragueTime:   cfg.PragueTime,
		VerkleTime:   cfg.VerkleTime,
		// Other fields left as nil since we don't track them
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

func (v *nativeValidator) OnTxStart(tx interface{}, from [20]byte) {
	if v == nil {
		return
	}

	txEvent, ok := tx.(TxEvent)
	if !ok {
		return
	}

	nativeTx := convertToNativeTransaction(txEvent)
	nativeFrom := common.Address(from)

	// Create minimal VMContext for testing (StateDB not needed for non-contract-creation txs)
	vmContext := &tracing.VMContext{}

	v.tracer.OnTxStart(vmContext, nativeTx, nativeFrom)
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

	// TODO: Convert arrays to common.Address
	// Call v.tracer.OnEnter(...)
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

	// TODO: Convert addr to common.Address and reason to native BalanceChangeReason
	// Call v.tracer.OnBalanceChange(...)
}

func (v *nativeValidator) OnNonceChange(addr [20]byte, prev, new uint64) {
	if v == nil {
		return
	}

	// TODO: Convert addr to common.Address
	// Call v.tracer.OnNonceChange(...)
}

func (v *nativeValidator) OnCodeChange(addr [20]byte, prevCodeHash, codeHash [32]byte, prevCode, code []byte) {
	if v == nil {
		return
	}

	// TODO: Convert addr to common.Address and hashes to common.Hash
	// Call v.tracer.OnCodeChange(...)
}

func (v *nativeValidator) OnStorageChange(addr [20]byte, slot, prev, new [32]byte) {
	if v == nil {
		return
	}

	// TODO: Convert addr to common.Address and arrays to common.Hash
	// Call v.tracer.OnStorageChange(...)
}

func (v *nativeValidator) OnGasChange(old, new uint64, reason interface{}) {
	if v == nil {
		return
	}

	// TODO: Convert reason to native GasChangeReason
	// Call v.tracer.OnGasChange(...)
}

func (v *nativeValidator) OnLog(log interface{}) {
	if v == nil {
		return
	}

	// TODO: Convert log to native types.Log
	// Call v.tracer.OnLog(...)
}

// convertToNativeTransaction converts our TxEvent to go-ethereum's types.Transaction
func convertToNativeTransaction(event TxEvent) *types.Transaction {
	var to *common.Address
	if event.To != nil {
		addr := common.Address(*event.To)
		to = &addr
	}

	// Create a LegacyTx for now (type 0)
	// TODO: Handle other transaction types (AccessList, DynamicFee, Blob)
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
