package mock

import (
	"context"
	"fmt"
	"time"

	"buildmax/internal/infra/db"

	"gorm.io/gorm"
)

// MockAgentStore is an in-memory AgentStore for tests.
type MockAgentStore struct {
	Agents []db.Agent
}

func (m *MockAgentStore) ListAgentsByUser(_ context.Context, userID string) ([]db.Agent, error) {
	var out []db.Agent
	for _, a := range m.Agents {
		if a.UserID == userID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *MockAgentStore) ListAgentsByTeam(_ context.Context, teamID string) ([]db.Agent, error) {
	var out []db.Agent
	for _, a := range m.Agents {
		if a.TeamID == teamID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *MockAgentStore) GetAgent(_ context.Context, agentID string) (*db.Agent, error) {
	for i := range m.Agents {
		if m.Agents[i].AgentID == agentID {
			return &m.Agents[i], nil
		}
	}
	return nil, nil
}

func (m *MockAgentStore) CreateAgent(_ context.Context, userID, name, description, instructions string) (*db.Agent, error) {
	return m.CreateAgentInTeam(context.Background(), "tm_personal", userID, name, description, instructions)
}

func (m *MockAgentStore) CreateAgentInTeam(_ context.Context, teamID, userID, name, description, instructions string) (*db.Agent, error) {
	a := db.Agent{
		AgentID:      fmt.Sprintf("a_%d", len(m.Agents)+1),
		UserID:       userID,
		TeamID:       teamID,
		Name:         name,
		Description:  description,
		Instructions: instructions,
		CreatedAt:    time.Now().Unix(),
	}
	m.Agents = append(m.Agents, a)
	return &m.Agents[len(m.Agents)-1], nil
}

func (m *MockAgentStore) UpdateAgent(_ context.Context, agentID, userID, name, description, instructions string) (*db.Agent, error) {
	for i := range m.Agents {
		if m.Agents[i].AgentID == agentID && m.Agents[i].UserID == userID {
			m.Agents[i].Name = name
			m.Agents[i].Description = description
			m.Agents[i].Instructions = instructions
			return &m.Agents[i], nil
		}
	}
	return nil, nil
}

func (m *MockAgentStore) UpdateAgentInTeam(_ context.Context, agentID, teamID, name, description, instructions string) (*db.Agent, error) {
	for i := range m.Agents {
		if m.Agents[i].AgentID == agentID && m.Agents[i].TeamID == teamID {
			m.Agents[i].Name = name
			m.Agents[i].Description = description
			m.Agents[i].Instructions = instructions
			return &m.Agents[i], nil
		}
	}
	return nil, nil
}

func (m *MockAgentStore) DeleteAgent(_ context.Context, agentID, userID string) error {
	for i := range m.Agents {
		if m.Agents[i].AgentID == agentID && m.Agents[i].UserID == userID {
			m.Agents = append(m.Agents[:i], m.Agents[i+1:]...)
			return nil
		}
	}
	return gorm.ErrRecordNotFound
}

func (m *MockAgentStore) DeleteAgentInTeam(_ context.Context, agentID, teamID string) error {
	for i := range m.Agents {
		if m.Agents[i].AgentID == agentID && m.Agents[i].TeamID == teamID {
			m.Agents = append(m.Agents[:i], m.Agents[i+1:]...)
			return nil
		}
	}
	return gorm.ErrRecordNotFound
}
