// Package workerapi defines the HTTP contract for the worker API
// (GET/PATCH /api/worker/task-runs/{run_id}). Server and executor both use these types.
package workerapi

// GetTaskRunResponse is the JSON response for GET /api/worker/task-runs/{run_id} (snake_case).
type GetTaskRunResponse struct {
	Run  TaskRunRun  `json:"run"`
	Task TaskRunTask `json:"task"`
}

// TaskRunRun is the run portion of the GET response.
type TaskRunRun struct {
	RunID     string `json:"run_id"`
	TaskID    string `json:"task_id"`
	Input     string `json:"input"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
}

// TaskRunTask is the task portion of the GET response.
type TaskRunTask struct {
	TaskID      string  `json:"task_id"`
	WorkspaceID string  `json:"workspace_id"`
	ProjectID   *string `json:"project_id,omitempty"`
	SessionID   *string `json:"session_id,omitempty"`
	LastRunID   *string `json:"last_run_id,omitempty"`
}

// PatchTaskRunRequest is the JSON body for PATCH /api/worker/task-runs/{run_id} (snake_case).
type PatchTaskRunRequest struct {
	Status       string  `json:"status"`
	SessionID    *string `json:"session_id,omitempty"`
	StartedAt    *int64  `json:"started_at,omitempty"`
	EndedAt      *int64  `json:"ended_at,omitempty"`
	Output       *string `json:"output,omitempty"`
	ErrorMessage *string `json:"error_message,omitempty"`
	Artifact     *ArtifactPayload `json:"artifact,omitempty"`
}

// ArtifactPayload is the artifact field in PATCH (and the payload passed to TaskRunUpdater).
type ArtifactPayload struct {
	ArtifactID   string `json:"artifact_id"`
	RelativePath string `json:"relative_path"`
}

// Run status constants for the worker API.
const (
	StatusPending   = "PENDING"
	StatusScheduled = "SCHEDULED"
	StatusRunning   = "RUNNING"
	StatusSucceeded = "SUCCEEDED"
	StatusFailed    = "FAILED"
)
