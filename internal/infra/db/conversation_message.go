package db

import (
	"buildmax/internal/core/model"
	"context"
	"time"

	"buildmax/internal/util"
)

// AppendMessage appends one message to the conversation. channel is stored for incoming turns such as
// role "user" and role "system"; tool_call_id is stored when role is "tool"; tool_calls (JSON) is
// stored when role is "assistant" with tool calls. Returns the created message.
func (s *Store) AppendMessage(ctx context.Context, conversationID, role, content string, channel *string, toolCallID *string, toolCallsJSON *string) (*model.ConversationMessage, error) {
	msg := &model.ConversationMessage{
		ConversationMessageID: util.NewPrefixedID(util.PrefixConversationMessage),
		ConversationID:        conversationID,
		Role:                  role,
		Content:               content,
		Channel:               channel,
		ToolCallID:            toolCallID,
		ToolCallsJSON:         toolCallsJSON,
		CreatedAt:             time.Now().Unix(),
	}
	if err := s.db.WithContext(ctx).Create(msg).Error; err != nil {
		return nil, err
	}
	return msg, nil
}

// ListMessages returns all messages for the conversation ordered by created_at ASC.
func (s *Store) ListMessages(ctx context.Context, conversationID string) ([]model.ConversationMessage, error) {
	var list []model.ConversationMessage
	err := s.db.WithContext(ctx).Where("conversation_id = ?", conversationID).
		Order("created_at ASC").
		Find(&list).Error
	return list, err
}
