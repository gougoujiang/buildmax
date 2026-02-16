// Package session provides in-memory chat sessions: id (UUID), title, created_at, and conversation history.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"buildmax/internal/llm"

	"github.com/google/uuid"
)

// ErrSessionNotFound is returned by LoadFromDir when the session file does not exist.
// Callers can use errors.Is(err, ErrSessionNotFound) to detect "load or create" cases.
var ErrSessionNotFound = errors.New("session not found")

// ctxKey is the type for context keys in this package (private to avoid collisions).
type ctxKey struct{}

var sessionIDKey = &ctxKey{}

// CtxWithSessionID returns a context that carries the given session ID.
// Tools can read it via SessionIDFromContext.
func CtxWithSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sessionIDKey, id)
}

// SessionIDFromContext returns the session ID from ctx, or ("", false) if not set.
func SessionIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(sessionIDKey).(string)
	return id, ok
}

// sessionFile is the JSON representation of a session on disk.
// Used only for encoding/decoding; Session's internal fields stay unexported.
// omitempty skips null/empty values so persisted JSON stays minimal.
type sessionFile struct {
	ID        string        `json:"id"`
	Title     string        `json:"title,omitempty"`
	CreatedAt string        `json:"created_at"` // RFC3339
	Messages  []llm.Message `json:"messages,omitempty"`
}

// Session holds conversation history (user, assistant, tool messages) and metadata.
// The system message is not stored; it is prepended at call time by the agent.
type Session struct {
	id        string
	title     string
	createdAt time.Time
	messages  []llm.Message
}

// NewSession creates a new session with a generated UUID, the given title,
// created_at set to the current time, and empty history. Title may be empty.
func NewSession(title string) *Session {
	return &Session{
		id:        uuid.New().String(),
		title:     title,
		createdAt: time.Now(),
		messages:  nil,
	}
}

// NewSessionFromData constructs a Session from persisted data (e.g. after LoadFromDir).
// Used to restore a session without exporting Session's internal fields.
func NewSessionFromData(id, title string, createdAt time.Time, messages []llm.Message) *Session {
	var msgs []llm.Message
	if len(messages) > 0 {
		msgs = make([]llm.Message, len(messages))
		copy(msgs, messages)
	}
	return &Session{
		id:        id,
		title:     title,
		createdAt: createdAt,
		messages:  msgs,
	}
}

// Append adds one message to the session's history. Caller must ensure
// role is user, assistant, or tool (system is not stored in session).
func (s *Session) Append(msg llm.Message) {
	s.messages = append(s.messages, msg)
}

// Messages returns a copy of the current conversation history so callers
// cannot mutate session state and the agent can safely append during the loop.
func (s *Session) Messages() []llm.Message {
	if len(s.messages) == 0 {
		return nil
	}
	out := make([]llm.Message, len(s.messages))
	copy(out, s.messages)
	return out
}

// ID returns the session id (UUID string).
func (s *Session) ID() string {
	return s.id
}

// Title returns the session title.
func (s *Session) Title() string {
	return s.title
}

// CreatedAt returns the creation timestamp.
func (s *Session) CreatedAt() time.Time {
	return s.createdAt
}

// SetTitle sets the session title (for future use).
func (s *Session) SetTitle(title string) {
	s.title = title
}

// SaveToDir serializes the session to JSON and writes it to dir/<s.ID()>.json.
// Creates dir (and parents) if it does not exist.
func SaveToDir(s *Session, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f := sessionFile{
		ID:        s.ID(),
		Title:     s.Title(),
		CreatedAt: s.CreatedAt().Format(time.RFC3339),
		Messages:  s.Messages(),
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, s.ID()+".json")
	return os.WriteFile(path, data, 0644)
}

// LoadFromDir reads dir/<sessionID>.json and returns a Session.
// Returns a clear error if the file is missing or the JSON is invalid.
func LoadFromDir(dir string, sessionID string) (*Session, error) {
	path := filepath.Join(dir, sessionID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
		}
		return nil, err
	}
	var f sessionFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("invalid session file: %w", err)
	}
	createdAt, err := time.Parse(time.RFC3339, f.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("invalid session file: bad created_at: %w", err)
	}
	return NewSessionFromData(f.ID, f.Title, createdAt, f.Messages), nil
}
