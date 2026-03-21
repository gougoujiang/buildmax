// Package auth provides credential persistence and an HTTP client for
// authenticating against the BuildMax server. It has no dependencies on
// TUI, desktop, Cobra, or any other project package.
package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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

// IsValid returns true when the credentials contain a non-empty token.
// It does not check token expiry.
func (c *Credentials) IsValid() bool {
	return c != nil && c.Token != ""
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
