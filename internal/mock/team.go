package mock

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
)

// MockTeamStore is an in-memory TeamStore for tests.
type MockTeamStore struct {
	Teams   []coreteam.Team
	Members []coreteam.Member
}

func (m *MockTeamStore) GetTeam(_ context.Context, teamID string) (*coreteam.Team, error) {
	for i := range m.Teams {
		if m.Teams[i].ID == teamID {
			return &m.Teams[i], nil
		}
	}
	return nil, nil
}

func (m *MockTeamStore) GetPersonalTeamByUser(_ context.Context, userID string) (*coreteam.Team, error) {
	for i := range m.Teams {
		if m.Teams[i].PersonalForUserID != nil && *m.Teams[i].PersonalForUserID == userID {
			return &m.Teams[i], nil
		}
	}
	return nil, nil
}

func (m *MockTeamStore) ListTeamsByUser(_ context.Context, userID string) ([]coreteam.Team, error) {
	var out []coreteam.Team
	for _, member := range m.Members {
		if member.UserID != userID {
			continue
		}
		for _, team := range m.Teams {
			if team.ID == member.TeamID {
				out = append(out, team)
			}
		}
	}
	return out, nil
}

func (m *MockTeamStore) CreateTeam(_ context.Context, name, createdBy, quotaTier string) (*coreteam.Team, error) {
	id := fmt.Sprintf("tm_%d", len(m.Teams)+1)
	team := coreteam.Team{
		ID:        id,
		Name:      name,
		QuotaTier: quotaTier,
		CreatedBy: createdBy,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	m.Teams = append(m.Teams, team)
	m.Members = append(m.Members, coreteam.Member{
		TeamID:    id,
		UserID:    createdBy,
		Role:      coreteam.RoleOwner,
		CreatedAt: time.Now().UTC(),
	})
	return &m.Teams[len(m.Teams)-1], nil
}

func (m *MockTeamStore) AddTeamMember(_ context.Context, teamID, userID, role string) (*coreteam.Member, error) {
	for i := range m.Members {
		if m.Members[i].TeamID == teamID && m.Members[i].UserID == userID {
			m.Members[i].Role = role
			return &m.Members[i], nil
		}
	}
	member := coreteam.Member{
		TeamID:    teamID,
		UserID:    userID,
		Role:      role,
		CreatedAt: time.Now().UTC(),
	}
	m.Members = append(m.Members, member)
	return &m.Members[len(m.Members)-1], nil
}

func (m *MockTeamStore) RemoveTeamMember(_ context.Context, teamID, userID string) error {
	out := m.Members[:0]
	for _, member := range m.Members {
		if member.TeamID == teamID && member.UserID == userID {
			continue
		}
		out = append(out, member)
	}
	m.Members = out
	return nil
}

func (m *MockTeamStore) ListTeamMembers(_ context.Context, teamID string) ([]coreteam.Member, error) {
	var out []coreteam.Member
	for _, member := range m.Members {
		if member.TeamID == teamID {
			out = append(out, member)
		}
	}
	return out, nil
}

func (m *MockTeamStore) ListAllTeams(_ context.Context, query string, limit, offset int) ([]coreteam.Team, int, error) {
	var all []coreteam.Team
	for i := range m.Teams {
		if query == "" || strings.Contains(m.Teams[i].Name, query) {
			all = append(all, m.Teams[i])
		}
	}
	page, total := paginate(all, limit, offset)
	return page, total, nil
}

func (m *MockTeamStore) CountTeamMembers(_ context.Context, teamIDs []string) (map[string]int, error) {
	wanted := make(map[string]bool, len(teamIDs))
	for _, id := range teamIDs {
		wanted[id] = true
	}
	out := make(map[string]int, len(teamIDs))
	for _, member := range m.Members {
		if wanted[member.TeamID] {
			out[member.TeamID]++
		}
	}
	return out, nil
}

func (m *MockTeamStore) SetTeamPluginCuration(_ context.Context, teamID string, mode coreplugin.Curation) error {
	for i := range m.Teams {
		if m.Teams[i].ID == teamID {
			m.Teams[i].PluginCuration = mode
			return nil
		}
	}
	return apierr.ErrNotFound
}
