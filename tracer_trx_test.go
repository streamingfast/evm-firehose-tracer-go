package firehose

import (
	"testing"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
)

func TestTracer_Transaction(t *testing.T) {
	t.Run("Transaction/Simple", func(t *testing.T) {
		NewBlockScenario(t).
			StartBlockTrx().
			EndBlockTrx(&ReceiptData{Status: 1}, nil, nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, uint64(TestBlock.Block.Number), block.Number)
			})
	})
}
