package firehose

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestOrdinal_Initial tests initial ordinal value
func TestOrdinal_Initial(t *testing.T) {
	ord := &Ordinal{}

	assert.Equal(t, uint64(0), ord.Peek(), "initial ordinal should be 0")
}

// TestOrdinal_Next tests sequential ordinal generation
func TestOrdinal_Next(t *testing.T) {
	ord := &Ordinal{}

	// Note: Next() increments THEN returns, so first call returns 1
	assert.Equal(t, uint64(1), ord.Next(), "first next should return 1")
	assert.Equal(t, uint64(2), ord.Next(), "second next should return 2")
	assert.Equal(t, uint64(3), ord.Next(), "third next should return 3")
	assert.Equal(t, uint64(3), ord.Peek(), "peek after 3 Next() calls should be 3")
}

// TestOrdinal_Peek tests peek without increment
func TestOrdinal_Peek(t *testing.T) {
	ord := &Ordinal{}

	ord.Next() // 1
	ord.Next() // 2

	// Peek should not increment
	assert.Equal(t, uint64(2), ord.Peek())
	assert.Equal(t, uint64(2), ord.Peek())
	assert.Equal(t, uint64(2), ord.Peek())

	// Next should still return next value
	assert.Equal(t, uint64(3), ord.Next())
	assert.Equal(t, uint64(3), ord.Peek())
}

// TestOrdinal_Set tests setting ordinal value
func TestOrdinal_Set(t *testing.T) {
	ord := &Ordinal{}

	ord.Next() // 1
	ord.Next() // 2

	// Set to specific value
	ord.Set(100)

	assert.Equal(t, uint64(100), ord.Peek())
	assert.Equal(t, uint64(101), ord.Next())
	assert.Equal(t, uint64(102), ord.Next())
}

// TestOrdinal_Reset tests reset functionality
func TestOrdinal_Reset(t *testing.T) {
	ord := &Ordinal{}

	ord.Next() // 1
	ord.Next() // 2
	ord.Next() // 3

	// Reset
	ord.Reset()

	assert.Equal(t, uint64(0), ord.Peek())
	assert.Equal(t, uint64(1), ord.Next())
	assert.Equal(t, uint64(2), ord.Next())
}

// TestOrdinal_Sequential tests many sequential calls
func TestOrdinal_Sequential(t *testing.T) {
	ord := &Ordinal{}

	// Generate 1000 ordinals (starting from 1)
	for i := uint64(1); i <= 1000; i++ {
		assert.Equal(t, i, ord.Next(), "ordinal %d should be sequential", i)
	}

	assert.Equal(t, uint64(1000), ord.Peek())
}

// TestOrdinal_Uniqueness tests that ordinals are unique
func TestOrdinal_Uniqueness(t *testing.T) {
	ord := &Ordinal{}

	seen := make(map[uint64]bool)
	for i := 0; i < 1000; i++ {
		val := ord.Next()
		assert.False(t, seen[val], "ordinal %d should be unique", val)
		seen[val] = true
	}

	assert.Equal(t, 1000, len(seen), "should have 1000 unique ordinals")
}
