package tests

import (
	"errors"
	"testing"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTracer_OnOpcodeFault tests opcode fault handling
func TestTracer_OnOpcodeFault(t *testing.T) {
	t.Run("invalid_opcode_fault", func(t *testing.T) {
		// Test that OnOpcodeFault sets ExecutedCode but doesn't directly set StatusFailed
		// The failure is handled by OnCallExit
		// Invalid opcode is a failure but NOT a revert (like out of gas)
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{0xff, 0xfe}).
			// Simulate an invalid opcode fault
			OpCodeFault(1, 0xfe, 21000, 0, errors.New("invalid opcode: opcode 0xfe not defined")).
			// Call fails due to the fault (propagates to OnCallExit)
			// Invalid opcode is a failure (reverted=true) but not a StatusReverted (not execution reverted)
			EndCallFailed([]byte{}, 0, errors.New("invalid opcode: opcode 0xfe not defined"), true).
			EndBlockTrx(failedReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// ExecutedCode should be set by OnOpcodeFault
				assert.True(t, call.ExecutedCode, "ExecutedCode should be set even for faulted opcodes")

				// StatusFailed is set by OnCallExit, not OnOpcodeFault
				assert.True(t, call.StatusFailed, "Call should be marked as failed")
				// Invalid opcode is not a "StatusReverted" (not ErrExecutionReverted)
				assert.False(t, call.StatusReverted, "Invalid opcode is failed but not reverted")
				assert.Contains(t, call.FailureReason, "invalid opcode")
			})
	})

	t.Run("stack_underflow_fault", func(t *testing.T) {
		// Stack underflow is a common fault condition
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{0x01}). // ADD without stack items
			OpCodeFault(0, 0x01, 21000, 0, errors.New("stack underflow")).
			EndCallFailed([]byte{}, 0, errors.New("stack underflow"), true).
			EndBlockTrx(failedReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.True(t, call.ExecutedCode, "ExecutedCode should be set")
				assert.True(t, call.StatusFailed, "Call should fail")
				assert.Contains(t, call.FailureReason, "stack underflow")
			})
	})

	t.Run("stack_overflow_fault", func(t *testing.T) {
		// Stack overflow (exceeding 1024 items)
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{0x60, 0x00}). // PUSH1 0
			OpCodeFault(1000, 0x60, 50000, 3, errors.New("stack limit reached 1024")).
			EndCallFailed([]byte{}, 50000, errors.New("stack limit reached 1024"), true).
			EndBlockTrx(failedReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.True(t, call.ExecutedCode)
				assert.True(t, call.StatusFailed)
				assert.Contains(t, call.FailureReason, "stack limit")
			})
	})

	t.Run("out_of_gas_fault", func(t *testing.T) {
		// Out of gas during opcode execution
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{0x5b}). // JUMPDEST
			OpCodeFault(0, 0x5b, 10, 5, testErrOutOfGas).
			EndCallFailed([]byte{}, 21000, testErrOutOfGas, true).
			EndBlockTrx(failedReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.True(t, call.ExecutedCode)
				assert.True(t, call.StatusFailed)
				// Out of gas is not a revert, it's a failure
				assert.False(t, call.StatusReverted, "Out of gas should not be reverted")
				assert.Equal(t, "out of gas", call.FailureReason)
			})
	})

	t.Run("nested_call_opcode_fault", func(t *testing.T) {
		// Opcode fault in a nested call
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			// Bob executes some code
			OpCode(0, 0x60, 100000, 3). // PUSH1
			// Bob calls Charlie
			StartCall(BobAddr, CharlieAddr, bigInt(50), 50000, []byte{0xfe}).
			// Charlie executes invalid opcode
			OpCodeFault(0, 0xfe, 50000, 0, errors.New("invalid opcode: 0xfe")).
			EndCallFailed([]byte{}, 0, errors.New("invalid opcode: 0xfe"), true).
			// Bob succeeds (handles the revert)
			EndCall([]byte{}, 90000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				require.Equal(t, 2, len(trx.Calls), "Should have 2 calls")

				rootCall := trx.Calls[0]
				nestedCall := trx.Calls[1]

				// Nested call should have ExecutedCode set and be failed
				assert.True(t, nestedCall.ExecutedCode, "Nested call should have ExecutedCode")
				assert.True(t, nestedCall.StatusFailed, "Nested call should fail")
				assert.Contains(t, nestedCall.FailureReason, "invalid opcode")

				// Root call should succeed and have ExecutedCode (from OpCode call)
				assert.False(t, rootCall.StatusFailed, "Root call should succeed")
				assert.True(t, rootCall.ExecutedCode, "Root call should have ExecutedCode")
			})
	})

	t.Run("multiple_opcode_faults_before_exit", func(t *testing.T) {
		// Multiple opcode faults before call exit (last fault's error is what matters)
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{0x60, 0x01, 0xfe}).
			OpCodeFault(0, 0x60, 21000, 3, errors.New("fault 1")).
			OpCodeFault(2, 0xfe, 20997, 0, errors.New("invalid opcode: final fault")).
			EndCallFailed([]byte{}, 0, errors.New("invalid opcode: final fault"), true).
			EndBlockTrx(failedReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.True(t, call.ExecutedCode)
				assert.True(t, call.StatusFailed)
				// The final error is what's recorded
				assert.Contains(t, call.FailureReason, "final fault")
			})
	})
}
