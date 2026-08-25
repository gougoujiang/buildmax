package task

import "time"

// Run output is not an Artifact. An Artifact is a file somebody chose to keep,
// addressed by its own ar_ ID and governed by team authorization and retention;
// a run output is what a run left behind, which is reproducibility state keyed
// by the run. They were named TaskRunArtifact and ArtifactWithTask, which read
// as Artifact types and are not. See docs/design/unified-artifacts.md.

// RunOutputFile is one file a task run left behind.
type RunOutputFile struct {
	TaskRunID    string `json:"task_run_id"`
	RelativePath string `json:"relative_path"`
}

// RunOutputListing is one row of a run-output listing, with the task and run it
// came from.
//
// ArtifactID holds the task_run_id. The field and its JSON name predate the
// Artifact this is not, and the route that serves it is the run-output
// compatibility path -- renaming either would change the wire.
type RunOutputListing struct {
	ArtifactID       string    `json:"artifact_id"`
	TaskID           string    `json:"task_id"`
	TaskRunID        string    `json:"task_run_id"`
	ConversationID   string    `json:"conversation_id"`
	UserID           string    `json:"user_id"`
	CreatedAt        time.Time `json:"created_at"`
	TaskInputSnippet string    `json:"task_input_snippet"`
}
