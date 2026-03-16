package tests

import (
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go/v5"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTracer_ReceiptAssignment tests receipt field assignment
func TestTracer_ReceiptAssignment(t *testing.T) {
	t.Run("receipt_fields_assigned", func(t *testing.T) {
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{0x01}).
			EndCall([]byte{0x42}, 20000).
			EndBlockTrx(receiptAt(5, 1, 21000, 100000, nil), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]

				assert.Equal(t, uint32(5), trx.Index, "Transaction index should match receipt")
				assert.Equal(t, uint64(21000), trx.GasUsed, "Gas used should match receipt")
				assert.Equal(t, pbeth.TransactionTraceStatus_SUCCEEDED, trx.Status, "Status should be SUCCEEDED for status=1")

				require.NotNil(t, trx.Receipt, "Receipt should be populated")
				assert.Equal(t, uint64(100000), trx.Receipt.CumulativeGasUsed, "Cumulative gas should match receipt")
			})
	})

	t.Run("receipt_status_failed", func(t *testing.T) {
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{0x01}).
			EndCall([]byte{}, 21000).
			EndBlockTrx(receiptAt(0, 0, 21000, 21000, nil), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				assert.Equal(t, pbeth.TransactionTraceStatus_FAILED, trx.Status, "Status should be FAILED for status=0")
			})
	})

	t.Run("receipt_status_reverted_overrides_success", func(t *testing.T) {
		// Even if receipt says succeeded (status=1), if root call reverted, transaction is REVERTED
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{0x01}).
			EndCallFailed([]byte("reverted"), 21000, testErrExecutionReverted, true).
			EndBlockTrx(receiptAt(0, 1, 21000, 21000, nil), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				assert.Equal(t, pbeth.TransactionTraceStatus_REVERTED, trx.Status, "Status should be REVERTED when root call reverts")
			})
	})
}

// TestTracer_LogOrdinalsAndIndexes tests log ordinal and index assignment
func TestTracer_LogOrdinalsAndIndexes(t *testing.T) {
	t.Run("logs_from_successful_call_assigned_to_receipt", func(t *testing.T) {
		// Scenario: Single call with 3 logs
		receiptLogs := []firehose.LogData{
			log1(BobAddr, topic("Transfer"), []byte{0x01}),
			log2(BobAddr, topic("Approval"), topic("Spender"), []byte{0x02}),
			log0(BobAddr, []byte{0x03}),
		}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			Log(BobAddr, [][32]byte{topic("Transfer")}, []byte{0x01}, 0).
			Log(BobAddr, [][32]byte{topic("Approval"), topic("Spender")}, []byte{0x02}, 1).
			Log(BobAddr, [][32]byte{}, []byte{0x03}, 2).
			EndCall([]byte{}, 95000).
			EndBlockTrx(receiptWithLogs(100000, receiptLogs), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Call should have 3 logs with ordinals and indexes
				require.Equal(t, 3, len(call.Logs), "Call should have 3 logs")
				assert.Greater(t, call.Logs[0].Ordinal, uint64(0), "Log 0 should have ordinal")
				assert.Greater(t, call.Logs[1].Ordinal, uint64(0), "Log 1 should have ordinal")
				assert.Greater(t, call.Logs[2].Ordinal, uint64(0), "Log 2 should have ordinal")
				assert.Equal(t, uint32(0), call.Logs[0].Index, "Log 0 should have index 0")
				assert.Equal(t, uint32(1), call.Logs[1].Index, "Log 1 should have index 1")
				assert.Equal(t, uint32(2), call.Logs[2].Index, "Log 2 should have index 2")

				// Receipt should have 3 logs with same ordinals and indexes
				require.NotNil(t, trx.Receipt, "Receipt should exist")
				require.Equal(t, 3, len(trx.Receipt.Logs), "Receipt should have 3 logs")

				for i := 0; i < 3; i++ {
					assert.Equal(t, call.Logs[i].Ordinal, trx.Receipt.Logs[i].Ordinal,
						"Receipt log %d ordinal should match call log", i)
					assert.Equal(t, call.Logs[i].Index, trx.Receipt.Logs[i].Index,
						"Receipt log %d index should match call log", i)
					assert.Equal(t, call.Logs[i].BlockIndex, trx.Receipt.Logs[i].BlockIndex,
						"Receipt log %d block index should match call log", i)
				}
			})
	})

	t.Run("logs_across_multiple_successful_calls", func(t *testing.T) {
		// Scenario: Root call logs 2, nested call logs 1, root call logs 1 more
		// Receipt should have all 4 logs in ordinal order
		receiptLogs := []firehose.LogData{
			log1(BobAddr, topic("Event1"), []byte{0x01}),
			log1(BobAddr, topic("Event2"), []byte{0x02}),
			log1(CharlieAddr, topic("Event3"), []byte{0x03}),
			log1(BobAddr, topic("Event4"), []byte{0x04}),
		}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			Log(BobAddr, [][32]byte{topic("Event1")}, []byte{0x01}, 0).
			Log(BobAddr, [][32]byte{topic("Event2")}, []byte{0x02}, 1).
			StartCall(BobAddr, CharlieAddr, bigInt(0), 50000, []byte{0x02}).
			Log(CharlieAddr, [][32]byte{topic("Event3")}, []byte{0x03}, 2).
			EndCall([]byte{}, 45000).
			Log(BobAddr, [][32]byte{topic("Event4")}, []byte{0x04}, 3).
			EndCall([]byte{}, 90000).
			EndBlockTrx(receiptWithLogs(100000, receiptLogs), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]

				// Collect all call logs
				var allCallLogs []*pbeth.Log
				for _, call := range trx.Calls {
					allCallLogs = append(allCallLogs, call.Logs...)
				}

				require.Equal(t, 4, len(allCallLogs), "Should have 4 call logs total")
				require.Equal(t, 4, len(trx.Receipt.Logs), "Should have 4 receipt logs")

				// Verify ordinals are in order
				for i := 1; i < len(trx.Receipt.Logs); i++ {
					assert.Greater(t, trx.Receipt.Logs[i].Ordinal, trx.Receipt.Logs[i-1].Ordinal,
						"Receipt log ordinals should be in order")
				}

				// Verify indexes are sequential
				for i := 0; i < len(trx.Receipt.Logs); i++ {
					assert.Equal(t, uint32(i), trx.Receipt.Logs[i].Index,
						"Receipt log %d should have index %d", i, i)
				}
			})
	})
}

