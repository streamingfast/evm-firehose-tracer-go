package tests

import (
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
)

// TestTracer_OnGasChange tests all gas change scenarios
func TestTracer_OnGasChange(t *testing.T) {
	// NOTE: We cannot test gas_change_unknown_reason_ignored because the native tracer
	// panics when it receives REASON_UNKNOWN (see gasChangeReasonFromChain in firehose.go:2318).
	// The shared tracer correctly filters it out at line 1167 in tracer.go, but we can't
	// validate against the native tracer since it can't handle UNKNOWN at all.
	// The filtering logic is tested by ensuring other tests don't include UNKNOWN gas changes.

	t.Run("gas_change_no_change_ignored", func(t *testing.T) {
		// Gas changes where old == new should be ignored
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			GasChange(100000, 100000, pbeth.GasChange_REASON_INTRINSIC_GAS).
			EndCall([]byte{}, 100000, nil).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// No-change gas changes should be filtered out
				assert.Equal(t, 0, len(call.GasChanges), "No-change gas changes should be ignored")
			})
	})

	t.Run("gas_change_with_active_call", func(t *testing.T) {
		// Normal gas change during active call
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			GasChange(100000, 90000, pbeth.GasChange_REASON_INTRINSIC_GAS).
			EndCall([]byte{}, 90000, nil).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 1, len(call.GasChanges))
				gc := call.GasChanges[0]
				assert.Equal(t, uint64(100000), gc.OldValue)
				assert.Equal(t, uint64(90000), gc.NewValue)
				assert.Equal(t, pbeth.GasChange_REASON_INTRINSIC_GAS, gc.Reason)
				assert.NotEqual(t, uint64(0), gc.Ordinal)
			})
	})

	t.Run("gas_change_deferred_state", func(t *testing.T) {
		// Gas change before call stack initialization (deferred state)
		// This happens for initial gas balance
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			GasChange(0, 21000, pbeth.GasChange_REASON_TX_INITIAL_BALANCE).
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Deferred gas change should be applied to root call
				assert.Equal(t, 1, len(call.GasChanges))
				gc := call.GasChanges[0]
				assert.Equal(t, uint64(0), gc.OldValue)
				assert.Equal(t, uint64(21000), gc.NewValue)
				assert.Equal(t, pbeth.GasChange_REASON_TX_INITIAL_BALANCE, gc.Reason)
			})
	})

	t.Run("multiple_gas_changes_in_transaction", func(t *testing.T) {
		// Multiple gas changes in same transaction
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			GasChange(100000, 90000, pbeth.GasChange_REASON_INTRINSIC_GAS).
			GasChange(90000, 80000, pbeth.GasChange_REASON_CODE_STORAGE).
			GasChange(80000, 70000, pbeth.GasChange_REASON_STATE_COLD_ACCESS).
			EndCall([]byte{}, 70000, nil).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 3, len(call.GasChanges), "Should have 3 gas changes")

				// Verify ordering (ordinals should be increasing)
				assert.True(t, call.GasChanges[0].Ordinal < call.GasChanges[1].Ordinal)
				assert.True(t, call.GasChanges[1].Ordinal < call.GasChanges[2].Ordinal)

				// Verify values
				assert.Equal(t, uint64(100000), call.GasChanges[0].OldValue)
				assert.Equal(t, uint64(90000), call.GasChanges[0].NewValue)
				assert.Equal(t, pbeth.GasChange_REASON_INTRINSIC_GAS, call.GasChanges[0].Reason)

				assert.Equal(t, uint64(90000), call.GasChanges[1].OldValue)
				assert.Equal(t, uint64(80000), call.GasChanges[1].NewValue)
				assert.Equal(t, pbeth.GasChange_REASON_CODE_STORAGE, call.GasChanges[1].Reason)

				assert.Equal(t, uint64(80000), call.GasChanges[2].OldValue)
				assert.Equal(t, uint64(70000), call.GasChanges[2].NewValue)
				assert.Equal(t, pbeth.GasChange_REASON_STATE_COLD_ACCESS, call.GasChanges[2].Reason)
			})
	})

	t.Run("gas_changes_across_multiple_calls", func(t *testing.T) {
		// Gas changes in nested calls
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 200000, []byte{}).
			GasChange(200000, 180000, pbeth.GasChange_REASON_CONTRACT_CREATION).
			StartCallRaw(1, byte(firehose.CallTypeCall), BobAddr, CharlieAddr, []byte{}, 100000, bigInt(0)).
			GasChange(100000, 90000, pbeth.GasChange_REASON_PRECOMPILED_CONTRACT).
			EndCall([]byte{}, 90000, nil).
			EndCall([]byte{}, 180000, nil).
			EndBlockTrx(successReceipt(200000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]

				// Root call has 1 gas change
				assert.Equal(t, 1, len(trx.Calls[0].GasChanges))
				assert.Equal(t, pbeth.GasChange_REASON_CONTRACT_CREATION, trx.Calls[0].GasChanges[0].Reason)

				// Nested call has 1 gas change
				assert.Equal(t, 1, len(trx.Calls[1].GasChanges))
				assert.Equal(t, pbeth.GasChange_REASON_PRECOMPILED_CONTRACT, trx.Calls[1].GasChanges[0].Reason)
			})
	})
}

