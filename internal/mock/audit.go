package mock

import (
	"context"
	"sort"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// MockAuditStore is an in-memory model.AuditStore for tests.
type MockAuditStore struct {
	Events []model.AuditEvent
	// Err, when set, is returned by RecordAuditEvent so a caller's behaviour
	// on a failed write can be exercised.
	Err error
}

func (m *MockAuditStore) RecordAuditEvent(_ context.Context, in model.AuditEvent) error {
	if m.Err != nil {
		return m.Err
	}
	in.ID = "ae_" + in.Action
	// Stamped here because the real store stamps it, and a caller that reads
	// its own writes back through a time filter — the quota service does — sees
	// nothing at all if a recorded event stays at time zero.
	if in.CreatedAt.IsZero() {
		in.CreatedAt = time.Now().UTC()
	}
	m.Events = append(m.Events, in)
	return nil
}

func (m *MockAuditStore) ListAuditEvents(_ context.Context, teamID string, limit, offset int) ([]model.AuditEvent, int, error) {
	var all []model.AuditEvent
	for _, e := range m.Events {
		if e.TeamID == teamID {
			all = append(all, e)
		}
	}
	page, total := paginate(all, limit, offset)
	return page, total, nil
}

func (m *MockAuditStore) SearchAuditEvents(_ context.Context, filter model.AuditFilter, limit, offset int) ([]model.AuditEvent, int, error) {
	var all []model.AuditEvent
	for _, e := range m.Events {
		if matchesAuditFilter(e, filter) {
			all = append(all, e)
		}
	}
	page, total := paginate(all, limit, offset)
	return page, total, nil
}

// ExportTeamAuditEvents walks a team's events newest-first from after.
func (m *MockAuditStore) ExportTeamAuditEvents(_ context.Context, teamID string, after model.AuditCursor, limit int) ([]model.AuditEvent, error) {
	return m.exportPage(func(e model.AuditEvent) bool { return e.TeamID == teamID }, after, limit), nil
}

// ExportAuditEvents walks every team's events newest-first from after.
func (m *MockAuditStore) ExportAuditEvents(_ context.Context, filter model.AuditFilter, after model.AuditCursor, limit int) ([]model.AuditEvent, error) {
	return m.exportPage(func(e model.AuditEvent) bool { return matchesAuditFilter(e, filter) }, after, limit), nil
}

// exportPage mirrors the store's keyset paging: newest first, continuing from
// the (created_at, id) pair the previous page stopped at.
//
// The mock assigns no ids, so position within Events stands in for one. It is
// assigned here rather than at write time because a test that appends events
// out of order should still page in the order it declared.
func (m *MockAuditStore) exportPage(keep func(model.AuditEvent) bool, after model.AuditCursor, limit int) []model.AuditEvent {
	if limit <= 0 {
		limit = 200
	}
	type ordered struct {
		event model.AuditEvent
		seq   int
	}
	var matched []ordered
	for i, e := range m.Events {
		if keep(e) {
			matched = append(matched, ordered{event: e, seq: i})
		}
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if !matched[i].event.CreatedAt.Equal(matched[j].event.CreatedAt) {
			return matched[i].event.CreatedAt.After(matched[j].event.CreatedAt)
		}
		return matched[i].seq > matched[j].seq
	})

	// Keyset paging: the cursor names the last event of the previous page, so
	// resume at the one after it. Declaration order stands in for the row key
	// the real store ties with.
	start := 0
	if !after.Zero() {
		start = len(matched)
		for i, o := range matched {
			if o.event.ID == after.ID {
				start = i + 1
				break
			}
		}
	}

	out := make([]model.AuditEvent, 0, limit)
	for _, o := range matched[min(start, len(matched)):] {
		out = append(out, o.event)
		if len(out) == limit {
			break
		}
	}
	return out
}

// PruneAuditEvents removes events older than the cutoff, at most limit of them.
func (m *MockAuditStore) PruneAuditEvents(_ context.Context, before time.Time, limit int) (int64, error) {
	if before.IsZero() {
		return 0, nil
	}
	if limit <= 0 {
		limit = 1000
	}
	kept := make([]model.AuditEvent, 0, len(m.Events))
	var removed int64
	for _, e := range m.Events {
		if e.CreatedAt.Before(before) && removed < int64(limit) {
			removed++
			continue
		}
		kept = append(kept, e)
	}
	m.Events = kept
	return removed, nil
}

// OldestAuditEventAt returns the earliest recorded timestamp, or zero.
func (m *MockAuditStore) OldestAuditEventAt(_ context.Context) (time.Time, error) {
	var oldest time.Time
	for _, e := range m.Events {
		if oldest.IsZero() || e.CreatedAt.Before(oldest) {
			oldest = e.CreatedAt
		}
	}
	return oldest, nil
}

func matchesAuditFilter(e model.AuditEvent, filter model.AuditFilter) bool {
	switch {
	case filter.WithoutTeam && e.TeamID != "":
		return false
	case !filter.WithoutTeam && filter.TeamID != "" && e.TeamID != filter.TeamID:
		return false
	}
	if filter.ActorID != "" && e.ActorID != filter.ActorID {
		return false
	}
	if filter.Action != "" && e.Action != filter.Action {
		return false
	}
	if !filter.Since.IsZero() && e.CreatedAt.Before(filter.Since) {
		return false
	}
	if !filter.Until.IsZero() && !e.CreatedAt.Before(filter.Until) {
		return false
	}
	return true
}
