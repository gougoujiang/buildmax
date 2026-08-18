package model

import "context"

// RunStatus is the canonical lifecycle status for task runs.
type RunStatus string

const (
	RunStatusPending   RunStatus = "PENDING"
	RunStatusScheduled RunStatus = "SCHEDULED"
	RunStatusRunning   RunStatus = "RUNNING"
	RunStatusSucceeded RunStatus = "SUCCEEDED"
	RunStatusFailed    RunStatus = "FAILED"
	// RunStatusCanceled is terminal and distinct from FAILED: nothing went
	// wrong, someone stopped the run. A canceled run keeps whatever output and
	// artifacts it had produced by then.
	RunStatusCanceled RunStatus = "CANCELED"
)

// RunStatusTerminal reports whether a run in this status has finished. A run
// leaves a non-terminal status only through its worker, the scheduler, or a
// cancel; a terminal one never changes again.
func RunStatusTerminal(status string) bool {
	switch RunStatus(status) {
	case RunStatusSucceeded, RunStatusFailed, RunStatusCanceled:
		return true
	default:
		return false
	}
}

// ActiveRunStatuses are the statuses a run passes through before it finishes.
// One task may hold at most one run in these statuses at a time.
var ActiveRunStatuses = []string{
	string(RunStatusPending),
	string(RunStatusScheduled),
	string(RunStatusRunning),
}

const (
	RunCreatedByTypeUser    = "user"
	RunCreatedByTypeWebhook = "webhook"
	RunCreatedByTypeSystem  = "system"
)

const (
	RunTriggerSourceTaskCreate         = "task_create"
	RunTriggerSourceTaskRerun          = "task_rerun"
	RunTriggerSourcePortalConversation = "portal_conversation"
	RunTriggerSourcePortalTaskCreate   = "portal_task_create"
	RunTriggerSourcePortalTaskRerun    = "portal_task_rerun"
	RunTriggerSourceIssueAgentRun      = "issue_agent_run"
	RunTriggerSourceWorkflowStep       = "workflow_step"
	RunTriggerSourceWebhook            = "webhook"
)

// Task holds the user-visible state for a background task.
type Task struct {
	ID                    uint    `json:"-"`
	TaskID                string  `json:"task_id"`
	ConversationID        string  `json:"conversation_id"`
	TeamID                string  `json:"team_id,omitempty"`
	IssueID               *string `json:"issue_id,omitempty"`
	Status                string  `json:"status"`
	Input                 string  `json:"input"`
	Title                 string  `json:"title,omitempty"`
	TitlePromptTokens     int     `json:"title_prompt_tokens,omitempty"`
	TitleCompletionTokens int     `json:"title_completion_tokens,omitempty"`
	Output                *string `json:"output,omitempty"`
	CreatedBy             string  `json:"created_by"`
	CreatedAt             int64   `json:"created_at"`
	StartedAt             *int64  `json:"started_at,omitempty"`
	EndedAt               *int64  `json:"ended_at,omitempty"`
	ErrorMessage          *string `json:"error_message,omitempty"`
	SessionID             *string `json:"session_id,omitempty"`
	LastRunID             *string `json:"last_run_id,omitempty"`
	AgentID               *string `json:"agent_id,omitempty"`
}

// TaskRun is one execution (initial or follow-up) of a task.
type TaskRun struct {
	ID               uint    `json:"-"`
	TaskRunID        string  `json:"task_run_id"`
	TaskID           string  `json:"task_id"`
	Input            string  `json:"input"`
	CreatedBy        string  `json:"created_by,omitempty"`
	CreatedByType    string  `json:"created_by_type,omitempty"`
	TriggerSource    string  `json:"trigger_source,omitempty"`
	Status           string  `json:"status"`
	Output           *string `json:"output,omitempty"`
	ErrorMessage     *string `json:"error_message,omitempty"`
	StartedAt        *int64  `json:"started_at,omitempty"`
	EndedAt          *int64  `json:"ended_at,omitempty"`
	SessionID        *string `json:"session_id,omitempty"`
	WorkerType       string  `json:"worker_type,omitempty"`
	K8sJobName       *string `json:"k8s_job_name,omitempty"`
	K8sJobCreatedAt  *int64  `json:"k8s_job_created_at,omitempty"`
	PromptTokens     *int    `json:"prompt_tokens,omitempty"`
	CompletionTokens *int    `json:"completion_tokens,omitempty"`
	// TracePath locates this run's durable trace inside run-global storage,
	// e.g. "traces/<session>/rt_….jsonl". Nil when no trace was written — the
	// run failed before an agent started, or tracing was disabled.
	TracePath *string `json:"trace_path,omitempty"`
	// CancelRequestedAt is when someone asked this run to stop. A cancel is
	// recorded rather than applied because the only thing that can stop a
	// started run is its own worker: the server states the intent, the worker
	// honors it and reports CANCELED. Nil means nobody has asked.
	CancelRequestedAt *int64 `json:"cancel_requested_at,omitempty"`
	// CancelRequestedBy is the user who asked. A team's runs can be stopped by
	// anyone on the team, so "why did this stop" needs a name to answer.
	CancelRequestedBy *string `json:"cancel_requested_by,omitempty"`
	CreatedAt         int64   `json:"created_at"`
}

