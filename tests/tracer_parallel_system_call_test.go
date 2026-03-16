package tests

import (
	"slices"
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go/v4"
	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParallel_SystemCall_PreTransactions tests parallel execution with pre-transaction system calls
// Scenario: System call happens before transactions, ordinals must be rebased correctly
func TestParallel_SystemCall_PreTransactions(t *testing.T) {
	coordinator := NewTracerTester(t)
	coordinator.StartBlock()

	// Pre-transaction system call (EIP-4788 beacon root)
	beaconRoot := hash32(12345)
	coordinator.SystemCall(
		SystemAddress,
		BeaconRootsAddress,
		beaconRoot[:],
		30_000_000,
		[]byte{},
		50_000,
	)

	// Spawn 2 isolated tracers for parallel execution
	isolated0 := coordinator.Spawn(0)
	isolated1 := coordinator.Spawn(1)

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

	// Commit in order
	coordinator.Commit(isolated0)
	coordinator.Commit(isolated1)

	coordinator.EndBlock(nil)

	// Parse the block output (skip native validator comparison since parallel execution isn't supported by native tracer)
	block := ParseFirehoseBlock(t, "shared tracer", coordinator.tracer.GetTestingOutputBuffer())

	// Verify structure
	require.Equal(t, 1, len(block.SystemCalls), "Should have 1 system call")
	require.Equal(t, 2, len(block.TransactionTraces), "Should have 2 transactions")

	// Verify ordinals are sequential across system call and transactions
	verifySequentialOrdinalsWithSystemCalls(t, block)

	// Verify system call ordinals come before transaction ordinals
	sysCall := block.SystemCalls[0]
	trx0Result := block.TransactionTraces[0]
	trx1Result := block.TransactionTraces[1]

	assert.True(t, sysCall.EndOrdinal < trx0Result.BeginOrdinal,
		"System call end ordinal (%d) should be < transaction 0 begin ordinal (%d)",
		sysCall.EndOrdinal, trx0Result.BeginOrdinal)

	assert.True(t, trx0Result.EndOrdinal < trx1Result.BeginOrdinal,
		"Transaction 0 end ordinal (%d) should be < transaction 1 begin ordinal (%d)",
		trx0Result.EndOrdinal, trx1Result.BeginOrdinal)

	t.Logf("Ordinal sequence: SysCall[%d-%d] -> Trx0[%d-%d] -> Trx1[%d-%d]",
		sysCall.BeginOrdinal, sysCall.EndOrdinal,
		trx0Result.BeginOrdinal, trx0Result.EndOrdinal,
		trx1Result.BeginOrdinal, trx1Result.EndOrdinal)
}

// TestParallel_SystemCall_PostTransactions tests parallel execution with post-transaction system calls
// Scenario: Transactions execute in parallel, then system call happens after all commits
func TestParallel_SystemCall_PostTransactions(t *testing.T) {
	coordinator := NewTracerTester(t)
	coordinator.StartBlock()

	// Spawn 2 isolated tracers for parallel execution
	isolated0 := coordinator.Spawn(0)
	isolated1 := coordinator.Spawn(1)

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

	// Commit all transactions before system call
	coordinator.Commit(isolated0)
	coordinator.Commit(isolated1)

	// Post-transaction system call (EIP-2935 parent hash storage)
	parentHash := hash32(99999)
	coordinator.SystemCall(
		SystemAddress,
		HistoryStorageAddress,
		parentHash[:],
		30_000_000,
		[]byte{},
		45_000,
	)

	coordinator.EndBlock(nil)

	// Parse the block output (skip native validator comparison since parallel execution isn't supported by native tracer)
	block := ParseFirehoseBlock(t, "shared tracer", coordinator.tracer.GetTestingOutputBuffer())

	// Verify structure
	require.Equal(t, 2, len(block.TransactionTraces), "Should have 2 transactions")
	require.Equal(t, 1, len(block.SystemCalls), "Should have 1 system call")

	// Verify ordinals are sequential across transactions and system call
	verifySequentialOrdinalsWithSystemCalls(t, block)

	// Verify transaction ordinals come before system call ordinals
	trx0Result := block.TransactionTraces[0]
	trx1Result := block.TransactionTraces[1]
	sysCall := block.SystemCalls[0]

	assert.True(t, trx0Result.EndOrdinal < trx1Result.BeginOrdinal,
		"Transaction 0 end ordinal (%d) should be < transaction 1 begin ordinal (%d)",
		trx0Result.EndOrdinal, trx1Result.BeginOrdinal)

	assert.True(t, trx1Result.EndOrdinal < sysCall.BeginOrdinal,
		"Transaction 1 end ordinal (%d) should be < system call begin ordinal (%d)",
		trx1Result.EndOrdinal, sysCall.BeginOrdinal)

	t.Logf("Ordinal sequence: Trx0[%d-%d] -> Trx1[%d-%d] -> SysCall[%d-%d]",
		trx0Result.BeginOrdinal, trx0Result.EndOrdinal,
		trx1Result.BeginOrdinal, trx1Result.EndOrdinal,
		sysCall.BeginOrdinal, sysCall.EndOrdinal)
}

// TestParallel_SystemCall_PreAndPost tests parallel execution with both pre and post system calls
// Scenario: Pre-tx system call -> Parallel transactions -> Post-tx system call
func TestParallel_SystemCall_PreAndPost(t *testing.T) {
	coordinator := NewTracerTester(t)
	coordinator.StartBlock()

	// Pre-transaction system call (EIP-4788 beacon root)
	beaconRoot := hash32(12345)
	coordinator.SystemCall(
		SystemAddress,
		BeaconRootsAddress,
		beaconRoot[:],
		30_000_000,
		[]byte{},
		50_000,
	)

	// Spawn 3 isolated tracers for parallel execution
	isolated0 := coordinator.Spawn(0)
	isolated1 := coordinator.Spawn(1)
	isolated2 := coordinator.Spawn(2)

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

	// Commit all transactions
	coordinator.Commit(isolated0)
	coordinator.Commit(isolated1)
	coordinator.Commit(isolated2)

	// Post-transaction system call (EIP-2935 parent hash storage)
	parentHash := hash32(99999)
	coordinator.SystemCall(
		SystemAddress,
		HistoryStorageAddress,
		parentHash[:],
		30_000_000,
		[]byte{},
		45_000,
	)

	coordinator.EndBlock(nil)

	// Parse the block output (skip native validator comparison since parallel execution isn't supported by native tracer)
	block := ParseFirehoseBlock(t, "shared tracer", coordinator.tracer.GetTestingOutputBuffer())

	// Verify structure
	require.Equal(t, 2, len(block.SystemCalls), "Should have 2 system calls")
	require.Equal(t, 3, len(block.TransactionTraces), "Should have 3 transactions")

	// Verify ordinals are sequential across all elements
	verifySequentialOrdinalsWithSystemCalls(t, block)

	// Verify ordinal ordering: pre-sys -> trx0 -> trx1 -> trx2 -> post-sys
	preSysCall := block.SystemCalls[0]
	trx0Result := block.TransactionTraces[0]
	trx1Result := block.TransactionTraces[1]
	trx2Result := block.TransactionTraces[2]
	postSysCall := block.SystemCalls[1]

	assert.True(t, preSysCall.EndOrdinal < trx0Result.BeginOrdinal,
		"Pre-system call end ordinal (%d) should be < transaction 0 begin ordinal (%d)",
		preSysCall.EndOrdinal, trx0Result.BeginOrdinal)

	assert.True(t, trx0Result.EndOrdinal < trx1Result.BeginOrdinal,
		"Transaction 0 end ordinal (%d) should be < transaction 1 begin ordinal (%d)",
		trx0Result.EndOrdinal, trx1Result.BeginOrdinal)

	assert.True(t, trx1Result.EndOrdinal < trx2Result.BeginOrdinal,
		"Transaction 1 end ordinal (%d) should be < transaction 2 begin ordinal (%d)",
		trx1Result.EndOrdinal, trx2Result.BeginOrdinal)

	assert.True(t, trx2Result.EndOrdinal < postSysCall.BeginOrdinal,
		"Transaction 2 end ordinal (%d) should be < post-system call begin ordinal (%d)",
		trx2Result.EndOrdinal, postSysCall.BeginOrdinal)

	t.Logf("Ordinal sequence: PreSys[%d-%d] -> Trx0[%d-%d] -> Trx1[%d-%d] -> Trx2[%d-%d] -> PostSys[%d-%d]",
		preSysCall.BeginOrdinal, preSysCall.EndOrdinal,
		trx0Result.BeginOrdinal, trx0Result.EndOrdinal,
		trx1Result.BeginOrdinal, trx1Result.EndOrdinal,
		trx2Result.BeginOrdinal, trx2Result.EndOrdinal,
		postSysCall.BeginOrdinal, postSysCall.EndOrdinal)
}

// TestParallel_SystemCall_MultiplePreTransactions tests multiple pre-transaction system calls
// Scenario: Multiple system calls before parallel transactions
func TestParallel_SystemCall_MultiplePreTransactions(t *testing.T) {
	coordinator := NewTracerTester(t)
	coordinator.StartBlock()

	// Pre-transaction system call 1 (EIP-4788 beacon root)
	beaconRoot := hash32(12345)
	coordinator.SystemCall(
		SystemAddress,
		BeaconRootsAddress,
		beaconRoot[:],
		30_000_000,
		[]byte{},
		50_000,
	)

	// Pre-transaction system call 2 (EIP-2935 parent hash)
	parentHash := hash32(99999)
	coordinator.SystemCall(
		SystemAddress,
		HistoryStorageAddress,
		parentHash[:],
		30_000_000,
		[]byte{},
		45_000,
	)

	// Spawn 2 isolated tracers
	isolated0 := coordinator.Spawn(0)
	isolated1 := coordinator.Spawn(1)

	// Transaction 0: Alice -> Bob
	trx0 := new(firehose.TxEventBuilder).
		Type(firehose.TxTypeLegacy).
		From(Alice).To(Bob).Value(bigInt(100)).
		Gas(21000).GasPrice(bigInt(10)).Nonce(0).Build()

	isolated0.
		StartTrx(trx0).
		StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
		EndCall([]byte{}, 21000).
		EndTrx(successReceipt(21000), nil)

	// Transaction 1: Bob -> Charlie
	trx1 := new(firehose.TxEventBuilder).
		Type(firehose.TxTypeLegacy).
		From(Bob).To(Charlie).Value(bigInt(50)).
		Gas(21000).GasPrice(bigInt(10)).Nonce(0).Build()

	isolated1.
		StartTrx(trx1).
		StartCall(BobAddr, CharlieAddr, bigInt(50), 21000, []byte{}).
		EndCall([]byte{}, 21000).
		EndTrx(successReceipt(21000), nil)

	// Commit in order
	coordinator.Commit(isolated0)
	coordinator.Commit(isolated1)

	coordinator.EndBlock(nil)

	// Parse the block output
	block := ParseFirehoseBlock(t, "shared tracer", coordinator.tracer.GetTestingOutputBuffer())

	// Verify structure
	require.Equal(t, 2, len(block.SystemCalls), "Should have 2 system calls")
	require.Equal(t, 2, len(block.TransactionTraces), "Should have 2 transactions")

	// Verify ordinals are sequential
	verifySequentialOrdinalsWithSystemCalls(t, block)

	// Verify ordering: sys1 -> sys2 -> trx0 -> trx1
	sys1 := block.SystemCalls[0]
	sys2 := block.SystemCalls[1]
	trx0Result := block.TransactionTraces[0]
	trx1Result := block.TransactionTraces[1]

	assert.True(t, sys1.EndOrdinal < sys2.BeginOrdinal,
		"System call 1 end (%d) should be < system call 2 begin (%d)",
		sys1.EndOrdinal, sys2.BeginOrdinal)

	assert.True(t, sys2.EndOrdinal < trx0Result.BeginOrdinal,
		"System call 2 end (%d) should be < transaction 0 begin (%d)",
		sys2.EndOrdinal, trx0Result.BeginOrdinal)

	assert.True(t, trx0Result.EndOrdinal < trx1Result.BeginOrdinal,
		"Transaction 0 end (%d) should be < transaction 1 begin (%d)",
		trx0Result.EndOrdinal, trx1Result.BeginOrdinal)

	t.Logf("Ordinal sequence: Sys1[%d-%d] -> Sys2[%d-%d] -> Trx0[%d-%d] -> Trx1[%d-%d]",
		sys1.BeginOrdinal, sys1.EndOrdinal,
		sys2.BeginOrdinal, sys2.EndOrdinal,
		trx0Result.BeginOrdinal, trx0Result.EndOrdinal,
		trx1Result.BeginOrdinal, trx1Result.EndOrdinal)
}

// verifySequentialOrdinalsWithSystemCalls verifies all ordinals in a block are sequential with no gaps
// This version also checks system calls, unlike the version in tracer_parallel_basic_test.go
func verifySequentialOrdinalsWithSystemCalls(t *testing.T, block *pbeth.Block) {
	t.Helper()

	ordinals := collectAllOrdinalsIncludingSystemCalls(block)

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

// collectAllOrdinalsIncludingSystemCalls collects all ordinals from a block including system calls
func collectAllOrdinalsIncludingSystemCalls(block *pbeth.Block) []uint64 {
	var ordinals []uint64

	// Collect from system calls
	for _, sysCall := range block.SystemCalls {
		ordinals = append(ordinals, collectCallOrdinals(sysCall)...)
	}

	// Collect from transactions
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
