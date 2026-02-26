package firehose

import (
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEIP7702_SignSetCodeAuthorization tests signature generation for EIP-7702 authorizations
// These tests are separate from tracer tests as they focus on cryptographic signature generation
func TestEIP7702_SignSetCodeAuthorization(t *testing.T) {
	chainID := uint256.NewInt(1) // Mainnet

	t.Run("sign_single_authorization", func(t *testing.T) {
		// Generate a private key for the authorizing account
		privateKey, err := crypto.GenerateKey()
		require.NoError(t, err, "Failed to generate private key")

		// Get the address from the private key
		authorizerAddress := crypto.PubkeyToAddress(privateKey.PublicKey)

		// Create delegation target address
		delegateAddress := common.HexToAddress("0x1234567890123456789012345678901234567890")

		// Create an authorization
		auth := types.SetCodeAuthorization{
			ChainID: *chainID,
			Address: delegateAddress,
			Nonce:   0,
		}

		// Sign the authorization
		signedAuth, err := types.SignSetCode(privateKey, auth)
		require.NoError(t, err, "Failed to sign authorization")

		// Verify the signature is valid
		assert.Equal(t, chainID, &signedAuth.ChainID, "ChainID should match")
		assert.Equal(t, delegateAddress, signedAuth.Address, "Address should match")
		assert.Equal(t, uint64(0), signedAuth.Nonce, "Nonce should match")
		// V is the recovery ID (0 or 1) - both are valid
		assert.Contains(t, []uint8{0, 1}, signedAuth.V, "V should be 0 or 1")
		assert.NotZero(t, signedAuth.R.Cmp(uint256.NewInt(0)), "R should be non-zero")
		assert.NotZero(t, signedAuth.S.Cmp(uint256.NewInt(0)), "S should be non-zero")

		// Recover the signer address from the signature
		recoveredAddress, err := signedAuth.Authority()
		require.NoError(t, err, "Failed to recover address from signature")
		assert.Equal(t, authorizerAddress, recoveredAddress, "Recovered address should match authorizer")
	})

	t.Run("sign_authorization_with_nonce", func(t *testing.T) {
		privateKey, err := crypto.GenerateKey()
		require.NoError(t, err)

		authorizerAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
		delegateAddress := common.HexToAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcd")

		// Create authorization with non-zero nonce
		auth := types.SetCodeAuthorization{
			ChainID: *chainID,
			Address: delegateAddress,
			Nonce:   42,
		}

		signedAuth, err := types.SignSetCode(privateKey, auth)
		require.NoError(t, err)

		assert.Equal(t, uint64(42), signedAuth.Nonce, "Nonce should be 42")

		// Verify recovery
		recoveredAddress, err := signedAuth.Authority()
		require.NoError(t, err)
		assert.Equal(t, authorizerAddress, recoveredAddress)
	})

	t.Run("multiple_authorizations_same_signer", func(t *testing.T) {
		privateKey, err := crypto.GenerateKey()
		require.NoError(t, err)

		authorizerAddress := crypto.PubkeyToAddress(privateKey.PublicKey)

		// Create multiple authorizations with different delegates
		delegate1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
		delegate2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

		auth1 := types.SetCodeAuthorization{
			ChainID: *chainID,
			Address: delegate1,
			Nonce:   0,
		}

		auth2 := types.SetCodeAuthorization{
			ChainID: *chainID,
			Address: delegate2,
			Nonce:   1,
		}

		signedAuth1, err := types.SignSetCode(privateKey, auth1)
		require.NoError(t, err)

		signedAuth2, err := types.SignSetCode(privateKey, auth2)
		require.NoError(t, err)

		// Both should recover to the same address
		recovered1, err := signedAuth1.Authority()
		require.NoError(t, err)
		recovered2, err := signedAuth2.Authority()
		require.NoError(t, err)

		assert.Equal(t, authorizerAddress, recovered1)
		assert.Equal(t, authorizerAddress, recovered2)
		assert.Equal(t, recovered1, recovered2, "Both should recover to same address")
	})

	t.Run("different_chain_ids_produce_different_signatures", func(t *testing.T) {
		privateKey, err := crypto.GenerateKey()
		require.NoError(t, err)

		delegateAddress := common.HexToAddress("0x1234567890123456789012345678901234567890")

		// Same authorization on different chains
		auth1 := types.SetCodeAuthorization{
			ChainID: *uint256.NewInt(1), // Mainnet
			Address: delegateAddress,
			Nonce:   0,
		}

		auth2 := types.SetCodeAuthorization{
			ChainID: *uint256.NewInt(11155111), // Sepolia
			Address: delegateAddress,
			Nonce:   0,
		}

		signedAuth1, err := types.SignSetCode(privateKey, auth1)
		require.NoError(t, err)

		signedAuth2, err := types.SignSetCode(privateKey, auth2)
		require.NoError(t, err)

		// Signatures should be different (R and S values)
		assert.NotEqual(t, signedAuth1.R, signedAuth2.R, "R values should differ for different chain IDs")
		assert.NotEqual(t, signedAuth1.S, signedAuth2.S, "S values should differ for different chain IDs")
	})

	t.Run("sig_hash_computation", func(t *testing.T) {
		// Test that SigHash is computed correctly
		auth := types.SetCodeAuthorization{
			ChainID: *uint256.NewInt(1),
			Address: common.HexToAddress("0x1234567890123456789012345678901234567890"),
			Nonce:   0,
		}

		hash1 := auth.SigHash()
		hash2 := auth.SigHash()

		// Same authorization should produce same hash
		assert.Equal(t, hash1, hash2, "Same authorization should produce same SigHash")

		// Different nonce should produce different hash
		auth.Nonce = 1
		hash3 := auth.SigHash()
		assert.NotEqual(t, hash1, hash3, "Different nonce should produce different SigHash")
	})
}

// TestEIP7702_SetCodeTransaction tests creating a full SetCode transaction
func TestEIP7702_SetCodeTransaction(t *testing.T) {
	t.Run("create_setcode_transaction", func(t *testing.T) {
		// Create signer private key (transaction sender)
		signerKey, err := crypto.GenerateKey()
		require.NoError(t, err)

		// Create authorizer private key (account delegating its code)
		authorizerKey, err := crypto.GenerateKey()
		require.NoError(t, err)

		delegateAddress := common.HexToAddress("0x1234567890123456789012345678901234567890")

		// Create and sign authorization
		auth := types.SetCodeAuthorization{
			ChainID: *uint256.NewInt(1),
			Address: delegateAddress,
			Nonce:   0,
		}

		signedAuth, err := types.SignSetCode(authorizerKey, auth)
		require.NoError(t, err)

		// Create SetCode transaction
		txData := &types.SetCodeTx{
			ChainID:    uint256.NewInt(1),
			Nonce:      0,
			GasTipCap:  uint256.NewInt(2),
			GasFeeCap:  uint256.NewInt(20),
			Gas:        100000,
			To:         common.HexToAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcd"),
			Value:      uint256.NewInt(100),
			Data:       []byte{},
			AccessList: types.AccessList{},
			AuthList:   []types.SetCodeAuthorization{signedAuth},
		}

		tx := types.NewTx(txData)

		// Sign the transaction with Prague signer (supports EIP-7702)
		signer := types.NewPragueSigner(big.NewInt(1))
		signedTx, err := types.SignTx(tx, signer, signerKey)
		require.NoError(t, err)

		// Verify transaction
		assert.Equal(t, uint8(types.SetCodeTxType), signedTx.Type(), "Transaction should be SetCodeTxType")
		assert.Equal(t, 1, len(signedTx.SetCodeAuthorizations()), "Should have one authorization")

		// Verify authorization is preserved
		txAuth := signedTx.SetCodeAuthorizations()[0]
		assert.Equal(t, delegateAddress, txAuth.Address, "Authorization address should match")
		assert.Equal(t, uint64(0), txAuth.Nonce, "Authorization nonce should match")
	})
}

// createSignedSetCodeAuth creates a signed SetCode authorization for testing
func createSignedSetCodeAuth(chainID *uint256.Int, delegateAddress common.Address, nonce uint64) (types.SetCodeAuthorization, *ecdsa.PrivateKey, error) {
	// Generate authorizer private key
	authorizerKey, err := crypto.GenerateKey()
	if err != nil {
		return types.SetCodeAuthorization{}, nil, err
	}

	// Create authorization
	auth := types.SetCodeAuthorization{
		ChainID: *chainID,
		Address: delegateAddress,
		Nonce:   nonce,
	}

	// Sign authorization
	signedAuth, err := types.SignSetCode(authorizerKey, auth)
	if err != nil {
		return types.SetCodeAuthorization{}, nil, err
	}

	return signedAuth, authorizerKey, nil
}
