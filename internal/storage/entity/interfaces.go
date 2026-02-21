package entity

import (
	"context"
	"errors"
)

// UserStore looks up users by email and creates new users.
type UserStore interface {
	UserByEmail(ctx context.Context, email string) (*User, error)
	// CreateUser creates a user with the given email. Returns ErrEmailExists if the email is already registered.
	CreateUser(ctx context.Context, email string) (*User, error)
}

// WorkspaceStore provides workspace persistence.
type WorkspaceStore interface {
	EnsureDefaultWorkspaceForUser(ctx context.Context, userID string) error
	ListWorkspacesByOwner(ctx context.Context, userID string) ([]Workspace, error)
	// WorkspaceBelongsToUser returns true if the workspace is owned by the user.
	WorkspaceBelongsToUser(ctx context.Context, workspaceID, userID string) (bool, error)
	// CreateWorkspace creates a new workspace for the user and returns it.
	CreateWorkspace(ctx context.Context, userID, name string) (*Workspace, error)
}

// AgentStore provides agent persistence. Agents are workspace-scoped.
type AgentStore interface {
	ListAgentsByWorkspace(ctx context.Context, workspaceID string) ([]Agent, error)
	GetAgent(ctx context.Context, agentID string) (*Agent, error)
	CreateAgent(ctx context.Context, workspaceID, name, description, instructions string) (*Agent, error)
}

// ChatStore provides chat persistence. Chats belong to a workspace.
// CreateChat creates chat + first ChatRun (both in one transaction).
type ChatStore interface {
	// ListChatsByWorkspace returns chats in the workspace. order is "asc" (oldest first) or "desc" (latest first); default "desc".
	ListChatsByWorkspace(ctx context.Context, workspaceID string, order string) ([]Chat, error)
	// ListChatsByWorkspacePaginated returns chats with optional executed_only filter, ordered by created_at DESC. total is total matching count.
	ListChatsByWorkspacePaginated(ctx context.Context, workspaceID string, executedOnly bool, limit, offset int) ([]Chat, int, error)
	GetChat(ctx context.Context, chatID string) (*Chat, error)
	GetChatBySessionID(ctx context.Context, sessionID string) (*Chat, error)
	// CreateChat creates a new chat and its first ChatRun (input, title, PENDING). Returns the chat with last_run_id set.
	CreateChat(ctx context.Context, workspaceID, input, title, createdBy string) (*Chat, error)
	UpdateChatStatus(ctx context.Context, chatID, status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) error
	UpdateChatStatusIf(ctx context.Context, chatID, expectedStatus, newStatus string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) (updated bool, err error)
}

// ErrRunInProgress is returned by CreateChatRun when the chat already has a run in PENDING, SCHEDULED, or RUNNING.
var ErrRunInProgress = errors.New("chat has a run already in progress")

// ChatRunStore provides chat run persistence.
type ChatRunStore interface {
	// CreateChatRun creates a new run (PENDING). Returns ErrRunInProgress if chat has any run in PENDING/SCHEDULED/RUNNING.
	CreateChatRun(ctx context.Context, chatID, input, createdBy string) (*ChatRun, error)
	// GetNextPendingChatRun returns the oldest run with status PENDING (by created_at), or (nil, nil) if none.
	GetNextPendingChatRun(ctx context.Context) (*ChatRun, error)
	GetChatRun(ctx context.Context, chatRunID string) (*ChatRun, error)
	// GetChatRunWithChat returns the run and its chat, or (nil, nil, nil) if run not found.
	GetChatRunWithChat(ctx context.Context, chatRunID string) (*ChatRun, *Chat, error)
	// UpdateChatRunStatusIf atomically updates run status when current status equals expectedStatus. Returns updated.
	UpdateChatRunStatusIf(ctx context.Context, chatRunID, expectedStatus, newStatus string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) (bool, error)
	UpdateChatRunStatus(ctx context.Context, chatRunID, status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) error
	UpdateChatRunWorkerInfo(ctx context.Context, chatRunID, workerType string, k8sJobName *string, k8sJobCreatedAt *int64) error
	// OnRunComplete creates chat_run_output_file rows (one per relativePath) and updates chat denormalized fields. Use for SUCCEEDED runs.
	OnRunComplete(ctx context.Context, chatRunID string, relativePaths []string) error
	// SyncChatFromRun updates chat denormalized fields and last_run_id from the run (no output). Use for FAILED runs.
	SyncChatFromRun(ctx context.Context, chatRunID string) error
}
