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

// bigInt creates a *big.Int from an int64 value
// This is a testing helper to reduce code clutter
func bigInt(n int64) *big.Int {
	return big.NewInt(n)
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
