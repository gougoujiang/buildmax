package db

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
)

// hashLoginCode is the only thing this package stores, so it is worth pinning
// down without a database: a change that started storing plaintext would still
// pass every integration test below, since they only ever redeem by plaintext.
func TestHashLoginCodeDoesNotStorePlaintext(t *testing.T) {
	const plaintext = "bmxlogin_abc123"
	hash := hashLoginCode(plaintext)
	if strings.Contains(hash, plaintext) || hash == plaintext {
		t.Fatalf("hash %q leaks the plaintext", hash)
	}
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64 hex characters", len(hash))
	}
	if hashLoginCode(plaintext) != hash {
		t.Error("hashing is not deterministic")
	}
	if hashLoginCode(plaintext+"x") == hash {
		t.Error("different codes hash the same")
	}
}

func newTestStore(t *testing.T) (*Store, context.Context) {
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
	t.Cleanup(func() { _ = s.Close() })
	return s, ctx
}

func TestLoginCodeLifecycle(t *testing.T) {
	s, ctx := newTestStore(t)
	const userID = "u_logincodetest"
	t.Cleanup(func() {
		_ = s.db.WithContext(ctx).Delete(&loginCodeRow{}, "user_id = ?", userID)
	})

	code, expiresAt, err := s.CreateLoginCode(ctx, userID, time.Hour)
	if err != nil {
		t.Fatalf("CreateLoginCode: %v", err)
	}
	if !strings.HasPrefix(code, loginCodePrefix) {
		t.Errorf("code %q does not carry the %q prefix that makes a leak recognizable", code, loginCodePrefix)
	}
	if expiresAt <= time.Now().Unix() {
		t.Errorf("expires_at %d is not in the future", expiresAt)
	}

	// The plaintext must not be recoverable from the row.
	var row loginCodeRow
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&row).Error; err != nil {
		t.Fatalf("read row: %v", err)
	}
	if row.CodeHash == code {
		t.Error("the code is stored in plaintext")
	}

	got, err := s.ConsumeLoginCode(ctx, code, time.Now().Unix())
	if err != nil {
		t.Fatalf("ConsumeLoginCode: %v", err)
	}
	if got != userID {
		t.Fatalf("ConsumeLoginCode = %q, want %q", got, userID)
	}

	// Single use: the second redemption resolves to nobody, without an error.
	got, err = s.ConsumeLoginCode(ctx, code, time.Now().Unix())
	if err != nil {
		t.Fatalf("second ConsumeLoginCode: %v", err)
	}
	if got != "" {
		t.Errorf("a spent code redeemed again as %q", got)
	}
}

func TestLoginCodeRejectsExpiredAndUnknown(t *testing.T) {
	s, ctx := newTestStore(t)
	const userID = "u_logincodeexpired"
	t.Cleanup(func() {
		_ = s.db.WithContext(ctx).Delete(&loginCodeRow{}, "user_id = ?", userID)
	})

	code, expiresAt, err := s.CreateLoginCode(ctx, userID, time.Second)
	if err != nil {
		t.Fatalf("CreateLoginCode: %v", err)
	}
	// Redeem as if the clock had passed the expiry, rather than sleeping.
	got, err := s.ConsumeLoginCode(ctx, code, expiresAt+1)
	if err != nil {
		t.Fatalf("ConsumeLoginCode: %v", err)
	}
	if got != "" {
		t.Errorf("an expired code redeemed as %q", got)
	}

	got, err = s.ConsumeLoginCode(ctx, "bmxlogin_never-issued", time.Now().Unix())
	if err != nil {
		t.Fatalf("ConsumeLoginCode on unknown code: %v", err)
	}
	if got != "" {
		t.Errorf("an unknown code redeemed as %q", got)
	}
}

// Two browsers submitting the same code at once must produce exactly one
// session. This is the reason redemption is a conditional UPDATE rather than a
// read followed by a write.
func TestLoginCodeConcurrentRedemptionHasOneWinner(t *testing.T) {
	s, ctx := newTestStore(t)
	const userID = "u_logincoderace"
	t.Cleanup(func() {
		_ = s.db.WithContext(ctx).Delete(&loginCodeRow{}, "user_id = ?", userID)
	})

	code, _, err := s.CreateLoginCode(ctx, userID, time.Hour)
	if err != nil {
		t.Fatalf("CreateLoginCode: %v", err)
	}

	const attempts = 8
	results := make([]string, attempts)
	errs := make([]error, attempts)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i], errs[i] = s.ConsumeLoginCode(ctx, code, time.Now().Unix())
		}()
	}
	close(start)
	wg.Wait()

	winners := 0
	for i := range attempts {
		if errs[i] != nil {
			t.Errorf("attempt %d: %v", i, errs[i])
			continue
		}
		if results[i] == userID {
			winners++
		}
	}
	if winners != 1 {
		t.Errorf("%d concurrent redemptions succeeded, want exactly 1", winners)
	}
}
