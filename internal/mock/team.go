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
	Teams       []coreteam.Team
	Members     []coreteam.Member
	Invitations []coreteam.Invitation

	invitationSeq int
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

func (m *MockTeamStore) TransferOwnership(_ context.Context, teamID, fromUserID, toUserID string) error {
	for i := range m.Members {
		if m.Members[i].TeamID != teamID {
			continue
		}
		if m.Members[i].UserID == toUserID {
			m.Members[i].Role = coreteam.RoleOwner
		}
		if m.Members[i].UserID == fromUserID {
			m.Members[i].Role = coreteam.RoleAdmin
		}
	}
	return nil
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

func (m *MockTeamStore) SetTeamSandboxDefaults(_ context.Context, teamID, networkTier, filesystemTier string) error {
	for i := range m.Teams {
		if m.Teams[i].ID == teamID {
			m.Teams[i].DefaultSandboxNetworkTier = networkTier
			m.Teams[i].DefaultSandboxFilesystemTier = filesystemTier
			return nil
		}
	}
	return apierr.ErrNotFound
}

func (m *MockTeamStore) CreateInvitation(_ context.Context, teamID, userID, role, invitedBy string, expiresAt time.Time) (*coreteam.Invitation, error) {
	m.invitationSeq++
	inv := coreteam.Invitation{
		ID:        fmt.Sprintf("inv_%d", m.invitationSeq),
		TeamID:    teamID,
		UserID:    userID,
		Role:      role,
		InvitedBy: invitedBy,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
	}
	m.Invitations = append(m.Invitations, inv)
	return &m.Invitations[len(m.Invitations)-1], nil
}

func (m *MockTeamStore) GetInvitation(_ context.Context, invitationID string) (*coreteam.Invitation, error) {
	for i := range m.Invitations {
		if m.Invitations[i].ID == invitationID {
			return &m.Invitations[i], nil
		}
	}
	return nil, nil
}

func (m *MockTeamStore) ListPendingInvitationsByTeam(_ context.Context, teamID string, now time.Time) ([]coreteam.Invitation, error) {
	var out []coreteam.Invitation
	for _, inv := range m.Invitations {
		if inv.TeamID == teamID && inv.Pending(now) {
			out = append(out, inv)
		}
	}
	return out, nil
}

func (m *MockTeamStore) ListPendingInvitationsByUser(_ context.Context, userID string, now time.Time) ([]coreteam.Invitation, error) {
	var out []coreteam.Invitation
	for _, inv := range m.Invitations {
		if inv.UserID == userID && inv.Pending(now) {
			out = append(out, inv)
		}
	}
	return out, nil
}

func (m *MockTeamStore) AcceptInvitation(ctx context.Context, invitationID string, now time.Time) (*coreteam.Invitation, error) {
	for i := range m.Invitations {
		if m.Invitations[i].ID != invitationID {
			continue
		}
		if !m.Invitations[i].Pending(now) {
			return nil, nil
		}
		m.Invitations[i].AcceptedAt = &now
		if _, err := m.AddTeamMember(ctx, m.Invitations[i].TeamID, m.Invitations[i].UserID, m.Invitations[i].Role); err != nil {
			return nil, err
		}
		return &m.Invitations[i], nil
	}
	return nil, nil
}

func (m *MockTeamStore) RevokeInvitation(_ context.Context, invitationID string, now time.Time) error {
	for i := range m.Invitations {
		if m.Invitations[i].ID == invitationID && m.Invitations[i].Pending(now) {
			m.Invitations[i].RevokedAt = &now
			return nil
		}
	}
	return nil
}
