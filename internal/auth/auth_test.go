package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
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
	var nilCreds *Credentials
	if nilCreds.IsValid() {
		t.Fatal("nil should not be valid")
	}
	if (&Credentials{}).IsValid() {
		t.Fatal("empty token should not be valid")
	}
	if !(&Credentials{Token: "x"}).IsValid() {
		t.Fatal("non-empty token should be valid")
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

func TestClientRequestOTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/otp/request" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["email"] != "a@b.com" || body["intent"] != "login" {
			t.Errorf("unexpected body: %v", body)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"otp_sent"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	if err := c.RequestOTP(context.Background(), "a@b.com", "login"); err != nil {
		t.Fatalf("RequestOTP: %v", err)
	}
}

func TestClientRequestOTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"user not found"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.RequestOTP(context.Background(), "a@b.com", "login")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "server 404: user not found" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClientLogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/login" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"jwt123","user":{"id":"u_1","email":"a@b.com","name":"Alice"}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	lr, err := c.Login(context.Background(), "a@b.com", "123456")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if lr.Token != "jwt123" || lr.User.Email != "a@b.com" || lr.User.ID != "u_1" {
		t.Fatalf("unexpected response: %+v", lr)
	}
}

func TestClientLoginError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid otp"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.Login(context.Background(), "a@b.com", "wrong")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "server 401: invalid otp" {
		t.Fatalf("unexpected error: %v", err)
	}
}
