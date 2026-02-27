package tests

import (
	"testing"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
)

// TestTracer_OnCodeChange tests all code change scenarios
func TestTracer_OnCodeChange(t *testing.T) {
	t.Run("code_change_with_active_call", func(t *testing.T) {
		// Normal code deployment during active call (CREATE/CREATE2)
		code := []byte{0x60, 0x80, 0x60, 0x40} // Simple bytecode
		codeHash := hash32(123)
		var prevHash [32]byte // Empty for new deployment

		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			CodeChange(BobAddr, prevHash, codeHash, nil, code).
			EndCall([]byte{}, 90000, nil).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 1, len(call.CodeChanges))
				cc := call.CodeChanges[0]
				assert.Equal(t, BobAddr[:], cc.Address)
				assert.Equal(t, prevHash[:], cc.OldHash)
				assert.Equal(t, codeHash[:], cc.NewHash)
				assert.Equal(t, code, cc.NewCode)
			})
	})

	t.Run("code_change_deferred_state_eip7702", func(t *testing.T) {
		// EIP-7702: Code change before call stack initialization
		// This happens when SetCode transaction sets delegation
		code := []byte{0xef, 0x01, 0x00} // EIP-7702 delegation bytecode
		codeHash := hash32(456)
		var prevHash [32]byte

		NewTracerTester(t).
			StartBlockTrxNoHooks().
			// Code change BEFORE call starts (EIP-7702 authorization)
			CodeChange(AliceAddr, prevHash, codeHash, nil, code).
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 21000, []byte{}).
			EndCall([]byte{}, 21000, nil).
			EndBlockTrx(successReceipt(21000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				// Deferred code change should be applied to root call
				assert.Equal(t, 1, len(call.CodeChanges))
				cc := call.CodeChanges[0]
				assert.Equal(t, AliceAddr[:], cc.Address)
				assert.Equal(t, code, cc.NewCode)
			})
	})

	t.Run("code_change_block_level", func(t *testing.T) {
		// Block-level code change (no transaction context)
		code := []byte{0x60, 0x01}
		codeHash := hash32(789)
		var prevHash [32]byte

		NewTracerTester(t).
			StartBlock().
			CodeChange(CharlieAddr, prevHash, codeHash, nil, code).
			EndBlock(nil).
			Validate(func(block *pbeth.Block) {
				// Block-level code changes
				assert.Equal(t, 1, len(block.CodeChanges))
				cc := block.CodeChanges[0]
				assert.Equal(t, CharlieAddr[:], cc.Address)
				assert.Equal(t, prevHash[:], cc.OldHash)
				assert.Equal(t, codeHash[:], cc.NewHash)
				assert.Equal(t, code, cc.NewCode)
			})
	})

	t.Run("code_change_with_previous_code", func(t *testing.T) {
		// Code change replacing existing code (upgrade scenario)
		oldCode := []byte{0x60, 0x01}
		newCode := []byte{0x60, 0x02}
		oldHash := hash32(111)
		newHash := hash32(222)

		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			CodeChange(BobAddr, oldHash, newHash, oldCode, newCode).
			EndCall([]byte{}, 90000, nil).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 1, len(call.CodeChanges))
				cc := call.CodeChanges[0]
				assert.Equal(t, oldHash[:], cc.OldHash)
				assert.Equal(t, oldCode, cc.OldCode)
				assert.Equal(t, newHash[:], cc.NewHash)
				assert.Equal(t, newCode, cc.NewCode)
			})
	})

	t.Run("multiple_code_changes_in_call", func(t *testing.T) {
		// Multiple code deployments in same call
		code1 := []byte{0x60, 0x01}
		code2 := []byte{0x60, 0x02}
		hash1 := hash32(333)
		hash2 := hash32(444)
		var prevHash [32]byte

		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 200000, []byte{}).
			CodeChange(CharlieAddr, prevHash, hash1, nil, code1).
			CodeChange(BobAddr, prevHash, hash2, nil, code2).
			EndCall([]byte{}, 180000, nil).
			EndBlockTrx(successReceipt(200000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 2, len(call.CodeChanges), "Should have 2 code changes")

				// Verify ordering (ordinals should be increasing)
				assert.True(t, call.CodeChanges[0].Ordinal < call.CodeChanges[1].Ordinal)

				// Verify addresses
				assert.Equal(t, CharlieAddr[:], call.CodeChanges[0].Address)
				assert.Equal(t, BobAddr[:], call.CodeChanges[1].Address)
			})
	})

	t.Run("code_change_hash_and_code_stored", func(t *testing.T) {
		// Verify both hash and code are stored correctly
		code := []byte{0x60, 0x80, 0x60, 0x40, 0x52, 0x60, 0x04, 0x36}
		codeHash := hash32(555)
		var prevHash [32]byte

		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			CodeChange(BobAddr, prevHash, codeHash, nil, code).
			EndCall([]byte{}, 90000, nil).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 1, len(call.CodeChanges))
				cc := call.CodeChanges[0]

				// Verify hash is stored
				assert.Equal(t, codeHash[:], cc.NewHash)
				assert.NotEqual(t, prevHash[:], cc.NewHash, "New hash should differ from empty")

				// Verify code is stored
				assert.Equal(t, code, cc.NewCode)
				assert.Equal(t, len(code), len(cc.NewCode))
			})
	})

	t.Run("code_change_empty_code", func(t *testing.T) {
		// Code change with empty code (rare but valid)
		var emptyCode []byte
		codeHash := hash32(666)
		var prevHash [32]byte

		NewTracerTester(t).
			StartBlockTrxNoHooks().
			StartRootCall(AliceAddr, BobAddr, bigInt(0), 100000, []byte{}).
			CodeChange(BobAddr, prevHash, codeHash, nil, emptyCode).
			EndCall([]byte{}, 90000, nil).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				call := trx.Calls[0]

				assert.Equal(t, 1, len(call.CodeChanges))
				cc := call.CodeChanges[0]
				assert.Equal(t, BobAddr[:], cc.Address)
				// Empty code should be nil or empty slice
				assert.True(t, len(cc.NewCode) == 0)
			})
	})
}
