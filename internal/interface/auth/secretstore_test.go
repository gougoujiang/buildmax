package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain keeps the package away from the contributor's own keychain. Tests
// that want a store install one with useSecretStore.
func TestMain(m *testing.M) {
	useNoSecretStore()
	os.Exit(m.Run())
}

func useNoSecretStore() {
	storeMu.Lock()
	store, storeResolved = nil, true
	storeMu.Unlock()
	forgetSecrets()
}

// useSecretStore installs s for one test and restores the file-only default.
func useSecretStore(t *testing.T, s secretStore) {
	t.Helper()
	storeMu.Lock()
	store, storeResolved = s, true
	storeMu.Unlock()
	forgetSecrets()
	t.Cleanup(useNoSecretStore)
}

// fakeStore stands in for the OS credential store and counts reads, which is
// how the cache test tells a hit from a miss.
type fakeStore struct {
	secrets loginSecrets
	held    bool
	loads   int
	saveErr error
}

func (f *fakeStore) Load() (loginSecrets, error) {
	f.loads++
	if !f.held {
		return loginSecrets{}, nil
	}
	return f.secrets, nil
}

func (f *fakeStore) Save(s loginSecrets) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.secrets, f.held = s, true
	return nil
}

func (f *fakeStore) Clear() error {
	f.secrets, f.held = loginSecrets{}, false
	return nil
}

func readRawFile(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return raw
}

