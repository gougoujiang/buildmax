package task

import (
	"context"
	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"
	"time"
)

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

// ValidRunStatusTransition reports whether a run may move directly from one
// status to another. Terminal statuses are immutable.
func ValidRunStatusTransition(from, to RunStatus) bool {
	switch from {
	case RunStatusPending:
		return to == RunStatusScheduled || to == RunStatusCanceled
	case RunStatusScheduled:
		return to == RunStatusRunning || to == RunStatusFailed || to == RunStatusCanceled
	case RunStatusRunning:
		return to == RunStatusSucceeded || to == RunStatusFailed || to == RunStatusCanceled
	default:
		return false
	}
}

// ActiveRunStatuses returns the statuses a run passes through before it
// finishes. It returns a fresh slice so callers cannot mutate the lifecycle
// definition for the rest of the process.
func ActiveRunStatuses() []string {
	return []string{
		string(RunStatusPending),
		string(RunStatusScheduled),
		string(RunStatusRunning),
	}
}

const (
	RunCreatedByTypeUser    = "user"
	RunCreatedByTypeWebhook = "webhook"
	RunCreatedByTypeSystem  = "system"
)

const (
	RunTriggerSourceTaskCreate = "task_create"
	RunTriggerSourceTaskRerun  = "task_rerun"
	// RunTriggerSourceTaskRetry marks a run that repeats an earlier one's
	// input rather than carrying new instructions. It is distinct from a rerun
	// because "this was run again unchanged" and "someone asked for something
	// else" are different answers to why a run exists.
	RunTriggerSourceTaskRetry          = "task_retry"
	RunTriggerSourcePortalConversation = "portal_conversation"
	RunTriggerSourcePortalTaskCreate   = "portal_task_create"
	RunTriggerSourcePortalTaskRerun    = "portal_task_rerun"
	RunTriggerSourceIssueAgentRun      = "issue_agent_run"
	RunTriggerSourceWorkflowStep       = "workflow_step"
	RunTriggerSourceWebhook            = "webhook"
)

// Task holds the user-visible state for a background task.
type Task struct {
	ID                    string     `json:"id"`
	ConversationID        string     `json:"conversation_id"`
	TeamID                string     `json:"team_id,omitempty"`
	IssueID               *string    `json:"issue_id,omitempty"`
	Status                string     `json:"status"`
	Input                 string     `json:"input"`
	Title                 string     `json:"title,omitempty"`
	TitlePromptTokens     int        `json:"title_prompt_tokens,omitempty"`
	TitleCompletionTokens int        `json:"title_completion_tokens,omitempty"`
	Output                *string    `json:"output,omitempty"`
	CreatedBy             string     `json:"created_by"`
	CreatedAt             time.Time  `json:"created_at"`
	StartedAt             *time.Time `json:"started_at,omitempty"`
	EndedAt               *time.Time `json:"ended_at,omitempty"`
	ErrorMessage          *string    `json:"error_message,omitempty"`
	SessionID             *string    `json:"session_id,omitempty"`
	LastRunID             *string    `json:"last_run_id,omitempty"`
	AgentID               *string    `json:"agent_id,omitempty"`
}

