package agentapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/session"
	"github.com/gougoujiang/buildmax/internal/util"
)

// SessionManager manages file-based session lifecycle for AgentApp.
type SessionManager struct {
	dir string
}

func (s *SessionManager) Create(defaultModel string) *SessionContext {
	sess := session.NewSession("")
	return NewSessionContext(sess, defaultModel)
}

func (s *SessionManager) Load(id, defaultModel string) (*SessionContext, error) {
	sess, err := LoadSession(s.dir, id)
	if err != nil {
		return nil, err
	}
	return NewSessionContext(sess, defaultModel), nil
}

func (s *SessionManager) Save(sess *SessionContext, workspace string) error {
	if s == nil || sess == nil {
		return nil
	}
	session.EnsureTitleFromFirstUserMessage(sess.Session, 100)
	if err := saveSession(sess.Session, s.dir); err != nil {
		return err
	}
	return upsertSessionItem(s.dir, session.SessionItem{
		ID:        sess.ID,
		Title:     sess.Title,
		Workspace: workspace,
		CreatedAt: sess.CreatedAt.Format(time.RFC3339),
	})
}

func (s *SessionManager) List() ([]session.SessionItem, error) {
	return LoadSessionList(s.dir)
}

func (s *SessionManager) GenerateTitle(ctx context.Context, client llm.LLMClient, sess *SessionContext) (string, llm.Usage, error) {
	if s == nil || sess == nil {
		return "", llm.Usage{}, nil
	}
	const prompt = `Generate a short, descriptive title (3-8 words) for this conversation. Return ONLY the title text, nothing else. Do not use quotes or punctuation at the start or end.`
	msgs := []llm.Message{{Role: "system", Content: prompt}}
	// The title comes from the first user message and the first assistant reply
	// after it, so the loop stops as soon as it has both.
	var gotUser bool
	for _, m := range sess.Messages {
		if !gotUser && m.Role == "user" {
			msgs = append(msgs, llm.Message{Role: "user", Content: m.Content})
			gotUser = true
			continue
		}
		if gotUser && m.Role == "assistant" && m.Content != "" {
			msgs = append(msgs, llm.Message{Role: "assistant", Content: util.ClipRunes(m.Content, 500)})
			break
		}
	}
	if !gotUser {
		return "", llm.Usage{}, nil
	}
	slog.Debug("generating session title via LLM")
	completion, err := client.ChatCompletionBlocking(ctx, msgs, nil)
	if err != nil {
		return "", llm.Usage{}, err
	}
	usage := completion.Usage
	title := cleanTitle(completion.Content)
	slog.Debug("generated session title", "title", title)
	return title, usage, nil
}

// Finalize runs the post-turn flow: accumulate token usage, persist the session,
// and generate a title via LLM if one is not yet set.
func (s *SessionManager) Finalize(ctx context.Context, client llm.LLMClient, sess *SessionContext, workspace string, stats agent.RunStats) (TurnFinalizeResult, error) {
	if sess == nil {
		return TurnFinalizeResult{}, nil
	}
	sess.PromptTokens += stats.PromptTokens
	sess.CompletionTokens += stats.CompletionTokens
	sess.CacheReadTokens += stats.CacheReadTokens
	sess.CacheWriteTokens += stats.CacheWriteTokens
	if err := s.Save(sess, workspace); err != nil {
		return TurnFinalizeResult{}, fmt.Errorf("persist session: %w", err)
	}
	if sess.Title != "" || client == nil {
		return TurnFinalizeResult{}, nil
	}
	title, usage, err := s.GenerateTitle(ctx, client, sess)
	if err != nil {
		slog.Warn("LLM title generation failed", "err", err)
		return TurnFinalizeResult{}, nil
	}
	if title == "" {
		return TurnFinalizeResult{}, nil
	}
	sess.Title = title
	sess.PromptTokens += usage.PromptTokens
	sess.CompletionTokens += usage.CompletionTokens
	sess.CacheReadTokens += usage.CacheReadTokens
	sess.CacheWriteTokens += usage.CacheWriteTokens
	if err := s.Save(sess, workspace); err != nil {
		slog.Error("re-persist session with title failed", "err", err)
	}
	return TurnFinalizeResult{
		Title:            title,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		CacheReadTokens:  usage.CacheReadTokens,
		CacheWriteTokens: usage.CacheWriteTokens,
	}, nil
}

// LoadSessionList reads the session index from dir without requiring an AgentApp instance.
func LoadSessionList(dir string) ([]session.SessionItem, error) {
	path := filepath.Join(dir, "sessions.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []session.SessionItem{}, nil
		}
		return nil, err
	}
	var entries []session.SessionItem
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []session.SessionItem{}
	}
	return entries, nil
}

