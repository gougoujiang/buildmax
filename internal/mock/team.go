package mock

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// MockTeamStore is an in-memory TeamStore for tests.
type MockTeamStore struct {
	Teams   []model.Team
	Members []model.TeamMember
}

func (m *MockTeamStore) GetTeam(_ context.Context, teamID string) (*model.Team, error) {
	for i := range m.Teams {
		if m.Teams[i].TeamID == teamID {
			return &m.Teams[i], nil
		}
	}
	return nil, nil
}

func (m *MockTeamStore) GetPersonalTeamByUser(_ context.Context, userID string) (*model.Team, error) {
	for i := range m.Teams {
		if m.Teams[i].PersonalForUserID != nil && *m.Teams[i].PersonalForUserID == userID {
			return &m.Teams[i], nil
		}
	}
	return nil, nil
}

func (m *MockTeamStore) ListTeamsByUser(_ context.Context, userID string) ([]model.Team, error) {
	var out []model.Team
	for _, member := range m.Members {
		if member.UserID != userID {
			continue
		}
		for _, team := range m.Teams {
			if team.TeamID == member.TeamID {
				out = append(out, team)
			}
		}
	}
	return out, nil
}

func (m *MockTeamStore) CreateTeam(_ context.Context, name, createdBy, quotaTier string) (*model.Team, error) {
	id := fmt.Sprintf("tm_%d", len(m.Teams)+1)
	team := model.Team{
		TeamID:    id,
		Name:      name,
		QuotaTier: quotaTier,
		CreatedBy: createdBy,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	m.Teams = append(m.Teams, team)
	m.Members = append(m.Members, model.TeamMember{
		TeamID:    id,
		UserID:    createdBy,
		Role:      model.TeamRoleOwner,
		CreatedAt: time.Now().Unix(),
	})
	return &m.Teams[len(m.Teams)-1], nil
}

func (m *MockTeamStore) AddTeamMember(_ context.Context, teamID, userID, role string) (*model.TeamMember, error) {
	for i := range m.Members {
		if m.Members[i].TeamID == teamID && m.Members[i].UserID == userID {
			m.Members[i].Role = role
			return &m.Members[i], nil
		}
	}
	member := model.TeamMember{
		TeamID:    teamID,
		UserID:    userID,
		Role:      role,
		CreatedAt: time.Now().Unix(),
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

func (m *MockTeamStore) ListTeamMembers(_ context.Context, teamID string) ([]model.TeamMember, error) {
	var out []model.TeamMember
	for _, member := range m.Members {
		if member.TeamID == teamID {
			out = append(out, member)
		}
	}
	return out, nil
}

func (m *MockTeamStore) ListAllTeams(_ context.Context, query string, limit, offset int) ([]model.Team, int, error) {
	var all []model.Team
	for i := range m.Teams {
		if query == "" || strings.Contains(m.Teams[i].Name, query) {
			all = append(all, m.Teams[i])
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
