package store

import "errors"

// User is the user model. JSON uses snake_case per project convention.
// Internal id is for DB only; API and JWT use user_id.
type User struct {
	ID        uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID    string `gorm:"type:varchar(64);uniqueIndex;not null" json:"user_id"`
	Email     string `gorm:"type:varchar(255);uniqueIndex;not null" json:"email"`
	Name      string `gorm:"type:varchar(255)" json:"name"`
	CreatedAt int64  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM (singular per project convention).
func (User) TableName() string {
	return "user"
}

// Workspace is the workspace model. JSON uses snake_case per project convention.
// Internal id is for DB only; API uses workspace_id.
type Workspace struct {
	ID           uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	WorkspaceID  string `gorm:"type:varchar(64);uniqueIndex;not null" json:"workspace_id"`
	OwnerUserID  string `gorm:"type:varchar(64);not null;index" json:"owner_user_id"`
	Name         string `gorm:"type:varchar(255);not null" json:"name"`
	CreatedAt    int64  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM (singular per project convention).
func (Workspace) TableName() string {
	return "workspace"
}

// Project is the project model. JSON uses snake_case per project convention.
// Internal id is for DB only; API uses project_id.
type Project struct {
	ID          uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	ProjectID   string `gorm:"type:varchar(64);uniqueIndex;not null" json:"project_id"`
	WorkspaceID string `gorm:"type:varchar(64);not null;index" json:"workspace_id"`
	Name        string `gorm:"type:varchar(255);not null" json:"name"`
	Description string `gorm:"type:text" json:"description"`
	CreatedAt   int64  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the table name for GORM (singular per project convention).
func (Project) TableName() string {
	return "project"
}

// Task is the task model. JSON uses snake_case per project convention.
// API exposes task_id as "id"; internal ID is for DB only.
// Tasks belong to a workspace; project is optional.
type Task struct {
	ID              uint    `gorm:"primaryKey;autoIncrement" json:"-"` // internal only, not in API
	TaskID          string  `gorm:"type:varchar(64);uniqueIndex;not null" json:"task_id"`
	WorkspaceID     string  `gorm:"type:varchar(64);not null;index" json:"workspace_id"`
	ProjectID       *string `gorm:"type:varchar(64);index" json:"project_id,omitempty"`
	Status          string  `gorm:"type:varchar(32);not null" json:"status"`
	Input           string  `gorm:"type:text;not null" json:"input"`
	Output          *string `gorm:"type:text" json:"output,omitempty"`
	CreatedBy       string  `gorm:"type:varchar(64);not null" json:"created_by"`
	CreatedAt       int64   `gorm:"autoCreateTime" json:"created_at"`
	StartedAt       *int64  `gorm:"" json:"started_at,omitempty"`
	EndedAt         *int64  `gorm:"" json:"ended_at,omitempty"`
	ErrorMessage    *string `gorm:"type:text" json:"error_message,omitempty"`
	SessionID       *string `gorm:"type:varchar(36)" json:"session_id,omitempty"`
	ArtifactSeq     int     `gorm:"column:artifact_seq" json:"artifact_seq"`
	LastArtifactID  *string `gorm:"type:varchar(64)" json:"last_artifact_id,omitempty"`
}

// TableName returns the table name for GORM (singular per project convention).
func (Task) TableName() string {
	return "task"
}

// Artifact is the artifact model (one per task run output). JSON uses snake_case.
type Artifact struct {
	ID         uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	TaskID     string `gorm:"type:varchar(64);not null;index" json:"task_id"`
	ArtifactID string `gorm:"type:varchar(64);uniqueIndex;not null" json:"artifact_id"`
	CreatedAt  int64  `gorm:"autoCreateTime" json:"created_at"`
	Seq        int    `gorm:"not null" json:"seq"`
}

// TableName returns the table name for GORM (singular per project convention).
func (Artifact) TableName() string {
	return "artifact"
}

// ArtifactItem is one file in an artifact. JSON uses snake_case.
type ArtifactItem struct {
	ID           uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	ArtifactID   string `gorm:"type:varchar(64);not null;index" json:"artifact_id"`
	RelativePath string `gorm:"type:varchar(512);not null" json:"relative_path"`
}

// TableName returns the table name for GORM (singular per project convention).
func (ArtifactItem) TableName() string {
	return "artifact_item"
}

// ArtifactWithTask is a DTO for listing artifacts with task context (not a table). JSON uses snake_case.
type ArtifactWithTask struct {
	ArtifactID       string  `json:"artifact_id"`
	TaskID           string  `json:"task_id"`
	WorkspaceID      string  `json:"workspace_id"`
	ProjectID        *string `json:"project_id,omitempty"`
	CreatedAt        int64   `json:"created_at"`
	Seq              int     `json:"seq"`
	TaskInputSnippet string  `json:"task_input_snippet"`
}

// ErrEmailExists is returned by CreateUser when the email is already registered.
var ErrEmailExists = errors.New("email already exists")