// TestTracer_LogsInRevertedCalls tests log handling for reverted calls
func TestTracer_LogsInRevertedCalls(t *testing.T) {
	t.Run("logs_in_reverted_call_not_in_receipt", func(t *testing.T) {
		// Scenario: Root call succeeds with 1 log, nested call reverts with 1 log
		// Receipt should only have the 1 log from root call
		receiptLogs := []firehose.LogData{
			log1(BobAddr, topic("Success"), []byte{0x01}),
		}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			Log(BobAddr, [][32]byte{topic("Success")}, []byte{0x01}, 0).
			StartCall(BobAddr, CharlieAddr, bigInt(0), 50000, []byte{0x02}).
			Log(CharlieAddr, [][32]byte{topic("Reverted")}, []byte{0x02}, 1).
			EndCallFailed([]byte("error"), 45000, testErrExecutionReverted, true).
			EndCall([]byte{}, 90000).
			EndBlockTrx(receiptWithLogs(100000, receiptLogs), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				rootCall := trx.Calls[0]
				revertedCall := trx.Calls[1]

				// Root call should have 1 log
				require.Equal(t, 1, len(rootCall.Logs), "Root call should have 1 log")
				assert.False(t, rootCall.StateReverted, "Root call should not be reverted")

				// Reverted call should have 1 log with BlockIndex=0
				require.Equal(t, 1, len(revertedCall.Logs), "Reverted call should have 1 log")
				assert.True(t, revertedCall.StateReverted, "Nested call should be reverted")
				assert.Equal(t, uint32(0), revertedCall.Logs[0].BlockIndex,
					"Log in reverted call should have BlockIndex=0")

				// Receipt should only have 1 log (from root call)
				require.NotNil(t, trx.Receipt, "Receipt should exist")
				require.Equal(t, 1, len(trx.Receipt.Logs), "Receipt should only have 1 log")
				assert.Equal(t, rootCall.Logs[0].Ordinal, trx.Receipt.Logs[0].Ordinal,
					"Receipt log should match root call log")
			})
	})

	t.Run("all_logs_removed_when_root_call_reverts", func(t *testing.T) {
		// Scenario: Root call reverts - no logs should be in receipt
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			Log(BobAddr, [][32]byte{topic("Event1")}, []byte{0x01}, 0).
			StartCall(BobAddr, CharlieAddr, bigInt(0), 50000, []byte{0x02}).
			Log(CharlieAddr, [][32]byte{topic("Event2")}, []byte{0x02}, 1).
			EndCall([]byte{}, 45000).
			Log(BobAddr, [][32]byte{topic("Event3")}, []byte{0x03}, 2).
			EndCallFailed([]byte("root reverted"), 90000, testErrExecutionReverted, true).
			EndBlockTrx(receiptWithLogs(100000, []firehose.LogData{}), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				rootCall := trx.Calls[0]

				assert.Equal(t, pbeth.TransactionTraceStatus_REVERTED, trx.Status,
					"Transaction should be reverted")
				assert.True(t, rootCall.StateReverted, "Root call should be reverted")

				// All calls should have StateReverted=true
				for i, call := range trx.Calls {
					assert.True(t, call.StateReverted, "Call %d should be reverted", i)
					// All logs should have BlockIndex=0
					for j, log := range call.Logs {
						assert.Equal(t, uint32(0), log.BlockIndex,
							"Call %d log %d should have BlockIndex=0", i, j)
					}
				}

				// Receipt should have no logs
				require.NotNil(t, trx.Receipt, "Receipt should exist")
				assert.Equal(t, 0, len(trx.Receipt.Logs), "Receipt should have no logs")
			})
	})

	t.Run("deeply_nested_reverted_calls", func(t *testing.T) {
		// Scenario: Root -> A (success, 1 log) -> B (reverted, 1 log) -> C (reverted, 1 log)
		// Receipt should only have 1 log from A
		receiptLogs := []firehose.LogData{
			log1(CharlieAddr, topic("Success"), []byte{0x01}),
		}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			// Call A succeeds
			StartCall(BobAddr, CharlieAddr, bigInt(0), 80000, []byte{0x02}).
			Log(CharlieAddr, [][32]byte{topic("Success")}, []byte{0x01}, 0).
			// Call B reverts
			StartCall(CharlieAddr, addressFromHex("0xdead"), bigInt(0), 60000, []byte{0x03}).
			Log(addressFromHex("0xdead"), [][32]byte{topic("Reverted1")}, []byte{0x02}, 1).
			// Call C reverts (child of B)
			StartCall(addressFromHex("0xdead"), addressFromHex("0xbeef"), bigInt(0), 40000, []byte{0x04}).
			Log(addressFromHex("0xbeef"), [][32]byte{topic("Reverted2")}, []byte{0x03}, 2).
			EndCallFailed([]byte("C error"), 35000, testErrExecutionReverted, true).
			EndCallFailed([]byte("B error"), 55000, testErrExecutionReverted, true).
			EndCall([]byte{}, 70000). // A succeeds
			EndCall([]byte{}, 85000). // Root succeeds
			EndBlockTrx(receiptWithLogs(100000, receiptLogs), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				require.Equal(t, 4, len(trx.Calls), "Should have 4 calls")

				callA := trx.Calls[1]
				callB := trx.Calls[2]
				callC := trx.Calls[3]

				// A should not be reverted
				assert.False(t, callA.StateReverted, "Call A should not be reverted")
				assert.Equal(t, uint32(0), callA.Logs[0].BlockIndex, "Call A log should keep original BlockIndex=0")

				// B and C should be reverted
				assert.True(t, callB.StateReverted, "Call B should be reverted")
				assert.True(t, callC.StateReverted, "Call C should be reverted")
				assert.Equal(t, uint32(0), callB.Logs[0].BlockIndex, "Call B log should have BlockIndex=0")
				assert.Equal(t, uint32(0), callC.Logs[0].BlockIndex, "Call C log should have BlockIndex=0")

				// Receipt should only have 1 log from A
				require.Equal(t, 1, len(trx.Receipt.Logs), "Receipt should have 1 log")
				assert.Equal(t, callA.Logs[0].Ordinal, trx.Receipt.Logs[0].Ordinal,
					"Receipt log should match call A log")
			})
	})

	t.Run("failed_transaction_with_logs", func(t *testing.T) {
		// Scenario: Transaction fails (receipt status=0) but didn't revert
		// Logs should still appear in receipt
		receiptLogs := []firehose.LogData{
			log1(BobAddr, topic("FailedEvent"), []byte{0x01}),
		}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			Log(BobAddr, [][32]byte{topic("FailedEvent")}, []byte{0x01}, 0).
			EndCall([]byte{}, 95000).                                          // Call succeeds (no revert)
			EndBlockTrx(failedReceiptWithLogs(100000, receiptLogs), nil, nil). // But receipt status=0
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				rootCall := trx.Calls[0]

				assert.Equal(t, pbeth.TransactionTraceStatus_FAILED, trx.Status,
					"Transaction should be FAILED (receipt status=0)")
				assert.False(t, rootCall.StateReverted, "Root call should not be state reverted")

				// Log should still be in receipt
				require.Equal(t, 1, len(trx.Receipt.Logs), "Receipt should have 1 log")
				assert.Equal(t, rootCall.Logs[0].Ordinal, trx.Receipt.Logs[0].Ordinal,
					"Receipt log should match call log")
			})
	})
}

