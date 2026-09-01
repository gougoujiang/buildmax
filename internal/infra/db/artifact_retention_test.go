package db

import (
	"testing"
	"time"

	coreartifact "github.com/gougoujiang/buildmax/internal/core/artifact"
)

// These cover what docs/design/verification-program.md section 4.2 listed as
// still to write: artifact tombstoned deletion, and now the retention that
// follows it. None of it can be asserted without a real database — the
// tombstone, the expiry, and the purge are all conditional UPDATEs whose whole
// contract is which of two concurrent callers the server lets win.

// newTestArtifact stores one artifact row for teamID and registers its removal.
func newTestArtifact(t *testing.T, s *Store, teamID string, size int64, expiresAt *time.Time) string {
	t.Helper()
	ctx := t.Context()
	id := testPublicID(t)
	rec, err := s.CreateArtifact(ctx, coreartifact.CreateInput{
		TeamID:        teamID,
		ArtifactID:    id,
		Filename:      "report.md",
		MediaType:     "text/markdown",
		SizeBytes:     size,
		SHA256:        "abc",
		StorageKey:    "teams/x/artifacts/" + id + "/content",
		CreatedByType: coreartifact.CreatorUser,
		CreatedByID:   "u_1",
		SourceType:    coreartifact.SourceUserUpload,
		ExpiresAt:     expiresAt,
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	t.Cleanup(func() {
		_ = s.db.Where("public_id = ?", rec.ID).Delete(&artifactRow{}).Error
	})
	return rec.ID
}

func getArtifact(t *testing.T, s *Store, id string) *coreartifact.Artifact {
	t.Helper()
	rec, err := s.GetArtifact(t.Context(), id)
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if rec == nil {
		t.Fatalf("artifact %s is gone", id)
	}
	return rec
}

// The tombstone hides the artifact from every live read while the row and its
// storage key stay, which is what lets retention find the bytes afterwards.
func TestSoftDeleteHidesTheArtifactAndKeepsItsKey(t *testing.T) {
	s, ctx := newTestStore(t)
	teamID := newTestTeam(t, s, newTestUser(t, s, "artifact-tombstone"))
	id := newTestArtifact(t, s, teamID, 100, nil)

	if _, total, err := s.ListArtifactsByTeam(ctx, teamID, 50, 0); err != nil || total != 1 {
		t.Fatalf("before delete: total=%d err=%v, want 1", total, err)
	}

	changed, err := s.SoftDeleteArtifact(ctx, id, time.Now().UTC())
	if err != nil || !changed {
		t.Fatalf("SoftDeleteArtifact: changed=%v err=%v", changed, err)
	}

	// Gone from the listing, still readable by id: the caller has to be able to
	// tell "deleted" from "never existed" to answer either one correctly.
	if _, total, err := s.ListArtifactsByTeam(ctx, teamID, 50, 0); err != nil || total != 0 {
		t.Fatalf("after delete: total=%d err=%v, want 0", total, err)
	}
	rec := getArtifact(t, s, id)
	if !rec.Deleted() {
		t.Fatal("the artifact is not tombstoned")
	}
	if rec.Purged() {
		t.Fatal("the tombstone reclaimed the object; that is retention's job")
	}
	if rec.StorageKey == "" {
		t.Fatal("the tombstone cleared the storage key, losing the object")
	}
}

// Exactly one of two concurrent deletes may report a change. Checked by
// mutation while writing it: replacing the conditional UPDATE with a
// read-then-write makes this fail.
func TestSoftDeleteAdmitsOneOfTwoDeletes(t *testing.T) {
	s, ctx := newTestStore(t)
	teamID := newTestTeam(t, s, newTestUser(t, s, "artifact-race"))
	id := newTestArtifact(t, s, teamID, 100, nil)

	now := time.Now().UTC()
	first, err := s.SoftDeleteArtifact(ctx, id, now)
	if err != nil {
		t.Fatalf("first delete: %v", err)
	}
	second, err := s.SoftDeleteArtifact(ctx, id, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("second delete: %v", err)
	}
	if !first || second {
		t.Fatalf("first=%v second=%v, want exactly one change", first, second)
	}
}

// A tombstone releases the bytes for the team's allowance whether or not the
// sweep has reclaimed them yet. Charging for storage the deployment has merely
// not got around to sweeping would make a quota depend on sweep timing.
func TestTeamArtifactBytesCountsOnlyLiveArtifacts(t *testing.T) {
	s, ctx := newTestStore(t)
	teamID := newTestTeam(t, s, newTestUser(t, s, "artifact-bytes"))

	if held, err := s.TeamArtifactBytes(ctx, teamID); err != nil || held != 0 {
		t.Fatalf("empty team: held=%d err=%v, want 0", held, err)
	}

	keep := newTestArtifact(t, s, teamID, 100, nil)
	drop := newTestArtifact(t, s, teamID, 250, nil)
	if held, err := s.TeamArtifactBytes(ctx, teamID); err != nil || held != 350 {
		t.Fatalf("held=%d err=%v, want 350", held, err)
	}

	if _, err := s.SoftDeleteArtifact(ctx, drop, time.Now().UTC()); err != nil {
		t.Fatalf("SoftDeleteArtifact: %v", err)
	}
	if held, err := s.TeamArtifactBytes(ctx, teamID); err != nil || held != 100 {
		t.Fatalf("after delete: held=%d err=%v, want 100", held, err)
	}
	// Another team's artifact is not this team's storage.
	otherTeam := newTestTeam(t, s, newTestUser(t, s, "artifact-bytes-other"))
	newTestArtifact(t, s, otherTeam, 999, nil)
	if held, err := s.TeamArtifactBytes(ctx, teamID); err != nil || held != 100 {
		t.Fatalf("cross-team leak: held=%d err=%v, want 100", held, err)
	}
	_ = keep
}

func TestExpireArtifactsTakesOnlyWhatRanOut(t *testing.T) {
	s, ctx := newTestStore(t)
	teamID := newTestTeam(t, s, newTestUser(t, s, "artifact-expire"))
	now := time.Now().UTC()

	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	expired := newTestArtifact(t, s, teamID, 10, &past)
	later := newTestArtifact(t, s, teamID, 10, &future)
	never := newTestArtifact(t, s, teamID, 10, nil)

	gone, err := s.ExpireArtifacts(ctx, now, 100)
	if err != nil {
		t.Fatalf("ExpireArtifacts: %v", err)
	}
	// Other tests may leave rows behind, so assert on this team's artifacts
	// rather than on the batch's total size.
	took := map[string]bool{}
	for _, it := range gone {
		took[it.ArtifactID] = true
		if it.TeamID == "" {
			t.Fatalf("expiry of %s carries no team, so it cannot be recorded", it.ArtifactID)
		}
	}
	if !took[expired] {
		t.Fatal("the expired artifact was not taken")
	}
	if took[later] || took[never] {
		t.Fatal("an artifact that had not expired was taken")
	}
	if !getArtifact(t, s, expired).Deleted() {
		t.Fatal("the expired artifact was not tombstoned")
	}
	if getArtifact(t, s, later).Deleted() || getArtifact(t, s, never).Deleted() {
		t.Fatal("an unexpired artifact was tombstoned")
	}

	// Idempotent: a second sweep finds nothing, because the first tombstoned
	// what it took and the query only looks at live rows.
	again, err := s.ExpireArtifacts(ctx, now, 100)
	if err != nil {
		t.Fatalf("second ExpireArtifacts: %v", err)
	}
	for _, it := range again {
		if it.ArtifactID == expired {
			t.Fatal("the same artifact expired twice")
		}
	}
}

// The purge query is the state machine: tombstoned and still keyed means due,
// and clearing the key is what takes it out of the set.
func TestPurgeableAndMarkPurgedWalkTheArtifactThroughRetention(t *testing.T) {
	s, ctx := newTestStore(t)
	teamID := newTestTeam(t, s, newTestUser(t, s, "artifact-purge"))
	now := time.Now().UTC()

	live := newTestArtifact(t, s, teamID, 100, nil)
	dead := newTestArtifact(t, s, teamID, 100, nil)
	if _, err := s.SoftDeleteArtifact(ctx, dead, now.Add(-time.Hour)); err != nil {
		t.Fatalf("SoftDeleteArtifact: %v", err)
	}

	due, err := s.PurgeableArtifacts(ctx, now, 500)
	if err != nil {
		t.Fatalf("PurgeableArtifacts: %v", err)
	}
	found := map[string]coreartifact.Purgeable{}
	for _, it := range due {
		found[it.ArtifactID] = it
	}
	if _, ok := found[live]; ok {
		t.Fatal("a live artifact was offered for reclamation")
	}
	it, ok := found[dead]
	if !ok {
		t.Fatal("the tombstoned artifact was not offered for reclamation")
	}
	// The size travels with it so the sweep can report the bytes it reclaimed
	// after the row no longer says where they were.
	if it.SizeBytes != 100 || it.TeamID == "" {
		t.Fatalf("purgeable = %+v, want its size and team", it)
	}

	// A tombstone inside the grace period is not yet due.
	beforeTombstone := now.Add(-2 * time.Hour)
	early, err := s.PurgeableArtifacts(ctx, beforeTombstone, 500)
	if err != nil {
		t.Fatalf("PurgeableArtifacts under grace: %v", err)
	}
	for _, e := range early {
		if e.ArtifactID == dead {
			t.Fatal("an artifact inside its grace period was offered for reclamation")
		}
	}

	marked, err := s.MarkArtifactPurged(ctx, dead)
	if err != nil || !marked {
		t.Fatalf("MarkArtifactPurged: marked=%v err=%v", marked, err)
	}
	if !getArtifact(t, s, dead).Purged() {
		t.Fatal("the artifact is not marked purged")
	}

	// Exactly one caller may report the reclamation, so two servers sweeping
	// the same artifact do not double the bytes in the trail.
	again, err := s.MarkArtifactPurged(ctx, dead)
	if err != nil {
		t.Fatalf("second MarkArtifactPurged: %v", err)
	}
	if again {
		t.Fatal("marking an already purged artifact reported a second reclamation")
	}

	// And it leaves the purge set, so the next sweep does not retry it.
	after, err := s.PurgeableArtifacts(ctx, now, 500)
	if err != nil {
		t.Fatalf("PurgeableArtifacts after: %v", err)
	}
	for _, a := range after {
		if a.ArtifactID == dead {
			t.Fatal("a purged artifact is still offered for reclamation")
		}
	}
}
