package firehose

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
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

		// --- Finality is ahead of the block (replayed block): capped to the block ---
		{
			name:        "finalized_ahead_of_block",
			blockNumber: 500,
			finalized:   600,
			expected:    500,
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

// referenceBlockLine renders a FIRE BLOCK line with proto.Marshal, fmt and the standard
// base64 encoder, as the output format is specified.
func referenceBlockLine(t *testing.T, out *blockOutput) string {
	block := out.block
	marshalled, err := proto.Marshal(block)
	require.NoError(t, err)

	return fmt.Sprintf("FIRE BLOCK %d %d %s %d %s %d %d %s\n",
		block.Number,
		out.printedFlashBlockIndex,
		block.ID(),
		block.Number-1,
		block.PreviousID(),
		out.libNum,
		block.MustTime().UnixNano(),
		base64.StdEncoding.EncodeToString(marshalled),
	)
}

func TestWriteBlockMatchesReferenceEncoding(t *testing.T) {
	var buffer bytes.Buffer
	tracer := NewTracer(&Config{OutputWriter: &buffer})

	for _, trxCount := range []int{0, 1, 7, 150} {
		block := newBenchBlock(trxCount)
		// Map fields are marshalled in random order, so they are checked separately below.
		for _, trx := range block.TransactionTraces {
			for _, call := range trx.Calls {
				call.KeccakPreimages = nil
			}
		}

		for _, out := range []*blockOutput{
			{block: block, libNum: block.Number - 10},
			{block: block, libNum: block.Number, printedFlashBlockIndex: 1003},
		} {
			buffer.Reset()
			tracer.writeBlock(out)
			require.Equal(t, referenceBlockLine(t, out), buffer.String(), "trx count %d", trxCount)
		}
	}
}

func TestWriteBlockRoundTrip(t *testing.T) {
	var buffer bytes.Buffer
	tracer := NewTracer(&Config{OutputWriter: &buffer})

	// Write a large block, then a smaller one to exercise buffer reuse.
	for _, trxCount := range []int{300, 3} {
		block := newBenchBlock(trxCount)

		buffer.Reset()
		tracer.writeBlock(&blockOutput{block: block, libNum: block.Number - 10})

		line := strings.TrimSuffix(buffer.String(), "\n")
		parts := strings.Split(line, " ")
		require.Len(t, parts, 10)

		payload, err := base64.StdEncoding.DecodeString(parts[9])
		require.NoError(t, err)

		decoded := &pbeth.Block{}
		require.NoError(t, proto.Unmarshal(payload, decoded))
		require.True(t, proto.Equal(block, decoded), "trx count %d", trxCount)
	}
}

// populateAllFields sets every field of msg, recursively, to a non-default value. Lists and
// maps get a single entry so that marshalling is deterministic.
func populateAllFields(msg protoreflect.Message, seed *int) {
	fields := msg.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		switch {
		case field.IsMap():
			entries := msg.Mutable(field).Map()
			entries.Set(sampleScalar(field.MapKey(), seed).MapKey(), sampleScalar(field.MapValue(), seed))
		case field.IsList():
			list := msg.Mutable(field).List()
			if field.Message() != nil {
				element := list.NewElement()
				populateAllFields(element.Message(), seed)
				list.Append(element)
			} else {
				list.Append(sampleScalar(field, seed))
			}
		case field.Message() != nil:
			populateAllFields(msg.Mutable(field).Message(), seed)
		default:
			msg.Set(field, sampleScalar(field, seed))
		}
	}
}

func sampleScalar(field protoreflect.FieldDescriptor, seed *int) protoreflect.Value {
	*seed++
	switch field.Kind() {
	case protoreflect.BoolKind:
		return protoreflect.ValueOfBool(true)
	case protoreflect.EnumKind:
		values := field.Enum().Values()
		return protoreflect.ValueOfEnum(values.Get(values.Len() - 1).Number())
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return protoreflect.ValueOfInt32(int32(*seed))
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return protoreflect.ValueOfInt64(int64(*seed))
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return protoreflect.ValueOfUint32(uint32(*seed))
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return protoreflect.ValueOfUint64(uint64(*seed))
	case protoreflect.FloatKind:
		return protoreflect.ValueOfFloat32(float32(*seed))
	case protoreflect.DoubleKind:
		return protoreflect.ValueOfFloat64(float64(*seed))
	case protoreflect.StringKind:
		return protoreflect.ValueOfString(fmt.Sprintf("value-%d", *seed))
	case protoreflect.BytesKind:
		return protoreflect.ValueOfBytes([]byte(fmt.Sprintf("bytes-%d", *seed)))
	}
	panic(fmt.Sprintf("unhandled kind %s for field %s", field.Kind(), field.FullName()))
}

// The FIRE BLOCK payload is produced by the vtproto marshaller, which is generated separately
// from the protobuf types and silently drops any field it does not know about.
func TestBlockMarshalVTCoversAllFields(t *testing.T) {
	block := &pbeth.Block{}
	seed := 0
	populateAllFields(block.ProtoReflect(), &seed)

	expected, err := proto.MarshalOptions{Deterministic: true}.Marshal(block)
	require.NoError(t, err)

	actual, err := block.MarshalVT()
	require.NoError(t, err)

	require.Equal(t, expected, actual, "vtproto marshalling of pbeth.Block differs from proto.Marshal, regenerate the vtproto code in firehose-ethereum/types")
}
