package sessionstore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func lockPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "writer.lock")
}

func TestAcquireWriterTakesAndReleasesTheLock(t *testing.T) {
	path := lockPath(t)
	first, err := AcquireWriter(path)
	if err != nil {
		t.Fatalf("AcquireWriter: %v", err)
	}
	// Two separate opens of the same file are two lock owners on every platform
	// here, so a second attempt must fail while the first holds it.
	if _, err := AcquireWriter(path); !errors.Is(err, ErrLocked) {
		_ = first.Release()
		t.Fatalf("second acquire err = %v, want ErrLocked", err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	second, err := AcquireWriter(path)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	if err := second.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

func TestAcquireWriterCreatesTheDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "writer.lock")
	lock, err := AcquireWriter(path)
	if err != nil {
		t.Fatalf("AcquireWriter: %v", err)
	}
	defer func() { _ = lock.Release() }()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat: %v", err)
	}
}

func TestAcquireWriterRecordsOwnerDiagnostics(t *testing.T) {
	path := lockPath(t)
	lock, err := AcquireWriter(path)
	if err != nil {
		t.Fatalf("AcquireWriter: %v", err)
	}
	defer func() { _ = lock.Release() }()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var owner lockOwner
	if err := json.Unmarshal(data, &owner); err != nil {
		t.Fatalf("owner is not readable json: %v (%s)", err, data)
	}
	if owner.PID != os.Getpid() {
		t.Errorf("pid = %d, want %d", owner.PID, os.Getpid())
	}
	if owner.AcquiredAt.IsZero() {
		t.Error("acquired_at not recorded")
	}
}

func TestStaleOwnerFileDoesNotBlockAcquisition(t *testing.T) {
	// The OS lock decides ownership. A file left behind by a process that died
	// — naming a pid that may since belong to something else entirely — must
	// not keep a session busy, which is the whole reason ownership is not
	// decided from these bytes.
	path := lockPath(t)
	stale := `{"pid":999999,"host":"gone","acquired_at":"2020-01-01T00:00:00Z"}` + "\n"
	if err := os.WriteFile(path, []byte(stale), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	lock, err := AcquireWriter(path)
	if err != nil {
		t.Fatalf("AcquireWriter over a stale owner file: %v", err)
	}
	defer func() { _ = lock.Release() }()
}

func TestFailedAcquisitionLeavesTheOwnerFileIntact(t *testing.T) {
	path := lockPath(t)
	held, err := AcquireWriter(path)
	if err != nil {
		t.Fatalf("AcquireWriter: %v", err)
	}
	defer func() { _ = held.Release() }()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if _, err := AcquireWriter(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("err = %v, want ErrLocked", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// A losing caller must not erase the winner's diagnostics on its way out.
	if string(after) != string(before) {
		t.Errorf("owner file changed after a failed acquire:\nbefore %s\nafter  %s", before, after)
	}
}

func TestReleaseIsSafeToCallTwice(t *testing.T) {
	path := lockPath(t)
	lock, err := AcquireWriter(path)
	if err != nil {
		t.Fatalf("AcquireWriter: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("first Release: %v", err)
	}
	// Callers release in a defer and sometimes also on the success path; the
	// second call must not report a failure that nobody can act on.
	if err := lock.Release(); err != nil {
		t.Errorf("second Release: %v", err)
	}
	if err := (*WriterLock)(nil).Release(); err != nil {
		t.Errorf("nil Release: %v", err)
	}
}

// TestOwnerFileIsReadableWhileHeld pins what the diagnostics are for. They
// exist so a person can see who holds a stuck session, which means they have to
// be readable at the one moment anybody would look: while the lock is held.
//
// This is not free on every platform. A Windows file lock is mandatory rather
// than advisory, so a locked byte range cannot be read at all — locking the
// start of the file would make the owner blob unreadable exactly when it
// matters. The lock therefore covers a byte the file never uses, and this test
// is what would catch that changing.
func TestOwnerFileIsReadableWhileHeld(t *testing.T) {
	path := lockPath(t)
	lock, err := AcquireWriter(path)
	if err != nil {
		t.Fatalf("AcquireWriter: %v", err)
	}
	defer func() { _ = lock.Release() }()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the owner file is unreadable while the lock is held: %v", err)
	}
	var owner lockOwner
	if err := json.Unmarshal(data, &owner); err != nil {
		t.Fatalf("owner diagnostics are not decodable: %v (%s)", err, data)
	}
	if owner.PID != os.Getpid() {
		t.Errorf("pid = %d, want %d", owner.PID, os.Getpid())
	}
}
