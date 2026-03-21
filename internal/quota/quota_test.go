package quota

import (
	"context"
	"testing"

	"buildmax/internal/storage/entity"
)

type mockUserStore struct {
	user *entity.User
	err  error
}

func (m *mockUserStore) UserByEmail(_ context.Context, _ string) (*entity.User, error) {
	return nil, nil
}

func (m *mockUserStore) GetUser(_ context.Context, _ string) (*entity.User, error) {
	return m.user, m.err
}

func (m *mockUserStore) CreateUser(_ context.Context, _ string, _ string) (*entity.User, error) {
	return nil, nil
}

func (m *mockUserStore) UpdateLoginMeta(_ context.Context, _ string, _ int64, _ string) error {
	return nil
}

type mockUsageReader struct {
	runCount, totalTokens int
	err                   error
}

func (m *mockUsageReader) UserUsageInWindow(_ context.Context, _ string, _, _ int64) (runCount, totalTokens int, err error) {
	return m.runCount, m.totalTokens, m.err
}

type mockTierStore struct {
	tier *entity.QuotaTier
	err  error
}

func (m *mockTierStore) GetQuotaTier(_ context.Context, _ string) (*entity.QuotaTier, error) {
	return m.tier, m.err
}

func TestCheck_NoUser_Allows(t *testing.T) {
	c := NewChecker(&mockUserStore{user: nil}, &mockUsageReader{}, &mockTierStore{}, "free_trial")
	allowed, _ := c.Check(context.Background(), "u_any", 1, 0)
	if !allowed {
		t.Error("Check: expected allow when user is nil")
	}
}

func TestCheck_UnknownTier_Allows(t *testing.T) {
	c := NewChecker(
		&mockUserStore{user: &entity.User{UserID: "u_1", QuotaTier: "unknown"}},
		&mockUsageReader{runCount: 0, totalTokens: 0},
		&mockTierStore{tier: nil},
		"free_trial",
	)
	allowed, _ := c.Check(context.Background(), "u_1", 1, 0)
	if !allowed {
		t.Error("Check: expected allow when tier not found")
	}
}

func TestCheck_RunLimitExceeded_Denies(t *testing.T) {
	c := NewChecker(
		&mockUserStore{user: &entity.User{UserID: "u_1", QuotaTier: "free_trial"}},
		&mockUsageReader{runCount: 10, totalTokens: 0},
		&mockTierStore{tier: &entity.QuotaTier{TierName: "free_trial", MaxRunsPerPeriod: 10, MaxTokensPerPeriod: 100000, PeriodDays: 30}},
		"free_trial",
	)
	allowed, reason := c.Check(context.Background(), "u_1", 1, 0)
	if allowed {
		t.Error("Check: expected deny when run count would exceed limit")
	}
	if reason != "quota exceeded: run limit" {
		t.Errorf("Check: reason = %q, want quota exceeded: run limit", reason)
	}
}

func TestCheck_TokenLimitExceeded_Denies(t *testing.T) {
	c := NewChecker(
		&mockUserStore{user: &entity.User{UserID: "u_1", QuotaTier: "free_trial"}},
		&mockUsageReader{runCount: 0, totalTokens: 100000},
		&mockTierStore{tier: &entity.QuotaTier{TierName: "free_trial", MaxRunsPerPeriod: 10, MaxTokensPerPeriod: 100000, PeriodDays: 30}},
		"free_trial",
	)
	allowed, reason := c.Check(context.Background(), "u_1", 0, 1)
	if allowed {
		t.Error("Check: expected deny when token count would exceed limit")
	}
	if reason != "quota exceeded: token limit" {
		t.Errorf("Check: reason = %q, want quota exceeded: token limit", reason)
	}
}

func TestCheck_UnderLimit_Allows(t *testing.T) {
	c := NewChecker(
		&mockUserStore{user: &entity.User{UserID: "u_1", QuotaTier: "free_trial"}},
		&mockUsageReader{runCount: 5, totalTokens: 50000},
		&mockTierStore{tier: &entity.QuotaTier{TierName: "free_trial", MaxRunsPerPeriod: 10, MaxTokensPerPeriod: 100000, PeriodDays: 30}},
		"free_trial",
	)
	allowed, reason := c.Check(context.Background(), "u_1", 1, 10000)
	if !allowed {
		t.Errorf("Check: expected allow; reason = %q", reason)
	}
	if reason != "" {
		t.Errorf("Check: reason = %q, want empty", reason)
	}
}

func TestCheck_EmptyUserTier_UsesDefault(t *testing.T) {
	c := NewChecker(
		&mockUserStore{user: &entity.User{UserID: "u_1", QuotaTier: ""}},
		&mockUsageReader{runCount: 10, totalTokens: 0},
		&mockTierStore{tier: &entity.QuotaTier{TierName: "free_trial", MaxRunsPerPeriod: 10, MaxTokensPerPeriod: 100000, PeriodDays: 30}},
		"free_trial",
	)
	allowed, _ := c.Check(context.Background(), "u_1", 1, 0)
	if allowed {
		t.Error("Check: expected deny when default tier limit reached")
	}
}
