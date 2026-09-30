package firehose

import (
	"encoding/base64"
	"encoding/binary"
	"slices"
)

const base64StdAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

// base64StdPairs maps every 12-bit input value to its two standard base64 characters,
// packed so that a little-endian store writes them in output order.
var base64StdPairs = func() (pairs [4096]uint16) {
	for i := range pairs {
		pairs[i] = uint16(base64StdAlphabet[i>>6]) | uint16(base64StdAlphabet[i&0x3f])<<8
	}
	return
}()

// appendBase64 appends the standard padded base64 encoding of src to dst. It produces the
// exact same output as base64.StdEncoding.AppendEncode, but encodes 6 input bytes per
// iteration through a 12-bit lookup table, which is about twice as fast.
func appendBase64(dst, src []byte) []byte {
	start := len(dst)
	encodedLen := base64.StdEncoding.EncodedLen(len(src))
	dst = slices.Grow(dst, encodedLen)[:start+encodedLen]
	out := dst[start:]

	si, di := 0, 0
	// Each iteration reads 8 bytes but only consumes the first 6, so stop while 8 remain.
	for ; len(src)-si >= 8; si, di = si+6, di+8 {
		v := binary.BigEndian.Uint64(src[si:])
		binary.LittleEndian.PutUint64(out[di:],
			uint64(base64StdPairs[v>>52&0xfff])|
				uint64(base64StdPairs[v>>40&0xfff])<<16|
				uint64(base64StdPairs[v>>28&0xfff])<<32|
				uint64(base64StdPairs[v>>16&0xfff])<<48)
	}

	// si is a multiple of 3, so the tail encodes independently of what came before.
	base64.StdEncoding.Encode(out[di:], src[si:])
	return dst
}
