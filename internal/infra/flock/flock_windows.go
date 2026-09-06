//go:build windows

package flock

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// tryLock takes an exclusive lock on the file's first byte without blocking.
// Windows releases it when the handle closes, process exit included, which is
// the property this package is for.
//
// One byte, not the whole file: Windows locks are mandatory, so locking the
// bytes the holder line occupies would deny every reader — including the
// refusal that wants to name who is there. The holder line is written past this
// byte instead; see holderOffset. Byte 0 rather than a byte beyond the content:
// a lock past end-of-file is not enforced against other processes, so it could
// be granted twice. TryAcquire grows the file to cover byte 0 before locking.
func tryLock(f *os.File) error {
	var overlapped windows.Overlapped // byte 0
	err := windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1, 0,
		&overlapped,
	)
	if err == nil {
		return nil
	}
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING) {
		return ErrHeld
	}
	return fmt.Errorf("flock: lock: %w", err)
}
