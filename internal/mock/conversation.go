package mock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// MockConversationStore is an in-memory ConversationStore for tests.
type MockConversationStore struct {
	Conversations []model.Conversation
}

func (m *MockConversationStore) CreateConversation(_ context.Context, userID, channel, createdBy string) (*model.Conversation, error) {
	return m.CreateConversationInTeam(context.Background(), "tm_personal", userID, channel, createdBy)
}

func (m *MockConversationStore) CreateConversationInTeam(_ context.Context, teamID, userID, channel, createdBy string) (*model.Conversation, error) {
	conv := model.Conversation{
		ID:        fmt.Sprintf("v_%d", len(m.Conversations)+1),
		UserID:    userID,
		TeamID:    teamID,
		Channel:   channel,
		CreatedBy: createdBy,
		CreatedAt: time.Now().UTC(),
	}
	m.Conversations = append(m.Conversations, conv)
	return &m.Conversations[len(m.Conversations)-1], nil
}

func (m *MockConversationStore) GetConversation(_ context.Context, conversationID string) (*model.Conversation, error) {
	for i := range m.Conversations {
		if m.Conversations[i].ID == conversationID {
			return &m.Conversations[i], nil
		}
	}
	return nil, nil
}

func (m *MockConversationStore) ListConversationsByUser(_ context.Context, userID string, limit, offset int) ([]model.Conversation, int, error) {
	var out []model.Conversation
	for _, conv := range m.Conversations {
		if conv.UserID == userID {
			out = append(out, conv)
		}
	}
	total := len(out)
	if offset > total {
		return []model.Conversation{}, total, nil
	}
	if limit <= 0 || offset+limit > total {
		limit = total - offset
	}
	return out[offset : offset+limit], total, nil
}

func (m *MockConversationStore) ListConversationsByTeam(_ context.Context, teamID string, limit, offset int) ([]model.Conversation, int, error) {
	var out []model.Conversation
	for _, conv := range m.Conversations {
		if conv.TeamID == teamID && !syntheticChannel(conv.Channel) {
			out = append(out, conv)
		}
	}
	total := len(out)
	if offset > total {
		return []model.Conversation{}, total, nil
	}
	if limit <= 0 || offset+limit > total {
		limit = total - offset
	}
	return out[offset : offset+limit], total, nil
}

func (m *MockConversationStore) UpdateConversationTitle(_ context.Context, conversationID, title string) error {
	for i := range m.Conversations {
		if m.Conversations[i].ID == conversationID {
			m.Conversations[i].Title = title
			return nil
		}
	}
	return nil
}

// MockConversationMessageStore is an in-memory ConversationMessageStore.
//
// Every appended message gets its own handle. That matters more than it looks:
// a run records the message that asked for it, and a store handing out one
// shared ID would let that assertion pass on any message at all.
type MockConversationMessageStore struct {
	mu       sync.Mutex
	Messages []model.ConversationMessage
}

func (m *MockConversationMessageStore) AppendMessage(_ context.Context, in model.AppendMessageInput) (*model.ConversationMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	msg := model.ConversationMessage{
		ID:                fmt.Sprintf("cm_mock_%d", len(m.Messages)+1),
		ConversationID:    in.ConversationID,
		Role:              in.Role,
		Content:           in.Content,
		Channel:           in.Channel,
		ToolCallID:        in.ToolCallID,
		ToolCallsJSON:     in.ToolCallsJSON,
		ProviderStateJSON: in.ProviderStateJSON,
		PartsJSON:         in.PartsJSON,
		CreatedAt:         seqTime(len(m.Messages) + 1),
	}
	m.Messages = append(m.Messages, msg)
	return &msg, nil
}

func (m *MockConversationMessageStore) ListMessages(_ context.Context, conversationID string) ([]model.ConversationMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.ConversationMessage, 0, len(m.Messages))
	for _, msg := range m.Messages {
		if msg.ConversationID == conversationID {
			out = append(out, msg)
		}
	}
	return out, nil
}

func (m *MockConversationMessageStore) GetMessage(_ context.Context, messageID string) (*model.ConversationMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Messages {
		if m.Messages[i].ID == messageID {
			msg := m.Messages[i]
			return &msg, nil
		}
	}
	return nil, nil
}

// syntheticChannel mirrors the store: a conversation nobody holds is not in the
// list. A double that returned them would let a regression pass here.
func syntheticChannel(channel string) bool {
	for _, c := range model.SyntheticChannels() {
		if c == channel {
			return true
		}
	}
	return false
}
