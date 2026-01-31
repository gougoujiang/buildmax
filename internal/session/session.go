// Package session provides in-memory chat sessions: id (UUID), title, created_at, and conversation history.
package session

import (
	"time"

	"github.com/google/uuid"
	"buildmax/internal/llm"
)

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
