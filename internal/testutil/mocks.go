package testutil

import (
	"context"
	"fmt"
	"io"
	"time"

	"buildmax/internal/storage/blob"
	"buildmax/internal/storage/entity"

	"gorm.io/gorm"
)

// MockChatStore is an in-memory ChatStore for tests.
type MockChatStore struct {
	List      []entity.Chat
	ListErr   error
	Create    *entity.Chat
	CreateErr error
}

func (m *MockChatStore) ListChatsByWorkspace(_ context.Context, workspaceID string, order string) ([]entity.Chat, error) {
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

func (m *MockChatStore) ListChatsByWorkspacePaginated(_ context.Context, workspaceID string, executedOnly bool, limit, offset int) ([]entity.Chat, int, error) {
	if m.ListErr != nil {
		return nil, 0, m.ListErr
	}
	var filtered []entity.Chat
	for _, c := range m.List {
		if c.WorkspaceID != workspaceID {
			continue
		}
		if executedOnly && (c.LastRunID == nil || *c.LastRunID == "") {
			continue
		}
		filtered = append(filtered, c)
	}
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

func (m *MockChatStore) CreateChat(_ context.Context, in *entity.CreateChatInput) (*entity.Chat, error) {
	if m.CreateErr != nil {
		return nil, m.CreateErr
	}
	if m.Create != nil {
		return m.Create, nil
	}
	if in == nil {
		return nil, nil
	}
	c := &entity.Chat{
		ChatID:         "chat-uuid-1",
		WorkspaceID:    in.WorkspaceID,
		Status:         "PENDING",
		Input:          in.Input,
		Title:          in.Title,
		CreatedBy:      in.CreatedBy,
		CreatedAt:      12345,
		ConversationID: in.ConversationID,
		AgentID:        in.AgentID,
	}
	return c, nil
}

func (m *MockChatStore) GetChatBySessionID(_ context.Context, sessionID string) (*entity.Chat, error) {
	for i := range m.List {
		if m.List[i].SessionID != nil && *m.List[i].SessionID == sessionID {
			return &m.List[i], nil
		}
	}
	return nil, nil
}

func (m *MockChatStore) UpdateChatStatus(_ context.Context, chatID, status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) error {
	for i := range m.List {
		if m.List[i].ChatID == chatID {
			m.List[i].Status = status
			if startedAt != nil {
				m.List[i].StartedAt = startedAt
			}
			if endedAt != nil {
				m.List[i].EndedAt = endedAt
			}
			if output != nil {
				m.List[i].Output = output
			}
			if errorMessage != nil {
				m.List[i].ErrorMessage = errorMessage
			}
			if sessionID != nil {
				m.List[i].SessionID = sessionID
			}
			return nil
		}
	}
	return nil
}

func (m *MockChatStore) UpdateChatStatusIf(_ context.Context, chatID, expectedStatus, newStatus string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) (bool, error) {
	for i := range m.List {
		if m.List[i].ChatID == chatID && m.List[i].Status == expectedStatus {
			m.List[i].Status = newStatus
			if startedAt != nil {
				m.List[i].StartedAt = startedAt
			}
			if endedAt != nil {
				m.List[i].EndedAt = endedAt
			}
			if output != nil {
				m.List[i].Output = output
			}
			if errorMessage != nil {
				m.List[i].ErrorMessage = errorMessage
			}
			if sessionID != nil {
				m.List[i].SessionID = sessionID
			}
			return true, nil
		}
	}
	return false, nil
}

func (m *MockChatStore) GetChat(_ context.Context, chatID string) (*entity.Chat, error) {
	for i := range m.List {
		if m.List[i].ChatID == chatID {
			return &m.List[i], nil
		}
	}
	return nil, nil
}

// MockChatRunStore is an in-memory ChatRunStore for tests.
type MockChatRunStore struct {
	Runs     []entity.ChatRun
	ChatList []entity.Chat
}

func (m *MockChatRunStore) CreateChatRun(_ context.Context, chatID, input, createdBy string) (*entity.ChatRun, error) {
	return nil, nil
}
func (m *MockChatRunStore) GetNextPendingChatRun(_ context.Context) (*entity.ChatRun, error) {
	return nil, nil
}
func (m *MockChatRunStore) GetChatRun(_ context.Context, chatRunID string) (*entity.ChatRun, error) {
	for i := range m.Runs {
		if m.Runs[i].ChatRunID == chatRunID {
			return &m.Runs[i], nil
		}
	}
	return nil, nil
}
func (m *MockChatRunStore) GetChatRunWithChat(_ context.Context, chatRunID string) (*entity.ChatRun, *entity.Chat, error) {
	var run *entity.ChatRun
	for i := range m.Runs {
		if m.Runs[i].ChatRunID == chatRunID {
			run = &m.Runs[i]
			break
		}
	}
	if run == nil {
		return nil, nil, nil
	}
	var chat *entity.Chat
	for i := range m.ChatList {
		if m.ChatList[i].ChatID == run.ChatID {
			chat = &m.ChatList[i]
			break
		}
	}
	return run, chat, nil
}
func (m *MockChatRunStore) ClaimChatRun(ctx context.Context, in entity.ClaimChatRunInput) (bool, error) {
	for i := range m.Runs {
		if m.Runs[i].ChatRunID == in.ChatRunID && m.Runs[i].Status == string(in.ExpectedStatus) {
			m.Runs[i].Status = string(in.NewStatus)
			if in.StartedAt != nil {
				m.Runs[i].StartedAt = in.StartedAt
			}
			if in.SessionID != nil {
				m.Runs[i].SessionID = in.SessionID
			}
			return true, nil
		}
	}
	return false, nil
}
func (m *MockChatRunStore) UpdateRun(ctx context.Context, in entity.UpdateChatRunInput) error {
	for i := range m.Runs {
		if m.Runs[i].ChatRunID == in.ChatRunID {
			m.Runs[i].Status = string(in.Status)
			if in.StartedAt != nil {
				m.Runs[i].StartedAt = in.StartedAt
			}
			if in.EndedAt != nil {
				m.Runs[i].EndedAt = in.EndedAt
			}
			if in.Output != nil {
				m.Runs[i].Output = in.Output
			}
			if in.ErrorMessage != nil {
				m.Runs[i].ErrorMessage = in.ErrorMessage
			}
			if in.SessionID != nil {
				m.Runs[i].SessionID = in.SessionID
			}
			if in.PromptTokens != nil {
				m.Runs[i].PromptTokens = in.PromptTokens
			}
			if in.CompletionTokens != nil {
				m.Runs[i].CompletionTokens = in.CompletionTokens
			}
			return nil
		}
	}
	return nil
}
func (m *MockChatRunStore) UpdateChatRunWorkerInfo(_ context.Context, chatRunID, workerType string, k8sJobName *string, k8sJobCreatedAt *int64) error {
	return nil
}
func (m *MockChatRunStore) OnRunComplete(_ context.Context, chatRunID string, relativePaths []string) error {
	return nil
}
func (m *MockChatRunStore) SyncChatFromRun(_ context.Context, chatRunID string) error {
	return nil
}

// MockRunOutputLister is an in-memory RunOutputLister for tests.
type MockRunOutputLister struct {
	List        []entity.ArtifactWithChat
	ListErr     error
	OutputFiles map[string][]entity.ChatRunArtifact
}

func (m *MockRunOutputLister) ListRunOutputsByWorkspace(_ context.Context, workspaceID string, chatID *string) ([]entity.ArtifactWithChat, error) {
	if m.ListErr != nil {
		return nil, m.ListErr
	}
	return m.List, nil
}

func (m *MockRunOutputLister) GetChatRunOutputFiles(_ context.Context, chatRunID string) ([]entity.ChatRunArtifact, error) {
	if m.OutputFiles != nil {
		return m.OutputFiles[chatRunID], nil
	}
	return nil, nil
}

// MockAgentStore is an in-memory AgentStore for tests.
type MockAgentStore struct {
	Agents []entity.Agent
}

func (m *MockAgentStore) ListAgentsByWorkspace(_ context.Context, workspaceID string) ([]entity.Agent, error) {
	var out []entity.Agent
	for _, a := range m.Agents {
		if a.WorkspaceID == workspaceID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *MockAgentStore) GetAgent(_ context.Context, agentID string) (*entity.Agent, error) {
	for i := range m.Agents {
		if m.Agents[i].AgentID == agentID {
			return &m.Agents[i], nil
		}
	}
	return nil, nil
}

func (m *MockAgentStore) CreateAgent(_ context.Context, workspaceID, name, description, instructions string) (*entity.Agent, error) {
	a := entity.Agent{
		AgentID:      fmt.Sprintf("a_%d", len(m.Agents)+1),
		WorkspaceID:  workspaceID,
		Name:         name,
		Description:  description,
		Instructions: instructions,
		CreatedAt:    time.Now().Unix(),
	}
	m.Agents = append(m.Agents, a)
	return &m.Agents[len(m.Agents)-1], nil
}

func (m *MockAgentStore) UpdateAgent(_ context.Context, agentID, workspaceID, name, description, instructions string) (*entity.Agent, error) {
	for i := range m.Agents {
		if m.Agents[i].AgentID == agentID && m.Agents[i].WorkspaceID == workspaceID {
			m.Agents[i].Name = name
			m.Agents[i].Description = description
			m.Agents[i].Instructions = instructions
			return &m.Agents[i], nil
		}
	}
	return nil, nil
}

func (m *MockAgentStore) DeleteAgent(_ context.Context, agentID, workspaceID string) error {
	for i := range m.Agents {
		if m.Agents[i].AgentID == agentID && m.Agents[i].WorkspaceID == workspaceID {
			m.Agents = append(m.Agents[:i], m.Agents[i+1:]...)
			return nil
		}
	}
	return gorm.ErrRecordNotFound
}

// MockArtifactStorage is an in-memory blob.ArtifactStorage for tests.
type MockArtifactStorage struct {
	Results map[string][]byte
	Files   map[string][]byte
}

// NewMockArtifactStorage returns a MockArtifactStorage ready for use.
func NewMockArtifactStorage() *MockArtifactStorage {
	return &MockArtifactStorage{Results: make(map[string][]byte)}
}

func (m *MockArtifactStorage) PutResult(_ context.Context, workspaceID, chatID, chatRunID string, data []byte) error {
	m.Results[workspaceID+"/"+chatID+"/"+chatRunID] = append([]byte(nil), data...)
	return nil
}

func (m *MockArtifactStorage) GetResult(_ context.Context, workspaceID, chatID, chatRunID string) ([]byte, error) {
	key := workspaceID + "/" + chatID + "/" + chatRunID
	if data, ok := m.Results[key]; ok {
		return data, nil
	}
	return nil, blob.ErrNotFound
}

func (m *MockArtifactStorage) PutArtifactFile(_ context.Context, workspaceID, chatID, chatRunID, relPath string, r io.Reader) error {
	if m.Files == nil {
		m.Files = make(map[string][]byte)
	}
	key := workspaceID + "/" + chatID + "/" + chatRunID + "/" + relPath
	data, _ := io.ReadAll(r)
	m.Files[key] = data
	return nil
}

func (m *MockArtifactStorage) GetArtifactFile(_ context.Context, workspaceID, chatID, chatRunID, relPath string) ([]byte, error) {
	if m.Files == nil {
		return nil, blob.ErrNotFound
	}
	key := workspaceID + "/" + chatID + "/" + chatRunID + "/" + relPath
	if data, ok := m.Files[key]; ok {
		return data, nil
	}
	return nil, blob.ErrNotFound
}
