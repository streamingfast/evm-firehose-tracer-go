package tests

import (
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go/v5"
	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTracer_OnSkippedBlock tests skipped block handling
func TestTracer_OnSkippedBlock(t *testing.T) {
	t.Run("empty_skipped_block", func(t *testing.T) {
		// Skipped blocks should have 0 transactions and be traced as normal empty blocks
		NewTracerTester(t).
			SkippedBlock(100).
			Validate(func(block *pbeth.Block) {
				// Should have a block with number 100
				assert.Equal(t, uint64(100), block.Number)

				// Should have no transactions (skipped blocks have 0 transactions)
				assert.Empty(t, block.TransactionTraces, "Skipped block should have no transactions")

				// Should have proper header
				require.NotNil(t, block.Header)
				assert.Equal(t, uint64(100), block.Header.Number)
			})
	})

	t.Run("skipped_block_preserves_coinbase", func(t *testing.T) {
		// Ensure skipped blocks preserve coinbase
		// Build custom block with Alice as coinbase
		// Hash must match the computed hash for these exact block parameters
		blockEvent := (&firehose.BlockEventBuilder{}).
			Number(200).
			Hash("0x96c3afce2aab3c77e1f8ce47d01e50817d98f884d697e0af9b35c11e2626be8b"). // Computed hash for these parameters
			ParentHash("0x0000000000000000000000000000000000000000000000000000000000000063").
			Timestamp(1704067200).
			Coinbase(Alice).
			GasLimit(15_000_000).
			Difficulty(bigInt(0)).
			Size(509).
			Bloom(make([]byte, 256)).
			Build()

		tester := NewTracerTester(t)
		tester.tracer.OnSkippedBlock(blockEvent)

		tester.Validate(func(block *pbeth.Block) {
			assert.Equal(t, uint64(200), block.Number)
			assert.Empty(t, block.TransactionTraces)

			require.NotNil(t, block.Header)
			assert.Equal(t, uint64(200), block.Header.Number)
			assert.Equal(t, AliceAddr[:], block.Header.Coinbase)
			assert.Equal(t, uint64(15_000_000), block.Header.GasLimit)
		})
	})

	t.Run("multiple_skipped_blocks", func(t *testing.T) {
		// Test processing multiple skipped blocks
		tester := NewTracerTester(t).
			SkippedBlock(100).
			SkippedBlock(101).
			SkippedBlock(102)

		// Parse all blocks from output
		sharedBlocks := ParseFirehoseBlocks(t, "shared tracer", tester.tracer.GetTestingOutputBuffer())

		// Should have 3 blocks
		require.Len(t, sharedBlocks, 3, "Should have 3 blocks from shared tracer")

		// Validate each block
		for i := 0; i < 3; i++ {
			expectedNumber := uint64(100 + i)

			assert.Equal(t, expectedNumber, sharedBlocks[i].Number, "Block %d number should be %d", i, expectedNumber)
			assert.Empty(t, sharedBlocks[i].TransactionTraces, "Block %d should have no transactions", i)
		}
	})
}

// SkippedBlock is a test helper for OnSkippedBlock
func (s *TracerTester) SkippedBlock(blockNumber uint64) *TracerTester {
	blockEvent := (&firehose.BlockEventBuilder{}).
		Number(blockNumber).
		Hash("0xe74fcc728df762055c71a999736bb89dd47c541807c3021a1b94de6761afaf25"). // Use standard test hash
		ParentHash("0x0000000000000000000000000000000000000000000000000000000000000063").
		Timestamp(1704067200).
		Coinbase(Miner).
		GasLimit(30_000_000).
		Difficulty(bigInt(0)).
		Size(509).                // Match TestBlock size
		Bloom(make([]byte, 256)). // Empty 256-byte logs bloom filter
		Build()

	s.tracer.OnSkippedBlock(blockEvent)
	return s
}
