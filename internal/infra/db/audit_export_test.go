package db

import (
	"context"
	"testing"
	"time"

	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
)

// seedAuditEvents writes n events for one actor at the given timestamps and
// returns a cleanup-scoped team id.
//
// The timestamps are set after the insert because RecordAuditEvent stamps
// created_at itself — which is right for a governance record and inconvenient
// for a test that needs to control the ordering the cursor walks.
// at builds a distinct, ordered instant from a small seed number, so a test can
// keep writing 100/200/300 and still be talking about times.
func at(seconds int) time.Time { return time.Unix(1_700_000_000+int64(seconds), 0).UTC() }

func seedAuditEvents(t *testing.T, s *Store, ctx context.Context, actor, teamID string, at []time.Time) {
	t.Helper()
	for range at {
		if err := s.RecordAuditEvent(ctx, coreaudit.Event{
			TeamID:    teamID,
			ActorType: coreaudit.ActorUser,
			ActorID:   actor,
			Action:    coreaudit.UserLogin,
		}); err != nil {
			t.Fatalf("RecordAuditEvent: %v", err)
		}
	}
	var rows []auditEventRow
	if err := s.db.WithContext(ctx).Where("actor_id = ?", actor).Order("id ASC").Find(&rows).Error; err != nil {
		t.Fatalf("read seeded rows: %v", err)
	}
	if len(rows) != len(at) {
		t.Fatalf("seeded %d rows, want %d", len(rows), len(at))
	}
	for i, row := range rows {
		if err := s.db.WithContext(ctx).Model(&auditEventRow{}).
			Where("id = ?", row.ID).
			Update("created_at", at[i]).Error; err != nil {
			t.Fatalf("stamp created_at: %v", err)
		}
	}
}

// A keyset cursor exists because offset paging is wrong here: the table is
// appended to while an export reads it, and under retention it is deleted from
// at the other end. Either shifts every offset behind it, and a page boundary
// then skips a record — which, in an evidence export, is the worst kind of bug
// because the file looks complete.
//
// Three of the four events deliberately share a timestamp: a cursor on
// created_at alone would drop the ties. The column is a DATETIME(6), so a tie
// is not the certainty an earlier version of this comment claimed -- but a
// bulk import, a replayed batch, or any writer stamping one time across
// several rows produces one, and an export that skipped a record then would
// look complete.
func TestExportTeamAuditEventsWalksEveryEventAcrossTies(t *testing.T) {
	s, ctx := newTestStore(t)
	actor := newTestUser(t, s, "audit")
	teamID := newTestTeam(t, s, actor)
	t.Cleanup(func() { _ = s.db.WithContext(ctx).Delete(&auditEventRow{}, "actor_id = ?", actor).Error })

	seedAuditEvents(t, s, ctx, actor, teamID, []time.Time{at(500), at(500), at(500), at(400)})

	var seen []string
	var cursor coreaudit.Cursor
	for range 10 {
		page, err := s.ExportTeamAuditEvents(ctx, teamID, cursor, 2)
		if err != nil {
			t.Fatalf("ExportTeamAuditEvents: %v", err)
		}
		if len(page) == 0 {
			break
		}
		for _, e := range page {
			seen = append(seen, e.ID)
		}
		last := page[len(page)-1]
		cursor = coreaudit.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}

	if len(seen) != 4 {
		t.Fatalf("walked %d events, want 4 (%v)", len(seen), seen)
	}
	unique := map[string]bool{}
	for _, id := range seen {
		if unique[id] {
			t.Fatalf("event %s returned twice: %v", id, seen)
		}
		unique[id] = true
	}
}

