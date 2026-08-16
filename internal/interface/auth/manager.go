package auth

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gougoujiang/buildmax/internal/config"
)

// AuthInfo is the caller-facing authentication view.
type AuthInfo struct {
	LoggedIn  bool   `json:"logged_in"`
	ServerURL string `json:"server_url,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	Email     string `json:"email,omitempty"`
	Name      string `json:"name,omitempty"`
}

// Info returns the current caller-facing auth state.
func Info() (AuthInfo, error) {
	creds, err := Load(config.AuthPath())
	if err != nil {
		return AuthInfo{}, err
	}
	if creds == nil || !creds.IsValid() {
		return AuthInfo{LoggedIn: false}, nil
	}
	return AuthInfo{
		LoggedIn:  true,
		ServerURL: creds.ServerURL,
		UserID:    creds.UserID,
		Email:     creds.Email,
		Name:      creds.Name,
	}, nil
}

// IsLoggedIn reports whether valid credentials are present.
func IsLoggedIn() (bool, error) {
	info, err := Info()
	if err != nil {
		return false, err
	}
	return info.LoggedIn, nil
}

// RequireLogin returns an error when no valid credentials are present.
func RequireLogin() error {
	loggedIn, err := IsLoggedIn()
	if err != nil {
		return fmt.Errorf("load auth: %w", err)
	}
	if !loggedIn {
		return fmt.Errorf("not logged in")
	}
	return nil
}

// TokenForServer returns the stored token for calls to serverURL.
//
// It refuses to hand the credential to any other host. A managed model entry
// names its own server, and settings.yaml is editable by anything running as
// the user, so a mismatch would otherwise send a BuildMax token — and every
// prompt that follows — wherever that file said.
func TokenForServer(serverURL string) (string, error) {
	if serverURL == "" {
		return "", errors.New("managed model entry has no server_url")
	}
	creds, err := Load(config.AuthPath())
	if err != nil {
		return "", fmt.Errorf("load auth: %w", err)
	}
	if creds == nil || creds.Token == "" {
		return "", errors.New("not logged in: run `buildmax login`")
	}
	if !sameServer(creds.ServerURL, serverURL) {
		return "", fmt.Errorf("logged in to %s, but this model entry uses %s: run `buildmax login` against that server",
			creds.ServerURL, serverURL)
	}
	if !creds.IsValid() {
		return "", errors.New("login has expired: run `buildmax login`")
	}
	return creds.Token, nil
}

// sameServer compares two server URLs ignoring a trailing slash.
func sameServer(a, b string) bool {
	return strings.TrimRight(a, "/") == strings.TrimRight(b, "/")
}

// SaveCredentials persists a login result.
func SaveCredentials(creds *Credentials) error {
	return Save(creds, config.AuthPath())
}

// Logout clears the stored credentials.
func Logout() error {
	return Clear(config.AuthPath())
}
