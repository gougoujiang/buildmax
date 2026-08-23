package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PluginSource is how a plugin directory came to exist. It is recorded by
// whatever put the directory there, never inferred from the directory itself.
type PluginSource string

const (
	// PluginSourceRepository is a Git working tree the user cloned or checked out.
	PluginSourceRepository PluginSource = "repository"
	// PluginSourceMarketplace is a release this machine downloaded and verified.
	PluginSourceMarketplace PluginSource = "marketplace"
	// PluginSourceLocal is an ordinary directory with no other provenance.
	PluginSourceLocal PluginSource = "local"
	// PluginSourceUnknown is a directory nothing recorded, which is the normal
	// state of a manual `git clone` until something inspects it.
	PluginSourceUnknown PluginSource = ""
)

// PluginStateVersion is the schema version written into .state.json. The file
// is machine-authored, so stamping it costs an author nothing and lets a later
// format change be detected instead of guessed at.
const PluginStateVersion = 1

// PluginStateFile is the name of the supplemental state file. It begins with a
// dot because §4.1 reserves dot-prefixed entries in the plugins directory for
// BuildMax's own staging, cache, and state.
const PluginStateFile = ".state.json"

const pluginStateLockFile = ".state.lock"

// PluginState is the installer's record for one plugin directory.
//
// It holds only what the directory cannot tell BuildMax itself. Nothing here is
// required to discover or load a plugin: a lost state file costs provenance and
// the disabled flag, not the plugin.
type PluginState struct {
	Source   PluginSource `json:"source,omitempty"`
	Disabled bool         `json:"disabled,omitempty"`

	// Repository provenance, when the installer knew it. A manual clone has
	// none until something inspects the checkout.
	RepositoryURL string `json:"repository_url,omitempty"`
	LastCommit    string `json:"last_commit,omitempty"`

	// Marketplace provenance, which identifies the exact bytes installed.
	MarketplaceServer string `json:"marketplace_server,omitempty"`
	CatalogID         string `json:"catalog_id,omitempty"`
	ReleaseVersion    string `json:"release_version,omitempty"`
	Digest            string `json:"digest,omitempty"`

	InstalledAt time.Time `json:"installed_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

// PluginStates is the on-disk shape of .state.json.
//
// Entries are keyed by directory name rather than by plugin name, because the
// directory is the only identity that exists before a manifest parses — and a
// manifest that fails to parse is exactly when the disabled flag still matters.
type PluginStates struct {
	Version int                    `json:"version"`
	Plugins map[string]PluginState `json:"plugins"`
}

// Get returns the record for a directory, and whether one was recorded.
func (s PluginStates) Get(dir string) (PluginState, bool) {
	st, ok := s.Plugins[dir]
	return st, ok
}

// Set records state for a directory.
func (s *PluginStates) Set(dir string, st PluginState) {
	if s.Plugins == nil {
		s.Plugins = map[string]PluginState{}
	}
	s.Plugins[dir] = st
}

// Remove drops a directory's record.
func (s *PluginStates) Remove(dir string) { delete(s.Plugins, dir) }

// pluginStateMu serializes this process's own use of the state file. The lock
// file below settles the same collision between processes; this settles the one
// between goroutines, which no file lock can see.
var pluginStateMu sync.Mutex

// LoadPluginStates reads pluginsDir/.state.json.
//
// A missing file returns empty state and no error: discovery does not depend on
// it. A damaged one is an error the caller reports as lost provenance, having
// still loaded every valid plugin directory.
func LoadPluginStates(pluginsDir string) (PluginStates, error) {
	pluginStateMu.Lock()
	data, err := os.ReadFile(filepath.Join(pluginsDir, PluginStateFile))
	pluginStateMu.Unlock()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PluginStates{Version: PluginStateVersion}, nil
		}
		return PluginStates{}, fmt.Errorf("read plugin state: %w", err)
	}
	var s PluginStates
	if err := json.Unmarshal(data, &s); err != nil {
		return PluginStates{}, fmt.Errorf("parse plugin state: %w", err)
	}
	if s.Plugins == nil {
		s.Plugins = map[string]PluginState{}
	}
	return s, nil
}

// UpdatePluginStates applies mutate to the state file under a lock held across
// the read and the write.
//
// Every writer goes through here rather than through a Load/Save pair, because
// the file is one shared map: a read-modify-write that is not held together
// loses whichever entry the other writer added. A CLI install racing a running
// Desktop is the ordinary case, not a rare one.
func UpdatePluginStates(pluginsDir string, mutate func(*PluginStates) error) error {
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		return fmt.Errorf("create plugins dir: %w", err)
	}
	pluginStateMu.Lock()
	defer pluginStateMu.Unlock()

	release, err := acquirePluginStateLock(pluginsDir)
	if err != nil {
		return err
	}
	defer release()

	states, err := loadPluginStatesLocked(pluginsDir)
	if err != nil {
		return err
	}
	if err := mutate(&states); err != nil {
		return err
	}
	states.Version = PluginStateVersion
	if states.Plugins == nil {
		states.Plugins = map[string]PluginState{}
	}
	return writePluginStates(pluginsDir, states)
}

// loadPluginStatesLocked is LoadPluginStates without the mutex, for a caller
// that already holds it.
func loadPluginStatesLocked(pluginsDir string) (PluginStates, error) {
	data, err := os.ReadFile(filepath.Join(pluginsDir, PluginStateFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PluginStates{Version: PluginStateVersion, Plugins: map[string]PluginState{}}, nil
		}
		return PluginStates{}, fmt.Errorf("read plugin state: %w", err)
	}
	var s PluginStates
	if err := json.Unmarshal(data, &s); err != nil {
		return PluginStates{}, fmt.Errorf("parse plugin state: %w", err)
	}
	if s.Plugins == nil {
		s.Plugins = map[string]PluginState{}
	}
	return s, nil
}

// renamePluginStateFile is a seam for testing a failed replacement.
var renamePluginStateFile = os.Rename

func writePluginStates(pluginsDir string, states PluginStates) error {
	data, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return fmt.Errorf("encode plugin state: %w", err)
	}
	tmp, err := os.CreateTemp(pluginsDir, ".state-*.json")
	if err != nil {
		return fmt.Errorf("create plugin state temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("write plugin state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write plugin state: %w", err)
	}
	if err := replacePluginState(tmpName, filepath.Join(pluginsDir, PluginStateFile)); err != nil {
		return fmt.Errorf("replace plugin state: %w", err)
	}
	return nil
}

const (
	// stateRenameAttempts bounds retries for a transient Windows sharing
	// violation while replacing the state file. POSIX lets a reader survive a
	// rename; Windows fails the rename while a reader has the file open.
	// Readers here decode a small document and close it, so waiting longer
	// would hide a permission problem rather than make the replace work.
	//
	// internal/interface/auth carries the same retry for auth.json, for the
	// same collision: this file has the reader-and-writer-in-two-processes
	// shape that UpdatePluginStates already documents.
	stateRenameAttempts   = 10
	stateRenameRetryDelay = 10 * time.Millisecond
)

func replacePluginState(tmpName, path string) error {
	var err error
	for attempt := range stateRenameAttempts {
		err = renamePluginStateFile(tmpName, path)
		if err == nil || !errors.Is(err, os.ErrPermission) || attempt == stateRenameAttempts-1 {
			return err
		}
		time.Sleep(stateRenameRetryDelay)
	}
	return err
}

// Lock timing. These are variables so a test can exercise the contention and
// takeover paths without waiting out the real values.
var (
	pluginLockTimeout    = 5 * time.Second
	pluginLockRetryDelay = 25 * time.Millisecond
	// pluginLockStaleAfter is when a held lock is treated as abandoned. Every
	// critical section here is a small read, a map edit, and a rename, so a
	// lock this old belongs to a process that died holding it. Waiting forever
	// on a dead process is the worse failure.
	pluginLockStaleAfter = 30 * time.Second
)

// acquirePluginStateLock takes the cross-process lock and returns its release.
func acquirePluginStateLock(pluginsDir string) (func(), error) {
	path := filepath.Join(pluginsDir, pluginStateLockFile)
	deadline := time.Now().Add(pluginLockTimeout)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			fmt.Fprintf(f, "%d\n", os.Getpid())
			_ = f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("lock plugin state: %w", err)
		}
		if st, statErr := os.Stat(path); statErr == nil && time.Since(st.ModTime()) > pluginLockStaleAfter {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("lock plugin state: %s is held by another process", path)
		}
		time.Sleep(pluginLockRetryDelay)
	}
}
