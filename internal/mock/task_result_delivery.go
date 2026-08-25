package mock

import (
	"context"
	"sort"
	"sync"
	"time"

	coretask "github.com/gougoujiang/buildmax/internal/core/task"
)

// MockTaskResultDeliveryStore is an in-memory TaskResultDeliveryStore.
//
// The claim is a real compare-and-set, not a stub that always succeeds: what it
// exists to prevent is one run being reported twice, and a double that cannot
// refuse a second claim would let that pass.
type MockTaskResultDeliveryStore struct {
	mu         sync.Mutex
	Deliveries map[string]*coretask.ResultDelivery
	// EnqueueErr, when set, is returned by every enqueue.
	EnqueueErr error
}

func (m *MockTaskResultDeliveryStore) ensure() {
	if m.Deliveries == nil {
		m.Deliveries = map[string]*coretask.ResultDelivery{}
	}
}

func (m *MockTaskResultDeliveryStore) EnqueueTaskResultDelivery(_ context.Context, taskRunID, conversationID string, now time.Time) error {
	if m.EnqueueErr != nil {
		return m.EnqueueErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensure()
	if _, exists := m.Deliveries[taskRunID]; exists {
		return nil
	}
	m.Deliveries[taskRunID] = &coretask.ResultDelivery{
		TaskRunID:      taskRunID,
		ConversationID: conversationID,
		Status:         coretask.DeliveryPending,
		NextAttemptAt:  now,
		CreatedAt:      now,
	}
	return nil
}

func (m *MockTaskResultDeliveryStore) ListDueTaskResultDeliveries(_ context.Context, now time.Time, limit int) ([]coretask.ResultDelivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensure()
	var out []coretask.ResultDelivery
	for _, d := range m.Deliveries {
		if d.Status == coretask.DeliveryPending && !d.NextAttemptAt.After(now) {
			out = append(out, *d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NextAttemptAt.Before(out[j].NextAttemptAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MockTaskResultDeliveryStore) ClaimTaskResultDelivery(_ context.Context, taskRunID string, now, nextAttemptAt time.Time) (*coretask.ResultDelivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensure()
	d, ok := m.Deliveries[taskRunID]
	if !ok || d.Status != coretask.DeliveryPending || d.NextAttemptAt.After(now) {
		return nil, nil
	}
	d.Attempts++
	d.NextAttemptAt = nextAttemptAt
	claimed := *d
	return &claimed, nil
}

func (m *MockTaskResultDeliveryStore) FinishTaskResultDelivery(_ context.Context, taskRunID, status string, lastError *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensure()
	d, ok := m.Deliveries[taskRunID]
	if !ok {
		return nil
	}
	d.Status = status
	if lastError != nil {
		d.LastError = lastError
	}
	return nil
}

func (m *MockTaskResultDeliveryStore) RecordTaskResultDeliveryFailure(_ context.Context, taskRunID, lastError string, nextAttemptAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensure()
	d, ok := m.Deliveries[taskRunID]
	if !ok || d.Status != coretask.DeliveryPending {
		return nil
	}
	msg := lastError
	d.LastError = &msg
	d.NextAttemptAt = nextAttemptAt
	return nil
}

// Get returns a copy of one delivery, or nil.
func (m *MockTaskResultDeliveryStore) Get(taskRunID string) *coretask.ResultDelivery {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensure()
	d, ok := m.Deliveries[taskRunID]
	if !ok {
		return nil
	}
	out := *d
	return &out
}
