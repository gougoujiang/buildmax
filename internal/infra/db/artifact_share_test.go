package db

import (
	"testing"
	"time"

	coreartifact "github.com/gougoujiang/buildmax/internal/core/artifact"
)

// These need a real database: a share's whole contract is the join to its
// artifact and the conditional revoke, neither of which a mock proves. They
// skip without BUILDMAX_TEST_DSN, like every store test here.

func newTestShare(t *testing.T, s *Store, artifactID, teamID, tokenHash string, expiresAt *time.Time) string {
	t.Helper()
	rec, err := s.CreateArtifactShare(t.Context(), coreartifact.CreateShareInput{
		ArtifactID:    artifactID,
		TeamID:        teamID,
		TokenSHA256:   tokenHash,
		CreatedByType: coreartifact.CreatorUser,
		CreatedByID:   "u_1",
		ExpiresAt:     expiresAt,
	})
	if err != nil {
		t.Fatalf("CreateArtifactShare: %v", err)
	}
	return rec.ShareID
}

func TestCreateAndResolveShareByTokenHash(t *testing.T) {
	s, ctx := newTestStore(t)
	teamID := newTestTeam(t, s, newTestUser(t, s, "share-resolve"))
	artifactID := newTestArtifact(t, s, teamID, 100, nil)
	shareID := newTestShare(t, s, artifactID, teamID, "hash-a", nil)

	resolved, err := s.GetArtifactShareByTokenHash(ctx, "hash-a")
	if err != nil || resolved == nil {
		t.Fatalf("resolve: %v, resolved=%v", err, resolved)
	}
	if resolved.Share.ShareID != shareID {
		t.Errorf("share id = %q, want %q", resolved.Share.ShareID, shareID)
	}
	if resolved.Share.ArtifactID != artifactID || resolved.Artifact.ID != artifactID {
		t.Errorf("resolved artifact = %q/%q, want %q", resolved.Share.ArtifactID, resolved.Artifact.ID, artifactID)
	}
	if resolved.Share.TeamID != teamID {
		t.Errorf("team = %q, want %q", resolved.Share.TeamID, teamID)
	}
}

func TestResolveUnknownTokenIsNil(t *testing.T) {
	s, ctx := newTestStore(t)
	resolved, err := s.GetArtifactShareByTokenHash(ctx, "nothing")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved != nil {
		t.Errorf("resolved = %+v, want nil for an unknown token", resolved)
	}
}

// A tombstoned artifact still resolves through the store (the store applies no
// liveness filter), so the service layer can turn it into the same 404 an
// unknown token gives. What matters here is the store returns it with the
// tombstone visible rather than hiding it or erroring.
func TestResolveShareReflectsATombstonedArtifact(t *testing.T) {
	s, ctx := newTestStore(t)
	teamID := newTestTeam(t, s, newTestUser(t, s, "share-tombstone"))
	artifactID := newTestArtifact(t, s, teamID, 100, nil)
	newTestShare(t, s, artifactID, teamID, "hash-b", nil)

	if _, err := s.SoftDeleteArtifact(ctx, artifactID, time.Now().UTC()); err != nil {
		t.Fatalf("SoftDeleteArtifact: %v", err)
	}
	resolved, err := s.GetArtifactShareByTokenHash(ctx, "hash-b")
	if err != nil || resolved == nil {
		t.Fatalf("resolve after delete: %v, resolved=%v", err, resolved)
	}
	if !resolved.Artifact.Deleted() {
		t.Error("resolved artifact is not marked tombstoned")
	}
}

func TestRevokeShareIsConditionalAndScoped(t *testing.T) {
	s, ctx := newTestStore(t)
	teamID := newTestTeam(t, s, newTestUser(t, s, "share-revoke"))
	artifactID := newTestArtifact(t, s, teamID, 100, nil)
	otherArtifact := newTestArtifact(t, s, teamID, 100, nil)
	shareID := newTestShare(t, s, artifactID, teamID, "hash-c", nil)

	// A share cannot be revoked through an artifact it does not belong to.
	if changed, err := s.RevokeArtifactShare(ctx, otherArtifact, shareID, time.Now().UTC()); err != nil || changed {
		t.Fatalf("cross-artifact revoke: changed=%v err=%v, want no change", changed, err)
	}
	// The first revoke changes it; the second is a no-op.
	if changed, err := s.RevokeArtifactShare(ctx, artifactID, shareID, time.Now().UTC()); err != nil || !changed {
		t.Fatalf("first revoke: changed=%v err=%v, want change", changed, err)
	}
	if changed, err := s.RevokeArtifactShare(ctx, artifactID, shareID, time.Now().UTC()); err != nil || changed {
		t.Fatalf("second revoke: changed=%v err=%v, want no change", changed, err)
	}
	resolved, err := s.GetArtifactShareByTokenHash(ctx, "hash-c")
	if err != nil || resolved == nil || resolved.Share.RevokedAt == nil {
		t.Fatalf("resolve after revoke: err=%v revoked=%v", err, resolved)
	}
}

func TestListAndRetrievalCount(t *testing.T) {
	s, ctx := newTestStore(t)
	teamID := newTestTeam(t, s, newTestUser(t, s, "share-list"))
	artifactID := newTestArtifact(t, s, teamID, 100, nil)
	shareID := newTestShare(t, s, artifactID, teamID, "hash-d", nil)
	newTestShare(t, s, artifactID, teamID, "hash-e", nil)

	shares, err := s.ListArtifactShares(ctx, artifactID)
	if err != nil || len(shares) != 2 {
		t.Fatalf("list: %v, n=%d, want 2", err, len(shares))
	}

	if err := s.RecordArtifactShareRetrieval(ctx, shareID, time.Now().UTC()); err != nil {
		t.Fatalf("record retrieval: %v", err)
	}
	resolved, err := s.GetArtifactShareByTokenHash(ctx, "hash-d")
	if err != nil || resolved == nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Share.RetrievalCount != 1 || resolved.Share.LastRetrievedAt == nil {
		t.Errorf("retrieval count = %d, last = %v, want 1 and a timestamp",
			resolved.Share.RetrievalCount, resolved.Share.LastRetrievedAt)
	}
}
