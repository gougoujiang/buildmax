package mock

import (
	"context"
	"fmt"
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
		CreatedBy:    createdBy,
		CreatedAt:    time.Now().UTC(),
	})
}

func (m *MockAgentStore) updateAgentAt(i int, updatedBy, name, description, instructions string) *model.Agent {
	if m.Agents[i].Name == name && m.Agents[i].Description == description && m.Agents[i].Instructions == instructions {
		return &m.Agents[i]
	}
	m.Agents[i].Name = name
	m.Agents[i].Description = description
	m.Agents[i].Instructions = instructions
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

func (m *MockAgentStore) CreateAgent(_ context.Context, userID, name, description, instructions string) (*model.Agent, error) {
	return m.CreateAgentInTeam(context.Background(), "tm_personal", userID, name, description, instructions)
}

func (m *MockAgentStore) CreateAgentInTeam(_ context.Context, teamID, userID, name, description, instructions string) (*model.Agent, error) {
	a := model.Agent{
		ID:           fmt.Sprintf("a_%d", len(m.Agents)+1),
		UserID:       userID,
		TeamID:       teamID,
		Name:         name,
		Description:  description,
		Instructions: instructions,
		Revision:     1,
		CreatedAt:    time.Now().UTC(),
	}
	m.Agents = append(m.Agents, a)
	created := &m.Agents[len(m.Agents)-1]
	m.appendRevision(created, userID)
	return created, nil
}

func (m *MockAgentStore) UpdateAgent(_ context.Context, agentID, userID, name, description, instructions string) (*model.Agent, error) {
	for i := range m.Agents {
		if m.Agents[i].ID == agentID && m.Agents[i].UserID == userID && m.Agents[i].DeletedAt == nil {
			return m.updateAgentAt(i, userID, name, description, instructions), nil
		}
	}
	return nil, nil
}

func (m *MockAgentStore) UpdateAgentInTeam(_ context.Context, agentID, teamID, updatedBy, name, description, instructions string) (*model.Agent, error) {
	for i := range m.Agents {
		if m.Agents[i].ID == agentID && m.Agents[i].TeamID == teamID && m.Agents[i].DeletedAt == nil {
			return m.updateAgentAt(i, updatedBy, name, description, instructions), nil
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
