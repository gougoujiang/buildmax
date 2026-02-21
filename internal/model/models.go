// Package model contains BuildMax domain types shared across layers.
//
// Notes:
// - JSON uses snake_case per project convention.
// - These types intentionally avoid importing persistence libraries; however, they may
//   carry struct tags/methods used by the persistence layer (e.g. GORM) as metadata.
package model

// User is the user model. JSON uses snake_case per project convention.
// Internal DB numeric id is intentionally not exposed; API and JWT use user_id.
type User struct {
	ID        uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID    string `gorm:"type:varchar(64);uniqueIndex;not null" json:"user_id"`
	Email     string `gorm:"type:varchar(255);uniqueIndex;not null" json:"email"`
	Name      string `gorm:"type:varchar(255)" json:"name"`
	CreatedAt int64  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM (singular per project convention).
func (User) TableName() string { return "user" }

// Workspace is the workspace model. JSON uses snake_case per project convention.
// Internal DB numeric id is intentionally not exposed; API uses workspace_id.
type Workspace struct {
	ID          uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	WorkspaceID string `gorm:"type:varchar(64);uniqueIndex;not null" json:"workspace_id"`
	OwnerUserID string `gorm:"type:varchar(64);not null;index" json:"owner_user_id"`
	Name        string `gorm:"type:varchar(255);not null" json:"name"`
	CreatedAt   int64  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM (singular per project convention).
func (Workspace) TableName() string { return "workspace" }

// Chat is the chat model. JSON uses snake_case per project convention.
// API exposes chat_id as "id". Chat holds denormalized "last run" state (status, output, etc.)
// and LastRunID for conversation/artifact lookup. Input is the initial (first run) prompt.
type Chat struct {
	ID              uint    `gorm:"primaryKey;autoIncrement" json:"-"`
	ChatID          string  `gorm:"type:varchar(64);uniqueIndex;not null" json:"chat_id"`
	WorkspaceID     string  `gorm:"type:varchar(64);not null;index" json:"workspace_id"`
	Status          string  `gorm:"type:varchar(32);not null" json:"status"`
	Input           string  `gorm:"type:text;not null" json:"input"`
	Title           string  `gorm:"type:varchar(256)" json:"title,omitempty"`
	Output          *string `gorm:"type:text" json:"output,omitempty"`
	CreatedBy       string  `gorm:"type:varchar(64);not null" json:"created_by"`
	CreatedAt       int64   `gorm:"autoCreateTime" json:"created_at"`
	StartedAt       *int64  `gorm:"" json:"started_at,omitempty"`
	EndedAt         *int64  `gorm:"" json:"ended_at,omitempty"`
	ErrorMessage    *string `gorm:"type:text" json:"error_message,omitempty"`
	SessionID       *string `gorm:"type:varchar(36)" json:"session_id,omitempty"`
	LastRunID *string `gorm:"type:varchar(64);index" json:"last_run_id,omitempty"`
}

// ChatRun is one execution (initial or follow-up) of a chat. Status: PENDING → SCHEDULED → RUNNING → SUCCEEDED | FAILED.
type ChatRun struct {
	ID              uint    `gorm:"primaryKey;autoIncrement" json:"-"`
	ChatRunID       string  `gorm:"type:varchar(64);uniqueIndex;not null" json:"chat_run_id"`
	ChatID          string  `gorm:"type:varchar(64);not null;index" json:"chat_id"`
	Input           string  `gorm:"type:text;not null" json:"input"`
	Status          string  `gorm:"type:varchar(32);not null" json:"status"`
	Output          *string `gorm:"type:text" json:"output,omitempty"`
	ErrorMessage    *string `gorm:"type:text" json:"error_message,omitempty"`
	StartedAt       *int64  `gorm:"" json:"started_at,omitempty"`
	EndedAt         *int64  `gorm:"" json:"ended_at,omitempty"`
	SessionID       *string `gorm:"type:varchar(36)" json:"session_id,omitempty"`
	WorkerType      string  `gorm:"type:varchar(32)" json:"worker_type,omitempty"`
	K8sJobName      *string `gorm:"type:varchar(128)" json:"k8s_job_name,omitempty"`
	K8sJobCreatedAt *int64  `gorm:"column:k8s_job_created_at" json:"k8s_job_created_at,omitempty"`
	CreatedAt       int64   `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM (singular).
func (ChatRun) TableName() string { return "chat_run" }

// TableName returns the table name for GORM (singular per project convention).
func (Chat) TableName() string { return "chat" }

// ChatRunOutputFile is one output file for a chat run (replaces artifact_item keyed by chat_run_id). JSON uses snake_case.
type ChatRunOutputFile struct {
	ChatRunID    string `gorm:"type:varchar(64);not null;primaryKey" json:"chat_run_id"`
	RelativePath string `gorm:"type:varchar(512);not null;primaryKey" json:"relative_path"`
}

// TableName returns the table name for GORM (singular per project convention).
func (ChatRunOutputFile) TableName() string { return "chat_run_output_file" }

// Agent is the agent model (workspace-scoped persona). JSON uses snake_case.
// Internal DB numeric id is not exposed; API uses agent_id.
type Agent struct {
	ID           uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	AgentID      string `gorm:"type:varchar(64);uniqueIndex;not null" json:"agent_id"`
	WorkspaceID  string `gorm:"type:varchar(64);not null;index" json:"workspace_id"`
	Name         string `gorm:"type:varchar(255);not null" json:"name"`
	Description  string `gorm:"type:text" json:"description"`
	Instructions string `gorm:"type:text" json:"instructions"`
	CreatedAt    int64  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM (singular per project convention).
func (Agent) TableName() string { return "agent" }

// ArtifactWithChat is a DTO for listing run outputs (artifacts) with chat/run context. ArtifactID holds chat_run_id for API compatibility. JSON uses snake_case.
type ArtifactWithChat struct {
	ArtifactID      string `json:"artifact_id"` // chat_run_id
	ChatID          string `json:"chat_id"`
	ChatRunID       string `json:"chat_run_id"`
	WorkspaceID     string `json:"workspace_id"`
	CreatedAt       int64  `json:"created_at"`
	ChatInputSnippet string `json:"chat_input_snippet"`
}
