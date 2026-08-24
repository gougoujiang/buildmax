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

// The locked region is one byte at a very high offset, past anything the file
// will ever contain.
//
// It matters which byte. A Windows file lock is mandatory, not advisory like
// flock: a locked range cannot be read by anyone, including a person or a test
// trying to see who holds the session. Locking the start of the file would
// therefore make the owner diagnostics unreadable exactly while the lock is
// held — which is the only time anybody wants to read them. Locking a byte that
// holds no data keeps ownership exclusive and the contents legible.
const (
	lockOffsetHigh = 0x40000000 // 1<<62, far beyond any diagnostics blob
	lockBytes      = 1
)

func lockRegion() windows.Overlapped {
	return windows.Overlapped{OffsetHigh: lockOffsetHigh}
}

func lockFile(f *os.File) error {
	overlapped := lockRegion()
	err := windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, lockBytes, 0, &overlapped,
	)
	if err == windows.ERROR_IO_PENDING || err == windows.ERROR_LOCK_VIOLATION {
		return errWouldBlock
	}
	return err
}

func unlockFile(f *os.File) error {
	overlapped := lockRegion()
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, lockBytes, 0, &overlapped)
}
