package firehose

import (
	"bytes"
	"encoding/base64"
	"fmt"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"google.golang.org/protobuf/proto"
)

// blockOutput carries a block together with the precomputed context needed to
// format and write its "FIRE BLOCK" line. The context is captured before the
// tracer state is reset at the end of a block, so the (possibly concurrent)
// flushing path can render the line using the correct per-block state.
type blockOutput struct {
	block *pbeth.Block

	// libNum is the last irreversible block number to advertise for this block.
	libNum uint64

	// printedFlashBlockIndex is the value emitted in the flash-block-index slot of
	// the FIRE BLOCK line. It is 0 for non-flash blocks, equals the flash block
	// index for partials, and equals `Idx + 1000` for the final flash block
	// iteration (so partial indices 1..9 emit as 1..9 and the final 10th partial
	// emits as 1010), matching the Optimism Geth firehose tracer behavior.
	printedFlashBlockIndex uint64
}

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
// Output format (one line):
//
//	FIRE BLOCK <block_num> <flash_block_idx> <block_hash> <prev_num> <prev_hash> <lib_num> <timestamp_unix_nano> <payload_base64>
//
// flash_block_idx is 0 for non-flash blocks; for flash blocks it is the current
// flash block index plus 1000 when this is the final iteration for the block.
func (t *Tracer) printBlockToFirehose(out *blockOutput) ([]byte, error) {
	block := out.block

	marshalled, err := proto.Marshal(block)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal block: %w", err)
	}

	previousNum := uint64(0)
	if block.Number > 0 {
		previousNum = block.Number - 1
	}

	// Build the header plus base64 payload in a single buffer to minimize copies.
	// **Important** The final space in the Sprintf template is mandatory.
	buf := bytes.NewBuffer(nil)
	fmt.Fprintf(buf, "FIRE BLOCK %d %d %s %d %s %d %d ",
		block.Number,
		out.printedFlashBlockIndex,
		block.ID(),
		previousNum,
		block.PreviousID(),
		out.libNum,
		block.MustTime().UnixNano(),
	)

	encoder := base64.NewEncoder(base64.StdEncoding, buf)
	if _, err := encoder.Write(marshalled); err != nil {
		return nil, fmt.Errorf("write to base64 encoder should have been infallible: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("closing base64 encoder should have been infallible: %w", err)
	}

	buf.WriteString("\n")
	return buf.Bytes(), nil
}

// computeLibNum computes the last irreversible block number to advertise for the
// given block using the current finality status. It mirrors the Optimism Geth
// firehose tracer logic:
//   - When finality is known, use LastFinalizedBlock.
//   - When finality is empty, fall back to max(blockNumber-200, 0).
//   - In all cases, never let libNum fall more than 200 blocks behind blockNumber.
func computeLibNum(blockNumber uint64, finality *FinalityStatus) uint64 {
	libNum := finality.LastFinalizedBlock()

	if finality.IsEmpty() {
		if blockNumber >= 200 {
			libNum = blockNumber - 200
		} else {
			libNum = 0
		}
	}

	// Cap: libNum must never trail blockNumber by more than 200 blocks.
	if blockNumber >= 200 && libNum < blockNumber-200 {
		libNum = blockNumber - 200
	}

	return libNum
}

// computePrintedFlashBlockIndex returns the value to emit in the flash-block-index
// slot of the FIRE BLOCK line. See blockOutput.printedFlashBlockIndex for details.
func computePrintedFlashBlockIndex(flashBlockIndex *uint64, isFinal bool) uint64 {
	if flashBlockIndex == nil {
		return 0
	}
	idx := *flashBlockIndex
	if isFinal {
		idx += 1000
	}
	return idx
}
