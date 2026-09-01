package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	coreartifact "github.com/gougoujiang/buildmax/internal/core/artifact"
	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
)

// fakeArtifactStore is the purge half of the store, holding just enough state
// to tell "tombstoned" from "reclaimed".
type fakeArtifactStore struct {
	items []coreartifact.Artifact
	// expireErr and purgeErr, when set, fail that phase.
	expireErr error
	purgeErr  error
	markErr   error
}

func (f *fakeArtifactStore) ExpireArtifacts(_ context.Context, now time.Time, limit int) ([]coreartifact.Expired, error) {
	if f.expireErr != nil {
		return nil, f.expireErr
	}
	var out []coreartifact.Expired
	for i := range f.items {
		it := &f.items[i]
		if len(out) >= limit || it.DeletedAt != nil || it.ExpiresAt == nil || it.ExpiresAt.After(now) {
			continue
		}
		at := now
		it.DeletedAt = &at
		out = append(out, coreartifact.Expired{ArtifactID: it.ID, TeamID: it.TeamID})
	}
	return out, nil
}

func (f *fakeArtifactStore) PurgeableArtifacts(_ context.Context, before time.Time, limit int) ([]coreartifact.Purgeable, error) {
	if f.purgeErr != nil {
		return nil, f.purgeErr
	}
	var out []coreartifact.Purgeable
	for _, it := range f.items {
		if len(out) >= limit || it.DeletedAt == nil || it.DeletedAt.After(before) || it.StorageKey == "" {
			continue
		}
		out = append(out, coreartifact.Purgeable{ArtifactID: it.ID, TeamID: it.TeamID, SizeBytes: it.SizeBytes})
	}
	return out, nil
}

