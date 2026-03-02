package tests

import (
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go"
	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTracer_BlockHeader_EIPFields tests that all EIP-specific block header fields
// are properly populated from BlockData to the protobuf BlockHeader
func TestTracer_BlockHeader_EIPFields(t *testing.T) {
	t.Run("eip4895_withdrawals_root", func(t *testing.T) {
		// EIP-4895: Shanghai withdrawals root
		withdrawalsRoot := hash32(12345)

		blockEvent := firehose.BlockEvent{
			Block: firehose.BlockData{
				Number:          100,
				Hash:            hash32(1),
				ParentHash:      hash32(2),
				UncleHash:       hash32(3),
				Coinbase:        AliceAddr,
				Root:            hash32(4),
				TxHash:          hash32(5),
				ReceiptHash:     hash32(6),
				Bloom:           make([]byte, 256),
				Difficulty:      bigInt(0),
				GasLimit:        15_000_000,
				GasUsed:         0,
				Time:            1000,
				Extra:           []byte{},
				MixDigest:       hash32(7),
				Nonce:           0,
				BaseFee:         bigInt(1_000_000_000),
				Size:            1024,
				WithdrawalsRoot: &withdrawalsRoot, // EIP-4895
			},
		}

		NewTracerTester(t).
			ValidateWithCustomBlock(blockEvent, func(block *pbeth.Block) {
				require.NotNil(t, block.Header)
				require.NotNil(t, block.Header.WithdrawalsRoot)
				assert.Equal(t, withdrawalsRoot[:], block.Header.WithdrawalsRoot)
			})
	})

	t.Run("eip4844_blob_gas_fields", func(t *testing.T) {
		// EIP-4844: Cancun blob gas tracking
		blobGasUsed := uint64(262144)   // 128KB blob
		excessBlobGas := uint64(524288) // 256KB excess

		blockEvent := firehose.BlockEvent{
			Block: firehose.BlockData{
				Number:        100,
				Hash:          hash32(1),
				ParentHash:    hash32(2),
				UncleHash:     hash32(3),
				Coinbase:      AliceAddr,
				Root:          hash32(4),
				TxHash:        hash32(5),
				ReceiptHash:   hash32(6),
				Bloom:         make([]byte, 256),
				Difficulty:    bigInt(0),
				GasLimit:      15_000_000,
				GasUsed:       0,
				Time:          1000,
				Extra:         []byte{},
				MixDigest:     hash32(7),
				Nonce:         0,
				BaseFee:       bigInt(1_000_000_000),
				Size:          1024,
				BlobGasUsed:   &blobGasUsed,   // EIP-4844
				ExcessBlobGas: &excessBlobGas, // EIP-4844
			},
		}

		NewTracerTester(t).
			ValidateWithCustomBlock(blockEvent, func(block *pbeth.Block) {
				require.NotNil(t, block.Header)
				require.NotNil(t, block.Header.BlobGasUsed)
				assert.Equal(t, blobGasUsed, *block.Header.BlobGasUsed)
				require.NotNil(t, block.Header.ExcessBlobGas)
				assert.Equal(t, excessBlobGas, *block.Header.ExcessBlobGas)
			})
	})

	t.Run("eip4788_parent_beacon_root", func(t *testing.T) {
		// EIP-4788: Cancun parent beacon block root
		parentBeaconRoot := hash32(99999)

		blockEvent := firehose.BlockEvent{
			Block: firehose.BlockData{
				Number:           100,
				Hash:             hash32(1),
				ParentHash:       hash32(2),
				UncleHash:        hash32(3),
				Coinbase:         AliceAddr,
				Root:             hash32(4),
				TxHash:           hash32(5),
				ReceiptHash:      hash32(6),
				Bloom:            make([]byte, 256),
				Difficulty:       bigInt(0),
				GasLimit:         15_000_000,
				GasUsed:          0,
				Time:             1000,
				Extra:            []byte{},
				MixDigest:        hash32(7),
				Nonce:            0,
				BaseFee:          bigInt(1_000_000_000),
				Size:             1024,
				ParentBeaconRoot: &parentBeaconRoot, // EIP-4788
			},
		}

		NewTracerTester(t).
			ValidateWithCustomBlock(blockEvent, func(block *pbeth.Block) {
				require.NotNil(t, block.Header)
				require.NotNil(t, block.Header.ParentBeaconRoot)
				assert.Equal(t, parentBeaconRoot[:], block.Header.ParentBeaconRoot)
			})
	})

	t.Run("eip7685_requests_hash", func(t *testing.T) {
		// EIP-7685: Prague execution requests hash
		requestsHash := hash32(88888)

		blockEvent := firehose.BlockEvent{
			Block: firehose.BlockData{
				Number:       100,
				Hash:         hash32(1),
				ParentHash:   hash32(2),
				UncleHash:    hash32(3),
				Coinbase:     AliceAddr,
				Root:         hash32(4),
				TxHash:       hash32(5),
				ReceiptHash:  hash32(6),
				Bloom:        make([]byte, 256),
				Difficulty:   bigInt(0),
				GasLimit:     15_000_000,
				GasUsed:      0,
				Time:         1000,
				Extra:        []byte{},
				MixDigest:    hash32(7),
				Nonce:        0,
				BaseFee:      bigInt(1_000_000_000),
				Size:         1024,
				RequestsHash: &requestsHash, // EIP-7685
			},
		}

		NewTracerTester(t).
			ValidateWithCustomBlock(blockEvent, func(block *pbeth.Block) {
				require.NotNil(t, block.Header)
				require.NotNil(t, block.Header.RequestsHash)
				assert.Equal(t, requestsHash[:], block.Header.RequestsHash)
			})
	})

	t.Run("polygon_tx_dependency", func(t *testing.T) {
		// Polygon-specific: Transaction dependency metadata
		// Example: tx[1] depends on tx[0], tx[3] depends on tx[2]
		// Note: Empty slices become nil after protobuf round-trip, so we expect that
		txDependency := [][]uint64{
			nil,    // tx[0] has no dependencies
			{0},    // tx[1] depends on tx[0]
			nil,    // tx[2] has no dependencies
			{2},    // tx[3] depends on tx[2]
			{1, 3}, // tx[4] depends on both tx[1] and tx[3]
		}

		blockEvent := firehose.BlockEvent{
			Block: firehose.BlockData{
				Number:       100,
				Hash:         hash32(1),
				ParentHash:   hash32(2),
				UncleHash:    hash32(3),
				Coinbase:     AliceAddr,
				Root:         hash32(4),
				TxHash:       hash32(5),
				ReceiptHash:  hash32(6),
				Bloom:        make([]byte, 256),
				Difficulty:   bigInt(0),
				GasLimit:     15_000_000,
				GasUsed:      0,
				Time:         1000,
				Extra:        []byte{},
				MixDigest:    hash32(7),
				Nonce:        0,
				BaseFee:      bigInt(1_000_000_000),
				Size:         1024,
				TxDependency: txDependency, // Polygon-specific
			},
		}

		NewTracerTester(t).
			ValidateWithCustomBlock(blockEvent, func(block *pbeth.Block) {
				require.NotNil(t, block.Header)
				require.NotNil(t, block.Header.TxDependency)

				// Convert back to native format for comparison
				nativeDeps := block.Header.TxDependency.ToNative()
				assert.Equal(t, txDependency, nativeDeps)
			})
	})

	t.Run("all_eip_fields_combined", func(t *testing.T) {
		// Test all EIP fields together (representing a post-Prague block)
		withdrawalsRoot := hash32(11111)
		blobGasUsed := uint64(262144)
		excessBlobGas := uint64(524288)
		parentBeaconRoot := hash32(22222)
		requestsHash := hash32(33333)

		blockEvent := firehose.BlockEvent{
			Block: firehose.BlockData{
				Number:           100,
				Hash:             hash32(1),
				ParentHash:       hash32(2),
				UncleHash:        hash32(3),
				Coinbase:         AliceAddr,
				Root:             hash32(4),
				TxHash:           hash32(5),
				ReceiptHash:      hash32(6),
				Bloom:            make([]byte, 256),
				Difficulty:       bigInt(0),
				GasLimit:         15_000_000,
				GasUsed:          0,
				Time:             1000,
				Extra:            []byte{},
				MixDigest:        hash32(7),
				Nonce:            0,
				BaseFee:          bigInt(1_000_000_000),
				Size:             1024,
				WithdrawalsRoot:  &withdrawalsRoot,
				BlobGasUsed:      &blobGasUsed,
				ExcessBlobGas:    &excessBlobGas,
				ParentBeaconRoot: &parentBeaconRoot,
				RequestsHash:     &requestsHash,
			},
		}

		NewTracerTester(t).
			ValidateWithCustomBlock(blockEvent, func(block *pbeth.Block) {
				require.NotNil(t, block.Header)

				// Verify all EIP fields are present
				require.NotNil(t, block.Header.WithdrawalsRoot)
				assert.Equal(t, withdrawalsRoot[:], block.Header.WithdrawalsRoot)

				require.NotNil(t, block.Header.BlobGasUsed)
				assert.Equal(t, blobGasUsed, *block.Header.BlobGasUsed)

				require.NotNil(t, block.Header.ExcessBlobGas)
				assert.Equal(t, excessBlobGas, *block.Header.ExcessBlobGas)

				require.NotNil(t, block.Header.ParentBeaconRoot)
				assert.Equal(t, parentBeaconRoot[:], block.Header.ParentBeaconRoot)

				require.NotNil(t, block.Header.RequestsHash)
				assert.Equal(t, requestsHash[:], block.Header.RequestsHash)
			})
	})

	t.Run("nil_eip_fields_pre_fork", func(t *testing.T) {
		// Test that pre-fork blocks (without EIP fields) don't populate these fields
		blockEvent := firehose.BlockEvent{
			Block: firehose.BlockData{
				Number:      100,
				Hash:        hash32(1),
				ParentHash:  hash32(2),
				UncleHash:   hash32(3),
				Coinbase:    AliceAddr,
				Root:        hash32(4),
				TxHash:      hash32(5),
				ReceiptHash: hash32(6),
				Bloom:       make([]byte, 256),
				Difficulty:  bigInt(1000), // Pre-merge (has difficulty)
				GasLimit:    15_000_000,
				GasUsed:     0,
				Time:        1000,
				Extra:       []byte{},
				MixDigest:   hash32(7),
				Nonce:       12345,
				BaseFee:     bigInt(1_000_000_000),
				Size:        1024,
				// All EIP fields are nil (pre-fork block)
			},
		}

		NewTracerTester(t).
			ValidateWithCustomBlock(blockEvent, func(block *pbeth.Block) {
				require.NotNil(t, block.Header)

				// Verify all EIP fields are nil for pre-fork blocks
				assert.Nil(t, block.Header.WithdrawalsRoot)
				assert.Nil(t, block.Header.BlobGasUsed)
				assert.Nil(t, block.Header.ExcessBlobGas)
				assert.Nil(t, block.Header.ParentBeaconRoot)
				assert.Nil(t, block.Header.RequestsHash)
				assert.Nil(t, block.Header.TxDependency)
			})
	})
}
