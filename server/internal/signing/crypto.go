package signing

import (
	"crypto/rand"
	"io"
)

// cryptoRandRead is a wrapper around crypto/rand.Read for easier testing
var cryptoRandRead = func(b []byte) (int, error) {
	return io.ReadFull(rand.Reader, b)
}
