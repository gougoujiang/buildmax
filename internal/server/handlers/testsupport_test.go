package handlers

// Copies of the fixtures the split left in other packages. Duplicated rather
// than imported: a test helper crossing a package boundary makes the test
// boundary softer than the code's.

import (
	"context"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

type mockConversationMessageStore struct {
	messages []model.ConversationMessage
}

func (m *mockConversationMessageStore) AppendMessage(ctx context.Context, in model.AppendMessageInput) (*model.ConversationMessage, error) {
	msg := model.ConversationMessage{
		ID:                "cm_mock",
		ConversationID:    in.ConversationID,
		Role:              in.Role,
		Content:           in.Content,
		Channel:           in.Channel,
		ToolCallID:        in.ToolCallID,
		ToolCallsJSON:     in.ToolCallsJSON,
		ProviderStateJSON: in.ProviderStateJSON,
	}
	m.messages = append(m.messages, msg)
	return &msg, nil
}
func (m *mockConversationMessageStore) ListMessages(ctx context.Context, conversationID string) ([]model.ConversationMessage, error) {
	var out []model.ConversationMessage
	for _, msg := range m.messages {
		if msg.ConversationID == conversationID {
			out = append(out, msg)
		}
	}
	return out, nil
}
