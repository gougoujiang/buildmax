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
	CreateChat(ctx context.Context, in *CreateChatInput) (*Chat, error)
	UpdateChat(ctx context.Context, in UpdateChatInput) error
	ClaimChat(ctx context.Context, in ClaimChatInput) (updated bool, err error)
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
	// ClaimChatRun atomically updates a run when current status matches ExpectedStatus.
	ClaimChatRun(ctx context.Context, in ClaimChatRunInput) (bool, error)
	// UpdateRun updates a run's status and optional fields.
	UpdateRun(ctx context.Context, in UpdateChatRunInput) error
	UpdateChatRunWorkerInfo(ctx context.Context, chatRunID, workerType string, k8sJobName *string, k8sJobCreatedAt *int64) error
	// OnRunComplete creates chat_run_artifact rows (one per relativePath) and updates chat denormalized fields. Use for SUCCEEDED runs.
	OnRunComplete(ctx context.Context, chatRunID string, relativePaths []string) error
	// SyncChatFromRun updates chat denormalized fields and last_run_id from the run (no output). Use for FAILED runs.
	SyncChatFromRun(ctx context.Context, chatRunID string) error
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
