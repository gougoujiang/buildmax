// Package server: stream hub for Phase 1 (worker pushes deltas) and Phase 2 (Subscribe for SSE).
package server

import (
	"sync"
)

// StreamEventDone is sent on the subscription channel when the run finishes (SUCCEEDED/FAILED).
const StreamEventDone = "[[DONE]]"

// StreamHub is the interface for run-scoped stream buffers. Implementations may be in-memory (single instance)
// or backed by Redis (multi-instance).
type StreamHub interface {
	// Append adds a delta to the buffer and broadcasts to all subscribers for that run.
	Append(chatRunID, delta string)
	// Buffer returns the current buffered content for the run. Empty string if run has no buffer or was Done.
	Buffer(chatRunID string) string
	// Done marks the run as finished; sends StreamEventDone to subscribers, closes their channels, and clears state.
	Done(chatRunID string)
	// Subscribe returns a channel of deltas (and finally StreamEventDone) for the run. unsub must be called when done.
	Subscribe(chatRunID string) (events <-chan string, unsub func())
}

// subscriber channel buffer size; allows a slow client to lag without blocking the worker.
const streamSubscriberBuf = 256

// memStreamHub is an in-memory implementation of StreamHub. Safe for concurrent use.
type memStreamHub struct {
	mu         sync.RWMutex
	bufs       map[string]*runBuffer
	subs       map[string][]chan string // chatRunID -> subscriber channels (send-only side held here)
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

func (h *memStreamHub) Append(chatRunID, delta string) {
	if chatRunID == "" || delta == "" {
		return
	}
	h.mu.Lock()
	rb, ok := h.bufs[chatRunID]
	if !ok {
		rb = &runBuffer{}
		h.bufs[chatRunID] = rb
	}
	rb.b = append(rb.b, delta...)
	if h.maxLen > 0 && len(rb.b) > h.maxLen {
		rb.b = rb.b[len(rb.b)-h.maxLen:]
	}
	// broadcast to subscribers (copy slice to avoid holding lock during send)
	subList := make([]chan string, len(h.subs[chatRunID]))
	copy(subList, h.subs[chatRunID])
	h.mu.Unlock()

	for _, ch := range subList {
		select {
		case ch <- delta:
		default:
			// client too slow; skip this delta to avoid blocking worker
		}
	}
}

func (h *memStreamHub) Buffer(chatRunID string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	rb, ok := h.bufs[chatRunID]
	if !ok {
		return ""
	}
	return string(rb.b)
}

func (h *memStreamHub) Done(chatRunID string) {
	h.mu.Lock()
	subList := h.subs[chatRunID]
	delete(h.subs, chatRunID)
	delete(h.bufs, chatRunID)
	h.mu.Unlock()

	// Send done to subscribers; do not close channels (client calls unsub which closes).
	for _, ch := range subList {
		select {
		case ch <- StreamEventDone:
		default:
		}
	}
}

func (h *memStreamHub) Subscribe(chatRunID string) (events <-chan string, unsub func()) {
	if chatRunID == "" {
		ch := make(chan string, 1)
		close(ch)
		return ch, func() {}
	}
	ch := make(chan string, h.subBufSize)
	h.mu.Lock()
	h.subs[chatRunID] = append(h.subs[chatRunID], ch)
	h.mu.Unlock()

	unsub = func() {
		h.mu.Lock()
		list := h.subs[chatRunID]
		for i, c := range list {
			if c == ch {
				h.subs[chatRunID] = append(list[:i], list[i+1:]...)
				break
			}
		}
		if len(h.subs[chatRunID]) == 0 {
			delete(h.subs, chatRunID)
		}
		h.mu.Unlock()
		close(ch)
	}
	return ch, unsub
}
