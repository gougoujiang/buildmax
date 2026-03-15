package entity

// User is the user model. JSON uses snake_case per project convention.
// Internal DB numeric id is intentionally not exposed; API and JWT use user_id.
type User struct {
	ID        uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID    string `gorm:"type:varchar(64);uniqueIndex;not null" json:"user_id"`
	Email     string `gorm:"type:varchar(255);uniqueIndex;not null" json:"email"`
	Name      string `gorm:"type:varchar(255)" json:"name"`
	QuotaTier string `gorm:"type:varchar(64)" json:"quota_tier,omitempty"`
	CreatedAt int64  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM (singular per project convention).
func (User) TableName() string { return "user" }

// Chat is the task model. JSON uses snake_case per project convention.
// API exposes task_id as "id". Chat holds denormalized "last run" state (status, output, etc.)
// and LastRunID for conversation/artifact lookup. Input is the initial (first run) prompt.
type Chat struct {
	ID                    uint    `gorm:"primaryKey;autoIncrement" json:"-"`
	ChatID                string  `gorm:"column:task_id;type:varchar(64);uniqueIndex;not null" json:"task_id"`
	ConversationID        string  `gorm:"type:varchar(64);not null;index" json:"conversation_id"`
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
func (Chat) TableName() string { return "task" }

// TaskRun is one execution (initial or follow-up) of a task. Status: PENDING → SCHEDULED → RUNNING → SUCCEEDED | FAILED.
type TaskRun struct {
	ID               uint    `gorm:"primaryKey;autoIncrement" json:"-"`
	TaskRunID        string  `gorm:"column:task_run_id;type:varchar(64);uniqueIndex;not null" json:"task_run_id"`
	ChatID           string  `gorm:"column:task_id;type:varchar(64);not null;index" json:"task_id"`
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

// ArtifactWithChat is a DTO for listing run outputs (artifacts) with task/run context. ArtifactID holds task_run_id for API compatibility. JSON uses snake_case.
type ArtifactWithChat struct {
	ArtifactID       string `json:"artifact_id"`
	ChatID           string `json:"task_id"`
	TaskRunID        string `json:"task_run_id"`
	ConversationID   string `json:"conversation_id"`
	UserID           string `json:"user_id"`
	CreatedAt        int64  `json:"created_at"`
	ChatInputSnippet string `json:"task_input_snippet"`
}

// Conversation is the Tier 1 conversation container (multi-turn from portal, cron, webhook, telegram). JSON uses snake_case.
type Conversation struct {
	ID             uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	ConversationID string `gorm:"type:varchar(64);uniqueIndex;not null" json:"conversation_id"`
	UserID         string `gorm:"type:varchar(64);not null;index" json:"user_id"`
	Channel        string `gorm:"type:varchar(32);not null" json:"channel"`
	Title          string `gorm:"type:varchar(256)" json:"title,omitempty"`
	CreatedBy      string `gorm:"type:varchar(64);not null" json:"created_by"`
	CreatedAt      int64  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM (singular per project convention).
func (Conversation) TableName() string { return "conversation" }

// ConversationMessage is one message in a Tier 1 conversation (user, assistant, or tool). JSON uses snake_case.
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
