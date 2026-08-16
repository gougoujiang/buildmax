// Package desktop implements the BuildMax desktop app (Wails) and is used by cmd/buildmax-desktop.
package desktop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/core/session"
	"github.com/gougoujiang/buildmax/internal/interface/auth"
	"github.com/gougoujiang/buildmax/internal/interface/client"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// SessionDetail is the session payload returned to the frontend for display.
type SessionDetail struct {
	ID        string        `json:"id"`
	Title     string        `json:"title,omitempty"`
	CreatedAt string        `json:"created_at"`
	Messages  []llm.Message `json:"messages,omitempty"`
}

// ReplyPayload is returned when a desktop prompt completes successfully.
type ReplyPayload struct {
	Reply                 string `json:"reply"`
	SessionID             string `json:"session_id"`
	ContextTokens         int    `json:"context_tokens"`
	ContextWindow         int    `json:"context_window"`
	PromptTokens          int    `json:"prompt_tokens"`
	CompletionTokens      int    `json:"completion_tokens"`
	TotalPromptTokens     int    `json:"total_prompt_tokens"`
	TotalCompletionTokens int    `json:"total_completion_tokens"`
}

// StreamErrorPayload is emitted when streaming fails (event desktop/stream-error).
type StreamErrorPayload struct {
	Message string `json:"message"`
}

// ToolStartPayload is emitted when a tool call begins executing.
type ToolStartPayload struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	Args       string `json:"args"`
}

