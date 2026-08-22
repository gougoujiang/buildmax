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
	"strconv"
	"strings"
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
	KindMonitor  Kind = "monitor"
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
	// Deliver requests that completion wake the owning session with the
	// result. Default false: completion is only notified and queryable, per
	// docs/design/local-background-jobs.md ("When completion wakes the
	// model").
	Deliver bool
}

// SubagentSpec describes one background subagent job.
type SubagentSpec struct {
	Description string
	// Timeout cancels the subagent when exceeded; 0 means none.
	Timeout time.Duration
	// Deliver requests that the final reply wake the owning session.
	Deliver bool
}

// MonitorSpec describes one monitor job: a command whose stdout lines become
// events until exit, timeout, stop, or shutdown. Name and Args are the final
// argv after the same permission, sandbox, and environment resolution a Bash
// call gets.
type MonitorSpec struct {
	Command string
	Name    string
	Args    []string
	Dir     string
	Env     []string
	Timeout time.Duration
	// Persistent documents application-session lifetime intent — never
	// survival past process exit.
	Persistent bool
	// React makes each delivered line wake the owning session (the job's
	// Deliver flag). Default is notify-only: lines reach the UI and are
	// queryable, but cause no model call.
	React bool
}

// Job is an immutable snapshot of one job's identity and state.
type Job struct {
	ID         string
	Kind       Kind
	Command    string
	State      State
	StopReason StopReason
	// Deliver says the launching call asked for completion to wake the
	// owning session with the result; for a monitor, that its lines react.
	Deliver bool
	// Persistent records a monitor's application-session-lifetime intent.
	Persistent bool
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

// EventType distinguishes what an Event reports. The zero value is a
// lifecycle transition, so pre-monitor consumers keep working unchanged.
type EventType string

const (
	EventLifecycle   EventType = ""
	EventMonitorLine EventType = "monitor_line"
)

// Event is one notification carrying the job snapshot at that moment. A
// monitor line additionally carries the line and the count of lines that
// backpressure dropped since the previous delivered event.
type Event struct {
	Job          Job
	Type         EventType
	Line         string
	DroppedLines int
}

// DefaultMaxCommandJobs bounds concurrently running command jobs. Deliberately
// conservative until resource evidence supports more.
const DefaultMaxCommandJobs = 8

// DefaultMaxSubagentJobs bounds concurrently running background subagents.
// Each one is a full agent loop holding model calls; lower than commands.
const DefaultMaxSubagentJobs = 4

// DefaultMaxMonitors bounds concurrently running monitors.
const DefaultMaxMonitors = 4

// Monitor backpressure. Without these bounds, `tail -F` on a busy log is a
// context-exhaustion tool: the caps are what make a monitor event safe to
// deliver anywhere.
const (
	// monitorPollInterval is how often the line pump drains the stdout ring.
	monitorPollInterval = 200 * time.Millisecond
	// maxMonitorLineBytes truncates one line before it becomes an event.
	maxMonitorLineBytes = 2048
	// maxMonitorEventsPerSecond rate-limits delivered lines; the rest are
	// coalesced into the next event's DroppedLines count.
	maxMonitorEventsPerSecond = 5
)

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
	// linesDone, when set (monitor jobs), is closed by the line pump after
	// its final flush, so the terminal lifecycle event orders after the last
	// line event.
	linesDone chan struct{}
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
	maxMonitors  int
	wg           sync.WaitGroup
}

