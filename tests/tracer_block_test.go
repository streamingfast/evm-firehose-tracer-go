package tests

import (
	"testing"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
)

type testCase struct {
	Name     string
	Scenario func(tt *testing.T) *TracerTester
}

func TestSimpleBlockTest(t *testing.T) {
	t.Run("SimpleBlock_Builder", func(t *testing.T) {
		NewTracerTester(t).
			StartBlock().
			EndBlock(nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, uint64(TestBlock.Block.Number), block.Number)
			})
	})
}
