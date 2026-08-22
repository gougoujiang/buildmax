package db

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/util"
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

func createTestUser(t *testing.T, s *Store, ctx context.Context) *model.User {
	t.Helper()
	email := util.NewPrefixedID("disabletest") + "@example.com"
	user, err := s.CreateUser(ctx, email, "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	t.Cleanup(func() {
		if personal, _ := s.GetPersonalTeamByUser(ctx, user.ID); personal != nil {
			_ = s.db.WithContext(ctx).Delete(&teamMemberRow{}, "team_id = ?", personal.ID)
			_ = s.db.WithContext(ctx).Delete(&teamRow{}, "team_id = ?", personal.ID)
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
	now := time.Now().Unix()
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

	if err := s.SetUserDisabled(ctx, util.NewPrefixedID(util.PrefixUser), &now); !errors.Is(err, model.ErrUserNotFound) {
		t.Errorf("disabling an unknown account = %v, want ErrUserNotFound", err)
	}
}

func TestListUsers(t *testing.T) {
	s, ctx := openUserAdminStore(t)
	user := createTestUser(t, s, ctx)

	users, total, err := s.ListUsers(ctx, user.Email, 50, 0)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if total != 1 || len(users) != 1 || users[0].ID != user.ID {
		t.Fatalf("searching for an exact email returned %d of %d: %+v", len(users), total, users)
	}

	// A substring of the local part finds it too — searching by the part
	// before the @ is the common case, and a prefix-only match would miss it.
	if _, total, err := s.ListUsers(ctx, user.Email[3:9], 50, 0); err != nil || total == 0 {
		t.Errorf("substring search found nothing: %d, %v", total, err)
	}

	if _, total, err := s.ListUsers(ctx, "no-such-address-anywhere", 50, 0); err != nil || total != 0 {
		t.Errorf("a search matching nothing returned %d, %v", total, err)
	}

	// An unfiltered page is bounded whatever the caller asks for.
	got, _, err := s.ListUsers(ctx, "", 10_000, 0)
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
	now := time.Now().Unix()

	for _, sessionID := range []string{"as_one", "as_two"} {
		if _, _, err := s.CreateRefreshToken(ctx, model.NewRefreshToken{
			UserID: user.ID, SessionID: sessionID, Platform: "portal", TTL: time.Hour,
		}); err != nil {
			t.Fatalf("CreateRefreshToken: %v", err)
		}
	}
	// A second token in an existing chain, as rotation leaves during the grace
	// window. It must not read as a third session.
	if _, _, err := s.CreateRefreshToken(ctx, model.NewRefreshToken{
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
