package tests

import (
	"crypto/ecdsa"
	"encoding/hex"
	"math/big"

	firehose "github.com/streamingfast/evm-firehose-tracer-go"

	"github.com/ethereum/go-ethereum/crypto"
	eth "github.com/streamingfast/eth-go"
)

// Common test private keys and derived addresses
// These are deterministic keys for reproducible testing
var (
	// AliceKey is Alice's private key (deterministic for testing)
	AliceKey, _ = crypto.HexToECDSA("0000000000000000000000000000000000000000000000000000000000000001")
	// BobKey is Bob's private key (deterministic for testing)
	BobKey, _ = crypto.HexToECDSA("0000000000000000000000000000000000000000000000000000000000000002")
	// CharlieKey is Charlie's private key (deterministic for testing)
	CharlieKey, _ = crypto.HexToECDSA("0000000000000000000000000000000000000000000000000000000000000003")
	// MinerKey is the miner's private key (deterministic for testing)
	MinerKey, _ = crypto.HexToECDSA("0000000000000000000000000000000000000000000000000000000000000004")

	// Addresses derived from private keys (as [20]byte for tracer use)
	AliceAddr   = [20]byte(crypto.PubkeyToAddress(AliceKey.PublicKey))
	BobAddr     = [20]byte(crypto.PubkeyToAddress(BobKey.PublicKey))
	CharlieAddr = [20]byte(crypto.PubkeyToAddress(CharlieKey.PublicKey))
	MinerAddr   = [20]byte(crypto.PubkeyToAddress(MinerKey.PublicKey))
)

// GetTestPrivateKey returns a private key for a test address
// This is useful when you need to sign transactions or authorizations
func GetTestPrivateKey(addr [20]byte) *ecdsa.PrivateKey {
	if addr == AliceAddr {
		return AliceKey
	}
	if addr == BobAddr {
		return BobKey
	}
	if addr == CharlieAddr {
		return CharlieKey
	}
	if addr == MinerAddr {
		return MinerKey
	}
	return nil
}

// System call address constants (matching go-ethereum params package)
var (
	// SystemAddress is the address used as 'from' for system calls (0xfffffffffffffffffffffffffffffffffffffffe)
	SystemAddress = [20]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfe}

	// BeaconRootsAddress is the EIP-4788 beacon roots contract (0x000F3df6D732807Ef1319fB7B8bB8522d0Beac02)
	BeaconRootsAddress = [20]byte{0x00, 0x0F, 0x3d, 0xf6, 0xD7, 0x32, 0x80, 0x7E, 0xf1, 0x31, 0x9f, 0xB7, 0xB8, 0xbB, 0x85, 0x22, 0xd0, 0xBe, 0xac, 0x02}

	// HistoryStorageAddress is the EIP-2935/7709 parent block hash storage contract (0x0aae40965e6800cd9b1f4b05ff21581047e3f91e)
	HistoryStorageAddress = [20]byte{0x0a, 0xae, 0x40, 0x96, 0x5e, 0x68, 0x00, 0xcd, 0x9b, 0x1f, 0x4b, 0x05, 0xff, 0x21, 0x58, 0x10, 0x47, 0xe3, 0xf9, 0x1e}

	// WithdrawalQueueAddress is the EIP-7002 withdrawal queue contract
	WithdrawalQueueAddress = [20]byte{0x0b, 0x5d, 0xf4, 0x56, 0x89, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	// ConsolidationQueueAddress is the EIP-7251 consolidation queue contract
	ConsolidationQueueAddress = [20]byte{0x0c, 0x0d, 0x96, 0x10, 0x20, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
)

// bigInt creates a *big.Int from an int64 value
// This is a testing helper to reduce code clutter
func bigInt(n int64) *big.Int {
	return big.NewInt(n)
}

// mustBigInt creates a big.Int from a string (for large values that don't fit in int64)
func mustBigInt(s string) *big.Int {
	n := new(big.Int)
	n, ok := n.SetString(s, 10)
	if !ok {
		panic("invalid big int: " + s)
	}
	return n
}

// mustHash32FromHex converts a hex string to a [32]byte hash
// Panics if the hex string is invalid or not 32 bytes
func mustHash32FromHex(s string) [32]byte {
	// Remove 0x prefix if present
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		s = s[2:]
	}

	// Decode hex string
	bytes, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex string: " + s + ": " + err.Error())
	}

	// Check length
	if len(bytes) != 32 {
		panic("hex string must be 32 bytes, got " + string(rune(len(bytes))))
	}

	// Convert to [32]byte
	var hash [32]byte
	copy(hash[:], bytes)
	return hash
}

