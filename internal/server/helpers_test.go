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

// mockTaskStore is an in-memory TaskStore for tests.
type mockTaskStore struct {
	list      []entity.Task
	listErr   error
	create    *entity.Task
	createErr error
}

func (m *mockTaskStore) ListTasksByWorkspace(_ context.Context, workspaceID string, order string) ([]entity.Task, error) {
	list, _, err := m.ListTasksByWorkspacePaginated(context.Background(), workspaceID, false, 0, 0)
	if err != nil {
		return nil, err
	}
	if order == "asc" {
		for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
			list[i], list[j] = list[j], list[i]
		}
	}
	return list, nil
}

func (m *mockTaskStore) ListTasksByWorkspacePaginated(_ context.Context, workspaceID string, executedOnly bool, limit, offset int) ([]entity.Task, int, error) {
	if m.listErr != nil {
		return nil, 0, m.listErr
	}
	var filtered []entity.Task
	for _, t := range m.list {
		if t.WorkspaceID != workspaceID {
			continue
		}
		if executedOnly && (t.LastRunID == nil || *t.LastRunID == "") {
			continue
		}
		filtered = append(filtered, t)
	}
	// Sort by created_at DESC (simple reverse by index if already ascending in list)
	for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	}
	total := len(filtered)
	if offset > len(filtered) {
		return []entity.Task{}, total, nil
	}
	end := offset + limit
	if limit <= 0 {
		end = len(filtered)
	}
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], total, nil
}

func (m *mockTaskStore) CreateTask(_ context.Context, workspaceID, input, title, createdBy string) (*entity.Task, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	if m.create != nil {
		return m.create, nil
	}
	return &entity.Task{
		TaskID:      "task-uuid-1",
		WorkspaceID: workspaceID,
		Status:      "PENDING",
		Input:       input,
		Title:       title,
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

func (m *mockTaskStore) IncrementTaskSeq(_ context.Context, taskID string) (int, error) {
	for i := range m.list {
		if m.list[i].TaskID == taskID {
			m.list[i].ArtifactSeq++
			return m.list[i].ArtifactSeq, nil
		}
	}
	return 0, nil
}

func (m *mockTaskStore) GetTask(_ context.Context, taskID string) (*entity.Task, error) {
	for i := range m.list {
		if m.list[i].TaskID == taskID {
			return &m.list[i], nil
		}
	}
	return nil, nil
}

// mockTaskRunStore is an in-memory TaskRunStore for tests. Uses runs list and taskList to resolve GetTaskRunWithTask.
type mockTaskRunStore struct {
	runs     []entity.TaskRun
	taskList []entity.Task // tasks by task_id for GetTaskRunWithTask
}

func (m *mockTaskRunStore) CreateTaskRun(_ context.Context, taskID, input, createdBy string) (*entity.TaskRun, error) {
	return nil, nil
}
func (m *mockTaskRunStore) GetNextPendingTaskRun(_ context.Context) (*entity.TaskRun, error) { return nil, nil }
func (m *mockTaskRunStore) GetTaskRun(_ context.Context, runID string) (*entity.TaskRun, error) {
	for i := range m.runs {
		if m.runs[i].RunID == runID {
			return &m.runs[i], nil
		}
	}
	return nil, nil
}
func (m *mockTaskRunStore) GetTaskRunWithTask(_ context.Context, runID string) (*entity.TaskRun, *entity.Task, error) {
	var run *entity.TaskRun
	for i := range m.runs {
		if m.runs[i].RunID == runID {
			run = &m.runs[i]
			break
		}
	}
	if run == nil {
		return nil, nil, nil
	}
	var task *entity.Task
	for i := range m.taskList {
		if m.taskList[i].TaskID == run.TaskID {
			task = &m.taskList[i]
			break
		}
	}
	return run, task, nil
}
func (m *mockTaskRunStore) UpdateTaskRunStatusIf(_ context.Context, runID, expectedStatus, newStatus string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) (bool, error) {
	for i := range m.runs {
		if m.runs[i].RunID == runID && m.runs[i].Status == expectedStatus {
			m.runs[i].Status = newStatus
			if startedAt != nil {
				m.runs[i].StartedAt = startedAt
			}
			if sessionID != nil {
				m.runs[i].SessionID = sessionID
			}
			return true, nil
		}
	}
	return false, nil
}
func (m *mockTaskRunStore) UpdateTaskRunStatus(_ context.Context, runID, status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) error {
	for i := range m.runs {
		if m.runs[i].RunID == runID {
			m.runs[i].Status = status
			if startedAt != nil {
				m.runs[i].StartedAt = startedAt
			}
			if endedAt != nil {
				m.runs[i].EndedAt = endedAt
			}
			if output != nil {
				m.runs[i].Output = output
			}
			if errorMessage != nil {
				m.runs[i].ErrorMessage = errorMessage
			}
			if sessionID != nil {
				m.runs[i].SessionID = sessionID
			}
			return nil
		}
	}
	return nil
}
func (m *mockTaskRunStore) UpdateTaskRunWorkerInfo(_ context.Context, runID, workerType string, k8sJobName *string, k8sJobCreatedAt *int64) error {
	return nil
}
func (m *mockTaskRunStore) OnRunComplete(_ context.Context, runID, artifactID, relativePath string) error {
	return nil
}
func (m *mockTaskRunStore) SyncTaskFromRun(_ context.Context, runID string) error {
	return nil
}

// mockArtifactStore is an in-memory ArtifactStore for tests.
type mockArtifactStore struct {
	list      []entity.ArtifactWithTask
	listErr   error
	get       map[string]*entity.Artifact    // artifact_id -> artifact
	getErr    error
	listItems map[string][]entity.ArtifactItem // artifact_id -> items
}

func (m *mockArtifactStore) CreateArtifactWithItem(_ context.Context, taskID, taskRunID, artifactID string, seq int, relativePath string) error {
	return nil
}

func (m *mockArtifactStore) ListArtifactsByWorkspace(_ context.Context, workspaceID string, taskID *string) ([]entity.ArtifactWithTask, error) {
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
	results map[string][]byte // "workspaceID/taskID/runID/artifactID" -> content
}

func newMockArtifactStorage() *mockArtifactStorage {
	return &mockArtifactStorage{results: make(map[string][]byte)}
}

func (m *mockArtifactStorage) PutResult(_ context.Context, workspaceID, taskID, runID, artifactID string, data []byte) error {
	m.results[workspaceID+"/"+taskID+"/"+runID+"/"+artifactID] = append([]byte(nil), data...)
	return nil
}

func (m *mockArtifactStorage) GetResult(_ context.Context, workspaceID, taskID, runID, artifactID string) ([]byte, error) {
	key := workspaceID + "/" + taskID + "/" + runID + "/" + artifactID
	if data, ok := m.results[key]; ok {
		return data, nil
	}
	return nil, blob.ErrNotFound
}
