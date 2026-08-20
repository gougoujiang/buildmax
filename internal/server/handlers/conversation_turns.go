package handlers

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/gougoujiang/buildmax/internal/core/agent"
)

// maxQueuedTurns caps how many turns may wait behind the one running in a
// conversation. Past that a submission is refused rather than silently accepted:
// a user who cannot see the backlog should not be able to grow it without limit.
const maxQueuedTurns = agent.DefaultMaxQueuedMessages

// errTurnQueueFull is returned by the registry when a conversation is at its cap.
var errTurnQueueFull = errors.New("too many turns are already queued for this conversation")

// turnJob is one conversation turn waiting for its conversation to be free.
type turnJob struct {
	// run executes the turn. It is called on the registry's goroutine for the
	// conversation, never concurrently with another job for the same conversation.
	run func()
	// onDequeue, when set, is called just before run for a job that had to wait.
	// A job that started immediately never sees it, which is what lets a surface
	// announce "this queued message is starting now" without a race.
	onDequeue func()
	done      chan struct{}
	dropped   atomic.Bool
}

func newTurnJob(run func()) *turnJob {
	return &turnJob{run: run, done: make(chan struct{})}
}

// convTurnQueue serializes the turns of one conversation.
type convTurnQueue struct {
	mu      sync.Mutex
	running bool
	pending []*turnJob
}

// turnRegistry owns the per-conversation turn queues for the whole server.
//
// It lives on the Handler rather than on a WebSocket connection because a
// conversation outlives any one connection: the same conversation is reachable
// from a reconnected socket, a second browser tab, the HTTP API, and a system turn
// reporting a finished task. Serialization anchored to a connection would let two
// of those interleave their reads and writes of the same message history.
type turnRegistry struct {
	mu     sync.Mutex
	queues map[string]*convTurnQueue
}

func newTurnRegistry() *turnRegistry {
	return &turnRegistry{queues: make(map[string]*convTurnQueue)}
}

// Submit hands a turn to the conversation's queue. It returns 0 when the turn
// started immediately, or the job's 1-based position when it had to wait. Submit
// never blocks; the turn runs on a goroutine the registry owns.
func (r *turnRegistry) Submit(conversationID string, job *turnJob) (int, error) {
	q := r.queue(conversationID)
	q.mu.Lock()
	if q.running {
		if len(q.pending) >= maxQueuedTurns {
			q.mu.Unlock()
			return 0, errTurnQueueFull
		}
		q.pending = append(q.pending, job)
		pos := len(q.pending)
		q.mu.Unlock()
		return pos, nil
	}
	q.running = true
	q.mu.Unlock()
	go r.drain(conversationID, q, job)
	return 0, nil
}

// RunSync submits a turn and waits for it to finish. It is what a request that has
// to stream its own turn back to the caller uses, where a fire-and-forget submit
// would return before there was anything to send.
//
// A caller that goes away before its turn starts marks the job dropped and returns
// the context error; the queue moves on to the next turn.
func (r *turnRegistry) RunSync(ctx context.Context, conversationID string, run func()) error {
	job := newTurnJob(run)
	if _, err := r.Submit(conversationID, job); err != nil {
		return err
	}
	select {
	case <-job.done:
		return nil
	case <-ctx.Done():
		job.dropped.Store(true)
		return ctx.Err()
	}
}

// Waiting reports how many turns are queued behind the conversation's current turn.
func (r *turnRegistry) Waiting(conversationID string) int {
	r.mu.Lock()
	q, ok := r.queues[conversationID]
	r.mu.Unlock()
	if !ok {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

func (r *turnRegistry) queue(conversationID string) *convTurnQueue {
	r.mu.Lock()
	defer r.mu.Unlock()
	q, ok := r.queues[conversationID]
	if !ok {
		q = &convTurnQueue{}
		r.queues[conversationID] = q
	}
	return q
}

// drain runs job, then every turn queued behind it, one at a time.
func (r *turnRegistry) drain(conversationID string, q *convTurnQueue, job *turnJob) {
	for job != nil {
		if !job.dropped.Load() {
			job.run()
		}
		close(job.done)

		q.mu.Lock()
		if len(q.pending) == 0 {
			q.running = false
			q.mu.Unlock()
			r.forget(conversationID, q)
			return
		}
		job, q.pending = q.pending[0], q.pending[1:]
		q.mu.Unlock()
		if job.onDequeue != nil && !job.dropped.Load() {
			job.onDequeue()
		}
	}
}

// forget removes an idle queue so a long-lived server does not accumulate one
// entry per conversation it has ever served.
func (r *turnRegistry) forget(conversationID string, q *convTurnQueue) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.queues[conversationID] != q {
		return
	}
	q.mu.Lock()
	idle := !q.running && len(q.pending) == 0
	q.mu.Unlock()
	if idle {
		delete(r.queues, conversationID)
	}
}
