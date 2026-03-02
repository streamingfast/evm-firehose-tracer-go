package firehose

import (
	"encoding/hex"
	"math/big"
)

// BlockEventBuilder provides a fluent API for building blocks
type BlockEventBuilder struct {
	number     uint64
	hash       [32]byte
	parentHash [32]byte
	timestamp  uint64
	coinbase   [20]byte
	gasLimit   uint64
	difficulty *big.Int
	size       uint64
	bloom      []byte
}

// Number sets the block number
func (b *BlockEventBuilder) Number(number uint64) *BlockEventBuilder {
	b.number = number
	return b
}

// firehose.Hash sets the block hash
func (b *BlockEventBuilder) Hash(hash string) *BlockEventBuilder {
	b.hash = hashFromHex(hash)
	return b
}

// ParentHash sets the parent hash
func (b *BlockEventBuilder) ParentHash(hash string) *BlockEventBuilder {
	b.parentHash = hashFromHex(hash)
	return b
}

// Timestamp sets the timestamp
func (b *BlockEventBuilder) Timestamp(ts uint64) *BlockEventBuilder {
	b.timestamp = ts
	return b
}

// Coinbase sets the coinbase address
func (b *BlockEventBuilder) Coinbase(addr string) *BlockEventBuilder {
	b.coinbase = addressFromHex(addr)
	return b
}

// GasLimit sets the gas limit
func (b *BlockEventBuilder) GasLimit(limit uint64) *BlockEventBuilder {
	b.gasLimit = limit
	return b
}

// Difficulty sets the difficulty
func (b *BlockEventBuilder) Difficulty(difficulty *big.Int) *BlockEventBuilder {
	b.difficulty = difficulty
	return b
}

// Size sets the block size
func (b *BlockEventBuilder) Size(size uint64) *BlockEventBuilder {
	b.size = size
	return b
}

// Bloom sets the logs bloom filter
func (b *BlockEventBuilder) Bloom(bloom []byte) *BlockEventBuilder {
	b.bloom = bloom
	return b
}

// Build creates a BlockEvent
func (b *BlockEventBuilder) Build() BlockEvent {
	// IsMerge is true when difficulty is 0 (PoS blocks)
	// This matches go-ethereum's blockIsMerge() logic
	isMerge := b.difficulty != nil && b.difficulty.Sign() == 0

	return BlockEvent{
		Block: BlockData{
			Number:     b.number,
			Hash:       b.hash,
			ParentHash: b.parentHash,
			Time:       b.timestamp,
			Coinbase:   b.coinbase,
			GasLimit:   b.gasLimit,
			Difficulty: b.difficulty,
			IsMerge:    isMerge,
			Size:       b.size,
			Bloom:      b.bloom,
		},
		Finalized: nil,
	}
}

// TxEventBuilder provides a fluent API for building transactions
type TxEventBuilder struct {
	txType                uint8
	hash                  [32]byte
	from                  [20]byte
	to                    [20]byte
	value                 *big.Int
	gas                   uint64
	gasPrice              *big.Int
	nonce                 uint64
	data                  []byte
	v                     []byte
	r                     [32]byte
	s                     [32]byte
	maxFeePerGas          *big.Int
	maxPriorityFeePerGas  *big.Int
	accessList            AccessList
	blobGasFeeCap         *big.Int
	blobHashes            [][32]byte
	setCodeAuthorizations []SetCodeAuthorization
}

func NewTxEventBuilderFrom(other TxEvent) *TxEventBuilder {
	return new(TxEventBuilder).
		DefaultsFrom(other)
}

// Type sets the transaction type
func (t *TxEventBuilder) Type(txType uint8) *TxEventBuilder {
	t.txType = txType
	return t
}

// Hash sets the transaction hash
func (t *TxEventBuilder) Hash(hash string) *TxEventBuilder {
	t.hash = hashFromHex(hash)
	return t
}

// From sets the sender address
func (t *TxEventBuilder) From(addr string) *TxEventBuilder {
	t.from = addressFromHex(addr)
	return t
}

// To sets the recipient address
func (t *TxEventBuilder) To(addr string) *TxEventBuilder {
	t.to = addressFromHex(addr)
	return t
}

// Value sets the value in wei
func (t *TxEventBuilder) Value(value *big.Int) *TxEventBuilder {
	t.value = value
	return t
}

// Amount sets the value in ETH (helper)
func (t *TxEventBuilder) Amount(eth float64) *TxEventBuilder {
	t.value = ethToWei(eth)
	return t
}

// Gas sets the gas limit
func (t *TxEventBuilder) Gas(gas uint64) *TxEventBuilder {
	t.gas = gas
	return t
}

// GasPrice sets the gas price in wei
func (t *TxEventBuilder) GasPrice(price *big.Int) *TxEventBuilder {
	t.gasPrice = price
	return t
}

// Nonce sets the nonce
func (t *TxEventBuilder) Nonce(nonce uint64) *TxEventBuilder {
	t.nonce = nonce
	return t
}

// Data sets the transaction data
func (t *TxEventBuilder) Data(data []byte) *TxEventBuilder {
	t.data = data
	return t
}

// MaxFeePerGas sets the max fee per gas (EIP-1559)
func (t *TxEventBuilder) MaxFeePerGas(maxFee *big.Int) *TxEventBuilder {
	t.maxFeePerGas = maxFee
	return t
}

// MaxPriorityFeePerGas sets the max priority fee per gas (EIP-1559)
func (t *TxEventBuilder) MaxPriorityFeePerGas(maxPriorityFee *big.Int) *TxEventBuilder {
	t.maxPriorityFeePerGas = maxPriorityFee
	return t
}

