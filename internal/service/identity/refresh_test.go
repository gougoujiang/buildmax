package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	coreidentity "github.com/gougoujiang/buildmax/internal/core/identity"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/identity"
)

func newRefreshService(t *testing.T, user *coreidentity.User) (*identity.Service, *mock.MockRefreshTokenStore, string) {
	t.Helper()
	tokens := &mock.MockRefreshTokenStore{}
	svc := &identity.Service{
		Users: &mock.MockUserStore{
			ByEmail: map[string]*coreidentity.User{user.Email: user},
			ByID:    map[string]*coreidentity.User{user.ID: user},
		},
		RefreshTokens: tokens,
		Tokens:        fixedIssuer{},
		Sessions:      fixedSessions{},
		RefreshTTL:    time.Hour,
	}
	plaintext, _, err := tokens.CreateRefreshToken(context.Background(), coreidentity.NewRefreshToken{
		UserID: user.ID, SessionID: "sn_1", Platform: "cli", TTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}
	return svc, tokens, plaintext
}

// TestARefusedRefreshRevokesTheSession is the part a transport cannot check.
//
// Three of these refusals answer the same 401, and the handler's tests check
// that. What they cannot check is that the session was ended on the way out --
// and that is the point of refusing: a token presented after rotation means a
// copy exists, and an account that is gone or switched off outranks a row that
// still rotates. Answering without revoking leaves the copy working.
func TestARefusedRefreshRevokesTheSession(t *testing.T) {
	t.Run("a deleted account", func(t *testing.T) {
		user := &coreidentity.User{ID: "u_gone", Email: "gone@example.test"}
		svc, tokens, plaintext := newRefreshService(t, user)
		// The account disappears between issuing and refreshing.
		svc.Users.(*mock.MockUserStore).ByID = map[string]*coreidentity.User{}

		_, err := svc.Refresh(context.Background(), plaintext)
		var invalid *identity.InvalidRefresh
		if !errors.As(err, &invalid) {
			t.Fatalf("err = %v, want *InvalidRefresh", err)
		}
		if n, _ := tokens.CountUserSessions(context.Background(), user.ID, time.Now()); n != 0 {
			t.Errorf("%d session(s) survived a refusal; the session must end with it", n)
		}
	})

	t.Run("a disabled account", func(t *testing.T) {
		disabled := time.Now().UTC()
		user := &coreidentity.User{ID: "u_off", Email: "off@example.test", DisabledAt: &disabled}
		svc, tokens, plaintext := newRefreshService(t, user)

		if _, err := svc.Refresh(context.Background(), plaintext); !errors.Is(err, identity.ErrDisabled) {
			t.Fatalf("err = %v, want ErrDisabled", err)
		}
		if n, _ := tokens.CountUserSessions(context.Background(), user.ID, time.Now()); n != 0 {
			t.Errorf("%d session(s) survived a disabled account's refresh", n)
		}
	})
}

// TestAReusedTokenIsMarkedAsSuch keeps the one alarm separable from ordinary
// failure. Every refusal reads the same to a caller; only this one is worth
// waking somebody for, so the error has to say which it is.
func TestAReusedTokenIsMarkedAsSuch(t *testing.T) {
	user := &coreidentity.User{ID: "u_1", Email: "a@example.test"}
	svc, tokens, plaintext := newRefreshService(t, user)

	if _, err := svc.Refresh(context.Background(), plaintext); err != nil {
		t.Fatalf("the first refresh failed: %v", err)
	}
	// Age the spent token past the grace window rather than relying on the
	// clock to have moved. Two calls in a row land on the same instant where
	// the clock is coarse -- Windows resolves to about 15ms -- and a rotation
	// inside the grace window is a concurrent refresh, not a reused token.
	if !tokens.Backdate(plaintext, time.Minute) {
		t.Fatal("the spent token was not there to age")
	}
	_, err := svc.Refresh(context.Background(), plaintext)
	var invalid *identity.InvalidRefresh
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want *InvalidRefresh", err)
	}
	if !invalid.Reused {
		t.Error("a token presented after rotation was not marked reused")
	}
	if invalid.Error() != "invalid refresh token" {
		t.Errorf("Error() = %q; a reused token must read like any other failure", invalid.Error())
	}
}

// TestAnUnknownTokenIsNotMarkedReused keeps the alarm from firing on noise.
func TestAnUnknownTokenIsNotMarkedReused(t *testing.T) {
	user := &coreidentity.User{ID: "u_1", Email: "a@example.test"}
	svc, _, _ := newRefreshService(t, user)

	_, err := svc.Refresh(context.Background(), "rt_never_existed")
	var invalid *identity.InvalidRefresh
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want *InvalidRefresh", err)
	}
	if invalid.Reused {
		t.Error("a token that never existed was reported as reuse")
	}
}
