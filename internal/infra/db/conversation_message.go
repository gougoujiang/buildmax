package db

import (
	"context"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"time"

	"github.com/gougoujiang/buildmax/internal/util"
)

type conversationMessageRow struct {
	ID                    uint    `gorm:"primaryKey;autoIncrement"`
	ConversationMessageID string  `gorm:"type:varchar(64);uniqueIndex;not null"`
	ConversationID        string  `gorm:"type:varchar(64);not null;index"`
	Role                  string  `gorm:"type:varchar(16);not null"`
	Content               string  `gorm:"type:text;not null"`
	Channel               *string `gorm:"type:varchar(32)"`
	ToolCallID            *string `gorm:"type:varchar(64);column:tool_call_id"`
	ToolCallsJSON         *string `gorm:"type:text;column:tool_calls"`
	ProviderStateJSON     *string `gorm:"type:text;column:provider_state"`
	PartsJSON             *string `gorm:"type:mediumtext;column:parts"`
	CreatedAt             int64   `gorm:"autoCreateTime"`
}

func (conversationMessageRow) TableName() string { return "conversation_message" }

func toConversationMessage(row *conversationMessageRow) *model.ConversationMessage {
	if row == nil {
		return nil
	}
	return &model.ConversationMessage{
		ID:                row.ConversationMessageID,
		ConversationID:    row.ConversationID,
		Role:              row.Role,
		Content:           row.Content,
		Channel:           row.Channel,
		ToolCallID:        row.ToolCallID,
		ToolCallsJSON:     row.ToolCallsJSON,
		ProviderStateJSON: row.ProviderStateJSON,
		PartsJSON:         row.PartsJSON,
		CreatedAt:         row.CreatedAt,
	}
}

func toConversationMessages(rows []conversationMessageRow) []model.ConversationMessage {
	out := make([]model.ConversationMessage, len(rows))
	for i := range rows {
		out[i] = *toConversationMessage(&rows[i])
	}
	return out
}

func toConversationMessageRow(m *model.ConversationMessage) *conversationMessageRow {
	if m == nil {
		return nil
	}
	return &conversationMessageRow{
		ConversationMessageID: m.ID,
		ConversationID:        m.ConversationID,
		Role:                  m.Role,
		Content:               m.Content,
		Channel:               m.Channel,
		ToolCallID:            m.ToolCallID,
		ToolCallsJSON:         m.ToolCallsJSON,
		ProviderStateJSON:     m.ProviderStateJSON,
		PartsJSON:             m.PartsJSON,
		CreatedAt:             m.CreatedAt,
	}
}

// AppendMessage appends one message to the conversation. channel is stored for incoming turns such as
// role "user" and role "system"; tool_call_id is stored when role is "tool"; tool_calls (JSON) is
// stored when role is "assistant" with tool calls. Returns the created message.
func (s *Store) AppendMessage(ctx context.Context, in model.AppendMessageInput) (*model.ConversationMessage, error) {
	msg := &model.ConversationMessage{
		ID:                util.NewPrefixedID(util.PrefixConversationMessage),
		ConversationID:    in.ConversationID,
		Role:              in.Role,
		Content:           in.Content,
		Channel:           in.Channel,
		ToolCallID:        in.ToolCallID,
		ToolCallsJSON:     in.ToolCallsJSON,
		ProviderStateJSON: in.ProviderStateJSON,
		PartsJSON:         in.PartsJSON,
		CreatedAt:         time.Now().Unix(),
	}
	if err := s.db.WithContext(ctx).Create(toConversationMessageRow(msg)).Error; err != nil {
		return nil, err
	}
	return msg, nil
}

// ListMessages returns all messages for the conversation ordered by created_at ASC.
func (s *Store) ListMessages(ctx context.Context, conversationID string) ([]model.ConversationMessage, error) {
	var list []conversationMessageRow
	err := s.db.WithContext(ctx).Where("conversation_id = ?", conversationID).
		Order("created_at ASC").
		Find(&list).Error
	return toConversationMessages(list), err
}
