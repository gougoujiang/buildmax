// Package proc starts and supervises the OS processes behind local background
// jobs: process-group spawning, bounded output capture, and process-tree
// termination. It knows nothing about jobs, permissions, or sandboxing —
// callers pass the argv and environment they already resolved through the
// normal tool policy. Mirrors the design in
// docs/design/local-background-jobs.md.
package proc

import (
	"errors"
	"os/exec"
	"sync"
	"time"
)

const (
	// DefaultOutputLimit bounds each stream's retained output. Jobs keep a
	// recent window, not a transcript; the full stream never enters memory.
	DefaultOutputLimit = 256 * 1024
	// DefaultStopGrace is how long a termination request stays polite
	// before the whole process group is killed.
	DefaultStopGrace = 5 * time.Second
	// pipeGrace bounds how long Wait may block on output pipes held open by
	// surviving descendants after the direct child exited (exec.Cmd.WaitDelay).
	// Without it, `sh -c 'server &'` would keep the job "running" for as long
	// as the orphan holds stdout.
	pipeGrace = 5 * time.Second
)

// Reason says why a process reached its terminal state. The first cause wins:
// a stop request or timeout recorded before exit labels the result even if the
// process was already dying on its own.
type Reason string

const (
	ReasonExited  Reason = "exited"
	ReasonTimeout Reason = "timeout"
	ReasonStopped Reason = "stopped"
)

// Result is the terminal outcome of a supervised process.
type Result struct {
	Reason Reason
	// ExitCode is meaningful only when Reason is ReasonExited.
	ExitCode int
	// Err carries a wait failure other than a non-zero exit, if any.
	Err error
}

// Spec describes one process to supervise. Name and Args are the final argv:
// no shell interpretation, sandbox wrapping, or env scrubbing happens here.
type Spec struct {
	Name string
	Args []string
	Dir  string
	// Env is the child environment; nil inherits the parent's.
	Env []string
	// Timeout terminates the process tree when exceeded; 0 means none.
	Timeout time.Duration
	// OutputLimit is the per-stream retained-output bound in bytes;
	// 0 means DefaultOutputLimit.
	OutputLimit int
	// StopGrace is the TERM-to-KILL escalation window; 0 means DefaultStopGrace.
	StopGrace time.Duration
}

// Stream selects which captured output to read.
type Stream int

const (
	Stdout Stream = iota
	Stderr
)

// Proc is one supervised process. All methods are safe for concurrent use.
type Proc struct {
	cmd    *exec.Cmd
	killer *treeKiller
	stdout *ring
	stderr *ring
	grace  time.Duration
	timer  *time.Timer
	done   chan struct{}

	mu         sync.Mutex
	stopReason Reason // set by the first terminate call; empty until then
	finished   bool
	result     Result
}

// Start launches the process in its own process group (Job Object on Windows)
// and begins capturing output. A returned error means nothing is running.
func Start(spec Spec) (*Proc, error) {
	if spec.Name == "" {
		return nil, errors.New("proc: empty command name")
	}
	limit := spec.OutputLimit
	if limit <= 0 {
		limit = DefaultOutputLimit
	}
	grace := spec.StopGrace
	if grace <= 0 {
		grace = DefaultStopGrace
	}

	p := &Proc{
		stdout: newRing(limit),
		stderr: newRing(limit),
		grace:  grace,
		done:   make(chan struct{}),
	}
	cmd := exec.Command(spec.Name, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env
	cmd.Stdout = p.stdout
	cmd.Stderr = p.stderr
	cmd.SysProcAttr = sysProcAttr()
	cmd.WaitDelay = pipeGrace
	p.cmd = cmd

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	killer, err := newTreeKiller(cmd.Process)
	if err != nil {
		// Without a killer the tree cannot be stopped, which breaks the
		// package's one promise. Take the direct child down and refuse.
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}
	p.killer = killer

	if spec.Timeout > 0 {
		p.timer = time.AfterFunc(spec.Timeout, func() { p.terminate(ReasonTimeout) })
	}
	go p.reap()
	return p, nil
}

// PID returns the direct child's process ID, for display only.
func (p *Proc) PID() int { return p.cmd.Process.Pid }

// Done is closed when the process tree is finished and the result is set.
func (p *Proc) Done() <-chan struct{} { return p.done }

// Result returns the terminal outcome; ok is false while still running.
func (p *Proc) Result() (Result, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.result, p.finished
}

// Output reads captured output from cursor. Use cursor 0 for the oldest
// retained bytes; pass the returned Next to continue. max <= 0 reads
// everything available.
func (p *Proc) Output(s Stream, cursor uint64, max int) OutputChunk {
	if s == Stderr {
		return p.stderr.read(cursor, max)
	}
	return p.stdout.read(cursor, max)
}

// Stop requests termination of the whole process tree: polite first, then a
// kill after the grace period. It returns immediately; wait on Done.
// Idempotent, and a no-op once the process finished.
func (p *Proc) Stop() { p.terminate(ReasonStopped) }

// terminate records the first cause and starts the polite-then-kill sequence.
func (p *Proc) terminate(reason Reason) {
	p.mu.Lock()
	if p.finished || p.stopReason != "" {
		p.mu.Unlock()
		return
	}
	p.stopReason = reason
	p.mu.Unlock()

	_ = p.killer.signal(true)
	go func() {
		select {
		case <-p.done:
		case <-time.After(p.grace):
			_ = p.killer.signal(false)
		}
	}()
}

// reap waits for the process, sweeps whatever is left of its group, and
// publishes the result. The sweep also covers a naturally exited command
// whose descendants would otherwise survive unowned.
func (p *Proc) reap() {
	waitErr := p.cmd.Wait()
	if p.timer != nil {
		p.timer.Stop()
	}
	_ = p.killer.signal(false)
	p.killer.close()

	p.mu.Lock()
	reason := p.stopReason
	if reason == "" {
		reason = ReasonExited
	}
	res := Result{Reason: reason}
	if reason == ReasonExited {
		res.ExitCode = p.cmd.ProcessState.ExitCode()
		var exitErr *exec.ExitError
		if waitErr != nil && !errors.As(waitErr, &exitErr) && !errors.Is(waitErr, exec.ErrWaitDelay) {
			res.Err = waitErr
		}
	}
	p.result = res
	p.finished = true
	p.mu.Unlock()
	close(p.done)
}
