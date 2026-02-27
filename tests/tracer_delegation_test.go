package tests

import (
	"testing"

	firehose "github.com/streamingfast/evm-firehose-tracer-go"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTracer_EIP7702_DelegationDetection tests the detection of EIP-7702 delegation
// designators in contract code (0xef0100 prefix)
func TestTracer_EIP7702_DelegationDetection(t *testing.T) {
	t.Run("call_with_delegation_bytecode", func(t *testing.T) {
		// Create delegation bytecode: 0xef0100 + address (23 bytes total)
		// Delegation points to CharlieAddr
		delegationCode := append([]byte{0xef, 0x01, 0x00}, CharlieAddr[:]...)

		// Alice has delegation code pointing to Charlie
		// Set mock state so GetCode returns delegation bytecode
		auth, err := firehose.SignSetCodeAuth(AliceKey, 1, CharlieAddr, 0)
		require.NoError(t, err)

		txEvent := new(TxEventBuilder).
			Defaults().
			Type(TxTypeSetCode).
			SetCodeAuthorizations([]firehose.SetCodeAuthorization{auth}).
			Build()

		tester := NewTracerTesterPrague(t).
			SetMockStateCode(AliceAddr, delegationCode). // Set delegation code in mock state
			startBlockTrxWithEvent(txEvent)

		// EIP-7702: Authorization nonce change happens before root call
		tester.NonceChange(AliceAddr, 0, 1)

		// Code change: Alice gets delegation code
		tester.CodeChange(AliceAddr, hashBytes([]byte{}), hashBytes(delegationCode), []byte{}, delegationCode)

		tester.
			StartRootCall(BobAddr, AliceAddr, bigInt(100), 21000, []byte{0x01, 0x02}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Verify delegation was detected
				assert.NotNil(t, call.AddressDelegatesTo, "AddressDelegatesTo should be set")
				assert.Equal(t, CharlieAddr[:], call.AddressDelegatesTo, "Should delegate to CharlieAddr")
			})
	})

	t.Run("call_with_empty_code", func(t *testing.T) {
		// Alice has no code (empty account)
		// No delegation should be detected
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(BobAddr, AliceAddr, bigInt(100), 21000, []byte{0x01, 0x02}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// No delegation for empty code
				assert.Nil(t, call.AddressDelegatesTo, "AddressDelegatesTo should be nil for empty code")
			})
	})

	t.Run("call_with_regular_contract_code", func(t *testing.T) {
		// Alice has regular contract code (not delegation)
		regularCode := []byte{0x60, 0x80, 0x60, 0x40} // Some EVM bytecode

		NewTracerTester(t).
			StartBlockTrxNoHooks().
			CodeChange(AliceAddr, hashBytes([]byte{}), hashBytes(regularCode), []byte{}, regularCode).
			StartRootCall(BobAddr, AliceAddr, bigInt(100), 21000, []byte{0x01, 0x02}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// No delegation for regular code
				assert.Nil(t, call.AddressDelegatesTo, "AddressDelegatesTo should be nil for regular contract code")
			})
	})

	t.Run("call_with_invalid_delegation_wrong_length", func(t *testing.T) {
		// Invalid delegation: correct prefix but wrong length (should be 23 bytes)
		invalidDelegation := []byte{0xef, 0x01, 0x00, 0x11, 0x22} // Only 5 bytes

		NewTracerTester(t).
			StartBlockTrxNoHooks().
			CodeChange(AliceAddr, hashBytes([]byte{}), hashBytes(invalidDelegation), []byte{}, invalidDelegation).
			StartRootCall(BobAddr, AliceAddr, bigInt(100), 21000, []byte{0x01, 0x02}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// No delegation for invalid length
				assert.Nil(t, call.AddressDelegatesTo, "AddressDelegatesTo should be nil for invalid delegation length")
			})
	})

	t.Run("call_with_invalid_delegation_wrong_prefix", func(t *testing.T) {
		// Invalid delegation: wrong prefix (23 bytes but doesn't start with 0xef0100)
		invalidDelegation := make([]byte, 23)
		invalidDelegation[0] = 0xef
		invalidDelegation[1] = 0x01
		invalidDelegation[2] = 0x01 // Wrong! Should be 0x00
		copy(invalidDelegation[3:], CharlieAddr[:])

		NewTracerTester(t).
			StartBlockTrxNoHooks().
			CodeChange(AliceAddr, hashBytes([]byte{}), hashBytes(invalidDelegation), []byte{}, invalidDelegation).
			StartRootCall(BobAddr, AliceAddr, bigInt(100), 21000, []byte{0x01, 0x02}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// No delegation for wrong prefix
				assert.Nil(t, call.AddressDelegatesTo, "AddressDelegatesTo should be nil for invalid delegation prefix")
			})
	})

	t.Run("create_transaction_no_delegation_check", func(t *testing.T) {
		// CREATE transactions should NOT check for delegation (callType != CREATE check in code)
		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCreateCall(AliceAddr, [20]byte{}, bigInt(0), 21000, []byte{0x60, 0x80}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, pbeth.CallType_CREATE, call.CallType)
				// CREATE calls should not have delegation check
				assert.Nil(t, call.AddressDelegatesTo)
			})
	})
}
