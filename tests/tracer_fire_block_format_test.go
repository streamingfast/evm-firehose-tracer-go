package tests

// Tests for the FIRE BLOCK output line format.
//
// Wire format:
//
//	FIRE BLOCK <block_num> <flash_block_idx> <block_hash> <prev_num> <prev_hash> <lib_num> <timestamp_unix_nano> <payload_base64>
//
// Every test in this file validates ALL header fields, not just the one(s) under
// test, to catch regressions in field ordering or formatting.

import (
	"encoding/hex"
	"math/big"
	"testing"
	"time"

	firehose "github.com/streamingfast/evm-firehose-tracer-go/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertAllFields validates every wire-level header field of a FirehoseBlockEntry
// against the expected values derived from the input BlockData.
func assertAllFields(
	t *testing.T,
	entry FirehoseBlockEntry,
	wantBlockNum uint64,
	wantFlashIdx uint64,
	wantBlockHash [32]byte,
	wantPrevNum uint64,
	wantPrevHash [32]byte,
	wantLibNum uint64,
	wantTimestamp uint64, // Unix seconds from BlockData.Time
) {
	t.Helper()

	assert.Equal(t, wantBlockNum, entry.BlockNum, "block_num")
	assert.Equal(t, wantFlashIdx, entry.FlashBlockIdx, "flash_block_idx")
	assert.Equal(t, hex.EncodeToString(wantBlockHash[:]), entry.BlockHash, "block_hash")
	assert.Equal(t, wantPrevNum, entry.PrevNum, "prev_num")
	assert.Equal(t, hex.EncodeToString(wantPrevHash[:]), entry.PrevHash, "prev_hash")
	assert.Equal(t, wantLibNum, entry.LibNum, "lib_num")
	assert.Equal(t, time.Unix(int64(wantTimestamp), 0).UnixNano(), entry.TimestampNano, "timestamp_nano")

	// Also verify the protobuf payload is consistent with the header
	assert.Equal(t, wantBlockNum, entry.Block.Number, "protobuf block.Number")
	assert.Equal(t, wantBlockHash[:], entry.Block.Hash, "protobuf block.Hash")
	assert.Equal(t, wantPrevHash[:], entry.Block.Header.ParentHash, "protobuf block.Header.ParentHash")
}

// ---------------------------------------------------------------------------
// lib_num tests
// ---------------------------------------------------------------------------

// TestFireBlockFormat_LibNum_EmptyFinality verifies that when no finalized block
// is reported, the emitted lib_num follows the max(block_num-200, 0) fallback.
func TestFireBlockFormat_LibNum_EmptyFinality(t *testing.T) {
	tests := []struct {
		name           string
		blockNumber    uint64
		expectedLibNum uint64
	}{
		{"block_below_200", 100, 0},
		{"block_at_200", 200, 0},
		{"block_at_201", 201, 1},
		{"block_at_500", 500, 300},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bd := blockDataWithNumber(tt.blockNumber)
			tester := NewTracerTester(t)

			tester.tracer.OnBlockStart(firehose.BlockEvent{Block: bd})
			tester.tracer.OnBlockEnd(nil)

			entries := ParseFirehoseBlockEntries(t, tt.name, tester.tracer.GetTestingOutputBuffer())
			require.Len(t, entries, 1)

			prevNum := tt.blockNumber - 1
			if tt.blockNumber == 0 {
				prevNum = 0
			}
			assertAllFields(t, entries[0],
				tt.blockNumber,    // block_num
				0,                 // flash_block_idx (non-flash)
				bd.Hash,           // block_hash
				prevNum,           // prev_num
				bd.ParentHash,     // prev_hash
				tt.expectedLibNum, // lib_num
				bd.Time,           // timestamp
			)
		})
	}
}

// TestFireBlockFormat_LibNum_WithFinality verifies that when a finalized block
// is reported, lib_num uses it directly (as long as it's within 200 blocks).
func TestFireBlockFormat_LibNum_WithFinality(t *testing.T) {
	tests := []struct {
		name            string
		blockNumber     uint64
		finalizedNumber uint64
		expectedLibNum  uint64
	}{
		{"finalized_close", 500, 450, 450},
		{"finalized_exactly_200_behind", 500, 300, 300},
		{"finalized_equal_to_block", 500, 500, 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bd := blockDataWithNumber(tt.blockNumber)
			tester := NewTracerTester(t)

			tester.tracer.OnBlockStart(firehose.BlockEvent{
				Block:     bd,
				Finalized: &firehose.FinalizedBlockRef{Number: tt.finalizedNumber},
			})
			tester.tracer.OnBlockEnd(nil)

			entries := ParseFirehoseBlockEntries(t, tt.name, tester.tracer.GetTestingOutputBuffer())
			require.Len(t, entries, 1)

			assertAllFields(t, entries[0],
				tt.blockNumber,
				0,
				bd.Hash,
				tt.blockNumber-1,
				bd.ParentHash,
				tt.expectedLibNum,
				bd.Time,
			)
		})
	}
}

