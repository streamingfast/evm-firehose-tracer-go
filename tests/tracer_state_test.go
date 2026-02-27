package tests

import (
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go"

	"github.com/stretchr/testify/assert"
)

// TestTracer_State tests state change tracking hooks
func TestTracer_State(t *testing.T) {
	t.Run("log_conversion", func(t *testing.T) {
		// Test the conversion function directly
		addr := AliceAddr
		topics := [][32]byte{
			hashFromHex("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"), // Transfer event
			hashFromHex("0x0000000000000000000000000000000000000000000000000000000000000001"),
		}
		data := []byte{0xaa, 0xbb, 0xcc, 0xdd}
		blockIndex := uint32(5)

		// This validates that our conversion function works correctly
		// The native validator will use this same conversion
		nativeLog := firehose.ConvertToNativeLog(addr, topics, data, blockIndex)

		assert.Equal(t, addr[:], nativeLog.Address[:])
		assert.Equal(t, 2, len(nativeLog.Topics))
		assert.Equal(t, topics[0][:], nativeLog.Topics[0][:])
		assert.Equal(t, topics[1][:], nativeLog.Topics[1][:])
		assertEqualBytes(t, data, nativeLog.Data)
		assert.Equal(t, uint(blockIndex), nativeLog.Index)
	})
}
