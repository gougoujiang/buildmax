package mock

import (
	"context"
	"fmt"
	"slices"
	"time"

	agentdef "github.com/gougoujiang/buildmax/internal/core/agentdef"
	"github.com/gougoujiang/buildmax/internal/core/apierr"
	"github.com/gougoujiang/buildmax/internal/util"
)

// MockAgentStore is an in-memory AgentStore for tests. It records revisions the
// way the database store does, so a test can assert on history.
type MockAgentStore struct {
	Agents    []agentdef.Agent
	Revisions []agentdef.Revision
}

func (m *MockAgentStore) appendRevision(a *agentdef.Agent, createdBy string) {
	m.Revisions = append(m.Revisions, agentdef.Revision{
		AgentID:               a.ID,
		Revision:              a.Revision,
		Name:                  a.Name,
		Description:           a.Description,
		Instructions:          a.Instructions,
		Plugins:               a.Plugins,
		SandboxNetworkTier:    a.SandboxNetworkTier,
		SandboxFilesystemTier: a.SandboxFilesystemTier,
		CreatedBy:             createdBy,
		CreatedAt:             time.Now().UTC(),
	})
}

func (m *MockAgentStore) updateAgentAt(i int, updatedBy string, def agentdef.Definition) *agentdef.Agent {
	if m.Agents[i].Name == def.Name && m.Agents[i].Description == def.Description &&
		m.Agents[i].Instructions == def.Instructions && slices.Equal(m.Agents[i].Plugins, def.Plugins) &&
		m.Agents[i].SandboxNetworkTier == def.SandboxNetworkTier && m.Agents[i].SandboxFilesystemTier == def.SandboxFilesystemTier {
		return &m.Agents[i]
	}
	m.Agents[i].Name = def.Name
	m.Agents[i].Description = def.Description
	m.Agents[i].Instructions = def.Instructions
	m.Agents[i].Plugins = def.Plugins
	m.Agents[i].SandboxNetworkTier = def.SandboxNetworkTier
	m.Agents[i].SandboxFilesystemTier = def.SandboxFilesystemTier
	if m.Agents[i].Revision < 1 {
		m.Agents[i].Revision = 1
	}
	m.Agents[i].Revision++
	m.appendRevision(&m.Agents[i], updatedBy)
	return &m.Agents[i]
}

func (m *MockAgentStore) ListAgentsByUser(_ context.Context, userID string) ([]agentdef.Agent, error) {
	var out []agentdef.Agent
	for _, a := range m.Agents {
		if a.DeletedAt == nil && a.UserID == userID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *MockAgentStore) ListAgentsByTeam(_ context.Context, teamID string) ([]agentdef.Agent, error) {
	var out []agentdef.Agent
	for _, a := range m.Agents {
		if a.DeletedAt == nil && a.TeamID == teamID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *MockAgentStore) GetAgent(_ context.Context, agentID string) (*agentdef.Agent, error) {
	for i := range m.Agents {
		if m.Agents[i].ID == agentID && m.Agents[i].DeletedAt == nil {
			return &m.Agents[i], nil
		}
	}
	return nil, nil
}

func (m *MockAgentStore) GetAgentIncludingDeleted(_ context.Context, agentID string) (*agentdef.Agent, error) {
	for i := range m.Agents {
		if m.Agents[i].ID == agentID {
			return &m.Agents[i], nil
		}
	}
	return nil, nil
}

func (m *MockAgentStore) CreateAgentInTeam(_ context.Context, in agentdef.CreateInput) (*agentdef.Agent, error) {
	teamID := in.TeamID
	if teamID == "" {
		teamID = "tm_personal"
	}
	a := agentdef.Agent{
		ID:                    fmt.Sprintf("a_%d", len(m.Agents)+1),
		UserID:                in.UserID,
		TeamID:                teamID,
		Name:                  in.Def.Name,
		Description:           in.Def.Description,
		Instructions:          in.Def.Instructions,
		Plugins:               in.Def.Plugins,
		SandboxNetworkTier:    in.Def.SandboxNetworkTier,
		SandboxFilesystemTier: in.Def.SandboxFilesystemTier,
		Revision:              1,
		CreatedAt:             time.Now().UTC(),
	}
	m.Agents = append(m.Agents, a)
	created := &m.Agents[len(m.Agents)-1]
	m.appendRevision(created, in.UserID)
	return created, nil
}

func (m *MockAgentStore) UpdateAgentInTeam(_ context.Context, in agentdef.UpdateInput) (*agentdef.Agent, error) {
	for i := range m.Agents {
		if m.Agents[i].ID == in.AgentID && m.Agents[i].TeamID == in.TeamID && m.Agents[i].DeletedAt == nil {
			return m.updateAgentAt(i, in.UpdatedBy, in.Def), nil
		}
	}
	return nil, nil
}

func (m *MockAgentStore) ListAgentRevisions(_ context.Context, agentID string, limit, offset int) ([]agentdef.Revision, int, error) {
	var all []agentdef.Revision
	for i := len(m.Revisions) - 1; i >= 0; i-- {
		if m.Revisions[i].AgentID == agentID {
			all = append(all, m.Revisions[i])
		}
	}
	return pageRevisions(all, limit, offset), len(all), nil
}

func (m *MockAgentStore) GetAgentRevision(_ context.Context, agentID string, revision int) (*agentdef.Revision, error) {
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
	return apierr.ErrNotFound
}

func (m *MockAgentStore) DeleteAgentInTeam(_ context.Context, agentID, teamID string) error {
	for i := range m.Agents {
		if m.Agents[i].ID == agentID && m.Agents[i].TeamID == teamID && m.Agents[i].DeletedAt == nil {
			m.Agents[i].DeletedAt = util.Ptr(time.Now().UTC())
			return nil
		}
	}
	return apierr.ErrNotFound
}
