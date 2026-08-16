package mock

import (
	"context"

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
	in.AuditEventID = "ae_" + in.Action
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
	total := len(all)
	if offset > total {
		offset = total
	}
	all = all[offset:]
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	return all, total, nil
}
