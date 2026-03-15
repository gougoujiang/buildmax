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

// MockTaskStore is an in-memory TaskStore for tests.
type MockTaskStore struct {
	List      []entity.Chat
	ListErr   error
	Create    *entity.Chat
	CreateErr error
}

func (m *MockTaskStore) ListChatsByConversation(_ context.Context, conversationID string, order string) ([]entity.Chat, error) {
	list, _, err := m.ListChatsByConversationPaginated(context.Background(), conversationID, false, 0, 0)
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

func (m *MockTaskStore) ListChatsByConversationPaginated(_ context.Context, conversationID string, executedOnly bool, limit, offset int) ([]entity.Chat, int, error) {
	if m.ListErr != nil {
		return nil, 0, m.ListErr
	}
	var filtered []entity.Chat
	for _, c := range m.List {
		if c.ConversationID != conversationID {
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

func (m *MockTaskStore) CreateChat(_ context.Context, in *entity.CreateChatInput) (*entity.Chat, error) {
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
		ConversationID: in.ConversationID,
		Status:         "PENDING",
		Input:          in.Input,
		Title:          in.Title,
		CreatedBy:      in.CreatedBy,
		CreatedAt:      12345,
		AgentID:        in.AgentID,
	}
	return c, nil
}

func (m *MockTaskStore) GetChatBySessionID(_ context.Context, sessionID string) (*entity.Chat, error) {
	for i := range m.List {
		if m.List[i].SessionID != nil && *m.List[i].SessionID == sessionID {
			return &m.List[i], nil
		}
	}
	return nil, nil
}

func (m *MockTaskStore) UpdateChat(_ context.Context, in entity.UpdateChatInput) error {
	for i := range m.List {
		if m.List[i].ChatID == in.ChatID {
			m.List[i].Status = in.Status
			if in.StartedAt != nil {
				m.List[i].StartedAt = in.StartedAt
			}
			if in.EndedAt != nil {
				m.List[i].EndedAt = in.EndedAt
			}
			if in.Output != nil {
				m.List[i].Output = in.Output
			}
			if in.ErrorMessage != nil {
				m.List[i].ErrorMessage = in.ErrorMessage
			}
			if in.SessionID != nil {
				m.List[i].SessionID = in.SessionID
			}
			return nil
		}
	}
	return nil
}

func (m *MockTaskStore) ClaimChat(_ context.Context, in entity.ClaimChatInput) (bool, error) {
	for i := range m.List {
		if m.List[i].ChatID == in.ChatID && m.List[i].Status == in.ExpectedStatus {
			m.List[i].Status = in.NewStatus
			if in.StartedAt != nil {
				m.List[i].StartedAt = in.StartedAt
			}
			if in.EndedAt != nil {
				m.List[i].EndedAt = in.EndedAt
			}
			if in.Output != nil {
				m.List[i].Output = in.Output
			}
			if in.ErrorMessage != nil {
				m.List[i].ErrorMessage = in.ErrorMessage
			}
			if in.SessionID != nil {
				m.List[i].SessionID = in.SessionID
			}
			return true, nil
		}
	}
	return false, nil
}

func (m *MockTaskStore) GetChat(_ context.Context, chatID string) (*entity.Chat, error) {
	for i := range m.List {
		if m.List[i].ChatID == chatID {
			return &m.List[i], nil
		}
	}
	return nil, nil
}

// MockTaskRunStore is an in-memory TaskRunStore for tests.
type MockTaskRunStore struct {
	Runs     []entity.TaskRun
	ChatList []entity.Chat
}

func (m *MockTaskRunStore) CreateTaskRun(_ context.Context, chatID, input, createdBy string) (*entity.TaskRun, error) {
	return nil, nil
}
func (m *MockTaskRunStore) GetNextPendingTaskRun(_ context.Context) (*entity.TaskRun, error) {
	return nil, nil
}
func (m *MockTaskRunStore) GetTaskRun(_ context.Context, chatRunID string) (*entity.TaskRun, error) {
	for i := range m.Runs {
		if m.Runs[i].TaskRunID == chatRunID {
			return &m.Runs[i], nil
		}
	}
	return nil, nil
}
func (m *MockTaskRunStore) GetTaskRunWithChat(_ context.Context, chatRunID string) (*entity.TaskRun, *entity.Chat, error) {
	var run *entity.TaskRun
	for i := range m.Runs {
		if m.Runs[i].TaskRunID == chatRunID {
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
func (m *MockTaskRunStore) ClaimTaskRun(ctx context.Context, in entity.ClaimTaskRunInput) (bool, error) {
	for i := range m.Runs {
		if m.Runs[i].TaskRunID == in.TaskRunID && m.Runs[i].Status == string(in.ExpectedStatus) {
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
func (m *MockTaskRunStore) UpdateRun(ctx context.Context, in entity.UpdateTaskRunInput) error {
	for i := range m.Runs {
		if m.Runs[i].TaskRunID == in.TaskRunID {
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
func (m *MockTaskRunStore) UpdateTaskRunWorkerInfo(_ context.Context, chatRunID, workerType string, k8sJobName *string, k8sJobCreatedAt *int64) error {
	return nil
}
func (m *MockTaskRunStore) OnRunComplete(_ context.Context, chatRunID string, relativePaths []string) error {
	return nil
}
func (m *MockTaskRunStore) SyncTaskFromRun(_ context.Context, chatRunID string) error {
	return nil
}

// MockRunOutputLister is an in-memory RunOutputLister for tests.
type MockRunOutputLister struct {
	List        []entity.ArtifactWithChat
	ListErr     error
	OutputFiles map[string][]entity.TaskRunArtifact
}

func (m *MockRunOutputLister) ListRunOutputsByConversation(_ context.Context, conversationID string, chatID *string) ([]entity.ArtifactWithChat, error) {
	if m.ListErr != nil {
		return nil, m.ListErr
	}
	return m.List, nil
}

func (m *MockRunOutputLister) GetTaskRunOutputFiles(_ context.Context, chatRunID string) ([]entity.TaskRunArtifact, error) {
	if m.OutputFiles != nil {
		return m.OutputFiles[chatRunID], nil
	}
	return nil, nil
}

// MockAgentStore is an in-memory AgentStore for tests.
type MockAgentStore struct {
	Agents []entity.Agent
}

func (m *MockAgentStore) ListAgentsByUser(_ context.Context, userID string) ([]entity.Agent, error) {
	var out []entity.Agent
	for _, a := range m.Agents {
		if a.UserID == userID {
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

func (m *MockAgentStore) CreateAgent(_ context.Context, userID, name, description, instructions string) (*entity.Agent, error) {
	a := entity.Agent{
		AgentID:      fmt.Sprintf("a_%d", len(m.Agents)+1),
		UserID:       userID,
		Name:         name,
		Description:  description,
		Instructions: instructions,
		CreatedAt:    time.Now().Unix(),
	}
	m.Agents = append(m.Agents, a)
	return &m.Agents[len(m.Agents)-1], nil
}

func (m *MockAgentStore) UpdateAgent(_ context.Context, agentID, userID, name, description, instructions string) (*entity.Agent, error) {
	for i := range m.Agents {
		if m.Agents[i].AgentID == agentID && m.Agents[i].UserID == userID {
			m.Agents[i].Name = name
			m.Agents[i].Description = description
			m.Agents[i].Instructions = instructions
			return &m.Agents[i], nil
		}
	}
	return nil, nil
}

func (m *MockAgentStore) DeleteAgent(_ context.Context, agentID, userID string) error {
	for i := range m.Agents {
		if m.Agents[i].AgentID == agentID && m.Agents[i].UserID == userID {
			m.Agents = append(m.Agents[:i], m.Agents[i+1:]...)
			return nil
		}
	}
	return gorm.ErrRecordNotFound
}

// MockConversationStore is an in-memory ConversationStore for tests.
type MockConversationStore struct {
	Conversations []entity.Conversation
}

func (m *MockConversationStore) CreateConversation(_ context.Context, userID, channel, createdBy string) (*entity.Conversation, error) {
	conv := entity.Conversation{
		ConversationID: fmt.Sprintf("v_%d", len(m.Conversations)+1),
		UserID:         userID,
		Channel:        channel,
		CreatedBy:      createdBy,
		CreatedAt:      time.Now().Unix(),
	}
	m.Conversations = append(m.Conversations, conv)
	return &m.Conversations[len(m.Conversations)-1], nil
}

func (m *MockConversationStore) GetConversation(_ context.Context, conversationID string) (*entity.Conversation, error) {
	for i := range m.Conversations {
		if m.Conversations[i].ConversationID == conversationID {
			return &m.Conversations[i], nil
		}
	}
	return nil, nil
}

func (m *MockConversationStore) ListConversationsByUser(_ context.Context, userID string, limit, offset int) ([]entity.Conversation, int, error) {
	var out []entity.Conversation
	for _, conv := range m.Conversations {
		if conv.UserID == userID {
			out = append(out, conv)
		}
	}
	total := len(out)
	if offset > total {
		return []entity.Conversation{}, total, nil
	}
	if limit <= 0 || offset+limit > total {
		limit = total - offset
	}
	return out[offset : offset+limit], total, nil
}

func (m *MockConversationStore) UpdateConversationTitle(_ context.Context, conversationID, title string) error {
	for i := range m.Conversations {
		if m.Conversations[i].ConversationID == conversationID {
			m.Conversations[i].Title = title
			return nil
		}
	}
	return nil
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

func (m *MockArtifactStorage) PutResult(_ context.Context, ref blob.RunRef, data []byte) error {
	m.Results[ref.UserID+"/"+ref.ConversationID+"/"+ref.ChatID+"/"+ref.TaskRunID] = append([]byte(nil), data...)
	return nil
}

func (m *MockArtifactStorage) GetResult(_ context.Context, ref blob.RunRef) ([]byte, error) {
	key := ref.UserID + "/" + ref.ConversationID + "/" + ref.ChatID + "/" + ref.TaskRunID
	if data, ok := m.Results[key]; ok {
		return data, nil
	}
	return nil, blob.ErrNotFound
}

func (m *MockArtifactStorage) PutArtifactFile(_ context.Context, ref blob.RunObjectRef, r io.Reader) error {
	if m.Files == nil {
		m.Files = make(map[string][]byte)
	}
	key := ref.UserID + "/" + ref.ConversationID + "/" + ref.ChatID + "/" + ref.TaskRunID + "/" + ref.RelPath
	data, _ := io.ReadAll(r)
	m.Files[key] = data
	return nil
}

func (m *MockArtifactStorage) GetArtifactFile(_ context.Context, ref blob.RunObjectRef) ([]byte, error) {
	if m.Files == nil {
		return nil, blob.ErrNotFound
	}
	key := ref.UserID + "/" + ref.ConversationID + "/" + ref.ChatID + "/" + ref.TaskRunID + "/" + ref.RelPath
	if data, ok := m.Files[key]; ok {
		return data, nil
	}
	return nil, blob.ErrNotFound
}
