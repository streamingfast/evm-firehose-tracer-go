package tests

import (
	"math/big"
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go/v5"
	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTracer_BlockLevelBalanceChanges tests balance changes that occur at block level
// (outside of transactions): miner rewards, uncle rewards, transaction fee distribution
func TestTracer_BlockLevelBalanceChanges(t *testing.T) {
	t.Run("miner_block_reward", func(t *testing.T) {
		// Miner receives block reward at end of block
		blockReward := mustBigInt("2000000000000000000") // 2 ETH

		NewTracerTester(t).
			StartBlock().
			// Transaction happens
			StartTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndTrx(successReceipt(21000), nil).
			// Block-level reward (outside transaction)
			BalanceChange(MinerAddr, bigInt(0), blockReward, pbeth.BalanceChange_REASON_REWARD_MINE_BLOCK).
			EndBlock(nil).
			Validate(func(block *pbeth.Block) {
				// Verify block has the miner reward
				assert.Equal(t, 1, len(block.BalanceChanges))

				reward := block.BalanceChanges[0]
				assert.Equal(t, MinerAddr[:], reward.Address)
				assert.Nil(t, reward.OldValue) // 0 balance
				assert.Equal(t, blockReward.Bytes(), reward.NewValue.Bytes)
				assert.Equal(t, pbeth.BalanceChange_REASON_REWARD_MINE_BLOCK, reward.Reason)

				// Reward should have ordinal after transaction
				trx := block.TransactionTraces[0]
				assert.True(t, reward.Ordinal > trx.EndOrdinal,
					"Block reward should come after transaction")
			})
	})

	t.Run("uncle_reward", func(t *testing.T) {
		// Uncle miner receives reward for uncle block
		uncleReward := mustBigInt("1750000000000000000") // 1.75 ETH (7/8 of block reward)

		NewTracerTester(t).
			StartBlock().
			// Uncle reward at block level
			BalanceChange(CharlieAddr, bigInt(0), uncleReward, pbeth.BalanceChange_REASON_REWARD_MINE_UNCLE).
			EndBlock(nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 1, len(block.BalanceChanges))

				reward := block.BalanceChanges[0]
				assert.Equal(t, CharlieAddr[:], reward.Address)
				assert.Equal(t, pbeth.BalanceChange_REASON_REWARD_MINE_UNCLE, reward.Reason)
				assert.Equal(t, uncleReward.Bytes(), reward.NewValue.Bytes)
			})
	})

	t.Run("transaction_fee_reward", func(t *testing.T) {
		// Miner receives transaction fees
		gasUsed := uint64(21000)
		txFee := bigInt(1_050_000_000_000_000) // 21000 * 50 gwei

		NewTracerTester(t).
			StartBlock().
			StartTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), gasUsed, []byte{}).
			EndCall([]byte{}, gasUsed).
			EndTrx(successReceipt(gasUsed), nil).
			// Transaction fee reward to miner
			BalanceChange(MinerAddr, bigInt(0), txFee, pbeth.BalanceChange_REASON_REWARD_TRANSACTION_FEE).
			EndBlock(nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 1, len(block.BalanceChanges))

				reward := block.BalanceChanges[0]
				assert.Equal(t, MinerAddr[:], reward.Address)
				assert.Equal(t, pbeth.BalanceChange_REASON_REWARD_TRANSACTION_FEE, reward.Reason)
				assert.Equal(t, txFee.Bytes(), reward.NewValue.Bytes)
			})
	})

	t.Run("multiple_rewards_combined", func(t *testing.T) {
		// Block with: block reward + uncle reward + transaction fees
		blockReward := bigInt(2_000_000_000_000_000_000)
		uncleReward := bigInt(1_750_000_000_000_000_000)
		txFee := bigInt(1_050_000_000_000_000)

		NewTracerTester(t).
			StartBlock().
			// First transaction
			StartTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndTrx(successReceipt(21000), nil).
			// Second transaction
			StartTrx(TestLegacyTrx).
			StartCall(BobAddr, CharlieAddr, bigInt(50), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndTrx(successReceipt(21000), nil).
			// Block-level rewards
			BalanceChange(MinerAddr, bigInt(0), blockReward, pbeth.BalanceChange_REASON_REWARD_MINE_BLOCK).
			BalanceChange(CharlieAddr, bigInt(0), uncleReward, pbeth.BalanceChange_REASON_REWARD_MINE_UNCLE).
			BalanceChange(MinerAddr, blockReward, bigInt(0).Add(blockReward, txFee), pbeth.BalanceChange_REASON_REWARD_TRANSACTION_FEE).
			EndBlock(nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 2, len(block.TransactionTraces))
				assert.Equal(t, 3, len(block.BalanceChanges))

				// Verify all rewards are present
				rewards := block.BalanceChanges
				assert.Equal(t, pbeth.BalanceChange_REASON_REWARD_MINE_BLOCK, rewards[0].Reason)
				assert.Equal(t, pbeth.BalanceChange_REASON_REWARD_MINE_UNCLE, rewards[1].Reason)
				assert.Equal(t, pbeth.BalanceChange_REASON_REWARD_TRANSACTION_FEE, rewards[2].Reason)

				// Verify ordinal ordering (rewards after all transactions)
				lastTrx := block.TransactionTraces[1]
				for _, reward := range rewards {
					assert.True(t, reward.Ordinal > lastTrx.EndOrdinal,
						"All rewards should come after transactions")
				}
			})
	})

	t.Run("block_with_no_transactions_only_rewards", func(t *testing.T) {
		// Empty block that still gets miner reward
		blockReward := bigInt(2_000_000_000_000_000_000)

		NewTracerTester(t).
			StartBlock().
			BalanceChange(MinerAddr, bigInt(0), blockReward, pbeth.BalanceChange_REASON_REWARD_MINE_BLOCK).
			EndBlock(nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 0, len(block.TransactionTraces), "No transactions")
				assert.Equal(t, 1, len(block.BalanceChanges), "Has block reward")

				reward := block.BalanceChanges[0]
				assert.Equal(t, pbeth.BalanceChange_REASON_REWARD_MINE_BLOCK, reward.Reason)
			})
	})
}

