// Package streamhub provides task-scoped stream buffers for worker-push and client subscribe (SSE).
package websocket

import (
	"sync"
)

// StreamEventDone is sent on the subscription channel when the run finishes (SUCCEEDED/FAILED).
const StreamEventDone = "[[DONE]]"

// StreamHub is the interface for task-scoped stream buffers. Keys are task_id (one active run per task).
// Implementations may be in-memory (single instance) or backed by Redis (multi-instance).
type StreamHub interface {
	// Append adds a delta to the buffer and broadcasts to all subscribers for that task.
	Append(taskID, delta string)
	// Buffer returns the current buffered content for the task. Empty string if task has no buffer or was Done.
	Buffer(taskID string) string
	// Done marks the task's current run as finished; sends StreamEventDone to subscribers and clears state.
	Done(taskID string)
	// Subscribe returns a channel of deltas (and finally StreamEventDone) for the task. unsub must be called when done.
	Subscribe(taskID string) (events <-chan string, unsub func())
}

// subscriber channel buffer size; allows a slow client to lag without blocking the worker.
const streamSubscriberBuf = 256

// memStreamHub is an in-memory implementation of StreamHub. Safe for concurrent use.
type memStreamHub struct {
	mu         sync.RWMutex
	bufs       map[string]*runBuffer
	subs       map[string][]chan string // taskID -> subscriber channels (send-only side held here)
	maxLen     int                      // max buffer size per task; 0 = no limit
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
		maxLen:     2 * 1024 * 1024, // 2 MiB per task
		subBufSize: streamSubscriberBuf,
	}
}

func (h *memStreamHub) Append(taskID, delta string) {
	if taskID == "" || delta == "" {
		return
	}
	h.mu.Lock()
	rb, ok := h.bufs[taskID]
	if !ok {
		rb = &runBuffer{}
		h.bufs[taskID] = rb
	}
	rb.b = append(rb.b, delta...)
	if h.maxLen > 0 && len(rb.b) > h.maxLen {
		rb.b = rb.b[len(rb.b)-h.maxLen:]
	}
	// broadcast to subscribers (copy slice to avoid holding lock during send)
	subList := make([]chan string, len(h.subs[taskID]))
	copy(subList, h.subs[taskID])
	h.mu.Unlock()

	for _, ch := range subList {
		select {
		case ch <- delta:
		default:
			// client too slow; skip this delta to avoid blocking worker
		}
	}
}

func (h *memStreamHub) Buffer(taskID string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	rb, ok := h.bufs[taskID]
	if !ok {
		return ""
	}
	return string(rb.b)
}

func (h *memStreamHub) Done(taskID string) {
	h.mu.Lock()
	subList := h.subs[taskID]
	delete(h.subs, taskID)
	delete(h.bufs, taskID)
	h.mu.Unlock()

	// Send done to subscribers; do not close channels (client calls unsub which closes).
	for _, ch := range subList {
		select {
		case ch <- StreamEventDone:
		default:
		}
	}
}

func (h *memStreamHub) Subscribe(taskID string) (events <-chan string, unsub func()) {
	if taskID == "" {
		ch := make(chan string, 1)
		close(ch)
		return ch, func() {}
	}
	ch := make(chan string, h.subBufSize)
	h.mu.Lock()
	h.subs[taskID] = append(h.subs[taskID], ch)
	h.mu.Unlock()

	unsub = func() {
		h.mu.Lock()
		list := h.subs[taskID]
		for i, c := range list {
			if c == ch {
				h.subs[taskID] = append(list[:i], list[i+1:]...)
				break
			}
		}
		if len(h.subs[taskID]) == 0 {
			delete(h.subs, taskID)
		}
		h.mu.Unlock()
		close(ch)
	}
	return ch, unsub
}
