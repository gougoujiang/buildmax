package mock

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/util"
)

// MockAgentStore is an in-memory AgentStore for tests. It records revisions the
// way the database store does, so a test can assert on history.
type MockAgentStore struct {
	Agents    []model.Agent
	Revisions []model.AgentRevision
}

func (m *MockAgentStore) appendRevision(a *model.Agent, createdBy string) {
	m.Revisions = append(m.Revisions, model.AgentRevision{
		AgentID:      a.ID,
		Revision:     a.Revision,
		Name:         a.Name,
		Description:  a.Description,
		Instructions: a.Instructions,
		Plugins:      a.Plugins,
		CreatedBy:    createdBy,
		CreatedAt:    time.Now().UTC(),
	})
}

func (m *MockAgentStore) updateAgentAt(i int, updatedBy string, def model.AgentDefinition) *model.Agent {
	if m.Agents[i].Name == def.Name && m.Agents[i].Description == def.Description &&
		m.Agents[i].Instructions == def.Instructions && slices.Equal(m.Agents[i].Plugins, def.Plugins) {
		return &m.Agents[i]
	}
	m.Agents[i].Name = def.Name
	m.Agents[i].Description = def.Description
	m.Agents[i].Instructions = def.Instructions
	m.Agents[i].Plugins = def.Plugins
	if m.Agents[i].Revision < 1 {
		m.Agents[i].Revision = 1
	}
	m.Agents[i].Revision++
	m.appendRevision(&m.Agents[i], updatedBy)
	return &m.Agents[i]
}

func (m *MockAgentStore) ListAgentsByUser(_ context.Context, userID string) ([]model.Agent, error) {
	var out []model.Agent
	for _, a := range m.Agents {
		if a.DeletedAt == nil && a.UserID == userID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *MockAgentStore) ListAgentsByTeam(_ context.Context, teamID string) ([]model.Agent, error) {
	var out []model.Agent
	for _, a := range m.Agents {
		if a.DeletedAt == nil && a.TeamID == teamID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *MockAgentStore) GetAgent(_ context.Context, agentID string) (*model.Agent, error) {
	for i := range m.Agents {
		if m.Agents[i].ID == agentID && m.Agents[i].DeletedAt == nil {
			return &m.Agents[i], nil
		}
	}
	return nil, nil
}

func (m *MockAgentStore) GetAgentIncludingDeleted(_ context.Context, agentID string) (*model.Agent, error) {
	for i := range m.Agents {
		if m.Agents[i].ID == agentID {
			return &m.Agents[i], nil
		}
	}
	return nil, nil
}

func (m *MockAgentStore) CreateAgentInTeam(_ context.Context, in model.CreateAgentInput) (*model.Agent, error) {
	teamID := in.TeamID
	if teamID == "" {
		teamID = "tm_personal"
	}
	a := model.Agent{
		ID:           fmt.Sprintf("a_%d", len(m.Agents)+1),
		UserID:       in.UserID,
		TeamID:       teamID,
		Name:         in.Def.Name,
		Description:  in.Def.Description,
		Instructions: in.Def.Instructions,
		Plugins:      in.Def.Plugins,
		Revision:     1,
		CreatedAt:    time.Now().UTC(),
	}
	m.Agents = append(m.Agents, a)
	created := &m.Agents[len(m.Agents)-1]
	m.appendRevision(created, in.UserID)
	return created, nil
}

func (m *MockAgentStore) UpdateAgentInTeam(_ context.Context, in model.UpdateAgentInput) (*model.Agent, error) {
	for i := range m.Agents {
		if m.Agents[i].ID == in.AgentID && m.Agents[i].TeamID == in.TeamID && m.Agents[i].DeletedAt == nil {
			return m.updateAgentAt(i, in.UpdatedBy, in.Def), nil
		}
	}
	return nil, nil
}

func (m *MockAgentStore) ListAgentRevisions(_ context.Context, agentID string, limit, offset int) ([]model.AgentRevision, int, error) {
	var all []model.AgentRevision
	for i := len(m.Revisions) - 1; i >= 0; i-- {
		if m.Revisions[i].AgentID == agentID {
			all = append(all, m.Revisions[i])
		}
	}
	return pageRevisions(all, limit, offset), len(all), nil
}

func (m *MockAgentStore) GetAgentRevision(_ context.Context, agentID string, revision int) (*model.AgentRevision, error) {
	for i := range m.Revisions {
		if m.Revisions[i].AgentID == agentID && m.Revisions[i].Revision == revision {
			return &m.Revisions[i], nil
		}
	}
	return nil, nil
}

func (m *MockAgentStore) DeleteAgent(_ context.Context, agentID, userID string) error {
	for i := range m.Agents {
		if m.Agents[i].ID == agentID && m.Agents[i].UserID == userID && m.Agents[i].DeletedAt == nil {
			m.Agents[i].DeletedAt = util.Ptr(time.Now().UTC())
			return nil
		}
	}
	return model.ErrNotFound
}

func (m *MockAgentStore) DeleteAgentInTeam(_ context.Context, agentID, teamID string) error {
	for i := range m.Agents {
		if m.Agents[i].ID == agentID && m.Agents[i].TeamID == teamID && m.Agents[i].DeletedAt == nil {
			m.Agents[i].DeletedAt = util.Ptr(time.Now().UTC())
			return nil
		}
	}
	return model.ErrNotFound
}
