package firehose

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"google.golang.org/protobuf/proto"
)

// GetTestingOutputBuffer returns the output buffer from the tracer's config if it is a bytes.Buffer, otherwise returns nil
func (t *Tracer) GetTestingOutputBuffer() *bytes.Buffer {
	if buf, ok := t.config.OutputWriter.(*bytes.Buffer); ok {
		return buf
	}

	return nil
}

// printToFirehose writes a message to the Firehose output stream
func (t *Tracer) printToFirehose(args ...any) {
	line := fmt.Sprintln(args...)
	t.outputWriter.Write([]byte(line))
}

// flushToFirehose writes bytes directly to the output stream
func (t *Tracer) flushToFirehose(bytes []byte) error {
	_, err := t.outputWriter.Write(bytes)
	return err
}

// printBlockToFirehose serializes and writes a block to the output stream.
//
// libNum is the LIB (last irreversible block) number to emit in the header; it
// must be computed by the caller while the block's finality state is still live
// (the concurrent flushing path resets finality before the block is flushed, so
// reading it here would race). See FinalityStatus.LibNumForBlock.
func (t *Tracer) printBlockToFirehose(block *pbeth.Block, libNum uint64) ([]byte, error) {
	marshalled, err := proto.Marshal(block)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal block: %w", err)
	}

	// Encode as base64 for Firehose protocol
	encoded := base64.StdEncoding.EncodeToString(marshalled)

	// Format: "FIRE BLOCK <block_num> <block_hash> <parent_num> <parent_hash> <lib_num> <timestamp> <payload>"
	blockHash := hex.EncodeToString(block.Hash)
	parentHash := hex.EncodeToString(block.Header.ParentHash)
	line := fmt.Sprintf("FIRE BLOCK %d %s %d %s %d %d %s\n",
		block.Number,
		blockHash,
		block.Number-1, // parent number
		parentHash,
		libNum,
		block.Header.Timestamp.AsTime().UnixNano(),
		encoded)
	return []byte(line), nil
}
