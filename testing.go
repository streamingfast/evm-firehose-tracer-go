package firehose

import (
	"bytes"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
)

// Testing support - functions to help test packages access internal state

// SetTestingNativeValidator sets the native validator for testing purposes
func (t *Tracer) SetTestingNativeValidator(nv interface{}) {
	if v, ok := nv.(*nativeValidator); ok {
		t.nativeValidator = v
	}
}

// GetTestingNativeValidator returns the native validator for testing purposes
func (t *Tracer) GetTestingNativeValidator() interface{} {
	return t.nativeValidator
}

// NewTestingNativeValidator creates a new native validator for testing
func NewTestingNativeValidator(outputDir string) (interface{}, error) {
	return newNativeValidator(outputDir)
}

// GetTestingStateDB returns the mockStateDB from a native validator for testing
func GetTestingStateDB(nv interface{}) interface{} {
	if v, ok := nv.(*nativeValidator); ok {
		return v.stateDB
	}
	return nil
}

// NewTestingMockStateReader creates a mock state reader for testing
func NewTestingMockStateReader(stateDB interface{}) StateReader {
	// The interface{} is actually a *mockStateDB from nativeValidator.stateDB
	// We create a mockStateReader wrapper around it
	return newMockStateReaderFromDB(stateDB)
}

// Helper to create mockStateReader without exposing the type
func newMockStateReaderFromDB(db interface{}) StateReader {
	// This will be a *mockStateDB, we just wrap it
	return &mockStateReader{mockStateDB: db.(*mockStateDB)}
}

// SetTestingMockStateCode sets the code for an address in the mock StateDB
func SetTestingMockStateCode(nv interface{}, addr common.Address, code []byte) {
	if v, ok := nv.(*nativeValidator); ok {
		v.stateDB.SetCode(addr, code)
	}
}

// SetTestingMockStateNonce sets the nonce for an address in the mock StateDB
func SetTestingMockStateNonce(nv interface{}, addr common.Address, nonce uint64) {
	if v, ok := nv.(*nativeValidator); ok {
		v.stateDB.SetNonce(addr, nonce)
	}
}

// SetTestingMockStateExist sets whether an address exists in the mock StateDB
func SetTestingMockStateExist(nv interface{}, addr common.Address, exists bool) {
	if v, ok := nv.(*nativeValidator); ok {
		v.stateDB.SetExist(addr, exists)
	}
}

// CallTestingNativeValidatorOnOpcode calls OnOpcode on the native validator
func CallTestingNativeValidatorOnOpcode(nv interface{}, pc uint64, op byte, gas, cost uint64, depth int) {
	if v, ok := nv.(*nativeValidator); ok {
		v.OnOpcode(pc, op, gas, cost, depth)
	}
}

// CallTestingNativeValidatorOnKeccakPreimage calls OnKeccakPreimage on the native validator
func CallTestingNativeValidatorOnKeccakPreimage(nv interface{}, hash [32]byte, preimage []byte) {
	if v, ok := nv.(*nativeValidator); ok {
		v.OnKeccakPreimage(hash, preimage)
	}
}

// CallTestingNativeValidatorOnCallExit calls OnCallExit on the native validator
func CallTestingNativeValidatorOnCallExit(nv interface{}, depth int, output []byte, gasUsed uint64, err error, reverted bool) {
	if v, ok := nv.(*nativeValidator); ok {
		v.OnCallExit(depth, output, gasUsed, err, reverted)
	}
}

// GetTestingNativeValidatorBuffer returns the native validator's internal testing buffer
func GetTestingNativeValidatorBuffer(nv interface{}) *bytes.Buffer {
	if v, ok := nv.(*nativeValidator); ok {
		return v.tracer.InternalTestingBuffer()
	}
	return nil
}

// GetTestingLatestCallEnterSuicided returns whether the latest call enter was suicided
func (t *Tracer) GetTestingLatestCallEnterSuicided() bool {
	return t.latestCallEnterSuicided
}

// SetTestingLatestCallEnterSuicided sets whether the latest call enter was suicided
func (t *Tracer) SetTestingLatestCallEnterSuicided(value bool) {
	t.latestCallEnterSuicided = value
}

// GetTestingCallStackDepth returns the depth of the call stack
func (t *Tracer) GetTestingCallStackDepth() int {
	return t.callStack.Depth()
}

// GetTestingCallStackPeek returns the top call from the call stack
// Returns interface{} to avoid exposing the unexported Call type
func (t *Tracer) GetTestingCallStackPeek() interface{} {
	return t.callStack.Peek()
}

// SetTestingCallSuicide sets the Suicide field on a call
func SetTestingCallSuicide(call interface{}, suicide bool) {
	if c, ok := call.(*pbeth.Call); ok {
		c.Suicide = suicide
	}
}

// SetTestingCallExecutedCode sets the ExecutedCode field on a call
func SetTestingCallExecutedCode(call interface{}, executedCode bool) {
	if c, ok := call.(*pbeth.Call); ok {
		c.ExecutedCode = executedCode
	}
}

// GetTestingOutputWriter returns the output writer from the tracer's config
func (t *Tracer) GetTestingOutputWriter() *bytes.Buffer {
	if buf, ok := t.config.OutputWriter.(*bytes.Buffer); ok {
		return buf
	}
	return nil
}

// GetTestingTransaction returns the current transaction being traced
func (t *Tracer) GetTestingTransaction() *pbeth.TransactionTrace {
	return t.transaction
}

// ConvertToNativeLog converts log data to native go-ethereum types.Log
func ConvertToNativeLog(addr [20]byte, topics [][32]byte, data []byte, blockIndex uint32) *types.Log {
	return convertToNativeLog(addr, topics, data, blockIndex)
}
