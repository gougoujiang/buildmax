package model

// User is the user model. JSON uses snake_case per project convention.
// Internal numeric ID is retained for compatibility but is not part of the public API.
type User struct {
	ID                uint    `json:"-"`
	UserID            string  `json:"user_id"`
	Email             string  `json:"email"`
	Name              string  `json:"name"`
	QuotaTier         string  `json:"quota_tier,omitempty"`
	LastLoginAt       *int64  `json:"last_login_at,omitempty"`
	LastLoginPlatform *string `json:"last_login_platform,omitempty"`
	CreatedAt         int64   `json:"created_at"`
}

// Team is the ownership and collaboration boundary for working resources.
// A user's default personal team is represented by personal_for_user_id.
type Team struct {
	ID                uint    `json:"-"`
	TeamID            string  `json:"team_id"`
	Name              string  `json:"name"`
	PersonalForUserID *string `json:"personal_for_user_id,omitempty"`
	QuotaTier         string  `json:"quota_tier,omitempty"`
	CreatedBy         string  `json:"created_by"`
	CreatedAt         int64   `json:"created_at"`
	UpdatedAt         int64   `json:"updated_at"`
}

// TeamMember is one user's membership in a team.
type TeamMember struct {
	ID        uint   `json:"-"`
	TeamID    string `json:"team_id"`
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
	CreatedAt int64  `json:"created_at"`
}

// Workflow is a reusable team-scoped execution plan.
type Workflow struct {
	ID          uint   `json:"-"`
	WorkflowID  string `json:"workflow_id"`
	TeamID      string `json:"team_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Definition  string `json:"definition"`
	Status      string `json:"status"`
	CreatedBy   string `json:"created_by"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// WorkflowRun is one execution attempt of a workflow.
type WorkflowRun struct {
	ID             uint    `json:"-"`
	WorkflowRunID  string  `json:"workflow_run_id"`
	WorkflowID     string  `json:"workflow_id"`
	IssueID        *string `json:"issue_id,omitempty"`
	ConversationID string  `json:"conversation_id"`
	Status         string  `json:"status"`
	CreatedBy      string  `json:"created_by"`
	CreatedAt      int64   `json:"created_at"`
	StartedAt      *int64  `json:"started_at,omitempty"`
	EndedAt        *int64  `json:"ended_at,omitempty"`
	ErrorMessage   *string `json:"error_message,omitempty"`
}

// WorkflowStepRun is one durable step execution record under a workflow run.
type WorkflowStepRun struct {
	ID            uint    `json:"-"`
	StepRunID     string  `json:"workflow_step_run_id"`
	WorkflowRunID string  `json:"workflow_run_id"`
	StepID        string  `json:"step_id"`
	StepIndex     int     `json:"step_index"`
	StepType      string  `json:"step_type"`
	TargetAgentID *string `json:"target_agent_id,omitempty"`
	Prompt        string  `json:"prompt"`
	Status        string  `json:"status"`
	TaskID        *string `json:"task_id,omitempty"`
	TaskRunID     *string `json:"task_run_id,omitempty"`
	OutputSummary *string `json:"output_summary,omitempty"`
	ErrorMessage  *string `json:"error_message,omitempty"`
	CreatedAt     int64   `json:"created_at"`
	StartedAt     *int64  `json:"started_at,omitempty"`
	EndedAt       *int64  `json:"ended_at,omitempty"`
}

// Issue is the user-facing work-management object. It is intentionally separate
// from low-level task/task_run execution records.
type Issue struct {
	ID           uint    `json:"-"`
	IssueID      string  `json:"issue_id"`
	UserID       string  `json:"user_id"`
	TeamID       string  `json:"team_id,omitempty"`
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	Status       string  `json:"status"`
	AssigneeKind *string `json:"assignee_kind,omitempty"`
	AssigneeID   *string `json:"assignee_id,omitempty"`
	CreatedBy    string  `json:"created_by"`
	CreatedAt    int64   `json:"created_at"`
	UpdatedAt    int64   `json:"updated_at"`
}

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
	CreatedAt        int64   `json:"created_at"`
}

// TaskRunArtifact is one output file (artifact) for a task run.
type TaskRunArtifact struct {
	ID           uint   `json:"-"`
	TaskRunID    string `json:"task_run_id"`
	RelativePath string `json:"relative_path"`
}

// Agent is the agent model (user-scoped persona).
type Agent struct {
	ID           uint   `json:"-"`
	AgentID      string `json:"agent_id"`
	UserID       string `json:"user_id"`
	TeamID       string `json:"team_id,omitempty"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	CreatedAt    int64  `json:"created_at"`
}

// QuotaTier defines limits for a tier (e.g. free_trial, pro).
type QuotaTier struct {
	TierName           string `json:"tier_name"`
	MaxRunsPerPeriod   int    `json:"max_runs_per_period"`
	MaxTokensPerPeriod int    `json:"max_tokens_per_period"`
	PeriodDays         int    `json:"period_days"`
}

// ArtifactWithTask is a DTO for listing run outputs (artifacts) with task/run context.
// ArtifactID holds task_run_id for API compatibility.
type ArtifactWithTask struct {
	ArtifactID       string `json:"artifact_id"`
	TaskID           string `json:"task_id"`
	TaskRunID        string `json:"task_run_id"`
	ConversationID   string `json:"conversation_id"`
	UserID           string `json:"user_id"`
	CreatedAt        int64  `json:"created_at"`
	TaskInputSnippet string `json:"task_input_snippet"`
}

// Conversation is the Tier 1 conversation container.
type Conversation struct {
	ID             uint   `json:"-"`
	ConversationID string `json:"conversation_id"`
	UserID         string `json:"user_id"`
	TeamID         string `json:"team_id,omitempty"`
	Channel        string `json:"channel"`
	Title          string `json:"title,omitempty"`
	CreatedBy      string `json:"created_by"`
	CreatedAt      int64  `json:"created_at"`
}

// ConversationMessage is one message in a Tier 1 conversation.
type ConversationMessage struct {
	ID                    uint    `json:"-"`
	ConversationMessageID string  `json:"conversation_message_id"`
	ConversationID        string  `json:"conversation_id"`
	Role                  string  `json:"role"`
	Content               string  `json:"content"`
	Channel               *string `json:"channel,omitempty"`
	ToolCallID            *string `json:"tool_call_id,omitempty"`
	ToolCallsJSON         *string `json:"tool_calls,omitempty"`
	CreatedAt             int64   `json:"created_at"`
}
