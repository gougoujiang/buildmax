package agentapp

import (
	"context"
	"fmt"
	"sync"

	"github.com/gougoujiang/buildmax/internal/core/agent"
)

// RunHost is the slice of AgentApp the scheduler drives for one run. *AgentApp
// satisfies it; a test supplies a fake to exercise scheduling without a model.
// It is resolved per run rather than held, because a surface may own several
// AgentApps — Desktop keeps one per project — and each key runs against its own.
type RunHost interface {
	OpenSession(sessionID string) (*SessionContext, error)
	CloseSession(sess *SessionContext)
	RunPrompt(ctx context.Context, sess *SessionContext, prompt string, opts RunPromptOpts) (RunResult, error)
	RunBackgroundEvent(ctx context.Context, sess *SessionContext, ev BackgroundEvent, opts RunPromptOpts) (RunResult, error)
}

// HostFunc resolves the host a run should use. The scheduler calls it only when
// it commits to starting a run — never on the path that queues behind a run
// already in flight — so a surface can skip resolving a project it does not need
// and report a resolution failure as the submit's own error.
type HostFunc func() (RunHost, error)

// RunLifecycle receives one scheduled run's progress. Every method is called on
// the run's own goroutine, in order, and a surface adapts each to its event
// transport — Wails emit for Desktop, Bubble Tea messages for the CLI. OnStart
// and RunOpts are called once before the first turn; the others may be called
// repeatedly as the queue drains.
type RunLifecycle interface {
	// RunOpts supplies the per-run options — stream sink, event sink, approval
	// handler, digest — for this run. The scheduler fills in Pending with the
	// key's queue, so an implementation leaves that field zero.
	RunOpts() RunPromptOpts
	// OnStart runs once the session is open, before the first turn's output, so a
	// surface can announce the run ahead of any stream delta. Foreground runs
	// have nothing to announce and leave it empty.
	OnStart(sess *SessionContext)
	// TurnDone reports one completed turn. More turns follow when the queue is
	// not yet empty.
	TurnDone(out RunResult)
	// TurnError reports a failed turn that is not the run's last: queued prompts
	// remain, and a surface orders the error before the next prompt. The final
	// turn's error is reported through Done, not here.
	TurnError(err error)
	// Dequeued reports that next left the queue to run as the following turn.
	Dequeued(next string, snapshot []string)
	// Done reports the run finished after its last turn. The session is already
	// released; err is that turn's error, or nil on success.
	Done(out RunResult, err error)
}

// openingTurn runs a scheduled run's first turn — a user prompt for Submit, a
// background event for StartEvent. Every turn after it is a queued prompt.
type openingTurn func(ctx context.Context, sess *SessionContext, opts RunPromptOpts) (RunResult, error)

// RunScheduler serializes runs per key. At most one run per key is in flight; a
// prompt submitted while one runs is queued and drained as its own turn once
// the current run reaches a boundary. It owns the pending queue, the session for
// a run's whole life, and cancellation.
//
// It holds no surface concept and no AgentApp: event delivery stays with the
// caller through RunLifecycle, and the host to run against is passed per run.
// This is the run/queue machine the Desktop and CLI surfaces share, kept here
// because AGENTS.md makes agentapp the owner of that runtime rather than a thing
// each interface reimplements.
//
// Locking: the scheduler's mutex may be held while a StartEvent pop callback
// runs, so a caller must never hold its own lock while calling into the
// scheduler. The one direction is scheduler-lock then caller-lock, never the
// reverse.
type RunScheduler struct {
	mu     sync.Mutex
	runs   map[string]context.CancelFunc // keys with a run in flight
	queues map[string]*agent.MessageQueue
}

// NewRunScheduler returns an empty scheduler.
func NewRunScheduler() *RunScheduler {
	return &RunScheduler{
		runs:   map[string]context.CancelFunc{},
		queues: map[string]*agent.MessageQueue{},
	}
}

