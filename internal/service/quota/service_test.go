package quota

import (
	"context"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"
)

type mockTeamStore struct {
	team *model.Team
	err  error
}

func (m *mockTeamStore) GetTeam(_ context.Context, _ string) (*model.Team, error) {
	return m.team, m.err
}

func (m *mockTeamStore) GetPersonalTeamByUser(_ context.Context, _ string) (*model.Team, error) {
	return nil, nil
}

func (m *mockTeamStore) ListTeamsByUser(_ context.Context, _ string) ([]model.Team, error) {
	return nil, nil
}

func (m *mockTeamStore) CreateTeam(_ context.Context, _, _, _ string) (*model.Team, error) {
	return nil, nil
}

func (m *mockTeamStore) AddTeamMember(_ context.Context, _, _, _ string) (*model.TeamMember, error) {
	return nil, nil
}

func (m *mockTeamStore) RemoveTeamMember(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockTeamStore) ListAllTeams(_ context.Context, _ string, _, _ int) ([]model.Team, int, error) {
	return nil, 0, nil
}

func (m *mockTeamStore) CountTeamMembers(_ context.Context, _ []string) (map[string]int, error) {
	return nil, nil
}

func (m *mockTeamStore) ListTeamMembers(_ context.Context, _ string) ([]model.TeamMember, error) {
	return nil, nil
}

type mockUsageReader struct {
	runCount, totalTokens int
	err                   error
}

func (m *mockUsageReader) TeamUsageInWindow(_ context.Context, _ string, _, _ time.Time) (runCount, totalTokens int, err error) {
	return m.runCount, m.totalTokens, m.err
}

type mockTierStore struct {
	tier *model.QuotaTier
	err  error
}

func (m *mockTierStore) GetQuotaTier(_ context.Context, _ string) (*model.QuotaTier, error) {
	return m.tier, m.err
}

func TestCheck_NoTeam_Allows(t *testing.T) {
	c := &Service{
		TeamStore:   &mockTeamStore{team: nil},
		UsageReader: &mockUsageReader{},
		TierStore:   &mockTierStore{},
		DefaultTier: "free_trial",
	}
	allowed, _ := c.Check(context.Background(), "tm_any", 1, 0)
	if !allowed {
		t.Error("Check: expected allow when team is nil")
	}
}

func TestCheck_UnknownTier_Allows(t *testing.T) {
	c := &Service{
		TeamStore:   &mockTeamStore{team: &model.Team{ID: "tm_1", QuotaTier: "unknown"}},
		UsageReader: &mockUsageReader{runCount: 0, totalTokens: 0},
		TierStore:   &mockTierStore{tier: nil},
		DefaultTier: "free_trial",
	}
	allowed, _ := c.Check(context.Background(), "tm_1", 1, 0)
	if !allowed {
		t.Error("Check: expected allow when tier not found")
	}
}

func TestCheck_RunLimitExceeded_Denies(t *testing.T) {
	c := &Service{
		TeamStore:   &mockTeamStore{team: &model.Team{ID: "tm_1", QuotaTier: "free_trial"}},
		UsageReader: &mockUsageReader{runCount: 10, totalTokens: 0},
		TierStore:   &mockTierStore{tier: &model.QuotaTier{TierName: "free_trial", MaxRunsPerPeriod: 10, MaxTokensPerPeriod: 100000, PeriodDays: 30}},
		DefaultTier: "free_trial",
	}
	allowed, reason := c.Check(context.Background(), "tm_1", 1, 0)
	if allowed {
		t.Error("Check: expected deny when run count would exceed limit")
	}
	if reason != "quota exceeded: run limit" {
		t.Errorf("Check: reason = %q, want quota exceeded: run limit", reason)
	}
}

func TestCheck_TokenLimitExceeded_Denies(t *testing.T) {
	c := &Service{
		TeamStore:   &mockTeamStore{team: &model.Team{ID: "tm_1", QuotaTier: "free_trial"}},
		UsageReader: &mockUsageReader{runCount: 0, totalTokens: 100000},
		TierStore:   &mockTierStore{tier: &model.QuotaTier{TierName: "free_trial", MaxRunsPerPeriod: 10, MaxTokensPerPeriod: 100000, PeriodDays: 30}},
		DefaultTier: "free_trial",
	}
	allowed, reason := c.Check(context.Background(), "tm_1", 0, 1)
	if allowed {
		t.Error("Check: expected deny when token count would exceed limit")
	}
	if reason != "quota exceeded: token limit" {
		t.Errorf("Check: reason = %q, want quota exceeded: token limit", reason)
	}
}

func TestCheck_UnderLimit_Allows(t *testing.T) {
	c := &Service{
		TeamStore:   &mockTeamStore{team: &model.Team{ID: "tm_1", QuotaTier: "free_trial"}},
		UsageReader: &mockUsageReader{runCount: 5, totalTokens: 50000},
		TierStore:   &mockTierStore{tier: &model.QuotaTier{TierName: "free_trial", MaxRunsPerPeriod: 10, MaxTokensPerPeriod: 100000, PeriodDays: 30}},
		DefaultTier: "free_trial",
	}
	allowed, reason := c.Check(context.Background(), "tm_1", 1, 10000)
	if !allowed {
		t.Errorf("Check: expected allow; reason = %q", reason)
	}
	if reason != "" {
		t.Errorf("Check: reason = %q, want empty", reason)
	}
}

func TestCheck_EmptyTeamTier_UsesDefault(t *testing.T) {
	c := &Service{
		TeamStore:   &mockTeamStore{team: &model.Team{ID: "tm_1", QuotaTier: ""}},
		UsageReader: &mockUsageReader{runCount: 10, totalTokens: 0},
		TierStore:   &mockTierStore{tier: &model.QuotaTier{TierName: "free_trial", MaxRunsPerPeriod: 10, MaxTokensPerPeriod: 100000, PeriodDays: 30}},
		DefaultTier: "free_trial",
	}
	allowed, _ := c.Check(context.Background(), "tm_1", 1, 0)
	if allowed {
		t.Error("Check: expected deny when default tier limit reached")
	}
}

func (m *mockTeamStore) SetTeamPluginCuration(_ context.Context, _ string, _ coreplugin.Curation) error {
	return nil
}
