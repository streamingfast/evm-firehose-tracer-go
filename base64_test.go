package firehose

import (
	"encoding/base64"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppendBase64MatchesStdlib(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))

	for length := 0; length < 300; length++ {
		src := make([]byte, length)
		for i := range src {
			src[i] = byte(rng.UintN(256))
		}

		prefix := []byte("FIRE BLOCK ")
		expected := base64.StdEncoding.AppendEncode(append([]byte(nil), prefix...), src)

		require.Equal(t, string(expected), string(appendBase64(append([]byte(nil), prefix...), src)), "length %d", length)
		require.Equal(t, string(expected), string(appendBase64(append(make([]byte, 0, 1024), prefix...), src)), "length %d with spare capacity", length)
	}
}

func FuzzAppendBase64(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0xff, 0x00, 0x7f, 0x80, 0x01, 0xfe, 0x10, 0xef, 0x42})
	f.Fuzz(func(t *testing.T, src []byte) {
		require.Equal(t, base64.StdEncoding.EncodeToString(src), string(appendBase64(nil, src)))
	})
}
