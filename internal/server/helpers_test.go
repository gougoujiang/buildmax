package server

import (
	"context"
	"fmt"
	"time"

	"buildmax/internal/store"

	"github.com/golang-jwt/jwt/v5"
)

// signJWT builds a JWT with sub claim for use in tests.
func signJWT(sub, secret string) string {
	now := time.Now()
	claims := jwtClaims{
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

// mockUserStore is an in-memory UserStore for tests.
type mockUserStore struct {
	userByEmail  map[string]*store.User
	createErr    error // if set, CreateUser returns this error
	nextUserID   int   // used to generate unique user_id in CreateUser
}

func (m *mockUserStore) UserByEmail(_ context.Context, email string) (*store.User, error) {
	return m.userByEmail[email], nil
}

func (m *mockUserStore) CreateUser(ctx context.Context, email string) (*store.User, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	if m.userByEmail == nil {
		m.userByEmail = make(map[string]*store.User)
	}
	if existing := m.userByEmail[email]; existing != nil {
		return nil, store.ErrEmailExists
	}
	m.nextUserID++
	u := &store.User{
		UserID:    fmt.Sprintf("mock-u-%d", m.nextUserID),
		Email:     email,
		Name:      "",
		CreatedAt: time.Now().Unix(),
	}
	m.userByEmail[email] = u
	return u, nil
}

// mockWorkspaceStore is an in-memory WorkspaceStore for tests.
type mockWorkspaceStore struct {
	ensureErr error
	list      []store.Workspace
	listErr   error
}

func (m *mockWorkspaceStore) EnsureDefaultWorkspaceForUser(_ context.Context, _ string) error {
	return m.ensureErr
}

func (m *mockWorkspaceStore) ListWorkspacesByOwner(_ context.Context, _ string) ([]store.Workspace, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.list, nil
}

func (m *mockWorkspaceStore) WorkspaceBelongsToUser(_ context.Context, workspaceID, userID string) (bool, error) {
	for _, w := range m.list {
		if w.WorkspaceID == workspaceID && w.OwnerUserID == userID {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockWorkspaceStore) CreateWorkspace(_ context.Context, userID, name string) (*store.Workspace, error) {
	w := store.Workspace{
		WorkspaceID:  fmt.Sprintf("ws-%d", len(m.list)+1),
		OwnerUserID:  userID,
		Name:         name,
		CreatedAt:     time.Now().Unix(),
	}
	m.list = append(m.list, w)
	return &w, nil
}

// mockProjectStore is an in-memory ProjectStore for tests.
type mockProjectStore struct {
	list     []store.Project
	listErr  error
	create   *store.Project
	createErr error
}

func (m *mockProjectStore) GetProject(_ context.Context, projectID string) (*store.Project, error) {
	for i := range m.list {
		if m.list[i].ProjectID == projectID {
			return &m.list[i], nil
		}
	}
	return nil, nil
}

func (m *mockProjectStore) ListProjectsByWorkspace(_ context.Context, workspaceID string) ([]store.Project, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var out []store.Project
	for _, p := range m.list {
		if p.WorkspaceID == workspaceID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *mockProjectStore) CreateProject(_ context.Context, workspaceID, name, description string) (*store.Project, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	if m.create != nil {
		return m.create, nil
	}
	return &store.Project{
		ProjectID:   "proj1",
		WorkspaceID: workspaceID,
		Name:        name,
		Description: description,
		CreatedAt:   time.Now().Unix(),
	}, nil
}

// mockTaskStore is an in-memory TaskStore for tests.
type mockTaskStore struct {
	list      []store.Task
	listErr   error
	create    *store.Task
	createErr error
}

func (m *mockTaskStore) ListTasksByWorkspace(_ context.Context, workspaceID string, projectID *string) ([]store.Task, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var out []store.Task
	for _, t := range m.list {
		if t.WorkspaceID != workspaceID {
			continue
		}
		if projectID != nil && (t.ProjectID == nil || *t.ProjectID != *projectID) {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func (m *mockTaskStore) CreateTask(_ context.Context, workspaceID string, projectID *string, input, createdBy string) (*store.Task, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	if m.create != nil {
		return m.create, nil
	}
	return &store.Task{
		TaskID:      "task-uuid-1",
		WorkspaceID: workspaceID,
		ProjectID:   projectID,
		Status:      "PENDING",
		Input:       input,
		CreatedBy:   createdBy,
		CreatedAt:   12345,
	}, nil
}

func (m *mockTaskStore) GetTaskBySessionID(_ context.Context, sessionID string) (*store.Task, error) {
	for i := range m.list {
		if m.list[i].SessionID != nil && *m.list[i].SessionID == sessionID {
			return &m.list[i], nil
		}
	}
	return nil, nil
}

func (m *mockTaskStore) GetNextPendingTask(_ context.Context) (*store.Task, error) {
	for i := range m.list {
		if m.list[i].Status == "PENDING" {
			return &m.list[i], nil
		}
	}
	return nil, nil
}

func (m *mockTaskStore) UpdateTaskStatus(_ context.Context, taskID, status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) error {
	for i := range m.list {
		if m.list[i].TaskID == taskID {
			m.list[i].Status = status
			if startedAt != nil {
				m.list[i].StartedAt = startedAt
			}
			if endedAt != nil {
				m.list[i].EndedAt = endedAt
			}
			if output != nil {
				m.list[i].Output = output
			}
			if errorMessage != nil {
				m.list[i].ErrorMessage = errorMessage
			}
			if sessionID != nil {
				m.list[i].SessionID = sessionID
			}
			return nil
		}
	}
	return nil
}
