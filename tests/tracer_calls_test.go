package tests

import (
	"errors"
	"fmt"
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go/v5"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
)

// TestTracer_CallTypes tests all different call types
func TestTracer_CallTypes(t *testing.T) {
	t.Run("CALL", func(t *testing.T) {
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 1, len(block.TransactionTraces))
				trx := block.TransactionTraces[0]
				assert.Equal(t, 1, len(trx.Calls))

				call := trx.Calls[0]
				assert.Equal(t, pbeth.CallType_CALL, call.CallType)
				assert.Equal(t, AliceAddr[:], call.Caller)
				assert.Equal(t, BobAddr[:], call.Address)
				assert.Equal(t, uint64(100), call.Value.Uint64())
				assert.Equal(t, uint64(21000), call.GasLimit)
				assert.Equal(t, uint64(21000), call.GasConsumed)
				assert.False(t, call.StatusFailed)
				assert.False(t, call.StatusReverted)
			})
	})

	t.Run("STATICCALL", func(t *testing.T) {
		// STATICCALL can only happen as a nested call from a contract
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 50000, []byte{}).
			// Bob makes a STATICCALL to Charlie
			StartStaticCall(BobAddr, CharlieAddr, 21000, []byte{0x01, 0x02}).
			EndCall([]byte{0x03, 0x04}, 20000).
			EndCall([]byte{}, 45000).
			EndBlockTrx(successReceipt(50000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				assert.Equal(t, 2, len(trx.Calls), "Should have root call + nested STATICCALL")

				nestedCall := trx.Calls[1]
				assert.Equal(t, pbeth.CallType_STATIC, nestedCall.CallType)
				assert.Nil(t, nestedCall.Value, "STATICCALL should have nil value")
				assertEqualBytes(t, []byte{0x01, 0x02}, nestedCall.Input)
				assertEqualBytes(t, []byte{0x03, 0x04}, nestedCall.ReturnData)
			})
	})

	t.Run("DELEGATECALL", func(t *testing.T) {
		// DELEGATECALL can only happen as a nested call from a contract
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 50000, []byte{}).
			// Bob makes a DELEGATECALL to Charlie
			StartDelegateCall(BobAddr, CharlieAddr, bigInt(0), 21000, []byte{0x05}).
			EndCall([]byte{0x06}, 20000).
			EndCall([]byte{}, 45000).
			EndBlockTrx(successReceipt(50000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				assert.Equal(t, 2, len(trx.Calls), "Should have root call + nested DELEGATECALL")

				nestedCall := trx.Calls[1]
				assert.Equal(t, pbeth.CallType_DELEGATE, nestedCall.CallType)
				assert.Nil(t, nestedCall.Value, "DELEGATECALL should have nil value")
			})
	})

	t.Run("CALLCODE", func(t *testing.T) {
		// CALLCODE can only happen as a nested call from a contract
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 50000, []byte{}).
			// Bob makes a CALLCODE to Charlie
			StartCallCode(BobAddr, CharlieAddr, bigInt(50), 21000, []byte{}).
			EndCall([]byte{}, 20000).
			EndCall([]byte{}, 45000).
			EndBlockTrx(successReceipt(50000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				assert.Equal(t, 2, len(trx.Calls), "Should have root call + nested CALLCODE")

				nestedCall := trx.Calls[1]
				assert.Equal(t, pbeth.CallType_CALLCODE, nestedCall.CallType)
				assert.Equal(t, uint64(50), nestedCall.Value.Uint64())
			})
	})

	t.Run("CREATE", func(t *testing.T) {
		contractCode := []byte{0x60, 0x80, 0x60, 0x40}
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCreateCall(AliceAddr, BobAddr, bigInt(0), 53000, contractCode).
			EndCall(contractCode, 50000).
			EndBlockTrx(successReceipt(53000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, pbeth.CallType_CREATE, call.CallType)
				assertEqualBytes(t, contractCode, call.Input)
				// CREATE calls don't return the code in ReturnData
				assert.Empty(t, call.ReturnData, "CREATE call should have empty return data")
			})
	})

	t.Run("CREATE2", func(t *testing.T) {
		// CREATE2 can only happen as a nested call from a contract
		contractCode := []byte{0x60, 0x80, 0x60, 0x40}
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			// Bob deploys a contract using CREATE2
			StartCreate2Call(BobAddr, CharlieAddr, bigInt(0), 53000, contractCode).
			EndCall(contractCode, 50000).
			EndCall([]byte{}, 95000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				assert.Equal(t, 2, len(trx.Calls), "Should have root call + nested CREATE2")

				nestedCall := trx.Calls[1]
				assert.Equal(t, pbeth.CallType_CREATE, nestedCall.CallType)
				// CREATE2 uses same firehose.CallType as CREATE
				assert.Empty(t, nestedCall.ReturnData, "CREATE2 call should have empty return data")
			})
	})
}

