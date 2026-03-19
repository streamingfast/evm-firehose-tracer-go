package tests

import (
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go/v4"

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

		txEvent := new(firehose.TxEventBuilder).
			Defaults().
			Type(firehose.TxTypeSetCode).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{auth}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.tracer.OnTxStart(txEvent, tester.mockStateDB)

		// EIP-7702: Authorization application happens BEFORE the root call
		// The authorizer's nonce is incremented when the authorization is applied
		tester.NonceChange(AliceAddr, 0, 1)

		tester.
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
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

		txEvent := new(firehose.TxEventBuilder).
			Defaults().
			Type(firehose.TxTypeSetCode).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{auth}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.tracer.OnTxStart(txEvent, tester.mockStateDB)

		tester.
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
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

		txEvent := new(firehose.TxEventBuilder).
			Defaults().
			Type(firehose.TxTypeSetCode).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{auth}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.tracer.OnTxStart(txEvent, tester.mockStateDB)

		// NOTE: We do NOT add a nonce change for Alice
		// This simulates the authorization not being applied (e.g., wrong nonce, already applied, etc.)

		tester.
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
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

		txEvent := new(firehose.TxEventBuilder).
			Defaults().
			Type(firehose.TxTypeSetCode).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{aliceAuth, bobAuth, charlieAuth}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.tracer.OnTxStart(txEvent, tester.mockStateDB)

		// Add nonce changes for Alice and Bob (they get applied)
		// Charlie's authorization does NOT get applied (no nonce change)
		tester.NonceChange(AliceAddr, 0, 1)
		tester.NonceChange(BobAddr, 0, 1)

		tester.
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
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
		txEvent := new(firehose.TxEventBuilder).
			Defaults().
			Type(firehose.TxTypeSetCode).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.tracer.OnTxStart(txEvent, tester.mockStateDB)

		tester.
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
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

		txEvent := new(firehose.TxEventBuilder).
			Defaults().
			Type(firehose.TxTypeSetCode).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{auth}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.tracer.OnTxStart(txEvent, tester.mockStateDB)

		// Nonce change exists but doesn't match authorization's expected nonce
		tester.NonceChange(AliceAddr, 5, 6) // Wrong nonce range

		tester.
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
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

		txEvent := new(firehose.TxEventBuilder).
			Defaults().
			Type(firehose.TxTypeSetCode).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{auth1, auth2}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.tracer.OnTxStart(txEvent, tester.mockStateDB)

		// Only ONE nonce change for Alice (0→1)
		// Both authorizations are from Alice with nonce=0, but only one can match
		tester.NonceChange(AliceAddr, 0, 1)

		tester.
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
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

	t.Run("wrong_address_nonce_change_discarded", func(t *testing.T) {
		// Nonce change exists for the right nonce range (0→1) but for a DIFFERENT address.
		// This tests the bytes.Equal(change.Address, forAddress) condition.
		auth, err := firehose.SignSetCodeAuth(AliceKey, 1, CharlieAddr, 0)
		require.NoError(t, err)

		txEvent := new(firehose.TxEventBuilder).
			Defaults().
			Type(firehose.TxTypeSetCode).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{auth}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.tracer.OnTxStart(txEvent, tester.mockStateDB)

		// Nonce change is for BobAddr (0→1), but auth authority is AliceAddr
		tester.NonceChange(BobAddr, 0, 1)

		tester.
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				require.Equal(t, 1, len(trx.SetCodeAuthorizations))

				authResult := trx.SetCodeAuthorizations[0]
				assert.Equal(t, AliceAddr[:], authResult.Authority, "Authority should be Alice")
				assert.True(t, authResult.Discarded, "Authorization should be discarded: nonce change is for wrong address")
			})
	})

	t.Run("wrong_new_value_nonce_change_discarded", func(t *testing.T) {
		// Nonce change has matching address and OldValue but NewValue ≠ nonce+1.
		// This tests the change.NewValue == nonce+1 condition.
		auth, err := firehose.SignSetCodeAuth(AliceKey, 1, CharlieAddr, 0)
		require.NoError(t, err)

		txEvent := new(firehose.TxEventBuilder).
			Defaults().
			Type(firehose.TxTypeSetCode).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{auth}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.tracer.OnTxStart(txEvent, tester.mockStateDB)

		// OldValue=0 matches auth.Nonce=0, but NewValue=5 ≠ 0+1
		tester.NonceChange(AliceAddr, 0, 5)

		tester.
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				require.Equal(t, 1, len(trx.SetCodeAuthorizations))

				authResult := trx.SetCodeAuthorizations[0]
				assert.Equal(t, AliceAddr[:], authResult.Authority, "Authority should be Alice")
				assert.True(t, authResult.Discarded, "Authorization should be discarded: new nonce value is not nonce+1")
			})
	})

	t.Run("multiple_same_authority_different_nonces_all_valid", func(t *testing.T) {
		// Two authorizations from Alice with different nonces (0 and 1), each with
		// a matching nonce change. Both should be valid (not discarded).
		// This tests that each nonce change is consumed separately (usedNonceChange map).
		auth1, err := firehose.SignSetCodeAuth(AliceKey, 1, CharlieAddr, 0)
		require.NoError(t, err)

		auth2, err := firehose.SignSetCodeAuth(AliceKey, 1, BobAddr, 1)
		require.NoError(t, err)

		txEvent := new(firehose.TxEventBuilder).
			Defaults().
			Type(firehose.TxTypeSetCode).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{auth1, auth2}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.tracer.OnTxStart(txEvent, tester.mockStateDB)

		// Matching nonce changes for both authorizations
		tester.NonceChange(AliceAddr, 0, 1) // matches auth1 (nonce=0)
		tester.NonceChange(AliceAddr, 1, 2) // matches auth2 (nonce=1)

		tester.
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				require.Equal(t, 2, len(trx.SetCodeAuthorizations))

				auth1Result := trx.SetCodeAuthorizations[0]
				assert.Equal(t, AliceAddr[:], auth1Result.Authority)
				assert.False(t, auth1Result.Discarded, "auth1 should NOT be discarded: matching nonce change 0→1 present")

				auth2Result := trx.SetCodeAuthorizations[1]
				assert.Equal(t, AliceAddr[:], auth2Result.Authority)
				assert.False(t, auth2Result.Discarded, "auth2 should NOT be discarded: matching nonce change 1→2 present")
			})
	})

	t.Run("all_empty_authority_all_discarded", func(t *testing.T) {
		// Multiple authorizations all with invalid signatures → all discarded immediately.
		// This tests the len(auth.Authority) == 0 early-discard path for multiple auths.
		auth1, err := firehose.SignSetCodeAuth(AliceKey, 1, CharlieAddr, 0)
		require.NoError(t, err)
		auth1.V = auth1.V + 10 // corrupt signature

		auth2, err := firehose.SignSetCodeAuth(BobKey, 1, CharlieAddr, 0)
		require.NoError(t, err)
		auth2.V = auth2.V + 10 // corrupt signature

		txEvent := new(firehose.TxEventBuilder).
			Defaults().
			Type(firehose.TxTypeSetCode).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{auth1, auth2}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.tracer.OnTxStart(txEvent, tester.mockStateDB)

		// Even if nonce changes are present, empty-authority auths are discarded
		tester.NonceChange(AliceAddr, 0, 1)
		tester.NonceChange(BobAddr, 0, 1)

		tester.
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				require.Equal(t, 2, len(trx.SetCodeAuthorizations))

				for i, authResult := range trx.SetCodeAuthorizations {
					assert.Empty(t, authResult.Authority, "auth[%d]: authority should be empty (invalid signature)", i)
					assert.True(t, authResult.Discarded, "auth[%d]: should be discarded (empty authority)", i)
				}
			})
	})

	t.Run("nonce_change_during_call_matches_auth", func(t *testing.T) {
		// Nonce change happens DURING the root call execution (not in deferred state before
		// the call). The method must still find it in rootCall.NonceChanges.
		auth, err := firehose.SignSetCodeAuth(AliceKey, 1, CharlieAddr, 0)
		require.NoError(t, err)

		txEvent := new(firehose.TxEventBuilder).
			Defaults().
			Type(firehose.TxTypeSetCode).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{auth}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.tracer.OnTxStart(txEvent, tester.mockStateDB)

		// Nonce change happens DURING the call (not deferred before it)
		tester.StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{})
		tester.NonceChange(AliceAddr, 0, 1)
		tester.
			EndCall([]byte{}, 21000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				require.Equal(t, 1, len(trx.SetCodeAuthorizations))

				authResult := trx.SetCodeAuthorizations[0]
				assert.Equal(t, AliceAddr[:], authResult.Authority, "Authority should be Alice")
				assert.False(t, authResult.Discarded, "Authorization should NOT be discarded: nonce change present in root call")
			})
	})

	t.Run("zero_r_s_signature_serializes_as_empty", func(t *testing.T) {
		// Production (native tracer) behavior: when R and S are zero (e.g., an
		// all-zero/unset signature), they must serialize as empty bytes ("" in JSON),
		// not as 32 zero bytes ("AAAA...AAA=" in base64).
		//
		// The native tracer uses big.Int.Bytes() which returns []byte{} for zero, then
		// normalizeSignaturePoint maps that to nil. The shared tracer must do the same.
		auth := firehose.SetCodeAuthorization{
			ChainID: [32]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
			Address: CharlieAddr,
			Nonce:   0,
			V:       0,
			R:       [32]byte{}, // all zeros
			S:       [32]byte{}, // all zeros
		}

		txEvent := new(firehose.TxEventBuilder).
			Defaults().
			Type(firehose.TxTypeSetCode).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{auth}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.tracer.OnTxStart(txEvent, tester.mockStateDB)

		tester.
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				require.Equal(t, 1, len(trx.SetCodeAuthorizations))

				authResult := trx.SetCodeAuthorizations[0]

				// R and S must be nil (empty), not 32 zero bytes.
				// This matches production native tracer behavior where zero big.Int.Bytes()
				// → empty slice → normalizeSignaturePoint → nil.
				assert.Nil(t, authResult.R, "R should be nil (not 32 zero bytes) for zero signature")
				assert.Nil(t, authResult.S, "S should be nil (not 32 zero bytes) for zero signature")
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

		txEvent := new(firehose.TxEventBuilder).
			Defaults().
			Type(firehose.TxTypeSetCode).
			AccessList(accessList).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{auth}).
			Build()

		tester := NewTracerTester(t).StartBlock()
		tester.tracer.OnTxStart(txEvent, tester.mockStateDB)

		tester.NonceChange(AliceAddr, 0, 1)

		tester.
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
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
