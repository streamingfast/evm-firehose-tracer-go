package firehose

import (
	"os"
	"strconv"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
)

// FinalityStatus tracks the finality status of blocks
// This is used to mark blocks as finalized in consensus mechanisms
type FinalityStatus struct {
	lastFinalizedBlockNumber uint64
}

// Reset clears the finality status
func (fs *FinalityStatus) Reset() {
	fs.lastFinalizedBlockNumber = 0
}

// PopulateBlockStatus populates the finality status field of a block
func (fs *FinalityStatus) PopulateBlockStatus(block *pbeth.Block) {
	// Note: DetailLevel enum values depend on the protobuf schema
	// This is a placeholder - actual values should match the schema
	if block.Number <= fs.lastFinalizedBlockNumber {
		block.DetailLevel = pbeth.Block_DetailLevel(1) // EXTENDED
	} else {
		block.DetailLevel = pbeth.Block_DetailLevel(2) // TRACE
	}
}

// SetLastFinalizedBlock updates the last finalized block number
func (fs *FinalityStatus) SetLastFinalizedBlock(blockNumber uint64) {
	fs.lastFinalizedBlockNumber = blockNumber
}

// LastFinalizedBlock returns the last finalized block number
func (fs *FinalityStatus) LastFinalizedBlock() uint64 {
	return fs.lastFinalizedBlockNumber
}

// defaultLibNumFallbackDistance is how far behind the current block the LIB
// (last irreversible block) is assumed to be when no finalized block is known
// yet (e.g. pre-merge chains, or before the consensus layer reports finality).
const defaultLibNumFallbackDistance = 200

// libNumFallbackDistance is the resolved fallback distance. It can be overridden
// via the FORCE_FINALIZED_BLOCK_ABOVE_THRESHOLD environment variable, matching
// the knob carried by the go-ethereum native tracer. A value of 0 disables the
// fallback entirely (LIB stays at 0 until real finality is known).
var libNumFallbackDistance = resolveLibNumFallbackDistance()

func resolveLibNumFallbackDistance() uint64 {
	if v := os.Getenv("FORCE_FINALIZED_BLOCK_ABOVE_THRESHOLD"); v != "" {
		if thresh, err := strconv.ParseUint(v, 10, 64); err == nil {
			return thresh
		}
	}

	return defaultLibNumFallbackDistance
}

// LibNumForBlock returns the LIB (last irreversible block) number to emit in the
// "FIRE BLOCK" header for the given block number.
//
// When a finalized block is known, that block number is the LIB. Otherwise we
// fall back to blockNum - libNumFallbackDistance (clamped at 0), matching the
// historical behavior of the native tracer so downstream Firehose never sees a
// stuck lib=0 on chains that do not report finality. The fallback can be tuned
// (or disabled with 0) via FORCE_FINALIZED_BLOCK_ABOVE_THRESHOLD.
func (fs *FinalityStatus) LibNumForBlock(blockNum uint64) uint64 {
	if fs.lastFinalizedBlockNumber != 0 {
		return fs.lastFinalizedBlockNumber
	}

	// Fallback disabled: keep LIB at 0 until real finality is known.
	if libNumFallbackDistance == 0 {
		return 0
	}

	if blockNum >= libNumFallbackDistance {
		return blockNum - libNumFallbackDistance
	}

	return 0
}
