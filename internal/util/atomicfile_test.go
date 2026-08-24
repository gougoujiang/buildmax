package util

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// leftoverTemps reports the temp files WriteFileAtomic failed to clean up. The
// helper hides nothing else in dir, so any dotfile beside the target is a leak.
func leftoverTemps(t *testing.T, dir, target string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var out []string
	for _, e := range entries {
		if e.Name() != target && strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestWriteFileAtomicCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")

	if err := WriteFileAtomic(path, []byte("payload"), 0644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("content = %q, want %q", got, "payload")
	}
	if leaked := leftoverTemps(t, dir, "session.json"); leaked != nil {
		t.Errorf("temp files left behind: %v", leaked)
	}
}

func TestWriteFileAtomicAppliesPerm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not carry Unix permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")

	// CreateTemp makes the temp file 0600, so a published 0644 proves the mode
	// comes from the caller rather than leaking from the temp file.
	if err := WriteFileAtomic(path, []byte("payload"), 0644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0644 {
		t.Errorf("mode = %o, want %o", got, 0644)
	}
}

func TestWriteFileAtomicReplacesShorterContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")
	if err := os.WriteFile(path, []byte("a much longer previous document"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := WriteFileAtomic(path, []byte("short"), 0644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	// A rename replaces the file wholesale, so no tail of the longer previous
	// document may survive.
	if string(got) != "short" {
		t.Errorf("content = %q, want %q", got, "short")
	}
}

func TestWriteFileAtomicCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "session.json")

	if err := WriteFileAtomic(path, []byte("payload"), 0644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat: %v", err)
	}
}

func TestWriteFileAtomicKeepsPreviousFileWhenReplaceFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")
	if err := os.WriteFile(path, []byte("previous"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	failure := errors.New("replace failed")
	restore := renameFile
	renameFile = func(string, string) error { return failure }
	defer func() { renameFile = restore }()

	err := WriteFileAtomic(path, []byte("replacement"), 0644)
	if !errors.Is(err, failure) {
		t.Fatalf("err = %v, want %v", err, failure)
	}

	// The point of the helper: a failed write leaves the earlier document
	// complete rather than truncated.
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "previous" {
		t.Errorf("content = %q, want the previous document intact", got)
	}
	if leaked := leftoverTemps(t, dir, "session.json"); leaked != nil {
		t.Errorf("temp files left behind after failure: %v", leaked)
	}
}

func TestWriteFileAtomicRetriesTransientPermissionError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")

	restore := renameFile
	calls := 0
	renameFile = func(from, to string) error {
		calls++
		// A Windows reader holding the target makes the first replace fail with
		// a sharing violation that clears on its own.
		if calls < 3 {
			return &os.LinkError{Op: "rename", Old: from, New: to, Err: os.ErrPermission}
		}
		return os.Rename(from, to)
	}
	defer func() { renameFile = restore }()

	if err := WriteFileAtomic(path, []byte("payload"), 0644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	if calls != 3 {
		t.Errorf("rename attempts = %d, want 3", calls)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("content = %q, want %q", got, "payload")
	}
}

func TestWriteFileAtomicGivesUpAfterRepeatedPermissionErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")

	restore := renameFile
	calls := 0
	renameFile = func(from, to string) error {
		calls++
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: os.ErrPermission}
	}
	defer func() { renameFile = restore }()

	err := WriteFileAtomic(path, []byte("payload"), 0644)
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("err = %v, want a permission error", err)
	}
	// A permanent permission problem must surface rather than be retried
	// forever.
	if calls != atomicRenameAttempts {
		t.Errorf("rename attempts = %d, want %d", calls, atomicRenameAttempts)
	}
	if leaked := leftoverTemps(t, dir, "session.json"); leaked != nil {
		t.Errorf("temp files left behind after failure: %v", leaked)
	}
}
