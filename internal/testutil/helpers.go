// Package testutil provides test-only helpers for use from _test.go files:
// ptr helpers (e.g. PtrString), JWT (SignJWT), and in-memory mocks for entity stores and quota (MockUserStore, MockChatStore, etc.).
package testutil

import (
	"context"
	"fmt"
	"time"

	"buildmax/internal/ptr"
	"buildmax/internal/storage/entity"

	"github.com/golang-jwt/jwt/v5"
)

// PtrString returns a pointer to s. Useful for filling optional *string fields in tests.
func PtrString(s string) *string {
	return ptr.PtrString(s)
}

// testJWTClaims is used by SignJWT for tests (matches JWT sub claim).
type testJWTClaims struct {
	jwt.RegisteredClaims
	Sub string `json:"sub"`
}

// SignJWT builds a JWT with sub claim for use in tests.
func SignJWT(sub, secret string) string {
	now := time.Now()
	claims := testJWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Sub: sub,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, _ := token.SignedString([]byte(secret))
	return s
}

// MockUserStore is an in-memory UserStore for tests.
// Use ByEmail and ByID to pre-seed users; CreateErr and NextUserID for behavior.
type MockUserStore struct {
	ByEmail    map[string]*entity.User
	ByID       map[string]*entity.User
	CreateErr  error
	NextUserID int
}

func (m *MockUserStore) UserByEmail(_ context.Context, email string) (*entity.User, error) {
	if m.ByEmail == nil {
		return nil, nil
	}
	return m.ByEmail[email], nil
}

func (m *MockUserStore) GetUser(_ context.Context, userID string) (*entity.User, error) {
	if m.ByID != nil {
		return m.ByID[userID], nil
	}
	return nil, nil
}

func (m *MockUserStore) CreateUser(_ context.Context, email string, defaultQuotaTier string) (*entity.User, error) {
	if m.CreateErr != nil {
		return nil, m.CreateErr
	}
	if m.ByEmail == nil {
		m.ByEmail = make(map[string]*entity.User)
	}
	if m.ByID == nil {
		m.ByID = make(map[string]*entity.User)
	}
	if existing := m.ByEmail[email]; existing != nil {
		return nil, entity.ErrEmailExists
	}
	m.NextUserID++
	u := &entity.User{
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

// MockWorkspaceStore is an in-memory WorkspaceStore for tests.
type MockWorkspaceStore struct {
	EnsureErr error
	List      []entity.Workspace
	ListErr   error
}

func (m *MockWorkspaceStore) EnsureDefaultWorkspaceForUser(_ context.Context, _ string) error {
	return m.EnsureErr
}

func (m *MockWorkspaceStore) ListWorkspacesByOwner(_ context.Context, _ string) ([]entity.Workspace, error) {
	if m.ListErr != nil {
		return nil, m.ListErr
	}
	return m.List, nil
}

func (m *MockWorkspaceStore) WorkspaceBelongsToUser(_ context.Context, workspaceID, userID string) (bool, error) {
	for _, w := range m.List {
		if w.WorkspaceID == workspaceID && w.OwnerUserID == userID {
			return true, nil
		}
	}
	return false, nil
}

func (m *MockWorkspaceStore) CreateWorkspace(_ context.Context, userID, name string) (*entity.Workspace, error) {
	w := entity.Workspace{
		WorkspaceID: fmt.Sprintf("ws-%d", len(m.List)+1),
		OwnerUserID: userID,
		Name:        name,
		CreatedAt:   time.Now().Unix(),
	}
	m.List = append(m.List, w)
	return &w, nil
}
