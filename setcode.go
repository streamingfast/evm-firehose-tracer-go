package firehose

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"

	"github.com/btcsuite/btcd/btcec/v2"
	btcecdsa "github.com/btcsuite/btcd/btcec/v2/ecdsa"
	eth "github.com/streamingfast/eth-go"
	"github.com/streamingfast/eth-go/rlp"
)

// secp256k1 returns the secp256k1 elliptic curve used by Ethereum
func secp256k1() elliptic.Curve {
	return btcec.S256()
}

// DefaultSetCodeAuthRecovery provides the default implementation for EIP-7702 SetCode
// authorization signature recovery using standard Ethereum cryptography.
//
// This implementation:
// - Computes the signature hash as: keccak256(0x05 || rlp([chainID, address, nonce]))
// - Validates the signature values (v, r, s)
// - Recovers the public key using ECDSA signature recovery
// - Derives the Ethereum address from the public key
//
// Returns the authority (signer) address or an error if signature recovery fails.
func DefaultSetCodeAuthRecovery(chainID [32]byte, address [20]byte, nonce uint64, v uint32, r, s [32]byte) ([20]byte, error) {
	// Compute signature hash: keccak256(0x05 || rlp([chainID, address, nonce]))
	sighash := computeSetCodeSigHash(chainID, address, nonce)

	// Convert r and s to big.Int for validation
	rBig := new(big.Int).SetBytes(r[:])
	sBig := new(big.Int).SetBytes(s[:])

	// Validate signature values according to Ethereum rules
	if !validateSignatureValues(byte(v), rBig, sBig) {
		return [20]byte{}, errors.New("invalid signature values")
	}

	// Recover public key using secp256k1 recovery
	pub, err := recoverPublicKey(sighash[:], rBig, sBig, byte(v))
	if err != nil {
		return [20]byte{}, fmt.Errorf("signature recovery failed: %w", err)
	}

	// Derive Ethereum address: keccak256(pub)[12:]
	hash := eth.Keccak256(pub)
	var addr [20]byte
	copy(addr[:], hash[12:])

	return addr, nil
}

// computeSetCodeSigHash computes the signature hash for EIP-7702 SetCode authorization.
// Hash = keccak256(0x05 || rlp([chainID, address, nonce]))
func computeSetCodeSigHash(chainID [32]byte, address [20]byte, nonce uint64) [32]byte {
	// Convert chainID to big.Int for RLP encoding
	chainIDBig := new(big.Int).SetBytes(chainID[:])

	// RLP encode the list [chainID, address, nonce]
	// Using slice of interfaces for the RLP encoder
	authData := []interface{}{
		chainIDBig,
		address[:],
		nonce,
	}

	rlpEncoded, err := rlp.Encode(authData)
	if err != nil {
		panic(fmt.Sprintf("failed to RLP encode SetCode authorization: %v", err))
	}

	// Prepend 0x05 prefix byte
	prefixedData := make([]byte, 1+len(rlpEncoded))
	prefixedData[0] = 0x05
	copy(prefixedData[1:], rlpEncoded)

	// Hash the complete structure using eth-go's Keccak256
	hash := eth.Keccak256(prefixedData)

	var result [32]byte
	copy(result[:], hash)
	return result
}

// validateSignatureValues validates ECDSA signature values according to Ethereum rules
func validateSignatureValues(v byte, r, s *big.Int) bool {
	// secp256k1 curve parameters
	secp256k1N := new(big.Int)
	secp256k1N.SetString("fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", 16)
	secp256k1halfN := new(big.Int).Div(secp256k1N, big.NewInt(2))

	// Validate recovery ID
	if v != 0 && v != 1 && v != 27 && v != 28 {
		return false
	}

	// Validate r and s are in valid range
	if r.Sign() <= 0 || s.Sign() <= 0 {
		return false
	}
	if r.Cmp(secp256k1N) >= 0 || s.Cmp(secp256k1N) >= 0 {
		return false
	}

	// EIP-2: Reject high s-values (for malleability)
	if s.Cmp(secp256k1halfN) > 0 {
		return false
	}

	return true
}

