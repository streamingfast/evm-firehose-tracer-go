package tests

import (
	"testing"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTracer_OpcodeGasChanges tests gas change recording for specific opcodes
func TestTracer_OpcodeGasChanges(t *testing.T) {
	t.Run("call_opcode_gas_change", func(t *testing.T) {
		// CALL opcode (0xf1) should record a gas change with REASON_CALL
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			// Simulate CALL opcode execution with gas cost
			OpCode(0, 0xf1, 100000, 9000). // CALL costs ~9000 gas
			StartCall(BobAddr, CharlieAddr, bigInt(0), 90000, []byte{}).
			EndCall([]byte{}, 85000).
			EndCall([]byte{}, 95000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				rootCall := trx.Calls[0]

				// Should have gas change recorded for CALL opcode
				require.NotEmpty(t, rootCall.GasChanges, "Should have gas changes")

				// Find the CALL gas change
				var callGasChange *pbeth.GasChange
				for _, gc := range rootCall.GasChanges {
					if gc.Reason == pbeth.GasChange_REASON_CALL {
						callGasChange = gc
						break
					}
				}

				require.NotNil(t, callGasChange, "Should have CALL gas change")
				assert.Equal(t, uint64(100000), callGasChange.OldValue, "Old gas value")
				assert.Equal(t, uint64(91000), callGasChange.NewValue, "New gas value after CALL")
			})
	})

	t.Run("create_opcode_gas_change", func(t *testing.T) {
		// CREATE opcode (0xf0) should record a gas change with REASON_CONTRACT_CREATION
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 200000, []byte{}).
			// Simulate CREATE opcode execution
			OpCode(0, 0xf0, 200000, 32000). // CREATE costs ~32000 gas
			StartCreateCall(BobAddr, CharlieAddr, bigInt(0), 168000, []byte{0x60, 0x80}).
			EndCall([]byte{0x60, 0x80}, 150000).
			EndCall([]byte{}, 180000).
			EndBlockTrx(successReceipt(200000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				rootCall := trx.Calls[0]

				// Should have gas change for CREATE
				var createGasChange *pbeth.GasChange
				for _, gc := range rootCall.GasChanges {
					if gc.Reason == pbeth.GasChange_REASON_CONTRACT_CREATION {
						createGasChange = gc
						break
					}
				}

				require.NotNil(t, createGasChange, "Should have CREATE gas change")
				assert.Equal(t, uint64(200000), createGasChange.OldValue)
				assert.Equal(t, uint64(168000), createGasChange.NewValue)
			})
	})

	t.Run("create2_opcode_gas_change", func(t *testing.T) {
		// CREATE2 opcode (0xf5) should record a gas change with REASON_CONTRACT_CREATION2
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 200000, []byte{}).
			// Simulate CREATE2 opcode execution
			OpCode(0, 0xf5, 200000, 32000). // CREATE2 costs similar to CREATE
			StartCreate2Call(BobAddr, CharlieAddr, bigInt(0), 168000, []byte{0x60, 0x80}).
			EndCall([]byte{0x60, 0x80}, 150000).
			EndCall([]byte{}, 180000).
			EndBlockTrx(successReceipt(200000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				rootCall := trx.Calls[0]

				var create2GasChange *pbeth.GasChange
				for _, gc := range rootCall.GasChanges {
					if gc.Reason == pbeth.GasChange_REASON_CONTRACT_CREATION2 {
						create2GasChange = gc
						break
					}
				}

				require.NotNil(t, create2GasChange, "Should have CREATE2 gas change")
				assert.Equal(t, uint64(200000), create2GasChange.OldValue)
				assert.Equal(t, uint64(168000), create2GasChange.NewValue)
			})
	})

	t.Run("staticcall_opcode_gas_change", func(t *testing.T) {
		// STATICCALL opcode (0xfa) should record a gas change with REASON_STATIC_CALL
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			OpCode(0, 0xfa, 100000, 700). // STATICCALL costs ~700 gas
			StartStaticCall(BobAddr, CharlieAddr, 99300, []byte{}).
			EndCall([]byte{}, 95000).
			EndCall([]byte{}, 99000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				rootCall := trx.Calls[0]

				var staticCallGasChange *pbeth.GasChange
				for _, gc := range rootCall.GasChanges {
					if gc.Reason == pbeth.GasChange_REASON_STATIC_CALL {
						staticCallGasChange = gc
						break
					}
				}

				require.NotNil(t, staticCallGasChange, "Should have STATICCALL gas change")
				assert.Equal(t, uint64(100000), staticCallGasChange.OldValue)
				assert.Equal(t, uint64(99300), staticCallGasChange.NewValue)
			})
	})

	t.Run("delegatecall_opcode_gas_change", func(t *testing.T) {
		// DELEGATECALL opcode (0xf4) should record a gas change with REASON_DELEGATE_CALL
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			OpCode(0, 0xf4, 100000, 700). // DELEGATECALL costs ~700 gas
			StartDelegateCall(BobAddr, CharlieAddr, bigInt(0), 99300, []byte{}).
			EndCall([]byte{}, 95000).
			EndCall([]byte{}, 99000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				rootCall := trx.Calls[0]

				var delegateCallGasChange *pbeth.GasChange
				for _, gc := range rootCall.GasChanges {
					if gc.Reason == pbeth.GasChange_REASON_DELEGATE_CALL {
						delegateCallGasChange = gc
						break
					}
				}

				require.NotNil(t, delegateCallGasChange, "Should have DELEGATECALL gas change")
				assert.Equal(t, uint64(100000), delegateCallGasChange.OldValue)
				assert.Equal(t, uint64(99300), delegateCallGasChange.NewValue)
			})
	})

	t.Run("callcode_opcode_gas_change", func(t *testing.T) {
		// CALLCODE opcode (0xf2) should record a gas change with REASON_CALL_CODE
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			OpCode(0, 0xf2, 100000, 700). // CALLCODE costs ~700 gas
			StartCallCode(BobAddr, CharlieAddr, bigInt(0), 99300, []byte{}).
			EndCall([]byte{}, 95000).
			EndCall([]byte{}, 99000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				rootCall := trx.Calls[0]

				var callCodeGasChange *pbeth.GasChange
				for _, gc := range rootCall.GasChanges {
					if gc.Reason == pbeth.GasChange_REASON_CALL_CODE {
						callCodeGasChange = gc
						break
					}
				}

				require.NotNil(t, callCodeGasChange, "Should have CALLCODE gas change")
				assert.Equal(t, uint64(100000), callCodeGasChange.OldValue)
				assert.Equal(t, uint64(99300), callCodeGasChange.NewValue)
			})
	})

	t.Run("log_opcodes_gas_change", func(t *testing.T) {
		// LOG0-LOG4 opcodes (0xa0-0xa4) should record gas changes with REASON_EVENT_LOG
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			// LOG0
			OpCode(0, 0xa0, 100000, 375).
			// LOG1
			OpCode(0, 0xa1, 99625, 750).
			// LOG2
			OpCode(0, 0xa2, 98875, 1125).
			EndCall([]byte{}, 97750).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				rootCall := trx.Calls[0]

				// Should have 3 log gas changes
				logGasChanges := []*pbeth.GasChange{}
				for _, gc := range rootCall.GasChanges {
					if gc.Reason == pbeth.GasChange_REASON_EVENT_LOG {
						logGasChanges = append(logGasChanges, gc)
					}
				}

				assert.Equal(t, 3, len(logGasChanges), "Should have 3 log gas changes")
			})
	})

	t.Run("return_opcode_gas_change", func(t *testing.T) {
		// RETURN opcode (0xf3) should record a gas change with REASON_RETURN
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			OpCode(0, 0xf3, 95000, 0). // RETURN costs 0 (but we can still track it with cost=0)
			EndCall([]byte{0x42}, 95000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				rootCall := trx.Calls[0]

				// RETURN with cost=0 won't create a gas change (cost > 0 check)
				returnGasChanges := []*pbeth.GasChange{}
				for _, gc := range rootCall.GasChanges {
					if gc.Reason == pbeth.GasChange_REASON_RETURN {
						returnGasChanges = append(returnGasChanges, gc)
					}
				}

				// No gas change since cost was 0
				assert.Equal(t, 0, len(returnGasChanges), "RETURN with cost=0 doesn't create gas change")
			})
	})

	t.Run("selfdestruct_opcode_gas_change", func(t *testing.T) {
		// SELFDESTRUCT opcode (0xff) should record a gas change with REASON_SELF_DESTRUCT
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			// SELFDESTRUCT
			OpCode(0, 0xff, 95000, 5000). // SELFDESTRUCT costs 5000 gas
			Suicide(BobAddr, AliceAddr, bigInt(100)).
			EndCall([]byte{}, 90000).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				rootCall := trx.Calls[0]

				// Find SELFDESTRUCT gas change
				var selfDestructGasChange *pbeth.GasChange
				for _, gc := range rootCall.GasChanges {
					if gc.Reason == pbeth.GasChange_REASON_SELF_DESTRUCT {
						selfDestructGasChange = gc
						break
					}
				}

				require.NotNil(t, selfDestructGasChange, "Should have SELFDESTRUCT gas change")
				assert.Equal(t, uint64(95000), selfDestructGasChange.OldValue)
				assert.Equal(t, uint64(90000), selfDestructGasChange.NewValue)
			})
	})

	t.Run("copy_opcodes_gas_changes", func(t *testing.T) {
		// CALLDATACOPY (0x37), CODECOPY (0x39), EXTCODECOPY (0x3c), RETURNDATACOPY (0x3e)
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{0x01, 0x02, 0x03}).
			// CALLDATACOPY
			OpCode(0, 0x37, 100000, 9). // Minimal cost
			// CODECOPY
			OpCode(0, 0x39, 99991, 9).
			// EXTCODECOPY
			OpCode(0, 0x3c, 99982, 700).
			// RETURNDATACOPY
			OpCode(0, 0x3e, 99282, 9).
			EndCall([]byte{}, 99273).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				rootCall := trx.Calls[0]

				// Should have all copy gas changes
				reasons := []pbeth.GasChange_Reason{
					pbeth.GasChange_REASON_CALL_DATA_COPY,
					pbeth.GasChange_REASON_CODE_COPY,
					pbeth.GasChange_REASON_EXT_CODE_COPY,
					pbeth.GasChange_REASON_RETURN_DATA_COPY,
				}

				for _, reason := range reasons {
					found := false
					for _, gc := range rootCall.GasChanges {
						if gc.Reason == reason {
							found = true
							break
						}
					}
					assert.True(t, found, "Should have gas change for reason %s", reason)
				}
			})
	})

	t.Run("no_gas_change_for_unmapped_opcodes", func(t *testing.T) {
		// Opcodes not in the map (like PUSH, ADD, etc.) should not create gas changes
		NewTracerTester(t).
			StartBlockTrx(TestLegacyTrx).
			StartCall(AliceAddr, BobAddr, bigInt(100), 100000, []byte{}).
			// PUSH1 (0x60) - not in map
			OpCode(0, 0x60, 100000, 3).
			// ADD (0x01) - not in map
			OpCode(0, 0x01, 99997, 3).
			// MUL (0x02) - not in map
			OpCode(0, 0x02, 99994, 5).
			EndCall([]byte{}, 99989).
			EndBlockTrx(successReceipt(100000), nil, nil).
			Validate(func(block *pbeth.Block) {
				trx := block.TransactionTraces[0]
				rootCall := trx.Calls[0]

				// Should have no gas changes since none of these opcodes are in the map
				assert.Equal(t, 0, len(rootCall.GasChanges), "Unmapped opcodes should not create gas changes")
			})
	})
}