// The point of the whole change: with a credential store, auth.json holds no
// bearer secret. A test that only checked Load round-tripping would pass just
// as well with the tokens still sitting in the file.
func TestSaveKeepsSecretsOutOfTheFile(t *testing.T) {
	f := &fakeStore{}
	useSecretStore(t, f)
	path := filepath.Join(t.TempDir(), "auth.json")

	creds := &Credentials{ServerURL: "http://s", Token: "tok_abc", RefreshToken: "ref_abc", Email: "a@b.com"}
	if err := Save(creds, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw := readRawFile(t, path)
	if _, ok := raw["token"]; ok {
		t.Error("auth.json still holds the access token")
	}
	if _, ok := raw["refresh_token"]; ok {
		t.Error("auth.json still holds the refresh token")
	}
	if raw["storage"] != StorageKeyring {
		t.Errorf("storage = %v, want %q", raw["storage"], StorageKeyring)
	}
	if f.secrets.Token != "tok_abc" || f.secrets.RefreshToken != "ref_abc" {
		t.Errorf("store holds %+v", f.secrets)
	}
	if creds.Storage != StorageKeyring {
		t.Errorf("Save left Storage = %q", creds.Storage)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Token != "tok_abc" || loaded.RefreshToken != "ref_abc" {
		t.Errorf("loaded = %+v", loaded)
	}
	if loaded.Storage != StorageKeyring {
		t.Errorf("loaded Storage = %q", loaded.Storage)
	}
}

// A machine with no credential store keeps working, and says so.
func TestSaveWithoutStoreKeepsSecretsInTheFile(t *testing.T) {
	useNoSecretStore()
	path := filepath.Join(t.TempDir(), "auth.json")

	creds := &Credentials{ServerURL: "http://s", Token: "tok_abc", RefreshToken: "ref_abc"}
	if err := Save(creds, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw := readRawFile(t, path)
	if raw["token"] != "tok_abc" || raw["refresh_token"] != "ref_abc" {
		t.Errorf("file = %v, want the tokens inline", raw)
	}
	if raw["storage"] != StorageFile {
		t.Errorf("storage = %v, want %q", raw["storage"], StorageFile)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Token != "tok_abc" || loaded.Storage != StorageFile {
		t.Errorf("loaded = %+v", loaded)
	}
}

// A file written before the split, opened on a machine that has a credential
// store, must not leave the plaintext behind.
func TestLoadMovesLegacyFileSecretsIntoTheStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	useNoSecretStore()
	legacy := &Credentials{ServerURL: "http://s", Token: "tok_old", RefreshToken: "ref_old", Email: "a@b.com"}
	if err := Save(legacy, path); err != nil {
		t.Fatalf("Save legacy: %v", err)
	}

	f := &fakeStore{}
	useSecretStore(t, f)

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Token != "tok_old" || loaded.RefreshToken != "ref_old" {
		t.Fatalf("loaded = %+v", loaded)
	}
	if loaded.Storage != StorageKeyring {
		t.Errorf("Storage = %q, want %q", loaded.Storage, StorageKeyring)
	}
	if f.secrets.Token != "tok_old" {
		t.Errorf("store holds %+v", f.secrets)
	}
	raw := readRawFile(t, path)
	if _, ok := raw["token"]; ok {
		t.Error("the legacy plaintext token is still in auth.json")
	}
	if _, ok := raw["refresh_token"]; ok {
		t.Error("the legacy plaintext refresh token is still in auth.json")
	}
}

// A store that cannot accept the move must not cost someone their login.
func TestLoadKeepsWorkingWhenTheMoveFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	useNoSecretStore()
	legacy := &Credentials{ServerURL: "http://s", Token: "tok_old", RefreshToken: "ref_old"}
	if err := Save(legacy, path); err != nil {
		t.Fatalf("Save legacy: %v", err)
	}

	useSecretStore(t, &fakeStore{saveErr: errors.New("keyring locked")})

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Token != "tok_old" || loaded.Storage != StorageFile {
		t.Errorf("loaded = %+v, want the file's own tokens and %q", loaded, StorageFile)
	}
}

// TokenForServer runs before every managed call, so Load must not reach the
// credential store each time — on macOS that is a subprocess.
func TestLoadCachesSecretsUntilTheFileChanges(t *testing.T) {
	f := &fakeStore{}
	useSecretStore(t, f)
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := Save(&Credentials{ServerURL: "http://s", Token: "tok_1", RefreshToken: "ref_1"}, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	before := f.loads
	for i := 0; i < 3; i++ {
		if _, err := Load(path); err != nil {
			t.Fatalf("Load: %v", err)
		}
	}
	if f.loads != before {
		t.Errorf("store read %d times across three loads, want 0", f.loads-before)
	}

	// What another BuildMax process refreshing this login looks like from here:
	// a new saved_at in the file and a new secret in the store.
	f.secrets = loginSecrets{Token: "tok_2", RefreshToken: "ref_2"}
	other := &Credentials{ServerURL: "http://s", Token: "tok_2", RefreshToken: "ref_2"}
	if err := writeCredentialsFile(withNewStamp(other), path); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Token != "tok_2" {
		t.Errorf("token = %q, want the rotated one", loaded.Token)
	}
}

// withNewStamp writes what another process's Save would have left: a metadata
// file with a later saved_at and no secrets in it.
func withNewStamp(c *Credentials) *Credentials {
	out := *c
	out.Token, out.RefreshToken = "", ""
	out.Storage = StorageKeyring
	out.SavedAt = out.SavedAt.Add(1)
	return &out
}

func TestClearRemovesBothHalves(t *testing.T) {
	f := &fakeStore{}
	useSecretStore(t, f)
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := Save(&Credentials{ServerURL: "http://s", Token: "tok", RefreshToken: "ref"}, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := Clear(path); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if f.held {
		t.Error("the credential store still holds the secrets")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("auth.json still exists: %v", err)
	}
	loaded, err := Load(path)
	if err != nil || loaded != nil {
		t.Errorf("Load after Clear = %+v, %v", loaded, err)
	}
}

func TestStorageDescriptionNamesTheDowngrade(t *testing.T) {
	if got := StorageDescription(StorageKeyring); got == StorageDescription(StorageFile) {
		t.Fatal("both modes describe themselves the same way")
	}
	if got := StorageDescription(StorageFile); !strings.Contains(got, "auth.json") {
		t.Errorf("file description %q does not name the file", got)
	}
}
