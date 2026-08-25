package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
)

type fakeAuditPruner struct {
	events []coreaudit.Event
	// pruneErr, when set, is returned by PruneAuditEvents.
	pruneErr error
	// oldestErr, when set, is returned by OldestAuditEventAt.
	oldestErr error
	calls     int
}

func (f *fakeAuditPruner) PruneAuditEvents(_ context.Context, before time.Time, limit int) (int64, error) {
	f.calls++
	if f.pruneErr != nil {
		return 0, f.pruneErr
	}
	kept := make([]coreaudit.Event, 0, len(f.events))
	var removed int64
	for _, e := range f.events {
		if e.CreatedAt.Before(before) && removed < int64(limit) {
			removed++
			continue
		}
		kept = append(kept, e)
	}
	f.events = kept
	return removed, nil
}

func (f *fakeAuditPruner) OldestAuditEventAt(context.Context) (time.Time, error) {
	if f.oldestErr != nil {
		return time.Time{}, f.oldestErr
	}
	var oldest time.Time
	for _, e := range f.events {
		if oldest.IsZero() || e.CreatedAt.Before(oldest) {
			oldest = e.CreatedAt
		}
	}
	return oldest, nil
}

func (f *fakeAuditPruner) RecordAuditEvent(_ context.Context, in coreaudit.Event) error {
	f.events = append(f.events, in)
	return nil
}

func at(now time.Time, daysAgo int) time.Time {
	return now.AddDate(0, 0, -daysAgo)
}

func retainerAt(store *fakeAuditPruner, days int, now time.Time) *AuditRetainer {
	r := NewAuditRetainer(store, store, days, time.Hour)
	if r != nil {
		r.now = func() time.Time { return now }
	}
	return r
}

// The window decides what goes, and everything inside it stays. A retention
// sweep that took one event too many would be removing evidence nobody asked
// it to.
func TestAuditRetainerRemovesOnlyExpiredEvents(t *testing.T) {
	now := time.Now()
	store := &fakeAuditPruner{events: []coreaudit.Event{
		{Action: coreaudit.UserLogin, CreatedAt: at(now, 100)},
		{Action: coreaudit.UserLogin, CreatedAt: at(now, 91)},
		{Action: coreaudit.UserLogin, CreatedAt: at(now, 89)},
		{Action: coreaudit.UserLogin, CreatedAt: at(now, 1)},
	}}

	removed := retainerAt(store, 90, now).sweep(context.Background())
	if removed != 2 {
		t.Fatalf("removed %d events, want 2", removed)
	}
	for _, e := range store.events {
		if e.Action == coreaudit.UserLogin && e.CreatedAt.Before(at(now, 90)) {
			t.Errorf("event at %v survived the 90-day window", e.CreatedAt)
		}
	}
}

// The reason retention is allowed to delete at all: it says so. Without this
// record, a trail that starts partway through is indistinguishable from one
// somebody truncated, and only one of those is something a deployment chose.
func TestAuditRetainerRecordsWhatItRemoved(t *testing.T) {
	now := time.Now()
	store := &fakeAuditPruner{events: []coreaudit.Event{
		{Action: coreaudit.UserLogin, CreatedAt: at(now, 100)},
		{Action: coreaudit.UserLogin, CreatedAt: at(now, 95)},
	}}

	retainerAt(store, 90, now).sweep(context.Background())

	var pruned *coreaudit.Event
	for i := range store.events {
		if store.events[i].Action == coreaudit.EventsPruned {
			pruned = &store.events[i]
		}
	}
	if pruned == nil {
		t.Fatal("a sweep that deleted events recorded nothing")
	}
	if pruned.ActorType != coreaudit.ActorSystem {
		t.Errorf("actor type = %q, want %q", pruned.ActorType, coreaudit.ActorSystem)
	}
	if !strings.HasPrefix(pruned.Detail, "2 events") {
		t.Errorf("detail = %q, want it to name the count", pruned.Detail)
	}
	if !strings.Contains(pruned.Detail, "to") {
		t.Errorf("detail = %q, want it to name the range removed", pruned.Detail)
	}
}

// A sweep that removed nothing must stay silent. A daily "removed 0 events"
// row would bury the trail it is meant to explain.
func TestAuditRetainerRecordsNothingWhenNothingExpired(t *testing.T) {
	now := time.Now()
	store := &fakeAuditPruner{events: []coreaudit.Event{
		{Action: coreaudit.UserLogin, CreatedAt: at(now, 5)},
	}}

	if removed := retainerAt(store, 90, now).sweep(context.Background()); removed != 0 {
		t.Fatalf("removed %d events, want 0", removed)
	}
	if len(store.events) != 1 {
		t.Fatalf("store holds %d events, want the 1 it started with", len(store.events))
	}
}

// Keeping everything is the default, and a deployment that never chose a
// retention policy must not acquire one by accident.
func TestNewAuditRetainerIsNilWithoutAWindow(t *testing.T) {
	if r := NewAuditRetainer(&fakeAuditPruner{}, &fakeAuditPruner{}, 0, 0); r != nil {
		t.Error("a zero window produced a retainer")
	}
	if r := NewAuditRetainer(nil, nil, 90, 0); r != nil {
		t.Error("a nil store produced a retainer")
	}
	// Start and Stop on a nil retainer must be safe, because that is how the
	// caller avoids branching on it.
	var nilRetainer *AuditRetainer
	nilRetainer.Start()
	nilRetainer.Stop()
}

// A failed delete cannot be reported as a completed one. The sweep gives up,
// leaves the events in place, and writes no prune record — the next tick tries
// again.
func TestAuditRetainerRecordsNothingWhenThePruneFails(t *testing.T) {
	now := time.Now()
	store := &fakeAuditPruner{
		events:   []coreaudit.Event{{Action: coreaudit.UserLogin, CreatedAt: at(now, 100)}},
		pruneErr: errors.New("database is away"),
	}

	if removed := retainerAt(store, 90, now).sweep(context.Background()); removed != 0 {
		t.Fatalf("removed %d events on a failed prune, want 0", removed)
	}
	for _, e := range store.events {
		if e.Action == coreaudit.EventsPruned {
			t.Fatal("a failed prune claimed to have removed events")
		}
	}
}

// A backlog is cleared over several ticks rather than in one statement, so
// turning retention on for the first time cannot hold the table.
func TestAuditRetainerBoundsOneSweep(t *testing.T) {
	now := time.Now()
	store := &fakeAuditPruner{}
	for range auditPruneBatch*auditPruneMaxBatches + 10 {
		store.events = append(store.events, coreaudit.Event{Action: coreaudit.UserLogin, CreatedAt: at(now, 100)})
	}

	removed := retainerAt(store, 90, now).sweep(context.Background())
	if removed != auditPruneBatch*auditPruneMaxBatches {
		t.Fatalf("one sweep removed %d, want %d", removed, auditPruneBatch*auditPruneMaxBatches)
	}
	// The remainder is still there for the next tick — plus the record of what
	// this one removed.
	if len(store.events) != 11 {
		t.Fatalf("%d events left, want the 10 remaining plus the prune record", len(store.events))
	}
}
