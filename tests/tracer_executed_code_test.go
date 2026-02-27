package tests

import (
	"testing"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
)

// TestTracer_ExecutedCode tests ExecutedCode field in the non-backward-compatible path
// ExecutedCode is set to true when calls are made (firehose.go:1296 in captureInterpreterStep)
func TestTracer_ExecutedCode(t *testing.T) {
	t.Run("call_executed_code_true", func(t *testing.T) {
		// ExecutedCode = true for CALL when opcode executes
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{0x01, 0x02}).
			OpCode(0, 0x60, 21000, 3). // Execute a PUSH opcode
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, pbeth.CallType_CALL, call.CallType)
				assert.True(t, call.ExecutedCode, "ExecutedCode should be true when opcodes execute")
			})
	})

	t.Run("staticcall_executed_code_true", func(t *testing.T) {
		// ExecutedCode = true for STATICCALL
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, CharlieAddr, bigInt(0), 100000, []byte{0x01}).
			OpCode(0, 0x60, 100000, 3).
			StartStaticCall(1, CharlieAddr, BobAddr, 50000, []byte{0x02}).
			OpCode(0, 0x60, 50000, 3).
			EndCall([]byte{}, 45000, nil).
			EndCall([]byte{}, 95000, nil).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				staticCall := trx.Calls[1]

				assert.Equal(t, pbeth.CallType_STATIC, staticCall.CallType)
				assert.True(t, staticCall.ExecutedCode, "ExecutedCode should be true for STATICCALL")
			})
	})

	t.Run("delegatecall_executed_code_true", func(t *testing.T) {
		// ExecutedCode = true for DELEGATECALL
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, CharlieAddr, bigInt(0), 100000, []byte{0x01}).
			OpCode(0, 0x60, 100000, 3).
			StartDelegateCall(1, CharlieAddr, BobAddr, bigInt(0), 50000, []byte{0x02}).
			OpCode(0, 0x60, 50000, 3).
			EndCall([]byte{}, 45000, nil).
			EndCall([]byte{}, 95000, nil).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				delegateCall := trx.Calls[1]

				assert.Equal(t, pbeth.CallType_DELEGATE, delegateCall.CallType)
				assert.True(t, delegateCall.ExecutedCode, "ExecutedCode should be true for DELEGATECALL")
			})
	})

	t.Run("create_executed_code_true", func(t *testing.T) {
		// ExecutedCode = true for CREATE
		contractCode := []byte{0x60, 0x80, 0x60, 0x40}

		NewTracerTester(t).
			StartBlockTrx().
			StartRootCreateCall(AliceAddr, BobAddr, bigInt(0), 53000, contractCode).
			OpCode(0, 0x60, 53000, 3).
			EndCall(contractCode, 50000, nil).
			EndBlockTrx(successReceipt(53000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, pbeth.CallType_CREATE, call.CallType)
				assert.True(t, call.ExecutedCode, "ExecutedCode should be true for CREATE")
			})
	})
}