// TestTracer_BlockLevelCodeChanges tests code changes that occur at block level
// (outside transactions): consensus upgrades, hard forks, etc.
func TestTracer_BlockLevelCodeChanges(t *testing.T) {
	t.Run("block_level_code_deployment", func(t *testing.T) {
		// Code deployed at block level (e.g., system contract upgrade)
		contractCode := []byte{0x60, 0x80, 0x60, 0x40, 0x52} // Simple bytecode
		var emptyHash [32]byte

		NewTracerTester(t).
			StartBlock().
			// Transaction happens first
			StartTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndTrx(successReceipt(21000), nil).
			// Block-level code change (outside transaction)
			CodeChange(SystemAddress, emptyHash, hash32(12345), []byte{}, contractCode).
			EndBlock(nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 1, len(block.CodeChanges))

				codeChange := block.CodeChanges[0]
				assert.Equal(t, SystemAddress[:], codeChange.Address)
				assert.Equal(t, emptyHash[:], codeChange.OldHash)
				assert.Empty(t, codeChange.OldCode)
				newHash := hash32(12345)
				assert.Equal(t, newHash[:], codeChange.NewHash)
				assert.Equal(t, contractCode, codeChange.NewCode)

				// Code change should have ordinal after transaction
				trx := block.TransactionTraces[0]
				assert.True(t, codeChange.Ordinal > trx.EndOrdinal,
					"Block-level code change should come after transaction")
			})
	})

	t.Run("block_level_code_update", func(t *testing.T) {
		// Code updated at block level (e.g., hard fork contract modification)
		oldCode := []byte{0x60, 0x01}
		newCode := []byte{0x60, 0x02, 0x60, 0x03}

		NewTracerTester(t).
			StartBlock().
			CodeChange(BeaconRootsAddress, hash32(100), hash32(200), oldCode, newCode).
			EndBlock(nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 1, len(block.CodeChanges))

				codeChange := block.CodeChanges[0]
				assert.Equal(t, BeaconRootsAddress[:], codeChange.Address)
				assert.Equal(t, oldCode, codeChange.OldCode)
				assert.Equal(t, newCode, codeChange.NewCode)
			})
	})

	t.Run("multiple_block_level_code_changes", func(t *testing.T) {
		// Multiple contracts updated at block level (e.g., multi-contract hard fork)
		code1 := []byte{0x60, 0x01}
		code2 := []byte{0x60, 0x02}
		var emptyHash [32]byte

		NewTracerTester(t).
			StartBlock().
			CodeChange(BeaconRootsAddress, emptyHash, hash32(1), []byte{}, code1).
			CodeChange(HistoryStorageAddress, emptyHash, hash32(2), []byte{}, code2).
			EndBlock(nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 2, len(block.CodeChanges))

				// Verify both code changes
				assert.Equal(t, BeaconRootsAddress[:], block.CodeChanges[0].Address)
				assert.Equal(t, HistoryStorageAddress[:], block.CodeChanges[1].Address)

				// Verify ordinal ordering
				assert.True(t, block.CodeChanges[0].Ordinal < block.CodeChanges[1].Ordinal)
			})
	})
}