// TestTracer_CallFailures tests different failure scenarios
func TestTracer_CallFailures(t *testing.T) {
	t.Run("reverted_call", func(t *testing.T) {
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCallFailed([]byte{}, 5000, fmt.Errorf("revert: %w", testErrExecutionReverted), true).
			EndBlockTrx(failedReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.True(t, call.StatusFailed, "Call should be marked as failed")
				assert.True(t, call.StatusReverted, "Call should be marked as reverted")
				assert.Equal(t, "revert: execution reverted", call.FailureReason)
				assert.Equal(t, uint64(5000), call.GasConsumed)
			})
	})

	t.Run("failed_call_out_of_gas", func(t *testing.T) {
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCallFailed([]byte{}, 21000, errors.New("out of gas"), true).
			EndBlockTrx(failedReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.True(t, call.StatusFailed)
				assert.False(t, call.StatusReverted, "Out of gas is failed but not reverted")
				assert.Equal(t, "out of gas", call.FailureReason)
				assert.Equal(t, uint64(21000), call.GasConsumed, "Should consume all gas")
			})
	})

	t.Run("failed_call_invalid_opcode", func(t *testing.T) {
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{0xff}).
			EndCallFailed([]byte{}, 10000, errors.New("invalid opcode"), true).
			EndBlockTrx(failedReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.True(t, call.StatusFailed)
				assert.Equal(t, "invalid opcode", call.FailureReason)
			})
	})

	t.Run("pre_homestead_code_store_out_of_gas", func(t *testing.T) {
		// Pre-Homestead quirk: ErrCodeStoreOutOfGas with reverted=false
		// This matches go-ethereum's behavior in core/vm/evm.go:
		//   if !evm.chainRules.IsHomestead && errors.Is(err, ErrCodeStoreOutOfGas) {
		//       reverted = false
		//   }
		// In Frontier (pre-Homestead), code storage running out of gas was not treated
		// as a state revert - the call returns an error but reverted=false means no state rollback.
		// Reference: https://github.com/ethereum/go-ethereum/blob/master/core/vm/evm.go
		//
		// Since reverted=false, the Firehose model treats this as a successful call:
		// - StatusFailed: false (no state revert)
		// - StatusReverted: false (no revert)
		// - StateReverted: false (state is kept)
		// - FailureReason: empty (no failure from Firehose perspective)
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCreateCall(AliceAddr, BobAddr, bigInt(0), 53000, []byte{0x60, 0x80}).
			// Contract deployment fails at EVM level but reverted=false means state is kept
			EndCallFailed([]byte{}, 0, testErrCodeStoreOutOfGas, false).
			EndBlockTrx(successReceipt(53000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// In Firehose model, reverted=false means successful state transition
				assert.False(t, call.StatusFailed, "reverted=false means no state failure")
				assert.False(t, call.StatusReverted, "reverted=false means no revert")
				assert.False(t, call.StateReverted, "State is NOT reverted in pre-Homestead")
				assert.Equal(t, "", call.FailureReason, "No failure reason when reverted=false")

				// Gas consumption
				assert.Equal(t, uint64(0), call.GasConsumed, "Code store OOG consumes no gas")
			})
	})
}

