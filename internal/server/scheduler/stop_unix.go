//go:build !windows

package scheduler

import (
	"os/exec"
	"syscall"
	"time"
)

// stopWorkerPolitely makes a cancelled dispatch ask the worker to stop instead
// of killing it, and kills it if it does not stop within grace.
//
// The default for a cancelled exec.Cmd is Kill, which would take the run's
// output, its artifacts, and its outcome with it — everything the worker's own
// signal handling exists to save.
func stopWorkerPolitely(cmd *exec.Cmd, grace time.Duration) {
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = grace
}