// AccessList sets the access list (EIP-2930/EIP-1559)
func (t *TxEventBuilder) AccessList(accessList AccessList) *TxEventBuilder {
	t.accessList = accessList
	return t
}

// BlobGasFeeCap sets the blob gas fee cap (EIP-4844)
func (t *TxEventBuilder) BlobGasFeeCap(feeCap *big.Int) *TxEventBuilder {
	t.blobGasFeeCap = feeCap
	return t
}

// BlobHashes sets the blob hashes (EIP-4844)
func (t *TxEventBuilder) BlobHashes(hashes [][32]byte) *TxEventBuilder {
	t.blobHashes = hashes
	return t
}

// SetCodeAuthorizations sets the set code authorization list (EIP-7702)
func (t *TxEventBuilder) SetCodeAuthorizations(authList []SetCodeAuthorization) *TxEventBuilder {
	t.setCodeAuthorizations = authList
	return t
}

// V sets the signature V value
func (t *TxEventBuilder) V(v []byte) *TxEventBuilder {
	t.v = v
	return t
}

// R sets the signature R point
func (t *TxEventBuilder) R(r string) *TxEventBuilder {
	t.r = hashFromHex(r)
	return t
}

// S sets the signature S point
func (t *TxEventBuilder) S(s string) *TxEventBuilder {
	t.s = hashFromHex(s)
	return t
}

// Defaults sets common default values for testing:
// - Type: Legacy (0)
// - Hash: Zero hash (usually computed by native validator)
// - From: Alice
// - To: Bob
// - Value: 100 wei
// - Gas: 21000 (standard transfer)
// - GasPrice: 10 wei
// - Nonce: 0
// - MaxFeePerGas: 20 wei (for EIP-1559 transactions)
// - MaxPriorityFeePerGas: 2 wei (for EIP-1559 transactions)
//
// Does NOT set: AccessList, BlobGasFeeCap, BlobHashes, SetCodeAuthorizations
// (these should be set explicitly when needed)
func (t *TxEventBuilder) Defaults() *TxEventBuilder {
	return t.
		Type(TxTypeLegacy).
		Hash("0x0000000000000000000000000000000000000000000000000000000000000000").
		// From(Alice).
		// To(Bob).
		Value(bigInt(100)).
		Gas(21000).
		GasPrice(bigInt(10)).
		Nonce(0).
		MaxFeePerGas(bigInt(20)).
		MaxPriorityFeePerGas(bigInt(2))
}

func (t *TxEventBuilder) DefaultsFrom(other TxEvent) *TxEventBuilder {
	return t.
		Type(other.Type).
		Hash(hex.EncodeToString(other.Hash[:])).
		From(hex.EncodeToString(other.From[:])).
		To(hex.EncodeToString(other.To[:])).
		Value(other.Value).
		Gas(other.Gas).
		GasPrice(other.GasPrice).
		Nonce(other.Nonce).
		MaxFeePerGas(other.MaxFeePerGas).
		MaxPriorityFeePerGas(other.MaxPriorityFeePerGas)
}

func (t *TxEventBuilder) Build() TxEvent {
	return TxEvent{
		Type:                  t.txType,
		Hash:                  t.hash,
		From:                  t.from,
		To:                    &t.to,
		Value:                 t.value,
		Gas:                   t.gas,
		GasPrice:              t.gasPrice,
		Nonce:                 t.nonce,
		Input:                 t.data,
		V:                     t.v,
		R:                     t.r,
		S:                     t.s,
		MaxFeePerGas:          t.maxFeePerGas,
		MaxPriorityFeePerGas:  t.maxPriorityFeePerGas,
		AccessList:            t.accessList,
		BlobGasFeeCap:         t.blobGasFeeCap,
		BlobHashes:            t.blobHashes,
		SetCodeAuthorizations: t.setCodeAuthorizations,
	}
}

// Helper functions

// ethToWei converts ETH to wei
func ethToWei(eth float64) *big.Int {
	// 1 ETH = 10^18 wei
	wei := new(big.Float).Mul(big.NewFloat(eth), big.NewFloat(1e18))
	result, _ := wei.Int(nil)
	return result
}

// hashFromHex converts a hex string to a 32-byte hash
func hashFromHex(s string) [32]byte {
	// Remove 0x prefix if present
	if len(s) >= 2 && s[0] == '0' && s[1] == 'x' {
		s = s[2:]
	}

	var hash [32]byte
	// Pad with zeros if too short
	for len(s) < 64 {
		s = "0" + s
	}

	for i := 0; i < 32; i++ {
		hash[i] = hexToByte(s[i*2], s[i*2+1])
	}

	return hash
}

// addressFromHex converts a hex string to a 20-byte address
func addressFromHex(s string) [20]byte {
	// Remove 0x prefix if present
	if len(s) >= 2 && s[0] == '0' && s[1] == 'x' {
		s = s[2:]
	}

	var addr [20]byte
	// Pad with zeros if too short
	for len(s) < 40 {
		s = "0" + s
	}

	for i := 0; i < 20; i++ {
		addr[i] = hexToByte(s[i*2], s[i*2+1])
	}

	return addr
}

// hexToByte converts two hex characters to a byte
func hexToByte(c1, c2 byte) byte {
	return hexCharToByte(c1)<<4 | hexCharToByte(c2)
}

// hexCharToByte converts a single hex character to a byte
func hexCharToByte(c byte) byte {
	if c >= '0' && c <= '9' {
		return c - '0'
	}
	if c >= 'a' && c <= 'f' {
		return c - 'a' + 10
	}
	if c >= 'A' && c <= 'F' {
		return c - 'A' + 10
	}
	return 0
}
