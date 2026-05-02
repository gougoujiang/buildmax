package model

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
	// UpdateLoginMeta records the last login timestamp and platform for the user.
	UpdateLoginMeta(ctx context.Context, userID string, loginAt int64, platform string) error
}

// TeamStore provides team persistence and membership lookup.
type TeamStore interface {
	// GetTeam returns the team by team_id, or (nil, nil) when not found.
	GetTeam(ctx context.Context, teamID string) (*Team, error)
	// GetPersonalTeamByUser returns the default personal team for the user, or (nil, nil) when not found.
	GetPersonalTeamByUser(ctx context.Context, userID string) (*Team, error)
	// ListTeamsByUser returns all teams the user belongs to, ordered by created_at ASC.
	ListTeamsByUser(ctx context.Context, userID string) ([]Team, error)
	// CreateTeam creates a new team and owner membership.
	CreateTeam(ctx context.Context, name, createdBy, quotaTier string) (*Team, error)
	// AddTeamMember adds or updates a team membership.
	AddTeamMember(ctx context.Context, teamID, userID, role string) (*TeamMember, error)
	// RemoveTeamMember removes one membership from a team.
	RemoveTeamMember(ctx context.Context, teamID, userID string) error
	// ListTeamMembers returns members of the team ordered by created_at ASC.
	ListTeamMembers(ctx context.Context, teamID string) ([]TeamMember, error)
}

// WorkflowStore provides workflow and workflow execution persistence.
type WorkflowStore interface {
	ListWorkflowsByTeam(ctx context.Context, teamID string) ([]Workflow, error)
	CreateWorkflow(ctx context.Context, teamID, createdBy, name, description, definition string) (*Workflow, error)
	GetWorkflow(ctx context.Context, workflowID string) (*Workflow, error)
	UpdateWorkflow(ctx context.Context, workflowID, teamID string, in UpdateWorkflowInput) (*Workflow, error)
	CreateWorkflowRun(ctx context.Context, in CreateWorkflowRunInput) (*WorkflowRun, error)
	ListWorkflowRunsByWorkflow(ctx context.Context, workflowID string, limit, offset int) ([]WorkflowRun, int, error)
	ListWorkflowRunsByIssue(ctx context.Context, issueID string, limit, offset int) ([]WorkflowRun, int, error)
	GetWorkflowRun(ctx context.Context, workflowRunID string) (*WorkflowRun, error)
	ListWorkflowStepRuns(ctx context.Context, workflowRunID string) ([]WorkflowStepRun, error)
	CreateWorkflowStepRuns(ctx context.Context, workflowRunID string, steps []CreateWorkflowStepRunInput) ([]WorkflowStepRun, error)
	UpdateWorkflowRun(ctx context.Context, workflowRunID string, in UpdateWorkflowRunInput) (*WorkflowRun, error)
	UpdateWorkflowStepRun(ctx context.Context, stepRunID string, in UpdateWorkflowStepRunInput) (*WorkflowStepRun, error)
	GetWorkflowStepRunByTaskID(ctx context.Context, taskID string) (*WorkflowStepRun, error)
	GetWorkflowStepRunByTaskRunID(ctx context.Context, taskRunID string) (*WorkflowStepRun, error)
}

// QuotaTierStore provides quota tier limits by tier name.
type QuotaTierStore interface {
	// GetQuotaTier returns the tier limits by tier name, or (nil, nil) when not found.
	GetQuotaTier(ctx context.Context, tierName string) (*QuotaTier, error)
}

// UsageInWindowReader provides usage aggregation for a team in a time window.
type UsageInWindowReader interface {
	// TeamUsageInWindow returns run count and total tokens for the team in [sinceUnix, untilUnix].
	TeamUsageInWindow(ctx context.Context, teamID string, sinceUnix, untilUnix int64) (runCount, totalTokens int, err error)
}

