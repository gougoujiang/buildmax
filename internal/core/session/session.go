// Package session provides the core session model: identity, the linked
// history journal, the reducer that replays it, recovery analysis, and the
// persistence seam.
//
// A session's durable form is the bundle described in
// docs/design/local-session-storage.md: metadata in meta.json, the conversation
// in an append-only history.jsonl, traces and artifacts alongside them. This
// package owns what those records mean and how a branch reduces to the state a
// resumed turn starts from. Making them durable belongs to
// internal/infra/sessionstore; deciding when state commits belongs to
// internal/agentapp.
package session

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ErrSessionNotFound is returned when a session does not exist.
var ErrSessionNotFound = errors.New("session not found")

// ErrLocked reports that something else already holds a session's writer lock.
//
// It lives here rather than in the file backend because Store.Open's contract
// names it: a caller deciding whether to report "busy" or to fail programs
// against the interface, and would otherwise have to import an implementation
// to ask a question the interface already answers.
var ErrLocked = errors.New("session is open in another process")

type ctxKey struct{}

var sessionIDKey = &ctxKey{}

// CtxWithSessionID returns a context that carries the given session ID.
func CtxWithSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sessionIDKey, id)
}

// SessionIDFromContext returns the session ID from ctx, or ("", false) if not set.
func SessionIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(sessionIDKey).(string)
	return id, ok
}

// NewID returns a new session ID.
func NewID() string {
	return uuid.New().String()
}
