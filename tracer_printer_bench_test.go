package firehose

import (
	"encoding/binary"
	"fmt"
	"io"
	"testing"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// benchBytes returns a deterministic byte slice of the given length derived from seed.
func benchBytes(seed uint64, length int) []byte {
	out := make([]byte, length)
	for i := 0; i < length; i += 8 {
		var chunk [8]byte
		binary.LittleEndian.PutUint64(chunk[:], seed*0x9E3779B97F4A7C15+uint64(i))
		copy(out[i:], chunk[:])
	}
	return out
}

func benchBigInt(seed uint64) *pbeth.BigInt {
	return &pbeth.BigInt{Bytes: benchBytes(seed, 16)}
}

// newBenchBlock builds a fully traced block with trxCount transactions, each with a
// root call and a few nested calls carrying storage, balance, nonce, gas changes and logs.
func newBenchBlock(trxCount int) *pbeth.Block {
	block := &pbeth.Block{
		Hash:   benchBytes(1, 32),
		Number: 21_000_000,
		Size:   150_000,
		Ver:    4,
		Header: &pbeth.BlockHeader{
			ParentHash:       benchBytes(2, 32),
			UncleHash:        benchBytes(3, 32),
			Coinbase:         benchBytes(4, 20),
			StateRoot:        benchBytes(5, 32),
			TransactionsRoot: benchBytes(6, 32),
			ReceiptRoot:      benchBytes(7, 32),
			LogsBloom:        benchBytes(8, 256),
			Difficulty:       benchBigInt(9),
			Number:           21_000_000,
			GasLimit:         30_000_000,
			GasUsed:          29_000_000,
			Timestamp:        timestamppb.New(timestamppb.Now().AsTime().Truncate(1e9)),
			ExtraData:        benchBytes(10, 32),
			MixHash:          benchBytes(11, 32),
			BaseFeePerGas:    benchBigInt(12),
		},
		DetailLevel: pbeth.Block_DETAILLEVEL_EXTENDED,
	}

	ordinal := uint64(0)
	nextOrdinal := func() uint64 { ordinal++; return ordinal }

	for i := 0; i < trxCount; i++ {
		seed := uint64(i+1) * 1000
		trx := &pbeth.TransactionTrace{
			To:                   benchBytes(seed+1, 20),
			Nonce:                uint64(i),
			GasPrice:             benchBigInt(seed + 2),
			GasLimit:             200_000,
			Value:                benchBigInt(seed + 3),
			Input:                benchBytes(seed+4, 260),
			V:                    benchBytes(seed+5, 1),
			R:                    benchBytes(seed+6, 32),
			S:                    benchBytes(seed+7, 32),
			GasUsed:              150_000,
			Type:                 pbeth.TransactionTrace_TRX_TYPE_DYNAMIC_FEE,
			MaxFeePerGas:         benchBigInt(seed + 8),
			MaxPriorityFeePerGas: benchBigInt(seed + 9),
			Index:                uint32(i),
			Hash:                 benchBytes(seed+10, 32),
			From:                 benchBytes(seed+11, 20),
			BeginOrdinal:         nextOrdinal(),
			Status:               pbeth.TransactionTraceStatus_SUCCEEDED,
		}

		receipt := &pbeth.TransactionReceipt{
			StateRoot:         nil,
			CumulativeGasUsed: uint64(i) * 150_000,
			LogsBloom:         benchBytes(seed+12, 256),
		}

		for c := 0; c < 4; c++ {
			cseed := seed + uint64(c)*100
			call := &pbeth.Call{
				Index:        uint32(c + 1),
				ParentIndex:  uint32(c),
				Depth:        uint32(c),
				CallType:     pbeth.CallType_CALL,
				Caller:       benchBytes(cseed+20, 20),
				Address:      benchBytes(cseed+21, 20),
				Value:        benchBigInt(cseed + 22),
				GasLimit:     180_000,
				GasConsumed:  40_000,
				ReturnData:   benchBytes(cseed+23, 64),
				Input:        benchBytes(cseed+24, 196),
				ExecutedCode: true,
				BeginOrdinal: nextOrdinal(),
				KeccakPreimages: map[string]string{
					fmt.Sprintf("%x", benchBytes(cseed+25, 32)): fmt.Sprintf("%x", benchBytes(cseed+26, 64)),
					fmt.Sprintf("%x", benchBytes(cseed+27, 32)): fmt.Sprintf("%x", benchBytes(cseed+28, 64)),
				},
			}

			for s := 0; s < 4; s++ {
				call.StorageChanges = append(call.StorageChanges, &pbeth.StorageChange{
					Address:  call.Address,
					Key:      benchBytes(cseed+30+uint64(s), 32),
					OldValue: benchBytes(cseed+40+uint64(s), 32),
					NewValue: benchBytes(cseed+50+uint64(s), 32),
					Ordinal:  nextOrdinal(),
				})
			}

			for b := 0; b < 2; b++ {
				call.BalanceChanges = append(call.BalanceChanges, &pbeth.BalanceChange{
					Address:  benchBytes(cseed+60+uint64(b), 20),
					OldValue: benchBigInt(cseed + 62 + uint64(b)),
					NewValue: benchBigInt(cseed + 64 + uint64(b)),
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
					Topics:     [][]byte{benchBytes(cseed+70, 32), benchBytes(cseed+71, 32), benchBytes(cseed+72, 32)},
					Data:       benchBytes(cseed+73+uint64(l), 96),
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

func BenchmarkWriteBlock(b *testing.B) {
	for _, trxCount := range []int{10, 200, 1000} {
		block := newBenchBlock(trxCount)
		out := &blockOutput{block: block, libNum: block.Number - 10}

		b.Run(fmt.Sprintf("trx_%d", trxCount), func(b *testing.B) {
			tracer := NewTracer(&Config{OutputWriter: io.Discard})

			line, err := new(blockLineBuffers).render(out)
			if err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(len(line)))
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				tracer.writeBlock(out)
			}
		})
	}
}
