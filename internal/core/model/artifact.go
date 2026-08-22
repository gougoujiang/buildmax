package model

import "context"

// Artifact is a durable file BuildMax holds on a team's behalf.
//
// It is a first-class object rather than a by-product of whatever produced it:
// an agent, a background run, or a person uploading a file all create the same
// record, and the producer is kept as provenance rather than as a parent. That
// is the whole difference from the earlier artifact/artifact_item tables, which
// a task run owned and which migration 0001 removed. See
// docs/design/unified-artifacts.md.
//
// Content is immutable. There is no update path: a changed file is a new
// Artifact, because a reference someone saved must not quietly come to mean
// something else.
type Artifact struct {
	ID        string `json:"id"`
	TeamID    string `json:"team_id"`
	Filename  string `json:"filename"`
	MediaType string `json:"media_type"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
	// StorageKey is where the object store put the bytes. It is never
	// serialized: an API response, tool output, trace, or audit event that
	// carried it would leak deployment layout and outlive the layout's freedom
	// to change.
	StorageKey    string `json:"-"`
	CreatedByType string `json:"created_by_type"`
	CreatedByID   string `json:"created_by_id"`
	SourceType    string `json:"source_type"`
	SourceID      string `json:"source_id,omitempty"`
	Title         string `json:"title,omitempty"`
	// DeletedAt tombstones the artifact. Metadata and content stop being
	// served the moment it is set; removing the object itself is a later,
	// separate step under retention policy.
	DeletedAt *int64 `json:"deleted_at,omitempty"`
	ExpiresAt *int64 `json:"expires_at,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// Deleted reports whether the artifact has been tombstoned.
func (a *Artifact) Deleted() bool { return a != nil && a.DeletedAt != nil }

// Artifact source types record which operation produced the file. They are
// persisted, so they are permanent in the same way audit actions are.
const (
	// ArtifactSourceAgent is an agent that chose to publish a file. SourceID is
	// its run or session ID where one is known.
	ArtifactSourceAgent = "agent"
	// ArtifactSourceTaskRun is a worker's run output. SourceID is the task run.
	ArtifactSourceTaskRun = "task_run"
	// ArtifactSourceUserUpload is a member uploading a file directly.
	ArtifactSourceUserUpload = "user_upload"
	// ArtifactSourceSystem is BuildMax generating a file with no agent call.
	ArtifactSourceSystem = "system"
)

// Artifact creator kinds. These answer "what kind of actor", not "which user":
// automated work does not get a person's ID invented for it, which is the same
// rule the audit trail follows.
const (
	ArtifactCreatorUser   = "user"
	ArtifactCreatorAgent  = "agent"
	ArtifactCreatorWorker = "worker"
	ArtifactCreatorSystem = "system"
)

// CreateArtifactInput is everything the store needs to record one artifact.
// The caller has already stored the content and measured it.
type CreateArtifactInput struct {
	TeamID        string
	ArtifactID    string
	Filename      string
	MediaType     string
	SizeBytes     int64
	SHA256        string
	StorageKey    string
	CreatedByType string
	CreatedByID   string
	SourceType    string
	SourceID      string
	Title         string
	ExpiresAt     *int64
}

// ArtifactStore persists artifact metadata.
//
// It knows nothing about task runs, issues, or conversations. Provenance
// reaches it as two opaque strings, which is what keeps an artifact from
// acquiring an owner it should not have.
type ArtifactStore interface {
	CreateArtifact(ctx context.Context, in CreateArtifactInput) (*Artifact, error)
	// GetArtifact returns the artifact by its ar_ ID, or (nil, nil) when there
	// is none. A tombstoned artifact is returned, not hidden: the caller has to
	// tell "never existed" from "deleted" to answer either one correctly.
	GetArtifact(ctx context.Context, artifactID string) (*Artifact, error)
	// ListArtifactsByTeam returns live artifacts newest first, with the total.
	ListArtifactsByTeam(ctx context.Context, teamID string, limit, offset int) ([]Artifact, int, error)
	// ListArtifactsBySource returns live artifacts produced by any of the given
	// operations, newest first, keyed by source ID. It is how a work object
	// finds what its runs published without owning them.
	ListArtifactsBySource(ctx context.Context, sourceIDs []string) (map[string][]Artifact, error)
	// SoftDeleteArtifact tombstones the artifact and reports whether it changed
	// anything, so a repeat delete is distinguishable from a first one.
	SoftDeleteArtifact(ctx context.Context, artifactID string, deletedAt int64) (bool, error)
}

// TaskRunArtifact is one output file (artifact) for a task run.
type TaskRunArtifact struct {
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
