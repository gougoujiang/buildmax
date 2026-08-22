//go:build unix

package proc

import (
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// These tests probe liveness with unix signal 0, so they cannot run on
// Windows; the Job Object equivalent belongs in Windows CI.

// readGrandchildPID polls stdout for the PID the shell printed.
func readGrandchildPID(t *testing.T, p *Proc) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		line := strings.TrimSpace(string(p.Output(Stdout, 0, 0).Data))
		if line != "" {
			pid, err := strconv.Atoi(strings.Fields(line)[0])
			if err != nil {
				t.Fatalf("bad pid line %q: %v", line, err)
			}
			return pid
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("grandchild pid never appeared on stdout")
	return 0
}

// requireProcessGone polls until signal 0 reports the PID dead.
func requireProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("pid %d still alive", pid)
}

// A stop must take down the whole tree: killing only the shell while its
// child survives is a failed stop, per docs/design/local-background-jobs.md.
func TestStopKillsGrandchild(t *testing.T) {
	p, err := Start(shellSpec("sleep 60 & echo $!; wait"))
	if err != nil {
		t.Fatal(err)
	}
	pid := readGrandchildPID(t, p)
	p.Stop()
	res := waitDone(t, p, 10*time.Second)
	if res.Reason != ReasonStopped {
		t.Fatalf("reason = %q, want stopped", res.Reason)
	}
	requireProcessGone(t, pid)
}

// A naturally exited command must not leave unowned descendants: the reaper
// sweeps what is left of the group.
func TestNaturalExitSweepsGroup(t *testing.T) {
	p, err := Start(shellSpec("sleep 60 >/dev/null 2>&1 & echo $!"))
	if err != nil {
		t.Fatal(err)
	}
	pid := readGrandchildPID(t, p)
	res := waitDone(t, p, 10*time.Second)
	if res.Reason != ReasonExited || res.ExitCode != 0 {
		t.Fatalf("unexpected result: %+v", res)
	}
	requireProcessGone(t, pid)
}
