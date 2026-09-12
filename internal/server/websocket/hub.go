// Package streamhub provides run-scoped stream buffers for worker-push and client subscribe (SSE).
package websocket

import (
	"sync"
)

// StreamEventDone is sent on the subscription channel when the run finishes (SUCCEEDED/FAILED).
const StreamEventDone = "[[DONE]]"

// StreamHub is the interface for run-scoped stream buffers. Keys are task_run_id:
// a task's successive turns each own a distinct buffer, so one run's output is
// never replayed to the next run's subscribers.
// Implementations may be in-memory (single instance) or backed by Redis (multi-instance).
type StreamHub interface {
	// Append adds a delta to the run's buffer and broadcasts to its subscribers.
	Append(runID, delta string)
	// Buffer returns the run's buffered content so far. Empty string if the run has no buffer or was Done.
	Buffer(runID string) string
	// Done marks the run finished; sends StreamEventDone to subscribers and clears state.
	Done(runID string)
	// Subscribe returns a channel of deltas (and finally StreamEventDone) for the run. unsub must be called when done.
	Subscribe(runID string) (events <-chan string, unsub func())
}

// subscriber channel buffer size; allows a slow client to lag without blocking the worker.
const streamSubscriberBuf = 256

// memStreamHub is an in-memory implementation of StreamHub. Safe for concurrent use.
type memStreamHub struct {
	mu         sync.RWMutex
	bufs       map[string]*runBuffer
	subs       map[string][]chan string // runID -> subscriber channels (send-only side held here)
	maxLen     int                      // max buffer size per run; 0 = no limit
	subBufSize int
}

type runBuffer struct {
	b []byte
}

// NewStreamHub returns an in-memory StreamHub. Multi-instance scaling requires a Redis-backed impl.
func NewStreamHub() StreamHub {
	return &memStreamHub{
		bufs:       make(map[string]*runBuffer),
		subs:       make(map[string][]chan string),
		maxLen:     2 * 1024 * 1024, // 2 MiB per run
		subBufSize: streamSubscriberBuf,
	}
}

func (h *memStreamHub) Append(runID, delta string) {
	if runID == "" || delta == "" {
		return
	}
	h.mu.Lock()
	rb, ok := h.bufs[runID]
	if !ok {
		rb = &runBuffer{}
		h.bufs[runID] = rb
	}
	rb.b = append(rb.b, delta...)
	if h.maxLen > 0 && len(rb.b) > h.maxLen {
		rb.b = rb.b[len(rb.b)-h.maxLen:]
	}
	// broadcast to subscribers (copy slice to avoid holding lock during send)
	subList := make([]chan string, len(h.subs[runID]))
	copy(subList, h.subs[runID])
	h.mu.Unlock()

	for _, ch := range subList {
		select {
		case ch <- delta:
		default:
			// client too slow; skip this delta to avoid blocking worker
		}
	}
}

func (h *memStreamHub) Buffer(runID string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	rb, ok := h.bufs[runID]
	if !ok {
		return ""
	}
	return string(rb.b)
}

func (h *memStreamHub) Done(runID string) {
	h.mu.Lock()
	subList := h.subs[runID]
	delete(h.subs, runID)
	delete(h.bufs, runID)
	h.mu.Unlock()

	// Send done to subscribers; do not close channels (client calls unsub which closes).
	for _, ch := range subList {
		select {
		case ch <- StreamEventDone:
		default:
		}
	}
}

func (h *memStreamHub) Subscribe(runID string) (events <-chan string, unsub func()) {
	if runID == "" {
		ch := make(chan string, 1)
		close(ch)
		return ch, func() {}
	}
	ch := make(chan string, h.subBufSize)
	h.mu.Lock()
	h.subs[runID] = append(h.subs[runID], ch)
	h.mu.Unlock()

	unsub = func() {
		h.mu.Lock()
		list := h.subs[runID]
		for i, c := range list {
			if c == ch {
				h.subs[runID] = append(list[:i], list[i+1:]...)
				break
			}
		}
		if len(h.subs[runID]) == 0 {
			delete(h.subs, runID)
		}
		h.mu.Unlock()
		close(ch)
	}
	return ch, unsub
}
