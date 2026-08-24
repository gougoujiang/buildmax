//go:build unix

package sessionstore

import (
	"os"

	"golang.org/x/sys/unix"
)

// errWouldBlock is what lockFile returns when another owner holds the lock.
var errWouldBlock = unix.EWOULDBLOCK

// lockFile takes an exclusive advisory lock without waiting.
//
// flock is used rather than fcntl record locks because a session's journal is
// read by other processes while a writer holds it: fcntl locks are dropped as
// soon as the process closes any descriptor for the file, which turns an
// unrelated read into a silent loss of exclusivity.
func lockFile(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}

func unlockFile(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN)
}