// successReceipt creates a successful receipt (status 1) with the given gas used
func successReceipt(gasUsed uint64) *firehose.ReceiptData {
	return &firehose.ReceiptData{
		TransactionIndex:  0,
		Status:            1,
		GasUsed:           gasUsed,
		CumulativeGasUsed: gasUsed,
	}
}

// failedReceipt creates a failed receipt (status 0) with the given gas used
func failedReceipt(gasUsed uint64) *firehose.ReceiptData {
	return &firehose.ReceiptData{
		TransactionIndex:  0,
		Status:            0,
		GasUsed:           gasUsed,
		CumulativeGasUsed: gasUsed,
	}
}

// hash32 creates a [32]byte hash from a uint64 value
// The value is encoded as big-endian in the last 8 bytes
// This is useful for creating storage keys/values in tests
func hash32(n uint64) [32]byte {
	var hash [32]byte
	for i := 0; i < 8; i++ {
		hash[31-i] = byte(n >> (i * 8))
	}
	return hash
}

// receiptWithLogs creates a successful receipt with logs
// receiptWithLogs creates a successful receipt (status 1) with logs
func receiptWithLogs(gasUsed uint64, logs []firehose.LogData) *firehose.ReceiptData {
	return &firehose.ReceiptData{
		TransactionIndex:  0,
		Status:            1,
		GasUsed:           gasUsed,
		CumulativeGasUsed: gasUsed,
		Logs:              logs,
	}
}

// failedReceiptWithLogs creates a failed receipt (status 0) with logs
func failedReceiptWithLogs(gasUsed uint64, logs []firehose.LogData) *firehose.ReceiptData {
	return &firehose.ReceiptData{
		TransactionIndex:  0,
		Status:            0,
		GasUsed:           gasUsed,
		CumulativeGasUsed: gasUsed,
		Logs:              logs,
	}
}

// receiptAt creates a receipt at a specific transaction index
func receiptAt(index uint32, status uint64, gasUsed uint64, cumulativeGas uint64, logs []firehose.LogData) *firehose.ReceiptData {
	return &firehose.ReceiptData{
		TransactionIndex:  index,
		Status:            status,
		GasUsed:           gasUsed,
		CumulativeGasUsed: cumulativeGas,
		Logs:              logs,
	}
}

// hashBytes computes keccak256 hash of bytes for code hashes
func hashBytes(data []byte) [32]byte {
	hash := eth.Keccak256(data)
	var result [32]byte
	copy(result[:], hash)
	return result
}

// Log helpers for creating test logs

// logData creates a firehose.LogData with the given address, topics, and data
func logData(addr [20]byte, topics [][32]byte, data []byte) firehose.LogData {
	return firehose.LogData{
		Address: addr,
		Topics:  topics,
		Data:    data,
	}
}

// log0 creates a log with 0 topics
func log0(addr [20]byte, data []byte) firehose.LogData {
	return logData(addr, nil, data)
}

// log1 creates a log with 1 topic
func log1(addr [20]byte, topic0 [32]byte, data []byte) firehose.LogData {
	return logData(addr, [][32]byte{topic0}, data)
}

// log2 creates a log with 2 topics
func log2(addr [20]byte, topic0, topic1 [32]byte, data []byte) firehose.LogData {
	return logData(addr, [][32]byte{topic0, topic1}, data)
}

// log3 creates a log with 3 topics
func log3(addr [20]byte, topic0, topic1, topic2 [32]byte, data []byte) firehose.LogData {
	return logData(addr, [][32]byte{topic0, topic1, topic2}, data)
}

// log4 creates a log with 4 topics
func log4(addr [20]byte, topic0, topic1, topic2, topic3 [32]byte, data []byte) firehose.LogData {
	return logData(addr, [][32]byte{topic0, topic1, topic2, topic3}, data)
}

// topic creates a [32]byte topic from a string for testing
func topic(s string) [32]byte {
	return hashBytes([]byte(s))
}
