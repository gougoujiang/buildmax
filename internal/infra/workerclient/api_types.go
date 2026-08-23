// Package workerclient defines the worker API client and HTTP contract types.
package workerclient

import "time"

// GetTaskRunResponse is the JSON response for GET /api/worker/task-runs/{task_run_id} (snake_case).
type GetTaskRunResponse struct {
	Run  TaskRunRun  `json:"run"`
	Task TaskRunTask `json:"task"`
	LLM  *TaskRunLLM `json:"llm,omitempty"`
}

// TaskRunLLM tells a worker how to reach a model for this run.
//
// It is deliberately thin. A managed run learns an alias and nothing else — no
// endpoint, no upstream model identifier, no credential — because those stay
// inside the server's authorization boundary. A direct run learns nothing here
// and reads its model from the server.yaml it already mounts.
//
// Absent means direct, so a worker built before this field behaves as it always
// did.
type TaskRunLLM struct {
	// Transport is "direct" or "buildmax".
	Transport string `json:"transport"`
	// Alias is the team model alias to call. Empty uses the team default.
	Alias string `json:"alias,omitempty"`
	// ContextWindow is the usable context size for the alias; 0 disables
	// windowing.
	ContextWindow int `json:"context_window,omitempty"`
	// CallTimeout bounds one call, in seconds; 0 uses the client default.
	CallTimeout int `json:"call_timeout,omitempty"`
}

// TaskRunRun is the run portion of the GET response.
type TaskRunRun struct {
	ID     string `json:"id"`
	TaskID string `json:"task_id"`
	Input  string `json:"input"`
	Status string `json:"status"`
	// CancelRequested is true once someone has asked this run to stop. The
	// worker polls for it and is what actually stops: the server records the
	// intent, the run's own process ends it. Absent means no request, so a
	// worker built before cancellation existed reads what it always did.
	CancelRequested bool      `json:"cancel_requested,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// TaskRunTask is the task portion of the GET response.
type TaskRunTask struct {
	ID             string  `json:"id"`
	ConversationID string  `json:"conversation_id"`
	TeamID         string  `json:"team_id"`
	UserID         string  `json:"user_id"`
	SessionID      *string `json:"session_id,omitempty"`
	LastRunID      *string `json:"last_run_id,omitempty"`
	// AgentInstructions is the instruction text of the agent this task names, resolved by
	// the server. The worker appends it to the run's system prompt, which is re-sent whole on
	// every call, rather than leaving it in the task input, which the conversation eventually
	// compacts away.
	//
	// It travels here rather than on the worker's command line for the same reason the run
	// token does: argv is readable by every process on the machine, and this is text a user
	// wrote, which may carry something they would not publish.
	//
	// Absent means the task names no agent, or a server built before this field existed, so
	// a worker reads it as it always did.
	AgentInstructions string `json:"agent_instructions,omitempty"`
}

// PatchTaskRunRequest is the JSON body for PATCH /api/worker/task-runs/{task_run_id} (snake_case).
type PatchTaskRunRequest struct {
	Status           string           `json:"status"`
	SessionID        *string          `json:"session_id,omitempty"`
	StartedAt        *time.Time       `json:"started_at,omitempty"`
	EndedAt          *time.Time       `json:"ended_at,omitempty"`
	Output           *string          `json:"output,omitempty"`
	ErrorMessage     *string          `json:"error_message,omitempty"`
	Artifact         *ArtifactPayload `json:"artifact,omitempty"`
	PromptTokens     *int             `json:"prompt_tokens,omitempty"`
	CompletionTokens *int             `json:"completion_tokens,omitempty"`
	// TracePath locates the run's durable trace inside run-global storage, e.g.
	// "traces/<session>/rt_….jsonl". Sent on both success and failure; omitted
	// when no trace was written.
	TracePath *string `json:"trace_path,omitempty"`
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

// Run status values match model.RunStatus (PENDING, SCHEDULED, RUNNING, SUCCEEDED, FAILED, CANCELED).
// Use model.RunStatusPending, model.RunStatusScheduled, etc. when building or comparing status.
