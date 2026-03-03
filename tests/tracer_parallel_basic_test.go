package tests

import (
	"slices"
	"sync"
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go"
	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParallel_Basic_ThreeTransactionsSequential tests basic parallel execution with 3 transactions
// executed sequentially (not concurrently) and committed in order.
func TestParallel_Basic_ThreeTransactionsSequential(t *testing.T) {
	coordinator := NewTracerTester(t)
	coordinator.StartBlock()

	// Spawn 3 isolated tracers
	isolated0 := coordinator.Spawn(0)
	isolated1 := coordinator.Spawn(1)
	isolated2 := coordinator.Spawn(2)

	// Execute transactions sequentially (one after another)
	// Transaction 0: Alice -> Bob, 100 wei
	trx0 := new(firehose.TxEventBuilder).
		Type(firehose.TxTypeLegacy).
		From(Alice).To(Bob).Value(bigInt(100)).
		Gas(21000).GasPrice(bigInt(10)).Nonce(0).Build()

	isolated0.
		StartTrx(trx0).
		StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
		EndCall([]byte{}, 21000).
		EndTrx(successReceipt(21000), nil)

	// Transaction 1: Bob -> Charlie, 50 wei
	trx1 := new(firehose.TxEventBuilder).
		Type(firehose.TxTypeLegacy).
		From(Bob).To(Charlie).Value(bigInt(50)).
		Gas(21000).GasPrice(bigInt(10)).Nonce(0).Build()

	isolated1.
		StartTrx(trx1).
		StartCall(BobAddr, CharlieAddr, bigInt(50), 21000, []byte{}).
		EndCall([]byte{}, 21000).
		EndTrx(successReceipt(21000), nil)

	// Transaction 2: Charlie -> Alice, 25 wei
	trx2 := new(firehose.TxEventBuilder).
		Type(firehose.TxTypeLegacy).
		From(Charlie).To(Alice).Value(bigInt(25)).
		Gas(21000).GasPrice(bigInt(10)).Nonce(0).Build()

	isolated2.
		StartTrx(trx2).
		StartCall(CharlieAddr, AliceAddr, bigInt(25), 21000, []byte{}).
		EndCall([]byte{}, 21000).
		EndTrx(successReceipt(21000), nil)

	// Commit in order
	coordinator.Commit(isolated0)
	coordinator.Commit(isolated1)
	coordinator.Commit(isolated2)

	coordinator.EndBlock(nil)

	// Parse the block output (skip native validator comparison since parallel execution isn't supported by native tracer)
	block := ParseFirehoseBlock(t, "shared tracer", coordinator.tracer.GetTestingOutputBuffer())

	// Verify 3 transactions
	require.Equal(t, 3, len(block.TransactionTraces))

	// Verify ordinals are sequential
	verifySequentialOrdinals(t, block)

	// Verify transaction order matches commit order
	assert.Equal(t, AliceAddr[:], block.TransactionTraces[0].From)
	assert.Equal(t, BobAddr[:], block.TransactionTraces[1].From)
	assert.Equal(t, CharlieAddr[:], block.TransactionTraces[2].From)

	// Verify transaction values
	assert.Equal(t, bigInt(100).Bytes(), block.TransactionTraces[0].Value.Bytes)
	assert.Equal(t, bigInt(50).Bytes(), block.TransactionTraces[1].Value.Bytes)
	assert.Equal(t, bigInt(25).Bytes(), block.TransactionTraces[2].Value.Bytes)
}

// TestParallel_Basic_ThreeTransactionsConcurrent tests basic parallel execution with 3 transactions
// executed concurrently in goroutines and committed in order.
func TestParallel_Basic_ThreeTransactionsConcurrent(t *testing.T) {
	coordinator := NewTracerTester(t)
	coordinator.StartBlock()

	// Spawn 3 isolated tracers
	isolated0 := coordinator.Spawn(0)
	isolated1 := coordinator.Spawn(1)
	isolated2 := coordinator.Spawn(2)

	// Execute transactions concurrently
	// Create distinct transaction events for each transaction
	trx0 := new(firehose.TxEventBuilder).
		Type(firehose.TxTypeLegacy).
		From(Alice).To(Bob).Value(bigInt(100)).
		Gas(21000).GasPrice(bigInt(10)).Nonce(0).Build()

	trx1 := new(firehose.TxEventBuilder).
		Type(firehose.TxTypeLegacy).
		From(Bob).To(Charlie).Value(bigInt(50)).
		Gas(21000).GasPrice(bigInt(10)).Nonce(0).Build()

	trx2 := new(firehose.TxEventBuilder).
		Type(firehose.TxTypeLegacy).
		From(Charlie).To(Alice).Value(bigInt(25)).
		Gas(21000).GasPrice(bigInt(10)).Nonce(0).Build()

	var wg sync.WaitGroup

	wg.Go(func() {
		isolated0.
			StartTrx(trx0).
			StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndTrx(successReceipt(21000), nil)
	})

	wg.Go(func() {
		isolated1.
			StartTrx(trx1).
			StartCall(BobAddr, CharlieAddr, bigInt(50), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndTrx(successReceipt(21000), nil)
	})

	wg.Go(func() {
		isolated2.
			StartTrx(trx2).
			StartCall(CharlieAddr, AliceAddr, bigInt(25), 21000, []byte{}).
			EndCall([]byte{}, 21000).
			EndTrx(successReceipt(21000), nil)
	})

	wg.Wait()

	// Commit in order
	coordinator.Commit(isolated0)
	coordinator.Commit(isolated1)
	coordinator.Commit(isolated2)

	coordinator.EndBlock(nil)

	// Parse the block output (skip native validator comparison since parallel execution isn't supported by native tracer)
	block := ParseFirehoseBlock(t, "shared tracer", coordinator.tracer.GetTestingOutputBuffer())

	// Verify 3 transactions
	require.Equal(t, 3, len(block.TransactionTraces))

	// Verify ordinals are sequential
	verifySequentialOrdinals(t, block)

	// Verify transaction order matches commit order
	assert.Equal(t, AliceAddr[:], block.TransactionTraces[0].From)
	assert.Equal(t, BobAddr[:], block.TransactionTraces[1].From)
	assert.Equal(t, CharlieAddr[:], block.TransactionTraces[2].From)
}

// TestParallel_Basic_EmptyBlock verifies parallel mode works with zero transactions
func TestParallel_Basic_EmptyBlock(t *testing.T) {
	coordinator := NewTracerTester(t)
	coordinator.StartBlock()

	// No transactions spawned or executed

	coordinator.EndBlock(nil)

	// Parse the block output (skip native validator comparison since parallel execution isn't supported by native tracer)
	block := ParseFirehoseBlock(t, "shared tracer", coordinator.tracer.GetTestingOutputBuffer())

	assert.Equal(t, 0, len(block.TransactionTraces))
}

// TestParallel_Basic_SingleTransaction verifies parallel mode works with just one transaction
func TestParallel_Basic_SingleTransaction(t *testing.T) {
	coordinator := NewTracerTester(t)
	coordinator.StartBlock()

	isolated := coordinator.Spawn(0)

	isolated.
		StartTrx(TestLegacyTrx).
		StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
		EndCall([]byte{}, 21000).
		EndTrx(successReceipt(21000), nil)

	coordinator.Commit(isolated)

	coordinator.EndBlock(nil)

	// Parse the block output (skip native validator comparison since parallel execution isn't supported by native tracer)
	block := ParseFirehoseBlock(t, "shared tracer", coordinator.tracer.GetTestingOutputBuffer())

	assert.Equal(t, 1, len(block.TransactionTraces))
	verifySequentialOrdinals(t, block)
}

// TestParallel_Basic_NestedCalls verifies parallel mode works with nested calls
func TestParallel_Basic_NestedCalls(t *testing.T) {
	coordinator := NewTracerTester(t)
	coordinator.StartBlock()

	isolated := coordinator.Spawn(0)

	isolated.
		StartTrx(TestLegacyTrx).
		StartCall(AliceAddr, CharlieAddr, bigInt(0), 200000, []byte{0x01}).
		StartCall(CharlieAddr, BobAddr, bigInt(50), 150000, []byte{0x02}).
		StartCall(BobAddr, CharlieAddr, bigInt(25), 100000, []byte{0x03}).
		EndCall([]byte{0x04}, 80000).
		EndCall([]byte{0x05}, 140000).
		EndCall([]byte{0x06}, 180000).
		EndTrx(successReceipt(200000), nil)

	coordinator.Commit(isolated)

	coordinator.EndBlock(nil)

	// Parse the block output (skip native validator comparison since parallel execution isn't supported by native tracer)
	block := ParseFirehoseBlock(t, "shared tracer", coordinator.tracer.GetTestingOutputBuffer())

	require.Equal(t, 1, len(block.TransactionTraces))

	trx := block.TransactionTraces[0]
	assert.Equal(t, 3, len(trx.Calls), "should have 3 calls (root + 2 nested)")

	// Verify ordinals are sequential
	verifySequentialOrdinals(t, block)
}

// TestParallel_Basic_WithLogs verifies parallel mode works with logs
func TestParallel_Basic_WithLogs(t *testing.T) {
	coordinator := NewTracerTester(t)
	coordinator.StartBlock()

	isolated := coordinator.Spawn(0)

	topics := [][32]byte{hash32(100), hash32(200)}
	data := []byte{0x01, 0x02, 0x03}

	isolated.
		StartTrx(TestLegacyTrx).
		StartCall(AliceAddr, CharlieAddr, bigInt(0), 100000, []byte{}).
		Log(CharlieAddr, topics, data, 0).
		Log(CharlieAddr, topics, data, 1).
		EndCall([]byte{}, 90000).
		EndTrx(receiptWithLogs(100000, []firehose.LogData{
			{Address: CharlieAddr, Topics: topics, Data: data, BlockIndex: 0},
			{Address: CharlieAddr, Topics: topics, Data: data, BlockIndex: 1},
		}), nil)

	coordinator.Commit(isolated)

	coordinator.EndBlock(nil)

	// Parse the block output (skip native validator comparison since parallel execution isn't supported by native tracer)
	block := ParseFirehoseBlock(t, "shared tracer", coordinator.tracer.GetTestingOutputBuffer())

	require.Equal(t, 1, len(block.TransactionTraces))

	trx := block.TransactionTraces[0]
	assert.Equal(t, 2, len(trx.Calls[0].Logs))

	// Verify ordinals are sequential
	verifySequentialOrdinals(t, block)

	// Verify log ordinals
	log0Ordinal := trx.Calls[0].Logs[0].Ordinal
	log1Ordinal := trx.Calls[0].Logs[1].Ordinal
	assert.True(t, log1Ordinal > log0Ordinal, "log ordinals should be increasing")
}

// verifySequentialOrdinals verifies all ordinals in a block are sequential with no gaps
func verifySequentialOrdinals(t *testing.T, block *pbeth.Block) {
	t.Helper()

	ordinals := collectAllOrdinals(block)

	// Check for duplicates
	seen := make(map[uint64]bool)
	for _, ordinal := range ordinals {
		require.False(t, seen[ordinal], "duplicate ordinal %d found", ordinal)
		seen[ordinal] = true
	}

	// Sort ordinals to check for sequential values and no gaps
	sorted := make([]uint64, len(ordinals))
	copy(sorted, ordinals)
	slices.Sort(sorted)

	// Verify ordinals form a sequential sequence starting from some base
	if len(sorted) > 0 {
		base := sorted[0]
		for i, ordinal := range sorted {
			expected := base + uint64(i)
			require.Equal(t, expected, ordinal, "ordinal at sorted position %d should be %d but got %d (no gaps allowed)", i, expected, ordinal)
		}
	}
}

// collectAllOrdinals collects all ordinals from a block in the order they appear
func collectAllOrdinals(block *pbeth.Block) []uint64 {
	var ordinals []uint64

	for _, trx := range block.TransactionTraces {
		ordinals = append(ordinals, trx.BeginOrdinal)

		for _, call := range trx.Calls {
			ordinals = append(ordinals, collectCallOrdinals(call)...)
		}

		// Note: Receipt.Logs are duplicates of Call.Logs with same ordinals,
		// so we skip them here to avoid counting the same ordinals twice

		ordinals = append(ordinals, trx.EndOrdinal)
	}

	return ordinals
}

// collectCallOrdinals recursively collects all ordinals from a call
func collectCallOrdinals(call *pbeth.Call) []uint64 {
	var ordinals []uint64

	ordinals = append(ordinals, call.BeginOrdinal)

	// Collect from logs
	for _, log := range call.Logs {
		ordinals = append(ordinals, log.Ordinal)
	}

	// Collect from balance changes
	for _, change := range call.BalanceChanges {
		ordinals = append(ordinals, change.Ordinal)
	}

	// Collect from nonce changes
	for _, change := range call.NonceChanges {
		ordinals = append(ordinals, change.Ordinal)
	}

	// Collect from code changes
	for _, change := range call.CodeChanges {
		ordinals = append(ordinals, change.Ordinal)
	}

	// Collect from storage changes
	for _, change := range call.StorageChanges {
		ordinals = append(ordinals, change.Ordinal)
	}

	// Collect from gas changes
	for _, change := range call.GasChanges {
		ordinals = append(ordinals, change.Ordinal)
	}

	ordinals = append(ordinals, call.EndOrdinal)

	return ordinals
}
