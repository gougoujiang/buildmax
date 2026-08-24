//go:build windows

package sessionstore

import (
	"os"

	"golang.org/x/sys/windows"
)

// errWouldBlock is what lockFile returns when another owner holds the lock.
// Windows reports a failed immediate lock as a violation rather than as a
// would-block, so the two are mapped onto one sentinel for the caller.
var errWouldBlock = windows.ERROR_LOCK_VIOLATION

// lockByteRange is the region locked. The lock is on the range, not on the
// bytes, so locking one byte of a file whose contents are pure diagnostics is
// enough to make ownership exclusive.
const lockByteRange = 1

func lockFile(f *os.File) error {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, lockByteRange, 0, &overlapped,
	)
	if err == windows.ERROR_IO_PENDING || err == windows.ERROR_LOCK_VIOLATION {
		return errWouldBlock
	}
	return err
}

func unlockFile(f *os.File) error {
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, lockByteRange, 0, &overlapped)
}
