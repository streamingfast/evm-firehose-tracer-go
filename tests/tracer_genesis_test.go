package tests

import (
	"math/big"
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go/v4"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTracer_GenesisBlock tests genesis block processing
func TestTracer_GenesisBlock(t *testing.T) {
	t.Run("empty_genesis", func(t *testing.T) {
		// Scenario: Genesis block with no accounts
		alloc := firehose.GenesisAlloc{}

		NewTracerTester(t).
			GenesisBlock(0, hash32(100), alloc).
			Validate(func(block *pbeth.Block) {
				require.Equal(t, uint64(0), block.Number, "Block number should be 0")
				require.Equal(t, 1, len(block.TransactionTraces), "Should have 1 synthetic transaction")

				trx := block.TransactionTraces[0]
				require.Equal(t, 1, len(trx.Calls), "Should have 1 synthetic call")

				call := trx.Calls[0]
				assert.Equal(t, 0, len(call.BalanceChanges), "Should have no balance changes")
				assert.Equal(t, 0, len(call.CodeChanges), "Should have no code changes")
				assert.Equal(t, 0, len(call.NonceChanges), "Should have no nonce changes")
				assert.Equal(t, 0, len(call.StorageChanges), "Should have no storage changes")
			})
	})

	t.Run("single_account_with_balance", func(t *testing.T) {
		// Scenario: Genesis block with one account having only balance
		alloc := firehose.GenesisAlloc{
			AliceAddr: {
				Balance: mustBigInt("1000000000000000000"), // 1 ETH
			},
		}

		NewTracerTester(t).
			GenesisBlock(0, hash32(100), alloc).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Should have 1 balance change
				require.Equal(t, 1, len(call.BalanceChanges), "Should have 1 balance change")

				change := call.BalanceChanges[0]
				assert.Equal(t, AliceAddr[:], change.Address, "firehose.Address should be Alice")
				assert.Nil(t, change.OldValue, "Old value should be nil (zero)")
				require.NotNil(t, change.NewValue, "New value should not be nil")
				assert.Equal(t, mustBigInt("1000000000000000000"), new(big.Int).SetBytes(change.NewValue.Bytes), "New value should be 1 ETH")
				assert.Equal(t, pbeth.BalanceChange_REASON_GENESIS_BALANCE, change.Reason, "Reason should be GENESIS_BALANCE")
			})
	})

	t.Run("single_account_with_code", func(t *testing.T) {
		// Scenario: Genesis block with contract deployment
		contractCode := []byte{0x60, 0x80, 0x60, 0x40, 0x52}

		alloc := firehose.GenesisAlloc{
			BobAddr: {
				Code:    contractCode,
				Balance: bigInt(0),
			},
		}

		NewTracerTester(t).
			GenesisBlock(0, hash32(100), alloc).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Should have 1 code change
				require.Equal(t, 1, len(call.CodeChanges), "Should have 1 code change")

				change := call.CodeChanges[0]
				assert.Equal(t, BobAddr[:], change.Address, "firehose.Address should be Bob")
				assert.Equal(t, firehose.EmptyHash[:], change.OldHash, "Old hash should be empty")
				assert.Equal(t, contractCode, change.NewCode, "New code should match")

				// Verify code hash is computed correctly
				expectedHash := hashBytes(contractCode)
				assert.Equal(t, expectedHash[:], change.NewHash, "Code hash should be keccak256 of code")
			})
	})

	t.Run("single_account_with_nonce", func(t *testing.T) {
		// Scenario: Genesis block with account having a nonce
		alloc := firehose.GenesisAlloc{
			CharlieAddr: {
				Nonce:   42,
				Balance: bigInt(0),
			},
		}

		NewTracerTester(t).
			GenesisBlock(0, hash32(100), alloc).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Should have 1 nonce change
				require.Equal(t, 1, len(call.NonceChanges), "Should have 1 nonce change")

				change := call.NonceChanges[0]
				assert.Equal(t, CharlieAddr[:], change.Address, "firehose.Address should be Charlie")
				assert.Equal(t, uint64(0), change.OldValue, "Old nonce should be 0")
				assert.Equal(t, uint64(42), change.NewValue, "New nonce should be 42")
			})
	})

	t.Run("single_account_with_storage", func(t *testing.T) {
		// Scenario: Genesis block with contract having storage
		alloc := firehose.GenesisAlloc{
			BobAddr: {
				Storage: map[[32]byte][32]byte{
					hash32(1): hash32(100),
					hash32(2): hash32(200),
				},
				Balance: bigInt(0),
			},
		}

		NewTracerTester(t).
			GenesisBlock(0, hash32(100), alloc).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Should have 2 storage changes
				require.Equal(t, 2, len(call.StorageChanges), "Should have 2 storage changes")

				// Storage changes should be sorted by key
				// hash32(1) < hash32(2) in bytes order
				change0 := call.StorageChanges[0]
				change1 := call.StorageChanges[1]

				key1 := hash32(1)
				key2 := hash32(2)
				val100 := hash32(100)
				val200 := hash32(200)

				assert.Equal(t, BobAddr[:], change0.Address, "firehose.Address should be Bob")
				assert.Equal(t, key1[:], change0.Key, "First key should be hash32(1)")
				assert.Equal(t, firehose.EmptyHash[:], change0.OldValue, "Old value should be empty")
				assert.Equal(t, val100[:], change0.NewValue, "New value should be hash32(100)")

				assert.Equal(t, key2[:], change1.Key, "Second key should be hash32(2)")
				assert.Equal(t, val200[:], change1.NewValue, "New value should be hash32(200)")
			})
	})

	t.Run("complete_account", func(t *testing.T) {
		// Scenario: Account with all fields populated
		contractCode := []byte{0x60, 0x80, 0x60, 0x40, 0x52}

		alloc := firehose.GenesisAlloc{
			AliceAddr: {
				Balance: mustBigInt("5000000000000000000"), // 5 ETH
				Code:    contractCode,
				Nonce:   10,
				Storage: map[[32]byte][32]byte{
					hash32(1): hash32(111),
					hash32(2): hash32(222),
					hash32(3): hash32(333),
				},
			},
		}

		NewTracerTester(t).
			GenesisBlock(0, hash32(100), alloc).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Verify all changes are present
				assert.Equal(t, 1, len(call.BalanceChanges), "Should have 1 balance change")
				assert.Equal(t, 1, len(call.CodeChanges), "Should have 1 code change")
				assert.Equal(t, 1, len(call.NonceChanges), "Should have 1 nonce change")
				assert.Equal(t, 3, len(call.StorageChanges), "Should have 3 storage changes")

				// Verify balance
				assert.Equal(t, mustBigInt("5000000000000000000"), new(big.Int).SetBytes(call.BalanceChanges[0].NewValue.Bytes))

				// Verify code
				assert.Equal(t, contractCode, call.CodeChanges[0].NewCode)

				// Verify nonce
				assert.Equal(t, uint64(10), call.NonceChanges[0].NewValue)

				// Verify storage is sorted
				key1 := hash32(1)
				key2 := hash32(2)
				key3 := hash32(3)
				assert.Equal(t, key1[:], call.StorageChanges[0].Key)
				assert.Equal(t, key2[:], call.StorageChanges[1].Key)
				assert.Equal(t, key3[:], call.StorageChanges[2].Key)
			})
	})
}

