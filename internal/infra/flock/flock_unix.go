//go:build !windows

package flock

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// tryLock takes an exclusive advisory lock without blocking. The kernel drops
// it when the descriptor closes, process exit included.
func tryLock(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == nil {
		return nil
	}
	if errors.Is(err, unix.EWOULDBLOCK) {
		return ErrHeld
	}
	return fmt.Errorf("flock: lock: %w", err)
}
