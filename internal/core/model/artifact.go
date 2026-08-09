package model

// TaskRunArtifact is one output file (artifact) for a task run.
type TaskRunArtifact struct {
	ID           uint   `json:"-"`
	TaskRunID    string `json:"task_run_id"`
	RelativePath string `json:"relative_path"`
}

// ArtifactWithTask is a DTO for listing run outputs (artifacts) with task/run context.
// ArtifactID holds task_run_id for API compatibility.
type ArtifactWithTask struct {
	ArtifactID       string `json:"artifact_id"`
	TaskID           string `json:"task_id"`
	TaskRunID        string `json:"task_run_id"`
	ConversationID   string `json:"conversation_id"`
	UserID           string `json:"user_id"`
	CreatedAt        int64  `json:"created_at"`
	TaskInputSnippet string `json:"task_input_snippet"`
}
