package db

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
	coreidentity "github.com/gougoujiang/buildmax/internal/core/identity"
)

// The store is the only implementation the server wires in, so a signature
// drift here should stop the build rather than surface as a nil interface at
// startup.
var _ coreidentity.SystemGrantStore = (*Store)(nil)

func openGrantStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	dsn := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(config.EnvKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, ctx
}

// TestSystemGrantLifecycle is the lockout exercise from
// docs/design/system-administration.md section 11, at the store level: an
// authority can be granted, seen, revoked, and granted again, and the count
// that guards the last grant tracks it throughout.
func TestSystemGrantLifecycle(t *testing.T) {
	s, ctx := openGrantStore(t)
	userID := newTestUser(t, s, "grant")

	before, err := s.CountActiveSystemGrants(ctx, coreidentity.SystemRoleAdmin)
	if err != nil {
		t.Fatalf("CountActiveSystemGrants: %v", err)
	}

	grant, err := s.GrantSystemRole(ctx, userID, coreidentity.SystemRoleAdmin, coreaudit.ActorOperator, time.Unix(100, 0).UTC())
	if err != nil {
		t.Fatalf("GrantSystemRole: %v", err)
	}
	if grant.ID == "" || grant.RevokedAt != nil || !grant.Active() {
		t.Fatalf("new grant should be active with an id: %+v", grant)
	}

	roles, err := s.ActiveSystemRoles(ctx, userID)
	if err != nil {
		t.Fatalf("ActiveSystemRoles: %v", err)
	}
	if len(roles) != 1 || roles[0] != coreidentity.SystemRoleAdmin {
		t.Fatalf("ActiveSystemRoles = %v, want [%s]", roles, coreidentity.SystemRoleAdmin)
	}

	if n, err := s.CountActiveSystemGrants(ctx, coreidentity.SystemRoleAdmin); err != nil || n != before+1 {
		t.Fatalf("CountActiveSystemGrants = %d, %v; want %d", n, err, before+1)
	}

	// A second grant of a role already held is refused rather than silently
	// creating a second row, so a caller can say "already an admin".
	if _, err := s.GrantSystemRole(ctx, userID, coreidentity.SystemRoleAdmin, "u_other", time.Unix(101, 0).UTC()); !errors.Is(err, coreidentity.ErrSystemGrantExists) {
		t.Fatalf("second grant err = %v, want ErrSystemGrantExists", err)
	}

	found, err := s.RevokeSystemRole(ctx, userID, coreidentity.SystemRoleAdmin, time.Unix(200, 0).UTC())
	if err != nil || !found {
		t.Fatalf("RevokeSystemRole = %v, %v; want true, nil", found, err)
	}
	roles, err = s.ActiveSystemRoles(ctx, userID)
	if err != nil || len(roles) != 0 {
		t.Fatalf("after revoke ActiveSystemRoles = %v, %v; want empty", roles, err)
	}
	if n, err := s.CountActiveSystemGrants(ctx, coreidentity.SystemRoleAdmin); err != nil || n != before {
		t.Fatalf("after revoke count = %d, %v; want %d", n, err, before)
	}

	// Revoking again is not an error: the end state is what was asked for.
	if found, err := s.RevokeSystemRole(ctx, userID, coreidentity.SystemRoleAdmin, time.Unix(201, 0).UTC()); err != nil || found {
		t.Fatalf("second revoke = %v, %v; want false, nil", found, err)
	}

	// The revoked row stays, and a re-grant does not collide with it. This is
	// what the (user_id, role, revoked_at) unique index has to allow.
	regrant, err := s.GrantSystemRole(ctx, userID, coreidentity.SystemRoleAdmin, "u_admin", time.Unix(300, 0).UTC())
	if err != nil {
		t.Fatalf("re-grant after revoke: %v", err)
	}
	if regrant.ID == grant.ID {
		t.Errorf("re-grant reused the retired row's id %q", regrant.ID)
	}

	all, err := s.ListSystemGrants(ctx, true)
	if err != nil {
		t.Fatalf("ListSystemGrants: %v", err)
	}
	live, retired := 0, 0
	for _, g := range all {
		if g.UserID != userID {
			continue
		}
		if g.Active() {
			live++
		} else {
			retired++
		}
	}
	if live != 1 || retired != 1 {
		t.Errorf("history for %s: %d live, %d retired; want 1 and 1", userID, live, retired)
	}

	active, err := s.ListSystemGrants(ctx, false)
	if err != nil {
		t.Fatalf("ListSystemGrants(false): %v", err)
	}
	for _, g := range active {
		if !g.Active() {
			t.Errorf("ListSystemGrants(false) returned a revoked grant: %+v", g)
		}
	}
}

// TestGrantSystemRoleRejectsUnknownRole pins the decision that the role column
// exists for a future role but accepts only roles this build authorizes. A
// store that took any string would make the column a way to invent authority.
func TestGrantSystemRoleRejectsUnknownRole(t *testing.T) {
	s, ctx := openGrantStore(t)
	userID := newTestUser(t, s, "grant")

	if _, err := s.GrantSystemRole(ctx, userID, "system_observer", coreaudit.ActorOperator, time.Unix(100, 0).UTC()); !errors.Is(err, coreidentity.ErrSystemRoleUnknown) {
		t.Fatalf("GrantSystemRole(system_observer) err = %v, want ErrSystemRoleUnknown", err)
	}
	roles, err := s.ActiveSystemRoles(ctx, userID)
	if err != nil || len(roles) != 0 {
		t.Fatalf("ActiveSystemRoles = %v, %v; want empty", roles, err)
	}
}

// TestActiveSystemRolesIsEmptyForOrdinaryUsers is the case that runs on every
// authenticated request: almost nobody holds a grant, and the answer must be
// empty rather than an error.
func TestActiveSystemRolesIsEmptyForOrdinaryUsers(t *testing.T) {
	s, ctx := openGrantStore(t)
	roles, err := s.ActiveSystemRoles(ctx, testPublicID(t))
	if err != nil {
		t.Fatalf("ActiveSystemRoles: %v", err)
	}
	if len(roles) != 0 {
		t.Errorf("ActiveSystemRoles for an ungranted user = %v, want empty", roles)
	}
	if roles, err := s.ActiveSystemRoles(ctx, ""); err != nil || len(roles) != 0 {
		t.Errorf("ActiveSystemRoles(\"\") = %v, %v; want empty, nil", roles, err)
	}
}
