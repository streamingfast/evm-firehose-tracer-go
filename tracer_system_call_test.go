package firehose

import (
	"testing"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
)

// TestTracer_SystemCall tests system call tracking
// System calls are special transactions executed by the blockchain itself (e.g., beacon chain withdrawals, block rewards)
func TestTracer_SystemCall(t *testing.T) {
	t.Skip("System call tests not yet implemented - requires OnSystemCallStart/OnSystemCallEnd hooks")

	t.Run("SystemCall/BlockReward", func(t *testing.T) {
		scenario := NewTracerTester(t)
		scenario.StartBlock()

		// TODO: Add system call hooks when implemented
		// Example: Block reward distribution
		// scenario.Tracer.OnSystemCallStart(...)
		// scenario.Tracer.OnSystemCallEnd(...)

		scenario.EndBlock(nil)

		scenario.Validate(func(block *pbeth.Block) {
			assert.NotNil(t, block.SystemCalls, "Block should have system calls array")
			// TODO: Validate system call content
		})
	})
}

// TestTracer_Withdrawal tests beacon chain withdrawal tracking
func TestTracer_Withdrawal(t *testing.T) {
	t.Skip("Withdrawal tests not yet implemented - requires withdrawal hooks")

	t.Run("SystemCall/Withdrawal", func(t *testing.T) {
		scenario := NewTracerTester(t)
		scenario.StartBlock()

		// TODO: Add withdrawal tracking when implemented
		// Withdrawals are post-Shanghai (EIP-4895) beacon chain validator withdrawals

		scenario.EndBlock(nil)

		scenario.Validate(func(block *pbeth.Block) {
			// TODO: Validate withdrawals
			assert.NotNil(t, block.Header)
		})
	})
}