// TaskRunTerminalInfo describes a task run that reached a terminal state.
// Used by the workflow service to advance or finalize workflow step runs.
type TaskRunTerminalInfo struct {
	TaskRunID      string
	TaskID         string
	ConversationID string
	UserID         string
	Status         string
	Output         *string
	ErrorMessage   *string
}

// CreateTaskInput is the input for CreateTask.
type CreateTaskInput struct {
	ConversationID          string
	TeamID                  string
	Input                   string
	Title                   string
	CreatedBy               string
	InitialRunCreatedBy     string
	InitialRunCreatedByType string
	InitialRunTriggerSource string
	TitlePromptTokens       int
	TitleCompletionTokens   int
	AgentID                 *string
	IssueID                 *string
}

// ClaimTaskRunInput atomically transitions a run from ExpectedStatus to NewStatus.
type ClaimTaskRunInput struct {
	TaskRunID      string
	ExpectedStatus RunStatus
	NewStatus      RunStatus
	StartedAt      *int64
	EndedAt        *int64
	Output         *string
	ErrorMessage   *string
	SessionID      *string
}

// UpdateTaskInput updates a task to the given status with optional fields.
type UpdateTaskInput struct {
	TaskID       string
	Status       string
	StartedAt    *int64
	EndedAt      *int64
	Output       *string
	ErrorMessage *string
	SessionID    *string
}

// ClaimTaskInput atomically transitions a task from ExpectedStatus to NewStatus.
type ClaimTaskInput struct {
	TaskID         string
	ExpectedStatus string
	NewStatus      string
	StartedAt      *int64
	EndedAt        *int64
	Output         *string
	ErrorMessage   *string
	SessionID      *string
}

// UpdateTaskRunInput updates a run to the given status with optional fields.
type UpdateTaskRunInput struct {
	TaskRunID        string
	Status           RunStatus
	StartedAt        *int64
	EndedAt          *int64
	Output           *string
	ErrorMessage     *string
	SessionID        *string
	PromptTokens     *int
	CompletionTokens *int
	TracePath        *string
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

// TaskRunStore provides task run persistence.
type TaskRunStore interface {
	// CreateTaskRun creates a new run (PENDING). Returns ErrRunInProgress if the task has any run in PENDING/SCHEDULED/RUNNING.
	CreateTaskRun(ctx context.Context, taskID, input, createdBy, createdByType, triggerSource string) (*TaskRun, error)
	// CountTaskRunsByStatus returns how many runs are in each status. It is
	// the one number that answers "is work flowing through this deployment",
	// and it carries no team, input, or output — only counts.
	CountTaskRunsByStatus(ctx context.Context) (map[string]int, error)
	// GetNextPendingTaskRun returns the oldest run with status PENDING (by created_at), or (nil, nil) if none.
	GetNextPendingTaskRun(ctx context.Context) (*TaskRun, error)
	GetTaskRun(ctx context.Context, taskRunID string) (*TaskRun, error)
	// GetTaskRunWithTask returns the run and its task, or (nil, nil, nil) if run not found.
	GetTaskRunWithTask(ctx context.Context, taskRunID string) (*TaskRun, *Task, error)
	// GetActiveTaskRunByTask returns the task's run in PENDING, SCHEDULED, or
	// RUNNING, or (nil, nil) when the task has none. A task holds at most one.
	GetActiveTaskRunByTask(ctx context.Context, taskID string) (*TaskRun, error)
	// RequestTaskRunCancel records who asked a run to stop, and when, on a run
	// that has not reached a terminal status. Returns false when the run is
	// already terminal or already carries a request, so a second cancel
	// neither resets the clock the backstop measures nor overwrites the name
	// of whoever asked first.
	RequestTaskRunCancel(ctx context.Context, taskRunID, requestedBy string, requestedAt int64) (bool, error)
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