// TestTracer_NestedCalls tests nested call scenarios
func TestTracer_NestedCalls(t *testing.T) {
	t.Run("simple_nested_call", func(t *testing.T) {
		// Alice calls Bob, Bob calls Charlie
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{0x01}).
			// Bob makes a nested call to Charlie
			StartCall(BobAddr, CharlieAddr, bigInt(50), 50000, []byte{0x02}).
			EndCall([]byte{0x03}, 45000). // Charlie returns
			EndCall([]byte{0x04}, 90000). // Bob returns
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				assert.Equal(t, 2, len(trx.Calls), "Should have 2 calls (root + nested)")

				rootCall := trx.Calls[0]
				assert.Equal(t, pbeth.CallType_CALL, rootCall.CallType)
				assert.Equal(t, AliceAddr[:], rootCall.Caller)
				assert.Equal(t, BobAddr[:], rootCall.Address)
				assert.Equal(t, uint32(0), rootCall.Depth)
				assert.Equal(t, uint32(0), rootCall.ParentIndex, "Root call has no parent")

				nestedCall := trx.Calls[1]
				assert.Equal(t, pbeth.CallType_CALL, nestedCall.CallType)
				assert.Equal(t, BobAddr[:], nestedCall.Caller)
				assert.Equal(t, CharlieAddr[:], nestedCall.Address)
				assert.Equal(t, uint32(1), nestedCall.Depth)
				assert.Equal(t, uint32(1), nestedCall.ParentIndex, "Nested call parent is root")
			})
	})

	t.Run("deep_nested_calls", func(t *testing.T) {
		// Alice -> Bob -> Charlie -> Miner (depth 0, 1, 2, 3)
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 200000, []byte{}).
			StartCall(BobAddr, CharlieAddr, bigInt(50), 150000, []byte{}).
			StartCall(CharlieAddr, MinerAddr, bigInt(25), 100000, []byte{}).
			EndCall([]byte{}, 95000).  // Miner returns
			EndCall([]byte{}, 140000). // Charlie returns
			EndCall([]byte{}, 180000). // Bob returns
			EndBlockTrx(successReceipt(200000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				assert.Equal(t, 3, len(trx.Calls), "Should have 3 nested calls")

				// Verify depth progression
				for i, expectedDepth := range []uint32{0, 1, 2} {
					assert.Equal(t, expectedDepth, trx.Calls[i].Depth)
				}

				// Verify parent relationships
				assert.Equal(t, uint32(0), trx.Calls[0].ParentIndex) // Root has no parent
				assert.Equal(t, uint32(1), trx.Calls[1].ParentIndex) // Child of call 1 (index=1)
				assert.Equal(t, uint32(2), trx.Calls[2].ParentIndex) // Child of call 2 (index=2)
			})
	})

	t.Run("nested_with_failure", func(t *testing.T) {
		// Alice calls Bob, Bob calls Charlie (fails), Bob continues and succeeds
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			// Bob calls Charlie, which reverts
			StartCall(BobAddr, CharlieAddr, bigInt(50), 50000, []byte{}).
			EndCallFailed([]byte{}, 10000, fmt.Errorf("revert: %w", testErrExecutionReverted), true).
			// Bob continues and succeeds
			EndCall([]byte{0x05}, 80000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				assert.Equal(t, 2, len(trx.Calls))

				rootCall := trx.Calls[0]
				assert.False(t, rootCall.StatusFailed, "Root call should succeed")
				assert.False(t, rootCall.StatusReverted)

				nestedCall := trx.Calls[1]
				assert.True(t, nestedCall.StatusFailed, "Nested call should fail")
				assert.True(t, nestedCall.StatusReverted)
				assert.Equal(t, "revert: execution reverted", nestedCall.FailureReason)
			})
	})

	t.Run("nested_all_revert", func(t *testing.T) {
		// Alice calls Bob, Bob calls Charlie (fails), Bob also reverts
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			// Bob calls Charlie, which reverts
			StartCall(BobAddr, CharlieAddr, bigInt(50), 50000, []byte{}).
			EndCallFailed([]byte{}, 10000, fmt.Errorf("nested revert: %w", testErrExecutionReverted), true).
			// Bob also reverts
			EndCallFailed([]byte{}, 20000, fmt.Errorf("parent revert: %w", testErrExecutionReverted), true).
			EndBlockTrx(failedReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				assert.Equal(t, 2, len(trx.Calls))

				// Both calls should be reverted
				for i, call := range trx.Calls {
					assert.True(t, call.StatusFailed, "Call %d should be failed", i)
					assert.True(t, call.StatusReverted, "Call %d should be reverted", i)
				}

				// Transaction should also be reverted
				assert.Equal(t, pbeth.TransactionTraceStatus_REVERTED, trx.Status)
			})
	})

	t.Run("multiple_siblings", func(t *testing.T) {
		// Alice calls Bob, Bob calls Charlie and Miner (sibling calls)
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 200000, []byte{}).
			// First sibling: Bob calls Charlie
			StartCall(BobAddr, CharlieAddr, bigInt(30), 80000, []byte{}).
			EndCall([]byte{0x01}, 75000).
			// Second sibling: Bob calls Miner
			StartCall(BobAddr, MinerAddr, bigInt(20), 80000, []byte{}).
			EndCall([]byte{0x02}, 75000).
			EndCall([]byte{0x03}, 150000). // Bob returns
			EndBlockTrx(successReceipt(200000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				assert.Equal(t, 3, len(trx.Calls), "Should have 3 calls (root + 2 siblings)")

				firstSibling := trx.Calls[1]
				secondSibling := trx.Calls[2]

				// Both siblings should have same parent (root=1) and same depth
				assert.Equal(t, uint32(1), firstSibling.ParentIndex)
				assert.Equal(t, uint32(1), secondSibling.ParentIndex)
				assert.Equal(t, uint32(1), firstSibling.Depth)
				assert.Equal(t, uint32(1), secondSibling.Depth)

				// Verify addresses
				assert.Equal(t, CharlieAddr[:], firstSibling.Address)
				assert.Equal(t, MinerAddr[:], secondSibling.Address)
			})
	})
}

