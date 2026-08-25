package mock

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	"github.com/gougoujiang/buildmax/internal/core/model"
)

// MockPluginActivationStore is an in-memory team activation store for tests.
type MockPluginActivationStore struct {
	// rows is keyed by team and plugin, which is the pair the real unique
	// index covers.
	rows map[string]*model.PluginActivation
	next int
}

func NewMockPluginActivationStore() *MockPluginActivationStore {
	return &MockPluginActivationStore{rows: map[string]*model.PluginActivation{}}
}

func activationKey(teamID, pluginName string) string { return teamID + "\x00" + pluginName }

func (m *MockPluginActivationStore) ActivatePlugin(_ context.Context, in model.ActivatePluginInput) (*model.PluginActivation, error) {
	key := activationKey(in.TeamID, in.PluginName)
	if _, exists := m.rows[key]; exists {
		return nil, model.ErrPluginAlreadyActivated
	}
	m.next++
	now := time.Now().UTC()
	row := &model.PluginActivation{
		ID:          fmt.Sprintf("pa_%d", m.next),
		TeamID:      in.TeamID,
		PluginName:  in.PluginName,
		Version:     in.Version,
		Digest:      in.Digest,
		Enabled:     true,
		Origin:      in.Origin,
		ActivatedBy: in.ActorID,
		ActivatedAt: now,
		UpdatedBy:   in.ActorID,
		UpdatedAt:   now,
	}
	m.rows[key] = row
	out := *row
	return &out, nil
}

func (m *MockPluginActivationStore) GetPluginActivation(_ context.Context, teamID, pluginName string) (*model.PluginActivation, error) {
	row, ok := m.rows[activationKey(teamID, pluginName)]
	if !ok {
		return nil, nil
	}
	out := *row
	return &out, nil
}

func (m *MockPluginActivationStore) ListPluginActivations(_ context.Context, teamID string) ([]model.PluginActivation, error) {
	out := make([]model.PluginActivation, 0, len(m.rows))
	for _, row := range m.rows {
		if row.TeamID == teamID {
			out = append(out, *row)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *MockPluginActivationStore) MovePluginActivationPin(_ context.Context, in model.MovePluginActivationPinInput) (*model.PluginActivation, error) {
	row, ok := m.rows[activationKey(in.TeamID, in.PluginName)]
	if !ok {
		return nil, apierr.ErrNotFound
	}
	row.Version = in.Version
	row.Digest = in.Digest
	row.UpdatedBy = in.ActorID
	row.UpdatedAt = time.Now().UTC()
	out := *row
	return &out, nil
}

func (m *MockPluginActivationStore) SetPluginActivationEnabled(_ context.Context, teamID, pluginName string, enabled bool, actorID string) (*model.PluginActivation, error) {
	row, ok := m.rows[activationKey(teamID, pluginName)]
	if !ok {
		return nil, apierr.ErrNotFound
	}
	row.Enabled = enabled
	row.UpdatedBy = actorID
	row.UpdatedAt = time.Now().UTC()
	out := *row
	return &out, nil
}
