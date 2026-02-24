package firehose

import (
	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
)

// CallStack manages the hierarchy of EVM calls during transaction execution
// It tracks the depth-based call tree and maintains parent-child relationships
type CallStack struct {
	stack []*pbeth.Call
}

// NewCallStack creates a new empty call stack
func NewCallStack() *CallStack {
	return &CallStack{
		stack: make([]*pbeth.Call, 0, 32), // Pre-allocate for typical call depth
	}
}

// Push adds a new call to the stack
func (cs *CallStack) Push(call *pbeth.Call) {
	cs.stack = append(cs.stack, call)
}

// Pop removes and returns the top call from the stack
// Returns nil if the stack is empty
func (cs *CallStack) Pop() *pbeth.Call {
	if len(cs.stack) == 0 {
		return nil
	}

	call := cs.stack[len(cs.stack)-1]
	cs.stack = cs.stack[:len(cs.stack)-1]
	return call
}

// Peek returns the top call without removing it
// Returns nil if the stack is empty
func (cs *CallStack) Peek() *pbeth.Call {
	if len(cs.stack) == 0 {
		return nil
	}
	return cs.stack[len(cs.stack)-1]
}

// HasActiveCall returns true if there's at least one call on the stack
func (cs *CallStack) HasActiveCall() bool {
	return len(cs.stack) > 0
}

// Depth returns the current call depth (0 = no calls, 1 = root call, etc.)
func (cs *CallStack) Depth() int {
	return len(cs.stack)
}

// Reset clears the call stack
func (cs *CallStack) Reset() {
	cs.stack = cs.stack[:0]
}

// Root returns the root call (bottom of stack) or nil if empty
func (cs *CallStack) Root() *pbeth.Call {
	if len(cs.stack) == 0 {
		return nil
	}
	return cs.stack[0]
}

// Parent returns the parent of the current call (second from top)
// Returns nil if there's no parent
func (cs *CallStack) Parent() *pbeth.Call {
	if len(cs.stack) < 2 {
		return nil
	}
	return cs.stack[len(cs.stack)-2]
}

// ParentIndex returns the index of the parent call
// Returns 0 if there's no parent (for root call)
func (cs *CallStack) ParentIndex() uint32 {
	parent := cs.Parent()
	if parent == nil {
		return 0
	}
	return parent.Index
}
