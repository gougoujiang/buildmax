package server

import (
	"context"
	"fmt"
	"time"

	"buildmax/internal/storage/blob"
	"buildmax/internal/storage/entity"

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
	userByEmail  map[string]*entity.User
	createErr    error // if set, CreateUser returns this error
	nextUserID   int   // used to generate unique user_id in CreateUser
}

func (m *mockUserStore) UserByEmail(_ context.Context, email string) (*entity.User, error) {
	return m.userByEmail[email], nil
}

func (m *mockUserStore) CreateUser(ctx context.Context, email string) (*entity.User, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	if m.userByEmail == nil {
		m.userByEmail = make(map[string]*entity.User)
	}
	if existing := m.userByEmail[email]; existing != nil {
		return nil, entity.ErrEmailExists
	}
	m.nextUserID++
	u := &entity.User{
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
	list      []entity.Workspace
	listErr   error
}

func (m *mockWorkspaceStore) EnsureDefaultWorkspaceForUser(_ context.Context, _ string) error {
	return m.ensureErr
}

func (m *mockWorkspaceStore) ListWorkspacesByOwner(_ context.Context, _ string) ([]entity.Workspace, error) {
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

func (m *mockWorkspaceStore) CreateWorkspace(_ context.Context, userID, name string) (*entity.Workspace, error) {
	w := entity.Workspace{
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
	list     []entity.Project
	listErr  error
	create   *entity.Project
	createErr error
}

func (m *mockProjectStore) GetProject(_ context.Context, projectID string) (*entity.Project, error) {
	for i := range m.list {
		if m.list[i].ProjectID == projectID {
			return &m.list[i], nil
		}
	}
	return nil, nil
}

func (m *mockProjectStore) ListProjectsByWorkspace(_ context.Context, workspaceID string) ([]entity.Project, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var out []entity.Project
	for _, p := range m.list {
		if p.WorkspaceID == workspaceID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *mockProjectStore) CreateProject(_ context.Context, workspaceID, name, description string) (*entity.Project, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	if m.create != nil {
		return m.create, nil
	}
	return &entity.Project{
		ProjectID:   "proj1",
		WorkspaceID: workspaceID,
		Name:        name,
		Description: description,
		CreatedAt:   time.Now().Unix(),
	}, nil
}

// mockTaskStore is an in-memory TaskStore for tests.
type mockTaskStore struct {
	list      []entity.Task
	listErr   error
	create    *entity.Task
	createErr error
}

func (m *mockTaskStore) ListTasksByWorkspace(_ context.Context, workspaceID string, projectID *string) ([]entity.Task, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var out []entity.Task
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

func (m *mockTaskStore) CreateTask(_ context.Context, workspaceID string, projectID *string, input, createdBy string) (*entity.Task, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	if m.create != nil {
		return m.create, nil
	}
	return &entity.Task{
		TaskID:      "task-uuid-1",
		WorkspaceID: workspaceID,
		ProjectID:   projectID,
		Status:      "PENDING",
		Input:       input,
		CreatedBy:   createdBy,
		CreatedAt:   12345,
	}, nil
}

func (m *mockTaskStore) GetTaskBySessionID(_ context.Context, sessionID string) (*entity.Task, error) {
	for i := range m.list {
		if m.list[i].SessionID != nil && *m.list[i].SessionID == sessionID {
			return &m.list[i], nil
		}
	}
	return nil, nil
}

func (m *mockTaskStore) GetNextPendingTask(_ context.Context) (*entity.Task, error) {
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

func (m *mockTaskStore) UpdateTaskStatusIf(_ context.Context, taskID, expectedStatus, newStatus string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) (bool, error) {
	for i := range m.list {
		if m.list[i].TaskID == taskID && m.list[i].Status == expectedStatus {
			m.list[i].Status = newStatus
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
			return true, nil
		}
	}
	return false, nil
}

func (m *mockTaskStore) UpdateTaskWorkerInfo(_ context.Context, taskID, workerType string, k8sJobName *string, k8sJobCreatedAt *int64) error {
	for i := range m.list {
		if m.list[i].TaskID == taskID {
			m.list[i].WorkerType = workerType
			m.list[i].K8sJobName = k8sJobName
			m.list[i].K8sJobCreatedAt = k8sJobCreatedAt
			return nil
		}
	}
	return nil
}

func (m *mockTaskStore) IncrementTaskSeq(_ context.Context, taskID string) (int, error) {
	for i := range m.list {
		if m.list[i].TaskID == taskID {
			m.list[i].ArtifactSeq++
			return m.list[i].ArtifactSeq, nil
		}
	}
	return 0, nil
}

func (m *mockTaskStore) CreateArtifactWithItem(_ context.Context, taskID, artifactID string, seq int, relativePath string) error {
	return nil
}

func (m *mockTaskStore) GetTask(_ context.Context, taskID string) (*entity.Task, error) {
	for i := range m.list {
		if m.list[i].TaskID == taskID {
			return &m.list[i], nil
		}
	}
	return nil, nil
}

// mockArtifactStore is an in-memory ArtifactStore for tests.
type mockArtifactStore struct {
	list      []entity.ArtifactWithTask
	listErr   error
	get       map[string]*entity.Artifact    // artifact_id -> artifact
	getErr    error
	listItems map[string][]entity.ArtifactItem // artifact_id -> items
}

func (m *mockArtifactStore) CreateArtifactWithItem(_ context.Context, taskID, artifactID string, seq int, relativePath string) error {
	return nil
}

func (m *mockArtifactStore) ListArtifactsByWorkspace(_ context.Context, workspaceID string, taskID, projectID *string) ([]entity.ArtifactWithTask, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.list, nil
}

func (m *mockArtifactStore) GetArtifactByID(_ context.Context, artifactID string) (*entity.Artifact, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.get != nil {
		return m.get[artifactID], nil
	}
	return nil, nil
}

func (m *mockArtifactStore) ListArtifactItems(_ context.Context, artifactID string) ([]entity.ArtifactItem, error) {
	if m.listItems != nil {
		return m.listItems[artifactID], nil
	}
	return nil, nil
}

// mockArtifactStorage is an in-memory blob.ArtifactStorage for tests.
type mockArtifactStorage struct {
	results map[string][]byte // "workspaceID/taskID/artifactID" -> content
}

func newMockArtifactStorage() *mockArtifactStorage {
	return &mockArtifactStorage{results: make(map[string][]byte)}
}

func (m *mockArtifactStorage) PutResult(_ context.Context, workspaceID, taskID, artifactID string, data []byte) error {
	m.results[workspaceID+"/"+taskID+"/"+artifactID] = append([]byte(nil), data...)
	return nil
}

func (m *mockArtifactStorage) GetResult(_ context.Context, workspaceID, taskID, artifactID string) ([]byte, error) {
	key := workspaceID + "/" + taskID + "/" + artifactID
	if data, ok := m.results[key]; ok {
		return data, nil
	}
	return nil, blob.ErrNotFound
}
