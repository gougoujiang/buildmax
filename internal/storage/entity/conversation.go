package entity

import (
	"context"
	"errors"
	"time"

	"buildmax/internal/util"
	"gorm.io/gorm"
)

// CreateConversation creates a new Tier 1 conversation. Returns the conversation with conversation_id set.
func (s *Store) CreateConversation(ctx context.Context, userID, channel, createdBy string) (*Conversation, error) {
	now := time.Now().Unix()
	conv := &Conversation{
		ConversationID: util.NewPrefixedID(util.PrefixConversation),
		UserID:         userID,
		Channel:        channel,
		CreatedBy:      createdBy,
		CreatedAt:      now,
	}
	if err := s.db.WithContext(ctx).Create(conv).Error; err != nil {
		return nil, err
	}
	return conv, nil
}

// GetConversation returns the conversation by conversation_id, or (nil, nil) if not found.
func (s *Store) GetConversation(ctx context.Context, conversationID string) (*Conversation, error) {
	var c Conversation
	err := s.db.WithContext(ctx).Where("conversation_id = ?", conversationID).First(&c).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// ListConversationsByUser returns conversations for the user ordered by created_at DESC.
// total is the total count of matching conversations (ignoring limit/offset).
func (s *Store) ListConversationsByUser(ctx context.Context, userID string, limit, offset int) ([]Conversation, int, error) {
	var total int64
	if err := s.db.WithContext(ctx).Model(&Conversation{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []Conversation
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).Offset(offset).
		Find(&list).Error
	return list, int(total), err
}

// UpdateConversationTitle sets the title for the conversation (e.g. after first-round LLM generation).
func (s *Store) UpdateConversationTitle(ctx context.Context, conversationID, title string) error {
	return s.db.WithContext(ctx).Model(&Conversation{}).
		Where("conversation_id = ?", conversationID).
		Update("title", title).Error
}
