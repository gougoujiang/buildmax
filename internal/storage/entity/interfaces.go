package entity

import "context"

// UserStore looks up users by email and creates new users.
type UserStore interface {
	UserByEmail(ctx context.Context, email string) (*User, error)
	// CreateUser creates a user with the given email. Returns ErrEmailExists if the email is already registered.
	CreateUser(ctx context.Context, email string) (*User, error)
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

// ProjectStore provides project persistence.
type ProjectStore interface {
	GetProject(ctx context.Context, projectID string) (*Project, error)
	ListProjectsByWorkspace(ctx context.Context, workspaceID string) ([]Project, error)
	CreateProject(ctx context.Context, workspaceID, name, description string) (*Project, error)
}

// TaskStore provides task persistence. Tasks belong to a workspace; project is optional.
type TaskStore interface {
	// ListTasksByWorkspace returns tasks in the workspace. If projectID is non-nil, filter by that project.
	ListTasksByWorkspace(ctx context.Context, workspaceID string, projectID *string) ([]Task, error)
	// GetTask returns the task by task_id, or (nil, nil) if not found.
	GetTask(ctx context.Context, taskID string) (*Task, error)
	// GetTaskBySessionID returns the task that has the given session_id, or (nil, nil) if none.
	GetTaskBySessionID(ctx context.Context, sessionID string) (*Task, error)
	// CreateTask inserts a new task with status PENDING. projectID is optional (nil = no project).
	CreateTask(ctx context.Context, workspaceID string, projectID *string, input, createdBy string) (*Task, error)
	// GetNextPendingTask returns the oldest task with status PENDING (by created_at), or (nil, nil) if none.
	GetNextPendingTask(ctx context.Context) (*Task, error)
	// UpdateTaskStatus updates a task's status and optional fields (started_at, ended_at, output, error_message, session_id).
	// Only non-nil pointer fields are updated.
	UpdateTaskStatus(ctx context.Context, taskID, status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) error
	// UpdateTaskStatusIf updates a task's status and optional fields only when current status equals expectedStatus.
	// Returns updated = (exactly one row was updated). Used for atomic claim (e.g. PENDING→SCHEDULED, SCHEDULED→RUNNING).
	UpdateTaskStatusIf(ctx context.Context, taskID, expectedStatus, newStatus string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) (updated bool, err error)
	// UpdateTaskWorkerInfo updates worker_type, k8s_job_name, k8s_job_created_at for the task. Used after scheduler runs a worker.
	UpdateTaskWorkerInfo(ctx context.Context, taskID, workerType string, k8sJobName *string, k8sJobCreatedAt *int64) error
	// IncrementTaskSeq atomically increments the task's artifact_seq and returns the new value.
	IncrementTaskSeq(ctx context.Context, taskID string) (newSeq int, err error)
}

// ArtifactStore provides artifact persistence.
type ArtifactStore interface {
	// CreateArtifactWithItem creates one artifact row, one artifact_item row, and updates task.last_artifact_id. All in a transaction.
	CreateArtifactWithItem(ctx context.Context, taskID, artifactID string, seq int, relativePath string) error
	// ListArtifactsByWorkspace returns artifacts in the workspace, optionally filtered by task_id and project_id. Order: created_at DESC. Task_input_snippet is truncated to 200 chars.
	ListArtifactsByWorkspace(ctx context.Context, workspaceID string, taskID, projectID *string) ([]ArtifactWithTask, error)
	// GetArtifactByID returns the artifact by artifact_id, or (nil, nil) if not found.
	GetArtifactByID(ctx context.Context, artifactID string) (*Artifact, error)
	// ListArtifactItems returns all artifact_item rows for the given artifact_id, ordered by id.
	ListArtifactItems(ctx context.Context, artifactID string) ([]ArtifactItem, error)
}