// TestTracer_CallDataAndGas tests gas and data handling
func TestTracer_CallDataAndGas(t *testing.T) {
	t.Run("call_with_large_input", func(t *testing.T) {
		largeInput := make([]byte, 1024)
		for i := range largeInput {
			largeInput[i] = byte(i % 256)
		}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 50000, largeInput).
			EndCall([]byte{0x01}, 45000).
			EndBlockTrx(successReceipt(50000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assertEqualBytes(t, largeInput, call.Input)
				assert.Equal(t, uint64(50000), call.GasLimit)
				assert.Equal(t, uint64(45000), call.GasConsumed)
			})
	})

	t.Run("call_with_large_output", func(t *testing.T) {
		largeOutput := make([]byte, 2048)
		for i := range largeOutput {
			largeOutput[i] = byte((i * 3) % 256)
		}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			EndCall(largeOutput, 95000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assertEqualBytes(t, largeOutput, call.ReturnData)
			})
	})

	t.Run("call_with_zero_value", func(t *testing.T) {
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 21000, []byte{}).
			EndCall([]byte{}, 20000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Zero value is represented as nil in protobuf (both tracers behave this way)
				assert.Nil(t, call.Value)
			})
	})

	t.Run("call_gas_fully_consumed", func(t *testing.T) {
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000). // All gas consumed
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, call.GasLimit, call.GasConsumed, "All gas should be consumed")
			})
	})
}

