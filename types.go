package firehose

import (
	"encoding/hex"
	"errors"
	"math/big"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
)

// Minimal type system for Firehose Tracer
// Following the design principle: Accept [20]byte and [32]byte directly at boundaries
// to minimize type conversions when integrating with go-ethereum (common.Address IS [20]byte)

// BlockEvent contains the data needed for OnBlockStart
type BlockEvent struct {
	Block     BlockData
	Finalized *FinalizedBlockRef

	// FlashBlock is non-nil when this block is being processed as part of a flash block sequence.
	// Flash blocks are partial blocks emitted incrementally (used by Optimism/Katana).
	// Each iteration has a monotonically increasing Idx within the same block number.
	FlashBlock *FlashBlockData
}

// FlashBlockData contains flash block sequence metadata.
// Flash blocks are a mechanism used in Optimism/Katana where a single canonical block
// is built incrementally across multiple "flash" iterations. Each iteration adds more
// transactions to the block and is emitted as a partial block. The Idx field identifies
// the iteration number and must be strictly increasing within a given block number.
type FlashBlockData struct {
	Idx uint64 // Flash block index, monotonically increasing within the same block number

	// IsFinal is true when this is the final flash block iteration for the block number.
	// When true, the emitted FIRE BLOCK line encodes the index as Idx + 1000 (e.g. partial
	// indices 1..9 emit as 1..9, the final 10th partial emits as 1010), matching the
	// Optimism Geth firehose tracer behavior.
	IsFinal bool
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

	// EIP-4895: Shanghai withdrawals
	WithdrawalsRoot *[32]byte // Root hash of withdrawals tree (nil for pre-Shanghai blocks)

	// EIP-4844: Cancun blob gas tracking
	BlobGasUsed   *uint64 // Total blob gas consumed by blob transactions (nil for pre-Cancun blocks)
	ExcessBlobGas *uint64 // Running total of excess blob gas (nil for pre-Cancun blocks)

	// EIP-4788: Cancun beacon block root
	ParentBeaconRoot *[32]byte // Parent beacon block root for CL/EL sync (nil for pre-Cancun blocks)

	// EIP-7685: Prague execution requests
	RequestsHash *[32]byte // Root hash of execution layer requests (nil for pre-Prague blocks)

	// Polygon-specific: Transaction dependency metadata
	// List of transaction indexes that are dependent on each other in the block
	// Used by Polygon's parallel execution engine (nil for non-Polygon chains)
	TxDependency [][]uint64

	// SlotNumber was added by EIP-7843 and is ignored in legacy headers, it is scheduled to
	// be added in Amsterdam hard fork.
	SlotNumber *uint64
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

// Transaction types
const (
	TxTypeLegacy     = 0 // Legacy transaction (pre-EIP-2718)
	TxTypeAccessList = 1 // EIP-2930 access list transaction
	TxTypeDynamicFee = 2 // EIP-1559 dynamic fee transaction
	TxTypeBlob       = 3 // EIP-4844 blob transaction
	TxTypeSetCode    = 4 // EIP-7702 set code transaction
)

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

	// Signature fields
	V []byte   // Signature V value (can be nil for unsigned transactions)
	R [32]byte // Signature R point
	S [32]byte // Signature S point

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

// ReceiptData contains the minimal receipt data needed
type ReceiptData struct {
	TransactionIndex  uint32
	GasUsed           uint64
	Status            uint64
	Logs              []LogData
	LogsBloom         [256]byte
	CumulativeGasUsed uint64
	BlobGasUsed       uint64   // EIP-4844: Gas used for blob data
	BlobGasPrice      *big.Int // EIP-4844: Price per unit of blob gas
	StateRoot         []byte   // State root after transaction execution (for genesis blocks and pre-Byzantium)
}

// LogData contains log event data
type LogData struct {
	Address    [20]byte
	Topics     [][32]byte
	Data       []byte
	BlockIndex uint32 // Block-wide index of the log (prepopulated by chain implementation, matches go-ethereum behavior)
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
// These values match the EVM opcodes for the respective call types
type CallType byte

const (
	CallTypeCreate       CallType = 0xf0 // CREATE opcode
	CallTypeCall         CallType = 0xf1 // CALL opcode
	CallTypeCallCode     CallType = 0xf2 // CALLCODE opcode
	CallTypeDelegateCall CallType = 0xf4 // DELEGATECALL opcode
	CallTypeCreate2      CallType = 0xf5 // CREATE2 opcode
	CallTypeStaticCall   CallType = 0xfa // STATICCALL opcode
	CallTypeSelfDestruct CallType = 0xff // SELFDESTRUCT opcode (placeholder, handled specially)
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

// BigIntZero is a shared zero big.Int for convenience
var BigIntZero = big.NewInt(0)

// BigIntOne is a shared one big.Int for convenience
var BigIntOne = big.NewInt(1)

func bigInt(i int64) *big.Int { return big.NewInt(i) }

// bigIntToProtobuf converts a big.Int to protobuf BigInt
// Matches the semantics of firehoseBigIntFromNative in go-ethereum:
// - Returns nil for both nil and zero values
// - Only non-zero values get encoded
func bigIntToProtobuf(i *big.Int) *pbeth.BigInt {
	if i == nil || i.Sign() == 0 {
		return nil
	}
	return &pbeth.BigInt{Bytes: i.Bytes()}
}

// errorIsString checks if an error matches a target error message by walking the error chain
// This is NOT a replacement for errors.Is - it uses string matching to avoid importing vm package
// Geth errors when unwrapped will always lead to pure string comparison
func errorIsString(err error, target string) bool {
	if err == nil {
		return false
	}

	// Check current error message with exact string equality
	if err.Error() == target {
		return true
	}

	// Unwrap and check wrapped errors recursively
	if unwrapped := errors.Unwrap(err); unwrapped != nil {
		return errorIsString(unwrapped, target)
	}

	return false
}

var (
	// EmptyHash represents the zero hash (32 bytes of zeros)
	// Defined here for use in types and throughout the codebase
	EmptyHash = [32]byte{}
)

// GenesisAccount represents an account in the genesis block allocation
// This is a simplified version of go-ethereum's types.Account for tracer use
type GenesisAccount struct {
	// Code is the contract bytecode (if this is a contract account)
	Code []byte

	// Storage is the contract storage (key-value pairs)
	// Keys and values are 32-byte hashes
	Storage map[[32]byte][32]byte

	// Balance is the account balance in wei
	Balance *big.Int

	// Nonce is the account nonce
	Nonce uint64
}

// GenesisAlloc is a map of addresses to genesis accounts
// This represents the initial state allocation in the genesis block
type GenesisAlloc map[[20]byte]GenesisAccount
