package tests

import (
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go/v4"
	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
)

func TestTracer_CREATE_AddressCalculation(t *testing.T) {
	t.Run("create_with_nonce_0", func(t *testing.T) {
		// Test that CREATE address is calculated correctly using sender nonce
		// Expected address = firehose.CreateAddress(AliceAddr, 0)
		expectedAddr := firehose.CreateAddress(AliceAddr, 0)

		NewTracerTester(t).
			SetMockStateNonce(AliceAddr, 0).
			StartBlockTrx(TestLegacyTrx).
			StartCreateCall(AliceAddr, expectedAddr, bigInt(0), 53000, []byte{0x60, 0x80}).
			EndCall([]byte{}, 50000).
			EndBlockTrx(successReceipt(53000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, pbeth.CallType_CREATE, call.CallType)
				assert.Equal(t, expectedAddr[:], call.Address, "CREATE address should match computed address")
			})
	})

	t.Run("create_with_nonce_5", func(t *testing.T) {
		// Test with non-zero nonce
		expectedAddr := firehose.CreateAddress(AliceAddr, 5)

		NewTracerTester(t).
			SetMockStateNonce(AliceAddr, 5).
			StartBlockTrx(TestLegacyTrx).
			StartCreateCall(AliceAddr, expectedAddr, bigInt(0), 53000, []byte{0x60, 0x80}).
			EndCall([]byte{}, 50000).
			EndBlockTrx(successReceipt(53000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, pbeth.CallType_CREATE, call.CallType)
				assert.Equal(t, expectedAddr[:], call.Address, "CREATE address should match computed address with nonce 5")
			})
	})
}