// DeleteSession removes a session from the index file and deletes its data file.
func DeleteSession(dir, id string) error {
	entries, err := LoadSessionList(dir)
	if err != nil {
		return err
	}
	filtered := make([]session.SessionItem, 0, len(entries))
	for _, e := range entries {
		if e.ID != id {
			filtered = append(filtered, e)
		}
	}
	data, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "sessions.json"), data, 0644); err != nil {
		return err
	}
	sessPath := filepath.Join(dir, id+".json")
	if err := os.Remove(sessPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func RenameSession(dir, id, title string) error {
	title = cleanTitle(title)
	entries, err := LoadSessionList(dir)
	if err != nil {
		return err
	}
	found := false
	for i := range entries {
		if entries[i].ID == id {
			entries[i].Title = title
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%w: %s", session.ErrSessionNotFound, id)
	}
	if err := writeSessionList(dir, entries); err != nil {
		return err
	}
	sess, err := LoadSession(dir, id)
	if err != nil {
		if errors.Is(err, session.ErrSessionNotFound) {
			return nil
		}
		return err
	}
	sess.Title = title
	return saveSession(sess, dir)
}

func SetSessionPinned(dir, id string, pinned bool) error {
	entries, err := LoadSessionList(dir)
	if err != nil {
		return err
	}
	found := false
	for i := range entries {
		if entries[i].ID == id {
			entries[i].Pinned = pinned
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%w: %s", session.ErrSessionNotFound, id)
	}
	return writeSessionList(dir, entries)
}

func DeleteSessionsByWorkspace(dir, workspace string) ([]string, error) {
	workspaces := workspaceAliases(workspace)
	entries, err := LoadSessionList(dir)
	if err != nil {
		return nil, err
	}
	remaining := make([]session.SessionItem, 0, len(entries))
	deleted := make([]string, 0)
	for _, e := range entries {
		if _, ok := workspaces[filepath.Clean(e.Workspace)]; ok && e.Workspace != "" {
			deleted = append(deleted, e.ID)
			continue
		}
		if abs, err := filepath.Abs(e.Workspace); err == nil {
			if _, ok := workspaces[filepath.Clean(abs)]; ok && e.Workspace != "" {
				deleted = append(deleted, e.ID)
				continue
			}
		}
		remaining = append(remaining, e)
	}
	if err := writeSessionList(dir, remaining); err != nil {
		return nil, err
	}
	for _, id := range deleted {
		if err := os.Remove(filepath.Join(dir, id+".json")); err != nil && !os.IsNotExist(err) {
			return deleted, err
		}
	}
	return deleted, nil
}

func workspaceAliases(workspace string) map[string]struct{} {
	out := make(map[string]struct{})
	if workspace == "" {
		return out
	}
	cleaned := filepath.Clean(workspace)
	out[cleaned] = struct{}{}
	if abs, err := filepath.Abs(workspace); err == nil {
		out[filepath.Clean(abs)] = struct{}{}
	}
	return out
}

// LoadSession reads a single session file from dir without requiring an AgentApp instance.
func LoadSession(dir, id string) (*session.Session, error) {
	path := filepath.Join(dir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", session.ErrSessionNotFound, id)
		}
		return nil, err
	}
	var sess session.Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("invalid session file: %w", err)
	}
	return &sess, nil
}

func saveSession(s *session.Session, dir string) error {
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

// UpsertSessionItem adds or updates one entry in the session index at dir/sessions.json.
// Exported for test setup; production writes go through SessionManager.Save.
func UpsertSessionItem(dir string, entry session.SessionItem) error {
	return upsertSessionItem(dir, entry)
}

func cleanTitle(s string) string {
	s = strings.TrimSpace(s)
	for _, q := range []string{`"`, `'`, "`"} {
		if len(s) >= 2 && strings.HasPrefix(s, q) && strings.HasSuffix(s, q) {
			s = s[len(q) : len(s)-len(q)]
		}
	}
	s = strings.TrimSpace(s)
	return util.ClipRunes(s, 100)
}

func upsertSessionItem(dir string, entry session.SessionItem) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	entries, err := LoadSessionList(dir)
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
		entries[found].Pinned = entries[found].Pinned || entry.Pinned
		// created_at unchanged
	} else {
		entries = append(entries, entry)
	}
	return writeSessionList(dir, entries)
}

func writeSessionList(dir string, entries []session.SessionItem) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "sessions.json")
	return os.WriteFile(path, data, 0644)
}
