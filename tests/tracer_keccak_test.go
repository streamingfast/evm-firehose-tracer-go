package tests

import (
	"encoding/hex"
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go/v5"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTracer_KeccakPreimages tests that only the keccak preimages explaining a storage slot are kept
func TestTracer_KeccakPreimages(t *testing.T) {
	storedValue := hashBytes([]byte{0x01})

	t.Run("preimage_of_written_slot_is_kept", func(t *testing.T) {
		preimage := []byte("hello")
		hash := hashBytes(preimage)

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			Keccak(hash, preimage).
			StorageChange(BobAddr, hash, firehose.EmptyHash, storedValue).
			EndCall([]byte{}, 95000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				call := block.TransactionTraces[0].Calls[0]

				require.Len(t, call.KeccakPreimages, 1)
				assert.Equal(t, hex.EncodeToString(preimage), call.KeccakPreimages[hex.EncodeToString(hash[:])])
			})
	})

	t.Run("preimage_not_explaining_a_slot_is_dropped", func(t *testing.T) {
		preimage := []byte("event_signature")

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			Keccak(hashBytes(preimage), preimage).
			EndCall([]byte{}, 95000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				assert.Nil(t, block.TransactionTraces[0].Calls[0].KeccakPreimages)
			})
	})

	t.Run("only_written_slots_kept_among_several", func(t *testing.T) {
		preimage1, preimage2, preimage3 := []byte("storage_slot_1"), []byte("storage_slot_2"), []byte("event_signature")
		hash1, hash2, hash3 := hashBytes(preimage1), hashBytes(preimage2), hashBytes(preimage3)

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			Keccak(hash1, preimage1).
			Keccak(hash2, preimage2).
			Keccak(hash3, preimage3).
			StorageChange(BobAddr, hash1, firehose.EmptyHash, storedValue).
			StorageChange(BobAddr, hash2, firehose.EmptyHash, storedValue).
			EndCall([]byte{}, 95000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				call := block.TransactionTraces[0].Calls[0]

				require.Len(t, call.KeccakPreimages, 2)
				assert.Equal(t, hex.EncodeToString(preimage1), call.KeccakPreimages[hex.EncodeToString(hash1[:])])
				assert.Equal(t, hex.EncodeToString(preimage2), call.KeccakPreimages[hex.EncodeToString(hash2[:])])
			})
	})

	t.Run("preimage_kept_on_the_call_that_computed_it", func(t *testing.T) {
		preimage := []byte("parent_data")
		hash := hashBytes(preimage)

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			Keccak(hash, preimage).
			StartCall(BobAddr, CharlieAddr, bigInt(0), 50000, []byte{0x02}).
			StorageChange(CharlieAddr, hash, firehose.EmptyHash, storedValue).
			EndCall([]byte{}, 45000).
			EndCall([]byte{}, 95000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				parentCall, childCall := block.TransactionTraces[0].Calls[0], block.TransactionTraces[0].Calls[1]

				require.Len(t, parentCall.KeccakPreimages, 1)
				assert.Equal(t, hex.EncodeToString(preimage), parentCall.KeccakPreimages[hex.EncodeToString(hash[:])])
				assert.Nil(t, childCall.KeccakPreimages)
			})
	})

	t.Run("empty_preimage_stored_as_empty_string", func(t *testing.T) {
		hash := hashBytes([]byte{})

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			Keccak(hash, []byte{}).
			StorageChange(BobAddr, hash, firehose.EmptyHash, storedValue).
			EndCall([]byte{}, 95000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				call := block.TransactionTraces[0].Calls[0]

				require.Len(t, call.KeccakPreimages, 1)
				assert.Equal(t, "", call.KeccakPreimages[hex.EncodeToString(hash[:])])
			})
	})

	t.Run("preimage_larger_than_256_bytes_is_dropped", func(t *testing.T) {
		atLimit, overLimit := make([]byte, 256), make([]byte, 257)
		for i := range overLimit {
			overLimit[i] = byte(i)
		}
		atLimitHash, overLimitHash := hashBytes(atLimit), hashBytes(overLimit)

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			Keccak(atLimitHash, atLimit).
			Keccak(overLimitHash, overLimit).
			StorageChange(BobAddr, atLimitHash, firehose.EmptyHash, storedValue).
			StorageChange(BobAddr, overLimitHash, firehose.EmptyHash, storedValue).
			EndCall([]byte{}, 95000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				call := block.TransactionTraces[0].Calls[0]

				require.Len(t, call.KeccakPreimages, 1)
				assert.Contains(t, call.KeccakPreimages, hex.EncodeToString(atLimitHash[:]))
			})
	})

	t.Run("mapping_slot_preimage", func(t *testing.T) {
		// mapping(address => uint256) at slot 0: keccak256(abi.encode(key, slot))
		preimage := append(append(make([]byte, 12), AliceAddr[:]...), make([]byte, 32)...)
		hash := hashBytes(preimage)

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			Keccak(hash, preimage).
			StorageChange(BobAddr, hash, firehose.EmptyHash, storedValue).
			EndCall([]byte{}, 95000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				call := block.TransactionTraces[0].Calls[0]

				require.Len(t, call.KeccakPreimages, 1)
				assert.Equal(t, hex.EncodeToString(preimage), call.KeccakPreimages[hex.EncodeToString(hash[:])])
				require.Len(t, call.StorageChanges, 1)
				assert.Equal(t, hash[:], call.StorageChanges[0].Key)
			})
	})

	t.Run("system_call_preimage", func(t *testing.T) {
		preimage := []byte("system_slot")
		hash := hashBytes(preimage)

		NewTracerTester(t).
			StartBlock().
			StartSystemCall().
			StartCall(AliceAddr, CharlieAddr, bigInt(0), 30000000, nil).
			Keccak(hash, preimage).
			StorageChange(CharlieAddr, hash, firehose.EmptyHash, storedValue).
			EndCall(nil, 0).
			EndSystemCall().
			EndBlock(nil).
			Validate(func(block *pbeth.Block) {
				require.Len(t, block.SystemCalls, 1)
				assert.Equal(t, hex.EncodeToString(preimage), block.SystemCalls[0].KeccakPreimages[hex.EncodeToString(hash[:])])
			})
	})
}