// Submit runs prompt for key against sessionID, or queues it behind the key's
// in-flight run. It returns the prompt's 1-based queue position, or 0 when it
// started a run. host is resolved only on the path that starts a run — a prompt
// that queues resolves nothing — and its error is the submit's error.
// Cancellation of the run derives from parent.
func (s *RunScheduler) Submit(parent context.Context, key, sessionID, prompt string, host HostFunc, lc RunLifecycle) (int, error) {
	if pos, queued, err := s.enqueueIfBusy(key, prompt); queued || err != nil {
		return pos, err
	}
	// Not busy: resolve the host before reserving, so a resolution failure leaves
	// no slot behind for a prompt racing this one to strand itself against.
	h, err := host()
	if err != nil {
		return 0, err
	}
	runCtx, cancel, queue, started := s.reserve(parent, key)
	if !started {
		// A run began while the host resolved; the prompt joins its queue.
		pos, qerr := queue.Enqueue(prompt)
		if qerr != nil {
			return 0, fmt.Errorf("%w: %d messages are already waiting", qerr, queue.Len())
		}
		return pos, nil
	}
	opening := func(ctx context.Context, sess *SessionContext, opts RunPromptOpts) (RunResult, error) {
		return h.RunPrompt(ctx, sess, prompt, opts)
	}
	if err := s.start(runCtx, cancel, key, sessionID, h, opening, queue, lc); err != nil {
		return 0, err
	}
	return 0, nil
}

// StartEvent runs a background event for key as its own turn, if the key is
// idle. It resolves the host, then reserves the slot and calls pop under the
// scheduler's lock, so the event is consumed only when — and exactly when — the
// slot is won; a pop returning false, or a busy key, starts nothing and returns
// false. This is how a job delivery avoids double-running a parked event under a
// race.
func (s *RunScheduler) StartEvent(parent context.Context, key, sessionID string, host HostFunc, pop func() (BackgroundEvent, bool), lc RunLifecycle) (bool, error) {
	if s.Busy(key) {
		return false, nil
	}
	h, err := host()
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	if _, busy := s.runs[key]; busy {
		s.mu.Unlock()
		return false, nil
	}
	ev, ok := pop()
	if !ok {
		s.mu.Unlock()
		return false, nil
	}
	runCtx, cancel := context.WithCancel(parent)
	queue := s.reserveLocked(key, cancel)
	s.mu.Unlock()

	opening := func(ctx context.Context, sess *SessionContext, opts RunPromptOpts) (RunResult, error) {
		return h.RunBackgroundEvent(ctx, sess, ev, opts)
	}
	if err := s.start(runCtx, cancel, key, sessionID, h, opening, queue, lc); err != nil {
		return false, err
	}
	return true, nil
}

// enqueueIfBusy queues prompt when key already has a run in flight. queued is
// false when the key was idle, and the caller goes on to start a run.
func (s *RunScheduler) enqueueIfBusy(key, prompt string) (pos int, queued bool, err error) {
	s.mu.Lock()
	if _, busy := s.runs[key]; !busy {
		s.mu.Unlock()
		return 0, false, nil
	}
	q := s.queueLocked(key)
	s.mu.Unlock()
	pos, err = q.Enqueue(prompt)
	if err != nil {
		return 0, true, fmt.Errorf("%w: %d messages are already waiting", err, q.Len())
	}
	return pos, true, nil
}

// reserve marks key in flight and returns its run context and queue. started is
// false when a run began between the caller's busy check and here, in which case
// nothing is reserved and the returned queue is the running key's own.
func (s *RunScheduler) reserve(parent context.Context, key string) (context.Context, context.CancelFunc, *agent.MessageQueue, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, busy := s.runs[key]; busy {
		return nil, nil, s.queueLocked(key), false
	}
	runCtx, cancel := context.WithCancel(parent)
	return runCtx, cancel, s.reserveLocked(key, cancel), true
}

