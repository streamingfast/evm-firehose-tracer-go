package firehose

import (
	"bytes"

	"github.com/ethereum/go-ethereum/core/types"
	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
)

// Testing support - functions to help test packages access internal state

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
