// Package session provides in-memory chat sessions: id (UUID), title, created_at, and conversation history.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"buildmax/internal/core/model"

	"github.com/google/uuid"
)

// ErrSessionNotFound is returned by LoadFromDir when the session file does not exist.
// Callers can use errors.Is(err, ErrSessionNotFound) to detect "load or create" cases.
var ErrSessionNotFound = errors.New("session not found")

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

// Session holds conversation history (user, assistant, tool messages) and metadata.
// The system message is not stored; it is prepended at call time by the agent.
// JSON tags match the on-disk session file format (snake_case).
type Session struct {
	ID               string          `json:"id"`
	Title            string          `json:"title,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	Messages         []model.Message `json:"messages,omitempty"`
	PromptTokens     int             `json:"prompt_tokens,omitempty"`
	CompletionTokens int             `json:"completion_tokens,omitempty"`
}

// SessionItem is one session's metadata in the session index file (sessions.json).
type SessionItem struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	CreatedAt string `json:"created_at"` // RFC3339
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

// NewSessionFromData constructs a Session from persisted data (e.g. after LoadFromDir).
// promptTokens and completionTokens are accumulated usage (0 for old files).
func NewSessionFromData(id, title string, createdAt time.Time, messages []model.Message, promptTokens, completionTokens int) *Session {
	var msgs []model.Message
	if len(messages) > 0 {
		msgs = make([]model.Message, len(messages))
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

// Append adds one message to the session's history. Caller must ensure
// role is user, assistant, or tool (system is not stored in session).
func (s *Session) Append(msg model.Message) error {
	s.Messages = append(s.Messages, msg)
	return nil
}

// HistoryMessages returns the live message slice for the agent loop (implements agent.MessageHistory).
func (s *Session) HistoryMessages() []model.Message {
	return s.Messages
}

// EnsureTitleFromFirstUserMessage sets the session title from the first user message
// if the title is empty and at least one user message exists, truncated to maxLen runes.
// No-op otherwise.
func EnsureTitleFromFirstUserMessage(s *Session, maxLen int) {
	if s.Title != "" {
		return
	}
	for _, m := range s.Messages {
		if m.Role == "user" {
			content := m.Content
			if runes := []rune(content); len(runes) > maxLen {
				content = string(runes[:maxLen])
			}
			s.Title = content
			return
		}
	}
}

// SaveToDir serializes the session to JSON and writes it to dir/<ID>.json.
// Creates dir (and parents) if it does not exist.
func SaveToDir(s *Session, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, s.ID+".json")
	return os.WriteFile(path, data, 0644)
}

// LoadFromDir reads dir/<sessionID>.json and returns a Session.
// Returns ErrSessionNotFound if the file is missing.
func LoadFromDir(dir string, sessionID string) (*Session, error) {
	path := filepath.Join(dir, sessionID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
		}
		return nil, err
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("invalid session file: %w", err)
	}
	return &sess, nil
}

// PersistAfterReply runs the full post-reply flow: ensure title, save session file, upsert session index item.
// Returns the first error encountered.
func PersistAfterReply(s *Session, dir, workspace string, maxTitleLen int) error {
	EnsureTitleFromFirstUserMessage(s, maxTitleLen)
	if err := SaveToDir(s, dir); err != nil {
		return err
	}
	entry := SessionItem{
		ID:        s.ID,
		Title:     s.Title,
		Workspace: workspace,
		CreatedAt: s.CreatedAt.Format(time.RFC3339),
	}
	return UpsertSessionItem(dir, entry)
}

// LoadSessions reads dir/sessions.json and returns the list of entries.
// If the file is missing, returns an empty slice and nil error.
func LoadSessions(dir string) ([]SessionItem, error) {
	path := filepath.Join(dir, "sessions.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []SessionItem{}, nil
		}
		return nil, err
	}
	var entries []SessionItem
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []SessionItem{}
	}
	return entries, nil
}

// UpsertSessionItem loads dir/sessions.json, upserts the entry by ID, and writes it back.
// Creates dir if it does not exist.
func UpsertSessionItem(dir string, entry SessionItem) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	entries, err := LoadSessions(dir)
	if err != nil {
		return err
	}
	found := -1
	for i := range entries {
		if entries[i].ID == entry.ID {
			found = i
			break
		}
	}
	if found >= 0 {
		entries[found].Title = entry.Title
		entries[found].Workspace = entry.Workspace
		// created_at unchanged
	} else {
		entries = append(entries, entry)
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "sessions.json")
	return os.WriteFile(path, data, 0644)
}

// SortByCreatedAtDesc sorts entries in place by created_at descending (newest first).
// Entries with invalid created_at are placed last; stable order is kept among ties.
func SortByCreatedAtDesc(entries []SessionItem) {
	sort.SliceStable(entries, func(i, j int) bool {
		ti, ei := time.Parse(time.RFC3339, entries[i].CreatedAt)
		tj, ej := time.Parse(time.RFC3339, entries[j].CreatedAt)
		if ei != nil && ej != nil {
			return false
		}
		if ei != nil {
			return false
		}
		if ej != nil {
			return true
		}
		return ti.After(tj)
	})
}

// LastByCreatedAt returns the entry with the latest created_at, or nil if entries is empty.
// Entries with unparseable created_at are skipped.
func LastByCreatedAt(entries []SessionItem) *SessionItem {
	var best *SessionItem
	var bestT time.Time
	for i := range entries {
		t, err := time.Parse(time.RFC3339, entries[i].CreatedAt)
		if err != nil {
			continue
		}
		if best == nil || t.After(bestT) {
			best = &entries[i]
			bestT = t
		}
	}
	return best
}
