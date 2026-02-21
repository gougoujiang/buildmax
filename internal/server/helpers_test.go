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

// mockChatStore is an in-memory ChatStore for tests.
type mockChatStore struct {
	list      []entity.Chat
	listErr   error
	create    *entity.Chat
	createErr error
}

func (m *mockChatStore) ListChatsByWorkspace(_ context.Context, workspaceID string, order string) ([]entity.Chat, error) {
	list, _, err := m.ListChatsByWorkspacePaginated(context.Background(), workspaceID, false, 0, 0)
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

func (m *mockChatStore) ListChatsByWorkspacePaginated(_ context.Context, workspaceID string, executedOnly bool, limit, offset int) ([]entity.Chat, int, error) {
	if m.listErr != nil {
		return nil, 0, m.listErr
	}
	var filtered []entity.Chat
	for _, c := range m.list {
		if c.WorkspaceID != workspaceID {
			continue
		}
		if executedOnly && (c.LastRunID == nil || *c.LastRunID == "") {
			continue
		}
		filtered = append(filtered, c)
	}
	// Sort by created_at DESC (simple reverse by index if already ascending in list)
	for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	}
	total := len(filtered)
	if offset > len(filtered) {
		return []entity.Chat{}, total, nil
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

func (m *mockChatStore) CreateChat(_ context.Context, workspaceID, input, title, createdBy string) (*entity.Chat, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	if m.create != nil {
		return m.create, nil
	}
	return &entity.Chat{
		ChatID:      "chat-uuid-1",
		WorkspaceID: workspaceID,
		Status:      "PENDING",
		Input:       input,
		Title:       title,
		CreatedBy:   createdBy,
		CreatedAt:   12345,
	}, nil
}

func (m *mockChatStore) GetChatBySessionID(_ context.Context, sessionID string) (*entity.Chat, error) {
	for i := range m.list {
		if m.list[i].SessionID != nil && *m.list[i].SessionID == sessionID {
			return &m.list[i], nil
		}
	}
	return nil, nil
}

func (m *mockChatStore) UpdateChatStatus(_ context.Context, chatID, status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) error {
	for i := range m.list {
		if m.list[i].ChatID == chatID {
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

func (m *mockChatStore) UpdateChatStatusIf(_ context.Context, chatID, expectedStatus, newStatus string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) (bool, error) {
	for i := range m.list {
		if m.list[i].ChatID == chatID && m.list[i].Status == expectedStatus {
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

func (m *mockChatStore) IncrementChatSeq(_ context.Context, chatID string) (int, error) {
	for i := range m.list {
		if m.list[i].ChatID == chatID {
			m.list[i].ArtifactSeq++
			return m.list[i].ArtifactSeq, nil
		}
	}
	return 0, nil
}

func (m *mockChatStore) GetChat(_ context.Context, chatID string) (*entity.Chat, error) {
	for i := range m.list {
		if m.list[i].ChatID == chatID {
			return &m.list[i], nil
		}
	}
	return nil, nil
}

// mockChatRunStore is an in-memory ChatRunStore for tests. Uses runs list and chatList to resolve GetChatRunWithChat.
type mockChatRunStore struct {
	runs     []entity.ChatRun
	chatList []entity.Chat // chats by chat_id for GetChatRunWithChat
}

func (m *mockChatRunStore) CreateChatRun(_ context.Context, chatID, input, createdBy string) (*entity.ChatRun, error) {
	return nil, nil
}
func (m *mockChatRunStore) GetNextPendingChatRun(_ context.Context) (*entity.ChatRun, error) { return nil, nil }
func (m *mockChatRunStore) GetChatRun(_ context.Context, chatRunID string) (*entity.ChatRun, error) {
	for i := range m.runs {
		if m.runs[i].ChatRunID == chatRunID {
			return &m.runs[i], nil
		}
	}
	return nil, nil
}
func (m *mockChatRunStore) GetChatRunWithChat(_ context.Context, chatRunID string) (*entity.ChatRun, *entity.Chat, error) {
	var run *entity.ChatRun
	for i := range m.runs {
		if m.runs[i].ChatRunID == chatRunID {
			run = &m.runs[i]
			break
		}
	}
	if run == nil {
		return nil, nil, nil
	}
	var chat *entity.Chat
	for i := range m.chatList {
		if m.chatList[i].ChatID == run.ChatID {
			chat = &m.chatList[i]
			break
		}
	}
	return run, chat, nil
}
func (m *mockChatRunStore) UpdateChatRunStatusIf(_ context.Context, chatRunID, expectedStatus, newStatus string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) (bool, error) {
	for i := range m.runs {
		if m.runs[i].ChatRunID == chatRunID && m.runs[i].Status == expectedStatus {
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
func (m *mockChatRunStore) UpdateChatRunStatus(_ context.Context, chatRunID, status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) error {
	for i := range m.runs {
		if m.runs[i].ChatRunID == chatRunID {
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
func (m *mockChatRunStore) UpdateChatRunWorkerInfo(_ context.Context, chatRunID, workerType string, k8sJobName *string, k8sJobCreatedAt *int64) error {
	return nil
}
func (m *mockChatRunStore) OnRunComplete(_ context.Context, chatRunID, artifactID, relativePath string) error {
	return nil
}
func (m *mockChatRunStore) SyncChatFromRun(_ context.Context, chatRunID string) error {
	return nil
}

// mockArtifactStore is an in-memory ArtifactStore for tests.
type mockArtifactStore struct {
	list      []entity.ArtifactWithChat
	listErr   error
	get       map[string]*entity.Artifact    // artifact_id -> artifact
	getErr    error
	listItems map[string][]entity.ArtifactItem // artifact_id -> items
}

func (m *mockArtifactStore) CreateArtifactWithItem(_ context.Context, chatID, chatRunID, artifactID string, seq int, relativePath string) error {
	return nil
}

func (m *mockArtifactStore) ListArtifactsByWorkspace(_ context.Context, workspaceID string, chatID *string) ([]entity.ArtifactWithChat, error) {
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
	results map[string][]byte // "workspaceID/chatID/chatRunID/artifactID" -> content
}

func newMockArtifactStorage() *mockArtifactStorage {
	return &mockArtifactStorage{results: make(map[string][]byte)}
}

func (m *mockArtifactStorage) PutResult(_ context.Context, workspaceID, chatID, chatRunID, artifactID string, data []byte) error {
	m.results[workspaceID+"/"+chatID+"/"+chatRunID+"/"+artifactID] = append([]byte(nil), data...)
	return nil
}

func (m *mockArtifactStorage) GetResult(_ context.Context, workspaceID, chatID, chatRunID, artifactID string) ([]byte, error) {
	key := workspaceID + "/" + chatID + "/" + chatRunID + "/" + artifactID
	if data, ok := m.results[key]; ok {
		return data, nil
	}
	return nil, blob.ErrNotFound
}
