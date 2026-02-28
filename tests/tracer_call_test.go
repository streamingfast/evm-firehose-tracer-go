package tests

import (
	"testing"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
)

// TestTracer_Call tests call lifecycle hooks
func TestTracer_Call(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 1, len(block.TransactionTraces))
				trx := block.TransactionTraces[0]
				assert.Equal(t, 1, len(trx.Calls), "Should have one call")

				call := trx.Calls[0]
				assert.Equal(t, pbeth.CallType_CALL, call.CallType)
				assert.Equal(t, AliceAddr[:], call.Caller)
				assert.Equal(t, BobAddr[:], call.Address)
				assert.Equal(t, uint32(0), call.Depth)
				assert.Equal(t, uint64(21000), call.GasLimit)
			})
	})
}
