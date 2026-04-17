package firehose

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeLibNum(t *testing.T) {
	tests := []struct {
		name        string
		blockNumber uint64
		finalized   uint64 // 0 means empty finality status
		expected    uint64
	}{
		// --- Empty finality: falls back to max(block-200, 0) ---
		{
			name:        "empty_finality_block_0",
			blockNumber: 0,
			finalized:   0,
			expected:    0,
		},
		{
			name:        "empty_finality_block_1",
			blockNumber: 1,
			finalized:   0,
			expected:    0,
		},
		{
			name:        "empty_finality_block_199",
			blockNumber: 199,
			finalized:   0,
			expected:    0,
		},
		{
			name:        "empty_finality_block_200",
			blockNumber: 200,
			finalized:   0,
			expected:    0,
		},
		{
			name:        "empty_finality_block_201",
			blockNumber: 201,
			finalized:   0,
			expected:    1,
		},
		{
			name:        "empty_finality_block_1000",
			blockNumber: 1000,
			finalized:   0,
			expected:    800,
		},

		// --- Finality is set and within 200 blocks: use finalized directly ---
		{
			name:        "finalized_close_behind",
			blockNumber: 500,
			finalized:   400,
			expected:    400,
		},
		{
			name:        "finalized_exactly_200_behind",
			blockNumber: 500,
			finalized:   300,
			expected:    300,
		},
		{
			name:        "finalized_equal_to_block",
			blockNumber: 500,
			finalized:   500,
			expected:    500,
		},
		{
			name:        "finalized_one_behind",
			blockNumber: 500,
			finalized:   499,
			expected:    499,
		},

		// --- Finality is set but trails by more than 200: clamped ---
		{
			name:        "finalized_too_far_behind",
			blockNumber: 500,
			finalized:   100,
			expected:    300, // clamped to 500-200
		},
		{
			name:        "finalized_at_zero_high_block",
			blockNumber: 1000,
			finalized:   1, // non-zero so not empty, but very far behind
			expected:    800,
		},
		{
			name:        "finalized_at_1_block_250",
			blockNumber: 250,
			finalized:   1,
			expected:    50,
		},

		// --- Edge: small block numbers with finality set ---
		{
			name:        "finalized_small_block_no_clamp",
			blockNumber: 50,
			finalized:   10,
			expected:    10, // 50-10=40 < 200, no clamp
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &FinalityStatus{}
			if tt.finalized > 0 {
				fs.SetLastFinalizedBlock(tt.finalized)
			}

			got := computeLibNum(tt.blockNumber, fs)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestComputePrintedFlashBlockIndex(t *testing.T) {
	tests := []struct {
		name     string
		idx      *uint64
		isFinal  bool
		expected uint64
	}{
		{
			name:     "nil_not_flash_block",
			idx:      nil,
			isFinal:  false,
			expected: 0,
		},
		{
			name:     "partial_index_1",
			idx:      ptrUint64(1),
			isFinal:  false,
			expected: 1,
		},
		{
			name:     "partial_index_9",
			idx:      ptrUint64(9),
			isFinal:  false,
			expected: 9,
		},
		{
			name:     "final_index_10",
			idx:      ptrUint64(10),
			isFinal:  true,
			expected: 1010,
		},
		{
			name:     "final_index_1",
			idx:      ptrUint64(1),
			isFinal:  true,
			expected: 1001,
		},
		{
			name:     "final_index_0",
			idx:      ptrUint64(0),
			isFinal:  true,
			expected: 1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computePrintedFlashBlockIndex(tt.idx, tt.isFinal)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func ptrUint64(v uint64) *uint64 { return &v }
