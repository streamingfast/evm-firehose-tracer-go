package firehose

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"math/big"
	"strconv"
	"strings"
	"testing"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// TestBlock provides a standard test block with reasonable defaults
// This block represents block #100 with typical Ethereum mainnet settings
//
// IMPORTANT: Until native validator code is removed, this block MUST produce
// the exact same hash and size as the native Geth tracer would compute.
// The hash below is the real Keccak256 hash of the block header with these exact parameters.
// The size is the RLP-encoded size that Geth computes for this block.
// If you change any block parameters (timestamp, coinbase, etc.), you MUST recompute
// the hash and size by running the test and copying values from the native tracer output.
var TestBlock = (&BlockEventBuilder{}).
	Number(100).
	Hash("0x1a8717837b7c5f4f566e842ede0fbea43334985922b6cb2c0aee8cdd9d2155ab"). // Computed by native Geth tracer
	ParentHash("0x0000000000000000000000000000000000000000000000000000000000000063").
	Timestamp(1704067200).
	Coinbase(Miner).
	GasLimit(30_000_000). // 30M gas (standard Ethereum mainnet)
	Difficulty(big.NewInt(0)).
	Size(509).               // RLP-encoded block size computed by Geth
	Bloom(make([]byte, 256)). // Empty 256-byte logs bloom filter
	Build()

// TestTrx provides a standard test transaction with reasonable defaults
// This transaction represents a simple transfer
//
// IMPORTANT: Until native validator code is removed, this transaction MUST produce
// the exact same hash as the native Geth tracer would compute for testing purposes.
// The hash below is computed by the native Geth tracer for this exact transaction.
// If you change any transaction parameters, you MUST recompute the hash by running
// the test and copying the value from the native tracer output.
var TestTrx = new(TxEventBuilder).
	Hash("0xb4b19a89bd0181f4a102a778c0163f0a677a09fb6b7ad61d0bff56327115c339"). // Computed by native Geth tracer
	From(Alice).
	To(Bob).
	Value(big.NewInt(1000000000000000000)). // 1 ETH in wei
	Gas(21000).                              // Standard gas for simple transfer
	GasPrice(gweiToWei(20)).                 // 20 gwei
	Nonce(0).
	Build()

// TestBlockScenario provides a fluent API for building test scenarios
type TestBlockScenario struct {
	t *testing.T

	Block  *BlockEventBuilder
	Tracer *Tracer
	Buffer *bytes.Buffer
}

// NewBlockScenario creates a new scenario builder
func NewBlockScenario(t *testing.T) *TestBlockScenario {
	buffer := &bytes.Buffer{}

	scenario := &TestBlockScenario{
		t:      t,
		Buffer: buffer,
		Tracer: NewTracer(&Config{
			ChainConfig: &ChainConfig{
				ChainID: big.NewInt(1),
			},
			OutputWriter: buffer,
		}),
	}

	var err error
	scenario.Tracer.nativeValidator, err = newNativeValidator("")
	require.NoError(t, err, "creating native validator")

	scenario.Tracer.OnBlockchainInit("test", "1.0.0", scenario.Tracer.chainConfig)

	return scenario
}

// Tweak allows modifying the scenario using a custom function
func (s *TestBlockScenario) Tweak(transform func(*TestBlockScenario)) *TestBlockScenario {
	transform(s)
	return s
}

func (s *TestBlockScenario) StartBlock() *TestBlockScenario {
	s.Tracer.OnBlockStart(TestBlock)
	return s
}

// StartBlockTrx starts a block and a transaction
func (s *TestBlockScenario) StartBlockTrx() *TestBlockScenario {
	s.Tracer.OnBlockStart(TestBlock)
	s.Tracer.OnTxStart(TestTrx)
	return s
}

// EndBlockTrx ends the transaction and block with an optional error
func (s *TestBlockScenario) EndBlockTrx(receipt *ReceiptData, txErr, blockErr error) *TestBlockScenario {
	s.Tracer.OnTxEnd(receipt, txErr)
	s.Tracer.OnBlockEnd(blockErr)
	return s
}

func (s *TestBlockScenario) EndBlock(err error) *TestBlockScenario {
	s.Tracer.OnBlockEnd(err)
	return s
}

func (s *TestBlockScenario) Validate(validateFunc func(block *pbeth.Block)) {
	block := ParseFirehoseBlock(s.t, "shared tracer", s.Buffer)
	nativeBlock := ParseFirehoseBlock(s.t, "native tracer", s.Tracer.nativeValidator.tracer.InternalTestingBuffer())

	if !proto.Equal(block, nativeBlock) {
		s.t.Logf("Shared tracer buffer content:\n%s", s.Buffer.String())
		s.t.Logf("Native tracer buffer content:\n%s", s.Tracer.nativeValidator.tracer.InternalTestingBuffer().String())
		require.EqualExportedValues(s.t, nativeBlock, block)
	}

	validateFunc(block)
}

// BlockAssertion is a function that asserts on a block
type BlockAssertion func(t *testing.T, block *pbeth.Block)

// BlockAssertions provides fluent assertions for blocks
type BlockAssertions struct {
	t     *testing.T
	block *pbeth.Block
}

// Number asserts the block number
func (a *BlockAssertions) Number(expected uint64) *BlockAssertions {
	if a.block.Number != expected {
		a.t.Errorf("expected block number %d, got %d", expected, a.block.Number)
	}
	return a
}

