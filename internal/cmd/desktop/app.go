// Package desktop implements the BuildMax desktop app (Wails) and is used by cmd/buildmax-desktop.
package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"buildmax/internal/app/agentrun"
	"buildmax/internal/config"
	"buildmax/internal/session"
)

// SessionDetail is the session payload returned to the frontend for display.
type SessionDetail struct {
	ID        string          `json:"id"`
	Title     string          `json:"title,omitempty"`
	CreatedAt string          `json:"created_at"`
	Messages  []MessageDisplay `json:"messages,omitempty"`
}

// MessageDisplay is a single message for display (role + content).
type MessageDisplay struct {
	Role    string `json:"role"`
	Content string `json:"content,omitempty"`
}

// SendMessageResult is the result of SendMessage.
type SendMessageResult struct {
	Reply     string `json:"reply"`
	SessionID string `json:"session_id"`
}

// App holds desktop application state and implements Wails lifecycle hooks.
type App struct{}

// NewApp returns a new App instance.
func NewApp() *App {
	return &App{}
}

// Startup is called by Wails when the app is starting. It ensures the
// application data directory is set (config.DataDir / BUILDMAX_HOME) so
// future agent integration uses the same paths as the CLI.
func (a *App) Startup(ctx context.Context) {
	_ = config.DataDir()
}

// Shutdown is called by Wails when the app is shutting down. No-op for this task.
func (a *App) Shutdown(ctx context.Context) {}

// ListSessions returns the session list from disk (same as CLI).
func (a *App) ListSessions() ([]session.ListEntry, error) {
	dir := config.SessionsDir()
	entries, err := session.LoadList(dir)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	if entries == nil {
		entries = []session.ListEntry{}
	}
	return entries, nil
}

// GetSession loads one session by ID and returns it for display.
func (a *App) GetSession(sessionID string) (SessionDetail, error) {
	if sessionID == "" {
		return SessionDetail{}, fmt.Errorf("session ID required")
	}
	dir := config.SessionsDir()
	sess, err := session.LoadFromDir(dir, sessionID)
	if err != nil {
		if errors.Is(err, session.ErrSessionNotFound) {
			return SessionDetail{}, fmt.Errorf("session not found: %s", sessionID)
		}
		return SessionDetail{}, fmt.Errorf("load session: %w", err)
	}
	msgs := sess.Messages()
	display := make([]MessageDisplay, 0, len(msgs))
	for _, m := range msgs {
		display = append(display, MessageDisplay{Role: m.Role, Content: m.Content})
	}
	return SessionDetail{
		ID:        sess.ID(),
		Title:     sess.Title(),
		CreatedAt: sess.CreatedAt().Format(time.RFC3339),
		Messages:  display,
	}, nil
}

// SendMessage runs one user prompt in the given session (or creates a new session if sessionID is empty).
func (a *App) SendMessage(sessionID string, prompt string) (SendMessageResult, error) {
	if prompt == "" {
		return SendMessageResult{}, fmt.Errorf("prompt required")
	}
	workspace, err := os.Getwd()
	if err != nil {
		return SendMessageResult{}, fmt.Errorf("workspace: %w", err)
	}
	rt, err := agentrun.Open(agentrun.OpenInput{
		WorkspaceDir:  workspace,
		SessionID:     sessionID,
		ModelSelector: "",
	})
	if err != nil {
		return SendMessageResult{}, fmt.Errorf("open runtime: %w", err)
	}
	out, err := rt.RunPrompt(context.Background(), agentrun.RunInput{Prompt: prompt, Stream: nil})
	if err != nil {
		return SendMessageResult{}, fmt.Errorf("agent: %w", err)
	}
	return SendMessageResult{
		Reply:     out.Reply,
		SessionID: out.SessionID,
	}, nil
}
