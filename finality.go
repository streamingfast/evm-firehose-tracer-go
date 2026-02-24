package firehose

import (
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
