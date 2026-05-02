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

	"buildmax/internal/core/model"

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
	ID               string          `json:"id"`
	Title            string          `json:"title,omitempty"`
	CreatedAt        string          `json:"created_at"` // RFC3339
	Messages         []model.Message `json:"messages,omitempty"`
	PromptTokens     int             `json:"prompt_tokens,omitempty"`
	CompletionTokens int             `json:"completion_tokens,omitempty"`
}

// Session holds conversation history (user, assistant, tool messages) and metadata.
// The system message is not stored; it is prepended at call time by the agent.
// promptTokens and completionTokens hold accumulated token usage across turns.
type Session struct {
	id               string
	title            string
	createdAt        time.Time
	messages         []model.Message
	promptTokens     int
	completionTokens int
}

// NewID returns a new session ID.
func NewID() string {
	return uuid.New().String()
}

// NewSession creates a new session with a generated UUID, the given title,
// created_at set to the current time, and empty history. Title may be empty.
func NewSession(title string) *Session {
	return &Session{
		id:        NewID(),
		title:     title,
		createdAt: time.Now(),
		messages:  nil,
	}
}

// NewSessionFromData constructs a Session from persisted data (e.g. after LoadFromDir).
// Used to restore a session without exporting Session's internal fields.
// promptTokens and completionTokens are the accumulated usage from the session file (0 for old files).
func NewSessionFromData(id, title string, createdAt time.Time, messages []model.Message, promptTokens, completionTokens int) *Session {
	var msgs []model.Message
	if len(messages) > 0 {
		msgs = make([]model.Message, len(messages))
		copy(msgs, messages)
	}
	return &Session{
		id:               id,
		title:            title,
		createdAt:        createdAt,
		messages:         msgs,
		promptTokens:     promptTokens,
		completionTokens: completionTokens,
	}
}

// Append adds one message to the session's history. Caller must ensure
// role is user, assistant, or tool (system is not stored in session).
func (s *Session) Append(msg model.Message) error {
	s.messages = append(s.messages, msg)
	return nil
}

// Messages returns a copy of the current conversation history so callers
// cannot mutate session state and the agent can safely append during the loop.
func (s *Session) Messages() []model.Message {
	if len(s.messages) == 0 {
		return nil
	}
	out := make([]model.Message, len(s.messages))
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

// SetTitle sets the session title.
func (s *Session) SetTitle(title string) {
	s.title = title
}

// PromptTokens returns the accumulated prompt token count for the session.
func (s *Session) PromptTokens() int {
	return s.promptTokens
}

// CompletionTokens returns the accumulated completion token count for the session.
func (s *Session) CompletionTokens() int {
	return s.completionTokens
}

// AddUsage adds this turn's token counts to the session's accumulated usage.
func (s *Session) AddUsage(promptTokens, completionTokens int) {
	s.promptTokens += promptTokens
	s.completionTokens += completionTokens
}

// SaveToDir serializes the session to JSON and writes it to dir/<s.ID()>.json.
// Creates dir (and parents) if it does not exist.
func SaveToDir(s *Session, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f := sessionFile{
		ID:               s.ID(),
		Title:            s.Title(),
		CreatedAt:        s.CreatedAt().Format(time.RFC3339),
		Messages:         s.Messages(),
		PromptTokens:     s.PromptTokens(),
		CompletionTokens: s.CompletionTokens(),
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, s.ID()+".json")
	return os.WriteFile(path, data, 0644)
}

// PersistAfterReply runs the full "persist after reply" flow: ensure title from first user message,
// save session to dir, build list entry from session and workspace, and upsert the list entry.
// Returns the first error encountered.
func PersistAfterReply(s *Session, dir, workspace string, maxTitleLen int) error {
	EnsureTitleFromFirstUserMessage(s, maxTitleLen)
	if err := SaveToDir(s, dir); err != nil {
		return err
	}
	entry := ListEntry{
		ID:        s.ID(),
		Title:     s.Title(),
		Workspace: workspace,
		CreatedAt: s.CreatedAt().Format(time.RFC3339),
	}
	return UpsertListEntry(dir, entry)
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
	return NewSessionFromData(f.ID, f.Title, createdAt, f.Messages, f.PromptTokens, f.CompletionTokens), nil
}