// recoverPublicKey recovers the public key from an ECDSA signature
// Returns the uncompressed public key bytes (64 bytes: X + Y coordinates)
func recoverPublicKey(hash []byte, r, s *big.Int, v byte) ([]byte, error) {
	// Normalize v to 0 or 1
	recID := v
	if recID >= 27 {
		recID -= 27
	}
	if recID != 0 && recID != 1 {
		return nil, errors.New("invalid recovery id")
	}

	// Encode signature in compact format for btcec recovery
	// Compact signature format: [recoveryID + 27][R (32 bytes)][S (32 bytes)]
	var sigBytes [65]byte
	sigBytes[0] = recID + 27
	r.FillBytes(sigBytes[1:33])
	s.FillBytes(sigBytes[33:65])

	// Use btcec ecdsa package to recover the public key
	pubKey, _, err := btcecdsa.RecoverCompact(sigBytes[:], hash)
	if err != nil {
		return nil, fmt.Errorf("btcec recovery failed: %w", err)
	}

	// Return uncompressed public key (64 bytes: X + Y coordinates)
	// btcec returns (*btcec.PublicKey) which has X() and Y() methods
	result := make([]byte, 64)
	pubKey.X().FillBytes(result[0:32])
	pubKey.Y().FillBytes(result[32:64])

	return result, nil
}

// SignSetCodeAuth creates a signed SetCode authorization using the provided private key.
// This is the signing counterpart to DefaultSetCodeAuthRecovery.
//
// Parameters:
//   - privateKey: ECDSA private key to sign with
//   - chainID: Chain ID (e.g., 1 for Ethereum mainnet)
//   - delegateAddress: Address of the contract to delegate code execution to
//   - nonce: Nonce of the authorizing account
//
// Returns a SetCodeAuthorization with populated signature fields (V, R, S).
func SignSetCodeAuth(privateKey *ecdsa.PrivateKey, chainID uint64, delegateAddress [20]byte, nonce uint64) (SetCodeAuthorization, error) {
	// Convert chainID to 32-byte array
	chainIDBig := new(big.Int).SetUint64(chainID)
	var chainIDBytes [32]byte
	chainIDBig.FillBytes(chainIDBytes[:])

	// Compute signature hash
	sighash := computeSetCodeSigHash(chainIDBytes, delegateAddress, nonce)

	// Sign the hash using ECDSA
	rBig, sBig, err := signECDSA(privateKey, sighash[:])
	if err != nil {
		return SetCodeAuthorization{}, err
	}

	// Determine recovery ID (v)
	v, err := findRecoveryID(privateKey, sighash[:], rBig, sBig)
	if err != nil {
		return SetCodeAuthorization{}, err
	}

	// Convert to fixed-size arrays
	var r, s [32]byte
	rBig.FillBytes(r[:])
	sBig.FillBytes(s[:])

	return SetCodeAuthorization{
		ChainID: chainIDBytes,
		Address: delegateAddress,
		Nonce:   nonce,
		V:       uint32(v),
		R:       r,
		S:       s,
	}, nil
}

// signECDSA signs a hash using ECDSA and returns r and s components
func signECDSA(privateKey *ecdsa.PrivateKey, hash []byte) (*big.Int, *big.Int, error) {
	// Use standard ECDSA signing with cryptographic random source
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hash)
	if err != nil {
		return nil, nil, err
	}

	// Ensure s is in lower half of curve order (EIP-2 malleability protection)
	secp256k1N := secp256k1().Params().N
	secp256k1halfN := new(big.Int).Div(secp256k1N, big.NewInt(2))
	if s.Cmp(secp256k1halfN) > 0 {
		s = new(big.Int).Sub(secp256k1N, s)
	}

	return r, s, nil
}

// findRecoveryID determines the recovery ID (v) needed to recover the public key
func findRecoveryID(privateKey *ecdsa.PrivateKey, hash []byte, r, s *big.Int) (byte, error) {
	// Get expected public key
	expectedPubKey := make([]byte, 64)
	xBytes := privateKey.PublicKey.X.Bytes()
	yBytes := privateKey.PublicKey.Y.Bytes()
	copy(expectedPubKey[32-len(xBytes):32], xBytes)
	copy(expectedPubKey[64-len(yBytes):64], yBytes)

	// Try both recovery IDs (0 and 1)
	for v := byte(0); v <= 1; v++ {
		recovered, err := recoverPublicKey(hash, r, s, v)
		if err != nil {
			continue
		}

		// Check if recovered key matches expected key
		if len(recovered) == 64 {
			match := true
			for i := 0; i < 64; i++ {
				if recovered[i] != expectedPubKey[i] {
					match = false
					break
				}
			}
			if match {
				return v, nil
			}
		}
	}

	return 0, errors.New("could not determine recovery id")
}
