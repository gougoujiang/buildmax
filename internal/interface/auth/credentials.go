// Package auth provides credential persistence for the BuildMax client.
// It has no dependencies on TUI, desktop, Cobra, or any other project package.
package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Credentials holds the persisted authentication state (server URL, JWT
// token, and basic user info). Stored as JSON on disk.
type Credentials struct {
	ServerURL string `json:"server_url"`
	Token     string `json:"token"`
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	SavedAt   int64  `json:"saved_at"`
}

// IsValid returns true when the credentials contain a non-empty, unexpired
// JWT token. It parses the exp claim from the token payload without
// verifying the signature (clients don't have the server secret).
func (c *Credentials) IsValid() bool {
	if c == nil || c.Token == "" {
		return false
	}
	exp, err := extractJWTExp(c.Token)
	if err != nil {
		return false
	}
	return time.Now().Unix() < exp
}

// extractJWTExp decodes the JWT payload (middle segment) and returns
// the exp claim as a unix timestamp. Returns an error if the token is
// not a three-segment JWT or the exp claim is missing.
func extractJWTExp(tokenStr string) (int64, error) {
	parts := strings.SplitN(tokenStr, ".", 3)
	if len(parts) != 3 {
		return 0, errors.New("not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, err
	}
	var claims struct {
		Exp *float64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return 0, err
	}
	if claims.Exp == nil || *claims.Exp <= 0 {
		return 0, errors.New("missing exp claim")
	}
	return int64(*claims.Exp), nil
}

// Save marshals creds to JSON and writes them to path, creating parent
// directories as needed.
func Save(creds *Credentials, path string) error {
	if creds.SavedAt == 0 {
		creds.SavedAt = time.Now().Unix()
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// Load reads credentials from path. If the file does not exist, it returns
// (nil, nil) — not an error. Any other read or parse failure is an error.
func Load(path string) (*Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var c Credentials
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Clear removes the credentials file at path. It is not an error if the
// file does not exist.
func Clear(path string) error {
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
