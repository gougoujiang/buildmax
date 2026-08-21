package agent

import "sync"

// ApprovalDecision is what a user chose at an approval prompt.
type ApprovalDecision uint8

const (
	// ApprovalDeny blocks the call.
	ApprovalDeny ApprovalDecision = iota
	// ApprovalAllowOnce runs this call and asks again next time.
	ApprovalAllowOnce
	// ApprovalAllowSession runs this call and records a grant so the same
	// scope stops asking for the rest of the session.
	ApprovalAllowSession
)

// SessionGrants records approvals a user chose to keep for the session.
//
// Grants live in memory and die with the process. Persisting them means writing
// to the user's settings on their behalf, which is a different decision with a
// different blast radius — see docs/design/tool-permissions.md §7.
//
// The nil value is usable and grants nothing, so a surface that does not offer
// the session option needs no store.
type SessionGrants struct {
	mu     sync.Mutex
	scopes map[string]bool
}

// NewSessionGrants returns an empty store, suitable for one session.
func NewSessionGrants() *SessionGrants { return &SessionGrants{} }

// granted reports whether scope has been granted for this session.
func (g *SessionGrants) granted(scope string) bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.scopes[scope]
}

// grant records scope as approved for the rest of the session.
func (g *SessionGrants) grant(scope string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.scopes == nil {
		g.scopes = make(map[string]bool)
	}
	g.scopes[scope] = true
}

// Scopes returns the granted scopes, for display and tests.
func (g *SessionGrants) Scopes() []string {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]string, 0, len(g.scopes))
	for s := range g.scopes {
		out = append(out, s)
	}
	return out
}
