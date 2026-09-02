// Package secret implements the cryptography behind Team Secrets: envelope
// encryption of a Secret's item map, and the key-encryption-key providers that
// wrap the per-write data keys. It is the only place that touches plaintext
// item bytes and key material. See docs/design/team-secrets.md §9.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"

	coresecret "github.com/gougoujiang/buildmax/internal/core/secret"
)

// dekSize is 32 bytes: AES-256. A fresh DEK is generated per write, so a
// single key never encrypts two different item maps -- the property GCM
// requires and the reason the group is one atomic ciphertext.
const dekSize = 32

// KEKProvider wraps and unwraps a data-encryption key. The DEK never persists
// in the clear; only its wrapped form and the id of the KEK that wrapped it
// are stored. Implementations: a mounted key file (kekFileProvider), and later
// a cloud KMS or Vault transit key. See docs/design/team-secrets.md §9.1.
type KEKProvider interface {
	// Wrap seals dek and returns the wrapped bytes plus the id of the KEK
	// used, which unwrap needs to select the right key after a rotation.
	Wrap(dek []byte) (wrapped []byte, keyID string, err error)
	// Unwrap opens a DEK sealed under the KEK named by keyID.
	Unwrap(wrapped []byte, keyID string) (dek []byte, err error)
}

// Cipher seals and opens a Secret's item map with envelope encryption: a fresh
// random DEK per write under AES-256-GCM, and the DEK wrapped by the KEK.
type Cipher struct {
	kek KEKProvider
}

// NewCipher returns a Cipher over the given KEK provider.
func NewCipher(kek KEKProvider) *Cipher { return &Cipher{kek: kek} }

// Cipher is the Sealer the secret service depends on.
var _ coresecret.Sealer = (*Cipher)(nil)

// Seal encrypts items into a Sealed blob. aad is bound into the ciphertext, so
// a blob authenticated for one deployment/team/secret fails to open under
// another -- the caller passes the associated data that names those.
func (c *Cipher) Seal(items coresecret.Items, aad []byte) (coresecret.Sealed, error) {
	plaintext, err := json.Marshal(map[string]string(items))
	if err != nil {
		return coresecret.Sealed{}, fmt.Errorf("secret: marshal items: %w", err)
	}

	dek := make([]byte, dekSize)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return coresecret.Sealed{}, fmt.Errorf("secret: generate dek: %w", err)
	}

	gcm, err := newGCM(dek)
	if err != nil {
		return coresecret.Sealed{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return coresecret.Sealed{}, fmt.Errorf("secret: generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, aad)

	wrapped, keyID, err := c.kek.Wrap(dek)
	if err != nil {
		return coresecret.Sealed{}, fmt.Errorf("secret: wrap dek: %w", err)
	}
	return coresecret.Sealed{
		Ciphertext: ciphertext,
		Nonce:      nonce,
		WrappedDEK: wrapped,
		KeyID:      keyID,
	}, nil
}

// Open decrypts a Sealed blob back into items. aad must equal what Seal was
// given, or authentication fails. A failure here means tampering, a wrong KEK,
// or a mismatched associated data -- never a partial result.
func (c *Cipher) Open(s coresecret.Sealed, aad []byte) (coresecret.Items, error) {
	dek, err := c.kek.Unwrap(s.WrappedDEK, s.KeyID)
	if err != nil {
		return nil, fmt.Errorf("secret: unwrap dek: %w", err)
	}
	gcm, err := newGCM(dek)
	if err != nil {
		return nil, err
	}
	if len(s.Nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("secret: nonce is %d bytes, want %d", len(s.Nonce), gcm.NonceSize())
	}
	plaintext, err := gcm.Open(nil, s.Nonce, s.Ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("secret: open ciphertext: %w", err)
	}
	var items map[string]string
	if err := json.Unmarshal(plaintext, &items); err != nil {
		return nil, fmt.Errorf("secret: unmarshal items: %w", err)
	}
	return items, nil
}

// newGCM builds an AES-256-GCM AEAD from a 32-byte key.
func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != dekSize {
		return nil, fmt.Errorf("secret: key is %d bytes, want %d", len(key), dekSize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secret: aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secret: gcm: %w", err)
	}
	return gcm, nil
}
