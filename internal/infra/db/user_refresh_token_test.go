package db

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// The row must never be able to yield a working credential, so this is worth
// pinning down without a database — every test below redeems by plaintext and
// would pass just as well if the plaintext were what got stored.
func TestHashRefreshTokenDoesNotStorePlaintext(t *testing.T) {
	const plaintext = "bmxrefresh_abc123"
	hash := hashRefreshToken(plaintext)
	if strings.Contains(hash, plaintext) || hash == plaintext {
		t.Fatalf("hash %q leaks the plaintext", hash)
	}
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64 hex characters", len(hash))
	}
	if hashRefreshToken(plaintext) != hash {
		t.Error("hashing is not deterministic")
	}
	if hashRefreshToken(plaintext+"x") == hash {
		t.Error("different tokens hash the same")
	}
}

func TestNewRefreshTokenPlaintextIsPrefixedAndUnique(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		plaintext, err := newRefreshTokenPlaintext()
		if err != nil {
			t.Fatalf("newRefreshTokenPlaintext: %v", err)
		}
		if !strings.HasPrefix(plaintext, refreshTokenPrefix) {
			t.Fatalf("token %q does not carry the %q prefix that makes a leak recognizable", plaintext, refreshTokenPrefix)
		}
		if seen[plaintext] {
			t.Fatalf("token %q issued twice", plaintext)
		}
		seen[plaintext] = true
	}
}

// newRefreshTokenSession issues a token and registers cleanup for the whole
// session it opens.
func newRefreshTokenSession(t *testing.T, s *Store, userID, sessionID string, ttl time.Duration) string {
	t.Helper()
	ctx := context.Background()
	// Not t.Context(): that one is cancelled before cleanups run, so the
	// delete would fail silently and leave rows for the next test to trip on.
	t.Cleanup(func() {
		if err := s.db.WithContext(context.Background()).
			Delete(&userRefreshTokenRow{}, "user_id = ?", userID).Error; err != nil {
			t.Errorf("cleanup: delete rows for %s: %v", userID, err)
		}
	})
	plaintext, expiresAt, err := s.CreateRefreshToken(ctx, model.NewRefreshToken{
		UserID:    userID,
		SessionID: sessionID,
		Platform:  "cli",
		TTL:       ttl,
	})
	if err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}
	if expiresAt <= time.Now().Unix() {
		t.Fatalf("expires_at %d is not in the future", expiresAt)
	}
	return plaintext
}

func TestRefreshTokenRotates(t *testing.T) {
	s, ctx := newTestStore(t)
	const userID, sessionID = "u_refreshrotate", "as_refreshrotate"
	plaintext := newRefreshTokenSession(t, s, userID, sessionID, time.Hour)

	// The plaintext must not be recoverable from the row.
	var row userRefreshTokenRow
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&row).Error; err != nil {
		t.Fatalf("read row: %v", err)
	}
	if row.TokenHash == plaintext {
		t.Error("the token is stored in plaintext")
	}

	now := time.Now().Unix()
	rotated, err := s.RotateRefreshToken(ctx, plaintext, now, time.Hour, 30*time.Second)
	if err != nil {
		t.Fatalf("RotateRefreshToken: %v", err)
	}
	if rotated.Plaintext == "" || rotated.Plaintext == plaintext {
		t.Fatalf("rotation returned %q, want a token different from the one presented", rotated.Plaintext)
	}
	if rotated.UserID != userID {
		t.Errorf("user id = %q, want %q", rotated.UserID, userID)
	}
	// Rotation stays inside the session, which is what makes a session
	// revocable as a unit no matter how many times it has been renewed.
	if rotated.SessionID != sessionID {
		t.Errorf("session id = %q, want %q", rotated.SessionID, sessionID)
	}

	// The replacement is usable in turn.
	if _, err := s.RotateRefreshToken(ctx, rotated.Plaintext, now, time.Hour, 30*time.Second); err != nil {
		t.Errorf("rotating the replacement: %v", err)
	}
}

