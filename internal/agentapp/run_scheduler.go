package agentapp

import (
	"context"
	"fmt"
	"sync"

	"github.com/gougoujiang/buildmax/internal/core/agent"
)

// runHost is the slice of AgentApp the scheduler drives. *AgentApp satisfies it;
// a test supplies a fake to exercise scheduling without a model.
type runHost interface {
	OpenSession(sessionID string) (*SessionContext, error)
	CloseSession(sess *SessionContext)
	RunPrompt(ctx context.Context, sess *SessionContext, prompt string, opts RunPromptOpts) (RunResult, error)
}

// RunLifecycle receives one scheduled run's progress. Every method is called on
// the run's own goroutine, in order, and a surface adapts each to its event
// transport — Wails emit for Desktop, Bubble Tea messages for the CLI. RunOpts
// is called once before the first turn; the others may be called repeatedly as
// the queue drains.
type RunLifecycle interface {
	// RunOpts supplies the per-run options — stream sink, event sink, approval
	// handler, digest — for this run. The scheduler fills in Pending with the
	// key's queue, so an implementation leaves that field zero.
	RunOpts() RunPromptOpts
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

// RunScheduler serializes runs per key. At most one run per key is in flight; a
// prompt submitted while one runs is queued and drained as its own turn once
// the current run reaches a boundary. It owns the pending queue, the session for
// a run's whole life, and cancellation.
//
// It holds no surface concept: event delivery and formatting stay with the
// caller through RunLifecycle. This is the run/queue machine both the Desktop
// and CLI surfaces need, kept here because AGENTS.md makes agentapp the owner of
// the runtime the surfaces share rather than a thing each reimplements.
type RunScheduler struct {
	host runHost

	mu     sync.Mutex
	runs   map[string]context.CancelFunc // keys with a run in flight
	queues map[string]*agent.MessageQueue
}

// NewRunScheduler returns a scheduler that drives runs through host.
func NewRunScheduler(host runHost) *RunScheduler {
	return &RunScheduler{
		host:   host,
		runs:   map[string]context.CancelFunc{},
		queues: map[string]*agent.MessageQueue{},
	}
}

// Submit runs prompt for key against sessionID, or queues it behind the key's
// in-flight run. It returns the prompt's 1-based queue position, or 0 when it
// started a run. Cancellation of the run derives from parent.
func (s *RunScheduler) Submit(parent context.Context, key, sessionID, prompt string, lc RunLifecycle) (int, error) {
	s.mu.Lock()
	if _, busy := s.runs[key]; busy {
		q := s.queueLocked(key)
		s.mu.Unlock()
		pos, err := q.Enqueue(prompt)
		if err != nil {
			return 0, fmt.Errorf("%w: %d messages are already waiting", err, q.Len())
		}
		return pos, nil
	}
	// Reserve the slot under the lock so a Submit racing this one queues instead
	// of starting a second run for the key.
	runCtx, cancel := context.WithCancel(parent)
	s.runs[key] = cancel
	queue := s.queueLocked(key)
	s.mu.Unlock()

	sess, err := s.host.OpenSession(sessionID)
	if err != nil {
		s.mu.Lock()
		delete(s.runs, key)
		s.mu.Unlock()
		cancel()
		return 0, fmt.Errorf("open session: %w", err)
	}

	opts := lc.RunOpts()
	opts.Pending = queue
	go s.run(runCtx, cancel, key, sess, prompt, queue, opts, lc)
	return 0, nil
}

// run owns the session for its whole life and drains the queue one turn per
// iteration. Most queued prompts never reach the Dequeue below — the run itself
// folds them in at its iteration boundary through RunPromptOpts.Pending; this
// loop is the backstop for one queued after the run stopped reading.
func (s *RunScheduler) run(ctx context.Context, cancel context.CancelFunc, key string, sess *SessionContext, prompt string, queue *agent.MessageQueue, opts RunPromptOpts, lc RunLifecycle) {
	defer func() {
		s.mu.Lock()
		delete(s.runs, key)
		s.mu.Unlock()
		cancel()
		// Idempotent: the queue-empty branch already closed the session on the
		// normal path. This stands for the paths that return before it.
		s.host.CloseSession(sess)
	}()
	for current := prompt; ; {
		out, err := s.host.RunPrompt(ctx, sess, current, opts)
		if err == nil {
			lc.TurnDone(out)
		}
		next, ok := queue.Dequeue()
		if !ok {
			// Release the session before the run's last callback. A surface that
			// issues a rewind or fork the moment Done arrives would otherwise race
			// the deferred close and be told the session is busy by the run that
			// just finished — after a failure as much as after a success.
			s.host.CloseSession(sess)
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
		current = next
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
