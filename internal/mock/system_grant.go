package mock

import (
	"context"
	"sort"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// MockSystemGrantStore is an in-memory model.SystemGrantStore for tests.
type MockSystemGrantStore struct {
	Grants []model.SystemGrant
	// Err, when set, is returned by every read so a caller's behaviour on a
	// store failure can be exercised. An authorization check that fails open
	// on a database error is the bug worth having a test for.
	Err error
}

// GrantForTest adds an active grant without going through the validation
// GrantSystemRole applies. Test setup, not a store method.
func (m *MockSystemGrantStore) GrantForTest(userID, role string) {
	m.Grants = append(m.Grants, model.SystemGrant{
		SystemGrantID: "sg_" + userID + "_" + role,
		UserID:        userID,
		Role:          role,
		GrantedBy:     model.AuditActorOperator,
		GrantedAt:     1,
	})
}

func (m *MockSystemGrantStore) ActiveSystemRoles(_ context.Context, userID string) ([]string, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	var out []string
	for _, g := range m.Grants {
		if g.UserID == userID && g.Active() {
			out = append(out, g.Role)
		}
	}
	return out, nil
}

func (m *MockSystemGrantStore) ListSystemGrants(_ context.Context, includeRevoked bool) ([]model.SystemGrant, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	var out []model.SystemGrant
	for _, g := range m.Grants {
		if includeRevoked || g.Active() {
			out = append(out, g)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].GrantedAt > out[j].GrantedAt })
	return out, nil
}

func (m *MockSystemGrantStore) GrantSystemRole(_ context.Context, userID, role, grantedBy string, now int64) (*model.SystemGrant, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if !model.ValidSystemRole(role) {
		return nil, model.ErrSystemRoleUnknown
	}
	for _, g := range m.Grants {
		if g.UserID == userID && g.Role == role && g.Active() {
			return nil, model.ErrSystemGrantExists
		}
	}
	grant := model.SystemGrant{
		SystemGrantID: "sg_" + userID + "_" + role,
		UserID:        userID,
		Role:          role,
		GrantedBy:     grantedBy,
		GrantedAt:     now,
	}
	m.Grants = append(m.Grants, grant)
	return &grant, nil
}

func (m *MockSystemGrantStore) RevokeSystemRole(_ context.Context, userID, role string, now int64) (bool, error) {
	if m.Err != nil {
		return false, m.Err
	}
	for i := range m.Grants {
		if m.Grants[i].UserID == userID && m.Grants[i].Role == role && m.Grants[i].Active() {
			revoked := now
			m.Grants[i].RevokedAt = &revoked
			return true, nil
		}
	}
	return false, nil
}

func (m *MockSystemGrantStore) CountActiveSystemGrants(_ context.Context, role string) (int, error) {
	if m.Err != nil {
		return 0, m.Err
	}
	n := 0
	for _, g := range m.Grants {
		if g.Role == role && g.Active() {
			n++
		}
	}
	return n, nil
}