func (f *fakeArtifactStore) MarkArtifactPurged(_ context.Context, artifactID string) (bool, error) {
	if f.markErr != nil {
		return false, f.markErr
	}
	for i := range f.items {
		if f.items[i].ID == artifactID && f.items[i].StorageKey != "" {
			f.items[i].StorageKey = ""
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeArtifactStore) find(id string) *coreartifact.Artifact {
	for i := range f.items {
		if f.items[i].ID == id {
			return &f.items[i]
		}
	}
	return nil
}

type fakeRemover struct {
	removed []string
	err     error
}

func (f *fakeRemover) RemoveArtifact(_ context.Context, ref coreartifact.Ref) error {
	if f.err != nil {
		return f.err
	}
	f.removed = append(f.removed, ref.ArtifactID)
	return nil
}

type recordingWriter struct {
	events []coreaudit.Event
	err    error
}

func (r *recordingWriter) RecordAuditEvent(_ context.Context, ev coreaudit.Event) error {
	if r.err != nil {
		return r.err
	}
	r.events = append(r.events, ev)
	return nil
}

func (r *recordingWriter) withAction(action string) []coreaudit.Event {
	var out []coreaudit.Event
	for _, ev := range r.events {
		if ev.Action == action {
			out = append(out, ev)
		}
	}
	return out
}

func ptr(t time.Time) *time.Time { return &t }

func retainerFor(store *fakeArtifactStore, remover *fakeRemover, writer *recordingWriter, graceDays int, now time.Time) *ArtifactRetainer {
	a := NewArtifactRetainer(store, remover, writer, graceDays, time.Hour)
	a.now = func() time.Time { return now }
	return a
}

func TestSweepReclaimsATombstonedArtifact(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeArtifactStore{items: []coreartifact.Artifact{
		{ID: "ar_gone", TeamID: "t_1", StorageKey: "k1", SizeBytes: 100, DeletedAt: ptr(now.Add(-time.Hour))},
		{ID: "ar_live", TeamID: "t_1", StorageKey: "k2", SizeBytes: 200},
	}}
	remover := &fakeRemover{}
	writer := &recordingWriter{}

	_, purged := retainerFor(store, remover, writer, 0, now).sweep(context.Background())

	if purged != 1 {
		t.Fatalf("purged = %d, want 1", purged)
	}
	if len(remover.removed) != 1 || remover.removed[0] != "ar_gone" {
		t.Fatalf("removed = %v, want [ar_gone]", remover.removed)
	}
	// The live artifact keeps its bytes. A sweep that touched it would be
	// deleting a file nobody asked to delete.
	if store.find("ar_live").StorageKey == "" {
		t.Fatal("the live artifact was purged")
	}
	if !store.find("ar_gone").Purged() {
		t.Fatal("the tombstoned artifact is not marked purged")
	}
	events := writer.withAction(coreaudit.ArtifactsPurged)
	if len(events) != 1 {
		t.Fatalf("purge events = %d, want 1", len(events))
	}
	// The bytes are in the detail because "how much did this reclaim" is the
	// question an operator brings to the trail.
	if !strings.Contains(events[0].Detail, "100 bytes") {
		t.Fatalf("detail = %q, want it to name 100 bytes", events[0].Detail)
	}
}

func TestSweepHoldsBytesUntilTheGracePeriodPasses(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeArtifactStore{items: []coreartifact.Artifact{
		{ID: "ar_recent", TeamID: "t_1", StorageKey: "k1", DeletedAt: ptr(now.Add(-24 * time.Hour))},
	}}
	remover := &fakeRemover{}

	// Deleted a day ago under a seven-day grace: still held.
	if _, purged := retainerFor(store, remover, &recordingWriter{}, 7, now).sweep(context.Background()); purged != 0 {
		t.Fatalf("purged = %d under grace, want 0", purged)
	}
	if len(remover.removed) != 0 {
		t.Fatalf("removed %v under grace, want none", remover.removed)
	}

	// Eight days later the same artifact is due.
	later := now.Add(8 * 24 * time.Hour)
	if _, purged := retainerFor(store, remover, &recordingWriter{}, 7, later).sweep(context.Background()); purged != 1 {
		t.Fatalf("purged = %d past grace, want 1", purged)
	}
}

func TestSweepTombstonesWhatExpiredAndNamesIt(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeArtifactStore{items: []coreartifact.Artifact{
		{ID: "ar_old", TeamID: "t_1", StorageKey: "k1", ExpiresAt: ptr(now.Add(-time.Minute))},
		{ID: "ar_later", TeamID: "t_1", StorageKey: "k2", ExpiresAt: ptr(now.Add(time.Hour))},
		{ID: "ar_never", TeamID: "t_1", StorageKey: "k3"},
	}}
	writer := &recordingWriter{}

	expired, _ := retainerFor(store, &fakeRemover{}, writer, 0, now).sweep(context.Background())

	if expired != 1 {
		t.Fatalf("expired = %d, want 1", expired)
	}
	if !store.find("ar_old").Deleted() {
		t.Fatal("the expired artifact was not tombstoned")
	}
	if store.find("ar_later").Deleted() || store.find("ar_never").Deleted() {
		t.Fatal("an unexpired artifact was tombstoned")
	}
	// Per artifact, not a summary: this is the one tombstone no member asked
	// for, so a reader looking for why it went has to find it named.
	events := writer.withAction(coreaudit.ArtifactExpired)
	if len(events) != 1 || events[0].TargetID != "ar_old" {
		t.Fatalf("expiry events = %+v, want one naming ar_old", events)
	}
	if events[0].TeamID != "t_1" {
		t.Fatalf("event team = %q, want t_1", events[0].TeamID)
	}
}

// The two phases compose: expiry tombstones, and the purge then applies the
// same grace period it applies to a member's delete. Under a zero grace that
// means one pass does both, which is what a zero grace asks for — an expiry is
// not a softer deletion than a delete, and giving it its own extra hour would
// be a rule nobody configured.
func TestExpiryIsReclaimedUnderTheSameGraceAsADelete(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	expiredAt := ptr(now.Add(-time.Minute))

	immediate := &fakeArtifactStore{items: []coreartifact.Artifact{
		{ID: "ar_old", TeamID: "t_1", StorageKey: "k1", ExpiresAt: expiredAt},
	}}
	expired, purged := retainerFor(immediate, &fakeRemover{}, &recordingWriter{}, 0, now).sweep(context.Background())
	if expired != 1 || purged != 1 {
		t.Fatalf("zero grace: expired=%d purged=%d, want 1 and 1", expired, purged)
	}

	// With a grace the tombstone lands now and the bytes wait for it, exactly
	// as they would for an artifact somebody deleted by hand.
	held := &fakeArtifactStore{items: []coreartifact.Artifact{
		{ID: "ar_old", TeamID: "t_1", StorageKey: "k1", ExpiresAt: expiredAt},
	}}
	remover := &fakeRemover{}
	expired, purged = retainerFor(held, remover, &recordingWriter{}, 7, now).sweep(context.Background())
	if expired != 1 || purged != 0 {
		t.Fatalf("seven-day grace: expired=%d purged=%d, want 1 and 0", expired, purged)
	}
	if !held.find("ar_old").Deleted() || held.find("ar_old").Purged() {
		t.Fatal("the expired artifact should be tombstoned but still hold its bytes")
	}

	later := retainerFor(held, remover, &recordingWriter{}, 7, now.Add(8*24*time.Hour))
	if _, purged := later.sweep(context.Background()); purged != 1 {
		t.Fatalf("purged = %d once the grace passed, want 1", purged)
	}
}

// The row must never claim the bytes are gone when they are not: a failed
// removal leaves the key so the next sweep tries again.
func TestAFailedRemovalLeavesTheArtifactPurgeable(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeArtifactStore{items: []coreartifact.Artifact{
		{ID: "ar_gone", TeamID: "t_1", StorageKey: "k1", DeletedAt: ptr(now.Add(-time.Hour))},
	}}
	writer := &recordingWriter{}

	_, purged := retainerFor(store, &fakeRemover{err: errors.New("bucket unreachable")}, writer, 0, now).sweep(context.Background())

	if purged != 0 {
		t.Fatalf("purged = %d after a failed removal, want 0", purged)
	}
	if store.find("ar_gone").Purged() {
		t.Fatal("the artifact was marked purged although removal failed")
	}
	if len(writer.withAction(coreaudit.ArtifactsPurged)) != 0 {
		t.Fatal("a purge was recorded although nothing was reclaimed")
	}
}

// A sweep that reclaims nothing writes nothing. Otherwise an hourly loop fills
// the trail with rows saying it did not act.
func TestAnIdleSweepRecordsNothing(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeArtifactStore{items: []coreartifact.Artifact{
		{ID: "ar_live", TeamID: "t_1", StorageKey: "k1"},
	}}
	writer := &recordingWriter{}

	expired, purged := retainerFor(store, &fakeRemover{}, writer, 0, now).sweep(context.Background())

	if expired != 0 || purged != 0 {
		t.Fatalf("expired=%d purged=%d, want 0 and 0", expired, purged)
	}
	if len(writer.events) != 0 {
		t.Fatalf("recorded %d events on an idle sweep, want none", len(writer.events))
	}
}

// Retention must survive an unreachable trail: the bytes are already gone, and
// failing the sweep would mean never reclaiming anything while audit is down.
func TestReclaimingSurvivesAnUnwritableTrail(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeArtifactStore{items: []coreartifact.Artifact{
		{ID: "ar_gone", TeamID: "t_1", StorageKey: "k1", DeletedAt: ptr(now.Add(-time.Hour))},
	}}
	remover := &fakeRemover{}

	_, purged := retainerFor(store, remover, &recordingWriter{err: errors.New("audit down")}, 0, now).sweep(context.Background())

	if purged != 1 {
		t.Fatalf("purged = %d with the trail down, want 1", purged)
	}
	if !store.find("ar_gone").Purged() {
		t.Fatal("the artifact was not reclaimed when the trail was unwritable")
	}
}

// A deployment with no artifact storage gets no loop rather than one that
// cannot do anything, so the call sites need no branch.
func TestNoRetainerWithoutBothHalves(t *testing.T) {
	if a := NewArtifactRetainer(nil, &fakeRemover{}, nil, 0, 0); a != nil {
		t.Fatal("a retainer was built with no store")
	}
	if a := NewArtifactRetainer(&fakeArtifactStore{}, nil, nil, 0, 0); a != nil {
		t.Fatal("a retainer was built with no content store")
	}
	// Nil is safe to drive, which is what lets bootstrap start and stop it
	// unconditionally.
	var nilRetainer *ArtifactRetainer
	nilRetainer.Start()
	nilRetainer.Stop()
}

// Unlike the audit retainer there is no window that switches this off: bytes
// nobody reclaims are a leak, not a retained record.
func TestAZeroGraceStillBuildsARetainer(t *testing.T) {
	if a := NewArtifactRetainer(&fakeArtifactStore{}, &fakeRemover{}, nil, 0, 0); a == nil {
		t.Fatal("a zero grace period disabled the sweep")
	}
}

func TestStartAndStopRunASweep(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeArtifactStore{items: []coreartifact.Artifact{
		{ID: "ar_gone", TeamID: "t_1", StorageKey: "k1", DeletedAt: ptr(now.Add(-time.Hour))},
	}}
	remover := &fakeRemover{}
	a := retainerFor(store, remover, &recordingWriter{}, 0, now)

	// The loop sweeps once before waiting on the ticker, so a server restarted
	// more often than the interval still applies the policy.
	a.Start()
	a.Stop()

	if len(remover.removed) != 1 {
		t.Fatalf("removed = %v after start/stop, want one artifact", remover.removed)
	}
}