// start opens the session and launches the run goroutine. On an open failure it
// releases the reserved slot so the key is usable again.
func (s *RunScheduler) start(runCtx context.Context, cancel context.CancelFunc, key, sessionID string, host RunHost, opening openingTurn, queue *agent.MessageQueue, lc RunLifecycle) error {
	sess, err := host.OpenSession(sessionID)
	if err != nil {
		s.release(key)
		cancel()
		return fmt.Errorf("open session: %w", err)
	}
	opts := lc.RunOpts()
	opts.Pending = queue
	go s.run(runCtx, cancel, key, host, sess, opening, queue, opts, lc)
	return nil
}

// run owns the session for its whole life and drains the queue one turn per
// iteration. Most queued prompts never reach the Dequeue below — the run itself
// folds them in at its iteration boundary through RunPromptOpts.Pending; this
// loop is the backstop for one queued after the run stopped reading.
func (s *RunScheduler) run(ctx context.Context, cancel context.CancelFunc, key string, host RunHost, sess *SessionContext, opening openingTurn, queue *agent.MessageQueue, opts RunPromptOpts, lc RunLifecycle) {
	defer func() {
		s.release(key)
		cancel()
		// Idempotent: the queue-empty branch already closed the session on the
		// normal path. This stands for the paths that return before it.
		host.CloseSession(sess)
	}()
	lc.OnStart(sess)
	out, err := opening(ctx, sess, opts)
	for {
		if err == nil {
			lc.TurnDone(out)
		}
		next, ok := queue.Dequeue()
		if !ok {
			// Release the session before the run's last callback. A surface that
			// issues a rewind or fork the moment Done arrives would otherwise race
			// the deferred close and be told the session is busy by the run that
			// just finished — after a failure as much as after a success.
			host.CloseSession(sess)
			lc.Done(out, err)
			return
		}
		// A turn that failed with more still queued reports it now, so the error
		// and the prompt it belongs to stay in order. The queue still drains: it
		// holds what the user asked for, and stranding it with no run to release
		// it is worse than letting it fail on its own turn.
		if err != nil {
			lc.TurnError(err)
		}
		lc.Dequeued(next, queue.Snapshot())
		out, err = host.RunPrompt(ctx, sess, next, opts)
	}
}

// Cancel stops the in-flight run for key, if any, and discards its queue: the
// queued prompts were written for work the user has just called off, and
// delivering them afterwards would restart it in their name. A key with no run
// is a no-op.
func (s *RunScheduler) Cancel(key string) {
	s.mu.Lock()
	cancel := s.runs[key]
	q := s.queues[key]
	s.mu.Unlock()
	if q != nil {
		q.Drop()
	}
	if cancel != nil {
		cancel()
	}
}

// CancelAll stops every in-flight run and discards every queue. It is the
// shutdown path: a surface tearing down cancels all keys at once.
func (s *RunScheduler) CancelAll() {
	s.mu.Lock()
	cancels := s.runs
	queues := s.queues
	s.runs = map[string]context.CancelFunc{}
	s.mu.Unlock()
	for _, q := range queues {
		q.Drop()
	}
	for _, cancel := range cancels {
		cancel()
	}
}

// Busy reports whether key has a run in flight.
func (s *RunScheduler) Busy(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, busy := s.runs[key]
	return busy
}

// Queued returns the prompts waiting behind key's in-flight run, oldest first.
func (s *RunScheduler) Queued(key string) []string {
	s.mu.Lock()
	q := s.queues[key]
	s.mu.Unlock()
	if q == nil {
		return nil
	}
	return q.Snapshot()
}

// reserveLocked records key as in flight and returns its queue. The caller holds
// s.mu.
func (s *RunScheduler) reserveLocked(key string, cancel context.CancelFunc) *agent.MessageQueue {
	s.runs[key] = cancel
	return s.queueLocked(key)
}

// release clears key's in-flight mark.
func (s *RunScheduler) release(key string) {
	s.mu.Lock()
	delete(s.runs, key)
	s.mu.Unlock()
}

// queueLocked returns key's queue, allocating it on first use. The caller holds
// s.mu.
func (s *RunScheduler) queueLocked(key string) *agent.MessageQueue {
	q := s.queues[key]
	if q == nil {
		q = agent.NewMessageQueue(agent.DefaultMaxQueuedMessages)
		s.queues[key] = q
	}
	return q
}
