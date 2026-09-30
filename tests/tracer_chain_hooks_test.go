package tests

import (
	"bytes"
	"errors"
	"io"
	"math/big"
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go/v5"
	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTracer_BlockNumberForkActivation(t *testing.T) {
	delegationCode := append([]byte{0xef, 0x01, 0x00}, CharlieAddr[:]...)

	run := func(t *testing.T, pragueBlock int64) *pbeth.Call {
		var rootCall *pbeth.Call
		newTracerTesterWithConfig(t, &firehose.ChainConfig{
			ChainID:     big.NewInt(1),
			PragueBlock: big.NewInt(pragueBlock),
		}).
			SetMockStateCode(AliceAddr, delegationCode).
			StartBlockTrx(TestLegacyTrx).
			StartCall(BobAddr, AliceAddr, bigInt(0), 21000, nil).
			EndCall(nil, 21000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				rootCall = block.TransactionTraces[0].Calls[0]
			})
		return rootCall
	}

	t.Run("active_at_fork_block", func(t *testing.T) {
		call := run(t, 100) // TestBlock is #100
		assert.Equal(t, CharlieAddr[:], call.AddressDelegatesTo)
	})

	t.Run("inactive_before_fork_block", func(t *testing.T) {
		call := run(t, 101)
		assert.Nil(t, call.AddressDelegatesTo)
	})
}

var neverRevertedTopic = topic("never_reverted")

func isNeverRevertedTestLog(log *pbeth.Log) bool {
	return len(log.Topics) > 0 && bytes.Equal(log.Topics[0], neverRevertedTopic[:])
}

func newChainHooksTester(t *testing.T, config *firehose.Config) *TracerTester {
	config.ChainConfig = &firehose.ChainConfig{ChainID: big.NewInt(1)}
	return newTracerTesterWithFullConfig(t, config)
}

func TestTracer_LogOutsideCall(t *testing.T) {
	t.Run("log_after_root_call_is_attached_to_root_call", func(t *testing.T) {
		newChainHooksTester(t, &firehose.Config{}).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 21000, nil).
			Log(BobAddr, [][32]byte{topic("inside")}, nil, 0).
			EndCall(nil, 21000).
			Log(CharlieAddr, [][32]byte{topic("after")}, nil, 1).
			EndBlockTrx(receiptWithLogs(21000, []firehose.LogData{
				log1(BobAddr, topic("inside"), nil),
				log1(CharlieAddr, topic("after"), nil),
			}), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				rootCall := trx.Calls[0]

				require.Len(t, rootCall.Logs, 2)
				assert.Equal(t, CharlieAddr[:], rootCall.Logs[1].Address)
				assert.Equal(t, uint32(1), rootCall.Logs[1].Index)
				assert.Greater(t, rootCall.Logs[1].Ordinal, rootCall.EndOrdinal)

				require.Len(t, trx.Receipt.Logs, 2)
				assert.Equal(t, rootCall.Logs[1].Ordinal, trx.Receipt.Logs[1].Ordinal)
			})
	})

	t.Run("log_in_transaction_without_calls_panics", func(t *testing.T) {
		tester := newChainHooksTester(t, &firehose.Config{}).
			StartBlockTrx(TestLegacyTrx).
			Log(CharlieAddr, [][32]byte{topic("orphan")}, nil, 0)

		assert.Panics(t, func() {
			tester.EndTrx(receiptWithLogs(21000, []firehose.LogData{
				log1(CharlieAddr, topic("orphan"), nil),
			}), nil)
		})
	})

	t.Run("log_in_system_call_outside_call_panics", func(t *testing.T) {
		tester := newChainHooksTester(t, &firehose.Config{}).
			StartBlock().
			StartSystemCall().
			Log(CharlieAddr, [][32]byte{topic("orphan")}, nil, 0)

		assert.Panics(t, func() {
			tester.EndSystemCall()
		})
	})
}

