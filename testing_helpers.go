package firehose

import (
	"math/big"
)

// Common test address constants
var (
	AliceAddr   = addressFromHex(Alice)
	BobAddr     = addressFromHex(Bob)
	CharlieAddr = addressFromHex(Charlie)
	MinerAddr   = addressFromHex(Miner)
)

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

// successReceipt creates a successful receipt (status 1) with the given gas used
func successReceipt(gasUsed uint64) *ReceiptData {
	return &ReceiptData{
		Status:  1,
		GasUsed: gasUsed,
	}
}

// failedReceipt creates a failed receipt (status 0) with the given gas used
func failedReceipt(gasUsed uint64) *ReceiptData {
	return &ReceiptData{
		Status:  0,
		GasUsed: gasUsed,
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
func receiptWithLogs(gasUsed uint64, logs []LogData) *ReceiptData {
	return &ReceiptData{
		Status:  1,
		GasUsed: gasUsed,
		Logs:    logs,
	}
}
