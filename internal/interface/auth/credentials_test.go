package auth

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/testsupport"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "auth.json")

	creds := &Credentials{
		ServerURL: "http://localhost:5678",
		Token:     "tok_abc",
		UserID:    "u_123",
		Email:     "a@b.com",
		Name:      "Alice",
	}
	if err := Save(creds, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if creds.SavedAt == 0 {
		t.Fatal("SavedAt should be set")
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Token != "tok_abc" || loaded.Email != "a@b.com" || loaded.ServerURL != "http://localhost:5678" {
		t.Fatalf("loaded = %+v", loaded)
	}
}

// Windows refuses to replace a file while a concurrent reader still holds its
// handle. Save must retry that short-lived sharing violation rather than leave
// a rotated refresh token only in the caller that received it.
func TestSaveRetriesTransientReplaceFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(path, []byte(`{"token":"old"}`), 0600); err != nil {
		t.Fatalf("seed auth file: %v", err)
	}

	originalRename := renameCredentialsFile
	calls := 0
	renameCredentialsFile = func(oldPath, newPath string) error {
		calls++
		if calls == 1 {
			return os.ErrPermission
		}
		return originalRename(oldPath, newPath)
	}
	t.Cleanup(func() { renameCredentialsFile = originalRename })

	if err := Save(&Credentials{Token: "new"}, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if calls != 2 {
		t.Fatalf("rename calls = %d, want 2", calls)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Token != "new" {
		t.Errorf("stored token = %q, want new", loaded.Token)
	}
}

func TestLoadMissing(t *testing.T) {
	c, err := Load("/nonexistent/auth.json")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if c != nil {
		t.Fatalf("expected nil credentials, got %+v", c)
	}
}

func TestClear(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	_ = os.WriteFile(path, []byte(`{}`), 0644)

	if err := Clear(path); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("file should be removed")
	}

	// Clear on already-absent file should not error.
	if err := Clear(path); err != nil {
		t.Fatalf("Clear absent: %v", err)
	}
}

func TestIsValid(t *testing.T) {
	secret := "test-secret"
	validToken := testsupport.SignJWTWithExp("u_1", secret, 24*time.Hour)
	expiredToken := testsupport.SignJWTWithExp("u_1", secret, -1*time.Hour)

	tests := []struct {
		name  string
		creds *Credentials
		want  bool
	}{
		{"nil credentials", nil, false},
		{"empty token", &Credentials{}, false},
		{"malformed token", &Credentials{Token: "not-a-jwt"}, false},
		{"valid unexpired JWT", &Credentials{Token: validToken}, true},
		{"expired JWT", &Credentials{Token: expiredToken}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.creds.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not implement Unix file modes; os.Stat always reports 0666")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	_ = Save(&Credentials{Token: "secret"}, path)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Fatalf("expected 0600, got %04o", perm)
	}
}

// TestLoadWaitsForAReplacementInFlight pins the reason credentialsMu exists: a
// reader waits for a replacement instead of racing it.
//
// POSIX would let the read through either way, so the platform this protects is
// Windows, where opening the file while the rename is in flight fails with a
// sharing violation — which is how it was found, as an intermittent CI failure
// with eight concurrent callers. Asserting the wait rather than the absence of
// an error is what keeps the invariant testable on every platform.
func TestLoadWaitsForAReplacementInFlight(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := Save(&Credentials{Token: "first"}, path); err != nil {
		t.Fatalf("seed: %v", err)
	}

	renaming := make(chan struct{})
	release := make(chan struct{})
	originalRename := renameCredentialsFile
	renameCredentialsFile = func(oldPath, newPath string) error {
		close(renaming)
		<-release
		return originalRename(oldPath, newPath)
	}
	t.Cleanup(func() { renameCredentialsFile = originalRename })

	saved := make(chan error, 1)
	go func() { saved <- Save(&Credentials{Token: "second"}, path) }()
	<-renaming

	loaded := make(chan *Credentials, 1)
	go func() {
		creds, err := Load(path)
		if err != nil {
			t.Errorf("load during a replacement: %v", err)
		}
		loaded <- creds
	}()

	select {
	case creds := <-loaded:
		t.Fatalf("Load returned %+v while the replacement was in flight; it must wait for it", creds)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	if err := <-saved; err != nil {
		t.Fatalf("save: %v", err)
	}
	creds := <-loaded
	if creds == nil || creds.Token != "second" {
		t.Fatalf("loaded %+v after the replacement, want the token it wrote", creds)
	}
}
