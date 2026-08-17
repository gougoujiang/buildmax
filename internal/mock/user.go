package mock

import (
	"context"
	"fmt"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// MockUserStore is an in-memory UserStore for tests.
// Use ByEmail and ByID to pre-seed users; CreateErr and NextUserID for behavior.
type MockUserStore struct {
	ByEmail    map[string]*model.User
	ByID       map[string]*model.User
	CreateErr  error
	NextUserID int
}

func (m *MockUserStore) UserByEmail(_ context.Context, email string) (*model.User, error) {
	if m.ByEmail == nil {
		return nil, nil
	}
	return m.ByEmail[email], nil
}

func (m *MockUserStore) GetUser(_ context.Context, userID string) (*model.User, error) {
	if m.ByID != nil {
		return m.ByID[userID], nil
	}
	return nil, nil
}

func (m *MockUserStore) CreateUser(_ context.Context, email string, defaultQuotaTier string) (*model.User, error) {
	if m.CreateErr != nil {
		return nil, m.CreateErr
	}
	if m.ByEmail == nil {
		m.ByEmail = make(map[string]*model.User)
	}
	if m.ByID == nil {
		m.ByID = make(map[string]*model.User)
	}
	if existing := m.ByEmail[email]; existing != nil {
		return nil, model.ErrEmailExists
	}
	m.NextUserID++
	u := &model.User{
		UserID:    fmt.Sprintf("mock-u-%d", m.NextUserID),
		Email:     email,
		QuotaTier: defaultQuotaTier,
		Name:      "",
		CreatedAt: time.Now().Unix(),
	}
	m.ByEmail[email] = u
	m.ByID[u.UserID] = u
	return u, nil
}

func (m *MockUserStore) UpdateLoginMeta(_ context.Context, userID string, loginAt int64, platform string) error {
	if u, ok := m.ByID[userID]; ok {
		u.LastLoginAt = &loginAt
		u.LastLoginPlatform = &platform
	}
	return nil
}

// MockPasswordStore is an in-memory PasswordStore for tests. Seed Hashes with
// the encoded hash a test expects to verify against.
type MockPasswordStore struct {
	Hashes  map[string]string
	ReadErr error
	SetErr  error
}

func (m *MockPasswordStore) PasswordHash(_ context.Context, userID string) (string, error) {
	if m.ReadErr != nil {
		return "", m.ReadErr
	}
	return m.Hashes[userID], nil
}

func (m *MockPasswordStore) SetPassword(_ context.Context, userID, encodedHash string, _ int64) error {
	if m.SetErr != nil {
		return m.SetErr
	}
	if m.Hashes == nil {
		m.Hashes = make(map[string]string)
	}
	m.Hashes[userID] = encodedHash
	return nil
}
