package runterminal

import (
	"context"
	"log/slog"
	"sync"
)

// Group owns the terminal callbacks a server has in flight.
//
// The callbacks outlive the request that triggered them on purpose: a worker
// reporting its result should not wait for a Tier 1 turn and a workflow step to
// be written before its own call returns. But a goroutine nobody waits for is
// lost when the process stops, and half of what these callbacks do — advancing
// a workflow — has no retry behind it. So they are owned rather than spawned
// and forgotten.
//
// See docs/design/graceful-shutdown.md §9.
type Group struct {
	mu     sync.Mutex
	wg     sync.WaitGroup
	closed bool
}

// NewGroup returns a group that accepts work.
func NewGroup() *Group { return &Group{} }

// Go runs f, in the background while the group is open.
//
// Once the group is closed it runs f inline instead of refusing it. The caller
// at that point is a worker's final report, and doing the work on its goroutine
// is bounded by the same request the worker is already waiting on — better than
// dropping a workflow advance because the timing was unlucky.
func (g *Group) Go(f func()) {
	if g == nil {
		go f()
		return
	}
	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		f()
		return
	}
	g.wg.Add(1)
	g.mu.Unlock()
	go func() {
		defer g.wg.Done()
		f()
	}()
}

// Wait closes the group to new background work and waits for what is running,
// bounded by ctx. It reports whether everything finished.
func (g *Group) Wait(ctx context.Context) bool {
	if g == nil {
		return true
	}
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()

	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		slog.Warn("terminal callbacks did not finish before shutdown")
		return false
	}
}