// The whole reason rotation is worth its complexity: a token presented after it
// was spent means two holders, and neither keeps the session.
func TestRefreshTokenReuseRevokesTheSession(t *testing.T) {
	s, ctx := newTestStore(t)
	const userID, sessionID = "u_refreshreuse", "as_refreshreuse"
	stolen := newRefreshTokenSession(t, s, userID, sessionID, time.Hour)

	now := time.Now().Unix()
	const grace = 30 * time.Second
	legitimate, err := s.RotateRefreshToken(ctx, stolen, now, time.Hour, grace)
	if err != nil {
		t.Fatalf("first rotation: %v", err)
	}

	// Present the copy past the grace window. Time is a parameter here, so the
	// test states the elapsed time rather than waiting for it.
	later := now + int64(grace.Seconds()) + 1
	reused, err := s.RotateRefreshToken(ctx, stolen, later, time.Hour, grace)
	if !errors.Is(err, model.ErrRefreshTokenReused) {
		t.Fatalf("replaying a spent token: err = %v, want ErrRefreshTokenReused", err)
	}
	// The caller has to be able to record what was revoked.
	if reused.UserID != userID || reused.SessionID != sessionID {
		t.Errorf("reuse reported user %q session %q, want %q and %q",
			reused.UserID, reused.SessionID, userID, sessionID)
	}
	if reused.Plaintext != "" {
		t.Error("a reuse report handed back a usable token")
	}

	// The holder that did nothing wrong is signed out too. With two copies in
	// circulation there is no way to tell which one is the thief.
	if _, err := s.RotateRefreshToken(ctx, legitimate.Plaintext, later, time.Hour, grace); !errors.Is(err, model.ErrRefreshTokenInvalid) {
		t.Errorf("the session survived a reuse report: err = %v, want ErrRefreshTokenInvalid", err)
	}

	var live int64
	if err := s.db.WithContext(ctx).Model(&userRefreshTokenRow{}).
		Where("session_id = ? AND revoked_at IS NULL", sessionID).Count(&live).Error; err != nil {
		t.Fatalf("count live tokens: %v", err)
	}
	if live != 0 {
		t.Errorf("%d tokens in the session are still live after reuse", live)
	}
}

// Two processes sharing one credentials file refresh at the same moment. This
// is the case the grace window exists for: treating the second as a replay
// would sign a user out for running two commands at once.
func TestRefreshTokenConcurrentRotationsAllSucceed(t *testing.T) {
	s, ctx := newTestStore(t)
	const userID, sessionID = "u_refreshrace", "as_refreshrace"
	plaintext := newRefreshTokenSession(t, s, userID, sessionID, time.Hour)

	const attempts = 8
	now := time.Now().Unix()
	results := make([]model.RotatedRefreshToken, attempts)
	errs := make([]error, attempts)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i], errs[i] = s.RotateRefreshToken(ctx, plaintext, now, time.Hour, 30*time.Second)
		}()
	}
	close(start)
	wg.Wait()

	issued := make(map[string]bool)
	for i := range attempts {
		if errs[i] != nil {
			t.Errorf("attempt %d: %v", i, errs[i])
			continue
		}
		if issued[results[i].Plaintext] {
			t.Errorf("attempt %d received a token another attempt also received", i)
		}
		issued[results[i].Plaintext] = true
		if results[i].SessionID != sessionID {
			t.Errorf("attempt %d landed in session %q, want %q", i, results[i].SessionID, sessionID)
		}
	}
	if len(issued) != attempts {
		t.Errorf("%d of %d concurrent refreshes came away with a token", len(issued), attempts)
	}
}