// TestTracer_MixedCallTypes tests combinations of different call types
func TestTracer_MixedCallTypes(t *testing.T) {
	t.Run("call_then_staticcall", func(t *testing.T) {
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			StartStaticCall(BobAddr, CharlieAddr, 50000, []byte{}).
			EndCall([]byte{0x01}, 45000).
			EndCall([]byte{0x02}, 90000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				assert.Equal(t, 2, len(trx.Calls))

				assert.Equal(t, pbeth.CallType_CALL, trx.Calls[0].CallType)
				assert.Equal(t, pbeth.CallType_STATIC, trx.Calls[1].CallType)
			})
	})

	t.Run("call_then_delegatecall", func(t *testing.T) {
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			StartDelegateCall(BobAddr, CharlieAddr, bigInt(0), 50000, []byte{}).
			EndCall([]byte{0x01}, 45000).
			EndCall([]byte{0x02}, 90000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				assert.Equal(t, 2, len(trx.Calls))

				assert.Equal(t, pbeth.CallType_CALL, trx.Calls[0].CallType)
				assert.Equal(t, pbeth.CallType_DELEGATE, trx.Calls[1].CallType)
			})
	})

	t.Run("create_then_call", func(t *testing.T) {
		contractCode := []byte{0x60, 0x80}
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCreateCall(AliceAddr, BobAddr, bigInt(0), 100000, contractCode).
			// New contract makes a call to Charlie
			StartCall(BobAddr, CharlieAddr, bigInt(0), 50000, []byte{}).
			EndCall([]byte{0x01}, 45000).
			EndCall(contractCode, 90000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				assert.Equal(t, 2, len(trx.Calls))

				assert.Equal(t, pbeth.CallType_CREATE, trx.Calls[0].CallType)
				assert.Equal(t, pbeth.CallType_CALL, trx.Calls[1].CallType)
			})
	})
}

// TestTracer_WrappedErrors tests that errorIsString correctly walks error chains
func TestTracer_WrappedErrors(t *testing.T) {
	t.Run("wrapped_execution_reverted", func(t *testing.T) {
		// Create a wrapped error chain: "custom context" -> vm.ErrExecutionReverted
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCallFailed([]byte{}, 5000, fmt.Errorf("custom context: %w", testErrExecutionReverted), true).
			EndBlockTrx(failedReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.True(t, call.StatusFailed, "Call should be marked as failed")
				assert.True(t, call.StatusReverted, "Call should be marked as reverted even with wrapped error")
				assert.Equal(t, "custom context: execution reverted", call.FailureReason)
			})
	})

	t.Run("wrapped_insufficient_balance", func(t *testing.T) {
		// Test wrapped insufficient balance error
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCallFailed([]byte{}, 0, fmt.Errorf("transfer failed: %w", testErrInsufficientBalanceTransfer), true).
			EndBlockTrx(failedReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.True(t, call.StatusFailed)
				assert.True(t, call.StatusReverted, "Insufficient balance should be reverted")
				assert.Equal(t, "transfer failed: insufficient balance for transfer", call.FailureReason)
			})
	})

	t.Run("double_wrapped_error", func(t *testing.T) {
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCallFailed([]byte{}, 0, fmt.Errorf("context layer 2: %w", fmt.Errorf("context layer 1: %w", testErrMaxCallDepth)), true).
			EndBlockTrx(failedReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.True(t, call.StatusFailed)
				assert.True(t, call.StatusReverted, "Should find revert error through multiple wrapping layers")
				assert.Equal(t, "context layer 2: context layer 1: max call depth exceeded", call.FailureReason)
			})
	})
}

