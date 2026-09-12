package mock

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/icloudbb/buildmax/internal/core/apierr"
	coreplugin "github.com/icloudbb/buildmax/internal/core/plugin"
)

// MockPluginActivationStore is an in-memory space activation store for tests.
type MockPluginActivationStore struct {
	// rows is keyed by space and plugin, which is the pair the real unique
	// index covers.
	rows map[string]*coreplugin.Activation
	next int
}

func NewMockPluginActivationStore() *MockPluginActivationStore {
	return &MockPluginActivationStore{rows: map[string]*coreplugin.Activation{}}
}

func activationKey(spaceID, pluginName string) string { return spaceID + "\x00" + pluginName }

func (m *MockPluginActivationStore) ActivatePlugin(_ context.Context, in coreplugin.ActivateInput) (*coreplugin.Activation, error) {
	key := activationKey(in.SpaceID, in.PluginName)
	if _, exists := m.rows[key]; exists {
		return nil, coreplugin.ErrAlreadyActivated
	}
	m.next++
	now := time.Now().UTC()
	row := &coreplugin.Activation{
		ID:          fmt.Sprintf("pa_%d", m.next),
		SpaceID:     in.SpaceID,
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

func (m *MockPluginActivationStore) GetPluginActivation(_ context.Context, spaceID, pluginName string) (*coreplugin.Activation, error) {
	row, ok := m.rows[activationKey(spaceID, pluginName)]
	if !ok {
		return nil, nil
	}
	out := *row
	return &out, nil
}

func (m *MockPluginActivationStore) ListPluginActivations(_ context.Context, spaceID string) ([]coreplugin.Activation, error) {
	out := make([]coreplugin.Activation, 0, len(m.rows))
	for _, row := range m.rows {
		if row.SpaceID == spaceID {
			out = append(out, *row)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *MockPluginActivationStore) MovePluginActivationPin(_ context.Context, in coreplugin.MovePinInput) (*coreplugin.Activation, error) {
	row, ok := m.rows[activationKey(in.SpaceID, in.PluginName)]
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

func (m *MockPluginActivationStore) SetPluginActivationEnabled(_ context.Context, spaceID, pluginName string, enabled bool, actorID string) (*coreplugin.Activation, error) {
	row, ok := m.rows[activationKey(spaceID, pluginName)]
	if !ok {
		return nil, apierr.ErrNotFound
	}
	row.Enabled = enabled
	row.UpdatedBy = actorID
	row.UpdatedAt = time.Now().UTC()
	out := *row
	return &out, nil
}
