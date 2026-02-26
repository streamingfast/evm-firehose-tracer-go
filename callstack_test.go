package firehose

import (
	"testing"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCallStack_EmptyStack tests operations on an empty call stack
func TestCallStack_EmptyStack(t *testing.T) {
	cs := NewCallStack()

	assert.Equal(t, 0, cs.Depth(), "empty stack should have depth 0")
	assert.False(t, cs.HasActiveCall(), "empty stack should not have active call")
	assert.Nil(t, cs.Peek(), "peek on empty stack should return nil")
	assert.Nil(t, cs.Pop(), "pop on empty stack should return nil")
	assert.Nil(t, cs.Root(), "root on empty stack should return nil")
	assert.Nil(t, cs.Parent(), "parent on empty stack should return nil")
	assert.Equal(t, uint32(0), cs.ParentIndex(), "parent index on empty stack should be 0")
}

// TestCallStack_SingleCall tests push/pop with a single call
func TestCallStack_SingleCall(t *testing.T) {
	cs := NewCallStack()

	call := &pbeth.Call{
		CallType: pbeth.CallType_CALL,
	}

	// Push - Index, Depth, and ParentIndex are set by Push()
	cs.Push(call)

	assert.Equal(t, uint32(1), call.Index, "first call should have index 1")
	assert.Equal(t, uint32(0), call.Depth, "first call should have depth 0")
	assert.Equal(t, uint32(0), call.ParentIndex, "first call should have parent index 0")
	assert.Equal(t, 1, cs.Depth(), "depth after push should be 1")
	assert.True(t, cs.HasActiveCall(), "should have active call")
	assert.Equal(t, call, cs.Peek(), "peek should return the call")
	assert.Equal(t, call, cs.Root(), "root should return the call")
	assert.Nil(t, cs.Parent(), "parent should be nil for single call")
	assert.Equal(t, uint32(0), cs.ParentIndex(), "parent index should be 0 for root call")

	// Pop
	popped := cs.Pop()

	assert.Equal(t, call, popped, "popped call should match pushed call")
	assert.Equal(t, 0, cs.Depth(), "depth after pop should be 0")
	assert.False(t, cs.HasActiveCall(), "should not have active call after pop")
}

// TestCallStack_NestedCalls tests nested call operations
func TestCallStack_NestedCalls(t *testing.T) {
	cs := NewCallStack()

	// Create nested calls - Index, Depth, ParentIndex set by Push()
	call0 := &pbeth.Call{CallType: pbeth.CallType_CALL}
	call1 := &pbeth.Call{CallType: pbeth.CallType_CALL}
	call2 := &pbeth.Call{CallType: pbeth.CallType_DELEGATE}

	// Push depth 1
	cs.Push(call0)
	assert.Equal(t, uint32(1), call0.Index)
	assert.Equal(t, uint32(0), call0.Depth)
	assert.Equal(t, uint32(0), call0.ParentIndex)
	assert.Equal(t, 1, cs.Depth())
	assert.Equal(t, call0, cs.Peek())
	assert.Equal(t, call0, cs.Root())
	assert.Nil(t, cs.Parent())

	// Push depth 2
	cs.Push(call1)
	assert.Equal(t, uint32(2), call1.Index)
	assert.Equal(t, uint32(1), call1.Depth)
	assert.Equal(t, uint32(1), call1.ParentIndex)
	assert.Equal(t, 2, cs.Depth())
	assert.Equal(t, call1, cs.Peek())
	assert.Equal(t, call0, cs.Root())
	assert.Equal(t, call0, cs.Parent())
	assert.Equal(t, uint32(1), cs.ParentIndex())

	// Push depth 3
	cs.Push(call2)
	assert.Equal(t, uint32(3), call2.Index)
	assert.Equal(t, uint32(2), call2.Depth)
	assert.Equal(t, uint32(2), call2.ParentIndex)
	assert.Equal(t, 3, cs.Depth())
	assert.Equal(t, call2, cs.Peek())
	assert.Equal(t, call0, cs.Root())
	assert.Equal(t, call1, cs.Parent())
	assert.Equal(t, uint32(2), cs.ParentIndex())

	// Pop depth 3 -> 2
	popped := cs.Pop()
	assert.Equal(t, call2, popped)
	assert.Equal(t, 2, cs.Depth())
	assert.Equal(t, call1, cs.Peek())

	// Pop depth 2 -> 1
	popped = cs.Pop()
	assert.Equal(t, call1, popped)
	assert.Equal(t, 1, cs.Depth())
	assert.Equal(t, call0, cs.Peek())

	// Pop depth 1 -> 0
	popped = cs.Pop()
	assert.Equal(t, call0, popped)
	assert.Equal(t, 0, cs.Depth())
	assert.Nil(t, cs.Peek())
}

// TestCallStack_DeepNesting tests a deep call stack
func TestCallStack_DeepNesting(t *testing.T) {
	cs := NewCallStack()
	depth := 100

	// Push many calls
	calls := make([]*pbeth.Call, depth)
	for i := 0; i < depth; i++ {
		calls[i] = &pbeth.Call{CallType: pbeth.CallType_CALL}
		cs.Push(calls[i])
		// Verify Push() set Index correctly (starts at 1)
		assert.Equal(t, uint32(i+1), calls[i].Index)
	}

	assert.Equal(t, depth, cs.Depth())
	assert.Equal(t, calls[depth-1], cs.Peek())
	assert.Equal(t, calls[0], cs.Root())
	assert.Equal(t, calls[depth-2], cs.Parent())
	assert.Equal(t, uint32(depth-1), cs.ParentIndex()) // Last call's index

	// Pop all calls
	for i := depth - 1; i >= 0; i-- {
		popped := cs.Pop()
		assert.Equal(t, calls[i], popped)
		assert.Equal(t, i, cs.Depth())
	}

	assert.Equal(t, 0, cs.Depth())
}

// TestCallStack_Reset tests reset functionality
func TestCallStack_Reset(t *testing.T) {
	cs := NewCallStack()

	// Push some calls
	cs.Push(&pbeth.Call{CallType: pbeth.CallType_CALL})
	cs.Push(&pbeth.Call{CallType: pbeth.CallType_CALL})
	cs.Push(&pbeth.Call{CallType: pbeth.CallType_CALL})

	require.Equal(t, 3, cs.Depth())

	// Reset
	cs.Reset()

	assert.Equal(t, 0, cs.Depth())
	assert.False(t, cs.HasActiveCall())
	assert.Nil(t, cs.Peek())
	assert.Nil(t, cs.Root())
	assert.Nil(t, cs.Parent())

	// After reset, next push should start at index 1 again
	call := &pbeth.Call{CallType: pbeth.CallType_CALL}
	cs.Push(call)
	assert.Equal(t, uint32(1), call.Index)
}

// TestCallStack_ParentIndex tests ParentIndex method
func TestCallStack_ParentIndex(t *testing.T) {
	cs := NewCallStack()

	// Empty stack
	assert.Equal(t, uint32(0), cs.ParentIndex())

	// Root call (no parent)
	call1 := &pbeth.Call{CallType: pbeth.CallType_CALL}
	cs.Push(call1)
	assert.Equal(t, uint32(0), cs.ParentIndex())

	// Child call (has parent)
	call2 := &pbeth.Call{CallType: pbeth.CallType_CALL}
	cs.Push(call2)
	assert.Equal(t, uint32(1), cs.ParentIndex()) // Parent is call1 with index 1

	// Grandchild call (has parent)
	call3 := &pbeth.Call{CallType: pbeth.CallType_CALL}
	cs.Push(call3)
	assert.Equal(t, uint32(2), cs.ParentIndex()) // Parent is call2 with index 2
}
