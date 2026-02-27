package tests

import (
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTracer_TxTypes tests all transaction types
func TestTracer_TxTypes(t *testing.T) {
	t.Run("legacy", func(t *testing.T) {
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 1, len(block.TransactionTraces), "Should have one transaction")
				trx := block.TransactionTraces[0]

				// Validate transaction type
				assert.Equal(t, pbeth.TransactionTrace_TRX_TYPE_LEGACY, trx.Type)

				// Validate basic transaction fields (hash is computed by native validator)
				assert.NotEmpty(t, trx.Hash, "firehose.Hash should be set")
				assert.Equal(t, AliceAddr[:], trx.From)
				assert.Equal(t, BobAddr[:], trx.To)
				assert.Equal(t, uint64(100), trx.Value.Uint64())
				assert.Equal(t, uint64(21000), trx.GasLimit)
			})
	})

	t.Run("access_list", func(t *testing.T) {
		NewTracerTester(t).
			StartBlockTrx(TestAccessListTrx).
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 1, len(block.TransactionTraces), "Should have one transaction")
				trx := block.TransactionTraces[0]

				// Validate transaction type
				assert.Equal(t, pbeth.TransactionTrace_TRX_TYPE_ACCESS_LIST, trx.Type)

				// Validate basic transaction fields (hash is computed by native validator)
				assert.NotEmpty(t, trx.Hash, "firehose.Hash should be set")
				assert.Equal(t, AliceAddr[:], trx.From)
				assert.Equal(t, BobAddr[:], trx.To)

				// Validate access list is present
				assert.NotNil(t, trx.AccessList, "Access list should be present")
				assert.Equal(t, 1, len(trx.AccessList), "Should have one access list entry")
				assert.Equal(t, BobAddr[:], trx.AccessList[0].Address)
				assert.Equal(t, 1, len(trx.AccessList[0].StorageKeys), "Should have one storage key")
			})
	})

	t.Run("dynamic_fee", func(t *testing.T) {
		NewTracerTester(t).
			StartBlockTrx(TestDynamicFeeTrx).
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 1, len(block.TransactionTraces), "Should have one transaction")
				trx := block.TransactionTraces[0]

				// Validate transaction type
				assert.Equal(t, pbeth.TransactionTrace_TRX_TYPE_DYNAMIC_FEE, trx.Type)

				// Validate basic transaction fields (hash is computed by native validator)
				assert.NotEmpty(t, trx.Hash, "firehose.Hash should be set")
				assert.Equal(t, AliceAddr[:], trx.From)
				assert.Equal(t, BobAddr[:], trx.To)

				// Validate EIP-1559 fields
				assert.NotNil(t, trx.MaxFeePerGas, "MaxFeePerGas should be present")
				assert.Equal(t, uint64(20), trx.MaxFeePerGas.Uint64(), "MaxFeePerGas should be 20")
				assert.NotNil(t, trx.MaxPriorityFeePerGas, "MaxPriorityFeePerGas should be present")
				assert.Equal(t, uint64(2), trx.MaxPriorityFeePerGas.Uint64(), "MaxPriorityFeePerGas should be 2")

				// Validate access list is present
				assert.NotNil(t, trx.AccessList, "Access list should be present")
				assert.Equal(t, 1, len(trx.AccessList), "Should have one access list entry")
			})
	})

	t.Run("blob", func(t *testing.T) {
		NewTracerTester(t).
			StartBlockTrx(TestBlobTrx).
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 1, len(block.TransactionTraces), "Should have one transaction")
				trx := block.TransactionTraces[0]

				// Validate transaction type
				assert.Equal(t, pbeth.TransactionTrace_TRX_TYPE_BLOB, trx.Type)

				// Validate basic transaction fields (hash is computed by native validator)
				assert.NotEmpty(t, trx.Hash, "firehose.Hash should be set")
				assert.Equal(t, AliceAddr[:], trx.From)
				assert.Equal(t, BobAddr[:], trx.To)

				// Validate EIP-4844 blob fields
				assert.NotNil(t, trx.BlobGasFeeCap, "BlobGasFeeCap should be present")
				assert.Equal(t, uint64(5), trx.BlobGasFeeCap.Uint64(), "BlobGasFeeCap should be 5")
				assert.NotNil(t, trx.BlobHashes, "BlobHashes should be present")
				assert.Equal(t, 1, len(trx.BlobHashes), "Should have one blob hash")
			})
	})

	t.Run("set_code", func(t *testing.T) {
		// Create a properly signed SetCode authorization
		auth, err := firehose.SignSetCodeAuth(AliceKey, 1, CharlieAddr, 0)
		require.NoError(t, err)

		tester := NewTracerTester(t).StartBlockTrx(
			new(TxEventBuilder).
				Defaults().
				Type(TxTypeSetCode).
				SetCodeAuthorizations([]firehose.SetCodeAuthorization{auth}).
				Build(),
		)

		// EIP-7702: Authorization application happens BEFORE the root call
		// The authorizer's nonce is incremented when the authorization is applied
		tester.NonceChange(AliceAddr, 0, 1)

		tester.
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 1, len(block.TransactionTraces), "Should have one transaction")
				trx := block.TransactionTraces[0]

				// Validate transaction type
				assert.Equal(t, pbeth.TransactionTrace_TRX_TYPE_SET_CODE, trx.Type, "Type should be SET_CODE")

				// Validate basic transaction fields (hash is computed by native validator)
				assert.NotEmpty(t, trx.Hash, "firehose.Hash should be set")
				assert.Equal(t, AliceAddr[:], trx.From)
				assert.Equal(t, BobAddr[:], trx.To)

				// Validate EIP-7702 set code authorization list
				assert.NotNil(t, trx.SetCodeAuthorizations, "SetCodeAuthorizations should be present")
				assert.Equal(t, 1, len(trx.SetCodeAuthorizations), "Should have one authorization")
				auth := trx.SetCodeAuthorizations[0]
				assert.Equal(t, CharlieAddr[:], auth.Address, "Authorization address should match")
				assert.Equal(t, uint64(0), auth.Nonce, "Authorization nonce should be 0")

				// Validate that the authorization signature is valid and was applied
				// The native validator verifies the signature and checks for a nonce change
				assert.False(t, auth.Discarded, "Authorization should not be discarded (signature is valid and nonce was incremented)")
				assert.NotEmpty(t, auth.Authority, "Authority should be populated with the signer's address")
			})
	})
}
