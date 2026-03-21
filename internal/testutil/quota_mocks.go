package testutil

import (
	"context"

	"buildmax/internal/storage/entity"
)

// MockUsageReader returns fixed run count and token total for UserUsageInWindow.
type MockUsageReader struct {
	RunCount    int
	TotalTokens int
	Err         error
}

func (m *MockUsageReader) UserUsageInWindow(_ context.Context, _ string, _, _ int64) (int, int, error) {
	if m.Err != nil {
		return 0, 0, m.Err
	}
	return m.RunCount, m.TotalTokens, nil
}

// MockTierStore returns a fixed tier for GetQuotaTier.
type MockTierStore struct {
	Tier *entity.QuotaTier
	Err  error
}

func (m *MockTierStore) GetQuotaTier(_ context.Context, _ string) (*entity.QuotaTier, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Tier, nil
}

// DenyQuotaUserStore is used by quota 429 tests to supply a user with tier.
type DenyQuotaUserStore struct {
	User *entity.User
}

func (d *DenyQuotaUserStore) UserByEmail(_ context.Context, _ string) (*entity.User, error) {
	return nil, nil
}
func (d *DenyQuotaUserStore) GetUser(_ context.Context, _ string) (*entity.User, error) {
	return d.User, nil
}
func (d *DenyQuotaUserStore) CreateUser(_ context.Context, _, _ string) (*entity.User, error) {
	return nil, nil
}
func (d *DenyQuotaUserStore) UpdateLoginMeta(_ context.Context, _ string, _ int64, _ string) error {
	return nil
}

// DenyQuotaUsageReader is used by quota 429 tests.
type DenyQuotaUsageReader struct {
	RunCount    int
	TotalTokens int
}

func (d *DenyQuotaUsageReader) UserUsageInWindow(_ context.Context, _ string, _, _ int64) (int, int, error) {
	return d.RunCount, d.TotalTokens, nil
}

// DenyQuotaTierStore is used by quota 429 tests.
type DenyQuotaTierStore struct {
	Tier *entity.QuotaTier
}

func (d *DenyQuotaTierStore) GetQuotaTier(_ context.Context, _ string) (*entity.QuotaTier, error) {
	return d.Tier, nil
}