// TestFireBlockFormat_LibNum_ClampedAt200 verifies that even when a finalized
// block is reported, lib_num is clamped to at most 200 blocks behind block_num.
func TestFireBlockFormat_LibNum_ClampedAt200(t *testing.T) {
	bd := blockDataWithNumber(500)
	tester := NewTracerTester(t)

	tester.tracer.OnBlockStart(firehose.BlockEvent{
		Block:     bd,
		Finalized: &firehose.FinalizedBlockRef{Number: 100},
	})
	tester.tracer.OnBlockEnd(nil)

	entries := ParseFirehoseBlockEntries(t, "clamped", tester.tracer.GetTestingOutputBuffer())
	require.Len(t, entries, 1)

	assertAllFields(t, entries[0],
		500,
		0,
		bd.Hash,
		499,
		bd.ParentHash,
		300, // clamped to 500-200
		bd.Time,
	)
}

// TestFireBlockFormat_LibNum_AcrossMultipleBlocks verifies that finality set on
// one block does not leak to the next block.
func TestFireBlockFormat_LibNum_AcrossMultipleBlocks(t *testing.T) {
	tester := NewTracerTester(t)

	bd300 := blockDataWithNumber(300)
	bd301 := blockDataWithNumber(301)

	// Block 300, finalized at 250
	tester.tracer.OnBlockStart(firehose.BlockEvent{
		Block:     bd300,
		Finalized: &firehose.FinalizedBlockRef{Number: 250},
	})
	tester.tracer.OnBlockEnd(nil)

	// Block 301, no finality → empty heuristic → max(301-200,0)=101
	tester.tracer.OnBlockStart(firehose.BlockEvent{Block: bd301})
	tester.tracer.OnBlockEnd(nil)

	entries := ParseFirehoseBlockEntries(t, "multi_block", tester.tracer.GetTestingOutputBuffer())
	require.Len(t, entries, 2)

	assertAllFields(t, entries[0],
		300, 0, bd300.Hash, 299, bd300.ParentHash, 250, bd300.Time,
	)
	assertAllFields(t, entries[1],
		301, 0, bd301.Hash, 300, bd301.ParentHash, 101, bd301.Time,
	)
}

// ---------------------------------------------------------------------------
// flash_block_idx tests
// ---------------------------------------------------------------------------

// TestFireBlockFormat_FlashBlockIdx_NonFlash verifies that non-flash blocks
// emit flash_block_idx=0.
func TestFireBlockFormat_FlashBlockIdx_NonFlash(t *testing.T) {
	tester := NewTracerTester(t)
	tester.StartBlock()
	tester.EndBlock(nil)

	entries := ParseFirehoseBlockEntries(t, "non_flash", tester.tracer.GetTestingOutputBuffer())
	require.Len(t, entries, 1)

	bd := TestBlock.Block
	assertAllFields(t, entries[0],
		bd.Number, 0, bd.Hash, bd.Number-1, bd.ParentHash, 0, bd.Time,
	)
}

// TestFireBlockFormat_FlashBlockIdx_Partial verifies that partial flash blocks
// emit their flash block index directly.
func TestFireBlockFormat_FlashBlockIdx_Partial(t *testing.T) {
	tester := NewTracerTester(t)
	bd := TestBlock.Block

	tester.tracer.OnBlockStart(firehose.BlockEvent{
		Block:      bd,
		FlashBlock: &firehose.FlashBlockData{Idx: 5},
	})
	tester.tracer.OnBlockEnd(nil)

	entries := ParseFirehoseBlockEntries(t, "partial_flash", tester.tracer.GetTestingOutputBuffer())
	require.Len(t, entries, 1)

	assertAllFields(t, entries[0],
		bd.Number, 5, bd.Hash, bd.Number-1, bd.ParentHash, 0, bd.Time,
	)
}

// TestFireBlockFormat_FlashBlockIdx_Final verifies that the final flash block
// emits flash_block_idx = Idx + 1000.
func TestFireBlockFormat_FlashBlockIdx_Final(t *testing.T) {
	tester := NewTracerTester(t)
	bd := TestBlock.Block

	tester.tracer.OnBlockStart(firehose.BlockEvent{
		Block:      bd,
		FlashBlock: &firehose.FlashBlockData{Idx: 10, IsFinal: true},
	})
	tester.tracer.OnBlockEnd(nil)

	entries := ParseFirehoseBlockEntries(t, "final_flash", tester.tracer.GetTestingOutputBuffer())
	require.Len(t, entries, 1)

	assertAllFields(t, entries[0],
		bd.Number, 1010, bd.Hash, bd.Number-1, bd.ParentHash, 0, bd.Time,
	)
}

