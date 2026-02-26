package firehose

import (
	"testing"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
)

// TestTracer_SystemCall tests system call scenarios
// System calls are protocol-level calls executed outside regular transactions
// Examples: Beacon root updates (EIP-4788), parent hash storage (EIP-2935)
func TestTracer_SystemCall(t *testing.T) {
	t.Run("beacon_root_system_call", func(t *testing.T) {
		// EIP-4788: Beacon block root stored in contract
		beaconRoot := hash32(12345) // Simulated beacon root

		NewTracerTester(t).
			StartBlock().
			// System call happens before any transactions
			SystemCall(
				SystemAddress,       // from: system address (0xff...fe)
				BeaconRootsAddress,  // to: beacon roots contract
				beaconRoot[:],       // input: beacon root hash
				30_000_000,          // gas: 30M gas limit
				[]byte{},            // output: no return data
				50_000,              // gasUsed: ~50k gas consumed
			).
			// Then regular transaction
			StartTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				// Verify system call was recorded
				assert.Equal(t, 1, len(block.SystemCalls), "Should have 1 system call")
				assert.Equal(t, 1, len(block.TransactionTraces), "Should have 1 transaction")

				// Verify system call details
				sysCall := block.SystemCalls[0]
				assert.Equal(t, SystemAddress[:], sysCall.Caller)
				assert.Equal(t, BeaconRootsAddress[:], sysCall.Address)
				assert.Equal(t, beaconRoot[:], sysCall.Input)
				assert.Equal(t, uint64(30_000_000), sysCall.GasLimit)
				assert.Equal(t, uint64(50_000), sysCall.GasConsumed)
				assert.Equal(t, pbeth.CallType_CALL, sysCall.CallType)

				// Verify ordinals are assigned
				assert.NotEqual(t, uint64(0), sysCall.BeginOrdinal)
				assert.NotEqual(t, uint64(0), sysCall.EndOrdinal)
				assert.True(t, sysCall.BeginOrdinal < sysCall.EndOrdinal)
			})
	})

	t.Run("parent_hash_system_call", func(t *testing.T) {
		// EIP-2935/7709: Parent block hash storage
		parentHash := hash32(99999)

		NewTracerTester(t).
			StartBlock().
			SystemCall(
				SystemAddress,
				HistoryStorageAddress,
				parentHash[:],
				30_000_000,
				[]byte{},
				45_000,
			).
			StartTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 1, len(block.SystemCalls))
				sysCall := block.SystemCalls[0]
				assert.Equal(t, HistoryStorageAddress[:], sysCall.Address)
				assert.Equal(t, parentHash[:], sysCall.Input)
			})
	})

	t.Run("multiple_system_calls", func(t *testing.T) {
		// Multiple system calls in same block
		beaconRoot := hash32(1111)
		parentHash := hash32(2222)

		NewTracerTester(t).
			StartBlock().
			// First system call: beacon root
			SystemCall(
				SystemAddress,
				BeaconRootsAddress,
				beaconRoot[:],
				30_000_000,
				[]byte{},
				50_000,
			).
			// Second system call: parent hash
			SystemCall(
				SystemAddress,
				HistoryStorageAddress,
				parentHash[:],
				30_000_000,
				[]byte{},
				45_000,
			).
			// Then transaction
			StartTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 2, len(block.SystemCalls), "Should have 2 system calls")

				// Verify first system call
				sysCall1 := block.SystemCalls[0]
				assert.Equal(t, BeaconRootsAddress[:], sysCall1.Address)
				assert.Equal(t, beaconRoot[:], sysCall1.Input)

				// Verify second system call
				sysCall2 := block.SystemCalls[1]
				assert.Equal(t, HistoryStorageAddress[:], sysCall2.Address)
				assert.Equal(t, parentHash[:], sysCall2.Input)

				// Verify ordinal ordering
				assert.True(t, sysCall1.EndOrdinal < sysCall2.BeginOrdinal,
					"System calls should have sequential ordinals")
			})
	})

	t.Run("system_call_with_storage_changes", func(t *testing.T) {
		// System call that makes storage changes
		beaconRoot := hash32(5555)
		storageKey := hash32(1)
		storageValue := hash32(12345)
		var zeroVal [32]byte

		NewTracerTester(t).
			StartBlock().
			StartSystemCall().
			StartCallRaw(0, byte(CallTypeCall), SystemAddress, BeaconRootsAddress, beaconRoot[:], 30_000_000, bigInt(0)).
			// System call modifies storage
			StorageChange(BeaconRootsAddress, storageKey, zeroVal, storageValue).
			EndCall([]byte{}, 50_000, nil).
			EndSystemCall().
			StartTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 1, len(block.SystemCalls))
				sysCall := block.SystemCalls[0]

				// Verify storage change was recorded
				assert.Equal(t, 1, len(sysCall.StorageChanges))
				assert.Equal(t, BeaconRootsAddress[:], sysCall.StorageChanges[0].Address)
				assert.Equal(t, storageKey[:], sysCall.StorageChanges[0].Key)
				assert.Equal(t, storageValue[:], sysCall.StorageChanges[0].NewValue)
			})
	})

	t.Run("system_call_before_transactions", func(t *testing.T) {
		// System call happens before any transactions (most common case)
		beaconRoot := hash32(7777)

		NewTracerTester(t).
			StartBlock().
			SystemCall(SystemAddress, BeaconRootsAddress, beaconRoot[:], 30_000_000, []byte{}, 50_000).
			// First transaction
			StartTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			EndTrx(successReceipt(21000), nil).
			// Second transaction
			StartTrxNoHooks().
			StartRootCall(CharlieAddr, MinerAddr, bigInt(200), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 1, len(block.SystemCalls))
				assert.Equal(t, 2, len(block.TransactionTraces))

				// System call ordinals should be before transaction ordinals
				sysCall := block.SystemCalls[0]
				firstTrx := block.TransactionTraces[0]
				assert.True(t, sysCall.EndOrdinal < firstTrx.BeginOrdinal,
					"System call should complete before first transaction")
			})
	})

	t.Run("system_call_ordinal_assignment", func(t *testing.T) {
		// Verify ordinals are correctly assigned for system calls
		beaconRoot := hash32(8888)

		NewTracerTester(t).
			StartBlock().
			SystemCall(SystemAddress, BeaconRootsAddress, beaconRoot[:], 30_000_000, []byte{}, 50_000).
			StartTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				sysCall := block.SystemCalls[0]

				// System call should have non-zero ordinals
				assert.NotEqual(t, uint64(0), sysCall.BeginOrdinal)
				assert.NotEqual(t, uint64(0), sysCall.EndOrdinal)

				// BeginOrdinal < EndOrdinal
				assert.True(t, sysCall.BeginOrdinal < sysCall.EndOrdinal)
			})
	})

	t.Run("system_call_no_transactions", func(t *testing.T) {
		// Block with only system calls, no transactions
		beaconRoot := hash32(9999)

		NewTracerTester(t).
			StartBlock().
			SystemCall(SystemAddress, BeaconRootsAddress, beaconRoot[:], 30_000_000, []byte{}, 50_000).
			EndBlock(nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 1, len(block.SystemCalls))
				assert.Equal(t, 0, len(block.TransactionTraces))

				sysCall := block.SystemCalls[0]
				assert.Equal(t, BeaconRootsAddress[:], sysCall.Address)
			})
	})

	t.Run("system_call_before_and_after_transaction", func(t *testing.T) {
		// System call → Transaction → System call
		// Tests ordinal sequencing: sys1(1-2) → trx(3-6) → sys2(7-8)
		beaconRoot1 := hash32(1111)
		beaconRoot2 := hash32(2222)

		NewTracerTester(t).
			StartBlock().
			// First system call
			SystemCall(SystemAddress, BeaconRootsAddress, beaconRoot1[:], 30_000_000, []byte{}, 50_000).
			// Transaction
			StartTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(100), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			EndTrx(successReceipt(21000), nil).
			// Second system call
			SystemCall(SystemAddress, BeaconRootsAddress, beaconRoot2[:], 30_000_000, []byte{}, 50_000).
			EndBlock(nil).
			Validate(func(block *pbeth.Block) {
				assert.Equal(t, 2, len(block.SystemCalls), "Should have 2 system calls")
				assert.Equal(t, 1, len(block.TransactionTraces), "Should have 1 transaction")

				// First system call
				sysCall1 := block.SystemCalls[0]
				assert.Equal(t, beaconRoot1[:], sysCall1.Input)
				assert.Equal(t, uint64(1), sysCall1.BeginOrdinal)
				assert.Equal(t, uint64(2), sysCall1.EndOrdinal)

				// Transaction (ordinals continue from first system call)
				trx := block.TransactionTraces[0]
				assert.Equal(t, uint64(3), trx.BeginOrdinal)
				assert.Equal(t, uint64(6), trx.EndOrdinal)

				// Second system call (ordinals continue from transaction)
				sysCall2 := block.SystemCalls[1]
				assert.Equal(t, beaconRoot2[:], sysCall2.Input)
				assert.Equal(t, uint64(7), sysCall2.BeginOrdinal)
				assert.Equal(t, uint64(8), sysCall2.EndOrdinal)

				// Verify ordinal ordering across all elements
				assert.True(t, sysCall1.EndOrdinal < trx.BeginOrdinal,
					"First system call should complete before transaction")
				assert.True(t, trx.EndOrdinal < sysCall2.BeginOrdinal,
					"Transaction should complete before second system call")
			})
	})
}
