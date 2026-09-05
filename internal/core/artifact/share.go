package artifact

import (
	"context"
	"time"
)

// ArtifactShare is a revocable public link to one artifact.
//
// It is a separate record, not a flag on the artifact: an artifact may carry
// more than one live link with different lifetimes, each revocable on its own,
// and the artifact row stays immutable. The link's secret is a high-entropy
// token held only as its SHA-256 — the plaintext is returned once at creation
// and never stored, the same pattern login codes and webhook keys follow. See
// docs/design/artifact-public-sharing-and-preview.md §5.
type ArtifactShare struct {
	ShareID       string `json:"share_id"`
	ArtifactID    string `json:"artifact_id"`
	TeamID        string `json:"team_id"`
	CreatedByType string `json:"created_by_type"`
	CreatedByID   string `json:"created_by_id,omitempty"`
	// ExpiresAt bounds the link; nil is a link with no expiry (still revocable).
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// RevokedAt withdraws the link. A revoked share resolves to the same 404 a
	// never-existed token gives.
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	RetrievalCount  int64      `json:"retrieval_count"`
	LastRetrievedAt *time.Time `json:"last_retrieved_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// Live reports whether the share still grants access at now: not revoked, and
// not past its expiry. It says nothing about the artifact behind it — a caller
// resolving a link must also check the artifact is not tombstoned.
func (s *ArtifactShare) Live(now time.Time) bool {
	if s == nil || s.RevokedAt != nil {
		return false
	}
	if s.ExpiresAt != nil && !s.ExpiresAt.After(now) {
		return false
	}
	return true
}

// CreateShareInput is everything the store needs to record one link. The
// service has already generated the token and hashed it.
type CreateShareInput struct {
	ArtifactID    string
	TeamID        string
	TokenSHA256   string
	CreatedByType string
	CreatedByID   string
	ExpiresAt     *time.Time
}

// ResolvedShare pairs a share with the artifact it points at, as one lookup
// returns them. Both are needed together: the share to check liveness and count
// a retrieval, the artifact to authorize and stream.
type ResolvedShare struct {
	Share    ArtifactShare
	Artifact Artifact
}

// ShareStore persists public share links.
//
// Separate from Store because sharing is an additive capability layered over
// artifacts, not part of the artifact object itself. A deployment with the
// artifact capability but no share store simply cannot create public links.
type ShareStore interface {
	CreateArtifactShare(ctx context.Context, in CreateShareInput) (*ArtifactShare, error)
	// GetArtifactShareByTokenHash returns the share and its artifact by the
	// token's hash, or (nil, nil) when the token matches nothing. It does NOT
	// filter revoked, expired, or tombstoned: the caller applies liveness so a
	// public route answers every non-live case with the same 404, and a
	// management view can still show a revoked link. A tombstoned artifact is
	// returned like GetArtifact returns one.
	GetArtifactShareByTokenHash(ctx context.Context, tokenHash string) (*ResolvedShare, error)
	// ListArtifactShares returns an artifact's shares, newest first, including
	// revoked and expired ones for the management view.
	ListArtifactShares(ctx context.Context, artifactID string) ([]ArtifactShare, error)
	// RevokeArtifactShare marks a share revoked if it belongs to the artifact
	// and is not already revoked, reporting whether it changed anything so a
	// repeat revoke is distinguishable from a first.
	RevokeArtifactShare(ctx context.Context, artifactID, shareID string, revokedAt time.Time) (bool, error)
	// RecordArtifactShareRetrieval increments the count and stamps the time.
	// Best-effort telemetry: a failure must never block content delivery.
	RecordArtifactShareRetrieval(ctx context.Context, shareID string, at time.Time) error
}
