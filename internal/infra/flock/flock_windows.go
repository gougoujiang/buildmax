//go:build windows

package flock

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// lockByteOffset is the byte the lock is taken on, chosen far beyond any
// holder line the file will ever hold.
//
// Windows file locks are mandatory: a lock over the bytes the holder line
// occupies would stop anyone reading who is there, which is the one thing that
// line exists for. Locking a byte nothing is written to leaves the content
// readable and still lets exactly one holder win.
const lockByteOffset uint64 = 1 << 40

// tryLock takes an exclusive lock on one byte without blocking. Windows
// releases it when the handle closes, process exit included, which is the
// property this package is for.
//
// One byte, not the whole file: Windows locks are mandatory, so locking the
// bytes the holder line occupies would deny every reader — including the
// refusal that wants to name who is there. See lockByteOffset.
func tryLock(f *os.File) error {
	const offset = lockByteOffset
	overlapped := windows.Overlapped{
		Offset:     uint32(offset & 0xFFFFFFFF),
		OffsetHigh: uint32(offset >> 32),
	}
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
