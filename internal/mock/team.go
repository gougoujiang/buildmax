package mock

import (
	"context"
	"fmt"
	"time"

	"buildmax/internal/infra/db"
)

// MockTeamStore is an in-memory TeamStore for tests.
type MockTeamStore struct {
	Teams   []db.Team
	Members []db.TeamMember
}

func (m *MockTeamStore) GetTeam(_ context.Context, teamID string) (*db.Team, error) {
	for i := range m.Teams {
		if m.Teams[i].TeamID == teamID {
			return &m.Teams[i], nil
		}
	}
	return nil, nil
}

func (m *MockTeamStore) GetPersonalTeamByUser(_ context.Context, userID string) (*db.Team, error) {
	for i := range m.Teams {
		if m.Teams[i].PersonalForUserID != nil && *m.Teams[i].PersonalForUserID == userID {
			return &m.Teams[i], nil
		}
	}
	return nil, nil
}

func (m *MockTeamStore) ListTeamsByUser(_ context.Context, userID string) ([]db.Team, error) {
	var out []db.Team
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

func (m *MockTeamStore) CreateTeam(_ context.Context, name, createdBy, quotaTier string) (*db.Team, error) {
	id := fmt.Sprintf("tm_%d", len(m.Teams)+1)
	team := db.Team{
		TeamID:    id,
		Name:      name,
		QuotaTier: quotaTier,
		CreatedBy: createdBy,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	m.Teams = append(m.Teams, team)
	m.Members = append(m.Members, db.TeamMember{
		TeamID:    id,
		UserID:    createdBy,
		Role:      db.TeamRoleOwner,
		CreatedAt: time.Now().Unix(),
	})
	return &m.Teams[len(m.Teams)-1], nil
}

func (m *MockTeamStore) AddTeamMember(_ context.Context, teamID, userID, role string) (*db.TeamMember, error) {
	for i := range m.Members {
		if m.Members[i].TeamID == teamID && m.Members[i].UserID == userID {
			m.Members[i].Role = role
			return &m.Members[i], nil
		}
	}
	member := db.TeamMember{
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

func (m *MockTeamStore) ListTeamMembers(_ context.Context, teamID string) ([]db.TeamMember, error) {
	var out []db.TeamMember
	for _, member := range m.Members {
		if member.TeamID == teamID {
			out = append(out, member)
		}
	}
	return out, nil
}
