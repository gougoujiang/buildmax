package util

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"math/big"
	"strings"

	"github.com/google/uuid"
)

// PrefixAuthSession names one login chain of refresh tokens. It is the only
// prefixed entity-shaped identifier left: the rest became public IDs, and the
// two others still generated here -- "rt" for a trace file and "p" for a
// Desktop project -- name files and local UI state rather than rows.
const PrefixAuthSession = "as"

const (
	idBodyLen       = 20
	idBase36        = 36
	idAlphabetLower = "0123456789abcdefghijklmnopqrstuvwxyz"
)

var (
	idBig36   = big.NewInt(idBase36)
	idMaxBody = new(big.Int).Exp(big.NewInt(idBase36), big.NewInt(idBodyLen), nil)
)

// NewPrefixedID returns a string of the form "<prefix>_<body>", where body is 20
// characters from [a-z0-9] (lowercase base36), derived from 128 bits of
// crypto-random entropy. Ordering is by created_at; IDs are not time-ordered.
//
// Server entities do not use this. They use NewPublicID, whose value is what
// crosses every boundary; see docs/design/entity-identity.md. What is left here
// identifies a login chain, a trace file, and a Desktop project.
func NewPrefixedID(prefix string) string {
	u := uuid.New()
	n := new(big.Int).SetBytes(u[:])
	n.Mod(n, idMaxBody)
	return prefix + "_" + encodeBase36Lower(n, idBodyLen)
}

// encodeBase36Lower encodes a non-negative big.Int to a base36 string (0-9, a-z),
// zero-padded to length characters.
func encodeBase36Lower(n *big.Int, length int) string {
	if n.Sign() < 0 {
		n = new(big.Int).Set(n)
		n.Neg(n)
	}
	buf := make([]byte, length)
	for i := range buf {
		buf[i] = idAlphabetLower[0]
	}
	mod := new(big.Int)
	pos := length - 1
	for n.Sign() > 0 && pos >= 0 {
		n.DivMod(n, idBig36, mod)
		buf[pos] = idAlphabetLower[mod.Int64()]
		pos--
	}
	return string(buf)
}

// A public ID is the only handle a server entity is allowed to show the
// outside world. It is 96 bits of crypto-random data, stored as BINARY(12) and
// rendered as 20 lowercase base32 characters:
//
//	k7m2q4xz9rvt3bc8ndfp
//
// The alphabet is deliberately narrower than base64url. A public ID reaches
// Kubernetes Job names, object-storage paths on case-insensitive filesystems,
// URLs, and people retyping it, and base32 is the widest encoding that survives
// all four unchanged. See docs/design/entity-identity.md §4.2.
const (
	// PublicIDBytes is the stored width, matching BINARY(12).
	PublicIDBytes = 12
	// PublicIDLen is the canonical text length. 12 bytes is 96 bits, which
	// base32 renders in 20 characters with 4 bits of slack in the last one.
	PublicIDLen = 20
)

// publicIDEncoding is unpadded base32. Callers see it lowercased; base32
// itself is defined over the uppercase alphabet.
var publicIDEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// randRead is a seam so a test can prove that entropy failure becomes an error
// rather than a panic or a predictable ID.
var randRead = rand.Read

// NewPublicID returns a fresh public ID in canonical text form.
//
// It returns an error rather than panicking on entropy failure: that must
// surface as one failed create, not as a process abort inside a request.
func NewPublicID() (string, error) {
	b := make([]byte, PublicIDBytes)
	if _, err := randRead(b); err != nil {
		return "", fmt.Errorf("generate public id: %w", err)
	}
	return FormatPublicID(b), nil
}

// FormatPublicID renders 12 bytes as canonical text.
//
// A slice of any other length is a programming error, not input: it returns
// the empty string, which the store's read path treats as a corrupt row rather
// than passing a short handle outward.
func FormatPublicID(b []byte) string {
	if len(b) != PublicIDBytes {
		return ""
	}
	return strings.ToLower(publicIDEncoding.EncodeToString(b))
}

// ParsePublicID decodes canonical text to its 12 bytes, reporting whether the
// input was a public ID at all.
//
// Input is accepted in either case, so an ID retyped from a title-cased
// document still resolves, and the decoded bytes are re-encoded to prove the
// text was canonical. That last check is what makes the mapping one-to-one:
// 20 base32 characters carry 100 bits, and the 4 bits past the value must be
// zero. Without it, several texts would decode to one row.
func ParsePublicID(s string) ([]byte, bool) {
	if len(s) != PublicIDLen {
		return nil, false
	}
	lower := strings.ToLower(s)
	b, err := publicIDEncoding.DecodeString(strings.ToUpper(s))
	if err != nil || len(b) != PublicIDBytes {
		return nil, false
	}
	if FormatPublicID(b) != lower {
		return nil, false
	}
	return b, true
}