// TestTracer_Precompiles tests calls to precompiled contracts
func TestTracer_Precompiles(t *testing.T) {
	// Precompile addresses (from go-ethereum params/protocol_params.go)
	ecrecoverAddr := [20]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}
	sha256Addr := [20]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02}
	ripemd160Addr := [20]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03}
	bn256AddAddr := [20]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x06}
	bn256ScalarMulAddr := [20]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x07}

	t.Run("ecrecover_precompile_success", func(t *testing.T) {
		// Valid ecrecover input data
		input := make([]byte, 128)
		output := make([]byte, 32) // ecrecover returns 32 bytes (address)

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 50000, []byte{}).
			// Contract calls ecrecover precompile
			StartStaticCall(BobAddr, ecrecoverAddr, 5000, input).
			EndCall(output, 4500).
			EndCall([]byte{}, 45000).
			EndBlockTrx(successReceipt(50000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				assert.Equal(t, 2, len(trx.Calls))

				precompileCall := trx.Calls[1]
				assert.Equal(t, pbeth.CallType_STATIC, precompileCall.CallType)
				assert.Equal(t, ecrecoverAddr[:], precompileCall.Address)
				assert.False(t, precompileCall.StatusFailed)
				assert.Equal(t, uint64(128), uint64(len(precompileCall.Input)))
				assert.Equal(t, output, precompileCall.ReturnData)
			})
	})

	t.Run("sha256_precompile_success", func(t *testing.T) {
		input := []byte("test data for sha256")
		output := make([]byte, 32) // sha256 returns 32 bytes

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 50000, []byte{}).
			StartStaticCall(BobAddr, sha256Addr, 5000, input).
			EndCall(output, 4800).
			EndCall([]byte{}, 45000).
			EndBlockTrx(successReceipt(50000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				precompileCall := trx.Calls[1]
				assert.Equal(t, sha256Addr[:], precompileCall.Address)
				assert.False(t, precompileCall.StatusFailed)
			})
	})

	t.Run("ripemd160_precompile_success", func(t *testing.T) {
		input := []byte("test")
		output := make([]byte, 32) // ripemd160 returns 32 bytes (20 byte hash right-padded)

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 50000, []byte{}).
			StartStaticCall(BobAddr, ripemd160Addr, 5000, input).
			EndCall(output, 4900).
			EndCall([]byte{}, 45000).
			EndBlockTrx(successReceipt(50000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				precompileCall := trx.Calls[1]
				assert.Equal(t, ripemd160Addr[:], precompileCall.Address)
				assert.False(t, precompileCall.StatusFailed)
			})
	})

	t.Run("bn256_add_precompile_success", func(t *testing.T) {
		// bn256Add takes 128 bytes (4 * 32-byte values)
		input := make([]byte, 128)
		output := make([]byte, 64) // Returns 2 * 32-byte values

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 50000, []byte{}).
			StartStaticCall(BobAddr, bn256AddAddr, 10000, input).
			EndCall(output, 9500).
			EndCall([]byte{}, 40000).
			EndBlockTrx(successReceipt(50000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				precompileCall := trx.Calls[1]
				assert.Equal(t, bn256AddAddr[:], precompileCall.Address)
				assert.False(t, precompileCall.StatusFailed)
			})
	})

	t.Run("bn256_scalar_mul_precompile_success", func(t *testing.T) {
		// bn256ScalarMul takes 96 bytes
		input := make([]byte, 96)
		output := make([]byte, 64)

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 50000, []byte{}).
			StartStaticCall(BobAddr, bn256ScalarMulAddr, 10000, input).
			EndCall(output, 9000).
			EndCall([]byte{}, 40000).
			EndBlockTrx(successReceipt(50000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				precompileCall := trx.Calls[1]
				assert.Equal(t, bn256ScalarMulAddr[:], precompileCall.Address)
				assert.False(t, precompileCall.StatusFailed)
			})
	})

	t.Run("bn256_scalar_mul_precompile_failure", func(t *testing.T) {
		// Invalid input causes precompile to fail
		invalidInput := []byte{0x12, 0x34, 0x56} // Wrong size

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 50000, []byte{}).
			StartStaticCall(BobAddr, bn256ScalarMulAddr, 10000, invalidInput).
			EndCallFailed([]byte{}, 0, errors.New("invalid input"), true).
			EndCall([]byte{}, 40000).
			EndBlockTrx(successReceipt(50000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				precompileCall := trx.Calls[1]
				assert.Equal(t, bn256ScalarMulAddr[:], precompileCall.Address)
				assert.True(t, precompileCall.StatusFailed, "Precompile should fail with invalid input")
				assert.Equal(t, uint64(0), precompileCall.GasConsumed, "Failed precompile consumes no gas")
			})
	})

	t.Run("multiple_precompiles_in_transaction", func(t *testing.T) {
		// Transaction calls multiple precompiles
		sha256Input := []byte("test")
		sha256Output := make([]byte, 32)
		ecrecoverInput := make([]byte, 128)
		ecrecoverOutput := make([]byte, 32)

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 80000, []byte{}).
			// First precompile: sha256
			StartStaticCall(BobAddr, sha256Addr, 5000, sha256Input).
			EndCall(sha256Output, 4800).
			// Second precompile: ecrecover
			StartStaticCall(BobAddr, ecrecoverAddr, 5000, ecrecoverInput).
			EndCall(ecrecoverOutput, 4500).
			EndCall([]byte{}, 70000).
			EndBlockTrx(successReceipt(80000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				assert.Equal(t, 3, len(trx.Calls), "Root call + 2 precompile calls")

				assert.Equal(t, sha256Addr[:], trx.Calls[1].Address)
				assert.Equal(t, ecrecoverAddr[:], trx.Calls[2].Address)
				assert.False(t, trx.Calls[1].StatusFailed)
				assert.False(t, trx.Calls[2].StatusFailed)
			})
	})

	t.Run("nested_precompile_calls", func(t *testing.T) {
		// Root call -> Contract call -> Precompile call
		input := []byte("nested test")
		output := make([]byte, 32)

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 80000, []byte{}).
			// Bob calls Charlie
			StartCall(BobAddr, CharlieAddr, bigInt(0), 40000, []byte{}).
			// Charlie calls sha256 precompile
			StartStaticCall(CharlieAddr, sha256Addr, 5000, input).
			EndCall(output, 4800).
			EndCall([]byte{0x01}, 35000).
			EndCall([]byte{}, 45000).
			EndBlockTrx(successReceipt(80000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				assert.Equal(t, 3, len(trx.Calls))

				precompileCall := trx.Calls[2]
				assert.Equal(t, sha256Addr[:], precompileCall.Address)
				assert.Equal(t, uint32(2), precompileCall.ParentIndex, "Precompile's parent is Charlie (index 2)")
				assert.False(t, precompileCall.StatusFailed)
			})
	})
}

