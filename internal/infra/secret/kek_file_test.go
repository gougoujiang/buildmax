package secret

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func randKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, dekSize)
	if _, err := io.ReadFull(rand.Reader, k); err != nil {
		t.Fatal(err)
	}
	return k
}

func TestKEKFile_WrapUnwrapRoundTrip(t *testing.T) {
	kek, err := NewKEKFileProviderFromKeys(map[string][]byte{"file:root:1": randKey(t)}, "file:root:1")
	if err != nil {
		t.Fatal(err)
	}
	dek := randKey(t)
	wrapped, keyID, err := kek.Wrap(dek)
	if err != nil {
		t.Fatal(err)
	}
	if keyID != "file:root:1" {
		t.Fatalf("keyID = %q, want file:root:1", keyID)
	}
	got, err := kek.Unwrap(wrapped, keyID)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(dek) {
		t.Fatal("unwrapped DEK does not match")
	}
}

// TestKEKFile_RotationUnwrapsOldWrapsNew is the property KEK rotation relies
// on: after adding a new current key, a DEK wrapped under the old one still
// opens (the file holds both), and new wraps use the new key.
func TestKEKFile_RotationUnwrapsOldWrapsNew(t *testing.T) {
	old := randKey(t)
	kekOld, err := NewKEKFileProviderFromKeys(map[string][]byte{"file:root:1": old}, "file:root:1")
	if err != nil {
		t.Fatal(err)
	}
	dek := randKey(t)
	wrappedOld, oldID, err := kekOld.Wrap(dek)
	if err != nil {
		t.Fatal(err)
	}

	// Rotate: file now holds both keys, current is the new one.
	kekNew, err := NewKEKFileProviderFromKeys(map[string][]byte{
		"file:root:1": old,
		"file:root:2": randKey(t),
	}, "file:root:2")
	if err != nil {
		t.Fatal(err)
	}
	got, err := kekNew.Unwrap(wrappedOld, oldID)
	if err != nil {
		t.Fatalf("old-wrapped DEK must still open after rotation: %v", err)
	}
	if string(got) != string(dek) {
		t.Fatal("rotation changed the unwrapped DEK")
	}
	if _, newID, err := kekNew.Wrap(dek); err != nil || newID != "file:root:2" {
		t.Fatalf("new wrap keyID = %q err = %v, want file:root:2", newID, err)
	}
}

func TestKEKFile_UnwrapUnknownKeyFails(t *testing.T) {
	kek, err := NewKEKFileProviderFromKeys(map[string][]byte{"file:root:1": randKey(t)}, "file:root:1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kek.Unwrap([]byte("whatever"), "file:root:gone"); err == nil {
		t.Fatal("unwrapped under a key the file does not hold")
	}
}

func TestLoadKEKFile_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kek.json")
	writeKEKFile(t, path, kekFile{
		Current: "file:root:1",
		Keys:    map[string]string{"file:root:1": base64.StdEncoding.EncodeToString(randKey(t))},
	})
	if _, err := LoadKEKFile(path); err != nil {
		t.Fatalf("valid file rejected: %v", err)
	}
}

func TestLoadKEKFile_Rejects(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]kekFile{
		"no current":       {Keys: map[string]string{"file:root:1": base64.StdEncoding.EncodeToString(randKey(t))}},
		"no keys":          {Current: "file:root:1"},
		"current not held": {Current: "file:root:9", Keys: map[string]string{"file:root:1": base64.StdEncoding.EncodeToString(randKey(t))}},
		"wrong key size":   {Current: "file:root:1", Keys: map[string]string{"file:root:1": base64.StdEncoding.EncodeToString([]byte("short"))}},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, "kek.json")
			writeKEKFile(t, path, f)
			if _, err := LoadKEKFile(path); err == nil {
				t.Fatal("invalid KEK file accepted")
			}
		})
	}
}

func TestLoadKEKFile_MissingFile(t *testing.T) {
	if _, err := LoadKEKFile(filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Fatal("missing KEK file accepted")
	}
}

func writeKEKFile(t *testing.T, path string, f kekFile) {
	t.Helper()
	raw, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}
