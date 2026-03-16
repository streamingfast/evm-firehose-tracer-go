package tests

import (
	"encoding/hex"
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go/v4"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTracer_KeccakPreimages tests keccak preimage storage
func TestTracer_KeccakPreimages(t *testing.T) {
	t.Run("single_keccak_preimage", func(t *testing.T) {
		// Scenario: Contract computes keccak256 of some data
		// Example: keccak256("hello") = 0x1c8aff950685c2ed4bc3174f3472287b56d9517b9c948127319a09a7a36deac8
		preimage := []byte("hello")
		hash := hashBytes(preimage) // Compute actual keccak256

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			Keccak(hash, preimage).
			EndCall([]byte{}, 95000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				require.NotNil(t, call.KeccakPreimages, "KeccakPreimages should not be nil")
				assert.Equal(t, 1, len(call.KeccakPreimages), "Should have 1 keccak preimage")

				hashHex := hex.EncodeToString(hash[:])
				preimageHex := hex.EncodeToString(preimage)

				assert.Equal(t, preimageHex, call.KeccakPreimages[hashHex], "Preimage should match")
			})
	})

	t.Run("multiple_keccak_preimages_same_call", func(t *testing.T) {
		// Scenario: Contract computes multiple keccak256 hashes
		preimage1 := []byte("storage_slot_1")
		hash1 := hashBytes(preimage1)

		preimage2 := []byte("storage_slot_2")
		hash2 := hashBytes(preimage2)

		preimage3 := []byte("event_signature")
		hash3 := hashBytes(preimage3)

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			Keccak(hash1, preimage1).
			Keccak(hash2, preimage2).
			Keccak(hash3, preimage3).
			EndCall([]byte{}, 95000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				require.NotNil(t, call.KeccakPreimages, "KeccakPreimages should not be nil")
				assert.Equal(t, 3, len(call.KeccakPreimages), "Should have 3 keccak preimages")

				// Verify all three preimages
				assert.Equal(t, hex.EncodeToString(preimage1), call.KeccakPreimages[hex.EncodeToString(hash1[:])])
				assert.Equal(t, hex.EncodeToString(preimage2), call.KeccakPreimages[hex.EncodeToString(hash2[:])])
				assert.Equal(t, hex.EncodeToString(preimage3), call.KeccakPreimages[hex.EncodeToString(hash3[:])])
			})
	})

	t.Run("keccak_preimages_across_nested_calls", func(t *testing.T) {
		// Scenario: Keccak computations happen in both parent and child calls
		preimageParent := []byte("parent_data")
		hashParent := hashBytes(preimageParent)

		preimageChild := []byte("child_data")
		hashChild := hashBytes(preimageChild)

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			Keccak(hashParent, preimageParent).
			StartCall(BobAddr, CharlieAddr, bigInt(0), 50000, []byte{0x02}).
			Keccak(hashChild, preimageChild).
			EndCall([]byte{}, 45000).
			EndCall([]byte{}, 95000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				parentCall := trx.Calls[0]
				childCall := trx.Calls[1]

				// Parent call should have its preimage
				require.NotNil(t, parentCall.KeccakPreimages, "Parent KeccakPreimages should not be nil")
				assert.Equal(t, 1, len(parentCall.KeccakPreimages), "Parent should have 1 keccak preimage")
				assert.Equal(t, hex.EncodeToString(preimageParent), parentCall.KeccakPreimages[hex.EncodeToString(hashParent[:])])

				// Child call should have its preimage
				require.NotNil(t, childCall.KeccakPreimages, "Child KeccakPreimages should not be nil")
				assert.Equal(t, 1, len(childCall.KeccakPreimages), "Child should have 1 keccak preimage")
				assert.Equal(t, hex.EncodeToString(preimageChild), childCall.KeccakPreimages[hex.EncodeToString(hashChild[:])])
			})
	})

	t.Run("keccak_empty_preimage", func(t *testing.T) {
		// Scenario: keccak256 of empty data
		// Example: keccak256("") = 0xc5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470
		preimage := []byte{}
		hash := hashBytes(preimage)

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			Keccak(hash, preimage).
			EndCall([]byte{}, 95000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				require.NotNil(t, call.KeccakPreimages, "KeccakPreimages should not be nil")
				assert.Equal(t, 1, len(call.KeccakPreimages), "Should have 1 keccak preimage")

				hashHex := hex.EncodeToString(hash[:])
				// In non-backward-compatible mode (Ver 4), empty preimage is stored as empty string
				// (not as "." like the old tracer did)
				assert.Equal(t, "", call.KeccakPreimages[hashHex], "Empty preimage should be stored as empty string")
			})
	})

	t.Run("keccak_large_preimage", func(t *testing.T) {
		// Scenario: keccak256 of large data (e.g., contract bytecode, large calldata)
		preimage := make([]byte, 1024) // 1 KB of data
		for i := range preimage {
			preimage[i] = byte(i % 256)
		}
		hash := hashBytes(preimage)

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			Keccak(hash, preimage).
			EndCall([]byte{}, 95000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				require.NotNil(t, call.KeccakPreimages, "KeccakPreimages should not be nil")
				assert.Equal(t, 1, len(call.KeccakPreimages), "Should have 1 keccak preimage")

				hashHex := hex.EncodeToString(hash[:])
				preimageHex := hex.EncodeToString(preimage)

				assert.Equal(t, preimageHex, call.KeccakPreimages[hashHex], "Large preimage should match")
				assert.Equal(t, 2048, len(call.KeccakPreimages[hashHex]), "Hex-encoded preimage should be 2x original size")
			})
	})

	t.Run("keccak_storage_slot_mapping", func(t *testing.T) {
		// Real-world scenario: Mapping storage slot calculation
		// In Solidity, mapping(address => uint256) at slot 0 would compute:
		// keccak256(abi.encodePacked(key, slot))

		// Example: Storage slot for address 0x1234...5678 at mapping slot 0
		addressBytes := AliceAddr[:]
		slotBytes := make([]byte, 32) // slot 0
		preimage := append(addressBytes, slotBytes...)

		hash := hashBytes(preimage)

		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{0x01}).
			Keccak(hash, preimage).                                                    // Contract computes storage slot
			StorageChange(BobAddr, hash, firehose.EmptyHash, hashBytes([]byte{0x01})). // Then writes to that slot
			EndCall([]byte{}, 95000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Verify keccak preimage is stored
				require.NotNil(t, call.KeccakPreimages, "KeccakPreimages should not be nil")
				assert.Equal(t, 1, len(call.KeccakPreimages), "Should have 1 keccak preimage")

				hashHex := hex.EncodeToString(hash[:])
				preimageHex := hex.EncodeToString(preimage)
				assert.Equal(t, preimageHex, call.KeccakPreimages[hashHex])

				// Verify storage change also recorded
				require.Equal(t, 1, len(call.StorageChanges), "Should have 1 storage change")
				assert.Equal(t, hash[:], call.StorageChanges[0].Key, "Storage key should match keccak hash")
			})
	})
}
