package firehose

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
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
		block := newTracedTestBlock(trxCount)
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
		block := newTracedTestBlock(trxCount)

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

// testBytes returns a deterministic byte slice of the given length derived from seed.
func testBytes(seed uint64, length int) []byte {
	out := make([]byte, length)
	for i := 0; i < length; i += 8 {
		var chunk [8]byte
		binary.LittleEndian.PutUint64(chunk[:], seed*0x9E3779B97F4A7C15+uint64(i))
		copy(out[i:], chunk[:])
	}
	return out
}

func testBigInt(seed uint64) *pbeth.BigInt {
	return &pbeth.BigInt{Bytes: testBytes(seed, 16)}
}

// newTracedTestBlock builds a fully traced block with trxCount transactions, each with a
// root call and a few nested calls carrying storage, balance, nonce, gas changes and logs.
func newTracedTestBlock(trxCount int) *pbeth.Block {
	block := &pbeth.Block{
		Hash:   testBytes(1, 32),
		Number: 21_000_000,
		Size:   150_000,
		Ver:    4,
		Header: &pbeth.BlockHeader{
			ParentHash:       testBytes(2, 32),
			UncleHash:        testBytes(3, 32),
			Coinbase:         testBytes(4, 20),
			StateRoot:        testBytes(5, 32),
			TransactionsRoot: testBytes(6, 32),
			ReceiptRoot:      testBytes(7, 32),
			LogsBloom:        testBytes(8, 256),
			Difficulty:       testBigInt(9),
			Number:           21_000_000,
			GasLimit:         30_000_000,
			GasUsed:          29_000_000,
			Timestamp:        timestamppb.New(timestamppb.Now().AsTime().Truncate(1e9)),
			ExtraData:        testBytes(10, 32),
			MixHash:          testBytes(11, 32),
			BaseFeePerGas:    testBigInt(12),
		},
		DetailLevel: pbeth.Block_DETAILLEVEL_EXTENDED,
	}

	ordinal := uint64(0)
	nextOrdinal := func() uint64 { ordinal++; return ordinal }

	for i := 0; i < trxCount; i++ {
		seed := uint64(i+1) * 1000
		trx := &pbeth.TransactionTrace{
			To:                   testBytes(seed+1, 20),
			Nonce:                uint64(i),
			GasPrice:             testBigInt(seed + 2),
			GasLimit:             200_000,
			Value:                testBigInt(seed + 3),
			Input:                testBytes(seed+4, 260),
			V:                    testBytes(seed+5, 1),
			R:                    testBytes(seed+6, 32),
			S:                    testBytes(seed+7, 32),
			GasUsed:              150_000,
			Type:                 pbeth.TransactionTrace_TRX_TYPE_DYNAMIC_FEE,
			MaxFeePerGas:         testBigInt(seed + 8),
			MaxPriorityFeePerGas: testBigInt(seed + 9),
			Index:                uint32(i),
			Hash:                 testBytes(seed+10, 32),
			From:                 testBytes(seed+11, 20),
			BeginOrdinal:         nextOrdinal(),
			Status:               pbeth.TransactionTraceStatus_SUCCEEDED,
		}

		receipt := &pbeth.TransactionReceipt{
			StateRoot:         nil,
			CumulativeGasUsed: uint64(i) * 150_000,
			LogsBloom:         testBytes(seed+12, 256),
		}

		for c := 0; c < 4; c++ {
			cseed := seed + uint64(c)*100
			call := &pbeth.Call{
				Index:        uint32(c + 1),
				ParentIndex:  uint32(c),
				Depth:        uint32(c),
				CallType:     pbeth.CallType_CALL,
				Caller:       testBytes(cseed+20, 20),
				Address:      testBytes(cseed+21, 20),
				Value:        testBigInt(cseed + 22),
				GasLimit:     180_000,
				GasConsumed:  40_000,
				ReturnData:   testBytes(cseed+23, 64),
				Input:        testBytes(cseed+24, 196),
				ExecutedCode: true,
				BeginOrdinal: nextOrdinal(),
				KeccakPreimages: map[string]string{
					fmt.Sprintf("%x", testBytes(cseed+25, 32)): fmt.Sprintf("%x", testBytes(cseed+26, 64)),
					fmt.Sprintf("%x", testBytes(cseed+27, 32)): fmt.Sprintf("%x", testBytes(cseed+28, 64)),
				},
			}

			for s := 0; s < 4; s++ {
				call.StorageChanges = append(call.StorageChanges, &pbeth.StorageChange{
					Address:  call.Address,
					Key:      testBytes(cseed+30+uint64(s), 32),
					OldValue: testBytes(cseed+40+uint64(s), 32),
					NewValue: testBytes(cseed+50+uint64(s), 32),
					Ordinal:  nextOrdinal(),
				})
			}

			for b := 0; b < 2; b++ {
				call.BalanceChanges = append(call.BalanceChanges, &pbeth.BalanceChange{
					Address:  testBytes(cseed+60+uint64(b), 20),
					OldValue: testBigInt(cseed + 62 + uint64(b)),
					NewValue: testBigInt(cseed + 64 + uint64(b)),
					Reason:   pbeth.BalanceChange_REASON_TRANSFER,
					Ordinal:  nextOrdinal(),
				})
			}

			call.NonceChanges = append(call.NonceChanges, &pbeth.NonceChange{
				Address:  call.Caller,
				OldValue: uint64(i),
				NewValue: uint64(i) + 1,
				Ordinal:  nextOrdinal(),
			})

			call.GasChanges = append(call.GasChanges, &pbeth.GasChange{
				OldValue: 180_000,
				NewValue: 140_000,
				Reason:   pbeth.GasChange_REASON_CALL,
				Ordinal:  nextOrdinal(),
			})

			for l := 0; l < 2; l++ {
				log := &pbeth.Log{
					Address:    call.Address,
					Topics:     [][]byte{testBytes(cseed+70, 32), testBytes(cseed+71, 32), testBytes(cseed+72, 32)},
					Data:       testBytes(cseed+73+uint64(l), 96),
					Index:      uint32(len(receipt.Logs)),
					BlockIndex: uint32(i*8 + len(receipt.Logs)),
					Ordinal:    nextOrdinal(),
				}
				call.Logs = append(call.Logs, log)
				receipt.Logs = append(receipt.Logs, log)
			}

			call.EndOrdinal = nextOrdinal()
			trx.Calls = append(trx.Calls, call)
		}

		trx.Receipt = receipt
		trx.EndOrdinal = nextOrdinal()
		block.TransactionTraces = append(block.TransactionTraces, trx)
	}

	return block
}
