package tests

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go/v4"
	"github.com/stretchr/testify/require"
)

// libBlockEvent builds a minimal-but-valid block event for LIB testing. When
// finalized is non-nil it is reported as the block's finalized (LIB) reference.
func libBlockEvent(number uint64, finalized *firehose.FinalizedBlockRef) firehose.BlockEvent {
	return firehose.BlockEvent{
		Block: firehose.BlockData{
			Number:      number,
			Hash:        hash32(number),
			ParentHash:  hash32(number - 1),
			UncleHash:   hash32(3),
			Coinbase:    AliceAddr,
			Root:        hash32(4),
			TxHash:      hash32(5),
			ReceiptHash: hash32(6),
			Bloom:       make([]byte, 256),
			Difficulty:  bigInt(0),
			GasLimit:    15_000_000,
			Time:        1000,
			Extra:       []byte{},
			MixDigest:   hash32(7),
			BaseFee:     bigInt(1_000_000_000),
			Size:        1024,
		},
		Finalized: finalized,
	}
}

// firehoseLibNums extracts the lib_num field from every "FIRE BLOCK" line in the
// given output buffer, in order. The header layout is:
//
//	FIRE BLOCK <num> <hash> <parent_num> <parent_hash> <lib_num> <timestamp> <payload>
func firehoseLibNums(t *testing.T, buf *bytes.Buffer) []uint64 {
	t.Helper()

	var libs []uint64
	for _, line := range strings.Split(buf.String(), "\n") {
		if !strings.HasPrefix(line, "FIRE BLOCK ") {
			continue
		}

		parts := strings.SplitN(line, " ", 9)
		require.GreaterOrEqual(t, len(parts), 9, "FIRE BLOCK line should have 9 fields: %q", line)

		lib, err := strconv.ParseUint(parts[6], 10, 64)
		require.NoError(t, err, "parsing lib_num from %q", line)
		libs = append(libs, lib)
	}

	return libs
}

// TestTracer_LibNum guards the LIB ("last irreversible block") value emitted in
// the FIRE BLOCK header. A regression here (e.g. a hardcoded "0") silently
// breaks downstream Firehose finality, and is invisible to the protobuf golden
// tests because the LIB lives only in the header line, not in the block payload.
func TestTracer_LibNum(t *testing.T) {
	t.Run("uses finalized block number when known", func(t *testing.T) {
		tester := NewTracerTester(t)
		finalized := &firehose.FinalizedBlockRef{Number: 123, Hash: hash32(123)}

		tester.tracer.OnBlockStart(libBlockEvent(500, finalized))
		tester.tracer.OnBlockEnd(nil)

		require.Equal(t, []uint64{123}, firehoseLibNums(t, tester.tracer.GetTestingOutputBuffer()))
	})

	t.Run("falls back to block-200 when finality is unknown", func(t *testing.T) {
		tester := NewTracerTester(t)

		tester.tracer.OnBlockStart(libBlockEvent(500, nil))
		tester.tracer.OnBlockEnd(nil)

		require.Equal(t, []uint64{300}, firehoseLibNums(t, tester.tracer.GetTestingOutputBuffer()))
	})

	t.Run("fallback clamps to 0 for early blocks", func(t *testing.T) {
		tester := NewTracerTester(t)

		tester.tracer.OnBlockStart(libBlockEvent(100, nil))
		tester.tracer.OnBlockEnd(nil)

		require.Equal(t, []uint64{0}, firehoseLibNums(t, tester.tracer.GetTestingOutputBuffer()))
	})

	t.Run("concurrent flushing preserves per-block LIB", func(t *testing.T) {
		// The concurrent path captures the LIB at enqueue time because the
		// tracer resets its finality state before the flush worker runs.
		outputBuffer := &bytes.Buffer{}
		chainConfig := &firehose.ChainConfig{ChainID: bigInt(1)}
		tracer := firehose.NewTracer(&firehose.Config{
			ChainConfig:              chainConfig,
			EnableConcurrentFlushing: true,
			ConcurrentBufferSize:     4,
			OutputWriter:             outputBuffer,
		})
		tracer.OnBlockchainInit("test", "1.0.0", chainConfig, nil)

		// Each block reports a distinct finalized number; if the worker read a
		// shared/reset finality field instead of the captured value, these would
		// collapse to a wrong/stale LIB.
		for _, n := range []uint64{500, 501, 502} {
			finalized := &firehose.FinalizedBlockRef{Number: n - 10, Hash: hash32(n - 10)}
			tracer.OnBlockStart(libBlockEvent(n, finalized))
			tracer.OnBlockEnd(nil)
		}
		tracer.OnClose()

		require.Equal(t, []uint64{490, 491, 492}, firehoseLibNums(t, outputBuffer))
	})
}