// TestTracer_GenesisBlock_Ordering tests deterministic ordering of genesis changes
func TestTracer_GenesisBlock_Ordering(t *testing.T) {
	t.Run("multiple_accounts_sorted_by_address", func(t *testing.T) {
		// Scenario: Multiple accounts should be processed in sorted address order
		// Create accounts with deliberately unsorted addresses
		alloc := firehose.GenesisAlloc{
			CharlieAddr: {Balance: bigInt(300)}, // CharlieAddr bytes: 0x7e5f45...
			AliceAddr:   {Balance: bigInt(100)}, // AliceAddr bytes: 0x7e5f45... (but different)
			BobAddr:     {Balance: bigInt(200)}, // BobAddr bytes: 0x2b5ad5...
		}

		NewTracerTester(t).
			GenesisBlock(0, hash32(100), alloc).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				require.Equal(t, 3, len(call.BalanceChanges), "Should have 3 balance changes")

				// Balance changes should be ordered by address bytes
				// Sorted order: BobAddr (0x2b5a...) < CharlieAddr (0x6813eb...) < AliceAddr (0x7e5f...)
				change0 := call.BalanceChanges[0]
				change1 := call.BalanceChanges[1]
				change2 := call.BalanceChanges[2]

				// Verify they are in sorted order
				assert.Equal(t, BobAddr[:], change0.Address, "First should be BobAddr")
				assert.Equal(t, CharlieAddr[:], change1.Address, "Second should be CharlieAddr")
				assert.Equal(t, AliceAddr[:], change2.Address, "Third should be AliceAddr")

				// Verify values match
				assert.Equal(t, mustBigInt("200"), new(big.Int).SetBytes(change0.NewValue.Bytes))
				assert.Equal(t, mustBigInt("300"), new(big.Int).SetBytes(change1.NewValue.Bytes))
				assert.Equal(t, mustBigInt("100"), new(big.Int).SetBytes(change2.NewValue.Bytes))
			})
	})

	t.Run("storage_keys_sorted", func(t *testing.T) {
		// Scenario: Storage keys should be sorted deterministically
		alloc := firehose.GenesisAlloc{
			AliceAddr: {
				Storage: map[[32]byte][32]byte{
					hash32(999): hash32(9990),
					hash32(1):   hash32(10),
					hash32(500): hash32(5000),
					hash32(100): hash32(1000),
				},
				Balance: bigInt(0),
			},
		}

		NewTracerTester(t).
			GenesisBlock(0, hash32(100), alloc).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				require.Equal(t, 4, len(call.StorageChanges), "Should have 4 storage changes")

				// Extract keys to verify sorting
				keys := make([][32]byte, 4)
				for i, change := range call.StorageChanges {
					copy(keys[i][:], change.Key)
				}

				// Verify keys are in sorted order
				assert.Equal(t, hash32(1), keys[0], "First key should be hash32(1)")
				assert.Equal(t, hash32(100), keys[1], "Second key should be hash32(100)")
				assert.Equal(t, hash32(500), keys[2], "Third key should be hash32(500)")
				assert.Equal(t, hash32(999), keys[3], "Fourth key should be hash32(999)")
			})
	})

	t.Run("deterministic_across_runs", func(t *testing.T) {
		// Scenario: Genesis block should produce identical output across multiple runs
		// This tests that map iteration order doesn't affect output
		alloc := firehose.GenesisAlloc{
			AliceAddr:   {Balance: mustBigInt("1000000000000000000")},
			BobAddr:     {Balance: mustBigInt("2000000000000000000")},
			CharlieAddr: {Balance: mustBigInt("3000000000000000000")},
		}

		// Run twice and compare
		var firstBlock *pbeth.Block
		var secondBlock *pbeth.Block

		NewTracerTester(t).
			GenesisBlock(0, hash32(100), alloc).
			Validate(func(block *pbeth.Block) {
				firstBlock = block
			})

		NewTracerTester(t).
			GenesisBlock(0, hash32(100), alloc).
			Validate(func(block *pbeth.Block) {
				secondBlock = block
			})

		// Blocks should be identical
		require.NotNil(t, firstBlock)
		require.NotNil(t, secondBlock)

		// Compare transaction traces
		require.Equal(t, len(firstBlock.TransactionTraces), len(secondBlock.TransactionTraces))
		require.Equal(t, len(firstBlock.TransactionTraces[0].Calls), len(secondBlock.TransactionTraces[0].Calls))

		firstCall := firstBlock.TransactionTraces[0].Calls[0]
		secondCall := secondBlock.TransactionTraces[0].Calls[0]

		// Balance changes should be identical and in same order
		require.Equal(t, len(firstCall.BalanceChanges), len(secondCall.BalanceChanges))
		for i := range firstCall.BalanceChanges {
			assert.Equal(t, firstCall.BalanceChanges[i].Address, secondCall.BalanceChanges[i].Address,
				"Balance change %d address should match", i)
			assert.Equal(t, firstCall.BalanceChanges[i].NewValue.Bytes, secondCall.BalanceChanges[i].NewValue.Bytes,
				"Balance change %d value should match", i)
		}
	})
}

