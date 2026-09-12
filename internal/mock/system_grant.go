package mock

import (
	"context"
	"sort"
	"time"

	coreaudit "github.com/icloudbb/buildmax/internal/core/audit"
	coreidentity "github.com/icloudbb/buildmax/internal/core/identity"
)

// MockSystemGrantStore is an in-memory coreidentity.SystemGrantStore for tests.
type MockSystemGrantStore struct {
	Grants []coreidentity.SystemGrant
	// DisabledUsers marks accounts whose grants are not effective holders, so a
	// test can exercise the rule that a disabled account cannot be the holder
	// that keeps the deployment reachable. A user absent from the map is enabled.
	DisabledUsers map[string]bool
	// Err, when set, is returned by every read so a caller's behaviour on a
	// store failure can be exercised. An authorization check that fails open
	// on a database error is the bug worth having a test for.
	Err error
}

// GrantForTest adds an active grant without going through the validation
// GrantSystemRole applies. Test setup, not a store method.
func (m *MockSystemGrantStore) GrantForTest(userID, role string) {
	m.Grants = append(m.Grants, coreidentity.SystemGrant{
		ID:        "sg_" + userID + "_" + role,
		UserID:    userID,
		Role:      role,
		GrantedBy: coreaudit.ActorOperator,
		GrantedAt: seqTime(1),
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

func (m *MockSystemGrantStore) ListSystemGrants(_ context.Context, includeRevoked bool) ([]coreidentity.SystemGrant, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	var out []coreidentity.SystemGrant
	for _, g := range m.Grants {
		if includeRevoked || g.Active() {
			out = append(out, g)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].GrantedAt.After(out[j].GrantedAt) })
	return out, nil
}

func (m *MockSystemGrantStore) GrantSystemRole(_ context.Context, userID, role, grantedBy string, now time.Time) (*coreidentity.SystemGrant, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if !coreidentity.ValidSystemRole(role) {
		return nil, coreidentity.ErrSystemRoleUnknown
	}
	for _, g := range m.Grants {
		if g.UserID == userID && g.Role == role && g.Active() {
			return nil, coreidentity.ErrSystemGrantExists
		}
	}
	grant := coreidentity.SystemGrant{
		ID:        "sg_" + userID + "_" + role,
		UserID:    userID,
		Role:      role,
		GrantedBy: grantedBy,
		GrantedAt: now,
	}
	m.Grants = append(m.Grants, grant)
	return &grant, nil
}

func (m *MockSystemGrantStore) RevokeSystemRole(_ context.Context, userID, role string, now time.Time, keepLastHolder bool) (bool, error) {
	if m.Err != nil {
		return false, m.Err
	}
	idx := -1
	for i := range m.Grants {
		if m.Grants[i].UserID == userID && m.Grants[i].Role == role && m.Grants[i].Active() {
			idx = i
			break
		}
	}
	if idx == -1 {
		return false, nil
	}
	if keepLastHolder {
		// Effective holders left after this revoke, mirroring the store: an
		// active grant on an account that is not disabled, excluding the one
		// being revoked. None left means this was the last one.
		remaining := 0
		for i := range m.Grants {
			if i == idx {
				continue
			}
			g := m.Grants[i]
			if g.Role == role && g.Active() && !m.DisabledUsers[g.UserID] {
				remaining++
			}
		}
		if remaining == 0 {
			return false, coreidentity.ErrSystemGrantLastHolder
		}
	}
	revoked := now
	m.Grants[idx].RevokedAt = &revoked
	return true, nil
}

func (m *MockSystemGrantStore) CountActiveSystemGrants(_ context.Context, role string) (int, error) {
	if m.Err != nil {
		return 0, m.Err
	}
	n := 0
	for _, g := range m.Grants {
		if g.Role == role && g.Active() && !m.DisabledUsers[g.UserID] {
			n++
		}
	}
	return n, nil
}