// TestTracer_ReceiptLogsBloom tests that logsBloom is properly populated from receipt
func TestTracer_ReceiptLogsBloom(t *testing.T) {
	t.Run("empty_bloom_for_transaction_without_logs", func(t *testing.T) {
		receipt := &firehose.ReceiptData{
			TransactionIndex:  0,
			Status:            1,
			GasUsed:           21000,
			LogsBloom:         [256]byte{}, // Empty bloom (no logs)
			CumulativeGasUsed: 21000,
			Logs:              nil,
		}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{0x01}).
			EndCall([]byte{}, 20000).
			EndBlockTrx(receipt, nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				require.NotNil(t, trx.Receipt, "Receipt should exist")
				require.NotNil(t, trx.Receipt.LogsBloom, "LogsBloom should not be nil")
				assert.Equal(t, 256, len(trx.Receipt.LogsBloom), "LogsBloom should be 256 bytes")

				// Verify all bytes are zero (empty bloom)
				for i, b := range trx.Receipt.LogsBloom {
					assert.Equal(t, byte(0), b, "LogsBloom byte %d should be 0", i)
				}
			})
	})

	t.Run("non_empty_bloom_for_transaction_with_logs", func(t *testing.T) {
		// Create a non-empty bloom (simulated - in real scenario this would be computed from logs)
		bloom := [256]byte{}
		bloom[0] = 0x01 // Set first byte to non-zero to simulate bloom with data
		bloom[100] = 0x42
		bloom[255] = 0xff

		receiptLogs := []firehose.LogData{
			log1(BobAddr, topic("Transfer"), []byte{0x01}),
		}

		receipt := &firehose.ReceiptData{
			TransactionIndex:  0,
			Status:            1,
			GasUsed:           30000,
			LogsBloom:         bloom, // Non-empty bloom
			CumulativeGasUsed: 30000,
			Logs:              receiptLogs,
		}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			Log(BobAddr, [][32]byte{topic("Transfer")}, []byte{0x01}, 0).
			EndCall([]byte{}, 95000).
			EndBlockTrx(receipt, nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				require.NotNil(t, trx.Receipt, "Receipt should exist")
				require.NotNil(t, trx.Receipt.LogsBloom, "LogsBloom should not be nil")
				assert.Equal(t, 256, len(trx.Receipt.LogsBloom), "LogsBloom should be 256 bytes")

				// Verify bloom data matches what we passed in
				assert.Equal(t, byte(0x01), trx.Receipt.LogsBloom[0], "LogsBloom[0] should be 0x01")
				assert.Equal(t, byte(0x42), trx.Receipt.LogsBloom[100], "LogsBloom[100] should be 0x42")
				assert.Equal(t, byte(0xff), trx.Receipt.LogsBloom[255], "LogsBloom[255] should be 0xff")
			})
	})

	t.Run("bloom_preserved_from_receipt_input", func(t *testing.T) {
		// Verify that bloom filter is faithfully copied from ReceiptData to TransactionReceipt
		// This tests that the shared tracer properly accepts bloom from chain implementation
		// rather than attempting to compute it
		bloom := [256]byte{}
		// Set specific pattern in bloom to verify exact bytes are preserved
		bloom[0] = 0xaa
		bloom[1] = 0xbb
		bloom[50] = 0x12
		bloom[100] = 0x34
		bloom[200] = 0x56
		bloom[254] = 0x78
		bloom[255] = 0x9a

		receipt := &firehose.ReceiptData{
			TransactionIndex:  3, // Non-zero index to test that too
			Status:            1,
			GasUsed:           50000,
			LogsBloom:         bloom,
			CumulativeGasUsed: 150000,
		}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			EndCall([]byte{}, 95000).
			EndBlockTrx(receipt, nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				require.NotNil(t, trx.Receipt, "Receipt should exist")
				require.NotNil(t, trx.Receipt.LogsBloom, "LogsBloom should not be nil")
				assert.Equal(t, 256, len(trx.Receipt.LogsBloom), "LogsBloom should be 256 bytes")

				// Verify every byte we set is preserved exactly
				assert.Equal(t, byte(0xaa), trx.Receipt.LogsBloom[0], "LogsBloom[0] not preserved")
				assert.Equal(t, byte(0xbb), trx.Receipt.LogsBloom[1], "LogsBloom[1] not preserved")
				assert.Equal(t, byte(0x12), trx.Receipt.LogsBloom[50], "LogsBloom[50] not preserved")
				assert.Equal(t, byte(0x34), trx.Receipt.LogsBloom[100], "LogsBloom[100] not preserved")
				assert.Equal(t, byte(0x56), trx.Receipt.LogsBloom[200], "LogsBloom[200] not preserved")
				assert.Equal(t, byte(0x78), trx.Receipt.LogsBloom[254], "LogsBloom[254] not preserved")
				assert.Equal(t, byte(0x9a), trx.Receipt.LogsBloom[255], "LogsBloom[255] not preserved")

				// Verify unset bytes remain zero
				assert.Equal(t, byte(0x00), trx.Receipt.LogsBloom[2], "Unset bloom byte should be 0")
				assert.Equal(t, byte(0x00), trx.Receipt.LogsBloom[99], "Unset bloom byte should be 0")
			})
	})
}

