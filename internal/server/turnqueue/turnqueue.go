// Package turnqueue serializes the turns of one conversation.
//
// Two paths reach it -- a WebSocket message and an HTTP post -- and a
// conversation may only be running one turn at a time whichever arrives. The
// registry is server-scoped rather than connection-scoped for that reason: a
// second browser tab is a second connection but the same conversation.
package turnqueue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/gougoujiang/buildmax/internal/core/agent"
)

// MaxQueued caps how many turns may wait behind the one running in a
// conversation. Past that a submission is refused rather than silently accepted:
// a user who cannot see the backlog should not be able to grow it without limit.
const MaxQueued = agent.DefaultMaxQueuedMessages

// ErrQueueFull is returned by the registry when a conversation is at its cap.
var ErrQueueFull = errors.New("too many turns are already queued for this conversation")

// ErrDraining is returned once the server has begun shutting down. A turn is a
// model call that writes message history, so starting one this process cannot
// finish is worse than refusing it: the caller retries against an instance that
// will still be here.
var ErrDraining = errors.New("this server is shutting down; retry the turn")

// Job is one conversation turn waiting for its conversation to be free.
type Job struct {
	// run executes the turn. It is called on the registry's goroutine for the
	// conversation, never concurrently with another job for the same conversation.
	run func()
	// OnDequeue, when set, is called just before run for a job that had to wait.
	// A job that started immediately never sees it, which is what lets a surface
	// announce "this queued message is starting now" without a race.
	OnDequeue func()
	Done      chan struct{}
	Dropped   atomic.Bool
}

func NewJob(run func()) *Job {
	return &Job{run: run, Done: make(chan struct{})}
}

// convQueue serializes the turns of one conversation.
type convQueue struct {
	mu      sync.Mutex
	running bool
	pending []*Job
}

// Registry owns the per-conversation turn queues for the whole server.
//
// It lives on the Handler rather than on a WebSocket connection because a
// conversation outlives any one connection: the same conversation is reachable
// from a reconnected socket, a second browser tab, the HTTP API, and a system turn
// reporting a finished task. Serialization anchored to a connection would let two
// of those interleave their reads and writes of the same message history.
type Registry struct {
	mu     sync.Mutex
	queues map[string]*convQueue
	// draining refuses new turns from the moment the server starts stopping.
	draining bool
	// active counts the queue goroutines currently running turns, so a shutdown
	// can wait for them. A WebSocket carries a turn on a hijacked connection,
	// which http.Server.Shutdown does not wait for — without this, an answer
	// being written would simply vanish when the process exits.
	active sync.WaitGroup
}

func NewRegistry() *Registry {
	return &Registry{queues: make(map[string]*convQueue)}
}

// Submit hands a turn to the conversation's queue. It returns 0 when the turn
// started immediately, or the job's 1-based position when it had to wait. Submit
// never blocks; the turn runs on a goroutine the registry owns.
func (r *Registry) Submit(conversationID string, job *Job) (int, error) {
	// The draining check and the Add share one critical section with Drain, so
	// a turn admitted here is always counted before Wait can start waiting.
	r.mu.Lock()
	if r.draining {
		r.mu.Unlock()
		return 0, ErrDraining
	}
	r.active.Add(1)
	q, ok := r.queues[conversationID]
	if !ok {
		q = &convQueue{}
		r.queues[conversationID] = q
	}
	r.mu.Unlock()

	q.mu.Lock()
	if q.running {
		defer r.active.Done() // the goroutine already running this queue owns the count
		if len(q.pending) >= MaxQueued {
			q.mu.Unlock()
			return 0, ErrQueueFull
		}
		q.pending = append(q.pending, job)
		pos := len(q.pending)
		q.mu.Unlock()
		return pos, nil
	}
	q.running = true
	q.mu.Unlock()
	go func() {
		defer r.active.Done()
		r.drain(conversationID, q, job)
	}()
	return 0, nil
}

// Drain refuses new turns. Turns already running are left alone — Wait is what
// gives them their moment to finish.
func (r *Registry) Drain() {
	r.mu.Lock()
	r.draining = true
	r.mu.Unlock()
}

// Wait blocks until every running turn has finished or ctx expires, and reports
// whether they all finished. Call Drain first, or a new turn can keep it
// waiting indefinitely.
func (r *Registry) Wait(ctx context.Context) bool {
	done := make(chan struct{})
	go func() {
		r.active.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

// RunSync submits a turn and waits for it to finish. It is what a request that has
// to stream its own turn back to the caller uses, where a fire-and-forget submit
// would return before there was anything to send.
//
// A caller that goes away before its turn starts marks the job dropped and returns
// the context error; the queue moves on to the next turn.
func (r *Registry) RunSync(ctx context.Context, conversationID string, run func()) error {
	job := NewJob(run)
	if _, err := r.Submit(conversationID, job); err != nil {
		return err
	}
	select {
	case <-job.Done:
		return nil
	case <-ctx.Done():
		job.Dropped.Store(true)
		return ctx.Err()
	}
}

// Waiting reports how many turns are queued behind the conversation's current turn.
func (r *Registry) Waiting(conversationID string) int {
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

// drain runs job, then every turn queued behind it, one at a time.
func (r *Registry) drain(conversationID string, q *convQueue, job *Job) {
	for job != nil {
		if !job.Dropped.Load() {
			job.run()
		}
		close(job.Done)

		q.mu.Lock()
		if len(q.pending) == 0 {
			q.running = false
			q.mu.Unlock()
			r.forget(conversationID, q)
			return
		}
		job, q.pending = q.pending[0], q.pending[1:]
		q.mu.Unlock()
		if job.OnDequeue != nil && !job.Dropped.Load() {
			job.OnDequeue()
		}
	}
}

// forget removes an idle queue so a long-lived server does not accumulate one
// entry per conversation it has ever served.
func (r *Registry) forget(conversationID string, q *convQueue) {
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
