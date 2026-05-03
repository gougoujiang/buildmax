package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"buildmax/internal/util"
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
	validToken := util.SignJWTWithExp("u_1", secret, 24*time.Hour)
	expiredToken := util.SignJWTWithExp("u_1", secret, -1*time.Hour)

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
