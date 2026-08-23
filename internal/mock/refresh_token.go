package mock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// MockRefreshToken is one issued token in MockRefreshTokenStore.
type MockRefreshToken struct {
	UserID    string
	SessionID string
	Platform  string
	ExpiresAt time.Time
	UsedAt    *time.Time
	RevokedAt *time.Time
}

// MockRefreshTokenStore is an in-memory RefreshTokenStore for tests.
//
// It reproduces the rotation rules the real store enforces — spend on
// exchange, a grace window for concurrent callers, revoke the session on reuse
// — because those rules are what the handlers branch on. A mock that always
// succeeded would let a handler test pass while the endpoint handed a replayed
// credential a fresh session.
type MockRefreshTokenStore struct {
	mu     sync.Mutex
	Tokens map[string]*MockRefreshToken
	// CreateErr and RotateErr force a store failure.
	CreateErr error
	RotateErr error
	// issued counts tokens minted, so each plaintext is distinct.
	issued int
}

func (m *MockRefreshTokenStore) CreateRefreshToken(_ context.Context, in model.NewRefreshToken) (string, time.Time, error) {
	if m.CreateErr != nil {
		return "", time.Time{}, m.CreateErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mint(in.UserID, in.SessionID, in.Platform, time.Now().UTC(), in.TTL)
}

// mint requires m.mu.
func (m *MockRefreshTokenStore) mint(userID, sessionID, platform string, now time.Time, ttl time.Duration) (string, time.Time, error) {
	if m.Tokens == nil {
		m.Tokens = make(map[string]*MockRefreshToken)
	}
	if ttl <= 0 {
		ttl = model.RefreshTokenTTLDefault
	}
	m.issued++
	plaintext := fmt.Sprintf("mock-refresh-%s-%d", userID, m.issued)
	expiresAt := now.Add(ttl)
	m.Tokens[plaintext] = &MockRefreshToken{
		UserID:    userID,
		SessionID: sessionID,
		Platform:  platform,
		ExpiresAt: expiresAt,
	}
	return plaintext, expiresAt, nil
}

func (m *MockRefreshTokenStore) RotateRefreshToken(_ context.Context, plaintext string, now time.Time, ttl, grace time.Duration) (model.RotatedRefreshToken, error) {
	if m.RotateErr != nil {
		return model.RotatedRefreshToken{}, m.RotateErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.Tokens[plaintext]
	if !ok || row.RevokedAt != nil || !row.ExpiresAt.After(now) {
		return model.RotatedRefreshToken{}, model.ErrRefreshTokenInvalid
	}
	if row.UsedAt != nil && now.Sub(*row.UsedAt) > grace {
		m.revokeSession(row.SessionID, now)
		// The caller needs to know whose session was just revoked in order to
		// record it, so the identifiers come back alongside the error.
		return model.RotatedRefreshToken{UserID: row.UserID, SessionID: row.SessionID}, model.ErrRefreshTokenReused
	}
	if row.UsedAt == nil {
		spent := now
		row.UsedAt = &spent
	}
	next, expiresAt, err := m.mint(row.UserID, row.SessionID, row.Platform, now, ttl)
	if err != nil {
		return model.RotatedRefreshToken{}, err
	}
	return model.RotatedRefreshToken{
		UserID:    row.UserID,
		SessionID: row.SessionID,
		Plaintext: next,
		ExpiresAt: expiresAt,
	}, nil
}

// Backdate moves a spent token's used_at further into the past, so a test can
// put a token outside the rotation grace window without waiting for the clock.
// It reports whether the token was there to move.
func (m *MockRefreshTokenStore) Backdate(plaintext string, by time.Duration) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.Tokens[plaintext]
	if !ok || row.UsedAt == nil {
		return false
	}
	moved := row.UsedAt.Add(-by)
	row.UsedAt = &moved
	return true
}

func (m *MockRefreshTokenStore) RevokeRefreshTokenSession(_ context.Context, plaintext string, now time.Time) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.Tokens[plaintext]
	if !ok {
		return "", "", nil
	}
	m.revokeSession(row.SessionID, now)
	return row.UserID, row.SessionID, nil
}

func (m *MockRefreshTokenStore) RevokeSession(_ context.Context, sessionID string, now time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.revokeSession(sessionID, now), nil
}

// revokeSession requires m.mu.
func (m *MockRefreshTokenStore) revokeSession(sessionID string, now time.Time) int64 {
	var n int64
	for _, row := range m.Tokens {
		if row.SessionID == sessionID && row.RevokedAt == nil {
			revoked := now
			row.RevokedAt = &revoked
			n++
		}
	}
	return n
}

func (m *MockRefreshTokenStore) RevokeUserSessions(_ context.Context, userID string, now time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for _, tok := range m.Tokens {
		if tok.UserID == userID && tok.RevokedAt == nil {
			revoked := now
			tok.RevokedAt = &revoked
			n++
		}
	}
	return n, nil
}

func (m *MockRefreshTokenStore) CountUserSessions(_ context.Context, userID string, now time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sessions := make(map[string]bool)
	for _, tok := range m.Tokens {
		if tok.UserID == userID && tok.RevokedAt == nil && tok.ExpiresAt.After(now) {
			sessions[tok.SessionID] = true
		}
	}
	return len(sessions), nil
}

func (m *MockRefreshTokenStore) DeleteExpiredRefreshTokens(_ context.Context, before time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for plaintext, row := range m.Tokens {
		if !row.ExpiresAt.After(before) {
			delete(m.Tokens, plaintext)
			n++
		}
	}
	return n, nil
}