// AgentStore provides agent persistence. Agents are user-scoped.
type AgentStore interface {
	ListAgentsByUser(ctx context.Context, userID string) ([]Agent, error)
	ListAgentsByTeam(ctx context.Context, teamID string) ([]Agent, error)
	GetAgent(ctx context.Context, agentID string) (*Agent, error)
	CreateAgent(ctx context.Context, userID, name, description, instructions string) (*Agent, error)
	CreateAgentInTeam(ctx context.Context, teamID, userID, name, description, instructions string) (*Agent, error)
	UpdateAgent(ctx context.Context, agentID, userID, name, description, instructions string) (*Agent, error)
	UpdateAgentInTeam(ctx context.Context, agentID, teamID, name, description, instructions string) (*Agent, error)
	DeleteAgent(ctx context.Context, agentID, userID string) error
	DeleteAgentInTeam(ctx context.Context, agentID, teamID string) error
}

// IssueStore provides issue persistence. Issues are user-scoped.
type IssueStore interface {
	CreateIssue(ctx context.Context, userID string, in CreateIssueInput) (*Issue, error)
	CreateIssueInTeam(ctx context.Context, teamID, createdBy string, in CreateIssueInput) (*Issue, error)
	ListIssuesByUser(ctx context.Context, userID string, limit, offset int) ([]Issue, int, error)
	ListIssuesByTeam(ctx context.Context, teamID string, limit, offset int) ([]Issue, int, error)
	GetIssue(ctx context.Context, issueID string) (*Issue, error)
	UpdateIssue(ctx context.Context, issueID, userID string, in UpdateIssueInput) (*Issue, error)
	UpdateIssueInTeam(ctx context.Context, issueID, teamID string, in UpdateIssueInput) (*Issue, error)
}

type CreateWorkflowRunInput struct {
	WorkflowID     string
	IssueID        *string
	ConversationID string
	Status         string
	CreatedBy      string
	StartedAt      *int64
}

type UpdateWorkflowInput struct {
	Name        *string
	Description *string
	Definition  *string
	Status      *string
}

type CreateWorkflowStepRunInput struct {
	StepID        string
	StepIndex     int
	StepType      string
	TargetAgentID *string
	Prompt        string
	Status        string
}

type UpdateWorkflowRunInput struct {
	Status       string
	StartedAt    *int64
	EndedAt      *int64
	ErrorMessage *string
}

type UpdateWorkflowStepRunInput struct {
	Status        *string
	TaskID        *string
	TaskRunID     *string
	OutputSummary *string
	ErrorMessage  *string
	StartedAt     *int64
	EndedAt       *int64
}

// TaskStore provides task persistence. Tasks belong to a conversation.
// CreateTask creates a task plus its first TaskRun (both in one transaction).
type TaskStore interface {
	// ListTasksByConversation returns tasks in the conversation. order is "asc" (oldest first) or "desc" (latest first); default "desc".
	ListTasksByConversation(ctx context.Context, conversationID string, order string) ([]Task, error)
	// ListTasksByConversationPaginated returns tasks with optional executed_only filter, ordered by created_at DESC. total is total matching count.
	ListTasksByConversationPaginated(ctx context.Context, conversationID string, executedOnly bool, limit, offset int) ([]Task, int, error)
	ListTasksByIssue(ctx context.Context, issueID string, limit, offset int) ([]Task, int, error)
	GetTask(ctx context.Context, taskID string) (*Task, error)
	GetTaskBySessionID(ctx context.Context, sessionID string) (*Task, error)
	// CreateTask creates a new task and its first TaskRun (input, title, PENDING). Returns the task with last_run_id set.
	CreateTask(ctx context.Context, in *CreateTaskInput) (*Task, error)
	UpdateTask(ctx context.Context, in UpdateTaskInput) error
	ClaimTask(ctx context.Context, in ClaimTaskInput) (updated bool, err error)
}

