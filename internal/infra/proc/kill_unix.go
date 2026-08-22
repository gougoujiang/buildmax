//go:build unix

package proc

import (
	"os"
	"syscall"
)

// sysProcAttr makes the child a process-group leader, so one negative-PID
// signal reaches the whole tree instead of only the shell.
func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// treeKiller signals the child's process group. Descendants that call
// setsid/setpgid escape it; that is the known limit of group-based
// termination, accepted until evidence demands cgroup-style tracking.
type treeKiller struct {
	pgid int
}

func newTreeKiller(p *os.Process) (*treeKiller, error) {
	return &treeKiller{pgid: p.Pid}, nil
}

func (k *treeKiller) signal(graceful bool) error {
	sig := syscall.SIGKILL
	if graceful {
		sig = syscall.SIGTERM
	}
	err := syscall.Kill(-k.pgid, sig)
	if err == syscall.ESRCH {
		return nil
	}
	return err
}

func (k *treeKiller) close() {}
