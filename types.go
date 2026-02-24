package firehose

import (
	"encoding/hex"
	"math/big"
)

// Minimal type system for Firehose Tracer
// Following the design principle: Accept [20]byte and [32]byte directly at boundaries
// to minimize type conversions when integrating with go-ethereum (common.Address IS [20]byte)

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
