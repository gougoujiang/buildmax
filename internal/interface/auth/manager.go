package auth

import (
	"fmt"

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

// SaveCredentials persists a login result.
func SaveCredentials(creds *Credentials) error {
	return Save(creds, config.AuthPath())
}

// Logout clears the stored credentials.
func Logout() error {
	return Clear(config.AuthPath())
}
