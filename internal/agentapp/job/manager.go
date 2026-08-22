// Package job owns the local background jobs of one AgentApp: identity,
// state, bounded output, stop, lifecycle events, and shutdown. Jobs are
// process-scoped but session-owned; closing the manager stops everything it
// started. Process behavior lives in internal/infra/proc, and LLM-facing
// validation stays in internal/tool — this package coordinates. Mirrors the
// design in docs/design/local-background-jobs.md.
package job

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gougoujiang/buildmax/internal/infra/proc"
	"github.com/gougoujiang/buildmax/internal/util"
)

// Kind says what a job runs. The monitor kind arrives with its own delivery
// stage.
type Kind string

const (
	KindCommand  Kind = "command"
	KindSubagent Kind = "subagent"
)

// State is the job state machine:
// starting → running → succeeded | failed | canceled.
type State string

const (
	StateStarting  State = "starting"
	StateRunning   State = "running"
	StateSucceeded State = "succeeded"
	StateFailed    State = "failed"
	StateCanceled  State = "canceled"
)

// StopReason says why a canceled job ended. It is a field rather than extra
// terminal states so the state machine stays minimal.
type StopReason string

const (
	StopUser     StopReason = "user_stop"
	StopShutdown StopReason = "shutdown"
	StopTimeout  StopReason = "timeout"
)

// Provenance is captured immutably at launch. The sandbox fact is recorded
// here so a settings reload cannot silently relabel a running job's boundary.
type Provenance struct {
	Workspace        string
	SessionID        string
	ParentTraceID    string
	ParentToolCallID string
	Sandboxed        bool
}

// CommandSpec describes one background command. Name and Args are the final
// argv after permission, sandbox wrapping, and environment resolution —
// launching is not a second, weaker shell path around Bash.
type CommandSpec struct {
	// Command is the user-visible command string that was approved.
	Command string
	Name    string
	Args    []string
	Dir     string
	Env     []string
	Timeout time.Duration
}

// Job is an immutable snapshot of one job's identity and state.
type Job struct {
	ID         string
	Kind       Kind
	Command    string
	State      State
	StopReason StopReason
	// ExitCode is meaningful only in StateFailed after a non-zero exit.
	ExitCode   int
	Err        string
	PID        int
	Provenance Provenance
	CreatedAt  time.Time
	EndedAt    time.Time
}

// Running reports whether the job has not reached a terminal state.
func (j Job) Running() bool {
	return j.State == StateStarting || j.State == StateRunning
}

// Event is one lifecycle notification carrying the job snapshot after the
// transition.
type Event struct {
	Job Job
}

// DefaultMaxCommandJobs bounds concurrently running command jobs. Deliberately
// conservative until resource evidence supports more.
const DefaultMaxCommandJobs = 8

// DefaultMaxSubagentJobs bounds concurrently running background subagents.
// Each one is a full agent loop holding model calls; lower than commands.
const DefaultMaxSubagentJobs = 4

// eventBuffer is each subscriber's channel capacity. Lifecycle events are
// low-volume; a subscriber that falls this far behind loses the oldest
// notifications and re-reads state from List instead.
const eventBuffer = 16

var (
	ErrClosed   = errors.New("job manager is shutting down")
	ErrNotFound = errors.New("no such job")
)

type record struct {
	mu  sync.Mutex
	job Job
	// proc backs command jobs; cancel backs subagent jobs. Exactly one is set.
	proc   *proc.Proc
	cancel context.CancelFunc
	// reply holds a finished subagent job's final reply, read through Output.
	reply []byte
	// requested is the stop reason recorded when Stop was accepted, so the
	// terminal state can say who ended the job — the execution only knows it
	// was signaled.
	requested StopReason
}

func (r *record) snapshot() Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.job
}

// signalStop requests termination of whatever backs this job.
func (r *record) signalStop() {
	if r.proc != nil {
		r.proc.Stop()
		return
	}
	if r.cancel != nil {
		r.cancel()
	}
}

type subscriber struct {
	ch        chan Event
	sessionID string
	once      sync.Once
}

// shut closes the channel exactly once, whether the subscriber cancelled or
// the manager closed first.
func (s *subscriber) shut() { s.once.Do(func() { close(s.ch) }) }

// Manager owns every job of one AgentApp. All methods are safe for
// concurrent use.
type Manager struct {
	mu     sync.Mutex
	jobs   []*record // creation order
	byID   map[string]*record
	subs   map[int]*subscriber
	nextID int
	closed bool

	maxCommands  int
	maxSubagents int
	wg           sync.WaitGroup
}

func NewManager() *Manager {
	return &Manager{
		byID:         make(map[string]*record),
		subs:         make(map[int]*subscriber),
		maxCommands:  DefaultMaxCommandJobs,
		maxSubagents: DefaultMaxSubagentJobs,
	}
}