// TestTracer_BlockLevelWithdrawals tests EIP-4895 beacon chain withdrawals
func TestTracer_BlockLevelWithdrawals(t *testing.T) {
	t.Run("single_withdrawal", func(t *testing.T) {
		// Single validator withdrawal
		withdrawalAmount := mustBigInt("32000000000000000000") // 32 ETH

		NewTracerTester(t).
			StartBlock().
			// Withdrawal happens (balance change with WITHDRAWAL reason)
			BalanceChange(AliceAddr, bigInt(0), withdrawalAmount, pbeth.BalanceChange_REASON_WITHDRAWAL).
			EndBlock(nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 1, len(block.BalanceChanges))

				withdrawal := block.BalanceChanges[0]
				assert.Equal(t, AliceAddr[:], withdrawal.Address)
				assert.Equal(t, pbeth.BalanceChange_REASON_WITHDRAWAL, withdrawal.Reason)
				assert.Equal(t, withdrawalAmount.Bytes(), withdrawal.NewValue.Bytes)
			})
	})

	t.Run("multiple_withdrawals", func(t *testing.T) {
		// Multiple validator withdrawals in same block
		amount1 := mustBigInt("32000000000000000000")
		amount2 := mustBigInt("16000000000000000000")
		amount3 := mustBigInt("8000000000000000000")

		NewTracerTester(t).
			StartBlock().
			BalanceChange(AliceAddr, bigInt(0), amount1, pbeth.BalanceChange_REASON_WITHDRAWAL).
			BalanceChange(BobAddr, bigInt(0), amount2, pbeth.BalanceChange_REASON_WITHDRAWAL).
			BalanceChange(CharlieAddr, bigInt(0), amount3, pbeth.BalanceChange_REASON_WITHDRAWAL).
			EndBlock(nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 3, len(block.BalanceChanges))

				// Verify all are withdrawals
				for _, withdrawal := range block.BalanceChanges {
					assert.Equal(t, pbeth.BalanceChange_REASON_WITHDRAWAL, withdrawal.Reason)
				}

				// Verify addresses
				assert.Equal(t, AliceAddr[:], block.BalanceChanges[0].Address)
				assert.Equal(t, BobAddr[:], block.BalanceChanges[1].Address)
				assert.Equal(t, CharlieAddr[:], block.BalanceChanges[2].Address)
			})
	})

	t.Run("withdrawals_and_transactions", func(t *testing.T) {
		// Block with both transactions and withdrawals
		withdrawalAmount := mustBigInt("32000000000000000000")

		NewTracerTester(t).
			StartBlock().
			// Transaction
			StartTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndTrx(successReceipt(21000), nil).
			// Withdrawal after transaction
			BalanceChange(CharlieAddr, bigInt(0), withdrawalAmount, pbeth.BalanceChange_REASON_WITHDRAWAL).
			EndBlock(nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 1, len(block.TransactionTraces))
				assert.Equal(t, 1, len(block.BalanceChanges))

				withdrawal := block.BalanceChanges[0]
				assert.Equal(t, pbeth.BalanceChange_REASON_WITHDRAWAL, withdrawal.Reason)

				// Withdrawal should come after transaction
				trx := block.TransactionTraces[0]
				assert.True(t, withdrawal.Ordinal > trx.EndOrdinal)
			})
	})

	t.Run("withdrawals_and_rewards_combined", func(t *testing.T) {
		// Block with withdrawals, miner rewards, and transactions
		withdrawalAmount := mustBigInt("32000000000000000000")
		blockReward := bigInt(2_000_000_000_000_000_000)

		NewTracerTester(t).
			StartBlock().
			// Transaction
			StartTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndTrx(successReceipt(21000), nil).
			// Block-level state changes
			BalanceChange(CharlieAddr, bigInt(0), withdrawalAmount, pbeth.BalanceChange_REASON_WITHDRAWAL).
			BalanceChange(MinerAddr, bigInt(0), blockReward, pbeth.BalanceChange_REASON_REWARD_MINE_BLOCK).
			EndBlock(nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 1, len(block.TransactionTraces))
				assert.Equal(t, 2, len(block.BalanceChanges))

				// Verify both balance changes
				assert.Equal(t, pbeth.BalanceChange_REASON_WITHDRAWAL, block.BalanceChanges[0].Reason)
				assert.Equal(t, pbeth.BalanceChange_REASON_REWARD_MINE_BLOCK, block.BalanceChanges[1].Reason)

				// Both should come after transaction
				trx := block.TransactionTraces[0]
				assert.True(t, block.BalanceChanges[0].Ordinal > trx.EndOrdinal)
				assert.True(t, block.BalanceChanges[1].Ordinal > trx.EndOrdinal)
			})
	})
}

