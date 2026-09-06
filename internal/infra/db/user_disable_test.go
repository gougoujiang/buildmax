package db

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	coreidentity "github.com/gougoujiang/buildmax/internal/core/identity"
)

func openUserAdminStore(t *testing.T) (*Store, context.Context) {
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

func createTestUser(t *testing.T, s *Store, ctx context.Context) *coreidentity.User {
	t.Helper()
	email := testPublicID(t) + "@example.com"
	user, err := s.CreateUser(ctx, email, "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	t.Cleanup(func() {
		if personal, _ := s.GetPersonalSpaceByUser(ctx, user.ID); personal != nil {
			_ = s.db.WithContext(ctx).Delete(&spaceMemberRow{}, "space_id = ?", personal.ID)
			_ = s.db.WithContext(ctx).Delete(&spaceRow{}, "space_id = ?", personal.ID)
		}
		_ = s.db.WithContext(ctx).Delete(&userRefreshTokenRow{}, "user_id = ?", user.ID)
		_ = s.db.WithContext(ctx).Delete(&userRow{}, "user_id = ?", user.ID)
	})
	return user
}

func TestSetUserDisabled(t *testing.T) {
	s, ctx := openUserAdminStore(t)
	user := createTestUser(t, s, ctx)

	if user.Disabled() {
		t.Fatal("a new account should not be disabled")
	}
	now := time.Now().UTC()
	if err := s.SetUserDisabled(ctx, user.ID, &now); err != nil {
		t.Fatalf("SetUserDisabled: %v", err)
	}
	got, err := s.GetUser(ctx, user.ID)
	if err != nil || got == nil || !got.Disabled() {
		t.Fatalf("after disable: %+v, %v", got, err)
	}

	// Disabling twice is not an error. Under MySQL's default client flags an
	// update to the value a row already holds affects no rows, which is why
	// the store has to tell that apart from a missing account.
	if err := s.SetUserDisabled(ctx, user.ID, got.DisabledAt); err != nil {
		t.Errorf("disabling an already disabled account: %v", err)
	}

	if err := s.SetUserDisabled(ctx, user.ID, nil); err != nil {
		t.Fatalf("enable: %v", err)
	}
	got, err = s.GetUser(ctx, user.ID)
	if err != nil || got == nil || got.Disabled() {
		t.Fatalf("after enable: %+v, %v", got, err)
	}

	if err := s.SetUserDisabled(ctx, testPublicID(t), &now); !errors.Is(err, coreidentity.ErrUserNotFound) {
		t.Errorf("disabling an unknown account = %v, want ErrUserNotFound", err)
	}
}

// TestListUsersFilters exercises each field of UserFilter against real SQL,
// including the system-role subquery a mock cannot stand in for. It asserts
// membership rather than counts: the store's database is shared, so other
// tests' accounts may also match.
func TestListUsersFilters(t *testing.T) {
	s, ctx := openUserAdminStore(t)
	now := time.Now().UTC()
	plain := createTestUser(t, s, ctx).ID
	disabled := createTestUser(t, s, ctx).ID
	onPortal := createTestUser(t, s, ctx).ID
	admin := createTestUser(t, s, ctx).ID

	if err := s.SetUserDisabled(ctx, disabled, &now); err != nil {
		t.Fatalf("SetUserDisabled: %v", err)
	}
	if err := s.UpdateLoginMeta(ctx, onPortal, now, "portal"); err != nil {
		t.Fatalf("UpdateLoginMeta: %v", err)
	}
	if _, err := s.GrantSystemRole(ctx, admin, coreidentity.SystemRoleAdmin, "u_admin", now); err != nil {
		t.Fatalf("GrantSystemRole: %v", err)
	}

	ids := func(f coreidentity.UserFilter) map[string]bool {
		rows, _, err := s.ListUsers(ctx, f, 10_000, 0)
		if err != nil {
			t.Fatalf("ListUsers(%+v): %v", f, err)
		}
		out := map[string]bool{}
		for _, u := range rows {
			out[u.ID] = true
		}
		return out
	}
	tru, fls := true, false

	if got := ids(coreidentity.UserFilter{Disabled: &tru}); !got[disabled] || got[plain] {
		t.Errorf("Disabled=true: disabled present=%v, plain present=%v; want true, false", got[disabled], got[plain])
	}
	if got := ids(coreidentity.UserFilter{Disabled: &fls}); got[disabled] || !got[plain] {
		t.Errorf("Disabled=false: disabled present=%v, plain present=%v; want false, true", got[disabled], got[plain])
	}
	if got := ids(coreidentity.UserFilter{Platform: "portal"}); !got[onPortal] || got[plain] {
		t.Errorf("Platform=portal: onPortal present=%v, plain present=%v; want true, false", got[onPortal], got[plain])
	}
	if got := ids(coreidentity.UserFilter{SystemRole: coreidentity.SystemRoleAdmin}); !got[admin] || got[plain] {
		t.Errorf("SystemRole=system_admin: admin present=%v, plain present=%v; want true, false", got[admin], got[plain])
	}
}

