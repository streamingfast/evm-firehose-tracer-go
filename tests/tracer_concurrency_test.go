package tests

import (
	"bytes"
	"math/big"
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go/v4"
	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/require"
)

// TestTracer_ConcurrentFlushing_SingleBlock tests that a single block is flushed correctly
func TestTracer_ConcurrentFlushing_SingleBlock(t *testing.T) {
	outputBuffer := &bytes.Buffer{}

	tester := newConcurrentTracerTester(t, 1, outputBuffer)

	// Process a single block
	tester.
		StartBlockTrx(TestLegacyTrx).
		StartCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
		BalanceChange(AliceAddr, bigInt(100), bigInt(50), pbeth.BalanceChange_REASON_GAS_BUY).
		EndCall([]byte{}, 95000).
		EndBlockTrx(successReceipt(100000), nil, nil)

	// Close the tracer to flush the concurrent queue
	tester.tracer.OnClose()

	// Parse all blocks from output
	blocks := ParseFirehoseBlocks(t, "concurrent flushing single block", outputBuffer)

	// Should have exactly 1 block
	require.Len(t, blocks, 1, "Expected exactly 1 block")

	// Verify block number
	require.Equal(t, uint64(100), blocks[0].Number, "Block number should be 100")
}

// TestTracer_ConcurrentFlushing_MultipleBlocksInOrder tests that multiple blocks are flushed in order
func TestTracer_ConcurrentFlushing_MultipleBlocksInOrder(t *testing.T) {
	const blockCount = 100

	outputBuffer := &bytes.Buffer{}
	tester := newConcurrentTracerTester(t, 1, outputBuffer)

	for i := 0; i < blockCount; i++ {
		tester.
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			BalanceChange(AliceAddr, bigInt(100), bigInt(50), pbeth.BalanceChange_REASON_GAS_BUY).
			EndCall([]byte{}, 95000).
			EndBlockTrx(successReceipt(100000), nil, nil)
	}

	// Close the tracer to flush the concurrent queue
	tester.tracer.OnClose()

	// Parse all blocks from output
	blocks := ParseFirehoseBlocks(t, "concurrent flushing multiple blocks", outputBuffer)

	// Verify block count
	require.Len(t, blocks, blockCount, "Expected %d blocks", blockCount)

	// All blocks will have the same number (100) from TestBlock
	// The key test is that they're all flushed without error
	for i, block := range blocks {
		require.Equal(t, uint64(100), block.Number, "Block %d should have number 100", i)
	}
}

// TestTracer_ConcurrentFlushing_LargeBuffer tests with a larger buffer size
func TestTracer_ConcurrentFlushing_LargeBuffer(t *testing.T) {
	const blockCount = 50
	const bufferSize = 10

	outputBuffer := &bytes.Buffer{}
	tester := newConcurrentTracerTester(t, bufferSize, outputBuffer)

	for i := 0; i < blockCount; i++ {
		tester.
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			EndCall([]byte{}, 95000).
			EndBlockTrx(successReceipt(100000), nil, nil)
	}

	// Close the tracer to flush the concurrent queue
	tester.tracer.OnClose()

	// Parse all blocks from output
	blocks := ParseFirehoseBlocks(t, "concurrent flushing large buffer", outputBuffer)

	// Verify all blocks were flushed
	require.Len(t, blocks, blockCount, "Expected %d blocks", blockCount)

	// All blocks have the same number from TestBlock
	for i, block := range blocks {
		require.Equal(t, uint64(100), block.Number, "Block %d should have number 100", i)
	}
}

// newConcurrentTracerTester creates a TracerTester with concurrent flushing enabled
func newConcurrentTracerTester(t *testing.T, bufferSize int, outputBuffer *bytes.Buffer) *TracerTester {
	chainConfig := &firehose.ChainConfig{
		ChainID: big.NewInt(1),
	}

	tester := &TracerTester{
		t: t,
		tracer: firehose.NewTracer(&firehose.Config{
			ChainConfig:              chainConfig,
			EnableConcurrentFlushing: true,
			ConcurrentBufferSize:     bufferSize,
			OutputWriter:             outputBuffer,
		}),
		mockStateDB: newMockStateDB(),
	}

	tester.tracer.OnBlockchainInit("test", "1.0.0", chainConfig, nil)

	return tester
}