// TestTracer_ComplexBlockScenarios tests complex combinations of block-level state changes
func TestTracer_ComplexBlockScenarios(t *testing.T) {
	t.Run("full_block_with_all_state_types", func(t *testing.T) {
		// Block with: system call, transactions, withdrawals, rewards, code changes
		withdrawalAmount := mustBigInt("32000000000000000000")
		blockReward := bigInt(2_000_000_000_000_000_000)
		contractCode := []byte{0x60, 0x80}
		beaconRoot := hash32(12345)
		var emptyHash [32]byte

		NewTracerTester(t).
			StartBlock().
			// System call
			SystemCall(SystemAddress, BeaconRootsAddress, beaconRoot[:], 30_000_000, []byte{}, 50_000).
			// Transaction
			StartTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndTrx(successReceipt(21000), nil).
			// Block-level state changes
			BalanceChange(CharlieAddr, bigInt(0), withdrawalAmount, pbeth.BalanceChange_REASON_WITHDRAWAL).
			BalanceChange(MinerAddr, bigInt(0), blockReward, pbeth.BalanceChange_REASON_REWARD_MINE_BLOCK).
			CodeChange(HistoryStorageAddress, emptyHash, hash32(999), []byte{}, contractCode).
			EndBlock(nil).
			Validate(func(block *pbeth.Block) {
				// Verify all components present
				assert.Equal(t, 1, len(block.SystemCalls), "Has system call")
				assert.Equal(t, 1, len(block.TransactionTraces), "Has transaction")
				assert.Equal(t, 2, len(block.BalanceChanges), "Has withdrawals and rewards")
				assert.Equal(t, 1, len(block.CodeChanges), "Has code change")

				// Verify ordinal ordering
				sysCall := block.SystemCalls[0]
				trx := block.TransactionTraces[0]
				withdrawal := block.BalanceChanges[0]
				reward := block.BalanceChanges[1]
				codeChange := block.CodeChanges[0]

				// System call < Transaction < Block-level changes
				assert.True(t, sysCall.EndOrdinal < trx.BeginOrdinal)
				assert.True(t, trx.EndOrdinal < withdrawal.Ordinal)
				assert.True(t, trx.EndOrdinal < reward.Ordinal)
				assert.True(t, trx.EndOrdinal < codeChange.Ordinal)
			})
	})

	t.Run("block_with_multiple_system_calls_and_rewards", func(t *testing.T) {
		// System calls + transactions + rewards
		beaconRoot := hash32(11111)
		parentHash := hash32(22222)
		blockReward := bigInt(2_000_000_000_000_000_000)
		uncleReward := bigInt(1_750_000_000_000_000_000)

		NewTracerTester(t).
			StartBlock().
			// Multiple system calls
			SystemCall(SystemAddress, BeaconRootsAddress, beaconRoot[:], 30_000_000, []byte{}, 50_000).
			SystemCall(SystemAddress, HistoryStorageAddress, parentHash[:], 30_000_000, []byte{}, 45_000).
			// Transaction
			StartTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndTrx(successReceipt(21000), nil).
			// Rewards
			BalanceChange(MinerAddr, bigInt(0), blockReward, pbeth.BalanceChange_REASON_REWARD_MINE_BLOCK).
			BalanceChange(CharlieAddr, bigInt(0), uncleReward, pbeth.BalanceChange_REASON_REWARD_MINE_UNCLE).
			EndBlock(nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 2, len(block.SystemCalls))
				assert.Equal(t, 1, len(block.TransactionTraces))
				assert.Equal(t, 2, len(block.BalanceChanges))

				// Verify ordering: sys calls < transaction < rewards
				assert.True(t, block.SystemCalls[1].EndOrdinal < block.TransactionTraces[0].BeginOrdinal)
				assert.True(t, block.TransactionTraces[0].EndOrdinal < block.BalanceChanges[0].Ordinal)
			})
	})
}

