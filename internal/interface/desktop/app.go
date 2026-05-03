// Package desktop implements the BuildMax desktop app (Wails) and is used by cmd/buildmax-desktop.
package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"buildmax/internal/config"
	"buildmax/internal/execution/agentrun"
	"buildmax/internal/interface/auth"
	"buildmax/internal/interface/client"
	"buildmax/internal/session"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// AuthStatus is the auth state returned to the frontend.
type AuthStatus struct {
	LoggedIn  bool   `json:"logged_in"`
	ServerURL string `json:"server_url,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	Email     string `json:"email,omitempty"`
	Name      string `json:"name,omitempty"`
}

// SessionDetail is the session payload returned to the frontend for display.
type SessionDetail struct {
	ID        string           `json:"id"`
	Title     string           `json:"title,omitempty"`
	CreatedAt string           `json:"created_at"`
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

// StreamDonePayload is emitted when streaming finishes successfully (event desktop/stream-done).
type StreamDonePayload struct {
	Reply     string `json:"reply"`
	SessionID string `json:"session_id"`
}

// StreamErrorPayload is emitted when streaming fails (event desktop/stream-error).
type StreamErrorPayload struct {
	Message string `json:"message"`
}

const (
	eventStreamDelta = "desktop/stream-delta"
	eventStreamDone  = "desktop/stream-done"
	eventStreamError = "desktop/stream-error"
)

// App holds desktop application state and implements Wails lifecycle hooks.
type App struct {
	ctx context.Context
	mu  sync.Mutex
}

// NewApp returns a new App instance.
func NewApp() *App {
	return &App{}
}

// Startup is called by Wails when the app is starting. It ensures the
// application data directory is set (config.DataDir / BUILDMAX_HOME) so
// future agent integration uses the same paths as the CLI.
func (a *App) Startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	a.mu.Unlock()
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

// GetAuthStatus returns the current authentication state for the frontend.
func (a *App) GetAuthStatus() (*AuthStatus, error) {
	creds, err := auth.Load(config.AuthPath())
	if err != nil {
		return nil, fmt.Errorf("load auth: %w", err)
	}
	if !creds.IsValid() {
		return &AuthStatus{LoggedIn: false}, nil
	}
	return &AuthStatus{
		LoggedIn:  true,
		ServerURL: creds.ServerURL,
		UserID:    creds.UserID,
		Email:     creds.Email,
		Name:      creds.Name,
	}, nil
}

// RequestOTP calls the server's OTP endpoint. intent is "login" or "signup".
func (a *App) RequestOTP(serverURL, email, intent string) error {
	c := client.NewClient(serverURL)
	return c.RequestOTP(context.Background(), email, intent)
}

// DoLogin authenticates against the server and saves credentials on success.
func (a *App) DoLogin(serverURL, email, otp string) (*AuthStatus, error) {
	c := client.NewClient(serverURL)
	lr, err := c.Login(context.Background(), email, otp, "desktop")
	if err != nil {
		return nil, err
	}
	creds := &auth.Credentials{
		ServerURL: serverURL,
		Token:     lr.Token,
		UserID:    lr.User.ID,
		Email:     lr.User.Email,
		Name:      lr.User.Name,
	}
	if err := auth.Save(creds, config.AuthPath()); err != nil {
		return nil, fmt.Errorf("save credentials: %w", err)
	}
	return &AuthStatus{
		LoggedIn:  true,
		ServerURL: serverURL,
		UserID:    lr.User.ID,
		Email:     lr.User.Email,
		Name:      lr.User.Name,
	}, nil
}

// Logout clears stored credentials.
func (a *App) Logout() error {
	return auth.Clear(config.AuthPath())
}

func (a *App) requireLogin() error {
	creds, err := auth.Load(config.AuthPath())
	if err != nil {
		return fmt.Errorf("not logged in")
	}
	if !creds.IsValid() {
		return fmt.Errorf("not logged in")
	}
	return nil
}

// SendMessage runs one user prompt in the given session (or creates a new session if sessionID is empty).
func (a *App) SendMessage(sessionID string, prompt string) (SendMessageResult, error) {
	if err := a.requireLogin(); err != nil {
		return SendMessageResult{}, err
	}
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

// desktopStreamSink emits each delta to the frontend via Wails runtime events.
type desktopStreamSink struct {
	ctx context.Context
}

func (s *desktopStreamSink) OnDelta(delta string) {
	runtime.EventsEmit(s.ctx, eventStreamDelta, delta)
}

// SendMessageStream runs one user prompt with streaming: it returns immediately and
// emits desktop/stream-delta (string) for each content chunk, then desktop/stream-done
// (StreamDonePayload) or desktop/stream-error (StreamErrorPayload). The frontend should
// subscribe to these events before calling SendMessageStream.
func (a *App) SendMessageStream(sessionID string, prompt string) error {
	if err := a.requireLogin(); err != nil {
		return err
	}
	if prompt == "" {
		return fmt.Errorf("prompt required")
	}
	workspace, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("workspace: %w", err)
	}
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx == nil {
		return fmt.Errorf("app not ready")
	}
	rt, err := agentrun.Open(agentrun.OpenInput{
		WorkspaceDir:  workspace,
		SessionID:     sessionID,
		ModelSelector: "",
	})
	if err != nil {
		return fmt.Errorf("open runtime: %w", err)
	}
	sink := &desktopStreamSink{ctx: ctx}
	go func() {
		out, err := rt.RunPrompt(ctx, agentrun.RunInput{Prompt: prompt, Stream: sink})
		if err != nil {
			runtime.EventsEmit(ctx, eventStreamError, &StreamErrorPayload{Message: err.Error()})
			return
		}
		runtime.EventsEmit(ctx, eventStreamDone, &StreamDonePayload{Reply: out.Reply, SessionID: out.SessionID})
	}()
	return nil
}
