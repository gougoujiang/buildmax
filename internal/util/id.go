package util

import (
	"math/big"

	"github.com/google/uuid"
)

const (
	idAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	idBase     = 36
	idLen      = 25 // ceil(128 * log2 / log36) = 25
)

var idBig36 = big.NewInt(idBase)

// NewID generates a globally unique 25-character base36 ID (lowercase + digits).
// Internally creates a UUID v4, interprets its 128 bits as a big.Int,
// and encodes in base36.
func NewID() string {
	u := uuid.New()
	n := new(big.Int).SetBytes(u[:])
	return encodeBase36(n)
}

// encodeBase36 converts a non-negative big.Int to a base36 string,
// zero-padded to idLen characters.
func encodeBase36(n *big.Int) string {
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
		n.DivMod(n, idBig36, mod)
		buf[pos] = idAlphabet[mod.Int64()]
		pos--
	}
	return string(buf)
}
