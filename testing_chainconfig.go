package firehose

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
)

// EthereumSetCodeAuthRecovery implements SetCodeAuthRecovery using go-ethereum's signature recovery
// This is the standard implementation for Ethereum and EVM-compatible chains
func EthereumSetCodeAuthRecovery(chainID [32]byte, address [20]byte, nonce uint64, v uint32, r, s [32]byte) ([20]byte, error) {
	// Convert chainID from [32]byte to *uint256.Int
	chainIDInt := uint256.NewInt(0)
	chainIDInt.SetBytes(chainID[:])

	// Convert R and S from [32]byte to *uint256.Int
	rInt := uint256.NewInt(0)
	rInt.SetBytes(r[:])

	sInt := uint256.NewInt(0)
	sInt.SetBytes(s[:])

	// Build a types.SetCodeAuthorization with the signature
	auth := types.SetCodeAuthorization{
		ChainID: *chainIDInt,
		Address: common.Address(address),
		Nonce:   nonce,
		V:       uint8(v),
		R:       *rInt,
		S:       *sInt,
	}

	// Use go-ethereum's Authority() method to recover the signer
	authority, err := auth.Authority()
	if err != nil {
		return [20]byte{}, fmt.Errorf("failed to recover authority: %w", err)
	}

	return [20]byte(authority), nil
}

// NewTestChainConfig creates a chain config for testing with Ethereum-style signature recovery
func NewTestChainConfig() *ChainConfig {
	return &ChainConfig{
		SetCodeAuthRecovery: EthereumSetCodeAuthRecovery,
	}
}