func TestRefreshTokenRejectsExpiredRevokedAndUnknown(t *testing.T) {
	s, ctx := newTestStore(t)
	const userID = "u_refreshreject"
	now := time.Now().Unix()

	t.Run("expired", func(t *testing.T) {
		plaintext := newRefreshTokenSession(t, s, userID+"_exp", "as_refreshexp", time.Second)
		// Rotate as if the clock had passed the expiry, rather than sleeping.
		_, err := s.RotateRefreshToken(ctx, plaintext, now+3600, time.Hour, 30*time.Second)
		if !errors.Is(err, model.ErrRefreshTokenInvalid) {
			t.Errorf("err = %v, want ErrRefreshTokenInvalid", err)
		}
	})

	t.Run("revoked", func(t *testing.T) {
		const sessionID = "as_refreshrevoked"
		plaintext := newRefreshTokenSession(t, s, userID+"_rev", sessionID, time.Hour)
		n, err := s.RevokeSession(ctx, sessionID, now)
		if err != nil {
			t.Fatalf("RevokeSession: %v", err)
		}
		if n != 1 {
			t.Errorf("revoked %d tokens, want 1", n)
		}
		if _, err := s.RotateRefreshToken(ctx, plaintext, now, time.Hour, 30*time.Second); !errors.Is(err, model.ErrRefreshTokenInvalid) {
			t.Errorf("err = %v, want ErrRefreshTokenInvalid", err)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		if _, err := s.RotateRefreshToken(ctx, "bmxrefresh_never-issued", now, time.Hour, 30*time.Second); !errors.Is(err, model.ErrRefreshTokenInvalid) {
			t.Errorf("err = %v, want ErrRefreshTokenInvalid", err)
		}
	})

	t.Run("empty", func(t *testing.T) {
		if _, err := s.RotateRefreshToken(ctx, "", now, time.Hour, 30*time.Second); !errors.Is(err, model.ErrRefreshTokenInvalid) {
			t.Errorf("err = %v, want ErrRefreshTokenInvalid", err)
		}
	})
}

func TestRevokeRefreshTokenSession(t *testing.T) {
	s, ctx := newTestStore(t)
	const userID, sessionID = "u_refreshlogout", "as_refreshlogout"
	plaintext := newRefreshTokenSession(t, s, userID, sessionID, time.Hour)
	now := time.Now().Unix()

	// Renew once, so the session holds more than the token being presented.
	rotated, err := s.RotateRefreshToken(ctx, plaintext, now, time.Hour, 30*time.Second)
	if err != nil {
		t.Fatalf("RotateRefreshToken: %v", err)
	}

	gotUser, gotSession, err := s.RevokeRefreshTokenSession(ctx, rotated.Plaintext, now)
	if err != nil {
		t.Fatalf("RevokeRefreshTokenSession: %v", err)
	}
	if gotUser != userID || gotSession != sessionID {
		t.Errorf("revoked user %q session %q, want %q and %q", gotUser, gotSession, userID, sessionID)
	}
	if _, err := s.RotateRefreshToken(ctx, rotated.Plaintext, now, time.Hour, 30*time.Second); !errors.Is(err, model.ErrRefreshTokenInvalid) {
		t.Errorf("a revoked token still rotates: err = %v", err)
	}

	// Logging out something the store never knew is a success: the caller's
	// goal, that the token not work, already holds.
	gotUser, gotSession, err = s.RevokeRefreshTokenSession(ctx, "bmxrefresh_never-issued", now)
	if err != nil || gotUser != "" || gotSession != "" {
		t.Errorf("unknown token: (%q, %q, %v), want empty and no error", gotUser, gotSession, err)
	}
}

func TestDeleteExpiredRefreshTokens(t *testing.T) {
	s, ctx := newTestStore(t)
	const userID = "u_refreshsweep"
	plaintext := newRefreshTokenSession(t, s, userID, "as_refreshsweep", time.Hour)

	// A sweep at the current time leaves a live token alone. The counts this
	// call returns are database-wide, so the assertions are about this token
	// rather than about how many rows moved.
	if _, err := s.DeleteExpiredRefreshTokens(ctx, time.Now().Unix()); err != nil {
		t.Fatalf("DeleteExpiredRefreshTokens: %v", err)
	}
	if _, err := s.RotateRefreshToken(ctx, plaintext, time.Now().Unix(), time.Hour, 30*time.Second); err != nil {
		t.Fatalf("a live token was swept: %v", err)
	}

	// Past its expiry, it goes.
	n, err := s.DeleteExpiredRefreshTokens(ctx, time.Now().Add(2*time.Hour).Unix())
	if err != nil {
		t.Fatalf("DeleteExpiredRefreshTokens: %v", err)
	}
	if n < 1 {
		t.Errorf("swept %d tokens, want at least the expired one", n)
	}
	if _, err := s.RotateRefreshToken(ctx, plaintext, time.Now().Unix(), time.Hour, 30*time.Second); !errors.Is(err, model.ErrRefreshTokenInvalid) {
		t.Errorf("a swept token still rotates: err = %v", err)
	}
}