// TestTracer_GasChangeReasons tests all gas change reasons supported by native tracer
func TestTracer_GasChangeReasons(t *testing.T) {
	// Define the 13 gas change reasons supported by convertToNativeGasChangeReason
	// (see tracer_native_validator.go:569-600)
	// The native tracer only supports a subset of all gas change reasons.
	// Unsupported reasons would return tracing.GasChangeUnspecified (0) which causes
	// the native tracer to panic with "unknown tracer gas change reason value '0'".
	reasons := []pbeth.GasChange_Reason{
		pbeth.GasChange_REASON_TX_INITIAL_BALANCE,
		pbeth.GasChange_REASON_TX_REFUNDS,
		pbeth.GasChange_REASON_TX_LEFT_OVER_RETURNED,
		pbeth.GasChange_REASON_CALL_INITIAL_BALANCE,
		pbeth.GasChange_REASON_CALL_LEFT_OVER_RETURNED,
		pbeth.GasChange_REASON_INTRINSIC_GAS,
		pbeth.GasChange_REASON_CONTRACT_CREATION,
		pbeth.GasChange_REASON_CONTRACT_CREATION2,
		pbeth.GasChange_REASON_CODE_STORAGE,
		pbeth.GasChange_REASON_PRECOMPILED_CONTRACT,
		pbeth.GasChange_REASON_STATE_COLD_ACCESS,
		pbeth.GasChange_REASON_REFUND_AFTER_EXECUTION,
		pbeth.GasChange_REASON_FAILED_EXECUTION,
	}

	// Unsupported reasons (would cause native tracer panic):
	// REASON_CALL, REASON_CALL_CODE, REASON_CALL_DATA_COPY, REASON_CODE_COPY,
	// REASON_DELEGATE_CALL, REASON_EVENT_LOG, REASON_EXT_CODE_COPY, REASON_RETURN,
	// REASON_RETURN_DATA_COPY, REASON_REVERT, REASON_SELF_DESTRUCT, REASON_STATIC_CALL,
	// REASON_WITNESS_CONTRACT_INIT, REASON_WITNESS_CONTRACT_CREATION,
	// REASON_WITNESS_CODE_CHUNK, REASON_WITNESS_CONTRACT_COLLISION_CHECK,
	// REASON_TX_DATA_FLOOR

	for _, reason := range reasons {
		t.Run(reason.String(), func(t *testing.T) {
			NewTracerTester(t).
				StartBlockTrxNoHooks().
				StartRootCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
				GasChange(100000, 90000, reason).
				EndCall([]byte{}, 90000, nil).
				EndBlockTrx(successReceipt(100000), nil, nil).
				Validate(func(block *pbeth.Block) {
					trx := block.TransactionTraces[0]
					call := trx.Calls[0]

					assert.Equal(t, 1, len(call.GasChanges), "Should have 1 gas change")
					gc := call.GasChanges[0]
					assert.Equal(t, uint64(100000), gc.OldValue)
					assert.Equal(t, uint64(90000), gc.NewValue)
					assert.Equal(t, reason, gc.Reason)
					assert.NotEqual(t, uint64(0), gc.Ordinal)
				})
		})
	}
}
