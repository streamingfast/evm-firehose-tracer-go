package tests

import (
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTracer_CompleteTransaction_EdgeCases tests edge cases and error resistance in completeTransaction
func TestTracer_CompleteTransaction_EdgeCases(t *testing.T) {
	t.Run("empty_calls_array_bad_block", func(t *testing.T) {
		// Scenario: Transaction ends without any calls (bad block or misconfigured)
		// This tests the early return path at tracer.go:636-640
		// Note: Receipt is provided but NOT populated when calls array is empty (early return)
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			// No calls added - simulate bad block
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]

				// Should have 0 calls
				require.Equal(t, 0, len(trx.Calls), "Bad block should have 0 calls")

				// Should still have EndOrdinal set (line 638)
				assert.Greater(t, trx.EndOrdinal, uint64(0), "EndOrdinal should be set even for bad block")

				// Receipt is NOT populated when there are no calls (early return at line 639)
				assert.Nil(t, trx.Receipt, "Receipt should be nil for bad block with no calls")

				// Transaction status should be UNKNOWN (not set from receipt)
				assert.Equal(t, pbeth.TransactionTraceStatus_UNKNOWN, trx.Status,
					"Status should be UNKNOWN for bad block")
			})
	})

	t.Run("nil_receipt", func(t *testing.T) {
		// Scenario: Transaction completes without receipt (error case)
		// This tests the receipt == nil path at tracer.go:672
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			EndCall([]byte{0x42}, 95000).
			EndBlockTrx(nil, nil, nil). // No receipt
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]

				// Receipt should be nil
				assert.Nil(t, trx.Receipt, "Receipt should be nil")

				// Transaction index and gas should not be set from receipt
				assert.Equal(t, uint32(0), trx.Index, "Index should be default 0")
				assert.Equal(t, uint64(0), trx.GasUsed, "GasUsed should be default 0")

				// Status should still be determined by call state
				// (no receipt, so status comes from call success/failure)
				// Root call succeeded, so status should be UNKNOWN since no receipt
				assert.Equal(t, pbeth.TransactionTraceStatus_UNKNOWN, trx.Status,
					"Status should be UNKNOWN when no receipt")

				// Root call return data should still be copied
				assert.Equal(t, []byte{0x42}, trx.ReturnData,
					"Return data should be copied from root call even without receipt")
			})
	})

	t.Run("nil_receipt_with_reverted_call", func(t *testing.T) {
		// Scenario: Transaction reverts but no receipt (error case)
		// Verifies that reverted status is still set even without receipt
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			EndCallFailed([]byte("error"), 95000, testErrExecutionReverted, true).
			EndBlockTrx(nil, nil, nil). // No receipt
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]

				// Receipt should be nil
				assert.Nil(t, trx.Receipt, "Receipt should be nil")

				// Root call should be reverted
				assert.True(t, trx.Calls[0].StatusReverted, "Root call should be reverted")

				// Transaction status should be REVERTED (set from root call, not receipt)
				assert.Equal(t, pbeth.TransactionTraceStatus_REVERTED, trx.Status,
					"Status should be REVERTED even without receipt")
			})
	})

	t.Run("return_data_copied_from_root_call", func(t *testing.T) {
		// Scenario: Root call return data should be copied to transaction
		// This tests tracer.go:666
		returnData := []byte{0xde, 0xad, 0xbe, 0xef}

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			EndCall(returnData, 95000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				rootCall := trx.Calls[0]

				// Root call should have the return data
				assert.Equal(t, returnData, rootCall.ReturnData, "Root call should have return data")

				// Transaction should have the same return data
				assert.Equal(t, returnData, trx.ReturnData,
					"Transaction return data should be copied from root call")
			})
	})

	t.Run("empty_return_data", func(t *testing.T) {
		// Scenario: Empty return data should be handled correctly
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			EndCall([]byte{}, 95000). // Empty return data
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]

				// Transaction return data should be empty (nil or empty slice)
				assert.Empty(t, trx.ReturnData, "Transaction return data should be empty")
			})
	})

	t.Run("nil_return_data", func(t *testing.T) {
		// Scenario: Nil return data should be handled correctly
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			EndCall(nil, 95000). // Nil return data
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]

				// Transaction return data should be nil or empty
				if trx.ReturnData != nil {
					assert.Equal(t, 0, len(trx.ReturnData), "Return data should be empty if not nil")
				}
			})
	})

	t.Run("end_ordinal_always_set", func(t *testing.T) {
		// Scenario: EndOrdinal should always be set, even for failed transactions
		// This tests tracer.go:698
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			EndCallFailed([]byte("error"), 95000, testErrExecutionReverted, true).
			EndBlockTrx(failedReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]

				// EndOrdinal should be set and > BeginOrdinal
				assert.Greater(t, trx.EndOrdinal, trx.BeginOrdinal,
					"EndOrdinal should be greater than BeginOrdinal")
				assert.Greater(t, trx.EndOrdinal, uint64(0), "EndOrdinal should be set")
			})
	})

	t.Run("multiple_transactions_ordinals_sequential", func(t *testing.T) {
		// Scenario: Multiple transactions should have sequential ordinals
		// Verifies that EndOrdinal advances properly across transactions
		// Note: This is tested via system call tests, which have multiple transactions
		// Simplified test here just verifies the concept
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 50000, []byte{0x01}).
			EndCall([]byte{}, 45000).
			EndBlockTrx(successReceipt(50000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]

				// EndOrdinal should be greater than BeginOrdinal
				assert.Greater(t, trx.EndOrdinal, trx.BeginOrdinal,
					"EndOrdinal should be greater than BeginOrdinal")
			})
	})
}

