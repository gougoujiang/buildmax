package db

import (
	"context"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/util"
)

// seedAuditEvents writes n events for one actor at the given timestamps and
// returns a cleanup-scoped team id.
//
// The timestamps are set after the insert because RecordAuditEvent stamps
// created_at itself — which is right for a governance record and inconvenient
// for a test that needs to control the ordering the cursor walks.
func seedAuditEvents(t *testing.T, s *Store, ctx context.Context, actor, teamID string, at []int64) {
	t.Helper()
	for range at {
		if err := s.RecordAuditEvent(ctx, model.AuditEvent{
			TeamID:    teamID,
			ActorType: model.AuditActorUser,
			ActorID:   actor,
			Action:    model.AuditUserLogin,
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
// created_at alone would drop the ties, and created_at has one-second
// resolution so ties are ordinary rather than exotic.
func TestExportTeamAuditEventsWalksEveryEventAcrossTies(t *testing.T) {
	s, ctx := newTestStore(t)
	actor := util.NewPrefixedID(util.PrefixUser)
	teamID := util.NewPrefixedID(util.PrefixTeam)
	t.Cleanup(func() { _ = s.db.WithContext(ctx).Delete(&auditEventRow{}, "actor_id = ?", actor).Error })

	seedAuditEvents(t, s, ctx, actor, teamID, []int64{500, 500, 500, 400})

	var seen []string
	var cursor model.AuditCursor
	for range 10 {
		page, err := s.ExportTeamAuditEvents(ctx, teamID, cursor, 2)
		if err != nil {
			t.Fatalf("ExportTeamAuditEvents: %v", err)
		}
		if len(page) == 0 {
			break
		}
		for _, e := range page {
			seen = append(seen, e.AuditEventID)
		}
		last := page[len(page)-1]
		cursor = model.AuditCursor{CreatedAt: last.CreatedAt, ID: last.ID}
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
	actor := util.NewPrefixedID(util.PrefixUser)
	teamID := util.NewPrefixedID(util.PrefixTeam)
	t.Cleanup(func() { _ = s.db.WithContext(ctx).Delete(&auditEventRow{}, "actor_id = ?", actor).Error })

	seedAuditEvents(t, s, ctx, actor, teamID, []int64{100, 200, 300})

	events, err := s.ExportAuditEvents(ctx, model.AuditFilter{ActorID: actor, Since: 200}, model.AuditCursor{}, 100)
	if err != nil {
		t.Fatalf("ExportAuditEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	for _, e := range events {
		if e.CreatedAt < 200 {
			t.Errorf("event at %d is outside the filter", e.CreatedAt)
		}
	}
}

// Retention is the one thing that removes a governance record, so it has to
// remove exactly what it was told to and no more: everything before the cutoff,
// nothing at or after it, and never more than one batch at a time.
func TestPruneAuditEventsRemovesOnlyWhatExpired(t *testing.T) {
	s, ctx := newTestStore(t)
	actor := util.NewPrefixedID(util.PrefixUser)
	teamID := util.NewPrefixedID(util.PrefixTeam)
	t.Cleanup(func() { _ = s.db.WithContext(ctx).Delete(&auditEventRow{}, "actor_id = ?", actor).Error })

	now := time.Now().Unix()
	seedAuditEvents(t, s, ctx, actor, teamID, []int64{now - 400, now - 300, now - 200, now - 50})

	oldest, err := s.OldestAuditEventAt(ctx)
	if err != nil {
		t.Fatalf("OldestAuditEventAt: %v", err)
	}
	if oldest > now-400 {
		t.Errorf("OldestAuditEventAt = %d, want at most %d", oldest, now-400)
	}

	// A bounded prune stops at the batch size rather than taking everything it
	// is allowed to, so one sweep cannot hold the table.
	removed, err := s.PruneAuditEvents(ctx, now-100, 2)
	if err != nil {
		t.Fatalf("PruneAuditEvents: %v", err)
	}
	if removed != 2 {
		t.Fatalf("first batch removed %d, want 2", removed)
	}
	removed, err = s.PruneAuditEvents(ctx, now-100, 100)
	if err != nil {
		t.Fatalf("PruneAuditEvents: %v", err)
	}
	if removed != 1 {
		t.Fatalf("second batch removed %d, want 1", removed)
	}

	remaining, err := s.ExportAuditEvents(ctx, model.AuditFilter{ActorID: actor}, model.AuditCursor{}, 100)
	if err != nil {
		t.Fatalf("ExportAuditEvents: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("%d events left, want 1", len(remaining))
	}
	if remaining[0].CreatedAt != now-50 {
		t.Errorf("kept the event at %d, want the one at %d", remaining[0].CreatedAt, now-50)
	}
}

// A cutoff of zero means "no retention window", and it must not be read as
// "everything is expired".
func TestPruneAuditEventsIgnoresAZeroCutoff(t *testing.T) {
	s, ctx := newTestStore(t)
	actor := util.NewPrefixedID(util.PrefixUser)
	teamID := util.NewPrefixedID(util.PrefixTeam)
	t.Cleanup(func() { _ = s.db.WithContext(ctx).Delete(&auditEventRow{}, "actor_id = ?", actor).Error })

	seedAuditEvents(t, s, ctx, actor, teamID, []int64{100})

	removed, err := s.PruneAuditEvents(ctx, 0, 100)
	if err != nil {
		t.Fatalf("PruneAuditEvents: %v", err)
	}
	if removed != 0 {
		t.Fatalf("removed %d events on a zero cutoff, want 0", removed)
	}
}
