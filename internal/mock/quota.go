package mock

import (
	"context"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// MockUsageReader returns fixed run count and token total for TeamUsageInWindow.
type MockUsageReader struct {
	RunCount    int
	TotalTokens int
	Err         error
}

func (m *MockUsageReader) TeamUsageInWindow(_ context.Context, _ string, _, _ int64) (int, int, error) {
	if m.Err != nil {
		return 0, 0, m.Err
	}
	return m.RunCount, m.TotalTokens, nil
}

// MockTierStore returns a fixed tier for GetQuotaTier.
type MockTierStore struct {
	Tier *model.QuotaTier
	Err  error
}

func (m *MockTierStore) GetQuotaTier(_ context.Context, _ string) (*model.QuotaTier, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Tier, nil
}

// DenyQuotaTeamStore is used by quota 429 tests to supply a team with tier.
type DenyQuotaTeamStore struct {
	Team *model.Team
}

func (d *DenyQuotaTeamStore) GetTeam(_ context.Context, _ string) (*model.Team, error) {
	return d.Team, nil
}

func (d *DenyQuotaTeamStore) GetPersonalTeamByUser(_ context.Context, _ string) (*model.Team, error) {
	return d.Team, nil
}

func (d *DenyQuotaTeamStore) ListTeamsByUser(_ context.Context, _ string) ([]model.Team, error) {
	if d.Team == nil {
		return nil, nil
	}
	return []model.Team{*d.Team}, nil
}

func (d *DenyQuotaTeamStore) CreateTeam(_ context.Context, _, _, _ string) (*model.Team, error) {
	return nil, nil
}

func (d *DenyQuotaTeamStore) AddTeamMember(_ context.Context, _, _, _ string) (*model.TeamMember, error) {
	return nil, nil
}

func (d *DenyQuotaTeamStore) RemoveTeamMember(_ context.Context, _, _ string) error {
	return nil
}

func (d *DenyQuotaTeamStore) ListTeamMembers(_ context.Context, _ string) ([]model.TeamMember, error) {
	return nil, nil
}

// DenyQuotaUsageReader is used by quota 429 tests.
type DenyQuotaUsageReader struct {
	RunCount    int
	TotalTokens int
}

func (d *DenyQuotaUsageReader) TeamUsageInWindow(_ context.Context, _ string, _, _ int64) (int, int, error) {
	return d.RunCount, d.TotalTokens, nil
}

// DenyQuotaTierStore is used by quota 429 tests.
type DenyQuotaTierStore struct {
	Tier *model.QuotaTier
}

func (d *DenyQuotaTierStore) GetQuotaTier(_ context.Context, _ string) (*model.QuotaTier, error) {
	return d.Tier, nil
}

func (d *DenyQuotaTeamStore) ListAllTeams(_ context.Context, _ string, _, _ int) ([]model.Team, int, error) {
	if d.Team == nil {
		return nil, 0, nil
	}
	return []model.Team{*d.Team}, 1, nil
}

func (d *DenyQuotaTeamStore) CountTeamMembers(_ context.Context, _ []string) (map[string]int, error) {
	return nil, nil
}