// runningOfKind counts non-terminal jobs of one kind. Caller holds m.mu.
func (m *Manager) runningOfKind(kind Kind) int {
	n := 0
	for _, r := range m.jobs {
		if snap := r.snapshot(); snap.Kind == kind && snap.Running() {
			n++
		}
	}
	return n
}

// adopt registers a started record, or reports that shutdown began while the
// work was being launched — the caller must then stop what it started.
func (m *Manager) adopt(rec *record) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return false
	}
	m.jobs = append(m.jobs, rec)
	m.byID[rec.job.ID] = rec
	m.wg.Add(1)
	return true
}

// StartCommand launches one background command. The permission gate already
// ran: this either starts the process and returns the running job, or refuses
// with a reason the caller can surface.
func (m *Manager) StartCommand(spec CommandSpec, prov Provenance) (Job, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Job{}, ErrClosed
	}
	if running := m.runningOfKind(KindCommand); running >= m.maxCommands {
		m.mu.Unlock()
		return Job{}, fmt.Errorf("refusing to start: %d background commands already running (limit %d); stop one first", running, m.maxCommands)
	}
	m.mu.Unlock()

	p, err := proc.Start(proc.Spec{
		Name:    spec.Name,
		Args:    spec.Args,
		Dir:     spec.Dir,
		Env:     spec.Env,
		Timeout: spec.Timeout,
	})
	if err != nil {
		return Job{}, fmt.Errorf("start background command: %w", err)
	}

	rec := &record{
		job: Job{
			ID:         util.NewPrefixedID(util.PrefixJob),
			Kind:       KindCommand,
			Command:    spec.Command,
			State:      StateRunning,
			PID:        p.PID(),
			Provenance: prov,
			CreatedAt:  time.Now(),
		},
		proc: p,
	}
	if !m.adopt(rec) {
		// Shutdown began while the process was spawning; do not keep a job
		// nothing will sweep.
		p.Stop()
		<-p.Done()
		return Job{}, ErrClosed
	}

	snap := rec.snapshot()
	m.publish(snap)
	go m.reap(rec)
	return snap, nil
}

// StartSubagent launches one background subagent job. run receives a
// manager-owned context — canceled by Stop, shutdown, or the timeout — and
// returns the subagent's final reply. The caller stamps any provenance values
// run needs onto that context itself; the manager deliberately does not
// inherit the launching request context.
func (m *Manager) StartSubagent(description string, timeout time.Duration, prov Provenance, run func(context.Context) (string, error)) (Job, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Job{}, ErrClosed
	}
	if running := m.runningOfKind(KindSubagent); running >= m.maxSubagents {
		m.mu.Unlock()
		return Job{}, fmt.Errorf("refusing to start: %d background subagents already running (limit %d); wait for one or stop it", running, m.maxSubagents)
	}
	m.mu.Unlock()

	var jobCtx context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		jobCtx, cancel = context.WithTimeout(context.Background(), timeout)
	} else {
		jobCtx, cancel = context.WithCancel(context.Background())
	}

	rec := &record{
		job: Job{
			ID:         util.NewPrefixedID(util.PrefixJob),
			Kind:       KindSubagent,
			Command:    description,
			State:      StateRunning,
			Provenance: prov,
			CreatedAt:  time.Now(),
		},
		cancel: cancel,
	}
	if !m.adopt(rec) {
		cancel()
		return Job{}, ErrClosed
	}

	snap := rec.snapshot()
	m.publish(snap)
	go func() {
		defer m.wg.Done()
		defer cancel()
		reply, err := run(jobCtx)

		rec.mu.Lock()
		rec.job.EndedAt = time.Now()
		switch {
		case jobCtx.Err() == context.DeadlineExceeded && rec.requested == "":
			rec.job.State = StateCanceled
			rec.job.StopReason = StopTimeout
		case jobCtx.Err() != nil:
			rec.job.State = StateCanceled
			rec.job.StopReason = rec.requested
			if rec.job.StopReason == "" {
				rec.job.StopReason = StopUser
			}
		case err != nil:
			rec.job.State = StateFailed
			rec.job.Err = err.Error()
		default:
			rec.job.State = StateSucceeded
			rec.reply = []byte(reply)
		}
		done := rec.job
		rec.mu.Unlock()
		m.publish(done)
	}()
	return snap, nil
}

// reap waits for the process and publishes the terminal transition.
func (m *Manager) reap(rec *record) {
	defer m.wg.Done()
	<-rec.proc.Done()
	res, _ := rec.proc.Result()

	rec.mu.Lock()
	rec.job.EndedAt = time.Now()
	switch res.Reason {
	case proc.ReasonExited:
		switch {
		case res.Err != nil:
			rec.job.State = StateFailed
			rec.job.Err = res.Err.Error()
		case res.ExitCode == 0:
			rec.job.State = StateSucceeded
		default:
			rec.job.State = StateFailed
			rec.job.ExitCode = res.ExitCode
			rec.job.Err = fmt.Sprintf("exit code %d", res.ExitCode)
		}
	case proc.ReasonTimeout:
		rec.job.State = StateCanceled
		rec.job.StopReason = StopTimeout
	case proc.ReasonStopped:
		rec.job.State = StateCanceled
		rec.job.StopReason = rec.requested
		if rec.job.StopReason == "" {
			rec.job.StopReason = StopUser
		}
	}
	snap := rec.job
	rec.mu.Unlock()
	m.publish(snap)
}