// ToolEndPayload is emitted when a tool call finishes or is denied.
type ToolEndPayload struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	IsError    bool   `json:"is_error,omitempty"`
	Denied     bool   `json:"denied,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type RunStatusPayload struct {
	ContextTokens         int `json:"context_tokens"`
	ContextWindow         int `json:"context_window"`
	PromptTokens          int `json:"prompt_tokens"`
	CompletionTokens      int `json:"completion_tokens"`
	TotalPromptTokens     int `json:"total_prompt_tokens"`
	TotalCompletionTokens int `json:"total_completion_tokens"`
}

const (
	eventStreamDelta = "desktop/stream-delta"
	eventStreamDone  = "desktop/stream-done"
	eventStreamError = "desktop/stream-error"
	eventLLMStart    = "desktop/llm-start"
	eventToolStart   = "desktop/tool-start"
	eventToolEnd     = "desktop/tool-end"
	eventRunStatus   = "desktop/run-status"
)

// App holds desktop application state and implements Wails lifecycle hooks.
// Each project gets its own AgentApp and ApprovalHandler instance, created lazily on first use.
type App struct {
	ctx              context.Context
	mu               sync.Mutex
	agentApps        map[string]*agentapp.AgentApp      // keyed by project ID
	approvalHandlers map[string]*DesktopApprovalHandler // keyed by project ID
	// runCancels holds cancel funcs for in-flight runs, keyed by project ID.
	// At most one run is permitted per project at a time (see SendMessageStream).
	runCancels map[string]context.CancelFunc
}

// NewApp returns a new App instance.
func NewApp() *App {
	return &App{
		agentApps:        make(map[string]*agentapp.AgentApp),
		approvalHandlers: make(map[string]*DesktopApprovalHandler),
		runCancels:       make(map[string]context.CancelFunc),
	}
}

// Startup is called by Wails when the app is starting.
// AgentApp instances are created lazily on first use per project.
func (a *App) Startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	a.mu.Unlock()
	_ = config.DataDir()
}

// Shutdown closes all per-project AgentApp instances and cancels any in-flight runs.
func (a *App) Shutdown(_ context.Context) {
	a.mu.Lock()
	apps := a.agentApps
	cancels := a.runCancels
	a.agentApps = make(map[string]*agentapp.AgentApp)
	a.approvalHandlers = make(map[string]*DesktopApprovalHandler)
	a.runCancels = make(map[string]context.CancelFunc)
	a.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	for _, ag := range apps {
		_ = ag.Close()
	}
}

// agentAppForProject returns the cached AgentApp for the given project, creating
// it on first use. Concurrent calls for the same project are safe; at most one
// instance is created per project.
func (a *App) agentAppForProject(projectID string) (*agentapp.AgentApp, error) {
	a.mu.Lock()
	ag, ok := a.agentApps[projectID]
	a.mu.Unlock()
	if ok {
		return ag, nil
	}

	projects, err := readProjects()
	if err != nil {
		return nil, err
	}
	var proj *Project
	for i := range projects {
		if projects[i].ID == projectID {
			proj = &projects[i]
			break
		}
	}
	if proj == nil {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}

	handler := newDesktopApprovalHandler(a, projectID)
	ag, err = agentapp.NewAgentApp(agentapp.AppConfig{
		WorkspaceDir: proj.FolderPath,
		EnableMCP:    true,
		Policy:       agentapp.NewInteractivePolicy(),
		ManagedToken: auth.TokenForServer,
		Surface:      model.LLMCallSurfaceDesktop,
	})
	if err != nil {
		return nil, fmt.Errorf("init agent for project %q: %w", proj.Name, err)
	}

	a.mu.Lock()
	// Another goroutine may have created it while we were initializing.
	if existing, ok := a.agentApps[projectID]; ok {
		a.mu.Unlock()
		_ = ag.Close()
		return existing, nil
	}
	a.agentApps[projectID] = ag
	a.approvalHandlers[projectID] = handler
	a.mu.Unlock()
	return ag, nil
}

// --- Project bindings ---

// ListProjects returns all saved projects.
func (a *App) ListProjects() ([]Project, error) {
	return readProjects()
}

// OpenFolderDialog opens a native directory picker and returns the selected path.
func (a *App) OpenFolderDialog() (string, error) {
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx == nil {
		return "", fmt.Errorf("app not ready")
	}
	path, err := runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{
		Title: "Select Project Folder",
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

// CreateProject creates a new project with the given name and folder.
func (a *App) CreateProject(name, folderPath string) (*Project, error) {
	if name == "" {
		return nil, fmt.Errorf("project name required")
	}
	if folderPath == "" {
		return nil, fmt.Errorf("folder path required")
	}
	projects, err := readProjects()
	if err != nil {
		return nil, err
	}
	p := makeProject(name, folderPath)
	projects = append(projects, p)
	if err := writeProjects(projects); err != nil {
		return nil, err
	}
	return &p, nil
}

// RenameProject updates the name of a project.
func (a *App) RenameProject(id, newName string) error {
	if newName == "" {
		return fmt.Errorf("project name required")
	}
	projects, err := readProjects()
	if err != nil {
		return err
	}
	for i := range projects {
		if projects[i].ID == id {
			projects[i].Name = newName
			return writeProjects(projects)
		}
	}
	return fmt.Errorf("project not found: %s", id)
}

// DeleteProject removes a project and closes its AgentApp if one was running.
func (a *App) DeleteProject(id string) error {
	projects, err := readProjects()
	if err != nil {
		return err
	}
	remaining := make([]Project, 0, len(projects))
	found := false
	for _, p := range projects {
		if p.ID == id {
			found = true
			continue
		}
		remaining = append(remaining, p)
	}
	if !found {
		return fmt.Errorf("project not found: %s", id)
	}
	if err := writeProjects(remaining); err != nil {
		return err
	}
	a.mu.Lock()
	ag := a.agentApps[id]
	delete(a.agentApps, id)
	delete(a.approvalHandlers, id)
	a.mu.Unlock()
	if ag != nil {
		_ = ag.Close()
	}
	return nil
}

// --- Session bindings ---

// ListSessions returns all sessions across all projects.
func (a *App) ListSessions() ([]session.SessionItem, error) {
	entries, err := agentapp.LoadSessionList(config.SessionsDir())
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	if entries == nil {
		entries = []session.SessionItem{}
	}
	return entries, nil
}

func (a *App) RenameSession(sessionID, title string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID required")
	}
	return agentapp.RenameSession(config.SessionsDir(), sessionID, title)
}

func (a *App) DeleteSession(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID required")
	}
	return agentapp.DeleteSession(config.SessionsDir(), sessionID)
}

func (a *App) SetSessionPinned(sessionID string, pinned bool) error {
	if sessionID == "" {
		return fmt.Errorf("session ID required")
	}
	return agentapp.SetSessionPinned(config.SessionsDir(), sessionID, pinned)
}

func (a *App) ClearProjectSessions(projectID string) ([]string, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project ID required")
	}
	projects, err := readProjects()
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		if p.ID == projectID {
			return agentapp.DeleteSessionsByWorkspace(config.SessionsDir(), p.FolderPath)
		}
	}
	return nil, fmt.Errorf("project not found: %s", projectID)
}

// GetSession loads one session by ID and returns it for display.
func (a *App) GetSession(sessionID string) (SessionDetail, error) {
	if sessionID == "" {
		return SessionDetail{}, fmt.Errorf("session ID required")
	}
	sess, err := agentapp.LoadSession(config.SessionsDir(), sessionID)
	if err != nil {
		if errors.Is(err, session.ErrSessionNotFound) {
			return SessionDetail{}, fmt.Errorf("session not found: %s", sessionID)
		}
		return SessionDetail{}, fmt.Errorf("load session: %w", err)
	}
	display := make([]llm.Message, 0, len(sess.Messages))
	for _, m := range sess.Messages {
		if m.Role == "system" {
			continue
		}
		display = append(display, m)
	}
	return SessionDetail{
		ID:        sess.ID,
		Title:     sess.Title,
		CreatedAt: sess.CreatedAt.Format(time.RFC3339),
		Messages:  display,
	}, nil
}

func (a *App) GetRunStatus(projectID, sessionID string) (RunStatusPayload, error) {
	if projectID == "" {
		return RunStatusPayload{}, fmt.Errorf("project ID required")
	}
	ag, err := a.agentAppForProject(projectID)
	if err != nil {
		return RunStatusPayload{}, err
	}
	sess, err := ag.OpenSession(sessionID)
	if err != nil {
		return RunStatusPayload{}, fmt.Errorf("open session: %w", err)
	}
	st, err := ag.EstimateRunStatus(sess)
	if err != nil {
		return RunStatusPayload{}, err
	}
	return RunStatusPayload{
		ContextTokens:         st.ContextTokens,
		ContextWindow:         st.ContextWindow,
		PromptTokens:          st.PromptTokens,
		CompletionTokens:      st.CompletionTokens,
		TotalPromptTokens:     st.TotalPromptTokens,
		TotalCompletionTokens: st.TotalCompletionTokens,
	}, nil
}

// --- Auth bindings ---

// GetAuthStatus returns the current authentication state for the frontend.
func (a *App) GetAuthStatus() (*auth.AuthInfo, error) {
	info, err := auth.Info()
	if err != nil {
		return nil, fmt.Errorf("load auth: %w", err)
	}
	if !info.LoggedIn {
		return &auth.AuthInfo{LoggedIn: false}, nil
	}
	return &auth.AuthInfo{
		LoggedIn:  true,
		ServerURL: info.ServerURL,
		UserID:    info.UserID,
		Email:     info.Email,
		Name:      info.Name,
	}, nil
}

// RequestOTP calls the server's OTP endpoint.
func (a *App) RequestOTP(serverURL, email, intent string) error {
	c := client.NewClient(serverURL)
	return c.RequestOTP(context.Background(), email, intent)
}

// DoLogin authenticates against the server and saves credentials on success.
func (a *App) DoLogin(serverURL, email, otp string) (*auth.AuthInfo, error) {
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
	if err := auth.SaveCredentials(creds); err != nil {
		return nil, fmt.Errorf("save credentials: %w", err)
	}
	return &auth.AuthInfo{
		LoggedIn:  true,
		ServerURL: serverURL,
		UserID:    lr.User.ID,
		Email:     lr.User.Email,
		Name:      lr.User.Name,
	}, nil
}

// Logout clears stored credentials.
func (a *App) Logout() error {
	return auth.Logout()
}

// --- Chat bindings ---

// RespondApproval is called by the frontend when the user approves or denies a tool call.
// projectID must match the project that triggered the desktop/approval-request event.
func (a *App) RespondApproval(projectID string, approved bool) {
	a.mu.Lock()
	handler := a.approvalHandlers[projectID]
	a.mu.Unlock()
	if handler != nil {
		handler.respond(approved)
	}
}

// desktopStreamSink emits each delta to the frontend via Wails events.
type desktopStreamSink struct {
	ctx context.Context
}

func (s *desktopStreamSink) OnDelta(delta string) {
	runtime.EventsEmit(s.ctx, eventStreamDelta, delta)
}

// desktopEventSink returns an agent.EventSink that forwards tool events to the frontend via Wails events.
func desktopEventSink(ctx context.Context) func(agent.Event) {
	return func(e agent.Event) {
		switch e.Kind {
		case agent.EventLLMStart:
			runtime.EventsEmit(ctx, eventLLMStart, nil)
			runtime.EventsEmit(ctx, eventRunStatus, &RunStatusPayload{
				ContextTokens:    e.ContextTokens,
				ContextWindow:    e.ContextWindow,
				PromptTokens:     e.PromptTokens,
				CompletionTokens: e.CompletionTokens,
			})
		case agent.EventLLMEnd:
			runtime.EventsEmit(ctx, eventRunStatus, &RunStatusPayload{
				PromptTokens:     e.PromptTokens,
				CompletionTokens: e.CompletionTokens,
			})
		case agent.EventToolStart:
			runtime.EventsEmit(ctx, eventToolStart, &ToolStartPayload{ToolCallID: e.ToolCallID, ToolName: e.ToolName, Args: e.ToolArgs})
		case agent.EventToolEnd:
			runtime.EventsEmit(ctx, eventToolEnd, &ToolEndPayload{
				ToolCallID: e.ToolCallID,
				ToolName:   e.ToolName,
				DurationMs: e.ToolDuration.Milliseconds(),
				IsError:    strings.HasPrefix(e.ToolResult, "error:"),
			})
		case agent.EventToolDenied:
			runtime.EventsEmit(ctx, eventToolEnd, &ToolEndPayload{ToolCallID: e.ToolCallID, ToolName: e.ToolName, IsError: true, Denied: true, Reason: e.DenyReason})
		}
	}
}

// SendMessageStream runs a prompt in the given project and session with streaming.
// It returns immediately and emits desktop/stream-delta, then desktop/stream-done
// or desktop/stream-error. sessionID may be empty to start a new session.
//
// At most one run per project may be in flight; a second call while a run is
// active returns an error so the frontend can surface it.
func (a *App) SendMessageStream(projectID, sessionID, prompt string) error {
	if projectID == "" {
		return fmt.Errorf("project ID required")
	}
	if prompt == "" {
		return fmt.Errorf("prompt required")
	}
	ag, err := a.agentAppForProject(projectID)
	if err != nil {
		return err
	}
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx == nil {
		return fmt.Errorf("app not ready")
	}
	sess, err := ag.OpenSession(sessionID)
	if err != nil {
		return fmt.Errorf("open session: %w", err)
	}
	a.mu.Lock()
	if _, busy := a.runCancels[projectID]; busy {
		a.mu.Unlock()
		return fmt.Errorf("a run is already in progress for this project")
	}
	handler := a.approvalHandlers[projectID]
	runCtx, cancel := context.WithCancel(ctx)
	a.runCancels[projectID] = cancel
	a.mu.Unlock()

	sink := &desktopStreamSink{ctx: ctx}
	evSink := desktopEventSink(ctx)
	go func() {
		defer func() {
			a.mu.Lock()
			delete(a.runCancels, projectID)
			a.mu.Unlock()
			cancel()
		}()
		out, err := ag.RunPrompt(runCtx, sess, prompt, sink, handler, evSink)
		if err != nil {
			runtime.EventsEmit(ctx, eventStreamError, &StreamErrorPayload{Message: err.Error()})
			return
		}
		// Update last_used_at so the sidebar can order projects by recency.
		touchProjectLastUsed(projectID)
		runtime.EventsEmit(ctx, eventStreamDone, &ReplyPayload{
			Reply:                 out.Reply,
			SessionID:             out.SessionID,
			ContextTokens:         out.ContextTokens,
			ContextWindow:         out.ContextWindow,
			PromptTokens:          out.PromptTokens,
			CompletionTokens:      out.CompletionTokens,
			TotalPromptTokens:     out.TotalPromptTokens,
			TotalCompletionTokens: out.TotalCompletionTokens,
		})
	}()
	return nil
}

// CancelRun cancels the in-flight run for the given project, if any.
// Cancellation is cooperative: the agent loop returns the partial assistant
// reply produced so far and emits desktop/stream-done as a normal completion.
// Calling CancelRun when no run is in flight is a no-op.
func (a *App) CancelRun(projectID string) error {
	if projectID == "" {
		return fmt.Errorf("project ID required")
	}
	a.mu.Lock()
	cancel := a.runCancels[projectID]
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}
