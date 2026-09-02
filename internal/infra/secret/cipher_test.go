package secret

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	coresecret "github.com/gougoujiang/buildmax/internal/core/secret"
)

func testKEK(t *testing.T) KEKProvider {
	t.Helper()
	key := make([]byte, dekSize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		t.Fatal(err)
	}
	kek, err := NewKEKFileProviderFromKeys(map[string][]byte{"file:root:1": key}, "file:root:1")
	if err != nil {
		t.Fatal(err)
	}
	return kek
}

func TestCipher_RoundTrip(t *testing.T) {
	c := NewCipher(testKEK(t))
	items := coresecret.Items{"access_key_id": "AKIA", "secret_access_key": "wJa/secret", "region": "us-east-1"}
	aad := []byte("dep1|tm_1|sec_1")

	sealed, err := c.Seal(items, aad)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed.Ciphertext, []byte("wJa/secret")) {
		t.Fatal("ciphertext contains the plaintext value")
	}

	got, err := c.Open(sealed, aad)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(items) {
		t.Fatalf("got %d items, want %d", len(got), len(items))
	}
	for k, v := range items {
		if got[k] != v {
			t.Fatalf("item %q = %q, want %q", k, got[k], v)
		}
	}
}

func TestCipher_FreshNoncePerSeal(t *testing.T) {
	c := NewCipher(testKEK(t))
	items := coresecret.Items{"k": "v"}
	aad := []byte("d|t|s")
	a, err := c.Seal(items, aad)
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Seal(items, aad)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a.Nonce, b.Nonce) {
		t.Fatal("two seals reused a nonce")
	}
	if bytes.Equal(a.Ciphertext, b.Ciphertext) {
		t.Fatal("two seals of the same items produced identical ciphertext")
	}
}

func TestCipher_WrongAADFails(t *testing.T) {
	c := NewCipher(testKEK(t))
	sealed, err := c.Seal(coresecret.Items{"k": "v"}, []byte("d|tm_1|sec_1"))
	if err != nil {
		t.Fatal(err)
	}
	// Same deployment and secret, different team: a ciphertext moved to another
	// owner must not open.
	if _, err := c.Open(sealed, []byte("d|tm_2|sec_1")); err == nil {
		t.Fatal("opened under mismatched AAD")
	}
}

func TestCipher_TamperedCiphertextFails(t *testing.T) {
	c := NewCipher(testKEK(t))
	aad := []byte("d|t|s")
	sealed, err := c.Seal(coresecret.Items{"k": "v"}, aad)
	if err != nil {
		t.Fatal(err)
	}
	sealed.Ciphertext[0] ^= 0xFF
	if _, err := c.Open(sealed, aad); err == nil {
		t.Fatal("opened tampered ciphertext")
	}
}

func TestCipher_EmptyItemsRoundTrip(t *testing.T) {
	c := NewCipher(testKEK(t))
	aad := []byte("d|t|s")
	sealed, err := c.Seal(coresecret.Items{}, aad)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Open(sealed, aad)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d items, want 0", len(got))
	}
}
