package firehose

import (
	"testing"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTracer_DeferredCallState tests comprehensive deferred call state scenarios
// covering changes before root call, after root call, and mixed patterns
func TestTracer_DeferredCallState(t *testing.T) {
	// =============================================================================
	// Balance Changes - Deferred State
	// =============================================================================

	t.Run("balance_change_before_root_call", func(t *testing.T) {
		// Balance change occurs before root call starts (deferred to root call)
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			BalanceChange(AliceAddr, bigInt(1000), bigInt(790), pbeth.BalanceChange_REASON_GAS_BUY).
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				require.Equal(t, 1, len(call.BalanceChanges))
				bc := call.BalanceChanges[0]
				assert.Equal(t, AliceAddr[:], bc.Address)
				assert.Equal(t, pbeth.BalanceChange_REASON_GAS_BUY, bc.Reason)
			})
	})

	t.Run("balance_change_after_root_call", func(t *testing.T) {
		// Balance change occurs after root call ends (deferred to root call)
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			// Balance change AFTER call ends but before transaction ends
			BalanceChange(AliceAddr, bigInt(790), bigInt(800), pbeth.BalanceChange_REASON_GAS_REFUND).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				require.Equal(t, 1, len(call.BalanceChanges))
				bc := call.BalanceChanges[0]
				assert.Equal(t, AliceAddr[:], bc.Address)
				assert.Equal(t, pbeth.BalanceChange_REASON_GAS_REFUND, bc.Reason)
			})
	})

	// NOTE: Mixed tests (before+during+after) are more complex due to ordering expectations
	// and native validator interactions. The before/after tests above demonstrate that
	// deferred state works correctly in both scenarios.

	t.Run("balance_changes_mixed_before_during_after", func(t *testing.T) {
		// Balance changes: before call, during call, after call
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			// BEFORE: Gas buy
			BalanceChange(AliceAddr, bigInt(1000), bigInt(790), pbeth.BalanceChange_REASON_GAS_BUY).
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			// DURING: Transfer
			BalanceChange(AliceAddr, bigInt(790), bigInt(690), pbeth.BalanceChange_REASON_TRANSFER).
			BalanceChange(BobAddr, bigInt(500), bigInt(600), pbeth.BalanceChange_REASON_TRANSFER).
			EndCall([]byte{}, 21000, nil).
			// AFTER: Gas refund
			BalanceChange(AliceAddr, bigInt(690), bigInt(700), pbeth.BalanceChange_REASON_GAS_REFUND).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// All changes should be in root call, ordered chronologically
				require.Equal(t, 4, len(call.BalanceChanges))

				// Before call (deferred)
				assert.Equal(t, AliceAddr[:], call.BalanceChanges[0].Address)
				assert.Equal(t, pbeth.BalanceChange_REASON_GAS_BUY, call.BalanceChanges[0].Reason)

				// During call
				assert.Equal(t, AliceAddr[:], call.BalanceChanges[1].Address)
				assert.Equal(t, pbeth.BalanceChange_REASON_TRANSFER, call.BalanceChanges[1].Reason)

				assert.Equal(t, BobAddr[:], call.BalanceChanges[2].Address)
				assert.Equal(t, pbeth.BalanceChange_REASON_TRANSFER, call.BalanceChanges[2].Reason)

				// After call (deferred)
				assert.Equal(t, AliceAddr[:], call.BalanceChanges[3].Address)
				assert.Equal(t, pbeth.BalanceChange_REASON_GAS_REFUND, call.BalanceChanges[3].Reason)
			})
	})

	// =============================================================================
	// Nonce Changes - Deferred State
	// =============================================================================

	t.Run("nonce_change_before_root_call", func(t *testing.T) {
		// Nonce change before call starts (e.g., EIP-7702 SetCode)
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			NonceChange(AliceAddr, 0, 1).
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				require.Equal(t, 1, len(call.NonceChanges))
				nc := call.NonceChanges[0]
				assert.Equal(t, AliceAddr[:], nc.Address)
				assert.Equal(t, uint64(0), nc.OldValue)
				assert.Equal(t, uint64(1), nc.NewValue)
			})
	})

	t.Run("nonce_change_after_root_call", func(t *testing.T) {
		// Nonce change after call ends
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			NonceChange(AliceAddr, 5, 6).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				require.Equal(t, 1, len(call.NonceChanges))
				nc := call.NonceChanges[0]
				assert.Equal(t, AliceAddr[:], nc.Address)
				assert.Equal(t, uint64(5), nc.OldValue)
				assert.Equal(t, uint64(6), nc.NewValue)
			})
	})

	t.Run("nonce_changes_mixed_before_during_after", func(t *testing.T) {
		// Nonce changes: before, during, after call
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			// BEFORE: EIP-7702 SetCode nonce increment
			NonceChange(AliceAddr, 0, 1).
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			// DURING: Contract creation increments nonce
			NonceChange(BobAddr, 0, 1).
			EndCall([]byte{}, 21000, nil).
			// AFTER: Some post-execution nonce change
			NonceChange(CharlieAddr, 10, 11).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				require.Equal(t, 3, len(call.NonceChanges))

				// Before (deferred)
				assert.Equal(t, AliceAddr[:], call.NonceChanges[0].Address)
				assert.Equal(t, uint64(0), call.NonceChanges[0].OldValue)
				assert.Equal(t, uint64(1), call.NonceChanges[0].NewValue)

				// During
				assert.Equal(t, BobAddr[:], call.NonceChanges[1].Address)

				// After (deferred)
				assert.Equal(t, CharlieAddr[:], call.NonceChanges[2].Address)
			})
	})

	// =============================================================================
	// Code Changes - Deferred State
	// =============================================================================

	t.Run("code_change_before_root_call", func(t *testing.T) {
		// Code change before call starts (e.g., EIP-7702 SetCode)
		oldCode := []byte{0x60, 0x00}
		newCode := []byte{0x60, 0x01, 0x60, 0x02}
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			CodeChange(AliceAddr, hashBytes(oldCode), hashBytes(newCode), oldCode, newCode).
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				require.Equal(t, 1, len(call.CodeChanges))
				cc := call.CodeChanges[0]
				assert.Equal(t, AliceAddr[:], cc.Address)
				assert.Equal(t, oldCode, cc.OldCode)
				assert.Equal(t, newCode, cc.NewCode)
			})
	})

	t.Run("code_change_after_root_call", func(t *testing.T) {
		// Code change after call ends
		oldCode := []byte{}
		newCode := []byte{0x60, 0x03}
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			CodeChange(CharlieAddr, hashBytes(oldCode), hashBytes(newCode), oldCode, newCode).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				require.Equal(t, 1, len(call.CodeChanges))
				cc := call.CodeChanges[0]
				assert.Equal(t, CharlieAddr[:], cc.Address)
				assert.Empty(t, cc.OldCode, "Old code should be empty")
				assert.Equal(t, newCode, cc.NewCode)
			})
	})

	t.Run("code_changes_mixed_before_during_after", func(t *testing.T) {
		// Code changes: before, during, after call
		code1Old := []byte{0x60, 0x00}
		code1New := []byte{0x60, 0x01}
		code2Old := []byte{}
		code2New := []byte{0x60, 0x02}
		code3Old := []byte{0x60, 0x03}
		code3New := []byte{0x60, 0x04}

		NewTracerTester(t).
			StartBlockTrxNoHooks().
			// BEFORE: EIP-7702 SetCode
			CodeChange(AliceAddr, hashBytes(code1Old), hashBytes(code1New), code1Old, code1New).
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			// DURING: Contract deployment
			CodeChange(BobAddr, hashBytes(code2Old), hashBytes(code2New), code2Old, code2New).
			EndCall([]byte{}, 21000, nil).
			// AFTER: Another code change
			CodeChange(CharlieAddr, hashBytes(code3Old), hashBytes(code3New), code3Old, code3New).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				require.Equal(t, 3, len(call.CodeChanges))

				// Before (deferred)
				assert.Equal(t, AliceAddr[:], call.CodeChanges[0].Address)

				// During
				assert.Equal(t, BobAddr[:], call.CodeChanges[1].Address)

				// After (deferred)
				assert.Equal(t, CharlieAddr[:], call.CodeChanges[2].Address)
			})
	})

	// =============================================================================
	// Gas Changes - Deferred State
	// =============================================================================

	t.Run("gas_change_before_root_call", func(t *testing.T) {
		// Gas change before call starts (intrinsic gas)
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			GasChange(21000, 0, pbeth.GasChange_REASON_INTRINSIC_GAS).
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				require.Equal(t, 1, len(call.GasChanges))
				gc := call.GasChanges[0]
				assert.Equal(t, uint64(21000), gc.OldValue)
				assert.Equal(t, uint64(0), gc.NewValue)
				assert.Equal(t, pbeth.GasChange_REASON_INTRINSIC_GAS, gc.Reason)
			})
	})

	t.Run("gas_change_after_root_call", func(t *testing.T) {
		// Gas change after call ends (gas refund)
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			GasChange(0, 5000, pbeth.GasChange_REASON_REFUND_AFTER_EXECUTION).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				require.Equal(t, 1, len(call.GasChanges))
				gc := call.GasChanges[0]
				assert.Equal(t, uint64(0), gc.OldValue)
				assert.Equal(t, uint64(5000), gc.NewValue)
				assert.Equal(t, pbeth.GasChange_REASON_REFUND_AFTER_EXECUTION, gc.Reason)
			})
	})

	t.Run("gas_changes_mixed_before_during_after", func(t *testing.T) {
		// Gas changes: before, during, after call
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			// BEFORE: Intrinsic gas
			GasChange(50000, 29000, pbeth.GasChange_REASON_INTRINSIC_GAS).
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 50000, []byte{}).
			// DURING: Call execution (using STATE_COLD_ACCESS as example of gas consumed during call)
			GasChange(29000, 8000, pbeth.GasChange_REASON_STATE_COLD_ACCESS).
			EndCall([]byte{}, 8000, nil).
			// AFTER: Gas refund
			GasChange(8000, 13000, pbeth.GasChange_REASON_REFUND_AFTER_EXECUTION).
			EndBlockTrx(successReceipt(37000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				require.Equal(t, 3, len(call.GasChanges))

				// Before (deferred)
				assert.Equal(t, pbeth.GasChange_REASON_INTRINSIC_GAS, call.GasChanges[0].Reason)

				// During
				assert.Equal(t, pbeth.GasChange_REASON_STATE_COLD_ACCESS, call.GasChanges[1].Reason)

				// After (deferred)
				assert.Equal(t, pbeth.GasChange_REASON_REFUND_AFTER_EXECUTION, call.GasChanges[2].Reason)
			})
	})

	// =============================================================================
	// Mixed State Changes - Complex Scenarios
	// =============================================================================

	t.Run("all_state_types_before_root_call", func(t *testing.T) {
		// All types of state changes before root call
		oldCode := []byte{}
		newCode := []byte{0x60, 0x00}

		NewTracerTester(t).
			StartBlockTrxNoHooks().
			BalanceChange(AliceAddr, bigInt(1000), bigInt(790), pbeth.BalanceChange_REASON_GAS_BUY).
			NonceChange(AliceAddr, 0, 1).
			CodeChange(AliceAddr, hashBytes(oldCode), hashBytes(newCode), oldCode, newCode).
			GasChange(50000, 29000, pbeth.GasChange_REASON_INTRINSIC_GAS).
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 29000, []byte{}).
			EndCall([]byte{}, 29000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// All deferred state should be in root call
				assert.Equal(t, 1, len(call.BalanceChanges))
				assert.Equal(t, 1, len(call.NonceChanges))
				assert.Equal(t, 1, len(call.CodeChanges))
				assert.Equal(t, 1, len(call.GasChanges))
			})
	})

	t.Run("all_state_types_after_root_call", func(t *testing.T) {
		// All types of state changes after root call
		oldCode := []byte{0x60, 0x01}
		newCode := []byte{0x60, 0x02}

		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			BalanceChange(AliceAddr, bigInt(790), bigInt(800), pbeth.BalanceChange_REASON_GAS_REFUND).
			NonceChange(CharlieAddr, 5, 6).
			CodeChange(CharlieAddr, hashBytes(oldCode), hashBytes(newCode), oldCode, newCode).
			GasChange(0, 5000, pbeth.GasChange_REASON_REFUND_AFTER_EXECUTION).
			EndBlockTrx(successReceipt(16000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// All deferred state should be in root call
				assert.Equal(t, 1, len(call.BalanceChanges))
				assert.Equal(t, 1, len(call.NonceChanges))
				assert.Equal(t, 1, len(call.CodeChanges))
				assert.Equal(t, 1, len(call.GasChanges))
			})
	})

	t.Run("all_state_types_mixed_before_during_after", func(t *testing.T) {
		// Complex scenario: all state types before, during, and after root call
		code1Old := []byte{}
		code1New := []byte{0x60, 0x00}
		code2Old := []byte{}
		code2New := []byte{0x60, 0x01}
		code3Old := []byte{0x60, 0x02}
		code3New := []byte{0x60, 0x03}

		NewTracerTester(t).
			StartBlockTrxNoHooks().
			// === BEFORE ROOT CALL ===
			BalanceChange(AliceAddr, bigInt(1000), bigInt(790), pbeth.BalanceChange_REASON_GAS_BUY).
			NonceChange(AliceAddr, 0, 1).
			CodeChange(AliceAddr, hashBytes(code1Old), hashBytes(code1New), code1Old, code1New).
			GasChange(50000, 29000, pbeth.GasChange_REASON_INTRINSIC_GAS).
			// === DURING ROOT CALL ===
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 29000, []byte{}).
			BalanceChange(AliceAddr, bigInt(790), bigInt(690), pbeth.BalanceChange_REASON_TRANSFER).
			BalanceChange(BobAddr, bigInt(500), bigInt(600), pbeth.BalanceChange_REASON_TRANSFER).
			NonceChange(BobAddr, 0, 1).
			CodeChange(BobAddr, hashBytes(code2Old), hashBytes(code2New), code2Old, code2New).
			GasChange(29000, 8000, pbeth.GasChange_REASON_STATE_COLD_ACCESS).
			EndCall([]byte{}, 8000, nil).
			// === AFTER ROOT CALL ===
			BalanceChange(AliceAddr, bigInt(690), bigInt(700), pbeth.BalanceChange_REASON_GAS_REFUND).
			NonceChange(CharlieAddr, 10, 11).
			CodeChange(CharlieAddr, hashBytes(code3Old), hashBytes(code3New), code3Old, code3New).
			GasChange(8000, 13000, pbeth.GasChange_REASON_REFUND_AFTER_EXECUTION).
			EndBlockTrx(successReceipt(37000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// All state changes should be in root call, properly ordered
				require.Equal(t, 4, len(call.BalanceChanges), "Should have 4 balance changes")
				require.Equal(t, 3, len(call.NonceChanges), "Should have 3 nonce changes")
				require.Equal(t, 3, len(call.CodeChanges), "Should have 3 code changes")
				require.Equal(t, 3, len(call.GasChanges), "Should have 3 gas changes")

				// Verify ordering: before (deferred) -> during -> after (deferred)
				// Balance changes
				assert.Equal(t, pbeth.BalanceChange_REASON_GAS_BUY, call.BalanceChanges[0].Reason, "First balance change should be gas buy (before)")
				assert.Equal(t, pbeth.BalanceChange_REASON_TRANSFER, call.BalanceChanges[1].Reason, "Second balance change should be transfer (during)")
				assert.Equal(t, pbeth.BalanceChange_REASON_TRANSFER, call.BalanceChanges[2].Reason, "Third balance change should be transfer (during)")
				assert.Equal(t, pbeth.BalanceChange_REASON_GAS_REFUND, call.BalanceChanges[3].Reason, "Fourth balance change should be gas refund (after)")

				// Nonce changes
				assert.Equal(t, AliceAddr[:], call.NonceChanges[0].Address, "First nonce change should be Alice (before)")
				assert.Equal(t, BobAddr[:], call.NonceChanges[1].Address, "Second nonce change should be Bob (during)")
				assert.Equal(t, CharlieAddr[:], call.NonceChanges[2].Address, "Third nonce change should be Charlie (after)")

				// Code changes
				assert.Equal(t, AliceAddr[:], call.CodeChanges[0].Address, "First code change should be Alice (before)")
				assert.Equal(t, BobAddr[:], call.CodeChanges[1].Address, "Second code change should be Bob (during)")
				assert.Equal(t, CharlieAddr[:], call.CodeChanges[2].Address, "Third code change should be Charlie (after)")

				// Gas changes
				assert.Equal(t, pbeth.GasChange_REASON_INTRINSIC_GAS, call.GasChanges[0].Reason, "First gas change should be intrinsic (before)")
				assert.Equal(t, pbeth.GasChange_REASON_STATE_COLD_ACCESS, call.GasChanges[1].Reason, "Second gas change should be call (during)")
				assert.Equal(t, pbeth.GasChange_REASON_REFUND_AFTER_EXECUTION, call.GasChanges[2].Reason, "Third gas change should be refund (after)")
			})
	})
}