// Run is one execution (initial or follow-up) of a task.
type Run struct {
	ID               string     `json:"id"`
	TaskID           string     `json:"task_id"`
	Input            string     `json:"input"`
	CreatedBy        string     `json:"created_by,omitempty"`
	CreatedByType    string     `json:"created_by_type,omitempty"`
	TriggerSource    string     `json:"trigger_source,omitempty"`
	Status           string     `json:"status"`
	Output           *string    `json:"output,omitempty"`
	ErrorMessage     *string    `json:"error_message,omitempty"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	EndedAt          *time.Time `json:"ended_at,omitempty"`
	SessionID        *string    `json:"session_id,omitempty"`
	WorkerType       string     `json:"worker_type,omitempty"`
	K8sJobName       *string    `json:"k8s_job_name,omitempty"`
	K8sJobCreatedAt  *time.Time `json:"k8s_job_created_at,omitempty"`
	PromptTokens     *int       `json:"prompt_tokens,omitempty"`
	CompletionTokens *int       `json:"completion_tokens,omitempty"`
	// TracePath locates this run's durable trace inside run-global storage,
	// e.g. "traces/<session>/rt_….jsonl". Nil when no trace was written — the
	// run failed before an agent started, or tracing was disabled.
	TracePath *string `json:"trace_path,omitempty"`
	// CancelRequestedAt is when someone asked this run to stop. A cancel is
	// recorded rather than applied because the only thing that can stop a
	// started run is its own worker: the server states the intent, the worker
	// honors it and reports CANCELED. Nil means nobody has asked.
	CancelRequestedAt *time.Time `json:"cancel_requested_at,omitempty"`
	// CancelRequestedBy is the user who asked. A team's runs can be stopped by
	// anyone on the team, so "why did this stop" needs a name to answer.
	CancelRequestedBy *string `json:"cancel_requested_by,omitempty"`
	// RetryOfTaskRunID names the run this one repeats. Nil for every run that
	// carries its own instructions. The lineage is one level deep by record but
	// unbounded by use: retrying a retry points at the run it repeated, not at
	// the first of the chain.
	RetryOfTaskRunID *string `json:"retry_of_task_run_id,omitempty"`
	// AgentRevision numbers the agent definition this run was actually given.
	//
	// The definition is resolved when a worker asks for its run, not when the
	// task was created, so an edit takes effect on the next run. That is what
	// someone editing the field expects and it is also why this is recorded: a
	// run's instructions are otherwise whatever the agent says today, and no
	// record says which text produced this outcome. Nil for a run with no agent
	// and for runs that predate the column.
	AgentRevision *int `json:"agent_revision,omitempty"`
	// PluginPins are the releases this run was given, resolved when its worker
	// claimed it and fixed from that moment.
	//
	// Recorded for the reason AgentRevision is: afterwards nothing else can say
	// which versions this run actually had. The trace says so too, but a trace
	// is fail-open and lives in run-global storage, while this is the queryable
	// fact and what a retry reads. Nil for a run that resolved no plugins.
	PluginPins []coreplugin.Pin `json:"plugin_pins,omitempty"`
	// SourceMessageID names the conversation message this run was asked for in.
	//
	// Input is what Tier 1 decided to send a worker; this is what the person
	// actually said. They are not the same text and the difference is the point:
	// without it, nobody can tell a constraint the model dropped from one the
	// user never gave. Nil for a run with no message behind it — a workflow
	// step, an issue agent run, a retry, or a task created straight from the API.
	SourceMessageID *string   `json:"source_message_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// RunTerminalInfo describes a task run that reached a terminal state.
// Used by the workflow service to advance or finalize workflow step runs.
type RunTerminalInfo struct {
	TaskRunID      string
	TaskID         string
	ConversationID string
	// TeamID is the team that owns the task. Empty on a task created before
	// tasks carried one, which is why UserID is still here to fall back to.
	TeamID       string
	UserID       string
	Status       string
	Output       *string
	ErrorMessage *string
}

// CreateInput is the input for CreateTask.
type CreateInput struct {
	ConversationID          string
	TeamID                  string
	Input                   string
	Title                   string
	CreatedBy               string
	InitialRunCreatedBy     string
	InitialRunCreatedByType string
	InitialRunTriggerSource string
	// InitialRunSourceMessageID names the message that asked for this task.
	InitialRunSourceMessageID *string
	TitlePromptTokens         int
	TitleCompletionTokens     int
	AgentID                   *string
	IssueID                   *string
}

// UpdateInput updates a task to the given status with optional fields.
type UpdateInput struct {
	TaskID       string
	Status       string
	StartedAt    *time.Time
	EndedAt      *time.Time
	Output       *string
	ErrorMessage *string
	SessionID    *string
}

// ClaimInput atomically transitions a task from ExpectedStatus to NewStatus.
type ClaimInput struct {
	TaskID         string
	ExpectedStatus string
	NewStatus      string
	StartedAt      *time.Time
	EndedAt        *time.Time
	Output         *string
	ErrorMessage   *string
	SessionID      *string
}

// TransitionRunInput atomically moves a run from ExpectedStatus to
// NewStatus and projects the accepted state onto its task. Artifact paths, when
// present, are registered in the same transaction as a terminal transition.
type TransitionRunInput struct {
	TaskRunID             string
	ExpectedStatus        RunStatus
	NewStatus             RunStatus
	StartedAt             *time.Time
	EndedAt               *time.Time
	Output                *string
	ErrorMessage          *string
	SessionID             *string
	PromptTokens          *int
	CompletionTokens      *int
	TracePath             *string
	ArtifactRelativePaths []string
}

// Store provides task persistence. Tasks belong to a conversation.
// CreateTask creates a task plus its first Run (both in one transaction).
type Store interface {
	// ListTasksByConversation returns tasks in the conversation. order is "asc" (oldest first) or "desc" (latest first); default "desc".
	ListTasksByConversation(ctx context.Context, conversationID string, order string) ([]Task, error)
	// ListTasksByConversationPaginated returns tasks with optional executed_only filter, ordered by created_at DESC. total is total matching count.
	ListTasksByConversationPaginated(ctx context.Context, conversationID string, executedOnly bool, limit, offset int) ([]Task, int, error)
	ListTasksByIssue(ctx context.Context, issueID string, limit, offset int) ([]Task, int, error)
	GetTask(ctx context.Context, taskID string) (*Task, error)
	GetTaskBySessionID(ctx context.Context, sessionID string) (*Task, error)
	// CreateTask creates a new task and its first Run (input, title, PENDING). Returns the task with last_run_id set.
	CreateTask(ctx context.Context, in *CreateInput) (*Task, error)
	UpdateTask(ctx context.Context, in UpdateInput) error
	ClaimTask(ctx context.Context, in ClaimInput) (updated bool, err error)
}

// CreateRunInput describes a new run on an existing task.
type CreateRunInput struct {
	TaskID        string
	Input         string
	CreatedBy     string
	CreatedByType string
	TriggerSource string
	// RetryOfTaskRunID names the run this one repeats, when it repeats one.
	RetryOfTaskRunID *string
	// SourceMessageID names the conversation message that asked for this run.
	SourceMessageID *string
}

// RunStore provides task run persistence.
type RunStore interface {
	// CreateTaskRun creates a new run (PENDING). Returns ErrRunInProgress if the task has any run in PENDING/SCHEDULED/RUNNING.
	CreateTaskRun(ctx context.Context, in CreateRunInput) (*Run, error)
	// CountTaskRunsByStatus returns how many runs are in each status. It is
	// the one number that answers "is work flowing through this deployment",
	// and it carries no team, input, or output — only counts.
	CountTaskRunsByStatus(ctx context.Context) (map[string]int, error)
	// GetNextPendingTaskRun returns the oldest run with status PENDING (by created_at), or (nil, nil) if none.
	GetNextPendingTaskRun(ctx context.Context) (*Run, error)
	GetTaskRun(ctx context.Context, taskRunID string) (*Run, error)
	// GetTaskRunWithTask returns the run and its task, or (nil, nil, nil) if run not found.
	GetTaskRunWithTask(ctx context.Context, taskRunID string) (*Run, *Task, error)
	// ListTaskRunIDsByTasks returns each task's run IDs, newest first, keyed by
	// task ID. Tasks with no runs are absent from the map.
	//
	// It exists because a task's last run is not its only run: a retried task
	// has earlier ones, and what those produced did not stop existing.
	ListTaskRunIDsByTasks(ctx context.Context, taskIDs []string) (map[string][]string, error)
	// GetActiveTaskRunByTask returns the task's run in PENDING, SCHEDULED, or
	// RUNNING, or (nil, nil) when the task has none. A task holds at most one.
	GetActiveTaskRunByTask(ctx context.Context, taskID string) (*Run, error)
	// RequestTaskRunCancel records who asked a run to stop, and when, on a run
	// that has not reached a terminal status. Returns false when the run is
	// already terminal or already carries a request, so a second cancel
	// neither resets the clock the backstop measures nor overwrites the name
	// of whoever asked first.
	RequestTaskRunCancel(ctx context.Context, taskRunID, requestedBy string, requestedAt time.Time) (bool, error)
	// TransitionTaskRun atomically updates a run only when its current status
	// matches ExpectedStatus, then updates the task projection in the same
	// transaction. A false result means another actor won the transition.
	TransitionTaskRun(ctx context.Context, in TransitionRunInput) (bool, error)
	UpdateTaskRunWorkerInfo(ctx context.Context, taskRunID, workerType string, k8sJobName *string, k8sJobCreatedAt *time.Time) error
	// RecordTaskRunAgentRevision stores which agent definition a run was given.
	// The first write wins: a run executes under the instructions it was handed
	// at dispatch, and a later edit does not retroactively change what ran.
	RecordTaskRunAgentRevision(ctx context.Context, taskRunID string, revision int) error
	// RecordTaskRunPluginPins stores the releases a run was given. Like the
	// agent revision, the first write wins: a worker polls its run, and a
	// team's activation edited mid-run must not rewrite what actually ran.
	RecordTaskRunPluginPins(ctx context.Context, taskRunID string, pins []coreplugin.Pin) error
}
