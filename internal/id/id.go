// Package id generates short, globally unique identifiers.
// Each ID is a 22-character base62 string derived from a UUID v4 (128 bits).
package id

import (
	"math/big"

	"github.com/google/uuid"
)

const (
	alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	base     = 62
	idLen    = 22 // ceil(128 * log2 / log62) = 22
)

var big62 = big.NewInt(base)

// New generates a globally unique 22-character base62 ID.
// Internally creates a UUID v4, interprets its 128 bits as a big.Int,
// and encodes in base62.
func New() string {
	u := uuid.New()
	n := new(big.Int).SetBytes(u[:])
	return encode(n)
}

// encode converts a non-negative big.Int to a base62 string,
// zero-padded to idLen characters.
func encode(n *big.Int) string {
	if n.Sign() == 0 {
		buf := make([]byte, idLen)
		for i := range buf {
			buf[i] = alphabet[0]
		}
		return string(buf)
	}

	// Allocate result buffer and fill from the right.
	buf := make([]byte, idLen)
	for i := range buf {
		buf[i] = alphabet[0] // pre-fill with '0' for padding
	}

	mod := new(big.Int)
	pos := idLen - 1
	for n.Sign() > 0 {
		n.DivMod(n, big62, mod)
		buf[pos] = alphabet[mod.Int64()]
		pos--
	}
	return string(buf)
}
