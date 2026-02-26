package firehose

import (
	"encoding/hex"
	"math/big"
)

// Minimal type system for Firehose Tracer
// Following the design principle: Accept [20]byte and [32]byte directly at boundaries
// to minimize type conversions when integrating with go-ethereum (common.Address IS [20]byte)

// BlockEvent contains the data needed for OnBlockStart
type BlockEvent struct {
	Block     BlockData
	Finalized *FinalizedBlockRef

	// Precompile Detection (at least one should be provided by chain implementation)
	// The chain implementation should provide precompile information since it varies by:
	// - Chain type (Ethereum, BSC, Polygon, etc.)
	// - Fork rules (Istanbul adds blake2f, Cancun adds point evaluation, etc.)
	// - Custom chain precompiles
	//
	// Option 1: Provide a pre-built checker function
	IsPrecompiledAddr func(addr [20]byte) bool
	//
	// Option 2: Provide the list of addresses (tracer will build the checker)
	ActivePrecompiles [][20]byte
	//
	// If neither is provided, all addresses will be treated as non-precompiled.
	//
	// Example for go-ethereum integration:
	//   import "github.com/ethereum/go-ethereum/core/vm"
	//   activePrecompiles := vm.ActivePrecompiles(blockRules)
	//   event.ActivePrecompiles = make([][20]byte, len(activePrecompiles))
	//   for i, addr := range activePrecompiles {
	//       event.ActivePrecompiles[i] = addr
	//   }
}

// BlockData contains the minimal block data needed by the tracer
type BlockData struct {
	Number      uint64
	Hash        [32]byte
	ParentHash  [32]byte
	UncleHash   [32]byte
	Coinbase    [20]byte
	Root        [32]byte
	TxHash      [32]byte
	ReceiptHash [32]byte
	Bloom       []byte // 256 bytes
	Difficulty  *big.Int
	GasLimit    uint64
	GasUsed     uint64
	Time        uint64
	Extra       []byte
	MixDigest   [32]byte
	Nonce       uint64
	BaseFee     *big.Int
	Uncles      []UncleData
	Size        uint64
	Withdrawals []WithdrawalData
	IsMerge     bool
}

// UncleData contains uncle block header data
type UncleData struct {
	Hash        [32]byte
	ParentHash  [32]byte
	UncleHash   [32]byte
	Coinbase    [20]byte
	Root        [32]byte
	TxHash      [32]byte
	ReceiptHash [32]byte
	Bloom       []byte
	Difficulty  *big.Int
	Number      uint64
	GasLimit    uint64
	GasUsed     uint64
	Time        uint64
	Extra       []byte
	MixDigest   [32]byte
	Nonce       uint64
	BaseFee     *big.Int
}

// WithdrawalData contains withdrawal data
type WithdrawalData struct {
	Index          uint64
	ValidatorIndex uint64
	Address        [20]byte
	Amount         uint64
}

// FinalizedBlockRef contains information about the finalized block
type FinalizedBlockRef struct {
	Number uint64
	Hash   [32]byte
}

// TxEvent contains the data needed for OnTxStart
type TxEvent struct {
	Type     uint8
	Hash     [32]byte
	From     [20]byte
	To       *[20]byte // nil for contract creation
	Input    []byte
	Value    *big.Int
	Gas      uint64
	GasPrice *big.Int
	Nonce    uint64
	Index    uint32

	// EIP-1559 fields (type 2)
	MaxFeePerGas         *big.Int
	MaxPriorityFeePerGas *big.Int

	// EIP-2930/EIP-1559 access list (type 1, 2)
	AccessList AccessList

	// EIP-4844 blob fields (type 3)
	BlobGasFeeCap *big.Int
	BlobHashes    [][32]byte

	// EIP-7702 set code authorization list (type 4)
	SetCodeAuthorizations []SetCodeAuthorization
}

// AccessList represents EIP-2930 access list
type AccessList []AccessTuple

// AccessTuple is a single entry in an access list
type AccessTuple struct {
	Address     [20]byte
	StorageKeys [][32]byte
}

// SetCodeAuthorization represents EIP-7702 authorization
type SetCodeAuthorization struct {
	ChainID [32]byte
	Address [20]byte
	Nonce   uint64
	V       uint32
	R       [32]byte
	S       [32]byte
}

// CallFrame contains the data for OnCallEnter/OnCallExit
type CallFrame struct {
	Type        CallType
	From        [20]byte
	To          [20]byte
	Input       []byte
	Gas         uint64
	Value       *big.Int
	CodeAddress *[20]byte // For DELEGATECALL
}

// CallType represents the type of call
type CallType int

const (
	CallTypeCall CallType = iota
	CallTypeCallCode
	CallTypeDelegateCall
	CallTypeStaticCall
	CallTypeCreate
	CallTypeCreate2
	CallTypeSelfDestruct
)

// Address represents an Ethereum address (20 bytes)
// Can be used interchangeably with common.Address from go-ethereum
type Address [20]byte

// Hash represents a 32-byte hash
// Can be used interchangeably with common.Hash from go-ethereum
type Hash [32]byte

// Bytes represents arbitrary byte slices
type Bytes []byte

// BlockNumber represents a block number
type BlockNumber uint64

// TxIndex represents a transaction index within a block
type TxIndex uint

// LogIndex represents a log index within a transaction
type LogIndex uint

// OpCode represents an EVM opcode
type OpCode byte

// GasAmount represents an amount of gas
type GasAmount uint64

// Helper methods for Address

// Hex returns the hex-encoded string representation of the address
func (a Address) Hex() string {
	return "0x" + hex.EncodeToString(a[:])
}

// Bytes returns the address as a byte slice
func (a Address) Bytes() []byte {
	return a[:]
}

// IsZero returns true if the address is the zero address
func (a Address) IsZero() bool {
	return a == Address{}
}

// Helper methods for Hash

// Hex returns the hex-encoded string representation of the hash
func (h Hash) Hex() string {
	return "0x" + hex.EncodeToString(h[:])
}

// Bytes returns the hash as a byte slice
func (h Hash) Bytes() []byte {
	return h[:]
}

// IsZero returns true if the hash is the zero hash
func (h Hash) IsZero() bool {
	return h == Hash{}
}

// EmptyAddress is the zero address
var EmptyAddress = Address{}

// EmptyHash is the zero hash
var EmptyHash = Hash{}

// BigIntZero is a shared zero big.Int for convenience
var BigIntZero = big.NewInt(0)

// BigIntOne is a shared one big.Int for convenience
var BigIntOne = big.NewInt(1)
