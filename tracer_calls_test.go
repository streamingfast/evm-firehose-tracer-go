package firehose

import (
	"testing"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
)

// TestTracer_CallTypes tests all different call types
func TestTracer_CallTypes(t *testing.T) {
	t.Run("CALL", func(t *testing.T) {
		NewTracerTester(t).
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
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
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 50000, []byte{}).
			// Bob makes a STATICCALL to Charlie
			StartStaticCall(1, BobAddr, CharlieAddr, 21000, []byte{0x01, 0x02}).
			EndCall([]byte{0x03, 0x04}, 20000, nil).
			EndCall([]byte{}, 45000, nil).
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
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 50000, []byte{}).
			// Bob makes a DELEGATECALL to Charlie
			StartDelegateCall(1, BobAddr, CharlieAddr, bigInt(0), 21000, []byte{0x05}).
			EndCall([]byte{0x06}, 20000, nil).
			EndCall([]byte{}, 45000, nil).
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
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 50000, []byte{}).
			// Bob makes a CALLCODE to Charlie
			StartCallCode(1, BobAddr, CharlieAddr, bigInt(50), 21000, []byte{}).
			EndCall([]byte{}, 20000, nil).
			EndCall([]byte{}, 45000, nil).
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
			StartBlockTrx().
			StartRootCreateCall(AliceAddr, BobAddr, bigInt(0), 53000, contractCode).
			EndCall(contractCode, 50000, nil).
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
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			// Bob deploys a contract using CREATE2
			StartCreate2Call(1, BobAddr, CharlieAddr, bigInt(0), 53000, contractCode).
			EndCall(contractCode, 50000, nil).
			EndCall([]byte{}, 95000, nil).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				assert.Equal(t, 2, len(trx.Calls), "Should have root call + nested CREATE2")

				nestedCall := trx.Calls[1]
				assert.Equal(t, pbeth.CallType_CREATE, nestedCall.CallType)
				// CREATE2 uses same CallType as CREATE
				assert.Empty(t, nestedCall.ReturnData, "CREATE2 call should have empty return data")
			})
	})
}

// TestTracer_CallFailures tests different failure scenarios
func TestTracer_CallFailures(t *testing.T) {
	t.Run("reverted_call", func(t *testing.T) {
		revertReason := "execution reverted"
		NewTracerTester(t).
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCallReverted([]byte{}, 5000, revertReason).
			EndBlockTrx(failedReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.True(t, call.StatusFailed, "Call should be marked as failed")
				assert.True(t, call.StatusReverted, "Call should be marked as reverted")
				assert.Equal(t, revertReason, call.FailureReason)
				assert.Equal(t, uint64(5000), call.GasConsumed)
			})
	})

	t.Run("failed_call_out_of_gas", func(t *testing.T) {
		NewTracerTester(t).
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCallFailed([]byte{}, 21000, "out of gas").
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
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{0xff}).
			EndCallFailed([]byte{}, 10000, "invalid opcode").
			EndBlockTrx(failedReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.True(t, call.StatusFailed)
				assert.Equal(t, "invalid opcode", call.FailureReason)
			})
	})
}

