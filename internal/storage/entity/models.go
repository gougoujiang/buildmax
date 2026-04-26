package entity

// User is the user model. JSON uses snake_case per project convention.
// Internal DB numeric id is intentionally not exposed; API and JWT use user_id.
type User struct {
	ID                uint    `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID            string  `gorm:"type:varchar(64);uniqueIndex;not null" json:"user_id"`
	Email             string  `gorm:"type:varchar(255);uniqueIndex;not null" json:"email"`
	Name              string  `gorm:"type:varchar(255)" json:"name"`
	QuotaTier         string  `gorm:"type:varchar(64)" json:"quota_tier,omitempty"`
	LastLoginAt       *int64  `gorm:"" json:"last_login_at,omitempty"`
	LastLoginPlatform *string `gorm:"type:varchar(32)" json:"last_login_platform,omitempty"`
	CreatedAt         int64   `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM (singular per project convention).
func (User) TableName() string { return "user" }

// Team is the ownership and collaboration boundary for working resources.
// A user's default personal team is represented by personal_for_user_id.
type Team struct {
	ID                uint    `gorm:"primaryKey;autoIncrement" json:"-"`
	TeamID            string  `gorm:"column:team_id;type:varchar(64);uniqueIndex;not null" json:"team_id"`
	Name              string  `gorm:"type:varchar(255);not null" json:"name"`
	PersonalForUserID *string `gorm:"column:personal_for_user_id;type:varchar(64);uniqueIndex" json:"personal_for_user_id,omitempty"`
	CreatedBy         string  `gorm:"type:varchar(64);not null" json:"created_by"`
	CreatedAt         int64   `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt         int64   `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name for GORM (singular per project convention).
func (Team) TableName() string { return "team" }

// TeamMember is one user's membership in a team.
type TeamMember struct {
	ID        uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	TeamID    string `gorm:"column:team_id;type:varchar(64);not null;uniqueIndex:uq_team_member_team_user" json:"team_id"`
	UserID    string `gorm:"column:user_id;type:varchar(64);not null;uniqueIndex:uq_team_member_team_user" json:"user_id"`
	Role      string `gorm:"type:varchar(32);not null" json:"role"`
	CreatedAt int64  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM (singular per project convention).
func (TeamMember) TableName() string { return "team_member" }

// Workflow is a reusable team-scoped execution plan.
type Workflow struct {
	ID          uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	WorkflowID  string `gorm:"column:workflow_id;type:varchar(64);uniqueIndex;not null" json:"workflow_id"`
	TeamID      string `gorm:"column:team_id;type:varchar(64);not null;index" json:"team_id"`
	Name        string `gorm:"type:varchar(255);not null" json:"name"`
	Description string `gorm:"type:text;not null" json:"description"`
	Definition  string `gorm:"type:longtext;not null" json:"definition"`
	CreatedBy   string `gorm:"type:varchar(64);not null" json:"created_by"`
	CreatedAt   int64  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   int64  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name for GORM.
func (Workflow) TableName() string { return "workflow" }

// WorkflowRun is one execution attempt of a workflow.
type WorkflowRun struct {
	ID             uint    `gorm:"primaryKey;autoIncrement" json:"-"`
	WorkflowRunID  string  `gorm:"column:workflow_run_id;type:varchar(64);uniqueIndex;not null" json:"workflow_run_id"`
	WorkflowID     string  `gorm:"column:workflow_id;type:varchar(64);not null;index" json:"workflow_id"`
	IssueID        *string `gorm:"column:issue_id;type:varchar(64);index" json:"issue_id,omitempty"`
	ConversationID string  `gorm:"column:conversation_id;type:varchar(64);not null;index" json:"conversation_id"`
	Status         string  `gorm:"type:varchar(32);not null" json:"status"`
	CreatedBy      string  `gorm:"type:varchar(64);not null" json:"created_by"`
	CreatedAt      int64   `gorm:"autoCreateTime" json:"created_at"`
	StartedAt      *int64  `gorm:"" json:"started_at,omitempty"`
	EndedAt        *int64  `gorm:"" json:"ended_at,omitempty"`
	ErrorMessage   *string `gorm:"type:text" json:"error_message,omitempty"`
}

// TableName returns the table name for GORM.
func (WorkflowRun) TableName() string { return "workflow_run" }

// WorkflowStepRun is one durable step execution record under a workflow run.
type WorkflowStepRun struct {
	ID            uint    `gorm:"primaryKey;autoIncrement" json:"-"`
	StepRunID     string  `gorm:"column:workflow_step_run_id;type:varchar(64);uniqueIndex;not null" json:"workflow_step_run_id"`
	WorkflowRunID string  `gorm:"column:workflow_run_id;type:varchar(64);not null;index" json:"workflow_run_id"`
	StepID        string  `gorm:"column:step_id;type:varchar(128);not null" json:"step_id"`
	StepIndex     int     `gorm:"column:step_index;not null" json:"step_index"`
	StepType      string  `gorm:"column:step_type;type:varchar(32);not null" json:"step_type"`
	TargetAgentID *string `gorm:"column:target_agent_id;type:varchar(64);index" json:"target_agent_id,omitempty"`
	Prompt        string  `gorm:"type:text;not null" json:"prompt"`
	Status        string  `gorm:"type:varchar(32);not null" json:"status"`
	TaskID        *string `gorm:"column:task_id;type:varchar(64);index" json:"task_id,omitempty"`
	TaskRunID     *string `gorm:"column:task_run_id;type:varchar(64);index" json:"task_run_id,omitempty"`
	OutputSummary *string `gorm:"type:text" json:"output_summary,omitempty"`
	ErrorMessage  *string `gorm:"type:text" json:"error_message,omitempty"`
	CreatedAt     int64   `gorm:"autoCreateTime" json:"created_at"`
	StartedAt     *int64  `gorm:"" json:"started_at,omitempty"`
	EndedAt       *int64  `gorm:"" json:"ended_at,omitempty"`
}

// TableName returns the table name for GORM.
func (WorkflowStepRun) TableName() string { return "workflow_step_run" }

// Issue is the user-facing work-management object. It is intentionally separate
// from low-level task/task_run execution records.
type Issue struct {
	ID           uint    `gorm:"primaryKey;autoIncrement" json:"-"`
	IssueID      string  `gorm:"column:issue_id;type:varchar(64);uniqueIndex;not null" json:"issue_id"`
	UserID       string  `gorm:"type:varchar(64);not null;index" json:"user_id"`
	TeamID       string  `gorm:"type:varchar(64);index" json:"team_id,omitempty"`
	Title        string  `gorm:"type:varchar(255);not null" json:"title"`
	Description  string  `gorm:"type:text;not null" json:"description"`
	Status       string  `gorm:"type:varchar(32);not null" json:"status"`
	AssigneeKind *string `gorm:"type:varchar(32)" json:"assignee_kind,omitempty"`
	AssigneeID   *string `gorm:"type:varchar(64)" json:"assignee_id,omitempty"`
	CreatedBy    string  `gorm:"type:varchar(64);not null" json:"created_by"`
	CreatedAt    int64   `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    int64   `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name for GORM (singular per project convention).
func (Issue) TableName() string { return "issue" }

// Task is the task model. JSON uses snake_case per project convention.
// API exposes task_id as "id". Task holds denormalized "last run" state (status, output, etc.)
// and LastRunID for conversation/artifact lookup. Input is the initial (first run) prompt.
type Task struct {
	ID                    uint    `gorm:"primaryKey;autoIncrement" json:"-"`
	TaskID                string  `gorm:"column:task_id;type:varchar(64);uniqueIndex;not null" json:"task_id"`
	ConversationID        string  `gorm:"type:varchar(64);not null;index" json:"conversation_id"`
	TeamID                string  `gorm:"type:varchar(64);index" json:"team_id,omitempty"`
	Status                string  `gorm:"type:varchar(32);not null" json:"status"`
	Input                 string  `gorm:"type:text;not null" json:"input"`
	Title                 string  `gorm:"type:varchar(256)" json:"title,omitempty"`
	TitlePromptTokens     int     `gorm:"" json:"title_prompt_tokens,omitempty"`
	TitleCompletionTokens int     `gorm:"" json:"title_completion_tokens,omitempty"`
	Output                *string `gorm:"type:text" json:"output,omitempty"`
	CreatedBy             string  `gorm:"type:varchar(64);not null" json:"created_by"`
	CreatedAt             int64   `gorm:"autoCreateTime" json:"created_at"`
	StartedAt             *int64  `gorm:"" json:"started_at,omitempty"`
	EndedAt               *int64  `gorm:"" json:"ended_at,omitempty"`
	ErrorMessage          *string `gorm:"type:text" json:"error_message,omitempty"`
	SessionID             *string `gorm:"type:varchar(36)" json:"session_id,omitempty"`
	LastRunID             *string `gorm:"type:varchar(64);index" json:"last_run_id,omitempty"`
	AgentID               *string `gorm:"type:varchar(64);index" json:"agent_id,omitempty"`
}

// TableName returns the table name for GORM (singular per project convention).
func (Task) TableName() string { return "task" }

// TaskRun is one execution (initial or follow-up) of a task. Status: PENDING → SCHEDULED → RUNNING → SUCCEEDED | FAILED.
type TaskRun struct {
	ID               uint    `gorm:"primaryKey;autoIncrement" json:"-"`
	TaskRunID        string  `gorm:"column:task_run_id;type:varchar(64);uniqueIndex;not null" json:"task_run_id"`
	TaskID           string  `gorm:"column:task_id;type:varchar(64);not null;index" json:"task_id"`
	Input            string  `gorm:"type:text;not null" json:"input"`
	Status           string  `gorm:"type:varchar(32);not null" json:"status"`
	Output           *string `gorm:"type:text" json:"output,omitempty"`
	ErrorMessage     *string `gorm:"type:text" json:"error_message,omitempty"`
	StartedAt        *int64  `gorm:"" json:"started_at,omitempty"`
	EndedAt          *int64  `gorm:"" json:"ended_at,omitempty"`
	SessionID        *string `gorm:"type:varchar(36)" json:"session_id,omitempty"`
	WorkerType       string  `gorm:"type:varchar(32)" json:"worker_type,omitempty"`
	K8sJobName       *string `gorm:"type:varchar(128)" json:"k8s_job_name,omitempty"`
	K8sJobCreatedAt  *int64  `gorm:"column:k8s_job_created_at" json:"k8s_job_created_at,omitempty"`
	PromptTokens     *int    `gorm:"" json:"prompt_tokens,omitempty"`
	CompletionTokens *int    `gorm:"" json:"completion_tokens,omitempty"`
	CreatedAt        int64   `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM (singular).
func (TaskRun) TableName() string { return "task_run" }

// TaskRunArtifact is one output file (artifact) for a task run. Table name aligns with blob store path term. JSON uses snake_case.
type TaskRunArtifact struct {
	ID           uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	TaskRunID    string `gorm:"type:varchar(64);not null;uniqueIndex:uq_task_run_artifact_run_path" json:"task_run_id"`
	RelativePath string `gorm:"type:varchar(512);not null;uniqueIndex:uq_task_run_artifact_run_path" json:"relative_path"`
}

// TableName returns the table name for GORM (singular per project convention).
func (TaskRunArtifact) TableName() string { return "task_run_artifact" }

// Agent is the agent model (user-scoped persona). JSON uses snake_case.
// Internal DB numeric id is not exposed; API uses agent_id.
type Agent struct {
	ID           uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	AgentID      string `gorm:"type:varchar(64);uniqueIndex;not null" json:"agent_id"`
	UserID       string `gorm:"type:varchar(64);not null;index" json:"user_id"`
	TeamID       string `gorm:"type:varchar(64);index" json:"team_id,omitempty"`
	Name         string `gorm:"type:varchar(255);not null" json:"name"`
	Description  string `gorm:"type:text" json:"description"`
	Instructions string `gorm:"type:text" json:"instructions"`
	CreatedAt    int64  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM (singular per project convention).
func (Agent) TableName() string { return "agent" }

// QuotaTier is one row in the quota_tier table; defines limits for a tier (e.g. free_trial, pro).
type QuotaTier struct {
	TierName           string `gorm:"column:tier_name;primaryKey;type:varchar(64)" json:"tier_name"`
	MaxRunsPerPeriod   int    `gorm:"column:max_runs_per_period;not null" json:"max_runs_per_period"`
	MaxTokensPerPeriod int    `gorm:"column:max_tokens_per_period;not null" json:"max_tokens_per_period"`
	PeriodDays         int    `gorm:"column:period_days;not null" json:"period_days"`
}

// TableName returns the table name for GORM (singular per project convention).
func (QuotaTier) TableName() string { return "quota_tier" }

// ArtifactWithTask is a DTO for listing run outputs (artifacts) with task/run context. ArtifactID holds task_run_id for API compatibility. JSON uses snake_case.
type ArtifactWithTask struct {
	ArtifactID       string `json:"artifact_id"`
	TaskID           string `json:"task_id"`
	TaskRunID        string `json:"task_run_id"`
	ConversationID   string `json:"conversation_id"`
	UserID           string `json:"user_id"`
	CreatedAt        int64  `json:"created_at"`
	TaskInputSnippet string `json:"task_input_snippet"`
}

// Conversation is the Tier 1 conversation container (multi-turn from portal, cron, webhook, telegram). JSON uses snake_case.
type Conversation struct {
	ID             uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	ConversationID string `gorm:"type:varchar(64);uniqueIndex;not null" json:"conversation_id"`
	UserID         string `gorm:"type:varchar(64);not null;index" json:"user_id"`
	TeamID         string `gorm:"type:varchar(64);index" json:"team_id,omitempty"`
	Channel        string `gorm:"type:varchar(32);not null" json:"channel"`
	Title          string `gorm:"type:varchar(256)" json:"title,omitempty"`
	CreatedBy      string `gorm:"type:varchar(64);not null" json:"created_by"`
	CreatedAt      int64  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM (singular per project convention).
func (Conversation) TableName() string { return "conversation" }

// ConversationMessage is one message in a Tier 1 conversation (user, assistant, system, or tool). JSON uses snake_case.
type ConversationMessage struct {
	ID                    uint    `gorm:"primaryKey;autoIncrement" json:"-"`
	ConversationMessageID string  `gorm:"type:varchar(64);uniqueIndex;not null" json:"conversation_message_id"`
	ConversationID        string  `gorm:"type:varchar(64);not null;index" json:"conversation_id"`
	Role                  string  `gorm:"type:varchar(16);not null" json:"role"`
	Content               string  `gorm:"type:text;not null" json:"content"`
	Channel               *string `gorm:"type:varchar(32)" json:"channel,omitempty"`
	ToolCallID            *string `gorm:"type:varchar(64);column:tool_call_id" json:"tool_call_id,omitempty"`
	ToolCallsJSON         *string `gorm:"type:text;column:tool_calls" json:"tool_calls,omitempty"`
	CreatedAt             int64   `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM (singular per project convention).
func (ConversationMessage) TableName() string { return "conversation_message" }