func TestListUsers(t *testing.T) {
	s, ctx := openUserAdminStore(t)
	user := createTestUser(t, s, ctx)

	users, total, err := s.ListUsers(ctx, coreidentity.UserFilter{Query: user.Email}, 50, 0)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if total != 1 || len(users) != 1 || users[0].ID != user.ID {
		t.Fatalf("searching for an exact email returned %d of %d: %+v", len(users), total, users)
	}

	// A substring of the local part finds it too — searching by the part
	// before the @ is the common case, and a prefix-only match would miss it.
	if _, total, err := s.ListUsers(ctx, coreidentity.UserFilter{Query: user.Email[3:9]}, 50, 0); err != nil || total == 0 {
		t.Errorf("substring search found nothing: %d, %v", total, err)
	}

	if _, total, err := s.ListUsers(ctx, coreidentity.UserFilter{Query: "no-such-address-anywhere"}, 50, 0); err != nil || total != 0 {
		t.Errorf("a search matching nothing returned %d, %v", total, err)
	}

	// An unfiltered page is bounded whatever the caller asks for.
	got, _, err := s.ListUsers(ctx, coreidentity.UserFilter{}, 10_000, 0)
	if err != nil {
		t.Fatalf("ListUsers unbounded: %v", err)
	}
	if len(got) > 200 {
		t.Errorf("limit was not clamped: got %d rows", len(got))
	}
}

func TestRevokeUserSessionsAndCount(t *testing.T) {
	s, ctx := openUserAdminStore(t)
	user := createTestUser(t, s, ctx)
	now := time.Now().UTC()

	for _, sessionID := range []string{"as_one", "as_two"} {
		if _, _, err := s.CreateRefreshToken(ctx, coreidentity.NewRefreshToken{
			UserID: user.ID, SessionID: sessionID, Platform: "portal", TTL: time.Hour,
		}); err != nil {
			t.Fatalf("CreateRefreshToken: %v", err)
		}
	}
	// A second token in an existing chain, as rotation leaves during the grace
	// window. It must not read as a third session.
	if _, _, err := s.CreateRefreshToken(ctx, coreidentity.NewRefreshToken{
		UserID: user.ID, SessionID: "as_one", Platform: "portal", TTL: time.Hour,
	}); err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}

	count, err := s.CountUserSessions(ctx, user.ID, now)
	if err != nil || count != 2 {
		t.Fatalf("CountUserSessions = %d, %v; want 2", count, err)
	}

	revoked, err := s.RevokeUserSessions(ctx, user.ID, now)
	if err != nil || revoked != 3 {
		t.Fatalf("RevokeUserSessions = %d, %v; want 3 tokens", revoked, err)
	}
	count, err = s.CountUserSessions(ctx, user.ID, now)
	if err != nil || count != 0 {
		t.Fatalf("after revoke CountUserSessions = %d, %v; want 0", count, err)
	}
	// Revoking again retires nothing rather than failing.
	if revoked, err := s.RevokeUserSessions(ctx, user.ID, now); err != nil || revoked != 0 {
		t.Errorf("second revoke = %d, %v; want 0, nil", revoked, err)
	}
}
