package tests

import (
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
)

// TestTracer_MultipleStateChanges tests multiple different state change types in same transaction
func TestTracer_MultipleStateChanges(t *testing.T) {
	t.Run("create_with_value_transfer", func(t *testing.T) {
		// CONTRACT CREATE with value transfer combines:
		// - Balance change (value transfer from caller to contract)
		// - Code change (contract deployment)
		// - Potentially gas changes

		deployedCode := []byte{0x60, 0x80, 0x60, 0x40, 0x52} // Simple contract bytecode
		codeHash := hash32(123)
		var prevHash [32]byte

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(1000000), 200000, []byte{}). // 1 ETH value transfer
			// Balance changes during value transfer
			BalanceChange(AliceAddr, bigInt(10000000), bigInt(9000000), pbeth.BalanceChange_REASON_TRANSFER).
			BalanceChange(BobAddr, bigInt(0), bigInt(1000000), pbeth.BalanceChange_REASON_TRANSFER).
			// Code deployment
			CodeChange(BobAddr, prevHash, codeHash, nil, deployedCode).
			// Gas consumption
			GasChange(200000, 150000, pbeth.GasChange_REASON_CONTRACT_CREATION).
			EndCall([]byte{}, 150000).
			EndBlockTrx(successReceipt(200000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Verify all state changes are recorded
				assert.Equal(t, 2, len(call.BalanceChanges), "Should have 2 balance changes")
				assert.Equal(t, 1, len(call.CodeChanges), "Should have 1 code change")
				assert.Equal(t, 1, len(call.GasChanges), "Should have 1 gas change")

				// Verify ordinals are all increasing
				assert.True(t, call.BalanceChanges[0].Ordinal < call.BalanceChanges[1].Ordinal)
				assert.True(t, call.BalanceChanges[1].Ordinal < call.CodeChanges[0].Ordinal)
				assert.True(t, call.CodeChanges[0].Ordinal < call.GasChanges[0].Ordinal)
			})
	})

	t.Run("contract_initialization_with_storage_and_logs", func(t *testing.T) {
		// Contract constructor execution combines:
		// - Code change (deployment)
		// - Storage initialization
		// - Log emission (initialization events)

		deployedCode := []byte{0x60, 0x01}
		codeHash := hash32(456)
		var prevHash [32]byte

		storageKey := hash32(1)
		var zeroVal [32]byte
		storageVal := hash32(100)

		logData := []byte{0x01, 0x02}
		logTopics := [][32]byte{hash32(200)}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 200000, []byte{}).
			CodeChange(BobAddr, prevHash, codeHash, nil, deployedCode).
			StorageChange(BobAddr, storageKey, zeroVal, storageVal).
			Log(BobAddr, logTopics, logData, 0).
			EndCall([]byte{}, 180000).
			EndBlockTrx(receiptWithLogs(200000, []firehose.LogData{
				{Address: BobAddr, Topics: logTopics, Data: logData},
			}), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Verify all state changes are recorded
				assert.Equal(t, 1, len(call.CodeChanges), "Should have code change")
				assert.Equal(t, 1, len(call.StorageChanges), "Should have storage change")
				assert.Equal(t, 1, len(call.Logs), "Should have log")

				// Verify ordinals are increasing
				assert.True(t, call.CodeChanges[0].Ordinal < call.StorageChanges[0].Ordinal)
				assert.True(t, call.StorageChanges[0].Ordinal < call.Logs[0].Ordinal)

				// Verify receipt logs match
				assert.Equal(t, 1, len(trx.Receipt.Logs))
				assert.Equal(t, call.Logs[0].Ordinal, trx.Receipt.Logs[0].Ordinal)
			})
	})

	t.Run("comprehensive_transaction_all_state_types", func(t *testing.T) {
		// Comprehensive test with ALL state change types in one transaction:
		// - Balance changes
		// - Nonce change
		// - Code change
		// - Storage change
		// - Gas change
		// - Log emission

		deployedCode := []byte{0x60, 0x02}
		codeHash := hash32(789)
		var prevHash [32]byte

		storageKey := hash32(2)
		var zeroVal [32]byte
		storageVal := hash32(200)

		logData := []byte{0x03, 0x04}
		logTopics := [][32]byte{hash32(300)}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(500000), 300000, []byte{}).
			// All state change types
			BalanceChange(AliceAddr, bigInt(10000000), bigInt(9500000), pbeth.BalanceChange_REASON_TRANSFER).
			NonceChange(AliceAddr, 5, 6).
			CodeChange(BobAddr, prevHash, codeHash, nil, deployedCode).
			StorageChange(BobAddr, storageKey, zeroVal, storageVal).
			GasChange(300000, 250000, pbeth.GasChange_REASON_CONTRACT_CREATION).
			Log(BobAddr, logTopics, logData, 0).
			EndCall([]byte{}, 250000).
			EndBlockTrx(receiptWithLogs(300000, []firehose.LogData{
				{Address: BobAddr, Topics: logTopics, Data: logData},
			}), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Verify ALL state change types are present
				assert.Equal(t, 1, len(call.BalanceChanges), "Should have balance change")
				assert.Equal(t, 1, len(call.NonceChanges), "Should have nonce change")
				assert.Equal(t, 1, len(call.CodeChanges), "Should have code change")
				assert.Equal(t, 1, len(call.StorageChanges), "Should have storage change")
				assert.Equal(t, 1, len(call.GasChanges), "Should have gas change")
				assert.Equal(t, 1, len(call.Logs), "Should have log")

				// Verify ordinals are strictly increasing across ALL types
				var ordinals []uint64
				ordinals = append(ordinals, call.BalanceChanges[0].Ordinal)
				ordinals = append(ordinals, call.NonceChanges[0].Ordinal)
				ordinals = append(ordinals, call.CodeChanges[0].Ordinal)
				ordinals = append(ordinals, call.StorageChanges[0].Ordinal)
				ordinals = append(ordinals, call.GasChanges[0].Ordinal)
				ordinals = append(ordinals, call.Logs[0].Ordinal)

				// Check strict ordering
				for i := 1; i < len(ordinals); i++ {
					assert.True(t, ordinals[i-1] < ordinals[i],
						"Ordinals should be strictly increasing across all state change types")
				}
			})
	})

	t.Run("nested_calls_with_different_state_changes", func(t *testing.T) {
		// Multiple nested calls, each with different state change types

		// Root call: balance change
		// Nested call: storage + log

		storageKey := hash32(3)
		var zeroVal [32]byte
		storageVal := hash32(300)

		logData := []byte{0x05}
		logTopics := [][32]byte{hash32(400)}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 300000, []byte{}).
			// Root call: balance change
			BalanceChange(AliceAddr, bigInt(10000000), bigInt(9900000), pbeth.BalanceChange_REASON_TRANSFER).
			// Nested call
			StartCall(BobAddr, CharlieAddr, bigInt(0), 100000, []byte{}).
			StorageChange(CharlieAddr, storageKey, zeroVal, storageVal).
			Log(CharlieAddr, logTopics, logData, 0).
			EndCall([]byte{}, 90000).
			EndCall([]byte{}, 280000).
			EndBlockTrx(receiptWithLogs(300000, []firehose.LogData{
				{Address: CharlieAddr, Topics: logTopics, Data: logData},
			}), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]

				// Root call has balance change
				assert.Equal(t, 1, len(trx.Calls[0].BalanceChanges))
				assert.Equal(t, 0, len(trx.Calls[0].StorageChanges))
				assert.Equal(t, 0, len(trx.Calls[0].Logs))

				// Nested call has storage + log
				assert.Equal(t, 0, len(trx.Calls[1].BalanceChanges))
				assert.Equal(t, 1, len(trx.Calls[1].StorageChanges))
				assert.Equal(t, 1, len(trx.Calls[1].Logs))

				// Verify ordinals across calls are increasing
				assert.True(t, trx.Calls[0].BalanceChanges[0].Ordinal < trx.Calls[1].StorageChanges[0].Ordinal)
				assert.True(t, trx.Calls[1].StorageChanges[0].Ordinal < trx.Calls[1].Logs[0].Ordinal)
			})
	})
}