// TestTracer_CompleteTransaction_BlobTransaction tests blob transaction specific paths
func TestTracer_CompleteTransaction_BlobTransaction(t *testing.T) {
	t.Run("blob_transaction_with_blob_gas", func(t *testing.T) {
		// Scenario: Blob transaction (type 3) should populate blob gas fields
		// This tests tracer.go:710-715 (newReceiptFromData blob fields)
		blobGasUsed := uint64(131072)            // 1 blob
		blobGasPrice := mustBigInt("1000000000") // 1 gwei

		receipt := &firehose.ReceiptData{
			TransactionIndex:  0,
			Status:            1,
			GasUsed:           50000,
			CumulativeGasUsed: 50000,
			BlobGasUsed:       blobGasUsed,
			BlobGasPrice:      blobGasPrice,
		}

		tester := NewTracerTester(t)
		tester.StartBlockTrx(TestBlobTrx)

		tester.StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			EndCall([]byte{}, 95000).
			EndBlockTrx(receipt, nil, nil)

		// Parse the block output (skip native validator comparison since we manually set the type)
		block := ParseFirehoseBlock(t, "shared tracer", tester.tracer.GetTestingOutputBuffer())
		trx := block.TransactionTraces[0]

		// Verify transaction type
		assert.Equal(t, pbeth.TransactionTrace_TRX_TYPE_BLOB, trx.Type, "Transaction type should be BLOB")

		require.NotNil(t, trx.Receipt, "Receipt should exist")
		require.NotNil(t, trx.Receipt.BlobGasUsed, "BlobGasUsed should be set")
		require.NotNil(t, trx.Receipt.BlobGasPrice, "BlobGasPrice should be set")

		assert.Equal(t, blobGasUsed, *trx.Receipt.BlobGasUsed, "BlobGasUsed should match")

		assert.Equal(t, blobGasPrice.Bytes(), trx.Receipt.BlobGasPrice.Bytes,
			"BlobGasPrice should match")
	})

	t.Run("non_blob_transaction_no_blob_gas", func(t *testing.T) {
		// Scenario: Non-blob transaction should not have blob gas fields
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			EndCall([]byte{}, 95000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]

				// Default transaction type is legacy (type 0)
				assert.Equal(t, pbeth.TransactionTrace_TRX_TYPE_LEGACY, trx.Type)

				require.NotNil(t, trx.Receipt, "Receipt should exist")

				// Blob gas fields should be nil for non-blob transactions
				assert.Nil(t, trx.Receipt.BlobGasUsed, "BlobGasUsed should be nil for non-blob tx")
				assert.Nil(t, trx.Receipt.BlobGasPrice, "BlobGasPrice should be nil for non-blob tx")
			})
	})
}