// TestTracer_NestedCalls tests nested call scenarios
func TestTracer_NestedCalls(t *testing.T) {
	t.Run("simple_nested_call", func(t *testing.T) {
		// Alice calls Bob, Bob calls Charlie
		NewTracerTester(t).
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{0x01}).
			// Bob makes a nested call to Charlie
			StartCall(1, BobAddr, CharlieAddr, bigInt(50), 50000, []byte{0x02}).
			EndCall([]byte{0x03}, 45000, nil). // Charlie returns
			EndCall([]byte{0x04}, 90000, nil). // Bob returns
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
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 200000, []byte{}).
			StartCall(1, BobAddr, CharlieAddr, bigInt(50), 150000, []byte{}).
			StartCall(2, CharlieAddr, MinerAddr, bigInt(25), 100000, []byte{}).
			EndCall([]byte{}, 95000, nil).   // Miner returns
			EndCall([]byte{}, 140000, nil).  // Charlie returns
			EndCall([]byte{}, 180000, nil).  // Bob returns
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
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			// Bob calls Charlie, which reverts
			StartCall(1, BobAddr, CharlieAddr, bigInt(50), 50000, []byte{}).
			EndCallReverted([]byte{}, 10000, "revert").
			// Bob continues and succeeds
			EndCall([]byte{0x05}, 80000, nil).
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
				assert.Equal(t, "revert", nestedCall.FailureReason)
			})
	})

	t.Run("nested_all_revert", func(t *testing.T) {
		// Alice calls Bob, Bob calls Charlie (fails), Bob also reverts
		NewTracerTester(t).
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			// Bob calls Charlie, which reverts
			StartCall(1, BobAddr, CharlieAddr, bigInt(50), 50000, []byte{}).
			EndCallReverted([]byte{}, 10000, "nested revert").
			// Bob also reverts
			EndCallReverted([]byte{}, 20000, "parent revert").
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
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 200000, []byte{}).
			// First sibling: Bob calls Charlie
			StartCall(1, BobAddr, CharlieAddr, bigInt(30), 80000, []byte{}).
			EndCall([]byte{0x01}, 75000, nil).
			// Second sibling: Bob calls Miner
			StartCall(1, BobAddr, MinerAddr, bigInt(20), 80000, []byte{}).
			EndCall([]byte{0x02}, 75000, nil).
			EndCall([]byte{0x03}, 150000, nil). // Bob returns
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
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 50000, largeInput).
			EndCall([]byte{0x01}, 45000, nil).
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
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			EndCall(largeOutput, 95000, nil).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assertEqualBytes(t, largeOutput, call.ReturnData)
			})
	})

	t.Run("call_with_zero_value", func(t *testing.T) {
		NewTracerTester(t).
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 21000, []byte{}).
			EndCall([]byte{}, 20000, nil).
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
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil). // All gas consumed
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
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			// Bob makes a STATICCALL to Charlie
			StartCallRaw(1, byte(CallTypeStaticCall), BobAddr, CharlieAddr, []byte{}, 50000, bigInt(0)).
			EndCall([]byte{0x01}, 45000, nil).
			EndCall([]byte{0x02}, 90000, nil).
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
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			// Bob makes a DELEGATECALL to Charlie
			StartCallRaw(1, byte(CallTypeDelegateCall), BobAddr, CharlieAddr, []byte{}, 50000, bigInt(0)).
			EndCall([]byte{0x01}, 45000, nil).
			EndCall([]byte{0x02}, 90000, nil).
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
			StartBlockTrx().
			StartRootCreateCall(AliceAddr, BobAddr, bigInt(0), 100000, contractCode).
			// New contract makes a call to Charlie
			StartCall(1, BobAddr, CharlieAddr, bigInt(0), 50000, []byte{}).
			EndCall([]byte{0x01}, 45000, nil).
			EndCall(contractCode, 90000, nil).
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
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 5000, &wrapError{
				reason:  "custom context: execution failed",
				wrapped: testErrExecutionReverted,
			}).
			EndBlockTrx(failedReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.True(t, call.StatusFailed, "Call should be marked as failed")
				assert.True(t, call.StatusReverted, "Call should be marked as reverted even with wrapped error")
				assert.Equal(t, "custom context: execution failed", call.FailureReason)
			})
	})

	t.Run("wrapped_insufficient_balance", func(t *testing.T) {
		// Test wrapped insufficient balance error
		NewTracerTester(t).
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 0, &wrapError{
				reason:  "transfer failed: not enough funds",
				wrapped: testErrInsufficientBalanceTransfer,
			}).
			EndBlockTrx(failedReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.True(t, call.StatusFailed)
				assert.True(t, call.StatusReverted, "Insufficient balance should be reverted")
				assert.Equal(t, "transfer failed: not enough funds", call.FailureReason)
			})
	})

	t.Run("double_wrapped_error", func(t *testing.T) {
		// Test double-wrapped error chain
		wrappedOnce := &wrapError{
			reason:  "context layer 1",
			wrapped: testErrMaxCallDepth,
		}
		wrappedTwice := &wrapError{
			reason:  "context layer 2",
			wrapped: wrappedOnce,
		}

		NewTracerTester(t).
			StartBlockTrx().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 0, wrappedTwice).
			EndBlockTrx(failedReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.True(t, call.StatusFailed)
				assert.True(t, call.StatusReverted, "Should find revert error through multiple wrapping layers")
				assert.Equal(t, "context layer 2", call.FailureReason)
			})
	})
}
