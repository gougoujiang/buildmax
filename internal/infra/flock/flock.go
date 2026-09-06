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
	"io"
	"os"
	"path/filepath"
)

// ErrHeld is returned by TryAcquire when another live process holds the lock.
var ErrHeld = errors.New("lock is held by another process")

// holderOffset is where the holder line begins. Byte 0 is reserved for the
// lock so the line stays readable while the lock is held: Windows locks are
// mandatory, and a lock over the holder bytes would deny the one read the line
// exists for. The lock takes byte 0, the readable line follows it, and the file
// is grown to cover byte 0 before locking because a Windows lock past
// end-of-file is not enforced against other processes.
const holderOffset = 1

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
	// The lock byte must exist before it can be locked: a Windows lock past the
	// end of file is not enforced against another process. Write the reserved
	// byte 0 to cover it. This never shrinks the file, so a concurrent acquirer
	// cannot cut a holder line the eventual winner is writing; on Windows the
	// winner's mandatory lock simply denies this write, which is fine — the lock
	// attempt just below reports the contention.
	_, _ = f.WriteAt([]byte{0}, 0)
	if err := tryLock(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	// Write the holder line past the lock byte, then cut any longer line a
	// previous holder left so its tail cannot be read back as ours. Only the
	// lock holder writes here, so this needs no further guarding.
	if _, err := f.WriteAt(holder, holderOffset); err == nil {
		_ = f.Truncate(holderOffset + int64(len(holder)))
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
// does not exist. It is for messages, never for deciding whether the lock is
// held: read a stale line and you are back to guessing.
//
// The read starts at holderOffset so it never touches the locked byte, which a
// mandatory Windows lock would refuse to serve while the lock is held. That is
// the whole reason the holder line is stored past that byte.
func Holder(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Seek(holderOffset, io.SeekStart); err != nil {
		return nil
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return nil
	}
	return b
}