// TestTracer_LogBlockIndex tests BlockIndex assignment and removal
func TestTracer_LogBlockIndex(t *testing.T) {
	t.Run("successful_logs_have_block_index", func(t *testing.T) {
		receiptLogs := []firehose.LogData{
			log1(BobAddr, topic("Event"), []byte{0x01}),
		}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			Log(BobAddr, [][32]byte{topic("Event")}, []byte{0x01}, 0).
			EndCall([]byte{}, 95000).
			EndBlockTrx(receiptWithLogs(100000, receiptLogs), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Call log should have BlockIndex set
				require.Equal(t, 1, len(call.Logs), "Should have 1 log")
				assert.Equal(t, uint32(0), call.Logs[0].BlockIndex,
					"First log in block should have BlockIndex=0")

				// Receipt log should have same BlockIndex
				assert.Equal(t, call.Logs[0].BlockIndex, trx.Receipt.Logs[0].BlockIndex,
					"Receipt log BlockIndex should match call log")
			})
	})

	t.Run("reverted_logs_have_zero_block_index", func(t *testing.T) {
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			Log(BobAddr, [][32]byte{topic("Event")}, []byte{0x01}, 0).
			EndCallFailed([]byte("error"), 95000, testErrExecutionReverted, true).
			EndBlockTrx(receiptWithLogs(100000, []firehose.LogData{}), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Call log should have BlockIndex=0 (removed by removeLogBlockIndexOnStateRevertedCalls)
				require.Equal(t, 1, len(call.Logs), "Should have 1 log")
				assert.Equal(t, uint32(0), call.Logs[0].BlockIndex,
					"Reverted log should have BlockIndex=0")
			})
	})
}
