package tests

import (
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
)

// TestTracer_OnLog tests all log emission scenarios
func TestTracer_OnLog(t *testing.T) {
	t.Run("log_with_0_topics", func(t *testing.T) {
		// Log with no topics (anonymous event)
		data := []byte{0x01, 0x02, 0x03, 0x04}
		var topics [][32]byte // Empty topics

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			Log(BobAddr, topics, data, 0).
			EndCall([]byte{}, 90000, nil).
			EndBlockTrx(receiptWithLogs(100000, []firehose.LogData{
				{Address: BobAddr, Topics: topics, Data: data},
			}), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 1, len(call.Logs))
				log := call.Logs[0]
				assert.Equal(t, BobAddr[:], log.Address)
				assert.Equal(t, 0, len(log.Topics))
				assert.Equal(t, data, log.Data)
				assert.Equal(t, uint32(0), log.Index)
				assert.Equal(t, uint32(0), log.BlockIndex)
			})
	})

	t.Run("log_with_1_topic", func(t *testing.T) {
		// Log with 1 topic (common for simple events)
		data := []byte{0x01, 0x02}
		topics := [][32]byte{hash32(100)}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			Log(BobAddr, topics, data, 0).
			EndCall([]byte{}, 90000, nil).
			EndBlockTrx(receiptWithLogs(100000, []firehose.LogData{
				{Address: BobAddr, Topics: topics, Data: data},
			}), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 1, len(call.Logs))
				log := call.Logs[0]
				assert.Equal(t, 1, len(log.Topics))
				assert.Equal(t, topics[0][:], log.Topics[0])
			})
	})

	t.Run("log_with_2_topics", func(t *testing.T) {
		// Log with 2 topics (indexed event with 1 indexed parameter)
		data := []byte{0x01, 0x02}
		topics := [][32]byte{hash32(100), hash32(200)}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			Log(BobAddr, topics, data, 0).
			EndCall([]byte{}, 90000, nil).
			EndBlockTrx(receiptWithLogs(100000, []firehose.LogData{
				{Address: BobAddr, Topics: topics, Data: data},
			}), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 1, len(call.Logs))
				log := call.Logs[0]
				assert.Equal(t, 2, len(log.Topics))
				assert.Equal(t, topics[0][:], log.Topics[0])
				assert.Equal(t, topics[1][:], log.Topics[1])
			})
	})

	t.Run("log_with_3_topics", func(t *testing.T) {
		// Log with 3 topics (indexed event with 2 indexed parameters)
		data := []byte{0x01, 0x02}
		topics := [][32]byte{hash32(100), hash32(200), hash32(300)}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			Log(BobAddr, topics, data, 0).
			EndCall([]byte{}, 90000, nil).
			EndBlockTrx(receiptWithLogs(100000, []firehose.LogData{
				{Address: BobAddr, Topics: topics, Data: data},
			}), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 1, len(call.Logs))
				log := call.Logs[0]
				assert.Equal(t, 3, len(log.Topics))
				assert.Equal(t, topics[0][:], log.Topics[0])
				assert.Equal(t, topics[1][:], log.Topics[1])
				assert.Equal(t, topics[2][:], log.Topics[2])
			})
	})

	t.Run("log_with_4_topics", func(t *testing.T) {
		// Log with 4 topics (maximum - indexed event with 3 indexed parameters)
		data := []byte{0x01, 0x02}
		topics := [][32]byte{hash32(100), hash32(200), hash32(300), hash32(400)}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			Log(BobAddr, topics, data, 0).
			EndCall([]byte{}, 90000, nil).
			EndBlockTrx(receiptWithLogs(100000, []firehose.LogData{
				{Address: BobAddr, Topics: topics, Data: data},
			}), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 1, len(call.Logs))
				log := call.Logs[0]
				assert.Equal(t, 4, len(log.Topics))
				assert.Equal(t, topics[0][:], log.Topics[0])
				assert.Equal(t, topics[1][:], log.Topics[1])
				assert.Equal(t, topics[2][:], log.Topics[2])
				assert.Equal(t, topics[3][:], log.Topics[3])
			})
	})

	t.Run("multiple_logs_per_call", func(t *testing.T) {
		// Multiple logs in same call
		data1 := []byte{0x01}
		data2 := []byte{0x02}
		data3 := []byte{0x03}
		topics1 := [][32]byte{hash32(100)}
		topics2 := [][32]byte{hash32(200)}
		topics3 := [][32]byte{hash32(300)}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			Log(BobAddr, topics1, data1, 0).
			Log(BobAddr, topics2, data2, 1).
			Log(BobAddr, topics3, data3, 2).
			EndCall([]byte{}, 90000, nil).
			EndBlockTrx(receiptWithLogs(100000, []firehose.LogData{
				{Address: BobAddr, Topics: topics1, Data: data1},
				{Address: BobAddr, Topics: topics2, Data: data2},
				{Address: BobAddr, Topics: topics3, Data: data3},
			}), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 3, len(call.Logs), "Should have 3 logs")

				// Verify index incrementing
				assert.Equal(t, uint32(0), call.Logs[0].Index)
				assert.Equal(t, uint32(1), call.Logs[1].Index)
				assert.Equal(t, uint32(2), call.Logs[2].Index)

				// Verify ordinals are increasing
				assert.True(t, call.Logs[0].Ordinal < call.Logs[1].Ordinal)
				assert.True(t, call.Logs[1].Ordinal < call.Logs[2].Ordinal)
			})
	})

	t.Run("logs_across_multiple_calls", func(t *testing.T) {
		// Logs in nested calls
		data1 := []byte{0x01}
		data2 := []byte{0x02}
		topics1 := [][32]byte{hash32(100)}
		topics2 := [][32]byte{hash32(200)}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 200000, []byte{}).
			Log(BobAddr, topics1, data1, 0).
			StartCallRaw(1, byte(firehose.CallTypeCall), BobAddr, CharlieAddr, []byte{}, 100000, bigInt(0)).
			Log(CharlieAddr, topics2, data2, 1).
			EndCall([]byte{}, 90000, nil).
			EndCall([]byte{}, 180000, nil).
			EndBlockTrx(receiptWithLogs(200000, []firehose.LogData{
				{Address: BobAddr, Topics: topics1, Data: data1},
				{Address: CharlieAddr, Topics: topics2, Data: data2},
			}), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]

				// Root call has 1 log
				assert.Equal(t, 1, len(trx.Calls[0].Logs))
				assert.Equal(t, uint32(0), trx.Calls[0].Logs[0].Index)
				assert.Equal(t, uint32(0), trx.Calls[0].Logs[0].BlockIndex)

				// Nested call has 1 log
				assert.Equal(t, 1, len(trx.Calls[1].Logs))
				assert.Equal(t, uint32(1), trx.Calls[1].Logs[0].Index)
				assert.Equal(t, uint32(1), trx.Calls[1].Logs[0].BlockIndex)
			})
	})

	t.Run("log_with_empty_data", func(t *testing.T) {
		// Log with no data (only topics)
		var emptyData []byte
		topics := [][32]byte{hash32(100)}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			Log(BobAddr, topics, emptyData, 0).
			EndCall([]byte{}, 90000, nil).
			EndBlockTrx(receiptWithLogs(100000, []firehose.LogData{
				{Address: BobAddr, Topics: topics, Data: emptyData},
			}), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 1, len(call.Logs))
				log := call.Logs[0]
				assert.Equal(t, 1, len(log.Topics))
				assert.True(t, len(log.Data) == 0)
			})
	})

	t.Run("log_with_large_data", func(t *testing.T) {
		// Log with large data payload
		largeData := make([]byte, 1024)
		for i := range largeData {
			largeData[i] = byte(i % 256)
		}
		topics := [][32]byte{hash32(100)}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			Log(BobAddr, topics, largeData, 0).
			EndCall([]byte{}, 90000, nil).
			EndBlockTrx(receiptWithLogs(100000, []firehose.LogData{
				{Address: BobAddr, Topics: topics, Data: largeData},
			}), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 1, len(call.Logs))
				log := call.Logs[0]
				assert.Equal(t, largeData, log.Data)
				assert.Equal(t, 1024, len(log.Data))
			})
	})

	t.Run("log_topic_conversion", func(t *testing.T) {
		// Verify topic bytes are correctly converted
		topic1 := [32]byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0}
		topic2 := [32]byte{0xff, 0xee, 0xdd, 0xcc, 0xbb, 0xaa, 0x99, 0x88}
		topics := [][32]byte{topic1, topic2}
		data := []byte{0x01}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			Log(BobAddr, topics, data, 0).
			EndCall([]byte{}, 90000, nil).
			EndBlockTrx(receiptWithLogs(100000, []firehose.LogData{
				{Address: BobAddr, Topics: topics, Data: data},
			}), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 1, len(call.Logs))
				log := call.Logs[0]
				assert.Equal(t, 2, len(log.Topics))

				// Verify exact byte conversion
				assert.Equal(t, topic1[:], log.Topics[0])
				assert.Equal(t, topic2[:], log.Topics[1])
			})
	})
}
