package firehose

import (
	"io"
	"math/big"
)

// SetCodeAuthRecovery is a function that recovers the authority (signer address) from a SetCodeAuthorization
// This is chain-specific because different chains may have different signature schemes
// For Ethereum/EVM chains, this typically uses ECDSA signature recovery
//
// Parameters:
//   - chainID: The chain ID used in the authorization
//   - address: The address being delegated to
//   - nonce: The nonce in the authorization
//   - v, r, s: The signature components
//
// Returns:
//   - [20]byte: The recovered authority address (signer)
//   - error: Error if signature recovery fails
type SetCodeAuthRecovery func(chainID [32]byte, address [20]byte, nonce uint64, v uint32, r, s [32]byte) ([20]byte, error)

// ChainConfig defines the chain configuration for the tracer
// Simplified version - assumes all historical forks are active
// Only tracks future timestamp-based forks that may affect tracing behavior
type ChainConfig struct {
	ChainID *big.Int

	// Timestamp-based forks (nil = not activated, 0 = activated at genesis)
	// These are kept for potential future tracing behavior changes
	ShanghaiTime *uint64 // EIP-3651, EIP-3855, EIP-3860, EIP-4895 (withdrawals)
	CancunTime   *uint64 // EIP-4844 (blobs), EIP-1153 (transient storage), EIP-5656, EIP-6780
	PragueTime   *uint64 // EIP-7702 (set code), EIP-2537 (BLS precompile)
	VerkleTime   *uint64 // Verkle tree transition (future)

	// SetCodeAuthRecovery is a chain-specific function to recover authority from EIP-7702 authorizations
	// If nil, SetCode authorizations will not have their Authority field populated
	// Chain implementations should provide this to enable proper SetCode validation
	SetCodeAuthRecovery SetCodeAuthRecovery
}

// IsShanghai returns whether the given timestamp is >= Shanghai fork
func (c *ChainConfig) IsShanghai(num *big.Int, timestamp uint64) bool {
	return isTimestampForked(c.ShanghaiTime, timestamp)
}

// IsCancun returns whether the given timestamp is >= Cancun fork
func (c *ChainConfig) IsCancun(num *big.Int, timestamp uint64) bool {
	return isTimestampForked(c.CancunTime, timestamp)
}

// IsPrague returns whether the given timestamp is >= Prague fork
func (c *ChainConfig) IsPrague(num *big.Int, timestamp uint64) bool {
	return isTimestampForked(c.PragueTime, timestamp)
}

// IsVerkle returns whether the given timestamp is >= Verkle fork
func (c *ChainConfig) IsVerkle(num *big.Int, timestamp uint64) bool {
	return isTimestampForked(c.VerkleTime, timestamp)
}

// Rules wraps ChainConfig and provides block-scoped fork flags
// Computed ONCE per block, then passed to tracer hooks
// Simplified - only tracks what's actually needed for tracing behavior
type Rules struct {
	ChainID *big.Int

	// Timestamp-based forks
	IsMerge    bool // Post-merge (PoS)
	IsShanghai bool // EIP-4895 withdrawals
	IsCancun   bool // EIP-4844 blobs, EIP-1153 transient storage
	IsPrague   bool // EIP-7702 set code
	IsVerkle   bool // Verkle tree transition
}

// Rules computes the active fork rules for a specific block
// Note: All historical block-based forks (Homestead, Berlin, London, etc.) are assumed active
func (c *ChainConfig) Rules(num *big.Int, isMerge bool, timestamp uint64) Rules {
	return Rules{
		ChainID:    c.ChainID,
		IsMerge:    isMerge,
		IsShanghai: c.IsShanghai(num, timestamp),
		IsCancun:   c.IsCancun(num, timestamp),
		IsPrague:   c.IsPrague(num, timestamp),
		IsVerkle:   c.IsVerkle(num, timestamp),
	}
}

// Config holds tracer runtime configuration
type Config struct {
	// Chain configuration (fork activation rules)
	ChainConfig *ChainConfig

	// Feature flags
	EnableConcurrentFlushing bool
	ConcurrentBufferSize     int

	// Output destination (defaults to os.Stdout)
	OutputWriter io.Writer
}

// Helper function to check if a timestamp-based fork is active
func isTimestampForked(fork *uint64, timestamp uint64) bool {
	if fork == nil {
		return false
	}
	return *fork <= timestamp
}