// TestTracer_CREATE2EdgeCases tests CREATE2-specific edge cases from battlefield tests
func TestTracer_CREATE2EdgeCases(t *testing.T) {
	t.Run("create2_collision_address_already_exists", func(t *testing.T) {
		// Simulate CREATE2 collision: trying to deploy to an address that already has code
		contractAddr := CharlieAddr

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 200000, []byte{}).
			// First CREATE2 succeeds
			StartCreate2Call(BobAddr, contractAddr, bigInt(0), 100000, []byte{0x60, 0x80}).
			EndCall([]byte{0x60, 0x80}, 95000).
			// Second CREATE2 to same address fails
			StartCreate2Call(BobAddr, contractAddr, bigInt(0), 50000, []byte{0x60, 0x80}).
			EndCallFailed([]byte{}, 0, testErrExecutionReverted, true).
			EndCall([]byte{}, 50000).
			EndBlockTrx(successReceipt(200000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				assert.Equal(t, 3, len(trx.Calls), "Root + 2 CREATE2 calls")

				// First CREATE2 succeeds
				assert.False(t, trx.Calls[1].StatusFailed)
				assert.Equal(t, pbeth.CallType_CREATE, trx.Calls[1].CallType)

				// Second CREATE2 fails due to collision
				assert.True(t, trx.Calls[2].StatusFailed)
				assert.Equal(t, pbeth.CallType_CREATE, trx.Calls[2].CallType)
				assert.Equal(t, contractAddr[:], trx.Calls[2].Address)
			})
	})

	t.Run("create2_with_insufficient_funds", func(t *testing.T) {
		// CREATE2 fails because contract doesn't have enough balance to transfer
		contractAddr := CharlieAddr
		largeValue := mustBigInt("1000000000000000000000") // 1000 ETH

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			// CREATE2 with value larger than available balance
			StartCreate2Call(BobAddr, contractAddr, largeValue, 50000, []byte{0x60, 0x80}).
			EndCallFailed([]byte{}, 0, testErrInsufficientBalanceTransfer, true).
			EndCall([]byte{}, 50000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				create2Call := trx.Calls[1]

				assert.True(t, create2Call.StatusFailed)
				assert.True(t, create2Call.StatusReverted, "Insufficient balance should be reverted")
			})
	})
}

