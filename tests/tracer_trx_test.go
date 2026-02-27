package tests

import (
	"testing"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
)

func TestTracer_Transaction(t *testing.T) {
	t.Run("Transaction/Simple", func(t *testing.T) {
		NewTracerTester(t).
			StartBlockTrx().
			EndBlockTrx(successReceipt(0), nil, nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, uint64(TestBlock.Block.Number), block.Number)
			})
	})
}
