package firehose

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"

	"github.com/emmansun/base64"
	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
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
	t.flushToFirehose([]byte(fmt.Sprintln(args...)))
}

// maxWriteAttempts bounds how many times a short write to the output stream is retried.
const maxWriteAttempts = 10

// flushToFirehose writes bytes to the output stream, retrying short writes. It panics when
// the bytes still cannot be fully written, stopping the node instead of silently losing a block.
func (t *Tracer) flushToFirehose(data []byte) {
	var err error
	for attempt := 0; attempt < maxWriteAttempts; attempt++ {
		var written int
		written, err = t.outputWriter.Write(data)
		data = data[written:]
		if len(data) == 0 {
			return
		}
	}

	panic(fmt.Errorf("failed to write to Firehose output after %d attempts, %d bytes not written: %w", maxWriteAttempts, len(data), err))
}

// blockLineBuffers holds the scratch buffers used to render a FIRE BLOCK line. A traced
// block routinely marshals to several megabytes, so the buffers are kept and reused from
// one block to the next instead of being reallocated and garbage collected every time.
type blockLineBuffers struct {
	payload []byte
	line    []byte
}

// writeBlock serializes and writes a block to the output stream, panicking on failure.
// It must not be called concurrently: blocks are written either synchronously from
// OnBlockEnd or by the single ConcurrentFlushQueue worker, never both.
func (t *Tracer) writeBlock(out *blockOutput) {
	line, err := t.blockLine.render(out)
	if err != nil {
		panic(fmt.Errorf("failed to print block #%d to Firehose output: %w", out.block.Number, err))
	}

	t.flushToFirehose(line)
}

// render serializes the block into a FIRE BLOCK line. The returned slice aliases the
// receiver's buffers and is only valid until the next call to render.
//
// Output format (one line):
//
//	FIRE BLOCK <block_num> <flash_block_idx> <block_hash> <prev_num> <prev_hash> <lib_num> <timestamp_unix_nano> <payload_base64>
//
// flash_block_idx is 0 for non-flash blocks; for flash blocks it is the current
// flash block index plus 1000 when this is the final iteration for the block.
func (b *blockLineBuffers) render(out *blockOutput) ([]byte, error) {
	block := out.block

	size := block.SizeVT()
	b.payload = slices.Grow(b.payload[:0], size)[:size]
	n, err := block.MarshalToSizedBufferVT(b.payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal block: %w", err)
	}
	payload := b.payload[len(b.payload)-n:]

	previousNum := uint64(0)
	if block.Number > 0 {
		previousNum = block.Number - 1
	}

	// Header fields are small; 2*32 bytes of hashes plus 5 numbers fit well under 256 bytes.
	line := slices.Grow(b.line[:0], 256+base64.StdEncoding.EncodedLen(len(payload))+1)
	line = append(line, "FIRE BLOCK "...)
	line = strconv.AppendUint(line, block.Number, 10)
	line = append(line, ' ')
	line = strconv.AppendUint(line, out.printedFlashBlockIndex, 10)
	line = append(line, ' ')
	line = hex.AppendEncode(line, block.Hash)
	line = append(line, ' ')
	line = strconv.AppendUint(line, previousNum, 10)
	line = append(line, ' ')
	line = hex.AppendEncode(line, block.Header.ParentHash)
	line = append(line, ' ')
	line = strconv.AppendUint(line, out.libNum, 10)
	line = append(line, ' ')
	line = strconv.AppendInt(line, block.MustTime().UnixNano(), 10)
	// **Important** The space separating the header from the payload is mandatory.
	line = append(line, ' ')
	line = base64.StdEncoding.AppendEncode(line, payload)
	line = append(line, '\n')

	b.line = line
	return line, nil
}

// computeLibNum computes the last irreversible block number to advertise for the
// given block using the current finality status. It mirrors the Optimism Geth
// firehose tracer logic:
//   - When finality is known, use LastFinalizedBlock.
//   - When finality is empty, fall back to max(blockNumber-200, 0).
//   - In all cases, never let libNum fall more than 200 blocks behind blockNumber.
//   - In all cases, never let libNum be greater than blockNumber.
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

	// Cap: libNum must never be ahead of blockNumber, which happens when a known block
	// is replayed (e.g. OnSkippedBlock during sync) while finality is already past it.
	if libNum > blockNumber {
		libNum = blockNumber
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
