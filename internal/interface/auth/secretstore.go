package auth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"

	"github.com/gougoujiang/buildmax/internal/config"
)

// Where a login's two bearer secrets are kept.
const (
	// StorageKeyring means they are in the operating system's credential store
	// and auth.json holds only metadata.
	StorageKeyring = "keyring"
	// StorageFile means they are in auth.json itself, mode 0600. It is what a
	// machine with no usable credential store gets, and every surface reports
	// it: a guarantee that quietly stops holding is worse than one never made.
	StorageFile = "file"
)

// keyringService is what the entry is filed under. Someone looking for it in
// Keychain Access or Credential Manager searches a product name, not a package
// path.
const keyringService = "BuildMax"

// loginSecrets is the half of a login that authenticates. Either token is a
// bearer credential on its own.
//
// They are one entry because they rotate together: an exchange returns both,
// and two entries could disagree about which session is current.
type loginSecrets struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

// secretStore keeps loginSecrets somewhere the Agent's own tools do not reach.
//
// The local Agent runs model-selected commands and Bash sandboxing is off by
// default, so a file readable by this user is readable by whatever the model
// decides to run. The operating system's credential store is the one place on
// the machine that is not.
type secretStore interface {
	Load() (loginSecrets, error)
	Save(loginSecrets) error
	Clear() error
}

type keyringStore struct{ account string }

func (k keyringStore) Load() (loginSecrets, error) {
	raw, err := keyring.Get(keyringService, k.account)
	if err != nil {
		// Nothing stored is not a failure. Being signed out is a state.
		if errors.Is(err, keyring.ErrNotFound) {
			return loginSecrets{}, nil
		}
		return loginSecrets{}, err
	}
	var s loginSecrets
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return loginSecrets{}, err
	}
	return s, nil
}

func (k keyringStore) Save(s loginSecrets) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return keyring.Set(keyringService, k.account, string(raw))
}

func (k keyringStore) Clear() error {
	err := keyring.Delete(keyringService, k.account)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

var (
	storeMu       sync.Mutex
	storeResolved bool
	store         secretStore
)

// activeStore returns this machine's credential store, or nil when it has
// none — in which case the secrets stay in auth.json, where they always were.
//
// The answer is resolved once. Every managed call asks for a token, and the
// probe below is a subprocess on macOS.
func activeStore() secretStore {
	storeMu.Lock()
	defer storeMu.Unlock()
	if !storeResolved {
		store = resolveStore()
		storeResolved = true
	}
	return store
}

// resolveStore is a variable so a test can supply its own store rather than
// write to the contributor's real keychain.
var resolveStore = defaultStore

// defaultStore proves the credential store works before choosing it.
//
// Availability is a run-time fact, not a build-time one: a Linux container with
// no Secret Service, a Windows session without a credential store, and a macOS
// binary running as a service account all compile identically and only differ
// here. A read is the cheapest proof that does not write anything.
func defaultStore() secretStore {
	if strings.EqualFold(strings.TrimSpace(os.Getenv(config.EnvKeyBuildmaxCredentialStore)), StorageFile) {
		return nil
	}
	// The account is the data directory, so CLI and Desktop sharing one
	// BUILDMAX_HOME share one login — signing in once is meant to cover both —
	// while two different homes stay two different logins.
	s := keyringStore{account: config.DataDir()}
	if _, err := keyring.Get(keyringService, s.account); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		slog.Debug("no usable OS credential store; keeping credentials in auth.json", "err", err)
		return nil
	}
	return s
}

// storageKind names where Save would put the secrets.
func storageKind(s secretStore) string {
	if s == nil {
		return StorageFile
	}
	return StorageKeyring
}

// StorageDescription is what a surface tells someone about their credential
// storage. The file mode says why it is the file, because "0600 file" on its
// own reads as a choice rather than as the absence of an alternative.
func StorageDescription(kind string) string {
	if kind == StorageKeyring {
		return "OS credential store"
	}
	return "auth.json (this machine has no usable OS credential store)"
}
