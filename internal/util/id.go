package util

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
)

// A public ID is the only handle a server entity is allowed to show the
// outside world. It is 96 bits of crypto-random data rendered as 20 lowercase
// base32 characters, and that text form is also what the database stores:
//
//	gsyt7at6cjfr33d73mta
//
// The alphabet is deliberately narrower than base64url. A public ID reaches
// Kubernetes Job names, object-storage paths on case-insensitive filesystems,
// URLs, and people retyping it, and base32 is the widest encoding that survives
// all four unchanged. Storing the text rather than the raw bytes costs 8 bytes
// per value and makes every direct database query readable; the raw-byte form
// died with that trade. See docs/design/entity-identity.md §4.2.
const (
	// publicIDBytes is the entropy width. 12 bytes is 96 bits, which base32
	// renders in 20 characters with 4 bits of slack in the last one.
	publicIDBytes = 12
	// PublicIDLen is the canonical text length.
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
	b := make([]byte, publicIDBytes)
	if _, err := randRead(b); err != nil {
		return "", fmt.Errorf("generate public id: %w", err)
	}
	return strings.ToLower(publicIDEncoding.EncodeToString(b)), nil
}

// CanonicalPublicID reports whether s is a public ID and returns its one
// canonical text form.
//
// Input is accepted in either case, so an ID retyped from a title-cased
// document still resolves. The text is decoded and re-encoded to prove it was
// canonical apart from case: 20 base32 characters carry 100 bits, and the 4
// bits past the value must be zero. Without that check, several texts would
// name one row.
func CanonicalPublicID(s string) (string, bool) {
	if len(s) != PublicIDLen {
		return "", false
	}
	b, err := publicIDEncoding.DecodeString(strings.ToUpper(s))
	if err != nil || len(b) != publicIDBytes {
		return "", false
	}
	lower := strings.ToLower(s)
	if strings.ToLower(publicIDEncoding.EncodeToString(b)) != lower {
		return "", false
	}
	return lower, true
}
