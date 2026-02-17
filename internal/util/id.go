package util

import (
	"math/big"

	"github.com/google/uuid"
)

const (
	idAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	idBase     = 62
	idLen      = 22 // ceil(128 * log2 / log62) = 22
)

var idBig62 = big.NewInt(idBase)

// NewID generates a globally unique 22-character base62 ID.
// Internally creates a UUID v4, interprets its 128 bits as a big.Int,
// and encodes in base62.
func NewID() string {
	u := uuid.New()
	n := new(big.Int).SetBytes(u[:])
	return encodeBase62(n)
}

// encodeBase62 converts a non-negative big.Int to a base62 string,
// zero-padded to idLen characters.
func encodeBase62(n *big.Int) string {
	if n.Sign() == 0 {
		buf := make([]byte, idLen)
		for i := range buf {
			buf[i] = idAlphabet[0]
		}
		return string(buf)
	}

	buf := make([]byte, idLen)
	for i := range buf {
		buf[i] = idAlphabet[0]
	}

	mod := new(big.Int)
	pos := idLen - 1
	for n.Sign() > 0 {
		n.DivMod(n, idBig62, mod)
		buf[pos] = idAlphabet[mod.Int64()]
		pos--
	}
	return string(buf)
}
