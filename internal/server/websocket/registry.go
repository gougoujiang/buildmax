package websocket

import (
	"sync"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// ConnRegistry tracks active WebSocket connections per user.
type ConnRegistry struct {
	mu    sync.RWMutex
	conns map[string][]*Conn
}

func NewConnRegistry() *ConnRegistry {
	return &ConnRegistry{conns: make(map[string][]*Conn)}
}

func (r *ConnRegistry) Register(userID string, c *Conn) {
	r.mu.Lock()
	r.conns[userID] = append(r.conns[userID], c)
	r.mu.Unlock()
}

func (r *ConnRegistry) Unregister(userID string, c *Conn) {
	r.mu.Lock()
	list := r.conns[userID]
	for i, cc := range list {
		if cc == c {
			r.conns[userID] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(r.conns[userID]) == 0 {
		delete(r.conns, userID)
	}
	r.mu.Unlock()
}

func (r *ConnRegistry) ForUser(userID string) []*Conn {
	r.mu.RLock()
	list := r.conns[userID]
	out := make([]*Conn, len(list))
	copy(out, list)
	r.mu.RUnlock()
	return out
}

// audience returns every connection that may be looking at a team's resources.
//
// A team is the authorization boundary for what a Portal page can read, so it is
// also the right audience for a change to something that page shows. userID is
// the fallback for a task created before tasks carried a team: it still has an
// owner, and losing the event entirely is worse than telling only them.
func (r *ConnRegistry) audience(teamID, userID string) []*Conn {
	if teamID == "" {
		return r.ForUser(userID)
	}
	r.mu.RLock()
	var out []*Conn
	for _, list := range r.conns {
		for _, c := range list {
			if c.teamID == teamID {
				out = append(out, c)
			}
		}
	}
	r.mu.RUnlock()
	return out
}

// Broadcast sends one event to every connection watching the team.
//
// Nothing here picks a single connection. A person reads the Portal from as many
// tabs as they like and from none at all, and a teammate reads the same
// conversation from their own; an event that reaches one of those and not the
// rest leaves the others showing state that has already changed.
func (r *ConnRegistry) Broadcast(teamID, userID, eventType string, payload any) {
	for _, c := range r.audience(teamID, userID) {
		c.sendEvent(eventType, payload)
	}
}

// BroadcastSink streams one turn's deltas to everyone watching the team.
//
// A turn the server starts on its own — reporting a finished run, say — has no
// socket of its own to write to. This is what it streams through instead, and it
// costs nothing when nobody is connected.
func (r *ConnRegistry) BroadcastSink(teamID, userID, conversationID string) llm.StreamSink {
	return &broadcastSink{registry: r, teamID: teamID, userID: userID, conversationID: conversationID}
}

type broadcastSink struct {
	registry       *ConnRegistry
	teamID         string
	userID         string
	conversationID string
}

func (s *broadcastSink) OnDelta(delta string) {
	s.registry.Broadcast(s.teamID, s.userID, TypeMessageDelta, MessageDelta{
		ConversationID: s.conversationID,
		Delta:          delta,
	})
}
