package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// PasswordMinLength is the shortest password BuildMax accepts.
//
// It is longer than the usual eight because BuildMax has no login throttling
// yet: an attacker who can reach the server can guess as fast as the server
// will hash. Length is the only defense that does not need infrastructure, so
// it carries more weight here than it would elsewhere. See
// docs/deploy/authentication.md.
const PasswordMinLength = 12

// PasswordMaxLength bounds what will be hashed. Argon2 has no length limit of
// its own, but accepting unbounded input means accepting unbounded work.
const PasswordMaxLength = 1024

// ErrPasswordTooShort and ErrPasswordTooLong report an unusable password. They
// are separate from a failed login: these mean "choose another", not "wrong".
var (
	ErrPasswordTooShort = fmt.Errorf("password must be at least %d characters", PasswordMinLength)
	ErrPasswordTooLong  = fmt.Errorf("password must be at most %d characters", PasswordMaxLength)
)

// Argon2id parameters. They are encoded into every stored hash, so raising
// these later re-hashes new passwords without invalidating old ones.
const (
	argonTime    = 1
	argonMemory  = 64 * 1024 // 64 MiB
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

// HashPassword returns a PHC-encoded argon2id hash of plaintext.
//
// Argon2id rather than a plain SHA family: this is the one value in BuildMax
// that a person chose and may have reused elsewhere, so a leaked database must
// not turn into a list of working passwords for other services. Memory-hard
// hashing is what makes an offline attack on the dump expensive.
func HashPassword(plaintext string) (string, error) {
	if err := ValidatePassword(plaintext); err != nil {
		return "", err
	}
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(plaintext), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// ValidatePassword reports whether plaintext may be used as a password.
//
// Length only. A composition rule — a digit, a symbol, a capital — pushes
// people toward short predictable passwords that satisfy it, which is the
// opposite of what the length minimum is for.
func ValidatePassword(plaintext string) error {
	n := utf8.RuneCountInString(plaintext)
	if n < PasswordMinLength {
		return ErrPasswordTooShort
	}
	if n > PasswordMaxLength {
		return ErrPasswordTooLong
	}
	return nil
}

// VerifyPassword reports whether plaintext produced encodedHash.
//
// A malformed or empty hash is a mismatch rather than an error. Callers use
// this to decide whether to authenticate, and a stored value that cannot be
// parsed must not become a way in.
func VerifyPassword(encodedHash, plaintext string) bool {
	salt, want, memory, time, threads, err := decodePasswordHash(encodedHash)
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(plaintext), salt, time, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// DummyVerifyPassword performs the same work as VerifyPassword and always
// fails.
//
// Login calls it when the address has no account, so that a request for an
// unknown address costs the same as one for a known address with the wrong
// password. Without it the response time alone answers "does this person have
// an account here".
func DummyVerifyPassword(plaintext string) bool {
	salt := make([]byte, argonSaltLen)
	argon2.IDKey([]byte(plaintext), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return false
}

func decodePasswordHash(encodedHash string) (salt, key []byte, memory, time uint32, threads uint8, err error) {
	parts := strings.Split(encodedHash, "$")
	// "", "argon2id", "v=19", "m=...,t=...,p=...", salt, key
	if len(parts) != 6 || parts[1] != "argon2id" {
		return nil, nil, 0, 0, 0, errors.New("not an argon2id hash")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return nil, nil, 0, 0, 0, err
	}
	if version != argon2.Version {
		return nil, nil, 0, 0, 0, errors.New("unsupported argon2 version")
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return nil, nil, 0, 0, 0, err
	}
	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, 0, 0, 0, err
	}
	key, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, 0, 0, 0, err
	}
	if len(key) == 0 {
		return nil, nil, 0, 0, 0, errors.New("empty argon2id key")
	}
	return salt, key, memory, time, threads, nil
}