// Hash asserts the block hash
func (a *BlockAssertions) Hash(expected string) *BlockAssertions {
	expectedHash := hashFromHex(expected)
	if string(a.block.Hash) != string(expectedHash[:]) {
		a.t.Errorf("expected block hash %s, got %x", expected, a.block.Hash)
	}
	return a
}

// HasNoTransactions asserts the block has no transactions
func (a *BlockAssertions) HasNoTransactions() *BlockAssertions {
	if len(a.block.TransactionTraces) != 0 {
		a.t.Errorf("expected no transactions, got %d", len(a.block.TransactionTraces))
	}
	return a
}

// HasTransactions asserts the block has the expected number of transactions
func (a *BlockAssertions) HasTransactions(count int) *BlockAssertions {
	if len(a.block.TransactionTraces) != count {
		a.t.Errorf("expected %d transactions, got %d", count, len(a.block.TransactionTraces))
	}
	return a
}

// HasHeader asserts the block has a header
func (a *BlockAssertions) HasHeader() *BlockAssertions {
	if a.block.Header == nil {
		a.t.Error("expected block to have a header")
	}
	return a
}

// GasLimit asserts the gas limit
func (a *BlockAssertions) GasLimit(expected uint64) *BlockAssertions {
	if a.block.Header == nil {
		a.t.Error("block has no header")
		return a
	}
	if a.block.Header.GasLimit != expected {
		a.t.Errorf("expected gas limit %d, got %d", expected, a.block.Header.GasLimit)
	}
	return a
}

// Version asserts the protocol version is set
func (a *BlockAssertions) HasVersion() *BlockAssertions {
	if a.block.Ver == 0 {
		a.t.Error("expected block to have a version set")
	}
	return a
}

// Transaction returns assertions for a specific transaction
func (a *BlockAssertions) Transaction(index int) *TransactionAssertions {
	if index >= len(a.block.TransactionTraces) {
		a.t.Errorf("transaction index %d out of range (block has %d transactions)", index, len(a.block.TransactionTraces))
		return &TransactionAssertions{t: a.t, tx: nil}
	}
	return &TransactionAssertions{t: a.t, tx: a.block.TransactionTraces[index]}
}

// TransactionAssertions provides fluent assertions for transactions
type TransactionAssertions struct {
	t  *testing.T
	tx *pbeth.TransactionTrace
}

// Hash asserts the transaction hash
func (a *TransactionAssertions) Hash(expected string) *TransactionAssertions {
	if a.tx == nil {
		return a
	}
	expectedHash := hashFromHex(expected)
	if string(a.tx.Hash) != string(expectedHash[:]) {
		a.t.Errorf("expected tx hash %s, got %x", expected, a.tx.Hash)
	}
	return a
}

// HasCalls asserts the number of calls
func (a *TransactionAssertions) HasCalls(count int) *TransactionAssertions {
	if a.tx == nil {
		return a
	}
	if len(a.tx.Calls) != count {
		a.t.Errorf("expected %d calls, got %d", count, len(a.tx.Calls))
	}
	return a
}

// ParseFirehoseBlock parses a block from FIRE BLOCK output format
func ParseFirehoseBlock(t *testing.T, tag string, buffer *bytes.Buffer) *pbeth.Block {
	scanner := bufio.NewScanner(buffer)

	var initSeen bool
	var block *pbeth.Block

	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines
		if len(line) == 0 {
			continue
		}

		// Parse FIRE INIT
		if strings.HasPrefix(line, "FIRE INIT ") {
			parts := strings.SplitN(line, " ", 4)
			require.GreaterOrEqual(t, len(parts), 4, "For %s: FIRE INIT line should have at least 4 parts", tag)

			version := parts[2]
			require.Contains(t, []string{"3.0", "3.1"}, version, "For %s: protocol version should be 3.0 or 3.1", tag)

			initSeen = true
			continue
		}

		// Parse FIRE BLOCK
		if strings.HasPrefix(line, "FIRE BLOCK ") {
			require.True(t, initSeen, "For %s: FIRE INIT must appear before FIRE BLOCK", tag)

			// FIRE BLOCK <block_num> <block_hash> <parent_num> <parent_hash> <lib_num> <timestamp_unix_nano> <payload_base64>
			parts := strings.SplitN(line, " ", 9)
			require.GreaterOrEqual(t, len(parts), 9, "For %s: FIRE BLOCK line should have 9 parts", tag)

			// Extract base64-encoded payload (last field)
			payloadBase64 := parts[8]

			// Decode base64
			payloadBytes, err := base64.StdEncoding.DecodeString(payloadBase64)
			require.NoError(t, err, "For %s: base64 payload decode", tag)

			// Unmarshal protobuf
			block = &pbeth.Block{}
			err = proto.Unmarshal(payloadBytes, block)
			require.NoError(t, err, "For %s: protobuf unmarshal", tag)

			// Validate fields match (for integrity)
			blockNum, err := strconv.ParseUint(parts[2], 10, 64)
			require.NoError(t, err, "For %s: parse block number from FIRE BLOCK header", tag)
			require.Equal(t, blockNum, block.Number, "For %s: block number in header should match protobuf", tag)

			// We found the block, return it
			return block
		}
	}

	require.NoError(t, scanner.Err(), "For %s: reading buffer", tag)
	require.Fail(t, "For %s: no FIRE BLOCK found in buffer", tag)
	return nil
}
