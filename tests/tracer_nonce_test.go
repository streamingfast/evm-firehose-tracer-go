package tests

import (
	"testing"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
)

// TestTracer_OnNonceChange tests all nonce change scenarios
func TestTracer_OnNonceChange(t *testing.T) {
	t.Run("nonce_change_with_active_call", func(t *testing.T) {
		// Nonce change during active call execution
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			NonceChange(AliceAddr, 5, 6).
			EndCall([]byte{}, 21000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 1, len(call.NonceChanges))
				nc := call.NonceChanges[0]
				assert.Equal(t, AliceAddr[:], nc.Address)
				assert.Equal(t, uint64(5), nc.OldValue)
				assert.Equal(t, uint64(6), nc.NewValue)
			})
	})

	t.Run("nonce_change_deferred_state", func(t *testing.T) {
		// Nonce change before call stack initialization (deferred)
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			// Nonce change BEFORE call starts
			NonceChange(AliceAddr, 5, 6).
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Deferred nonce change should be applied to root call
				assert.Equal(t, 1, len(call.NonceChanges))
				nc := call.NonceChanges[0]
				assert.Equal(t, AliceAddr[:], nc.Address)
				assert.Equal(t, uint64(5), nc.OldValue)
				assert.Equal(t, uint64(6), nc.NewValue)
			})
	})

	t.Run("multiple_nonce_changes_in_call", func(t *testing.T) {
		// Multiple nonce changes in same call (unusual but possible)
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			NonceChange(AliceAddr, 5, 6).
			NonceChange(BobAddr, 10, 11).
			NonceChange(AliceAddr, 6, 7).
			EndCall([]byte{}, 21000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 3, len(call.NonceChanges), "Should have 3 nonce changes")

				// Verify ordering (ordinals should be increasing)
				assert.True(t, call.NonceChanges[0].Ordinal < call.NonceChanges[1].Ordinal)
				assert.True(t, call.NonceChanges[1].Ordinal < call.NonceChanges[2].Ordinal)

				// Verify values
				assert.Equal(t, AliceAddr[:], call.NonceChanges[0].Address)
				assert.Equal(t, uint64(5), call.NonceChanges[0].OldValue)
				assert.Equal(t, uint64(6), call.NonceChanges[0].NewValue)

				assert.Equal(t, BobAddr[:], call.NonceChanges[1].Address)
				assert.Equal(t, uint64(10), call.NonceChanges[1].OldValue)
				assert.Equal(t, uint64(11), call.NonceChanges[1].NewValue)

				assert.Equal(t, AliceAddr[:], call.NonceChanges[2].Address)
				assert.Equal(t, uint64(6), call.NonceChanges[2].OldValue)
				assert.Equal(t, uint64(7), call.NonceChanges[2].NewValue)
			})
	})

	t.Run("nonce_change_zero_to_one", func(t *testing.T) {
		// Test the common case of first nonce increment (0 -> 1)
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			NonceChange(AliceAddr, 0, 1).
			EndCall([]byte{}, 21000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 1, len(call.NonceChanges))
				nc := call.NonceChanges[0]
				assert.Equal(t, uint64(0), nc.OldValue)
				assert.Equal(t, uint64(1), nc.NewValue)
			})
	})
}
