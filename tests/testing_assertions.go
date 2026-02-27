package tests

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

// assertEqualBytes compares two byte slices by converting them to hex strings
// Expected can be either a string (hex) or []byte
func assertEqualBytes(t *testing.T, expected interface{}, actual []byte, msgAndArgs ...interface{}) bool {
	t.Helper()

	var expectedHex string
	switch v := expected.(type) {
	case string:
		// Remove 0x prefix if present
		if len(v) >= 2 && v[0] == '0' && v[1] == 'x' {
			expectedHex = v[2:]
		} else {
			expectedHex = v
		}
	case []byte:
		expectedHex = hex.EncodeToString(v)
	default:
		t.Fatalf("Expected must be string (hex) or []byte, got %T", expected)
		return false
	}

	actualHex := hex.EncodeToString(actual)
	return assert.Equal(t, expectedHex, actualHex, msgAndArgs...)
}
