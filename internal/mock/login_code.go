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

// ConsumeLoginCode mirrors the real store: unknown, spent, and expired codes are
// all reported the same way, as no user rather than as an error.
func (m *MockLoginCodeStore) ConsumeLoginCode(_ context.Context, plaintext string, now int64) (string, error) {
	if m.ConsumeErr != nil {
		return "", m.ConsumeErr
	}
	code, ok := m.Codes[plaintext]
	if !ok || code.Used || code.ExpiresAt <= now {
		return "", nil
	}
	code.Used = true
	return code.UserID, nil
}
