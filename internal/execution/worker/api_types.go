// Package worker defines the worker API client and HTTP contract types.
package worker

// GetTaskRunResponse is the JSON response for GET /api/worker/task-runs/{task_run_id} (snake_case).
type GetTaskRunResponse struct {
	Run  TaskRunRun  `json:"run"`
	Task TaskRunTask `json:"task"`
}

// TaskRunRun is the run portion of the GET response.
type TaskRunRun struct {
	TaskRunID string `json:"task_run_id"`
	TaskID    string `json:"task_id"`
	Input     string `json:"input"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
}

// TaskRunTask is the task portion of the GET response.
type TaskRunTask struct {
	TaskID         string  `json:"task_id"`
	ConversationID string  `json:"conversation_id"`
	TeamID         string  `json:"team_id"`
	UserID         string  `json:"user_id"`
	SessionID      *string `json:"session_id,omitempty"`
	LastRunID      *string `json:"last_run_id,omitempty"`
}

// PatchTaskRunRequest is the JSON body for PATCH /api/worker/task-runs/{task_run_id} (snake_case).
type PatchTaskRunRequest struct {
	Status           string           `json:"status"`
	SessionID        *string          `json:"session_id,omitempty"`
	StartedAt        *int64           `json:"started_at,omitempty"`
	EndedAt          *int64           `json:"ended_at,omitempty"`
	Output           *string          `json:"output,omitempty"`
	ErrorMessage     *string          `json:"error_message,omitempty"`
	Artifact         *ArtifactPayload `json:"artifact,omitempty"`
	PromptTokens     *int             `json:"prompt_tokens,omitempty"`
	CompletionTokens *int             `json:"completion_tokens,omitempty"`
}

// ArtifactPayload is the artifact field in PATCH when status is SUCCEEDED (run output files).
type ArtifactPayload struct {
	RelativePath  string   `json:"relative_path,omitempty"`  // deprecated: use relative_paths
	RelativePaths []string `json:"relative_paths,omitempty"` // all files in the run output (e.g. result.md)
}

// StreamDeltaRequest is the JSON body for POST /api/worker/task-runs/{task_run_id}/stream (snake_case).
type StreamDeltaRequest struct {
	Delta string `json:"delta"`
}

// Run status values match model.RunStatus (PENDING, SCHEDULED, RUNNING, SUCCEEDED, FAILED).
// Use model.RunStatusPending, model.RunStatusScheduled, etc. when building or comparing status.