func TestTracer_IsNeverRevertedLog(t *testing.T) {
	reverted := errors.New(firehose.TextExecutionRevertedErr)

	newChainHooksTester(t, &firehose.Config{
		IsNeverRevertedLog: isNeverRevertedTestLog,
	}).
		StartBlockTrx(TestLegacyTrx).
		StartCall(AliceAddr, BobAddr, bigInt(0), 21000, nil).
		Log(BobAddr, [][32]byte{neverRevertedTopic}, nil, 0).
		Log(BobAddr, [][32]byte{topic("reverted")}, nil, 5).
		EndCallFailed(nil, 21000, reverted, true).
		Log(CharlieAddr, [][32]byte{neverRevertedTopic}, nil, 1).
		EndBlockTrx(failedReceiptWithLogs(21000, []firehose.LogData{
			log1(BobAddr, neverRevertedTopic, nil),
			log1(CharlieAddr, neverRevertedTopic, nil),
		}), nil, nil).
		Validate(func(block *pbeth.Block) {
			trx := block.TransactionTraces[0]
			rootCall := trx.Calls[0]
			require.True(t, rootCall.StateReverted)
			require.Len(t, rootCall.Logs, 3)

			assert.Equal(t, uint32(0), rootCall.Logs[0].BlockIndex)
			assert.Equal(t, uint32(0), rootCall.Logs[1].BlockIndex, "reverted log loses its block index")
			assert.Equal(t, uint32(1), rootCall.Logs[2].BlockIndex, "never reverted log keeps its block index")

			require.Len(t, trx.Receipt.Logs, 2)
			assert.Equal(t, rootCall.Logs[0].Ordinal, trx.Receipt.Logs[0].Ordinal)
			assert.Equal(t, rootCall.Logs[2].Ordinal, trx.Receipt.Logs[1].Ordinal)
			assert.Equal(t, rootCall.Logs[2].Index, trx.Receipt.Logs[1].Index)
		})
}

func TestTracer_BeforeBlockFlush(t *testing.T) {
	addedHash := hash32(42)
	var seen *pbeth.Block
	newChainHooksTester(t, &firehose.Config{
		BeforeBlockFlush: func(block *pbeth.Block) {
			seen = block
			block.TransactionTraces = append(block.TransactionTraces, &pbeth.TransactionTrace{Hash: addedHash[:], Index: 1})
		},
	}).
		StartBlockTrx(TestLegacyTrx).
		StartCall(AliceAddr, BobAddr, bigInt(0), 21000, nil).
		EndCall(nil, 21000).
		EndBlockTrx(successReceipt(21000), nil, nil).
		Validate(func(block *pbeth.Block) {
			require.NotNil(t, seen)
			require.Len(t, block.TransactionTraces, 2)
			assert.Equal(t, addedHash[:], block.TransactionTraces[1].Hash)
		})
}

// shortWriter writes at most chunk bytes per call and fails once failAfter calls are made (0 = never)
type shortWriter struct {
	out       bytes.Buffer
	chunk     int
	calls     int
	failAfter int
}

func (w *shortWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.failAfter > 0 && w.calls > w.failAfter {
		return 0, io.ErrClosedPipe
	}

	n := min(len(p), w.chunk)
	w.out.Write(p[:n])
	if n < len(p) {
		return n, io.ErrShortWrite
	}
	return n, nil
}

func TestTracer_OutputWriteFailures(t *testing.T) {
	t.Run("short_writes_are_retried", func(t *testing.T) {
		writer := &shortWriter{chunk: 64}
		tracer := firehose.NewTracer(&firehose.Config{OutputWriter: writer})
		tracer.OnBlockchainInit("test", "1.0.0", &firehose.ChainConfig{ChainID: big.NewInt(1)}, nil)
		assert.Equal(t, "FIRE INIT 3.1 firehose-evm-tracer/test 1.0.0\n", writer.out.String())
	})

	t.Run("persistent_failure_panics", func(t *testing.T) {
		writer := &shortWriter{chunk: 1 << 20, failAfter: 1}
		tracer := firehose.NewTracer(&firehose.Config{OutputWriter: writer})
		tracer.OnBlockchainInit("test", "1.0.0", &firehose.ChainConfig{ChainID: big.NewInt(1)}, nil)
		tracer.OnBlockStart(TestBlock)

		assert.Panics(t, func() { tracer.OnBlockEnd(nil) })
	})
}
