package firehose

import "sync/atomic"

// Ordinal is a monotonically increasing counter for assigning sequential IDs
// to events within a block. This ensures all events have a deterministic ordering.
type Ordinal struct {
	value atomic.Uint64
}

// Next returns the next ordinal value and increments the counter
func (o *Ordinal) Next() uint64 {
	return o.value.Add(1)
}

// Peek returns the current ordinal value without incrementing
func (o *Ordinal) Peek() uint64 {
	return o.value.Load()
}

// Set sets the ordinal to a specific value
func (o *Ordinal) Set(value uint64) {
	o.value.Store(value)
}

// Reset resets the ordinal back to 0
func (o *Ordinal) Reset() {
	o.value.Store(0)
}
