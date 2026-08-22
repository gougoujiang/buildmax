package mock

import (
	"context"
	"sort"
	"sync"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// MockArtifactStore is an in-memory model.ArtifactStore for tests.
type MockArtifactStore struct {
	mu    sync.Mutex
	items []model.Artifact
	// CreateErr, when set, fails every create — the case where content is
	// already durable and the record is not.
	CreateErr error
}

func (m *MockArtifactStore) CreateArtifact(_ context.Context, in model.CreateArtifactInput) (*model.Artifact, error) {
	if m.CreateErr != nil {
		return nil, m.CreateErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rec := model.Artifact{
		ID:            in.ArtifactID,
		TeamID:        in.TeamID,
		Filename:      in.Filename,
		MediaType:     in.MediaType,
		SizeBytes:     in.SizeBytes,
		SHA256:        in.SHA256,
		StorageKey:    in.StorageKey,
		CreatedByType: in.CreatedByType,
		CreatedByID:   in.CreatedByID,
		SourceType:    in.SourceType,
		SourceID:      in.SourceID,
		Title:         in.Title,
		ExpiresAt:     in.ExpiresAt,
		CreatedAt:     int64(len(m.items) + 1),
	}
	m.items = append(m.items, rec)
	out := rec
	return &out, nil
}

func (m *MockArtifactStore) GetArtifact(_ context.Context, artifactID string) (*model.Artifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.items {
		if m.items[i].ID == artifactID {
			out := m.items[i]
			return &out, nil
		}
	}
	return nil, nil
}

func (m *MockArtifactStore) ListArtifactsByTeam(_ context.Context, teamID string, limit, offset int) ([]model.Artifact, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var live []model.Artifact
	for i := range m.items {
		if m.items[i].TeamID == teamID && m.items[i].DeletedAt == nil {
			live = append(live, m.items[i])
		}
	}
	sort.SliceStable(live, func(i, j int) bool { return live[i].CreatedAt > live[j].CreatedAt })
	page, total := paginate(live, limit, offset)
	return page, total, nil
}

func (m *MockArtifactStore) ListArtifactsBySource(_ context.Context, sourceIDs []string) (map[string][]model.Artifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := make(map[string]bool, len(sourceIDs))
	for _, id := range sourceIDs {
		want[id] = true
	}
	out := make(map[string][]model.Artifact)
	for i := len(m.items) - 1; i >= 0; i-- {
		it := m.items[i]
		if it.DeletedAt == nil && want[it.SourceID] {
			out[it.SourceID] = append(out[it.SourceID], it)
		}
	}
	return out, nil
}

func (m *MockArtifactStore) SoftDeleteArtifact(_ context.Context, artifactID string, deletedAt int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.items {
		if m.items[i].ID == artifactID && m.items[i].DeletedAt == nil {
			m.items[i].DeletedAt = &deletedAt
			return true, nil
		}
	}
	return false, nil
}

// Count reports how many records exist, tombstoned ones included. Tests use it
// to prove that a failed create left none.
func (m *MockArtifactStore) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.items)
}
