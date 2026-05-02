// Package testutil provides test-only helpers for use from _test.go files:
// ptr helpers (e.g. PtrString), JWT (SignJWT), and in-memory mocks for entity stores and quota (MockUserStore, MockTaskStore, etc.).
package testutil

import (
	"context"
	"fmt"
	"time"

	"buildmax/internal/infra/db"
	"buildmax/internal/util"

	"github.com/golang-jwt/jwt/v5"
)

// PtrString returns a pointer to s. Useful for filling optional *string fields in tests.
func PtrString(s string) *string {
	return util.PtrString(s)
}

// testJWTClaims is used by SignJWT for tests (matches JWT sub claim).
type testJWTClaims struct {
	jwt.RegisteredClaims
	Sub string `json:"sub"`
}

// SignJWT builds a JWT with sub claim and 24h expiry for use in tests.
func SignJWT(sub, secret string) string {
	return SignJWTWithExp(sub, secret, 24*time.Hour)
}

// SignJWTWithExp builds a JWT with sub claim and the given expiry offset from now.
// Use a negative duration to create an already-expired token.
func SignJWTWithExp(sub, secret string, expiresIn time.Duration) string {
	now := time.Now()
	claims := testJWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(expiresIn)),
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
	ByEmail    map[string]*db.User
	ByID       map[string]*db.User
	CreateErr  error
	NextUserID int
}

func (m *MockUserStore) UserByEmail(_ context.Context, email string) (*db.User, error) {
	if m.ByEmail == nil {
		return nil, nil
	}
	return m.ByEmail[email], nil
}

func (m *MockUserStore) GetUser(_ context.Context, userID string) (*db.User, error) {
	if m.ByID != nil {
		return m.ByID[userID], nil
	}
	return nil, nil
}

func (m *MockUserStore) CreateUser(_ context.Context, email string, defaultQuotaTier string) (*db.User, error) {
	if m.CreateErr != nil {
		return nil, m.CreateErr
	}
	if m.ByEmail == nil {
		m.ByEmail = make(map[string]*db.User)
	}
	if m.ByID == nil {
		m.ByID = make(map[string]*db.User)
	}
	if existing := m.ByEmail[email]; existing != nil {
		return nil, db.ErrEmailExists
	}
	m.NextUserID++
	u := &db.User{
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

// MockTeamStore is an in-memory TeamStore for tests.
type MockTeamStore struct {
	Teams   []db.Team
	Members []db.TeamMember
}

func (m *MockTeamStore) GetTeam(_ context.Context, teamID string) (*db.Team, error) {
	for i := range m.Teams {
		if m.Teams[i].TeamID == teamID {
			return &m.Teams[i], nil
		}
	}
	return nil, nil
}

func (m *MockTeamStore) GetPersonalTeamByUser(_ context.Context, userID string) (*db.Team, error) {
	for i := range m.Teams {
		if m.Teams[i].PersonalForUserID != nil && *m.Teams[i].PersonalForUserID == userID {
			return &m.Teams[i], nil
		}
	}
	return nil, nil
}

func (m *MockTeamStore) ListTeamsByUser(_ context.Context, userID string) ([]db.Team, error) {
	var out []db.Team
	for _, member := range m.Members {
		if member.UserID != userID {
			continue
		}
		for _, team := range m.Teams {
			if team.TeamID == member.TeamID {
				out = append(out, team)
			}
		}
	}
	return out, nil
}

func (m *MockTeamStore) CreateTeam(_ context.Context, name, createdBy, quotaTier string) (*db.Team, error) {
	id := fmt.Sprintf("tm_%d", len(m.Teams)+1)
	team := db.Team{
		TeamID:    id,
		Name:      name,
		QuotaTier: quotaTier,
		CreatedBy: createdBy,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	m.Teams = append(m.Teams, team)
	m.Members = append(m.Members, db.TeamMember{
		TeamID:    id,
		UserID:    createdBy,
		Role:      db.TeamRoleOwner,
		CreatedAt: time.Now().Unix(),
	})
	return &m.Teams[len(m.Teams)-1], nil
}

func (m *MockTeamStore) AddTeamMember(_ context.Context, teamID, userID, role string) (*db.TeamMember, error) {
	for i := range m.Members {
		if m.Members[i].TeamID == teamID && m.Members[i].UserID == userID {
			m.Members[i].Role = role
			return &m.Members[i], nil
		}
	}
	member := db.TeamMember{
		TeamID:    teamID,
		UserID:    userID,
		Role:      role,
		CreatedAt: time.Now().Unix(),
	}
	m.Members = append(m.Members, member)
	return &m.Members[len(m.Members)-1], nil
}

func (m *MockTeamStore) RemoveTeamMember(_ context.Context, teamID, userID string) error {
	out := m.Members[:0]
	for _, member := range m.Members {
		if member.TeamID == teamID && member.UserID == userID {
			continue
		}
		out = append(out, member)
	}
	m.Members = out
	return nil
}

func (m *MockTeamStore) ListTeamMembers(_ context.Context, teamID string) ([]db.TeamMember, error) {
	var out []db.TeamMember
	for _, member := range m.Members {
		if member.TeamID == teamID {
			out = append(out, member)
		}
	}
	return out, nil
}
