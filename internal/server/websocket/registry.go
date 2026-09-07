package websocket

import (
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// ConnRegistry tracks active WebSocket connections per user.
//
// A single replica holds only the connections that landed on it. When a
// publisher is set, an event is published to the shared coordination bus instead
// of being fanned out inline, and every replica's Consume loop — including this
// one's — delivers it to the connections that replica holds. That is what makes
// an event raised on one replica reach a socket on another. See
// docs/design/server-coordination.md §6.
type ConnRegistry struct {
	mu    sync.RWMutex
	conns map[string][]*Conn
	// publish, when set, sends a marshalled busEnvelope to the coordination bus.
	// Nil selects the single-instance path: Broadcast fans out locally.
	publish func([]byte)
}

func NewConnRegistry() *ConnRegistry {
	return &ConnRegistry{conns: make(map[string][]*Conn)}
}

// SetPublisher routes broadcasts through the coordination bus. It is called once
// at assembly when a Redis coordination backend is configured.
func (r *ConnRegistry) SetPublisher(fn func([]byte)) {
	r.mu.Lock()
	r.publish = fn
	r.mu.Unlock()
}

// busEnvelope is one broadcast crossing the coordination bus. Data is the
// already-encoded event so a receiving replica writes it without re-encoding.
type busEnvelope struct {
	SpaceID string `json:"space_id"`
	UserID  string `json:"user_id"`
	Data    []byte `json:"data"`
}

// Consume delivers envelopes received from the coordination bus to this replica's
// own connections. It runs until incoming is closed, which the bus does on
// shutdown.
func (r *ConnRegistry) Consume(incoming <-chan []byte) {
	for raw := range incoming {
		var env busEnvelope
		if err := json.Unmarshal(raw, &env); err != nil {
			slog.With("component", "websocket").Warn("coordination envelope decode", "err", err)
			continue
		}
		r.deliverRaw(env.SpaceID, env.UserID, env.Data)
	}
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

// audience returns every connection that may be looking at a space's resources.
//
// A space is the authorization boundary for what a Portal page can read, so it is
// also the right audience for a change to something that page shows. userID is
// the fallback for a task created before tasks carried a space: it still has an
// owner, and losing the event entirely is worse than telling only them.
func (r *ConnRegistry) audience(spaceID, userID string) []*Conn {
	if spaceID == "" {
		return r.ForUser(userID)
	}
	r.mu.RLock()
	var out []*Conn
	for _, list := range r.conns {
		for _, c := range list {
			if c.spaceID == spaceID {
				out = append(out, c)
			}
		}
	}
	r.mu.RUnlock()
	return out
}

// Broadcast sends one event to every connection watching the space.
//
// Nothing here picks a single connection. A person reads the Portal from as many
// tabs as they like and from none at all, and a spacemate reads the same
// conversation from their own; an event that reaches one of those and not the
// rest leaves the others showing state that has already changed.
//
// The event is encoded once. With a publisher set it goes to the coordination
// bus and this replica delivers it through its own Consume loop, so the publish
// path never fans out locally and no connection is written twice.
func (r *ConnRegistry) Broadcast(spaceID, userID, eventType string, payload any) {
	data, err := Encode(eventType, payload)
	if err != nil {
		slog.With("component", "websocket").Warn("broadcast encode", "type", eventType, "err", err)
		return
	}
	r.mu.RLock()
	publish := r.publish
	r.mu.RUnlock()
	if publish != nil {
		env, err := json.Marshal(busEnvelope{SpaceID: spaceID, UserID: userID, Data: data})
		if err != nil {
			slog.With("component", "websocket").Warn("broadcast envelope encode", "err", err)
			return
		}
		publish(env)
		return
	}
	r.deliverRaw(spaceID, userID, data)
}

// deliverRaw writes an already-encoded event to every local connection watching
// the space.
func (r *ConnRegistry) deliverRaw(spaceID, userID string, data []byte) {
	for _, c := range r.audience(spaceID, userID) {
		c.sendRaw(data)
	}
}

// BroadcastSink streams one turn's deltas to everyone watching the space.
//
// A turn the server starts on its own — reporting a finished run, say — has no
// socket of its own to write to. This is what it streams through instead, and it
// costs nothing when nobody is connected.
func (r *ConnRegistry) BroadcastSink(spaceID, userID, conversationID string) llm.StreamSink {
	return &broadcastSink{registry: r, spaceID: spaceID, userID: userID, conversationID: conversationID}
}

type broadcastSink struct {
	registry       *ConnRegistry
	spaceID        string
	userID         string
	conversationID string
}

func (s *broadcastSink) OnDelta(delta string) {
	s.registry.Broadcast(s.spaceID, s.userID, TypeMessageDelta, MessageDelta{
		ConversationID: s.conversationID,
		Delta:          delta,
	})
}