// TestTracer_ConstructorEdgeCases tests constructor-related edge cases
func TestTracer_ConstructorEdgeCases(t *testing.T) {
	t.Run("constructor_with_storage_and_logs", func(t *testing.T) {
		// Constructor performs state changes
		contractAddr := CharlieAddr
		code := []byte{0x60, 0x80, 0x60, 0x40, 0x52}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			StartCreateCall(BobAddr, contractAddr, bigInt(0), 80000, code).
			// Constructor changes storage
			StorageChange(contractAddr, hash32(1), hash32(0), hash32(100)).
			// Constructor emits log
			Log(contractAddr, [][32]byte{hash32(1)}, []byte{0xaa, 0xbb}, 0).
			EndCall(code, 70000).
			EndCall([]byte{}, 30000).
			EndBlockTrx(receiptWithLogs(100000, []firehose.LogData{
				{Address: contractAddr, Topics: [][32]byte{hash32(1)}, Data: []byte{0xaa, 0xbb}},
			}), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				createCall := trx.Calls[1]

				assert.False(t, createCall.StatusFailed)
				assert.Equal(t, 1, len(createCall.StorageChanges))
				assert.Equal(t, 1, len(createCall.Logs))
			})
	})

	t.Run("constructor_fails_reverts_state_changes", func(t *testing.T) {
		// Constructor that fails should revert its state changes
		contractAddr := CharlieAddr
		code := []byte{0x60, 0x80}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			StartCreateCall(BobAddr, contractAddr, bigInt(0), 80000, code).
			// Constructor tries to change storage (will be reverted)
			StorageChange(contractAddr, hash32(1), hash32(0), hash32(100)).
			// Constructor fails
			EndCallFailed([]byte{}, 0, testErrExecutionReverted, true).
			EndCall([]byte{}, 20000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				createCall := trx.Calls[1]

				assert.True(t, createCall.StatusFailed)
				assert.True(t, createCall.StatusReverted)
				// Storage changes exist in call but should be marked as reverted
				assert.Equal(t, 1, len(createCall.StorageChanges))
			})
	})

	t.Run("recursive_constructor_failure", func(t *testing.T) {
		// Constructor creates another contract, which fails
		firstContractAddr := CharlieAddr
		secondContractAddr := addressFromHex("0x0000000000000000000000000000000000000abc")
		code := []byte{0x60, 0x80}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 200000, []byte{}).
			// First CREATE
			StartCreateCall(BobAddr, firstContractAddr, bigInt(0), 150000, code).
			// First constructor creates second contract
			StartCreateCall(firstContractAddr, secondContractAddr, bigInt(0), 80000, code).
			// Second constructor fails
			EndCallFailed([]byte{}, 0, testErrExecutionReverted, true).
			// First constructor also fails due to nested failure
			EndCallFailed([]byte{}, 0, testErrExecutionReverted, true).
			EndCall([]byte{}, 50000).
			EndBlockTrx(successReceipt(200000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				assert.Equal(t, 3, len(trx.Calls), "Root + 2 CREATE calls")

				// Both CREATEs should fail
				assert.True(t, trx.Calls[1].StatusFailed, "First CREATE should fail")
				assert.True(t, trx.Calls[2].StatusFailed, "Second CREATE should fail")
				assert.Equal(t, uint32(2), trx.Calls[2].ParentIndex, "Second CREATE's parent is first CREATE")
			})
	})

	t.Run("constructor_out_of_gas", func(t *testing.T) {
		// Constructor runs out of gas during execution
		contractAddr := CharlieAddr
		code := []byte{0x60, 0x80}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			StartCreateCall(BobAddr, contractAddr, bigInt(0), 50000, code).
			// Constructor runs out of gas
			EndCallFailed([]byte{}, 0, testErrOutOfGas, true).
			EndCall([]byte{}, 50000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				createCall := trx.Calls[1]

				assert.True(t, createCall.StatusFailed)
				assert.False(t, createCall.StatusReverted, "Out of gas is not reverted")
				assert.Equal(t, uint64(0), createCall.GasConsumed)
			})
	})
}
