package mock

import (
	"context"
	"fmt"
	"time"

	"buildmax/internal/core/model"
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
		ConversationID: fmt.Sprintf("v_%d", len(m.Conversations)+1),
		UserID:         userID,
		TeamID:         teamID,
		Channel:        channel,
		CreatedBy:      createdBy,
		CreatedAt:      time.Now().Unix(),
	}
	m.Conversations = append(m.Conversations, conv)
	return &m.Conversations[len(m.Conversations)-1], nil
}

func (m *MockConversationStore) GetConversation(_ context.Context, conversationID string) (*model.Conversation, error) {
	for i := range m.Conversations {
		if m.Conversations[i].ConversationID == conversationID {
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
		if conv.TeamID == teamID {
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
		if m.Conversations[i].ConversationID == conversationID {
			m.Conversations[i].Title = title
			return nil
		}
	}
	return nil
}
