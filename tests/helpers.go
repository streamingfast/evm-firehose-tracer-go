package tests

// hashFromHex converts a hex string to a 32-byte hash
func hashFromHex(s string) [32]byte {
	// Remove 0x prefix if present
	if len(s) >= 2 && s[0] == '0' && s[1] == 'x' {
		s = s[2:]
	}

	var hash [32]byte
	// Pad with zeros if too short
	for len(s) < 64 {
		s = "0" + s
	}

	for i := 0; i < 32; i++ {
		hash[i] = hexToByte(s[i*2], s[i*2+1])
	}

	return hash
}

// addressFromHex converts a hex string to a 20-byte address
func addressFromHex(s string) [20]byte {
	// Remove 0x prefix if present
	if len(s) >= 2 && s[0] == '0' && s[1] == 'x' {
		s = s[2:]
	}

	var addr [20]byte
	// Pad with zeros if too short
	for len(s) < 40 {
		s = "0" + s
	}

	for i := 0; i < 20; i++ {
		addr[i] = hexToByte(s[i*2], s[i*2+1])
	}

	return addr
}

// hexToByte converts two hex characters to a byte
func hexToByte(c1, c2 byte) byte {
	return hexCharToByte(c1)<<4 | hexCharToByte(c2)
}

// hexCharToByte converts a single hex character to a byte
func hexCharToByte(c byte) byte {
	if c >= '0' && c <= '9' {
		return c - '0'
	}
	if c >= 'a' && c <= 'f' {
		return c - 'a' + 10
	}
	if c >= 'A' && c <= 'F' {
		return c - 'A' + 10
	}
	return 0
}
