package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLoadPluginStatesMissingFileIsEmpty(t *testing.T) {
	got, err := LoadPluginStates(t.TempDir())
	if err != nil {
		t.Fatalf("a missing state file is not an error: %v", err)
	}
	if len(got.Plugins) != 0 {
		t.Errorf("got %d entries, want none", len(got.Plugins))
	}
}

func TestPluginStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := PluginState{
		Source:            PluginSourceMarketplace,
		MarketplaceServer: "https://buildmax.example.com",
		CatalogID:         "pl_00000000000000000000",
		ReleaseVersion:    "1.2.0",
		Digest:            "sha256:abc",
		InstalledAt:       time.Now().Unix(),
	}
	if err := UpdatePluginStates(dir, func(s *PluginStates) error {
		s.Set("code-review", want)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	got, err := LoadPluginStates(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != PluginStateVersion {
		t.Errorf("Version = %d, want %d", got.Version, PluginStateVersion)
	}
	st, ok := got.Get("code-review")
	if !ok {
		t.Fatal("entry not found")
	}
	if st != want {
		t.Errorf("round trip changed the record:\n got %+v\nwant %+v", st, want)
	}

	// Persisted JSON uses snake_case keys.
	raw, err := os.ReadFile(filepath.Join(dir, PluginStateFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"release_version"`, `"marketplace_server"`, `"installed_at"`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("state file is missing %s:\n%s", key, raw)
		}
	}
}

func TestUpdatePluginStatesRemove(t *testing.T) {
	dir := t.TempDir()
	if err := UpdatePluginStates(dir, func(s *PluginStates) error {
		s.Set("a", PluginState{Source: PluginSourceLocal})
		s.Set("b", PluginState{Source: PluginSourceLocal})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePluginStates(dir, func(s *PluginStates) error {
		s.Remove("a")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPluginStates(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Get("a"); ok {
		t.Error("removed entry is still present")
	}
	if _, ok := got.Get("b"); !ok {
		t.Error("unrelated entry was lost")
	}
}

// Every writer takes the file whole, so concurrent installs cannot drop each
// other's entry.
func TestUpdatePluginStatesKeepsConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	const writers = 12
	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = UpdatePluginStates(dir, func(s *PluginStates) error {
				s.Set(fmt.Sprintf("plugin-%02d", i), PluginState{Source: PluginSourceLocal})
				return nil
			})
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("writer %d: %v", i, err)
		}
	}
	got, err := LoadPluginStates(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Plugins) != writers {
		t.Errorf("got %d entries, want %d — a write was lost", len(got.Plugins), writers)
	}
}

func TestUpdatePluginStatesRefusesALockedFile(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, pluginStateLockFile)
	if err := os.WriteFile(lock, []byte("999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	restore := shortenPluginLockTimeout(t)
	defer restore()

	err := UpdatePluginStates(dir, func(*PluginStates) error { return nil })
	if err == nil {
		t.Fatal("writing while another process holds the lock should fail")
	}
	if !strings.Contains(err.Error(), pluginStateLockFile) {
		t.Errorf("error should name the lock file: %v", err)
	}
	if _, statErr := os.Stat(lock); statErr != nil {
		t.Error("a lock this writer did not take must not be removed")
	}
}

// A lock older than any real critical section belongs to a process that died
// holding it. Waiting forever on it is the worse failure.
func TestUpdatePluginStatesTakesOverAStaleLock(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, pluginStateLockFile)
	if err := os.WriteFile(lock, []byte("999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * pluginLockStaleAfter)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatal(err)
	}
	restore := shortenPluginLockTimeout(t)
	defer restore()

	if err := UpdatePluginStates(dir, func(s *PluginStates) error {
		s.Set("code-review", PluginState{Source: PluginSourceLocal})
		return nil
	}); err != nil {
		t.Fatalf("a stale lock should be taken over: %v", err)
	}
	if _, err := os.Stat(lock); !errors.Is(err, os.ErrNotExist) {
		t.Error("the lock should be released after the write")
	}
}

func TestUpdatePluginStatesReleasesTheLockOnFailure(t *testing.T) {
	dir := t.TempDir()
	sentinel := errors.New("no thanks")
	if err := UpdatePluginStates(dir, func(*PluginStates) error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want the mutate error", err)
	}
	if _, err := os.Stat(filepath.Join(dir, pluginStateLockFile)); !errors.Is(err, os.ErrNotExist) {
		t.Error("the lock outlived a failed update")
	}
	if _, err := os.Stat(filepath.Join(dir, PluginStateFile)); !errors.Is(err, os.ErrNotExist) {
		t.Error("a refused update must not write the file")
	}
}

func TestUpdatePluginStatesLeavesNoTempFileBehind(t *testing.T) {
	dir := t.TempDir()
	if err := UpdatePluginStates(dir, func(s *PluginStates) error {
		s.Set("code-review", PluginState{Source: PluginSourceLocal})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != PluginStateFile {
		t.Errorf("plugins directory should hold only the state file: %v", names(entries))
	}
}

func TestUpdatePluginStatesKeepsTheOldFileWhenReplaceFails(t *testing.T) {
	dir := t.TempDir()
	if err := UpdatePluginStates(dir, func(s *PluginStates) error {
		s.Set("keep-me", PluginState{Source: PluginSourceLocal})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(dir, PluginStateFile))
	if err != nil {
		t.Fatal(err)
	}

	saved := renamePluginStateFile
	renamePluginStateFile = func(string, string) error { return errors.New("disk full") }
	defer func() { renamePluginStateFile = saved }()

	if err := UpdatePluginStates(dir, func(s *PluginStates) error {
		s.Set("new", PluginState{Source: PluginSourceLocal})
		return nil
	}); err == nil {
		t.Fatal("a failed replace should be reported")
	}

	after, err := os.ReadFile(filepath.Join(dir, PluginStateFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Error("a failed replace must leave the previous state intact")
	}
	var parsed PluginStates
	if err := json.Unmarshal(after, &parsed); err != nil {
		t.Errorf("state file is no longer valid JSON: %v", err)
	}
}

func shortenPluginLockTimeout(t *testing.T) func() {
	t.Helper()
	savedTimeout, savedDelay := pluginLockTimeout, pluginLockRetryDelay
	pluginLockTimeout, pluginLockRetryDelay = 20*time.Millisecond, time.Millisecond
	return func() { pluginLockTimeout, pluginLockRetryDelay = savedTimeout, savedDelay }
}

func names(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}
