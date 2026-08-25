package db

import (
	"context"
	"errors"
	"time"

	coreconv "github.com/gougoujiang/buildmax/internal/core/conversation"

	"github.com/gougoujiang/buildmax/internal/util"
	"gorm.io/gorm"
)

type conversationMessageRow struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement"`
	PublicID string `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_conversation_message_public_id;not null"`
	// The composite index is what serves a transcript read: every listing is
	// one conversation's messages in created_at order.
	ConversationID    uint64    `gorm:"column:conversation_id;not null;index:idx_conversation_message_conversation,priority:1"`
	Role              string    `gorm:"type:varchar(16);not null"`
	Content           string    `gorm:"type:text;not null"`
	Channel           *string   `gorm:"type:varchar(32)"`
	ToolCallID        *string   `gorm:"type:varchar(64);column:tool_call_id"`
	ToolCallsJSON     *string   `gorm:"type:text;column:tool_calls"`
	ProviderStateJSON *string   `gorm:"type:text;column:provider_state"`
	PartsJSON         *string   `gorm:"type:mediumtext;column:parts"`
	CreatedAt         time.Time `gorm:"autoCreateTime;index:idx_conversation_message_conversation,priority:2"`
}

func (conversationMessageRow) TableName() string { return "conversation_message" }

// conversationMessageReadRow is the row plus its conversation's handle.
type conversationMessageReadRow struct {
	Row                  conversationMessageRow `gorm:"embedded"`
	ConversationPublicID string                 `gorm:"column:conversation_public_id"`
}

func (s *Store) conversationMessageSelect(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Model(&conversationMessageRow{}).
		Select("conversation_message.*, c.public_id AS conversation_public_id").
		Joins("INNER JOIN conversation c ON c.id = conversation_message.conversation_id")
}

func toConversationMessage(row *conversationMessageReadRow) *coreconv.Message {
	if row == nil {
		return nil
	}
	return &coreconv.Message{
		ID:                row.Row.PublicID,
		ConversationID:    row.ConversationPublicID,
		Role:              row.Row.Role,
		Content:           row.Row.Content,
		Channel:           row.Row.Channel,
		ToolCallID:        row.Row.ToolCallID,
		ToolCallsJSON:     row.Row.ToolCallsJSON,
		ProviderStateJSON: row.Row.ProviderStateJSON,
		PartsJSON:         row.Row.PartsJSON,
		CreatedAt:         row.Row.CreatedAt,
	}
}

func toConversationMessages(rows []conversationMessageReadRow) []coreconv.Message {
	out := make([]coreconv.Message, len(rows))
	for i := range rows {
		out[i] = *toConversationMessage(&rows[i])
	}
	return out
}

// AppendMessage appends one message to the conversation. channel is stored for incoming turns such as
// role "user" and role "system"; tool_call_id is stored when role is "tool"; tool_calls (JSON) is
// stored when role is "assistant" with tool calls. Returns the created message.
func (s *Store) AppendMessage(ctx context.Context, in coreconv.AppendInput) (*coreconv.Message, error) {
	convKey, err := lookupKey(ctx, s.db, "conversation", in.ConversationID)
	if err != nil {
		return nil, err
	}
	row := &conversationMessageRow{
		ConversationID:    convKey,
		Role:              in.Role,
		Content:           in.Content,
		Channel:           in.Channel,
		ToolCallID:        in.ToolCallID,
		ToolCallsJSON:     in.ToolCallsJSON,
		ProviderStateJSON: in.ProviderStateJSON,
		PartsJSON:         in.PartsJSON,
		CreatedAt:         time.Now().UTC(),
	}
	if err := createWithPublicID(ctx, s.db, "uq_conversation_message_public_id",
		func(id string) { row.PublicID = id }, row); err != nil {
		return nil, err
	}
	return toConversationMessage(&conversationMessageReadRow{
		Row: *row, ConversationPublicID: canonicalPublicID(in.ConversationID),
	}), nil
}

// GetMessage returns one message by handle, or (nil, nil) when there is none.
func (s *Store) GetMessage(ctx context.Context, messageID string) (*coreconv.Message, error) {
	id, ok := util.CanonicalPublicID(messageID)
	if !ok {
		return nil, nil
	}
	var row conversationMessageReadRow
	err := s.conversationMessageSelect(ctx).Where("conversation_message.public_id = ?", id).Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toConversationMessage(&row), nil
}

// ListMessages returns all messages for the conversation ordered by created_at ASC.
func (s *Store) ListMessages(ctx context.Context, conversationID string) ([]coreconv.Message, error) {
	id, ok := util.CanonicalPublicID(conversationID)
	if !ok {
		return nil, nil
	}
	var list []conversationMessageReadRow
	err := s.conversationMessageSelect(ctx).Where("c.public_id = ?", id).
		Order("conversation_message.created_at ASC").
		Find(&list).Error
	return toConversationMessages(list), err
}
