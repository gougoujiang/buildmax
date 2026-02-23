// Package server: stream hub for Phase 1 (worker pushes deltas); Phase 2 will add Subscribe for SSE.
package server

import (
	"sync"
)

// StreamHub is the interface for run-scoped stream buffers. Implementations may be in-memory (single instance)
// or backed by Redis (multi-instance). Phase 1: Append and Buffer; Phase 2: Subscribe for SSE.
type StreamHub interface {
	// Append adds a delta to the buffer for the given chat run. Safe to call from multiple goroutines.
	Append(chatRunID, delta string)
	// Buffer returns the current buffered content for the run. Empty string if run has no buffer or was Done.
	Buffer(chatRunID string) string
	// Done marks the run as finished; clears buffer and releases resources. Call when run reaches SUCCEEDED/FAILED.
	Done(chatRunID string)
}

// memStreamHub is an in-memory implementation of StreamHub. Safe for concurrent use.
type memStreamHub struct {
	mu     sync.RWMutex
	bufs   map[string]*runBuffer
	maxLen int // max buffer size per run to avoid unbounded growth; 0 = no limit
}

type runBuffer struct {
	b []byte
}

// NewStreamHub returns an in-memory StreamHub. For Phase 1 use only; multi-instance scaling requires a Redis-backed impl.
func NewStreamHub() StreamHub {
	return &memStreamHub{
		bufs:   make(map[string]*runBuffer),
		maxLen: 2 * 1024 * 1024, // 2 MiB per run
	}
}

func (h *memStreamHub) Append(chatRunID, delta string) {
	if chatRunID == "" || delta == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	rb, ok := h.bufs[chatRunID]
	if !ok {
		rb = &runBuffer{}
		h.bufs[chatRunID] = rb
	}
	rb.b = append(rb.b, delta...)
	if h.maxLen > 0 && len(rb.b) > h.maxLen {
		rb.b = rb.b[len(rb.b)-h.maxLen:]
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
	defer h.mu.Unlock()
	delete(h.bufs, chatRunID)
}