// blockEventWithWithdrawals builds a BlockEvent that carries the given withdrawals list
func blockEventWithWithdrawals(withdrawals []firehose.WithdrawalData) firehose.BlockEvent {
	withdrawalsRoot := hash32(999)
	return firehose.BlockEvent{
		Block: firehose.BlockData{
			Number:          100,
			Hash:            hash32(1),
			ParentHash:      hash32(2),
			UncleHash:       hash32(3),
			Coinbase:        AliceAddr,
			Root:            hash32(4),
			TxHash:          hash32(5),
			ReceiptHash:     hash32(6),
			Bloom:           make([]byte, 256),
			Difficulty:      big.NewInt(0),
			GasLimit:        30_000_000,
			Time:            1704067200,
			Size:            512,
			Withdrawals:     withdrawals,
			WithdrawalsRoot: &withdrawalsRoot,
		},
	}
}

// TestTracer_WithdrawalRecording tests that block.Withdrawals are always recorded in v5
func TestTracer_WithdrawalRecording(t *testing.T) {
	withdrawals := []firehose.WithdrawalData{
		{Index: 0, ValidatorIndex: 1, Address: AliceAddr, Amount: 32_000_000_000},
		{Index: 1, ValidatorIndex: 2, Address: BobAddr, Amount: 16_000_000_000},
	}

	t.Run("withdrawals_always_recorded", func(t *testing.T) {
		NewTracerTester(t).ValidateWithCustomBlock(blockEventWithWithdrawals(withdrawals), func(block *pbeth.Block) {
			require.Len(t, block.Withdrawals, 2)
			assert.Equal(t, uint64(0), block.Withdrawals[0].Index)
			assert.Equal(t, uint64(1), block.Withdrawals[0].ValidatorIndex)
			assert.Equal(t, AliceAddr[:], block.Withdrawals[0].Address)
			assert.Equal(t, uint64(32_000_000_000), block.Withdrawals[0].Amount)
			assert.Equal(t, uint64(1), block.Withdrawals[1].Index)
			assert.Equal(t, BobAddr[:], block.Withdrawals[1].Address)
		})
	})

	t.Run("withdrawals_root_always_set_in_header", func(t *testing.T) {
		NewTracerTester(t).ValidateWithCustomBlock(blockEventWithWithdrawals(withdrawals), func(block *pbeth.Block) {
			require.NotNil(t, block.Header.WithdrawalsRoot, "header WithdrawalsRoot should be set")
			expectedRoot := hash32(999)
			assert.Equal(t, expectedRoot[:], block.Header.WithdrawalsRoot)
		})
	})

	t.Run("no_withdrawals_in_block_event", func(t *testing.T) {
		NewTracerTester(t).ValidateWithCustomBlock(blockEventWithWithdrawals(nil), func(block *pbeth.Block) {
			assert.Empty(t, block.Withdrawals)
		})
	})
}