// TestTracer_GenesisBlock_EdgeCases tests edge cases and special scenarios
func TestTracer_GenesisBlock_EdgeCases(t *testing.T) {
	t.Run("zero_balance_not_recorded", func(t *testing.T) {
		// Scenario: Accounts with zero balance should not have balance changes recorded
		alloc := firehose.GenesisAlloc{
			AliceAddr: {
				Balance: bigInt(0), // Zero balance
				Nonce:   1,         // But has nonce
			},
		}

		NewTracerTester(t).
			GenesisBlock(0, hash32(100), alloc).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Should have nonce change but no balance change
				assert.Equal(t, 0, len(call.BalanceChanges), "Zero balance should not be recorded")
				assert.Equal(t, 1, len(call.NonceChanges), "Nonce change should be recorded")
			})
	})

	t.Run("nil_balance_not_recorded", func(t *testing.T) {
		// Scenario: Accounts with nil balance should not have balance changes
		alloc := firehose.GenesisAlloc{
			BobAddr: {
				Balance: nil, // Nil balance
				Code:    []byte{0x60, 0x80},
			},
		}

		NewTracerTester(t).
			GenesisBlock(0, hash32(100), alloc).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Should have code change but no balance change
				assert.Equal(t, 0, len(call.BalanceChanges), "Nil balance should not be recorded")
				assert.Equal(t, 1, len(call.CodeChanges), "Code change should be recorded")
			})
	})

	t.Run("zero_nonce_not_recorded", func(t *testing.T) {
		// Scenario: Zero nonce should not be recorded
		alloc := firehose.GenesisAlloc{
			CharlieAddr: {
				Nonce:   0, // Zero nonce
				Balance: bigInt(100),
			},
		}

		NewTracerTester(t).
			GenesisBlock(0, hash32(100), alloc).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Should have balance change but no nonce change
				assert.Equal(t, 1, len(call.BalanceChanges), "Balance change should be recorded")
				assert.Equal(t, 0, len(call.NonceChanges), "Zero nonce should not be recorded")
			})
	})

	t.Run("empty_code_not_recorded", func(t *testing.T) {
		// Scenario: Empty code should not be recorded
		alloc := firehose.GenesisAlloc{
			AliceAddr: {
				Code:    []byte{}, // Empty code
				Balance: bigInt(100),
			},
		}

		NewTracerTester(t).
			GenesisBlock(0, hash32(100), alloc).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Should have balance change but no code change
				assert.Equal(t, 1, len(call.BalanceChanges), "Balance change should be recorded")
				assert.Equal(t, 0, len(call.CodeChanges), "Empty code should not be recorded")
			})
	})

	t.Run("empty_storage_not_recorded", func(t *testing.T) {
		// Scenario: Empty storage map should not record any changes
		alloc := firehose.GenesisAlloc{
			BobAddr: {
				Storage: map[[32]byte][32]byte{}, // Empty storage
				Balance: bigInt(100),
			},
		}

		NewTracerTester(t).
			GenesisBlock(0, hash32(100), alloc).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Should have balance change but no storage changes
				assert.Equal(t, 1, len(call.BalanceChanges), "Balance change should be recorded")
				assert.Equal(t, 0, len(call.StorageChanges), "Empty storage should not record changes")
			})
	})

	t.Run("receipt_status_success", func(t *testing.T) {
		// Scenario: Genesis transaction should have successful receipt
		alloc := firehose.GenesisAlloc{
			AliceAddr: {Balance: bigInt(100)},
		}

		NewTracerTester(t).
			GenesisBlock(0, hash32(100), alloc).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]

				// Transaction should be successful
				assert.Equal(t, pbeth.TransactionTraceStatus_SUCCEEDED, trx.Status,
					"Genesis transaction should succeed")

				// Receipt should exist and be successful
				require.NotNil(t, trx.Receipt, "Receipt should exist")
				// Genesis receipt has no logs
				assert.Equal(t, 0, len(trx.Receipt.Logs), "Genesis receipt should have no logs")
			})
	})
}
