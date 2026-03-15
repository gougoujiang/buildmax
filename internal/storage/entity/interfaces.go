package entity

import (
	"context"
	"errors"
)

// UserStore looks up users by email and creates new users.
type UserStore interface {
	UserByEmail(ctx context.Context, email string) (*User, error)
	// GetUser returns the user by user_id, or (nil, nil) when not found.
	GetUser(ctx context.Context, userID string) (*User, error)
	// CreateUser creates a user with the given email. defaultQuotaTier is applied when non-empty. Returns ErrEmailExists if the email is already registered.
	CreateUser(ctx context.Context, email string, defaultQuotaTier string) (*User, error)
}

// QuotaTierStore provides quota tier limits by tier name.
type QuotaTierStore interface {
	// GetQuotaTier returns the tier limits by tier name, or (nil, nil) when not found.
	GetQuotaTier(ctx context.Context, tierName string) (*QuotaTier, error)
}

// UsageInWindowReader provides usage aggregation for a user in a time window.
type UsageInWindowReader interface {
	// UserUsageInWindow returns run count and total tokens for the user in [sinceUnix, untilUnix].
	UserUsageInWindow(ctx context.Context, userID string, sinceUnix, untilUnix int64) (runCount, totalTokens int, err error)
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
	UpdateAgent(ctx context.Context, agentID, workspaceID, name, description, instructions string) (*Agent, error)
	DeleteAgent(ctx context.Context, agentID, workspaceID string) error
}

// TaskStore provides task persistence. Tasks belong to a workspace.
// CreateChat creates a task plus its first TaskRun (both in one transaction).
type TaskStore interface {
	// ListChatsByWorkspace returns tasks in the workspace. order is "asc" (oldest first) or "desc" (latest first); default "desc".
	ListChatsByWorkspace(ctx context.Context, workspaceID string, order string) ([]Chat, error)
	// ListChatsByWorkspacePaginated returns tasks with optional executed_only filter, ordered by created_at DESC. total is total matching count.
	ListChatsByWorkspacePaginated(ctx context.Context, workspaceID string, executedOnly bool, limit, offset int) ([]Chat, int, error)
	GetChat(ctx context.Context, chatID string) (*Chat, error)
	GetChatBySessionID(ctx context.Context, sessionID string) (*Chat, error)
	// CreateChat creates a new task and its first TaskRun (input, title, PENDING). Returns the task with last_run_id set.
	CreateChat(ctx context.Context, in *CreateChatInput) (*Chat, error)
	UpdateChat(ctx context.Context, in UpdateChatInput) error
	ClaimChat(ctx context.Context, in ClaimChatInput) (updated bool, err error)
}

// ErrRunInProgress is returned by CreateTaskRun when the task already has a run in PENDING, SCHEDULED, or RUNNING.
var ErrRunInProgress = errors.New("task has a run already in progress")

// TaskRunStore provides task run persistence.
type TaskRunStore interface {
	// CreateTaskRun creates a new run (PENDING). Returns ErrRunInProgress if the task has any run in PENDING/SCHEDULED/RUNNING.
	CreateTaskRun(ctx context.Context, chatID, input, createdBy string) (*TaskRun, error)
	// GetNextPendingTaskRun returns the oldest run with status PENDING (by created_at), or (nil, nil) if none.
	GetNextPendingTaskRun(ctx context.Context) (*TaskRun, error)
	GetTaskRun(ctx context.Context, chatRunID string) (*TaskRun, error)
	// GetTaskRunWithChat returns the run and its task, or (nil, nil, nil) if run not found.
	GetTaskRunWithChat(ctx context.Context, chatRunID string) (*TaskRun, *Chat, error)
	// ClaimTaskRun atomically updates a run when current status matches ExpectedStatus.
	ClaimTaskRun(ctx context.Context, in ClaimTaskRunInput) (bool, error)
	// UpdateRun updates a run's status and optional fields.
	UpdateRun(ctx context.Context, in UpdateTaskRunInput) error
	UpdateTaskRunWorkerInfo(ctx context.Context, chatRunID, workerType string, k8sJobName *string, k8sJobCreatedAt *int64) error
	// OnRunComplete creates task_run_artifact rows (one per relativePath) and updates task denormalized fields. Use for SUCCEEDED runs.
	OnRunComplete(ctx context.Context, chatRunID string, relativePaths []string) error
	// SyncTaskFromRun updates task denormalized fields and last_run_id from the run (no output). Use for FAILED runs.
	SyncTaskFromRun(ctx context.Context, chatRunID string) error
}

// ConversationStore provides Tier 1 conversation persistence. Conversations are workspace-scoped.
type ConversationStore interface {
	CreateConversation(ctx context.Context, workspaceID, channel, createdBy string) (*Conversation, error)
	GetConversation(ctx context.Context, conversationID string) (*Conversation, error)
	ListConversationsByWorkspace(ctx context.Context, workspaceID string, limit, offset int) ([]Conversation, int, error)
	UpdateConversationTitle(ctx context.Context, conversationID, title string) error
}

// ConversationMessageStore provides Tier 1 conversation message persistence.
// For role=assistant with tool calls, toolCallsJSON should be the JSON-encoded array of tool calls (id, name, arguments).
type ConversationMessageStore interface {
	AppendMessage(ctx context.Context, conversationID, role, content string, channel *string, toolCallID *string, toolCallsJSON *string) (*ConversationMessage, error)
	ListMessages(ctx context.Context, conversationID string) ([]ConversationMessage, error)
}

// WorkspaceWebhookKeyStore provides per-workspace webhook API key persistence.
// Keys are stored by hash; plaintext is returned only from CreateKey.
type WorkspaceWebhookKeyStore interface {
	// CreateKey creates a new webhook key for the workspace. Returns plaintext key (e.g. whsec_...) and key_id. Caller must store plaintext securely; it is not persisted.
	CreateKey(ctx context.Context, workspaceID, name string) (plaintextKey, keyID string, err error)
	// GetWorkspaceIDByKey looks up the workspace_id for the given plaintext key. Returns empty string if not found.
	GetWorkspaceIDByKey(ctx context.Context, plaintextKey string) (workspaceID string, err error)
	// ListKeys returns key metadata for the workspace (no plaintext).
	ListKeys(ctx context.Context, workspaceID string) ([]WebhookKeyMeta, error)
	// RevokeKey deletes the key by keyID if it belongs to the workspace.
	RevokeKey(ctx context.Context, workspaceID, keyID string) error
}
