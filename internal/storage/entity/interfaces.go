package entity

import (
	"context"
	"errors"
)

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
// CreateTask creates task + first TaskRun (both in one transaction).
type TaskStore interface {
	ListTasksByWorkspace(ctx context.Context, workspaceID string, projectID *string) ([]Task, error)
	GetTask(ctx context.Context, taskID string) (*Task, error)
	GetTaskBySessionID(ctx context.Context, sessionID string) (*Task, error)
	// CreateTask creates a new task and its first TaskRun (input, title, PENDING). Returns the task with last_run_id set.
	CreateTask(ctx context.Context, workspaceID string, projectID *string, input, title, createdBy string) (*Task, error)
	UpdateTaskStatus(ctx context.Context, taskID, status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) error
	UpdateTaskStatusIf(ctx context.Context, taskID, expectedStatus, newStatus string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) (updated bool, err error)
	IncrementTaskSeq(ctx context.Context, taskID string) (newSeq int, err error)
}

// ErrRunInProgress is returned by CreateTaskRun when the task already has a run in PENDING, SCHEDULED, or RUNNING.
var ErrRunInProgress = errors.New("task has a run already in progress")

// TaskRunStore provides task run persistence.
type TaskRunStore interface {
	// CreateTaskRun creates a new run (PENDING). Returns ErrRunInProgress if task has any run in PENDING/SCHEDULED/RUNNING.
	CreateTaskRun(ctx context.Context, taskID, input, createdBy string) (*TaskRun, error)
	// GetNextPendingTaskRun returns the oldest run with status PENDING (by created_at), or (nil, nil) if none.
	GetNextPendingTaskRun(ctx context.Context) (*TaskRun, error)
	GetTaskRun(ctx context.Context, runID string) (*TaskRun, error)
	// GetTaskRunWithTask returns the run and its task, or (nil, nil, nil) if run not found.
	GetTaskRunWithTask(ctx context.Context, runID string) (*TaskRun, *Task, error)
	// UpdateTaskRunStatusIf atomically updates run status when current status equals expectedStatus. Returns updated.
	UpdateTaskRunStatusIf(ctx context.Context, runID, expectedStatus, newStatus string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) (bool, error)
	UpdateTaskRunStatus(ctx context.Context, runID, status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) error
	UpdateTaskRunWorkerInfo(ctx context.Context, runID, workerType string, k8sJobName *string, k8sJobCreatedAt *int64) error
	// OnRunComplete creates artifact, updates task denormalized fields and last_run_id, and task.session_id if run set it. Use for SUCCEEDED runs.
	OnRunComplete(ctx context.Context, runID, artifactID, relativePath string) error
	// SyncTaskFromRun updates task denormalized fields and last_run_id from the run (no artifact). Use for FAILED runs.
	SyncTaskFromRun(ctx context.Context, runID string) error
}

// ArtifactStore provides artifact persistence.
type ArtifactStore interface {
	// CreateArtifactWithItem creates one artifact row (with task_run_id), one artifact_item row, and updates task.last_artifact_id.
	CreateArtifactWithItem(ctx context.Context, taskID, taskRunID, artifactID string, seq int, relativePath string) error
	// ListArtifactsByWorkspace returns artifacts in the workspace, optionally filtered by task_id and project_id. Order: created_at DESC. Task_input_snippet is truncated to 200 chars.
	ListArtifactsByWorkspace(ctx context.Context, workspaceID string, taskID, projectID *string) ([]ArtifactWithTask, error)
	// GetArtifactByID returns the artifact by artifact_id, or (nil, nil) if not found.
	GetArtifactByID(ctx context.Context, artifactID string) (*Artifact, error)
	// ListArtifactItems returns all artifact_item rows for the given artifact_id, ordered by id.
	ListArtifactItems(ctx context.Context, artifactID string) ([]ArtifactItem, error)
}
