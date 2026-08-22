package util

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
)

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
