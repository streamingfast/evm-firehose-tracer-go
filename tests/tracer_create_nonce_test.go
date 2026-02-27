package tests

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
)

func TestTracer_CREATE_AddressCalculation(t *testing.T) {
	t.Run("create_with_nonce_0", func(t *testing.T) {
		// Test that CREATE address is calculated correctly using sender nonce
		// Expected address = crypto.CreateAddress(AliceAddr, 0)
		expectedAddr := crypto.CreateAddress(toCommonAddress(AliceAddr), 0)

		NewTracerTester(t).
			SetMockStateNonce(AliceAddr, 0).
			StartBlockTrx(TestLegacyTrx).
			StartRootCreateCall(AliceAddr, AddrBytes(expectedAddr), bigInt(0), 53000, []byte{0x60, 0x80}).
			EndCall([]byte{}, 50000).
			EndBlockTrx(successReceipt(53000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, pbeth.CallType_CREATE, call.CallType)
				assert.Equal(t, expectedAddr.Bytes(), call.Address, "CREATE address should match computed address")
			})
	})

	t.Run("create_with_nonce_5", func(t *testing.T) {
		// Test with non-zero nonce
		expectedAddr := crypto.CreateAddress(toCommonAddress(AliceAddr), 5)

		NewTracerTester(t).
			SetMockStateNonce(AliceAddr, 5).
			StartBlockTrx(TestLegacyTrx).
			StartRootCreateCall(AliceAddr, AddrBytes(expectedAddr), bigInt(0), 53000, []byte{0x60, 0x80}).
			EndCall([]byte{}, 50000).
			EndBlockTrx(successReceipt(53000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, pbeth.CallType_CREATE, call.CallType)
				assert.Equal(t, expectedAddr.Bytes(), call.Address, "CREATE address should match computed address with nonce 5")
			})
	})
}

func AddrBytes(addr common.Address) [20]byte {
	var result [20]byte
	copy(result[:], addr.Bytes())
	return result
}
