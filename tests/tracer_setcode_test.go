package tests

import (
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTracer_firehose.SetCodeAuthorization tests EIP-7702 SetCode authorization validation
func TestTracer_SetCodeAuthorization(t *testing.T) {
	t.Run("valid_authorization_with_nonce_change", func(t *testing.T) {
		// Alice signs an authorization to delegate to Charlie's code
		auth, err := firehose.SignSetCodeAuth(AliceKey, 1, CharlieAddr, 0)
		require.NoError(t, err, "Failed to sign authorization")

		txEvent := new(TxEventBuilder).
			Defaults().
			Type(TxTypeSetCode).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{auth}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.Tracer.OnTxStart(txEvent, tester.stateReader)

		// EIP-7702: Authorization application happens BEFORE the root call
		// The authorizer's nonce is incremented when the authorization is applied
		tester.NonceChange(AliceAddr, 0, 1)

		tester.
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				require.Equal(t, 1, len(block.TransactionTraces), "Should have one transaction")
				trx := block.TransactionTraces[0]

				// Validate transaction type
				assert.Equal(t, pbeth.TransactionTrace_TRX_TYPE_SET_CODE, trx.Type)

				// Validate authorization list
				require.NotNil(t, trx.SetCodeAuthorizations, "SetCodeAuthorizations should be present")
				require.Equal(t, 1, len(trx.SetCodeAuthorizations), "Should have one authorization")

				authResult := trx.SetCodeAuthorizations[0]

				// Validate authorization fields
				assert.Equal(t, CharlieAddr[:], authResult.Address, "Authorization address should match")
				assert.Equal(t, uint64(0), authResult.Nonce, "Authorization nonce should be 0")

				// Validate signature recovery - Authority should be populated with Alice's address
				assert.NotEmpty(t, authResult.Authority, "Authority should be populated from signature")
				assert.Equal(t, AliceAddr[:], authResult.Authority, "Authority should be Alice (the signer)")

				// Validate that the authorization was NOT discarded (signature valid + nonce change present)
				assert.False(t, authResult.Discarded, "Authorization should not be discarded")
			})
	})

	t.Run("invalid_signature_discarded", func(t *testing.T) {
		// Create authorization with valid signature
		auth, err := firehose.SignSetCodeAuth(AliceKey, 1, CharlieAddr, 0)
		require.NoError(t, err)

		// Corrupt the signature by changing V
		auth.V = auth.V + 10 // Invalid V value

		txEvent := new(TxEventBuilder).
			Defaults().
			Type(TxTypeSetCode).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{auth}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.Tracer.OnTxStart(txEvent, tester.stateReader)

		tester.
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				require.Equal(t, 1, len(block.TransactionTraces), "Should have one transaction")
				trx := block.TransactionTraces[0]

				require.NotNil(t, trx.SetCodeAuthorizations)
				require.Equal(t, 1, len(trx.SetCodeAuthorizations))

				authResult := trx.SetCodeAuthorizations[0]

				// Invalid signature - Authority should be empty
				assert.Empty(t, authResult.Authority, "Authority should be empty for invalid signature")

				// Invalid signature - should be discarded
				assert.True(t, authResult.Discarded, "Authorization with invalid signature should be discarded")
			})
	})

	t.Run("missing_nonce_change_discarded", func(t *testing.T) {
		// Alice signs an authorization to delegate to Charlie's code
		auth, err := firehose.SignSetCodeAuth(AliceKey, 1, CharlieAddr, 0)
		require.NoError(t, err)

		txEvent := new(TxEventBuilder).
			Defaults().
			Type(TxTypeSetCode).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{auth}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.Tracer.OnTxStart(txEvent, tester.stateReader)

		// NOTE: We do NOT add a nonce change for Alice
		// This simulates the authorization not being applied (e.g., wrong nonce, already applied, etc.)

		tester.
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				require.Equal(t, 1, len(block.TransactionTraces), "Should have one transaction")
				trx := block.TransactionTraces[0]

				require.NotNil(t, trx.SetCodeAuthorizations)
				require.Equal(t, 1, len(trx.SetCodeAuthorizations))

				authResult := trx.SetCodeAuthorizations[0]

				// Signature is valid - Authority should be populated
				assert.NotEmpty(t, authResult.Authority, "Authority should be populated from valid signature")
				assert.Equal(t, AliceAddr[:], authResult.Authority, "Authority should be Alice")

				// But no nonce change - should be discarded
				assert.True(t, authResult.Discarded, "Authorization without nonce change should be discarded")
			})
	})

	t.Run("multiple_authorizations_mixed_validity", func(t *testing.T) {
		// Alice signs a valid authorization
		aliceAuth, err := firehose.SignSetCodeAuth(AliceKey, 1, CharlieAddr, 0)
		require.NoError(t, err)

		// Bob signs a valid authorization
		bobAuth, err := firehose.SignSetCodeAuth(BobKey, 1, CharlieAddr, 0)
		require.NoError(t, err)

		// Charlie signs an authorization (but we won't add nonce change - will be discarded)
		charlieAuth, err := firehose.SignSetCodeAuth(CharlieKey, 1, AliceAddr, 0)
		require.NoError(t, err)

		txEvent := new(TxEventBuilder).
			Defaults().
			Type(TxTypeSetCode).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{aliceAuth, bobAuth, charlieAuth}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.Tracer.OnTxStart(txEvent, tester.stateReader)

		// Add nonce changes for Alice and Bob (they get applied)
		// Charlie's authorization does NOT get applied (no nonce change)
		tester.NonceChange(AliceAddr, 0, 1)
		tester.NonceChange(BobAddr, 0, 1)

		tester.
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				require.Equal(t, 1, len(block.TransactionTraces), "Should have one transaction")
				trx := block.TransactionTraces[0]

				require.NotNil(t, trx.SetCodeAuthorizations)
				require.Equal(t, 3, len(trx.SetCodeAuthorizations), "Should have three authorizations")

				// Alice's authorization - valid signature + nonce change = NOT discarded
				aliceResult := trx.SetCodeAuthorizations[0]
				assert.Equal(t, AliceAddr[:], aliceResult.Authority, "Alice's authority should match")
				assert.False(t, aliceResult.Discarded, "Alice's authorization should NOT be discarded")

				// Bob's authorization - valid signature + nonce change = NOT discarded
				bobResult := trx.SetCodeAuthorizations[1]
				assert.Equal(t, BobAddr[:], bobResult.Authority, "Bob's authority should match")
				assert.False(t, bobResult.Discarded, "Bob's authorization should NOT be discarded")

				// Charlie's authorization - valid signature but NO nonce change = discarded
				charlieResult := trx.SetCodeAuthorizations[2]
				assert.Equal(t, CharlieAddr[:], charlieResult.Authority, "Charlie's authority should match")
				assert.True(t, charlieResult.Discarded, "Charlie's authorization should be discarded (no nonce change)")
			})
	})

	t.Run("empty_authorizations_list", func(t *testing.T) {
		// SetCode transaction with empty authorization list should be valid
		txEvent := new(TxEventBuilder).
			Defaults().
			Type(TxTypeSetCode).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.Tracer.OnTxStart(txEvent, tester.stateReader)

		tester.
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				require.Equal(t, 1, len(block.TransactionTraces), "Should have one transaction")
				trx := block.TransactionTraces[0]

				// Validate transaction type
				assert.Equal(t, pbeth.TransactionTrace_TRX_TYPE_SET_CODE, trx.Type)

				// Empty authorization list results in nil (matching native tracer behavior)
				assert.Nil(t, trx.SetCodeAuthorizations, "SetCodeAuthorizations should be nil for empty list")
			})
	})

	t.Run("nonce_mismatch_discarded", func(t *testing.T) {
		// Authorization expects nonce 0→1, but actual nonce change is 5→6
		auth, err := firehose.SignSetCodeAuth(AliceKey, 1, CharlieAddr, 0)
		require.NoError(t, err)

		txEvent := new(TxEventBuilder).
			Defaults().
			Type(TxTypeSetCode).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{auth}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.Tracer.OnTxStart(txEvent, tester.stateReader)

		// Nonce change exists but doesn't match authorization's expected nonce
		tester.NonceChange(AliceAddr, 5, 6) // Wrong nonce range

		tester.
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				require.Equal(t, 1, len(block.TransactionTraces), "Should have one transaction")
				trx := block.TransactionTraces[0]

				require.NotNil(t, trx.SetCodeAuthorizations)
				require.Equal(t, 1, len(trx.SetCodeAuthorizations))

				authResult := trx.SetCodeAuthorizations[0]

				// Signature is valid
				assert.NotEmpty(t, authResult.Authority, "Authority should be populated")
				assert.Equal(t, AliceAddr[:], authResult.Authority, "Authority should be Alice")

				// But nonce mismatch - should be discarded
				assert.True(t, authResult.Discarded, "Authorization with nonce mismatch should be discarded")
			})
	})

	t.Run("duplicate_nonce_change_only_one_used", func(t *testing.T) {
		// Two authorizations from same authority with same nonce
		// Only one nonce change available - second should be discarded
		auth1, err := firehose.SignSetCodeAuth(AliceKey, 1, CharlieAddr, 0)
		require.NoError(t, err)

		// Create second authorization from same key (Alice) with same nonce
		auth2, err := firehose.SignSetCodeAuth(AliceKey, 1, BobAddr, 0)
		require.NoError(t, err)

		txEvent := new(TxEventBuilder).
			Defaults().
			Type(TxTypeSetCode).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{auth1, auth2}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.Tracer.OnTxStart(txEvent, tester.stateReader)

		// Only ONE nonce change for Alice (0→1)
		// Both authorizations are from Alice with nonce=0, but only one can match
		tester.NonceChange(AliceAddr, 0, 1)

		tester.
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				require.Equal(t, 1, len(block.TransactionTraces), "Should have one transaction")
				trx := block.TransactionTraces[0]

				require.NotNil(t, trx.SetCodeAuthorizations)
				require.Equal(t, 2, len(trx.SetCodeAuthorizations))

				// Both have valid signatures
				auth1Result := trx.SetCodeAuthorizations[0]
				assert.Equal(t, AliceAddr[:], auth1Result.Authority)

				auth2Result := trx.SetCodeAuthorizations[1]
				assert.Equal(t, AliceAddr[:], auth2Result.Authority)

				// Only one can use the nonce change
				// First one gets it, second one is discarded
				discardedCount := 0
				if auth1Result.Discarded {
					discardedCount++
				}
				if auth2Result.Discarded {
					discardedCount++
				}

				assert.Equal(t, 1, discardedCount, "Exactly one authorization should be discarded (nonce change reuse prevention)")
			})
	})

	t.Run("setcode_with_access_list", func(t *testing.T) {
		// SetCode transaction can include an access list (EIP-1559 feature)
		auth, err := firehose.SignSetCodeAuth(AliceKey, 1, CharlieAddr, 0)
		require.NoError(t, err)

		// Create access list
		accessList := firehose.AccessList{
			{
				Address:     BobAddr,
				StorageKeys: [][32]byte{hashFromHex("0x0000000000000000000000000000000000000000000000000000000000000001")},
			},
		}

		txEvent := new(TxEventBuilder).
			Defaults().
			Type(TxTypeSetCode).
			AccessList(accessList).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{auth}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.Tracer.OnTxStart(txEvent, tester.stateReader)

		tester.NonceChange(AliceAddr, 0, 1)

		tester.
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				require.Equal(t, 1, len(block.TransactionTraces), "Should have one transaction")
				trx := block.TransactionTraces[0]

				// Validate transaction type
				assert.Equal(t, pbeth.TransactionTrace_TRX_TYPE_SET_CODE, trx.Type)

				// Validate access list is present
				assert.NotNil(t, trx.AccessList, "Access list should be present")
				assert.Equal(t, 1, len(trx.AccessList), "Should have one access list entry")

				// Validate authorization
				require.NotNil(t, trx.SetCodeAuthorizations)
				require.Equal(t, 1, len(trx.SetCodeAuthorizations))

				authResult := trx.SetCodeAuthorizations[0]
				assert.False(t, authResult.Discarded, "Authorization should not be discarded")
				assert.Equal(t, AliceAddr[:], authResult.Authority)
			})
	})
}