// ErrRunInProgress is returned by CreateTaskRun when the task already has a run in PENDING, SCHEDULED, or RUNNING.
var ErrRunInProgress = errors.New("task has a run already in progress")

// TaskRunStore provides task run persistence.
type TaskRunStore interface {
	// CreateTaskRun creates a new run (PENDING). Returns ErrRunInProgress if the task has any run in PENDING/SCHEDULED/RUNNING.
	CreateTaskRun(ctx context.Context, taskID, input, createdBy, createdByType, triggerSource string) (*TaskRun, error)
	// GetNextPendingTaskRun returns the oldest run with status PENDING (by created_at), or (nil, nil) if none.
	GetNextPendingTaskRun(ctx context.Context) (*TaskRun, error)
	GetTaskRun(ctx context.Context, taskRunID string) (*TaskRun, error)
	// GetTaskRunWithTask returns the run and its task, or (nil, nil, nil) if run not found.
	GetTaskRunWithTask(ctx context.Context, taskRunID string) (*TaskRun, *Task, error)
	// ClaimTaskRun atomically updates a run when current status matches ExpectedStatus.
	ClaimTaskRun(ctx context.Context, in ClaimTaskRunInput) (bool, error)
	// UpdateRun updates a run's status and optional fields.
	UpdateRun(ctx context.Context, in UpdateTaskRunInput) error
	UpdateTaskRunWorkerInfo(ctx context.Context, taskRunID, workerType string, k8sJobName *string, k8sJobCreatedAt *int64) error
	// OnRunComplete creates task_run_artifact rows (one per relativePath) and updates task denormalized fields. Use for SUCCEEDED runs.
	OnRunComplete(ctx context.Context, taskRunID string, relativePaths []string) error
	// SyncTaskFromRun updates task denormalized fields and last_run_id from the run (no output). Use for FAILED runs.
	SyncTaskFromRun(ctx context.Context, taskRunID string) error
}

// ConversationStore provides Tier 1 conversation persistence. Conversations are user-scoped.
type ConversationStore interface {
	CreateConversation(ctx context.Context, userID, channel, createdBy string) (*Conversation, error)
	CreateConversationInTeam(ctx context.Context, teamID, userID, channel, createdBy string) (*Conversation, error)
	GetConversation(ctx context.Context, conversationID string) (*Conversation, error)
	ListConversationsByUser(ctx context.Context, userID string, limit, offset int) ([]Conversation, int, error)
	ListConversationsByTeam(ctx context.Context, teamID string, limit, offset int) ([]Conversation, int, error)
	UpdateConversationTitle(ctx context.Context, conversationID, title string) error
}

// ConversationMessageStore provides Tier 1 conversation message persistence.
// For role=assistant with tool calls, toolCallsJSON should be the JSON-encoded array of tool calls (id, name, arguments).
type ConversationMessageStore interface {
	AppendMessage(ctx context.Context, conversationID, role, content string, channel *string, toolCallID *string, toolCallsJSON *string) (*ConversationMessage, error)
	ListMessages(ctx context.Context, conversationID string) ([]ConversationMessage, error)
}

// UserWebhookKeyStore provides per-user webhook API key persistence.
// Keys are stored by hash; plaintext is returned only from CreateKey.
type UserWebhookKeyStore interface {
	// CreateKey creates a new webhook key for the user. Returns plaintext key (e.g. whsec_...) and key_id. Caller must store plaintext securely; it is not persisted.
	CreateKey(ctx context.Context, userID, name string) (plaintextKey, keyID string, err error)
	// GetUserIDByKey looks up the user_id for the given plaintext key. Returns empty string if not found.
	GetUserIDByKey(ctx context.Context, plaintextKey string) (userID string, err error)
	// ListKeys returns key metadata for the user (no plaintext).
	ListKeys(ctx context.Context, userID string) ([]WebhookKeyMeta, error)
	// RevokeKey deletes the key by keyID if it belongs to the user.
	RevokeKey(ctx context.Context, userID, keyID string) error
}
