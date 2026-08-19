// Package session provides the core session model: id, title, created_at, and conversation history.
// File-based persistence lives in internal/agentapp (SessionStore).
package session

import (
	"context"
	"errors"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/util"

	"github.com/google/uuid"
)

// ErrSessionNotFound is returned when a session file does not exist.
var ErrSessionNotFound = errors.New("session not found")

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

// Session holds conversation history (user, assistant, tool messages) and metadata.
// The system message is not stored; it is prepended at call time by the agent.
// JSON tags match the on-disk session file format (snake_case).
type Session struct {
	ID               string        `json:"id"`
	Title            string        `json:"title,omitempty"`
	CreatedAt        time.Time     `json:"created_at"`
	Messages         []llm.Message `json:"messages,omitempty"`
	PromptTokens     int           `json:"prompt_tokens,omitempty"`
	CompletionTokens int           `json:"completion_tokens,omitempty"`
	// CompactionIdx is the index into Messages where the latest compaction boundary falls.
	// Messages before this index have been summarized into CompactionSummary.
	// Zero means no compaction has occurred.
	CompactionIdx     int    `json:"compaction_idx,omitempty"`
	CompactionSummary string `json:"compaction_summary,omitempty"`
	// NoteEntries and TodoEntries are durable session state: unlike a tool result, they are
	// not messages, so compaction cannot take them. The fields are named apart from the
	// Notes/Todos accessors that implement agent.NoteStore; the JSON keys are the plain names.
	NoteEntries []agent.Note `json:"notes,omitempty"`
	TodoEntries []agent.Todo `json:"todos,omitempty"`
}

// SessionItem is one session's metadata in the session index file (sessions.json).
type SessionItem struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	CreatedAt string `json:"created_at"` // RFC3339
	Pinned    bool   `json:"pinned,omitempty"`
}

// NewID returns a new session ID.
func NewID() string {
	return uuid.New().String()
}

// NewSession creates a new session with a generated UUID, the given title,
// created_at set to the current time, and empty history. Title may be empty.
func NewSession(title string) *Session {
	return &Session{
		ID:        NewID(),
		Title:     title,
		CreatedAt: time.Now(),
	}
}

// NewSessionFromData constructs a Session from persisted data.
func NewSessionFromData(id, title string, createdAt time.Time, messages []llm.Message, promptTokens, completionTokens int) *Session {
	var msgs []llm.Message
	if len(messages) > 0 {
		msgs = make([]llm.Message, len(messages))
		copy(msgs, messages)
	}
	return &Session{
		ID:               id,
		Title:            title,
		CreatedAt:        createdAt,
		Messages:         msgs,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
	}
}

// Append adds one message to the session's history.
func (s *Session) Append(msg llm.Message) error {
	s.Messages = append(s.Messages, msg)
	return nil
}

// HistoryMessages returns the LLM-facing message slice. When a compaction boundary exists,
// only messages from CompactionIdx onward are returned; earlier messages have been summarized.
func (s *Session) HistoryMessages() []llm.Message {
	if s.CompactionIdx > 0 && s.CompactionIdx <= len(s.Messages) {
		return s.Messages[s.CompactionIdx:]
	}
	return s.Messages
}

// PriorSummary returns the summary stored by the most recent compaction, or "" when this
// session has never been compacted. Implements agent.CompactionHistory so RunLoop can feed
// the previous summary back into the next compaction instead of discarding what it covered.
func (s *Session) PriorSummary() string { return s.CompactionSummary }

// AddCompaction advances the compaction boundary by summarizedCount messages and stores the
// summary. The summary is expected to subsume any earlier one, so replacing is correct.
// Implements agent.CompactionHistory so RunLoop can persist the boundary across turns.
func (s *Session) AddCompaction(summary string, summarizedCount int) {
	s.CompactionSummary = summary
	s.CompactionIdx += summarizedCount
	if s.CompactionIdx > len(s.Messages) {
		s.CompactionIdx = len(s.Messages)
	}
}

// Notes returns the session's durable notes. Implements agent.NoteStore.
func (s *Session) Notes() []agent.Note { return s.NoteEntries }

// SetNotes replaces the session's notes, preserving the age of entries whose text is unchanged
// so a rewrite of the list does not make every entry look new. Implements agent.NoteStore.
func (s *Session) SetNotes(notes []agent.Note, iter int) {
	s.NoteEntries = agent.StampNotes(s.NoteEntries, notes, iter)
}

// Todos returns the session's durable task list. Implements agent.NoteStore.
func (s *Session) Todos() []agent.Todo { return s.TodoEntries }

// SetTodos replaces the session's task list, preserving the age of entries whose content and
// status are both unchanged. Implements agent.NoteStore.
func (s *Session) SetTodos(todos []agent.Todo, iter int) {
	s.TodoEntries = agent.StampTodos(s.TodoEntries, todos, iter)
}

// EnsureTitleFromFirstUserMessage sets the session title from the first user message
// if the title is empty, truncated to maxLen runes. No-op otherwise.
func EnsureTitleFromFirstUserMessage(s *Session, maxLen int) {
	if s.Title != "" {
		return
	}
	for _, m := range s.Messages {
		if m.Role == "user" {
			s.Title = util.ClipRunes(m.Content, maxLen)
			return
		}
	}
}
