package tests

import (
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go/v5"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
)

// TestTracer_Suicide tests SELFDESTRUCT opcode scenarios
// SELFDESTRUCT is a special EVM opcode that destroys a contract and sends its balance to a beneficiary
func TestTracer_Suicide(t *testing.T) {
	t.Run("normal_suicide_different_beneficiary", func(t *testing.T) {
		// Contract suicides, sending balance to different address
		contractBalance := bigInt(500)

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			// Bob (contract) self-destructs, sending balance to Charlie
			Suicide(BobAddr, CharlieAddr, contractBalance).
			EndCall([]byte{}, 50000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				rootCall := trx.Calls[0]

				// Verify call is marked as suicided
				assert.True(t, rootCall.Suicide, "Call should be marked as suicided")
				assert.True(t, rootCall.ExecutedCode, "Call should have ExecutedCode=true")

				// Verify balance changes (should have 2: SUICIDE_WITHDRAW + SUICIDE_REFUND)
				balanceChanges := rootCall.BalanceChanges
				assert.Equal(t, 2, len(balanceChanges), "Should have 2 balance changes")

				// First: Contract balance withdrawn
				withdraw := balanceChanges[0]
				assert.Equal(t, BobAddr[:], withdraw.Address)
				assert.Equal(t, contractBalance.Bytes(), withdraw.OldValue.Bytes)
				assert.Nil(t, withdraw.NewValue, "Zero balance should be nil in protobuf")
				assert.Equal(t, pbeth.BalanceChange_REASON_SUICIDE_WITHDRAW, withdraw.Reason)

				// Second: Beneficiary receives balance
				refund := balanceChanges[1]
				assert.Equal(t, CharlieAddr[:], refund.Address)
				assert.Nil(t, refund.OldValue, "Zero balance should be nil in protobuf")
				assert.Equal(t, contractBalance.Bytes(), refund.NewValue.Bytes)
				assert.Equal(t, pbeth.BalanceChange_REASON_SUICIDE_REFUND, refund.Reason)
			})
	})

	t.Run("suicide_to_self", func(t *testing.T) {
		// Contract suicides to itself (edge case)
		contractBalance := bigInt(1000)

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			// Bob self-destructs to itself
			Suicide(BobAddr, BobAddr, contractBalance).
			EndCall([]byte{}, 50000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				rootCall := trx.Calls[0]

				// Verify suicide flag
				assert.True(t, rootCall.Suicide)
				assert.True(t, rootCall.ExecutedCode)

				// Balance changes for suicide-to-self
				balanceChanges := rootCall.BalanceChanges
				assert.Equal(t, 2, len(balanceChanges))

				// Withdraw from contract
				withdraw := balanceChanges[0]
				assert.Equal(t, BobAddr[:], withdraw.Address)
				assert.Equal(t, pbeth.BalanceChange_REASON_SUICIDE_WITHDRAW, withdraw.Reason)

				// Refund to same address (beneficiary == contract)
				refund := balanceChanges[1]
				assert.Equal(t, BobAddr[:], refund.Address)
				assert.Equal(t, pbeth.BalanceChange_REASON_SUICIDE_REFUND, refund.Reason)
			})
	})

	t.Run("suicide_with_zero_balance", func(t *testing.T) {
		// Contract with zero balance suicides — in v5 the 0→0 balance changes
		// are skipped because the tracer now filters equivalent changes.
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			// Bob has zero balance
			Suicide(BobAddr, CharlieAddr, bigInt(0)).
			EndCall([]byte{}, 50000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				rootCall := trx.Calls[0]

				// Still marked as suicide even with zero balance
				assert.True(t, rootCall.Suicide)
				assert.True(t, rootCall.ExecutedCode)

				// v5: 0→0 balance changes are now skipped
				assert.Equal(t, 0, len(rootCall.BalanceChanges), "Zero-to-zero balance changes should be skipped in v5")
			})
	})

	t.Run("suicide_in_nested_call", func(t *testing.T) {
		// Suicide happens in a nested call, not root
		contractBalance := bigInt(750)

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 200000, []byte{}).
			// Bob calls Charlie
			StartCall(BobAddr, CharlieAddr, bigInt(0), 100000, []byte{}).
			// Charlie self-destructs
			Suicide(CharlieAddr, MinerAddr, contractBalance).
			EndCall([]byte{}, 50000).  // Charlie returns
			EndCall([]byte{}, 150000). // Bob returns
			EndBlockTrx(successReceipt(200000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				assert.Equal(t, 2, len(trx.Calls), "Should have 2 calls")

				// Root call (Bob) - NOT suicided
				rootCall := trx.Calls[0]
				assert.False(t, rootCall.Suicide, "Root call should not be suicided")

				// Nested call (Charlie) - IS suicided
				nestedCall := trx.Calls[1]
				assert.True(t, nestedCall.Suicide, "Nested call should be suicided")
				assert.True(t, nestedCall.ExecutedCode)

				// Balance changes on the nested call
				balanceChanges := nestedCall.BalanceChanges
				assert.Equal(t, 2, len(balanceChanges))

				withdraw := balanceChanges[0]
				assert.Equal(t, CharlieAddr[:], withdraw.Address)
				assert.Equal(t, pbeth.BalanceChange_REASON_SUICIDE_WITHDRAW, withdraw.Reason)

				refund := balanceChanges[1]
				assert.Equal(t, MinerAddr[:], refund.Address)
				assert.Equal(t, pbeth.BalanceChange_REASON_SUICIDE_REFUND, refund.Reason)
			})
	})

	t.Run("multiple_suicides_in_transaction", func(t *testing.T) {
		// Multiple contracts suicide in same transaction
		balance1 := bigInt(100)
		balance2 := bigInt(200)

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 300000, []byte{}).
			// First suicide: Bob → Charlie
			Suicide(BobAddr, CharlieAddr, balance1).
			// Bob calls Miner
			StartCall(BobAddr, MinerAddr, bigInt(0), 100000, []byte{}).
			// Second suicide: Miner → Alice
			Suicide(MinerAddr, AliceAddr, balance2).
			EndCall([]byte{}, 50000).  // Miner returns
			EndCall([]byte{}, 250000). // Bob returns
			EndBlockTrx(successReceipt(300000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]

				// Root call - first suicide
				rootCall := trx.Calls[0]
				assert.True(t, rootCall.Suicide)
				assert.Equal(t, 2, len(rootCall.BalanceChanges))

				// Nested call - second suicide
				nestedCall := trx.Calls[1]
				assert.True(t, nestedCall.Suicide)
				assert.Equal(t, 2, len(nestedCall.BalanceChanges))

				// Verify different beneficiaries
				assert.Equal(t, CharlieAddr[:], rootCall.BalanceChanges[1].Address)
				assert.Equal(t, AliceAddr[:], nestedCall.BalanceChanges[1].Address)
			})
	})

	t.Run("suicide_with_storage_and_logs", func(t *testing.T) {
		// Suicide combined with storage changes and logs
		contractBalance := bigInt(300)
		storageKey := hash32(1)
		storageValue := hash32(42)
		var zeroVal [32]byte

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 200000, []byte{}).
			// Storage change before suicide
			StorageChange(BobAddr, storageKey, zeroVal, storageValue).
			// Log event
			Log(BobAddr, [][32]byte{hash32(100)}, []byte{0x01, 0x02}, 0).
			// Then suicide
			Suicide(BobAddr, CharlieAddr, contractBalance).
			EndCall([]byte{}, 150000).
			EndBlockTrx(receiptWithLogs(200000, []firehose.LogData{
				{Address: BobAddr, Topics: [][32]byte{hash32(100)}, Data: []byte{0x01, 0x02}},
			}), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				rootCall := trx.Calls[0]

				// Verify suicide
				assert.True(t, rootCall.Suicide)

				// Verify storage change exists
				assert.Equal(t, 1, len(rootCall.StorageChanges))
				assert.Equal(t, storageKey[:], rootCall.StorageChanges[0].Key)

				// Verify log exists
				assert.Equal(t, 1, len(rootCall.Logs))
				assert.Equal(t, BobAddr[:], rootCall.Logs[0].Address)

				// Verify balance changes (suicide)
				assert.Equal(t, 2, len(rootCall.BalanceChanges))
				assert.Equal(t, pbeth.BalanceChange_REASON_SUICIDE_WITHDRAW, rootCall.BalanceChanges[0].Reason)
				assert.Equal(t, pbeth.BalanceChange_REASON_SUICIDE_REFUND, rootCall.BalanceChanges[1].Reason)
			})
	})

	t.Run("suicide_ordinal_assignment", func(t *testing.T) {
		// Verify ordinals are properly assigned for suicide balance changes
		contractBalance := bigInt(500)

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			Suicide(BobAddr, CharlieAddr, contractBalance).
			EndCall([]byte{}, 50000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				rootCall := trx.Calls[0]

				balanceChanges := rootCall.BalanceChanges
				assert.Equal(t, 2, len(balanceChanges))

				// Ordinals should be assigned and sequential
				withdraw := balanceChanges[0]
				refund := balanceChanges[1]

				assert.NotEqual(t, uint64(0), withdraw.Ordinal, "Withdraw should have ordinal")
				assert.NotEqual(t, uint64(0), refund.Ordinal, "Refund should have ordinal")
				assert.True(t, withdraw.Ordinal < refund.Ordinal, "Withdraw ordinal should be before refund")
			})
	})
}
