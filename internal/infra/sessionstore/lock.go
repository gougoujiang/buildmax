// Package sessionstore is the file backend for local sessions: the JSONL
// journal codec, the single-writer lock, and tail repair.
//
// It owns physical durability only. What the records mean, how a branch reduces,
// and when state may be committed all live above it — see
// docs/design/local-session-storage.md §14.
package sessionstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrLocked reports that another process owns the session's writer lock.
var ErrLocked = errors.New("session is open in another process")

// WriterLock is exclusive ownership of one session's mutable state.
//
// The OS lock decides ownership. The file's contents are diagnostics for a
// person looking at a stuck session and are never consulted to decide whether
// the lock is held: a recorded PID cannot say whether that process is still
// alive on this machine, and treating it as if it could is what leaves a
// session permanently busy after a crash. A kernel lock is released when its
// owner exits, however it exits.
type WriterLock struct {
	file *os.File
}

// lockOwner is what a WriterLock writes into the file it holds. Every field is
// advisory; see WriterLock.
type lockOwner struct {
	PID        int       `json:"pid"`
	Host       string    `json:"host,omitempty"`
	AcquiredAt time.Time `json:"acquired_at"`
}

// AcquireWriter takes the exclusive lock at path, creating the file if needed.
// It does not wait: a caller that finds a session busy should say so rather
// than block a person behind another window's turn.
func AcquireWriter(path string) (*WriterLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	// No O_TRUNC: the file must survive an attempt that fails to take the lock,
	// so a losing caller cannot erase the winner's diagnostics.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		if errors.Is(err, errWouldBlock) {
			return nil, fmt.Errorf("%w: %s", ErrLocked, path)
		}
		// A filesystem that cannot lock is reported rather than treated as an
		// empty session: pretending exclusivity we do not have is worse than
		// refusing to open.
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	lock := &WriterLock{file: f}
	lock.writeOwner()
	return lock, nil
}

// writeOwner records who holds the lock. A failure is ignored on purpose: these
// bytes help a person diagnose a stuck session and hold no authority, so losing
// them must not stop a session from opening.
func (l *WriterLock) writeOwner() {
	host, _ := os.Hostname()
	data, err := json.Marshal(lockOwner{PID: os.Getpid(), Host: host, AcquiredAt: time.Now().UTC()})
	if err != nil {
		return
	}
	if err := l.file.Truncate(0); err != nil {
		return
	}
	if _, err := l.file.WriteAt(append(data, '\n'), 0); err != nil {
		return
	}
}

// Release drops the lock. Closing the file releases it on every platform here,
// so an unlock failure still ends with the descriptor closed.
func (l *WriterLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := unlockFile(l.file)
	closeErr := l.file.Close()
	l.file = nil
	if err != nil {
		return err
	}
	return closeErr
}
