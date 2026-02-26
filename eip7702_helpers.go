package firehose

import (
	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/holiman/uint256"
)

// CreateSignedSetCodeAuthWithKey creates a signed SetCode authorization using a specific private key
// This is useful when you need the authorization to be from a known address
func CreateSignedSetCodeAuthWithKey(privateKey *ecdsa.PrivateKey, chainID uint64, delegateAddress common.Address, nonce uint64) (types.SetCodeAuthorization, error) {
	auth := types.SetCodeAuthorization{
		ChainID: *uint256.NewInt(chainID),
		Address: delegateAddress,
		Nonce:   nonce,
	}

	return types.SignSetCode(privateKey, auth)
}

// CreateValidSetCodeTrxEvent creates a properly signed SetCode transaction for tracer testing
// This generates a new private key for the authorizer and returns both the event and the key
func CreateValidSetCodeTrxEvent() (*TxEvent, *ecdsa.PrivateKey, error) {
	// Generate key for the authorizer (the account delegating its code)
	authorizerKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, nil, err
	}

	// Create and sign authorization for CharlieAddr to delegate to
	auth := types.SetCodeAuthorization{
		ChainID: *uint256.NewInt(1),            // Chain ID 1
		Address: common.Address(CharlieAddr),   // Delegate to Charlie's address
		Nonce:   0,
	}

	signedAuth, err := types.SignSetCode(authorizerKey, auth)
	if err != nil {
		return nil, nil, err
	}

	// Build the transaction event with properly signed authorization
	// Convert uint256.Int to [32]byte for R and S
	var rBytes, sBytes [32]byte
	signedAuth.R.WriteToArray32(&rBytes)
	signedAuth.S.WriteToArray32(&sBytes)

	txEvent := new(TxEventBuilder).
		Type(TxTypeSetCode).
		Hash("0x0000000000000000000000000000000000000000000000000000000000000004"). // Placeholder, will be computed
		From(Alice).
		To(Bob).
		Value(bigInt(100)).
		Gas(21000).
		GasPrice(bigInt(10)).
		MaxFeePerGas(bigInt(20)).
		MaxPriorityFeePerGas(bigInt(2)).
		SetCodeAuthorizations([]SetCodeAuthorization{
			{
				ChainID: hashFromBytes(signedAuth.ChainID.Bytes()),
				Address: signedAuth.Address,
				Nonce:   signedAuth.Nonce,
				V:       uint32(signedAuth.V),
				R:       rBytes,
				S:       sBytes,
			},
		}).
		Nonce(0).
		Build()

	return &txEvent, authorizerKey, nil
}

// Helper to convert bytes to hash for SetCodeAuthorization
func hashFromBytes(b []byte) [32]byte {
	var hash [32]byte
	copy(hash[len(hash)-len(b):], b)
	return hash
}