func NewManager() *Manager {
	return &Manager{
		byID:         make(map[string]*record),
		subs:         make(map[int]*subscriber),
		maxCommands:  DefaultMaxCommandJobs,
		maxSubagents: DefaultMaxSubagentJobs,
		maxMonitors:  DefaultMaxMonitors,
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
			ID:         newJobID(),
			Kind:       KindCommand,
			Command:    spec.Command,
			State:      StateRunning,
			Deliver:    spec.Deliver,
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
	m.publish(Event{Job: snap})
	go m.reap(rec)
	return snap, nil
}

// StartSubagent launches one background subagent job. run receives a
// manager-owned context — canceled by Stop, shutdown, or the timeout — and
// returns the subagent's final reply. The caller stamps any provenance values
// run needs onto that context itself; the manager deliberately does not
// inherit the launching request context.
func (m *Manager) StartSubagent(spec SubagentSpec, prov Provenance, run func(context.Context) (string, error)) (Job, error) {
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
	if spec.Timeout > 0 {
		jobCtx, cancel = context.WithTimeout(context.Background(), spec.Timeout)
	} else {
		jobCtx, cancel = context.WithCancel(context.Background())
	}

	rec := &record{
		job: Job{
			ID:         newJobID(),
			Kind:       KindSubagent,
			Command:    spec.Description,
			State:      StateRunning,
			Deliver:    spec.Deliver,
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
	m.publish(Event{Job: snap})
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
		m.publish(Event{Job: done})
	}()
	return snap, nil
}

// StartMonitor launches one monitor job. Lifecycle is a command job's; in
// addition, a line pump turns stdout lines into bounded, rate-limited
// monitor events until the process ends.
func (m *Manager) StartMonitor(spec MonitorSpec, prov Provenance) (Job, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Job{}, ErrClosed
	}
	if running := m.runningOfKind(KindMonitor); running >= m.maxMonitors {
		m.mu.Unlock()
		return Job{}, fmt.Errorf("refusing to start: %d monitors already running (limit %d); stop one first", running, m.maxMonitors)
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
		return Job{}, fmt.Errorf("start monitor: %w", err)
	}

	rec := &record{
		job: Job{
			ID:         newJobID(),
			Kind:       KindMonitor,
			Command:    spec.Command,
			State:      StateRunning,
			Deliver:    spec.React,
			Persistent: spec.Persistent,
			PID:        p.PID(),
			Provenance: prov,
			CreatedAt:  time.Now(),
		},
		proc:      p,
		linesDone: make(chan struct{}),
	}
	if !m.adopt(rec) {
		p.Stop()
		<-p.Done()
		return Job{}, ErrClosed
	}

	snap := rec.snapshot()
	m.publish(Event{Job: snap})
	go m.pumpMonitorLines(rec)
	go m.reap(rec)
	return snap, nil
}

// pumpMonitorLines drains the monitor's stdout ring into line events,
// applying every backpressure bound before anything can reach a model:
// line-byte cap, event rate limit, coalesced drop accounting, and drop
// accounting for ring overruns and full subscriber buffers. Stderr is
// diagnostic output and never becomes an event.
func (m *Manager) pumpMonitorLines(rec *record) {
	defer close(rec.linesDone)
	var cursor uint64
	var partial []byte
	dropped := 0 // lines lost to rate limit, overruns, or full subscribers
	window := time.Now()
	sentInWindow := 0

	flush := func(final bool) {
		chunk := rec.proc.Output(proc.Stdout, cursor, 0)
		cursor = chunk.Next
		if chunk.Dropped > 0 {
			// The ring overran between polls; whatever the partial line was,
			// it is gone, and an unknown number of lines with it. Count what
			// is countable: at least one.
			partial = nil
			dropped++
		}
		data := append(partial, chunk.Data...)
		lines := strings.Split(string(data), "\n")
		partial = []byte(lines[len(lines)-1])
		lines = lines[:len(lines)-1]
		if final && len(partial) > 0 {
			lines = append(lines, string(partial))
			partial = nil
		}
		for _, line := range lines {
			if line == "" {
				continue
			}
			if now := time.Now(); now.Sub(window) >= time.Second {
				window = now
				sentInWindow = 0
			}
			if sentInWindow >= maxMonitorEventsPerSecond {
				dropped++
				continue
			}
			sentInWindow++
			if len(line) > maxMonitorLineBytes {
				line = string(truncateUTF8([]byte(line), maxMonitorLineBytes)) + "…"
			}
			ev := Event{Job: rec.snapshot(), Type: EventMonitorLine, Line: line, DroppedLines: dropped}
			if m.publish(ev) {
				dropped = 0
			} else {
				// No subscriber had room; the line is gone but not silently.
				dropped++
			}
		}
	}

	ticker := time.NewTicker(monitorPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-rec.proc.Done():
			flush(true)
			if dropped > 0 {
				// Nothing further will carry the count; an empty-line event
				// is the drop summary, so the accounting is complete even
				// when the monitor ends mid-flood.
				m.publish(Event{Job: rec.snapshot(), Type: EventMonitorLine, DroppedLines: dropped})
			}
			return
		case <-ticker.C:
			flush(false)
		}
	}
}

// truncateUTF8 cuts b at max bytes without splitting a rune.
func truncateUTF8(b []byte, max int) []byte {
	if len(b) <= max {
		return b
	}
	cut := max
	for cut > 0 && b[cut]&0xC0 == 0x80 {
		cut--
	}
	return b[:cut]
}

// reap waits for the process and publishes the terminal transition. For a
// monitor it also waits for the line pump's final flush, so the terminal
// event is ordered after the last line event.
func (m *Manager) reap(rec *record) {
	defer m.wg.Done()
	<-rec.proc.Done()
	if rec.linesDone != nil {
		<-rec.linesDone
	}
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
	m.publish(Event{Job: snap})
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

// publish fans the event to interested subscribers without ever blocking job
// progress. It reports whether every interested subscriber accepted it, so a
// monitor pump can count a full buffer as a dropped line instead of losing it
// silently. A lifecycle event that misses a buffer is recoverable from List.
func (m *Manager) publish(ev Event) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	delivered := true
	for _, sub := range m.subs {
		if sub.sessionID != "" && sub.sessionID != ev.Job.Provenance.SessionID {
			continue
		}
		select {
		case sub.ch <- ev:
		default:
			delivered = false
		}
	}
	return delivered
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

// jobIDPrefix keeps background jobs readable where they are read.
//
// A job ID reaches the model as a bare string inside tool output — "job:
// jb_ivyoh5qcfu6ypfkhyedq" beside a command line and a file path — which is the
// one case a type prefix earns: a route, a JSON field, and a column say what
// they name, and free prose does not. Server entities carry none for exactly
// that reason; see docs/design/entity-identity.md.
const jobIDPrefix = "jb_"

// newJobID mints a job identifier. A job lives for one process and is never
// persisted, so entropy failure costs a readable name and nothing else: the
// caller would rather run the job.
func newJobID() string {
	id, err := util.NewPublicID()
	if err != nil {
		return jobIDPrefix + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return jobIDPrefix + id
}
