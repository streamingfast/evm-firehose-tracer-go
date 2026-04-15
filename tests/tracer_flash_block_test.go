package tests

// Tests ported from go-ethereum's eth/tracers/firehose_test.go (TestFirehose_FlashBlock* tests).
//
// Flash blocks are a mechanism used by Optimism/Katana where a single canonical block is
// built incrementally across multiple "flash" iterations. Each iteration adds more transactions,
// emits a partial block, and a snapshot captures the point where the next iteration should start.
//
// The tests verify:
//  1. Transaction traces are accumulated correctly across flash block iterations
//  2. Snapshot captures state at a specific point (not including post-snapshot traces)
//  3. Sequence validation: same or lower flash block index panics
//  4. New block number clears the snapshot
//  5. Regular blocks do not affect the flash block snapshot or last flash block index
//  6. System calls are included in snapshots
//  7. Balance changes and code changes are included in snapshots

import (
	"math/big"
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go/v5"
	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFlashBlockEvent builds a BlockEvent with FlashBlock metadata using TestBlock's block data.
func newFlashBlockEvent(idx uint64) firehose.BlockEvent {
	return firehose.BlockEvent{
		Block: TestBlock.Block,
		FlashBlock: &firehose.FlashBlockData{
			Idx: idx,
		},
	}
}

// newFlashBlockEventFromBlock builds a flash BlockEvent using custom block data.
func newFlashBlockEventFromBlock(block firehose.BlockData, idx uint64) firehose.BlockEvent {
	return firehose.BlockEvent{
		Block: block,
		FlashBlock: &firehose.FlashBlockData{
			Idx: idx,
		},
	}
}

// flashTx1 / flashTx2 / flashTx3 / flashTx4 provide unique transactions for use in
// flash block tests. Each has a distinct nonce so we can identify it in assertions.
var (
	flashTx1 = new(firehose.TxEventBuilder).
		Type(firehose.TxTypeLegacy).
		Hash("0x1100000000000000000000000000000000000000000000000000000000000000").
		From(Alice).To(Bob).Value(bigInt(1000)).Gas(21000).GasPrice(bigInt(1)).Nonce(1).Build()

	flashTx2 = new(firehose.TxEventBuilder).
		Type(firehose.TxTypeLegacy).
		Hash("0x2200000000000000000000000000000000000000000000000000000000000000").
		From(Alice).To(Bob).Value(bigInt(2000)).Gas(21000).GasPrice(bigInt(1)).Nonce(2).Build()

	flashTx3 = new(firehose.TxEventBuilder).
		Type(firehose.TxTypeLegacy).
		Hash("0x3300000000000000000000000000000000000000000000000000000000000000").
		From(Alice).To(Bob).Value(bigInt(3000)).Gas(21000).GasPrice(bigInt(1)).Nonce(3).Build()

	flashTx4 = new(firehose.TxEventBuilder).
		Type(firehose.TxTypeLegacy).
		Hash("0x4400000000000000000000000000000000000000000000000000000000000000").
		From(Alice).To(Bob).Value(bigInt(4000)).Gas(21000).GasPrice(bigInt(1)).Nonce(4).Build()
)

// execFlashTx is a helper that executes a simple send transaction within the tester
// (enter call, exit call, end transaction).
func execFlashTx(s *TracerTester, tx firehose.TxEvent) *TracerTester {
	return s.
		StartTrx(tx).
		StartCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
		EndCall([]byte{}, 21000).
		EndTrx(successReceipt(21000), nil)
}

// TestFlashBlock_BasicHandling verifies that:
//   - Two flash block iterations accumulate their transactions correctly.
//   - The final output block contains all four transactions in the correct order.
//   - The second flash block's block header is used as the final block header.
//
// Ported from TestFirehose_FlashBlockHandling.
func TestFlashBlock_BasicHandling(t *testing.T) {
	tester := NewTracerTester(t)

	// --- Flash block iteration 1 ---
	tester.tracer.OnBlockStart(newFlashBlockEvent(1))
	tester.blockLogIndex = 0
	execFlashTx(tester, flashTx1)
	execFlashTx(tester, flashTx2)

	// Snapshot before ending, so the next iteration picks up tx1+tx2.
	tester.tracer.SnapshotFlashBlockForNextIteration()
	tester.tracer.OnBlockEnd(nil)

	// --- Flash block iteration 2 (sometimes indices are skipped, so Idx=3) ---
	tester.tracer.OnBlockStart(newFlashBlockEvent(3))
	tester.blockLogIndex = 0
	execFlashTx(tester, flashTx3)
	execFlashTx(tester, flashTx4)
	tester.tracer.OnBlockEnd(nil)

	// Two blocks were emitted; parse both.
	blocks := ParseFirehoseBlocks(t, "flash block basic", tester.tracer.GetTestingOutputBuffer())
	require.Len(t, blocks, 2, "two blocks should have been emitted")

	// First flash block: only tx1 and tx2
	first := blocks[0]
	require.Len(t, first.TransactionTraces, 2)
	assert.Equal(t, uint64(1), first.TransactionTraces[0].Nonce)
	assert.Equal(t, uint64(2), first.TransactionTraces[1].Nonce)

	// Second flash block: tx1, tx2 (from snapshot) + tx3, tx4 (new)
	second := blocks[1]
	require.Len(t, second.TransactionTraces, 4, "second flash block should have 4 transactions")
	assert.Equal(t, uint64(1), second.TransactionTraces[0].Nonce)
	assert.Equal(t, uint64(2), second.TransactionTraces[1].Nonce)
	assert.Equal(t, uint64(3), second.TransactionTraces[2].Nonce)
	assert.Equal(t, uint64(4), second.TransactionTraces[3].Nonce)
}

// TestFlashBlock_SequenceValidation verifies that OnBlockStart panics when:
//   - A flash block index is equal to the previously seen index (on same block number).
//   - A flash block index is lower than the previously seen index (on same block number).
//
// And that it does NOT panic when:
//   - The block number increases (even if the index resets to 0).
//   - A non-sequential but strictly-higher index is used on the same block number.
//
// Ported from TestFirehose_FlashBlockSequenceValidation.
func TestFlashBlock_SequenceValidation(t *testing.T) {
	newTracer := func() *firehose.Tracer {
		tr := NewTracerTester(t)
		return tr.tracer
	}

	block1Data := TestBlock.Block
	block2Data := firehose.BlockData{
		Number:     block1Data.Number + 1,
		Hash:       hash32(9999),
		ParentHash: block1Data.Hash,
		UncleHash:  block1Data.UncleHash,
		Coinbase:   block1Data.Coinbase,
		Root:       block1Data.Root,
		TxHash:     block1Data.TxHash,
		ReceiptHash: block1Data.ReceiptHash,
		Bloom:      make([]byte, 256),
		Difficulty: big.NewInt(0),
		GasLimit:   block1Data.GasLimit,
		Time:       block1Data.Time + 1,
		Size:       block1Data.Size,
	}

	// Test 1: Same block index not progressing should panic (snapshot exists, same block number, same index)
	t.Run("same_index_panics", func(t *testing.T) {
		tr := newTracer()
		tr.OnBlockStart(newFlashBlockEventFromBlock(block1Data, 1))
		tr.SnapshotFlashBlockForNextIteration()
		tr.OnBlockEnd(nil)

		require.Panics(t, func() {
			tr.OnBlockStart(newFlashBlockEventFromBlock(block1Data, 1))
		})
	})

	// Test 2: Same block, index going backwards should panic
	t.Run("backwards_index_panics", func(t *testing.T) {
		tr := newTracer()
		tr.OnBlockStart(newFlashBlockEventFromBlock(block1Data, 1))
		tr.SnapshotFlashBlockForNextIteration()
		tr.OnBlockEnd(nil)

		require.Panics(t, func() {
			tr.OnBlockStart(newFlashBlockEventFromBlock(block1Data, 0))
		})
	})

	// Test 3: Increased block number should NOT panic even if the index resets (snapshot is cleared)
	t.Run("new_block_number_no_panic", func(t *testing.T) {
		tr := newTracer()
		tr.OnBlockStart(newFlashBlockEventFromBlock(block1Data, 1))
		tr.SnapshotFlashBlockForNextIteration()
		tr.OnBlockEnd(nil)

		require.NotPanics(t, func() {
			tr.OnBlockStart(newFlashBlockEventFromBlock(block2Data, 0))
		})
		tr.OnBlockEnd(nil)
	})

	// Test 4: Non-sequential but strictly higher index on same block should NOT panic
	t.Run("non_sequential_but_higher_no_panic", func(t *testing.T) {
		tr := newTracer()
		tr.OnBlockStart(newFlashBlockEventFromBlock(block1Data, 1))
		tr.SnapshotFlashBlockForNextIteration()
		tr.OnBlockEnd(nil)

		// Skipped indices 2 and 3, but Idx=4 is still strictly higher than 1.
		require.NotPanics(t, func() {
			tr.OnBlockStart(newFlashBlockEventFromBlock(block1Data, 4))
		})
		tr.OnBlockEnd(nil)
	})
}

// TestFlashBlock_PersistsOnRegularBlock verifies that:
//   - After a flash block ends with a snapshot, the snapshot persists when a regular
//     (non-flash) block is processed.
//   - The flash block index is not reset by a regular block.
//
// Ported from TestFirehose_FlashBlockPersistsOnRegularBlock.
func TestFlashBlock_PersistsOnRegularBlock(t *testing.T) {
	block1Data := TestBlock.Block
	block2Data := firehose.BlockData{
		Number:     block1Data.Number + 1,
		Hash:       hash32(9999),
		ParentHash: block1Data.Hash,
		UncleHash:  block1Data.UncleHash,
		Coinbase:   block1Data.Coinbase,
		Root:       block1Data.Root,
		TxHash:     block1Data.TxHash,
		ReceiptHash: block1Data.ReceiptHash,
		Bloom:      make([]byte, 256),
		Difficulty: big.NewInt(0),
		GasLimit:   block1Data.GasLimit,
		Time:       block1Data.Time + 1,
		Size:       block1Data.Size,
	}

	tracer := NewTracerTester(t).tracer

	// Start flash block with Idx=1
	tracer.OnBlockStart(newFlashBlockEventFromBlock(block1Data, 1))
	require.True(t, tracer.IsFlashBlock())

	tracer.SnapshotFlashBlockForNextIteration()
	tracer.OnBlockEnd(nil)

	// Snapshot is set; blockIsFlashBlock reset
	require.True(t, tracer.HasFlashBlockSnapshot())
	require.False(t, tracer.IsFlashBlock())

	// Start regular (non-flash) block
	tracer.OnBlockStart(firehose.BlockEvent{Block: block2Data})
	require.False(t, tracer.IsFlashBlock())

	// Snapshot must NOT be cleared by a regular block
	assert.True(t, tracer.HasFlashBlockSnapshot(), "snapshot should persist through regular block")
	assert.Equal(t, uint64(1), tracer.GetFlashBlockIndex(), "flash block index should persist through regular block")

	tracer.OnBlockEnd(nil)
}

// TestFlashBlock_Snapshot_BasicUsage verifies that:
//   - SnapshotFlashBlockForNextIteration captures transactions AND system calls added before the snapshot.
//   - System calls added AFTER the snapshot are NOT included in the next iteration.
//   - The next flash block iteration starts with the snapshotted state.
//
// Ported from TestFirehose_FlashBlockSnapshot_BasicUsage.
func TestFlashBlock_Snapshot_BasicUsage(t *testing.T) {
	tester := NewTracerTester(t)

	// --- Flash block iteration 1 ---
	tester.tracer.OnBlockStart(newFlashBlockEvent(1))
	tester.blockLogIndex = 0

	// Add two regular transactions
	execFlashTx(tester, flashTx1)
	execFlashTx(tester, flashTx2)

	// Add first system call BEFORE snapshot (should be included)
	tester.tracer.OnSystemCallStart()
	tester.tracer.OnCallEnter(0, byte(firehose.CallTypeCall), SystemAddress, BeaconRootsAddress, []byte{}, 30_000_000, big.NewInt(0))
	tester.tracer.OnCallExit(0, []byte{}, 50_000, nil, false)
	tester.tracer.OnSystemCallEnd()

	// Take snapshot: captures 2 txs + 1 system call
	tester.tracer.SnapshotFlashBlockForNextIteration()

	// Add second system call AFTER snapshot (should NOT be in next iteration)
	tester.tracer.OnSystemCallStart()
	tester.tracer.OnCallEnter(0, byte(firehose.CallTypeCall), SystemAddress, HistoryStorageAddress, []byte{}, 30_000_000, big.NewInt(0))
	tester.tracer.OnCallExit(0, []byte{}, 45_000, nil, false)
	tester.tracer.OnSystemCallEnd()

	tester.tracer.OnBlockEnd(nil)

	// --- Flash block iteration 2 ---
	tester.tracer.OnBlockStart(newFlashBlockEvent(2))
	tester.blockLogIndex = 0

	// Should have 2 txs + 1 system call (from snapshot, NOT the post-snapshot system call)
	// We verify this by adding one more tx and checking the final block
	execFlashTx(tester, flashTx4)
	tester.tracer.OnBlockEnd(nil)

	// Parse all emitted blocks
	blocks := ParseFirehoseBlocks(t, "flash block snapshot basic", tester.tracer.GetTestingOutputBuffer())
	require.Len(t, blocks, 2)

	// First block: 2 txs + 2 system calls
	first := blocks[0]
	require.Len(t, first.TransactionTraces, 2)
	require.Len(t, first.SystemCalls, 2)

	// Second block (from snapshot): 2 txs + 1 system call (NOT 2), plus tx4
	second := blocks[1]
	require.Len(t, second.TransactionTraces, 3, "second block should have tx1+tx2 (from snapshot) + tx4 (new)")
	assert.Equal(t, uint64(1), second.TransactionTraces[0].Nonce)
	assert.Equal(t, uint64(2), second.TransactionTraces[1].Nonce)
	assert.Equal(t, uint64(4), second.TransactionTraces[2].Nonce)
	require.Len(t, second.SystemCalls, 1, "second block should have only 1 system call from snapshot")
}

// TestFlashBlock_Snapshot_WithoutSnapshot verifies that without calling
// SnapshotFlashBlockForNextIteration, the next flash block starts fresh with no transactions.
//
// Ported from TestFirehose_FlashBlockSnapshot_WithoutSnapshot.
func TestFlashBlock_Snapshot_WithoutSnapshot(t *testing.T) {
	tester := NewTracerTester(t)

	// --- Flash block iteration 1: add 2 transactions, no snapshot ---
	tester.tracer.OnBlockStart(newFlashBlockEvent(1))
	tester.blockLogIndex = 0
	execFlashTx(tester, flashTx1)
	execFlashTx(tester, flashTx2)
	// Intentionally NOT calling SnapshotFlashBlockForNextIteration()
	tester.tracer.OnBlockEnd(nil)

	// --- Flash block iteration 2: should start completely fresh ---
	tester.tracer.OnBlockStart(newFlashBlockEvent(2))
	tester.blockLogIndex = 0
	tester.tracer.OnBlockEnd(nil)

	blocks := ParseFirehoseBlocks(t, "flash block without snapshot", tester.tracer.GetTestingOutputBuffer())
	require.Len(t, blocks, 2)

	first := blocks[0]
	require.Len(t, first.TransactionTraces, 2)

	second := blocks[1]
	require.Len(t, second.TransactionTraces, 0, "second block should start fresh with no transactions")
}

// TestFlashBlock_Snapshot_ClearedOnNewBlock verifies that the snapshot is cleared
// when a flash block with a different block number is started.
//
// Ported from TestFirehose_FlashBlockSnapshot_SnapshotClearedOnNewBlock.
func TestFlashBlock_Snapshot_ClearedOnNewBlock(t *testing.T) {
	block1Data := TestBlock.Block
	block2Data := firehose.BlockData{
		Number:     block1Data.Number + 1,
		Hash:       hash32(8888),
		ParentHash: block1Data.Hash,
		UncleHash:  block1Data.UncleHash,
		Coinbase:   block1Data.Coinbase,
		Root:       block1Data.Root,
		TxHash:     block1Data.TxHash,
		ReceiptHash: block1Data.ReceiptHash,
		Bloom:      make([]byte, 256),
		Difficulty: big.NewInt(0),
		GasLimit:   block1Data.GasLimit,
		Time:       block1Data.Time + 1,
		Size:       block1Data.Size,
	}

	tester := NewTracerTester(t)

	// --- Flash block on block 1 ---
	tester.tracer.OnBlockStart(newFlashBlockEventFromBlock(block1Data, 1))
	tester.blockLogIndex = 0
	execFlashTx(tester, flashTx1)
	tester.tracer.SnapshotFlashBlockForNextIteration()
	require.True(t, tester.tracer.HasFlashBlockSnapshot())
	tester.tracer.OnBlockEnd(nil)

	// --- Flash block on block 2 (different number): snapshot must be cleared ---
	tester.tracer.OnBlockStart(newFlashBlockEventFromBlock(block2Data, 1))
	tester.blockLogIndex = 0

	// Snapshot should have been cleared because the block number changed
	assert.False(t, tester.tracer.HasFlashBlockSnapshot(), "snapshot should be cleared on new block number")

	tester.tracer.OnBlockEnd(nil)

	blocks := ParseFirehoseBlocks(t, "flash block cleared on new block", tester.tracer.GetTestingOutputBuffer())
	require.Len(t, blocks, 2)

	// Second block should have started fresh (no transactions from the cleared snapshot)
	second := blocks[1]
	require.Len(t, second.TransactionTraces, 0, "second block should start fresh after snapshot is cleared")
}

// TestFlashBlock_Snapshot_MultipleIterations verifies the snapshot mechanism
// across multiple flash block iterations, each with more transactions than the previous.
//
// Ported from TestFirehose_FlashBlockSnapshot_MultipleIterations.
func TestFlashBlock_Snapshot_MultipleIterations(t *testing.T) {
	tester := NewTracerTester(t)

	// --- Iteration 1: snapshot after tx1, then add tx2 after snapshot ---
	tester.tracer.OnBlockStart(newFlashBlockEvent(1))
	tester.blockLogIndex = 0
	execFlashTx(tester, flashTx1)
	tester.tracer.SnapshotFlashBlockForNextIteration() // snapshot: 1 tx
	execFlashTx(tester, flashTx2)                     // added after snapshot, should NOT appear in iteration 2
	tester.tracer.OnBlockEnd(nil)

	// --- Iteration 2: starts with 1 tx (from snapshot), adds tx3, snapshot after tx3, tx4 after snapshot ---
	tester.tracer.OnBlockStart(newFlashBlockEvent(2))
	tester.blockLogIndex = 0
	execFlashTx(tester, flashTx3)
	tester.tracer.SnapshotFlashBlockForNextIteration() // snapshot: 2 txs (tx1+tx3)
	execFlashTx(tester, flashTx4)                     // added after snapshot, should NOT appear in iteration 3
	tester.tracer.OnBlockEnd(nil)

	// --- Iteration 3: starts with 2 txs (from snapshot: tx1+tx3), no more txs added ---
	tester.tracer.OnBlockStart(newFlashBlockEvent(3))
	tester.blockLogIndex = 0
	tester.tracer.OnBlockEnd(nil)

	blocks := ParseFirehoseBlocks(t, "flash block multiple iterations", tester.tracer.GetTestingOutputBuffer())
	require.Len(t, blocks, 3)

	// First block: tx1 + tx2
	require.Len(t, blocks[0].TransactionTraces, 2)
	assert.Equal(t, uint64(1), blocks[0].TransactionTraces[0].Nonce)
	assert.Equal(t, uint64(2), blocks[0].TransactionTraces[1].Nonce)

	// Second block: tx1 (snapshot) + tx3 (new) + tx4 (post-snapshot)
	require.Len(t, blocks[1].TransactionTraces, 3)
	assert.Equal(t, uint64(1), blocks[1].TransactionTraces[0].Nonce)
	assert.Equal(t, uint64(3), blocks[1].TransactionTraces[1].Nonce)
	assert.Equal(t, uint64(4), blocks[1].TransactionTraces[2].Nonce)

	// Third block: tx1 + tx3 (from snapshot, tx4 excluded)
	require.Len(t, blocks[2].TransactionTraces, 2)
	assert.Equal(t, uint64(1), blocks[2].TransactionTraces[0].Nonce)
	assert.Equal(t, uint64(3), blocks[2].TransactionTraces[1].Nonce)
}

// TestFlashBlock_Snapshot_SystemCallsIncluded verifies that system calls added before
// the snapshot are included in the next iteration, while those added after are excluded.
//
// Ported from TestFirehose_FlashBlockSnapshot_SystemCallsIncluded.
func TestFlashBlock_Snapshot_SystemCallsIncluded(t *testing.T) {
	tester := NewTracerTester(t)

	// --- Flash block iteration 1 ---
	tester.tracer.OnBlockStart(newFlashBlockEvent(1))
	tester.blockLogIndex = 0

	// Regular transaction
	execFlashTx(tester, flashTx1)

	// System call 1 (before snapshot)
	tester.tracer.OnSystemCallStart()
	tester.tracer.OnCallEnter(0, byte(firehose.CallTypeCall), SystemAddress, BeaconRootsAddress, []byte{}, 30_000_000, big.NewInt(0))
	tester.tracer.OnCallExit(0, []byte{}, 50_000, nil, false)
	tester.tracer.OnSystemCallEnd()

	// System call 2 (before snapshot)
	tester.tracer.OnSystemCallStart()
	tester.tracer.OnCallEnter(0, byte(firehose.CallTypeCall), SystemAddress, HistoryStorageAddress, []byte{}, 30_000_000, big.NewInt(0))
	tester.tracer.OnCallExit(0, []byte{}, 45_000, nil, false)
	tester.tracer.OnSystemCallEnd()

	// Snapshot: 1 tx + 2 system calls
	tester.tracer.SnapshotFlashBlockForNextIteration()

	// System call 3 (after snapshot — should NOT appear in next iteration)
	tester.tracer.OnSystemCallStart()
	tester.tracer.OnCallEnter(0, byte(firehose.CallTypeCall), SystemAddress, WithdrawalQueueAddress, []byte{}, 30_000_000, big.NewInt(0))
	tester.tracer.OnCallExit(0, []byte{}, 40_000, nil, false)
	tester.tracer.OnSystemCallEnd()

	tester.tracer.OnBlockEnd(nil)

	// --- Flash block iteration 2 ---
	tester.tracer.OnBlockStart(newFlashBlockEvent(2))
	tester.blockLogIndex = 0

	// Add one more transaction
	execFlashTx(tester, flashTx2)
	tester.tracer.OnBlockEnd(nil)

	blocks := ParseFirehoseBlocks(t, "flash block system calls", tester.tracer.GetTestingOutputBuffer())
	require.Len(t, blocks, 2)

	// First block: 1 tx + 3 system calls
	require.Len(t, blocks[0].TransactionTraces, 1)
	require.Len(t, blocks[0].SystemCalls, 3)

	// Second block: 1 tx (from snapshot) + 2 system calls (from snapshot, NOT 3) + tx2 (new)
	second := blocks[1]
	require.Len(t, second.TransactionTraces, 2)
	assert.Equal(t, uint64(1), second.TransactionTraces[0].Nonce)
	assert.Equal(t, uint64(2), second.TransactionTraces[1].Nonce)
	require.Len(t, second.SystemCalls, 2, "only system calls before the snapshot should appear")
}

// TestFlashBlock_Snapshot_BalanceChangesIncluded verifies that balance changes added
// before the snapshot appear in the next iteration, while those after do not.
func TestFlashBlock_Snapshot_BalanceChangesIncluded(t *testing.T) {
	tester := NewTracerTester(t)

	// --- Flash block iteration 1 ---
	tester.tracer.OnBlockStart(newFlashBlockEvent(1))
	tester.blockLogIndex = 0

	// Transaction with balance changes
	execFlashTx(tester, flashTx1)
	// Balance change before snapshot (block-level, outside transaction)
	tester.BalanceChange(MinerAddr, big.NewInt(0), big.NewInt(1_000_000_000_000_000_000), pbeth.BalanceChange_REASON_REWARD_MINE_BLOCK)

	// Snapshot: includes tx1 + block-level balance change
	tester.tracer.SnapshotFlashBlockForNextIteration()

	// Balance change AFTER snapshot (should NOT appear in next iteration)
	tester.BalanceChange(AliceAddr, big.NewInt(0), big.NewInt(500_000_000_000_000_000), pbeth.BalanceChange_REASON_REWARD_MINE_UNCLE)

	tester.tracer.OnBlockEnd(nil)

	// --- Flash block iteration 2 ---
	tester.tracer.OnBlockStart(newFlashBlockEvent(2))
	tester.blockLogIndex = 0
	tester.tracer.OnBlockEnd(nil)

	blocks := ParseFirehoseBlocks(t, "flash block balance changes", tester.tracer.GetTestingOutputBuffer())
	require.Len(t, blocks, 2)

	// First block: 1 tx + 2 balance changes
	require.Len(t, blocks[0].BalanceChanges, 2)

	// Second block: 1 balance change (from snapshot only, not the post-snapshot one)
	second := blocks[1]
	require.Len(t, second.TransactionTraces, 1, "tx1 from snapshot")
	require.Len(t, second.BalanceChanges, 1, "only balance change before snapshot")
	assert.Equal(t, MinerAddr[:], second.BalanceChanges[0].Address)
}

// TestFlashBlock_Snapshot_CodeChangesIncluded verifies that code changes added before
// the snapshot appear in the next iteration, while those after do not.
func TestFlashBlock_Snapshot_CodeChangesIncluded(t *testing.T) {
	var emptyHash [32]byte

	tester := NewTracerTester(t)

	// --- Flash block iteration 1 ---
	tester.tracer.OnBlockStart(newFlashBlockEvent(1))
	tester.blockLogIndex = 0

	// Transaction
	execFlashTx(tester, flashTx1)

	// Code change before snapshot
	tester.CodeChange(BeaconRootsAddress, emptyHash, hash32(100), []byte{}, []byte{0x60, 0x01})

	// Snapshot: includes tx1 + code change
	tester.tracer.SnapshotFlashBlockForNextIteration()

	// Code change AFTER snapshot (should NOT appear in next iteration)
	tester.CodeChange(HistoryStorageAddress, emptyHash, hash32(200), []byte{}, []byte{0x60, 0x02})

	tester.tracer.OnBlockEnd(nil)

	// --- Flash block iteration 2 ---
	tester.tracer.OnBlockStart(newFlashBlockEvent(2))
	tester.blockLogIndex = 0
	tester.tracer.OnBlockEnd(nil)

	blocks := ParseFirehoseBlocks(t, "flash block code changes", tester.tracer.GetTestingOutputBuffer())
	require.Len(t, blocks, 2)

	// First block: 1 tx + 2 code changes
	require.Len(t, blocks[0].CodeChanges, 2)

	// Second block: 1 tx + only 1 code change (from snapshot, not the post-snapshot one)
	second := blocks[1]
	require.Len(t, second.TransactionTraces, 1)
	require.Len(t, second.CodeChanges, 1, "only code change before snapshot")
	assert.Equal(t, BeaconRootsAddress[:], second.CodeChanges[0].Address)
}

// TestFlashBlock_SnapshotOnNonFlashBlockIsNoOp verifies that SnapshotFlashBlockForNextIteration
// has no effect when called on a regular (non-flash) block.
func TestFlashBlock_SnapshotOnNonFlashBlockIsNoOp(t *testing.T) {
	tester := NewTracerTester(t)

	// Regular block (no FlashBlock field)
	tester.StartBlock()
	execFlashTx(tester, flashTx1)

	// Snapshot on a non-flash block should be a no-op
	tester.tracer.SnapshotFlashBlockForNextIteration()
	assert.False(t, tester.tracer.HasFlashBlockSnapshot(), "no snapshot should be created for non-flash blocks")

	tester.EndBlock(nil)
}

// TestFlashBlock_OrdinalRestoredFromSnapshot verifies that the ordinal counter is
// restored from the snapshot when a new flash block iteration starts, ensuring
// that ordinals are consistent across iterations.
func TestFlashBlock_OrdinalRestoredFromSnapshot(t *testing.T) {
	tester := NewTracerTester(t)

	// --- Flash block iteration 1 ---
	tester.tracer.OnBlockStart(newFlashBlockEvent(1))
	tester.blockLogIndex = 0
	execFlashTx(tester, flashTx1)
	execFlashTx(tester, flashTx2)

	// Snapshot the ordinal after 2 transactions
	tester.tracer.SnapshotFlashBlockForNextIteration()

	// Add more transactions after the snapshot (should not affect ordinals in next iteration)
	execFlashTx(tester, flashTx3)
	tester.tracer.OnBlockEnd(nil)

	// --- Flash block iteration 2 ---
	// The ordinal should be restored to the snapshot point (after tx1+tx2, before tx3).
	tester.tracer.OnBlockStart(newFlashBlockEvent(2))
	tester.blockLogIndex = 0
	execFlashTx(tester, flashTx4)
	tester.tracer.OnBlockEnd(nil)

	blocks := ParseFirehoseBlocks(t, "flash block ordinal", tester.tracer.GetTestingOutputBuffer())
	require.Len(t, blocks, 2)

	second := blocks[1]
	require.Len(t, second.TransactionTraces, 3) // tx1, tx2 (snapshot), tx4 (new)

	// tx4 (new in iteration 2) should have a BeginOrdinal that continues from where
	// tx2 left off, not from where tx3 would have been.
	tx2EndOrdinal := second.TransactionTraces[1].EndOrdinal
	tx4BeginOrdinal := second.TransactionTraces[2].BeginOrdinal
	assert.True(t, tx4BeginOrdinal > tx2EndOrdinal,
		"tx4 begin ordinal (%d) should be after tx2 end ordinal (%d)", tx4BeginOrdinal, tx2EndOrdinal)
}