// The deployment-scoped export answers under the same filters as the search it
// belongs to. If the two ever diverge, an operator's export stops matching the
// screen they took it from.
func TestExportAuditEventsHonoursTheFilter(t *testing.T) {
	s, ctx := newTestStore(t)
	actor := newTestUser(t, s, "audit")
	teamID := newTestTeam(t, s, actor)
	t.Cleanup(func() { _ = s.db.WithContext(ctx).Delete(&auditEventRow{}, "actor_id = ?", actor).Error })

	seedAuditEvents(t, s, ctx, actor, teamID, []time.Time{at(100), at(200), at(300)})

	events, err := s.ExportAuditEvents(ctx, coreaudit.Filter{ActorID: actor, Since: at(200)}, coreaudit.Cursor{}, 100)
	if err != nil {
		t.Fatalf("ExportAuditEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	for _, e := range events {
		if e.CreatedAt.Before(at(200)) {
			t.Errorf("event at %v is outside the filter", e.CreatedAt)
		}
	}
}

// Retention is the one thing that removes a governance record, so it has to
// remove exactly what it was told to and no more: everything before the cutoff,
// nothing at or after it, and never more than one batch at a time.
func TestPruneAuditEventsRemovesOnlyWhatExpired(t *testing.T) {
	s, ctx := newTestStore(t)
	actor := newTestUser(t, s, "audit")
	teamID := newTestTeam(t, s, actor)
	t.Cleanup(func() { _ = s.db.WithContext(ctx).Delete(&auditEventRow{}, "actor_id = ?", actor).Error })

	// Truncated to what a DATETIME(6) column can hold. Every other test here
	// stamps whole seconds through at(), so only this one, which needs times
	// relative to the present, can carry a remainder the column drops -- and
	// then reading a row back would not Equal the value that was written.
	// Linux keeps that remainder and macOS usually does not, so a developer's
	// machine is the wrong place to find out.
	now := time.Now().UTC().Truncate(time.Microsecond)
	// Retention is deployment-wide, so the counts below are over the whole
	// table and cannot tolerate a row this test did not write -- a sign-in from
	// a stack sharing this database, or an earlier run that was interrupted
	// before its cleanup. Everything the prune would reach is removed first,
	// which is what makes an absolute count mean anything here.
	if err := s.db.WithContext(ctx).Where("created_at < ?", now.Add(-100*time.Second)).Delete(&auditEventRow{}).Error; err != nil {
		t.Fatalf("clear events older than the cutoff: %v", err)
	}
	seedAuditEvents(t, s, ctx, actor, teamID, []time.Time{now.Add(-400 * time.Second), now.Add(-300 * time.Second), now.Add(-200 * time.Second), now.Add(-50 * time.Second)})

	oldest, err := s.OldestAuditEventAt(ctx)
	if err != nil {
		t.Fatalf("OldestAuditEventAt: %v", err)
	}
	if oldest.After(now.Add(-400 * time.Second)) {
		t.Errorf("OldestAuditEventAt = %v, want at most %v", oldest, now.Add(-400*time.Second))
	}

	// A bounded prune stops at the batch size rather than taking everything it
	// is allowed to, so one sweep cannot hold the table.
	removed, err := s.PruneAuditEvents(ctx, now.Add(-100*time.Second), 2)
	if err != nil {
		t.Fatalf("PruneAuditEvents: %v", err)
	}
	if removed != 2 {
		t.Fatalf("first batch removed %d, want 2", removed)
	}
	removed, err = s.PruneAuditEvents(ctx, now.Add(-100*time.Second), 100)
	if err != nil {
		t.Fatalf("PruneAuditEvents: %v", err)
	}
	if removed != 1 {
		t.Fatalf("second batch removed %d, want 1", removed)
	}

	remaining, err := s.ExportAuditEvents(ctx, coreaudit.Filter{ActorID: actor}, coreaudit.Cursor{}, 100)
	if err != nil {
		t.Fatalf("ExportAuditEvents: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("%d events left, want 1", len(remaining))
	}
	if !remaining[0].CreatedAt.Equal(now.Add(-50 * time.Second)) {
		t.Errorf("kept the event at %v, want the one at %v", remaining[0].CreatedAt, now.Add(-50*time.Second))
	}
}

// A cutoff of zero means "no retention window", and it must not be read as
// "everything is expired".
func TestPruneAuditEventsIgnoresAZeroCutoff(t *testing.T) {
	s, ctx := newTestStore(t)
	actor := newTestUser(t, s, "audit")
	teamID := newTestTeam(t, s, actor)
	t.Cleanup(func() { _ = s.db.WithContext(ctx).Delete(&auditEventRow{}, "actor_id = ?", actor).Error })

	seedAuditEvents(t, s, ctx, actor, teamID, []time.Time{at(100)})

	removed, err := s.PruneAuditEvents(ctx, time.Time{}, 100)
	if err != nil {
		t.Fatalf("PruneAuditEvents: %v", err)
	}
	if removed != 0 {
		t.Fatalf("removed %d events on a zero cutoff, want 0", removed)
	}
}
