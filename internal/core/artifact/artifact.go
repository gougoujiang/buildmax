package artifact

import (
	"context"
	"time"
)

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
	//
	// It is also the record of whether the bytes are still held. Every artifact
	// gets a key when it is created, so an empty one means retention has
	// reclaimed the object — see Purged. That is why the purge sweep clears it
	// rather than adding a column: "no key" and "nothing stored" are the same
	// fact, and two columns that must agree eventually will not.
	StorageKey    string `json:"-"`
	CreatedByType string `json:"created_by_type"`
	CreatedByID   string `json:"created_by_id"`
	SourceType    string `json:"source_type"`
	SourceID      string `json:"source_id,omitempty"`
	Title         string `json:"title,omitempty"`
	// DeletedAt tombstones the artifact. Metadata and content stop being
	// served the moment it is set; removing the object itself is a later,
	// separate step under retention policy.
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	// ExpiresAt tombstones the artifact once it passes, applied by the
	// retention sweep. An artifact with none is kept until somebody deletes it.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// Deleted reports whether the artifact has been tombstoned.
func (a *Artifact) Deleted() bool { return a != nil && a.DeletedAt != nil }

// Purged reports whether retention has reclaimed the object.
//
// Only meaningful for a tombstoned artifact: a live one always has its bytes.
func (a *Artifact) Purged() bool { return a != nil && a.DeletedAt != nil && a.StorageKey == "" }

// Artifact source types record which operation produced the file. They are
// persisted, so they are permanent in the same way audit actions are.
const (
	// SourceAgent is an agent that chose to publish a file. SourceID is
	// its run or session ID where one is known.
	SourceAgent = "agent"
	// SourceTaskRun is a worker's run output. SourceID is the task run.
	SourceTaskRun = "task_run"
	// SourceUserUpload is a member uploading a file directly.
	SourceUserUpload = "user_upload"
	// SourceSystem is BuildMax generating a file with no agent call.
	SourceSystem = "system"
)

// Artifact creator kinds. These answer "what kind of actor", not "which user":
// automated work does not get a person's ID invented for it, which is the same
// rule the audit trail follows.
const (
	CreatorUser   = "user"
	CreatorAgent  = "agent"
	CreatorWorker = "worker"
	CreatorSystem = "system"
)

// CreateInput is everything the store needs to record one artifact.
// The caller has already stored the content and measured it.
type CreateInput struct {
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
	ExpiresAt     *time.Time
}

// Store persists artifact metadata.
//
// It knows nothing about task runs, issues, or conversations. Provenance
// reaches it as two opaque strings, which is what keeps an artifact from
// acquiring an owner it should not have.
type Store interface {
	CreateArtifact(ctx context.Context, in CreateInput) (*Artifact, error)
	// GetArtifact returns the artifact by its opaque ID, or (nil, nil) when there
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
	SoftDeleteArtifact(ctx context.Context, artifactID string, deletedAt time.Time) (bool, error)
	// TeamArtifactBytes sums what the team's live artifacts hold. Tombstoned
	// ones are excluded whether or not their objects have gone yet: the team
	// has given them up, and charging for storage the deployment has merely not
	// swept would make a quota depend on sweep timing.
	TeamArtifactBytes(ctx context.Context, teamID string) (int64, error)
}

// Expired is an artifact the retention sweep tombstoned, named so the trail can
// say which one went.
type Expired struct {
	ArtifactID string
	TeamID     string
}

// Purgeable is a tombstoned artifact whose bytes are still held.
type Purgeable struct {
	ArtifactID string
	TeamID     string
	SizeBytes  int64
}

// PurgeStore applies retention: it tombstones what has expired and reclaims the
// objects of what is already tombstoned.
//
// Narrow on purpose, in the same way audit's PruneStore is. Every other holder
// of a Store can tombstone but cannot reach an object removal, and nothing here
// names a particular artifact to destroy — a sweep takes a cutoff and a batch
// size, and marking one purged is only permitted to follow a removal the sweep
// just performed.
type PurgeStore interface {
	// ExpireArtifacts tombstones live artifacts whose ExpiresAt has passed,
	// at most limit of them, and reports which.
	ExpireArtifacts(ctx context.Context, now time.Time, limit int) ([]Expired, error)
	// PurgeableArtifacts returns artifacts tombstoned at or before the cutoff
	// whose objects have not been reclaimed, at most limit of them.
	PurgeableArtifacts(ctx context.Context, before time.Time, limit int) ([]Purgeable, error)
	// MarkArtifactPurged clears the storage key, recording that the object is
	// gone. It reports whether it changed anything, so two sweeps racing on one
	// artifact count it once.
	MarkArtifactPurged(ctx context.Context, artifactID string) (bool, error)
}
