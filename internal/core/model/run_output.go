package model

import "time"

// Run output is not an Artifact. An Artifact is a file somebody chose to keep,
// addressed by its own ar_ ID and governed by team authorization and retention;
// these describe what a task run left behind, which is reproducibility state
// keyed by the run. See docs/design/unified-artifacts.md.
//
// They live here until the task domain moves out of this package.

// TaskRunArtifact is one output file (artifact) for a task run.
type TaskRunArtifact struct {
	TaskRunID    string `json:"task_run_id"`
	RelativePath string `json:"relative_path"`
}

// ArtifactWithTask is a DTO for listing run outputs (artifacts) with task/run context.
// ArtifactID holds task_run_id for API compatibility.
type ArtifactWithTask struct {
	ArtifactID       string    `json:"artifact_id"`
	TaskID           string    `json:"task_id"`
	TaskRunID        string    `json:"task_run_id"`
	ConversationID   string    `json:"conversation_id"`
	UserID           string    `json:"user_id"`
	CreatedAt        time.Time `json:"created_at"`
	TaskInputSnippet string    `json:"task_input_snippet"`
}
