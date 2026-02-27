package tests

import (
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
)

// TestTracer_OnStorageChange tests all storage change scenarios
func TestTracer_OnStorageChange(t *testing.T) {
	t.Run("basic_storage_change", func(t *testing.T) {
		// Basic storage change during call execution
		key := hash32(1)
		oldVal := hash32(100)
		newVal := hash32(200)
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			StorageChange(BobAddr, key, oldVal, newVal).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 1, len(call.StorageChanges))
				sc := call.StorageChanges[0]
				assert.Equal(t, BobAddr[:], sc.Address)
				assert.Equal(t, key[:], sc.Key)
				assert.Equal(t, oldVal[:], sc.OldValue)
				assert.Equal(t, newVal[:], sc.NewValue)
			})
	})

	t.Run("multiple_storage_changes_in_call", func(t *testing.T) {
		// Multiple storage changes in same call
		key1 := hash32(1)
		key2 := hash32(2)
		key3 := hash32(3)
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			StorageChange(BobAddr, key1, hash32(100), hash32(200)).
			StorageChange(BobAddr, key2, hash32(300), hash32(400)).
			StorageChange(BobAddr, key3, hash32(500), hash32(600)).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 3, len(call.StorageChanges), "Should have 3 storage changes")

				// Verify ordering (ordinals should be increasing)
				assert.True(t, call.StorageChanges[0].Ordinal < call.StorageChanges[1].Ordinal)
				assert.True(t, call.StorageChanges[1].Ordinal < call.StorageChanges[2].Ordinal)

				// Verify each storage change
				assert.Equal(t, key1[:], call.StorageChanges[0].Key)
				assert.Equal(t, key2[:], call.StorageChanges[1].Key)
				assert.Equal(t, key3[:], call.StorageChanges[2].Key)
			})
	})

	t.Run("multiple_calls_with_storage_changes", func(t *testing.T) {
		// Multiple calls, each with storage changes
		key1 := hash32(1)
		key2 := hash32(2)
		key10 := hash32(10)
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			StorageChange(BobAddr, key1, hash32(100), hash32(200)).
			StartCallRaw(1, byte(firehose.CallTypeCall), BobAddr, AliceAddr, []byte{}, 50000, bigInt(0)).
			StorageChange(AliceAddr, key10, hash32(1000), hash32(2000)).
			EndCall([]byte{}, 50000, nil).
			StorageChange(BobAddr, key2, hash32(300), hash32(400)).
			EndCall([]byte{}, 100000, nil).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]

				// Root call should have 2 storage changes
				rootCall := trx.Calls[0]
				assert.Equal(t, 2, len(rootCall.StorageChanges))
				assert.Equal(t, key1[:], rootCall.StorageChanges[0].Key)
				assert.Equal(t, key2[:], rootCall.StorageChanges[1].Key)

				// Nested call should have 1 storage change
				nestedCall := trx.Calls[1]
				assert.Equal(t, 1, len(nestedCall.StorageChanges))
				assert.Equal(t, key10[:], nestedCall.StorageChanges[0].Key)
			})
	})

	t.Run("storage_change_full_32_bytes", func(t *testing.T) {
		// Test full 32-byte key and value handling
		var key, oldVal, newVal [32]byte
		// Fill with patterns to verify all bytes are preserved
		for i := 0; i < 32; i++ {
			key[i] = byte(i)
			oldVal[i] = byte(i * 2)
			newVal[i] = byte(i * 3)
		}

		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			StorageChange(BobAddr, key, oldVal, newVal).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 1, len(call.StorageChanges))
				sc := call.StorageChanges[0]
				assert.Equal(t, key[:], sc.Key)
				assert.Equal(t, oldVal[:], sc.OldValue)
				assert.Equal(t, newVal[:], sc.NewValue)
			})
	})

	t.Run("storage_change_zero_values", func(t *testing.T) {
		// Test storage change from/to zero values (common for initialization/deletion)
		var zero [32]byte
		key1 := hash32(1)
		key2 := hash32(2)
		val100 := hash32(100)
		val200 := hash32(200)
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			// Initialize: zero -> non-zero
			StorageChange(BobAddr, key1, zero, val100).
			// Delete: non-zero -> zero
			StorageChange(BobAddr, key2, val200, zero).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 2, len(call.StorageChanges))

				// Verify first change: initialization
				sc1 := call.StorageChanges[0]
				assert.Equal(t, zero[:], sc1.OldValue)
				assert.Equal(t, val100[:], sc1.NewValue)

				// Verify second change: deletion
				sc2 := call.StorageChanges[1]
				assert.Equal(t, val200[:], sc2.OldValue)
				assert.Equal(t, zero[:], sc2.NewValue)
			})
	})

	t.Run("storage_change_no_change_recorded", func(t *testing.T) {
		// Storage "changes" where oldValue == newValue ARE recorded (not filtered)
		// This differs from gas changes which filter no-change cases
		key := hash32(1)
		value := hash32(100) // Same for old and new

		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			StorageChange(BobAddr, key, value, value). // No actual change
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Native tracer records storage changes even when old==new
				// This is different behavior from gas changes which filter no-changes
				assert.Equal(t, 1, len(call.StorageChanges), "No-change storage updates should be recorded")
				sc := call.StorageChanges[0]
				assert.Equal(t, BobAddr[:], sc.Address)
				assert.Equal(t, key[:], sc.Key)
				assert.Equal(t, value[:], sc.OldValue)
				assert.Equal(t, value[:], sc.NewValue)
				assert.NotEqual(t, uint64(0), sc.Ordinal)
			})
	})
}
