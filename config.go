package firehose

import (
	"fmt"
	"io"
	"math/big"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
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

// StateReader provides read-only access to blockchain state during transaction execution
// Blockchain implementations must provide this interface to enable state-dependent tracing:
//   - EIP-7702 delegation detection (GetCode)
//   - CREATE address calculation (GetNonce)
//   - EIP-158 account existence checks (Exist)
//
// This is provided per-transaction via TxEvent.StateReader
type StateReader interface {
	// GetCode returns the code for the given address
	// Returns nil/empty slice if the address has no code (EOA or non-existent account)
	GetCode(addr [20]byte) []byte

	// GetNonce returns the nonce for the given address
	// Returns 0 if the address doesn't exist
	GetNonce(addr [20]byte) uint64

	// Exist returns true if the address exists in the state
	// An address exists if it has non-zero nonce, non-zero balance, or code
	Exist(addr [20]byte) bool
}

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

	// Block-number-based activation of the same forks, for chains that schedule them by
	// block number (e.g. Polygon PoS). A fork is active when either its time or its block
	// condition holds.
	ShanghaiBlock *big.Int
	CancunBlock   *big.Int
	PragueBlock   *big.Int

	// SetCodeAuthRecovery is a chain-specific function to recover authority from EIP-7702 authorizations
	// If nil, DefaultSetCodeAuthRecovery will be used (standard Ethereum ECDSA signature recovery)
	// Custom chains can override this if they use different signature schemes
	SetCodeAuthRecovery SetCodeAuthRecovery
}

// IsShanghai returns whether the given timestamp is >= Shanghai fork
func (c *ChainConfig) IsShanghai(num *big.Int, timestamp uint64) bool {
	return isTimestampForked(c.ShanghaiTime, timestamp) || isBlockForked(c.ShanghaiBlock, num)
}

// IsCancun returns whether the given timestamp is >= Cancun fork
func (c *ChainConfig) IsCancun(num *big.Int, timestamp uint64) bool {
	return isTimestampForked(c.CancunTime, timestamp) || isBlockForked(c.CancunBlock, num)
}

// IsPrague returns whether the given timestamp is >= Prague fork
func (c *ChainConfig) IsPrague(num *big.Int, timestamp uint64) bool {
	return isTimestampForked(c.PragueTime, timestamp) || isBlockForked(c.PragueBlock, num)
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
	IgnoreGenesisBlock       bool
	EnableConcurrentFlushing bool
	ConcurrentBufferSize     int
	// Output destination (defaults to os.Stdout)
	OutputWriter io.Writer

	// Chain-specific hooks, all optional.

	// AllowLogsOutsideCall accepts logs emitted while no call is active in a transaction
	// (e.g. Polygon's fee transfer log emitted after the root call ends). Such logs are
	// attached to the transaction's root call. When false, such a log is an invalid state.
	AllowLogsOutsideCall bool

	// IsNeverRevertedLog reports logs that stay in the receipt even when the call that
	// emitted them is reverted (e.g. Polygon's fee transfer log).
	IsNeverRevertedLog func(log *pbeth.Log) bool

	// BeforeBlockFlush is called with the completed block right before it is written out,
	// letting the chain rewrite it (e.g. Polygon merging its system transactions).
	BeforeBlockFlush func(block *pbeth.Block)
}

// LogKeyValues returns a flat list of key-value pairs suitable for structured logging,
// one pair per Config field (excluding ChainConfig, OutputWriter and function hooks).
// Keys are prefixed with "config_" and values are human-readable strings.
func (c *Config) LogKeyValues() []any {
	return []any{
		"config_ignore_genesis_block", fmt.Sprintf("%t", c.IgnoreGenesisBlock),
		"config_enable_concurrent_flushing", fmt.Sprintf("%t", c.EnableConcurrentFlushing),
		"config_concurrent_buffer_size", fmt.Sprintf("%d", c.ConcurrentBufferSize),
		"config_allow_logs_outside_call", fmt.Sprintf("%t", c.AllowLogsOutsideCall),
	}
}

// Helper function to check if a timestamp-based fork is active
func isTimestampForked(fork *uint64, timestamp uint64) bool {
	if fork == nil {
		return false
	}
	return *fork <= timestamp
}

// Helper function to check if a block-number-based fork is active
func isBlockForked(fork *big.Int, num *big.Int) bool {
	if fork == nil || num == nil {
		return false
	}
	return fork.Cmp(num) <= 0
}
