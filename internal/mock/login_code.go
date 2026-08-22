package mock

import (
	"context"
	"time"
)

// MockLoginCode is one issued code in MockLoginCodeStore.
type MockLoginCode struct {
	UserID    string
	ExpiresAt int64
	Used      bool
}

// MockLoginCodeStore is an in-memory LoginCodeStore for tests. Seed Codes with
// the plaintext a test will submit; ConsumeErr forces a store failure.
type MockLoginCodeStore struct {
	Codes      map[string]*MockLoginCode
	ConsumeErr error
	Issued     []string
}

func (m *MockLoginCodeStore) CreateLoginCode(_ context.Context, userID string, ttl time.Duration) (string, int64, error) {
	if m.Codes == nil {
		m.Codes = make(map[string]*MockLoginCode)
	}
	plaintext := "mock-code-" + userID
	expiresAt := time.Now().Add(ttl).Unix()
	m.Codes[plaintext] = &MockLoginCode{UserID: userID, ExpiresAt: expiresAt}
	m.Issued = append(m.Issued, plaintext)
	return plaintext, expiresAt, nil
}

// ConsumeLoginCode mirrors the real store: unknown, spent, expired, and
// somebody else's codes are all reported the same way, as not redeemed rather
// than as an error, and only a match is marked used.
func (m *MockLoginCodeStore) ConsumeLoginCode(_ context.Context, plaintext, userID string, now int64) (bool, error) {
	if m.ConsumeErr != nil {
		return false, m.ConsumeErr
	}
	code, ok := m.Codes[plaintext]
	if !ok || code.Used || code.ExpiresAt <= now || code.UserID != userID {
		return false, nil
	}
	code.Used = true
	return true, nil
}
