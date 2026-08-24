// Package auth provides credential persistence and renewal for the BuildMax
// client. It depends on internal/config for the file location and
// internal/interface/client to exchange a refresh token, and on nothing from
// the TUI, desktop, or Cobra layers above it.
package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Credentials is one login, assembled from both places it is kept: the
// metadata from auth.json and the two secrets from wherever this machine keeps
// secrets.
//
// Token and RefreshToken carry `omitempty` because auth.json holds them only in
// StorageFile mode. Under StorageKeyring they are blanked before the file is
// written, which is the whole point of the split.
type Credentials struct {
	ServerURL string `json:"server_url"`
	// Token is the access token. The field keeps its original name so that a
	// credentials file written before refresh tokens existed still loads.
	Token string `json:"token,omitempty"`
	// RefreshToken renews the access token without another login code. Empty
	// means this login ends when Token expires — either an old file or a server
	// that stores no refresh tokens.
	RefreshToken string    `json:"refresh_token,omitempty"`
	UserID       string    `json:"user_id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	SavedAt      time.Time `json:"saved_at"`
	// Storage names where the two tokens came from: StorageKeyring or
	// StorageFile. Load always sets it from the store that answered, so it
	// describes this machine rather than the machine that wrote the file.
	Storage string `json:"storage,omitempty"`
}

const (
	// credentialRenameAttempts bounds retries for a transient Windows sharing
	// violation while replacing auth.json. Readers open the old file only long
	// enough to decode it, so waiting longer would hide a persistent permission
	// problem rather than make an atomic update more reliable.
	credentialRenameAttempts   = 10
	credentialRenameRetryDelay = 10 * time.Millisecond
)

// renameCredentialsFile is a seam for testing transient replacement failures.
var renameCredentialsFile = os.Rename

// credentialsMu serializes this process's use of the credentials file.
//
// Replacing auth.json is a rename, which POSIX lets a concurrent reader survive
// and Windows does not: while the replace is in flight, a reader's open fails
// with a sharing violation. The retry above covers the writer's half of that
// collision, and it is the only half it can cover — a reader in another process
// still has to survive on its own. This lock settles the half that can be
// settled outright: goroutines in one binary, which is what a run renewing its
// token while another caller reads it actually is.
var credentialsMu sync.RWMutex

// IsValid returns true when the credentials contain a non-empty, unexpired
// access token. It parses the exp claim from the token payload without
// verifying the signature (clients don't have the server secret).
func (c *Credentials) IsValid() bool {
	if c == nil || c.Token == "" {
		return false
	}
	exp, err := extractJWTExp(c.Token)
	if err != nil {
		return false
	}
	return time.Now().Before(exp)
}

// IsUsable reports whether these credentials can still authenticate a call,
// either because the access token is good or because it can be renewed.
//
// This is the question every command actually has: "am I signed in?" — not
// "is this particular token still fresh?", which stopped being the same
// question once a refresh token existed.
func (c *Credentials) IsUsable() bool {
	if c == nil {
		return false
	}
	return c.IsValid() || c.RefreshToken != ""
}

// needsRefresh reports whether the access token is spent or close enough to it
// that a call starting now might outlive it.
func (c *Credentials) needsRefresh(skew time.Duration) bool {
	if c == nil || c.Token == "" {
		return true
	}
	exp, err := extractJWTExp(c.Token)
	if err != nil {
		// An unparseable token is not one to keep sending.
		return true
	}
	return !time.Now().Add(skew).Before(exp)
}

// extractJWTExp decodes the JWT payload (middle segment) and returns the exp
// claim. Returns an error if the token is not a three-segment JWT or the exp
// claim is missing.
//
// The claim stays a NumericDate on the wire — that is RFC 7519's format, not
// ours — and becomes an instant here, at the boundary.
func extractJWTExp(tokenStr string) (time.Time, error) {
	parts := strings.SplitN(tokenStr, ".", 3)
	if len(parts) != 3 {
		return time.Time{}, errors.New("not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, err
	}
	var claims struct {
		Exp *float64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, err
	}
	if claims.Exp == nil || *claims.Exp <= 0 {
		return time.Time{}, errors.New("missing exp claim")
	}
	return time.Unix(int64(*claims.Exp), 0).UTC(), nil
}

// secretCache keeps this process from asking the OS credential store on every
// managed call. TokenForServer runs before each one, and on macOS a credential
// store read is a subprocess.
//
// auth.json's saved_at is the invalidation signal. Every Save stamps it, so a
// refresh in another BuildMax process changes it and this one re-reads; the
// metadata file is a cheap read either way. Sharing one login across CLI and
// Desktop is the point, so noticing the other process's rotation is not
// optional.
var secretCache struct {
	mu      sync.Mutex
	savedAt time.Time
	secrets loginSecrets
	valid   bool
}

func rememberSecrets(savedAt time.Time, s loginSecrets) {
	secretCache.mu.Lock()
	defer secretCache.mu.Unlock()
	secretCache.savedAt, secretCache.secrets, secretCache.valid = savedAt, s, true
}

func forgetSecrets() {
	secretCache.mu.Lock()
	defer secretCache.mu.Unlock()
	secretCache.valid = false
}

// cachedSecrets returns the secrets stored alongside the auth.json that was
// last written at savedAt, reading the store only when that stamp has moved.
func cachedSecrets(store secretStore, savedAt time.Time) (loginSecrets, error) {
	secretCache.mu.Lock()
	if secretCache.valid && secretCache.savedAt.Equal(savedAt) {
		defer secretCache.mu.Unlock()
		return secretCache.secrets, nil
	}
	secretCache.mu.Unlock()

	s, err := store.Load()
	if err != nil {
		return loginSecrets{}, err
	}
	rememberSecrets(savedAt, s)
	return s, nil
}

// Save persists creds: the two secrets go to this machine's credential store,
// and auth.json keeps everything else.
//
// A machine with no credential store keeps the secrets in auth.json as before.
// That is a downgrade, so Save records which one happened in the file rather
// than leaving the reader to infer it from a missing field.
func Save(creds *Credentials, path string) error {
	store := activeStore()
	// Stamped on every save, not only the first: saved_at is what tells another
	// process its cached secrets are stale.
	savedAt := time.Now().UTC()

	onDisk := *creds
	onDisk.SavedAt = savedAt
	onDisk.Storage = storageKind(store)
	if store != nil {
		secrets := loginSecrets{Token: creds.Token, RefreshToken: creds.RefreshToken}
		if err := store.Save(secrets); err != nil {
			return fmt.Errorf("store credentials in the OS credential store: %w", err)
		}
		onDisk.Token, onDisk.RefreshToken = "", ""
		rememberSecrets(savedAt, secrets)
	}
	if err := writeCredentialsFile(&onDisk, path); err != nil {
		return err
	}
	creds.SavedAt = savedAt
	creds.Storage = onDisk.Storage
	return nil
}

// writeCredentialsFile replaces path with creds, atomically.
//
// The write goes to a temporary file and is renamed into place. Refreshing
// rewrites this file while other BuildMax processes may be reading it, and a
// truncate-then-write would let one of them load a half-written file and
// conclude it is not signed in.
func writeCredentialsFile(creds *Credentials, path string) error {
	credentialsMu.Lock()
	defer credentialsMu.Unlock()
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".auth-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		// Removing a file that was renamed away is expected and not an error.
		_ = os.Remove(tmpName)
	}()
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	for attempt := 0; attempt < credentialRenameAttempts; attempt++ {
		err = renameCredentialsFile(tmpName, path)
		if err == nil || !errors.Is(err, os.ErrPermission) || attempt == credentialRenameAttempts-1 {
			return err
		}
		time.Sleep(credentialRenameRetryDelay)
	}
	return err
}

// Load reads credentials from path and fills in the secrets from wherever this
// machine keeps them. If the file does not exist, it returns (nil, nil) — not
// an error. Any other read or parse failure is an error.
func Load(path string) (*Credentials, error) {
	credentialsMu.RLock()
	data, err := os.ReadFile(path)
	credentialsMu.RUnlock()
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
	store := activeStore()
	if store == nil {
		c.Storage = StorageFile
		return &c, nil
	}
	if c.Token != "" || c.RefreshToken != "" {
		// Secrets still in the file: either it predates this split, or it was
		// written on a machine — or a run — with no credential store. Move them
		// now and rewrite the file without them. A read that performs a write is
		// unusual, but leaving plaintext bearer tokens on disk once a safe place
		// exists is exactly what this is here to stop, and the move spends
		// nothing: the session is untouched.
		moved := c
		if err := Save(&moved, path); err != nil {
			slog.Warn("could not move stored credentials into the OS credential store", "err", err)
			c.Storage = StorageFile
			return &c, nil
		}
		return &moved, nil
	}
	secrets, err := cachedSecrets(store, c.SavedAt)
	if err != nil {
		return nil, fmt.Errorf("read credentials from the OS credential store: %w", err)
	}
	c.Token, c.RefreshToken = secrets.Token, secrets.RefreshToken
	c.Storage = StorageKeyring
	return &c, nil
}

// Clear removes the credentials file at path and the secrets that belong to it.
// It is not an error if either is already gone.
func Clear(path string) error {
	store := activeStore()
	forgetSecrets()

	credentialsMu.Lock()
	err := os.Remove(path)
	credentialsMu.Unlock()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// After the file, so a credential store that refuses to delete still leaves
	// the machine signed out: Load needs the metadata, and it is gone.
	if store != nil {
		if err := store.Clear(); err != nil {
			return fmt.Errorf("remove credentials from the OS credential store: %w", err)
		}
	}
	return nil
}
