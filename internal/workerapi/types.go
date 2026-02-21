// Package workerapi defines the HTTP contract for the worker API
// (GET/PATCH /api/worker/chat-runs/{chat_run_id}). Server and executor both use these types.
package workerapi

// GetChatRunResponse is the JSON response for GET /api/worker/chat-runs/{chat_run_id} (snake_case).
type GetChatRunResponse struct {
	Run  ChatRunRun  `json:"run"`
	Chat ChatRunChat `json:"chat"`
}

// ChatRunRun is the run portion of the GET response.
type ChatRunRun struct {
	ChatRunID string `json:"chat_run_id"`
	ChatID    string `json:"chat_id"`
	Input     string `json:"input"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
}

// ChatRunChat is the chat portion of the GET response.
type ChatRunChat struct {
	ChatID    string  `json:"chat_id"`
	WorkspaceID string  `json:"workspace_id"`
	SessionID   *string `json:"session_id,omitempty"`
	LastRunID   *string `json:"last_run_id,omitempty"`
}

// PatchChatRunRequest is the JSON body for PATCH /api/worker/chat-runs/{chat_run_id} (snake_case).
type PatchChatRunRequest struct {
	Status       string  `json:"status"`
	SessionID    *string `json:"session_id,omitempty"`
	StartedAt    *int64  `json:"started_at,omitempty"`
	EndedAt      *int64  `json:"ended_at,omitempty"`
	Output       *string `json:"output,omitempty"`
	ErrorMessage *string `json:"error_message,omitempty"`
	Artifact     *ArtifactPayload `json:"artifact,omitempty"`
}

// ArtifactPayload is the artifact field in PATCH (and the payload passed to ChatRunUpdater).
type ArtifactPayload struct {
	ArtifactID    string   `json:"artifact_id"`
	RelativePath  string   `json:"relative_path,omitempty"`   // deprecated: use relative_paths
	RelativePaths []string `json:"relative_paths,omitempty"`  // all files in the artifact (e.g. result.md, other generated files)
}

// Run status constants for the worker API.
const (
	StatusPending   = "PENDING"
	StatusScheduled = "SCHEDULED"
	StatusRunning   = "RUNNING"
	StatusSucceeded = "SUCCEEDED"
	StatusFailed    = "FAILED"
)
