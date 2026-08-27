// Package flock is an advisory file lock that the operating system releases
// when the holding process exits.
//
// It exists so occupancy can be a fact rather than a heuristic: a recorded
// process ID has to be checked for liveness, defended against reuse, and
// cleaned up after a crash, and every one of those is a way to leave a
// resource locked by a process that is gone. A kernel lock answers "is anyone
// there" by construction. See docs/design/workspace-root-and-worktrees.md D10.
package flock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrHeld is returned by TryAcquire when another live process holds the lock.
var ErrHeld = errors.New("lock is held by another process")

// Lock is a held advisory lock. Release, or let the process exit.
type Lock struct {
	file *os.File
}

// TryAcquire takes the lock at path without blocking, writing holder into the
// file so a refusal can name who is there. It returns ErrHeld when another
// live process holds it; a lock whose holder has exited is free, with no
// staleness rule to get wrong.
//
// holder is descriptive only. Nothing reads it to decide whether the lock is
// held — the kernel answers that.
func TryAcquire(path string, holder []byte) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("flock: create directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("flock: open: %w", err)
	}
	if err := tryLock(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	// Truncate before writing: the previous holder's line is not ours to keep,
	// and a shorter one would otherwise leave its tail behind.
	if err := f.Truncate(0); err == nil {
		_, _ = f.WriteAt(holder, 0)
		_ = f.Sync()
	}
	return &Lock{file: f}, nil
}

// Release drops the lock. Safe on nil and safe to call twice.
func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	f := l.file
	l.file = nil
	// Closing the descriptor releases the lock on both platforms; the file
	// itself stays so the next holder can overwrite it.
	return f.Close()
}

// Holder returns what the current holder wrote, or empty when the lock file
// does not exist. Readable while the lock is held on every platform: see
// tryLock's byte-range choice on Windows, whose locks are mandatory. It is for messages, never for deciding whether the lock is
// held: read a stale line and you are back to guessing.
func Holder(path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return b
}