// TestFireBlockFormat_FlashBlockIdx_Sequence verifies that a sequence of partial
// then final flash blocks emits the correct indices in order.
func TestFireBlockFormat_FlashBlockIdx_Sequence(t *testing.T) {
	tester := NewTracerTester(t)
	bd := TestBlock.Block

	// Partial 1
	tester.tracer.OnBlockStart(firehose.BlockEvent{
		Block:      bd,
		FlashBlock: &firehose.FlashBlockData{Idx: 1},
	})
	tester.tracer.SnapshotFlashBlockForNextIteration()
	tester.tracer.OnBlockEnd(nil)

	// Partial 3 (indices can skip)
	tester.tracer.OnBlockStart(firehose.BlockEvent{
		Block:      bd,
		FlashBlock: &firehose.FlashBlockData{Idx: 3},
	})
	tester.tracer.SnapshotFlashBlockForNextIteration()
	tester.tracer.OnBlockEnd(nil)

	// Final 5
	tester.tracer.OnBlockStart(firehose.BlockEvent{
		Block:      bd,
		FlashBlock: &firehose.FlashBlockData{Idx: 5, IsFinal: true},
	})
	tester.tracer.OnBlockEnd(nil)

	entries := ParseFirehoseBlockEntries(t, "flash_sequence", tester.tracer.GetTestingOutputBuffer())
	require.Len(t, entries, 3)

	assertAllFields(t, entries[0],
		bd.Number, 1, bd.Hash, bd.Number-1, bd.ParentHash, 0, bd.Time,
	)
	assertAllFields(t, entries[1],
		bd.Number, 3, bd.Hash, bd.Number-1, bd.ParentHash, 0, bd.Time,
	)
	assertAllFields(t, entries[2],
		bd.Number, 1005, bd.Hash, bd.Number-1, bd.ParentHash, 0, bd.Time,
	)
}

// ---------------------------------------------------------------------------
// prev_num edge case
// ---------------------------------------------------------------------------

// TestFireBlockFormat_PrevNum_BlockZero verifies that block 0 emits prev_num=0
// (not underflow).
func TestFireBlockFormat_PrevNum_BlockZero(t *testing.T) {
	bd := firehose.BlockData{
		Number:     0,
		Hash:       hash32(0),
		ParentHash: hash32(0), // genesis parent is zero
		Bloom:      make([]byte, 256),
		Difficulty: big.NewInt(0),
		GasLimit:   30_000_000,
		Time:       1704067200,
		Size:       509,
	}

	tester := NewTracerTester(t)
	tester.tracer.OnBlockStart(firehose.BlockEvent{Block: bd})
	tester.tracer.OnBlockEnd(nil)

	entries := ParseFirehoseBlockEntries(t, "block_zero", tester.tracer.GetTestingOutputBuffer())
	require.Len(t, entries, 1)

	assertAllFields(t, entries[0],
		0,             // block_num
		0,             // flash_block_idx
		bd.Hash,       // block_hash
		0,             // prev_num (no underflow)
		bd.ParentHash, // prev_hash
		0,             // lib_num
		bd.Time,       // timestamp
	)
}

// ---------------------------------------------------------------------------
// Combined: finality + flash
// ---------------------------------------------------------------------------

// TestFireBlockFormat_FinalityAndFlashBlock verifies that both lib_num and
// flash_block_idx are set correctly on the same FIRE BLOCK line.
func TestFireBlockFormat_FinalityAndFlashBlock(t *testing.T) {
	bd := blockDataWithNumber(500)
	tester := NewTracerTester(t)

	tester.tracer.OnBlockStart(firehose.BlockEvent{
		Block:      bd,
		Finalized:  &firehose.FinalizedBlockRef{Number: 480},
		FlashBlock: &firehose.FlashBlockData{Idx: 7, IsFinal: true},
	})
	tester.tracer.OnBlockEnd(nil)

	entries := ParseFirehoseBlockEntries(t, "finality_and_flash", tester.tracer.GetTestingOutputBuffer())
	require.Len(t, entries, 1)

	assertAllFields(t, entries[0],
		500,
		1007, // 7 + 1000 (final)
		bd.Hash,
		499,
		bd.ParentHash,
		480, // finalized
		bd.Time,
	)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// blockDataWithNumber creates a minimal BlockData for the given block number,
// suitable for testing output-format concerns where only the number matters.
func blockDataWithNumber(number uint64) firehose.BlockData {
	parentNum := number - 1
	if number == 0 {
		parentNum = 0
	}

	return firehose.BlockData{
		Number:     number,
		Hash:       hash32(number),
		ParentHash: hash32(parentNum),
		Bloom:      make([]byte, 256),
		Difficulty: big.NewInt(0),
		GasLimit:   30_000_000,
		Time:       1704067200 + number,
		Size:       509,
	}
}
