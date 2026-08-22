package objectstore

import (
	"context"
	"io"
	"path"
)

// ArtifactRef names one artifact's content object.
//
// Team is here because it partitions the key space, not because callers
// address an artifact by team: an artifact is reached by its ar_ ID, and the
// service that holds the record supplies the team it belongs to.
type ArtifactRef struct {
	TeamID     string
	ArtifactID string
}

// ArtifactStorage stores one immutable content object per artifact.
//
// Write-once by construction: there is no update, because a changed file is a
// new artifact. The key an implementation derives is private to this package —
// nothing above it may build, parse, or persist one, which is what leaves the
// bucket layout free to change. See docs/design/unified-artifacts.md section 9.
type ArtifactStorage interface {
	// PutArtifact streams content in under a reserved, unused artifact ID and
	// reports the key it wrote. The key is returned so the record can keep what
	// was actually used rather than a recomputation that a later layout change
	// would silently invalidate; it is for operators reading the database, and
	// never leaves the server.
	PutArtifact(ctx context.Context, ref ArtifactRef, r io.Reader) (string, error)
	// OpenArtifact returns the content stream, which the caller closes.
	// ErrNotFound when the object is absent.
	OpenArtifact(ctx context.Context, ref ArtifactRef) (io.ReadCloser, error)
	// RemoveArtifact deletes the object. Content that is already gone is not
	// an error: removal is what the caller asked for and it is already true,
	// and the crash-recovery path must be able to run twice.
	RemoveArtifact(ctx context.Context, ref ArtifactRef) error
}

// ArtifactObjectKey returns the object key holding one artifact's content.
//
// The shape mirrors the ownership question an operator asks of a bucket — whose
// data is this — and is disjoint from both the persist key space
// (<prefix>/<team_id>/home/) and the run-output key space
// (<prefix>/<user_id>/artifacts/), which are keyed differently and predate this.
func ArtifactObjectKey(prefix string, ref ArtifactRef) string {
	return path.Join(prefix, "teams", ref.TeamID, "artifacts", ref.ArtifactID, "content")
}
