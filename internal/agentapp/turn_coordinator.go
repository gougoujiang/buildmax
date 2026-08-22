package agentapp

import (
	"errors"
	"sync"
)

// ErrTurnActive reports that a session already has a running turn. Callers
// that can wait should queue behind the run (surfaces already do); nothing
// may run a second turn concurrently.
var ErrTurnActive = errors.New("a turn is already running for this session")

// turnCoordinator enforces the one-writer-per-session rule at the RunPrompt
// chokepoint. The rule used to be a surface convention — each UI allows one
// run — which background job delivery would silently break; this turns an
// overlapping turn into a refused call instead of a history data race.
// Mirrors docs/design/local-background-jobs.md ("One session writer,
// enforced"). The zero value is ready to use.
type turnCoordinator struct {
	mu     sync.Mutex
	active map[string]struct{}
}

func (c *turnCoordinator) begin(sessionID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.active[sessionID]; ok {
		return ErrTurnActive
	}
	if c.active == nil {
		c.active = make(map[string]struct{})
	}
	c.active[sessionID] = struct{}{}
	return nil
}

func (c *turnCoordinator) end(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.active, sessionID)
}
