package scheduler

import (
	"os/exec"
	"time"
)

// stopWorkerPolitely has nothing polite to send on Windows, which has no
// signal a console process can be asked to shut down with — the same reason
// internal/infra/proc/kill_windows.go gives for treating graceful and forceful
// the same there.
//
// So a cancelled dispatch kills the worker, and the run it was executing is
// closed by the stale-run reaper rather than reporting for itself. Local
// process mode on Windows is a development shape; the reference deployment
// dispatches Kubernetes Jobs, whose pods do get SIGTERM.
func stopWorkerPolitely(cmd *exec.Cmd, grace time.Duration) {
	cmd.WaitDelay = grace
}