// List returns snapshots of every job in creation order.
func (m *Manager) List() []Job {
	m.mu.Lock()
	recs := make([]*record, len(m.jobs))
	copy(recs, m.jobs)
	m.mu.Unlock()
	out := make([]Job, len(recs))
	for i, r := range recs {
		out[i] = r.snapshot()
	}
	return out
}

// Get returns one job snapshot.
func (m *Manager) Get(id string) (Job, bool) {
	m.mu.Lock()
	rec, ok := m.byID[id]
	m.mu.Unlock()
	if !ok {
		return Job{}, false
	}
	return rec.snapshot(), true
}

// Output reads captured output incrementally; see proc.OutputChunk for
// cursor semantics. For a subagent job, stdout is its final reply (available
// once it succeeds) and stderr is empty — failure detail lives on the job's
// Err field.
func (m *Manager) Output(id string, stream proc.Stream, cursor uint64, max int) (proc.OutputChunk, error) {
	m.mu.Lock()
	rec, ok := m.byID[id]
	m.mu.Unlock()
	if !ok {
		return proc.OutputChunk{}, ErrNotFound
	}
	if rec.proc != nil {
		return rec.proc.Output(stream, cursor, max), nil
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	var data []byte
	if stream == proc.Stdout {
		data = rec.reply
	}
	if cursor > uint64(len(data)) {
		cursor = uint64(len(data))
	}
	out := data[cursor:]
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return proc.OutputChunk{
		Data: append([]byte(nil), out...),
		Next: cursor + uint64(len(out)),
	}, nil
}

// Stop requests termination of one job on the user's behalf. Stopping an
// already-finished job is not an error; the caller sees the terminal state.
func (m *Manager) Stop(id string) error {
	return m.stop(id, StopUser)
}

func (m *Manager) stop(id string, reason StopReason) error {
	m.mu.Lock()
	rec, ok := m.byID[id]
	m.mu.Unlock()
	if !ok {
		return ErrNotFound
	}
	rec.mu.Lock()
	if rec.requested == "" {
		rec.requested = reason
	}
	rec.mu.Unlock()
	rec.signalStop()
	return nil
}

// Subscribe delivers lifecycle events for jobs owned by sessionID; empty
// means every session. A slow subscriber loses oldest events rather than
// blocking job progress — state is always recoverable from List. The
// returned cancel is idempotent.
func (m *Manager) Subscribe(sessionID string) (<-chan Event, func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextID
	m.nextID++
	sub := &subscriber{ch: make(chan Event, eventBuffer), sessionID: sessionID}
	m.subs[id] = sub
	cancel := func() {
		m.mu.Lock()
		delete(m.subs, id)
		m.mu.Unlock()
		sub.shut()
	}
	return sub.ch, cancel
}

func (m *Manager) publish(j Job) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, sub := range m.subs {
		if sub.sessionID != "" && sub.sessionID != j.Provenance.SessionID {
			continue
		}
		select {
		case sub.ch <- Event{Job: j}:
		default:
		}
	}
}

// Close stops accepting jobs, requests cancellation everywhere, and waits.
// The processes escalate to a kill on their own grace period; ctx bounds how
// long Close waits for the sweep, and the error names what did not confirm
// exit in time.
func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	recs := make([]*record, len(m.jobs))
	copy(recs, m.jobs)
	m.mu.Unlock()

	for _, rec := range recs {
		if rec.snapshot().Running() {
			rec.mu.Lock()
			if rec.requested == "" {
				rec.requested = StopShutdown
			}
			rec.mu.Unlock()
			rec.signalStop()
		}
	}

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	var err error
	select {
	case <-done:
	case <-ctx.Done():
		var stuck []string
		for _, rec := range recs {
			if rec.snapshot().Running() {
				stuck = append(stuck, rec.snapshot().ID)
			}
		}
		err = fmt.Errorf("job manager shutdown timed out; unconfirmed exits: %v", stuck)
	}
	// Release subscribers so event pumps ranging over the channel exit.
	// Terminal events published during the sweep are buffered and stay
	// readable after close.
	m.mu.Lock()
	subs := make([]*subscriber, 0, len(m.subs))
	for _, s := range m.subs {
		subs = append(subs, s)
	}
	m.subs = make(map[int]*subscriber)
	m.mu.Unlock()
	for _, s := range subs {
		s.shut()
	}
	return err
}
