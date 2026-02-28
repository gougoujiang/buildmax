package entity

import (
	"context"
	"errors"
	"time"

	"buildmax/internal/util"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ListChatsByWorkspace returns chats in the workspace, ordered by created_at.
// order is "asc" (oldest first) or "desc" (latest first); default "desc".
func (s *Store) ListChatsByWorkspace(ctx context.Context, workspaceID string, order string) ([]Chat, error) {
	var list []Chat
	q := s.db.WithContext(ctx).Where("workspace_id = ?", workspaceID)
	if order == "asc" {
		q = q.Order("created_at ASC")
	} else {
		q = q.Order("created_at DESC")
	}
	err := q.Find(&list).Error
	return list, err
}

// ListChatsByWorkspacePaginated returns chats with optional executed_only filter, ordered by created_at DESC.
// executedOnly: when true, only chats that have been run (last_run_id IS NOT NULL) are returned.
// total is the total number of matching chats (ignoring limit/offset).
func (s *Store) ListChatsByWorkspacePaginated(ctx context.Context, workspaceID string, executedOnly bool, limit, offset int) ([]Chat, int, error) {
	q := s.db.WithContext(ctx).Model(&Chat{}).Where("workspace_id = ?", workspaceID)
	if executedOnly {
		q = q.Where("last_run_id IS NOT NULL AND last_run_id != ''")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []Chat
	q = s.db.WithContext(ctx).Where("workspace_id = ?", workspaceID)
	if executedOnly {
		q = q.Where("last_run_id IS NOT NULL AND last_run_id != ''")
	}
	err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&list).Error
	return list, int(total), err
}

// GetChat returns the chat by chat_id, or (nil, nil) if not found.
func (s *Store) GetChat(ctx context.Context, chatID string) (*Chat, error) {
	var c Chat
	err := s.db.WithContext(ctx).Where("chat_id = ?", chatID).First(&c).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// GetChatBySessionID returns the chat with the given session_id, or (nil, nil) if not found.
func (s *Store) GetChatBySessionID(ctx context.Context, sessionID string) (*Chat, error) {
	var c Chat
	err := s.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&c).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// CreateChat creates a new chat and its first ChatRun (PENDING) in one transaction. Returns the chat with last_run_id and session_id set.
// titlePromptTokens and titleCompletionTokens record LLM usage for title generation (0 when title was truncated input).
// agentID is optional; when set, the chat is associated with that workspace agent.
func (s *Store) CreateChat(ctx context.Context, workspaceID, input, title, createdBy string, titlePromptTokens, titleCompletionTokens int, agentID *string) (*Chat, error) {
	now := time.Now().Unix()
	chatID := util.NewPrefixedID(util.PrefixChat)
	chatRunID := util.NewPrefixedID(util.PrefixChatRun)
	sessionID := uuid.New().String() // UUID for buildmax CLI (session not exposed to user)
	c := &Chat{
		ChatID:                chatID,
		WorkspaceID:            workspaceID,
		Status:                 "PENDING",
		Input:                  input,
		Title:                  title,
		TitlePromptTokens:      titlePromptTokens,
		TitleCompletionTokens:  titleCompletionTokens,
		CreatedBy:              createdBy,
		CreatedAt:              now,
		LastRunID:              &chatRunID,
		SessionID:              &sessionID,
		AgentID:                agentID,
	}
	run := &ChatRun{
		ChatRunID:  chatRunID,
		ChatID:     chatID,
		Input:      input,
		Status:     "PENDING",
		CreatedAt:  now,
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(c).Error; err != nil {
			return err
		}
		return tx.Create(run).Error
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

// UpdateChatStatus updates a chat's status and optional fields.
// Only non-nil pointer fields are written; status is always set.
func (s *Store) UpdateChatStatus(ctx context.Context, chatID, status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) error {
	updates := map[string]interface{}{"status": status}
	if startedAt != nil {
		updates["started_at"] = *startedAt
	}
	if endedAt != nil {
		updates["ended_at"] = *endedAt
	}
	if output != nil {
		updates["output"] = *output
	}
	if errorMessage != nil {
		updates["error_message"] = *errorMessage
	}
	if sessionID != nil {
		updates["session_id"] = *sessionID
	}
	return s.db.WithContext(ctx).Model(&Chat{}).Where("chat_id = ?", chatID).Updates(updates).Error
}

// UpdateChatStatusIf updates a chat's status and optional fields only when current status equals expectedStatus.
// Returns updated = (exactly one row was updated). Used for atomic claim (e.g. PENDING→SCHEDULED, SCHEDULED→RUNNING).
func (s *Store) UpdateChatStatusIf(ctx context.Context, chatID, expectedStatus, newStatus string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) (bool, error) {
	updates := map[string]interface{}{"status": newStatus}
	if startedAt != nil {
		updates["started_at"] = *startedAt
	}
	if endedAt != nil {
		updates["ended_at"] = *endedAt
	}
	if output != nil {
		updates["output"] = *output
	}
	if errorMessage != nil {
		updates["error_message"] = *errorMessage
	}
	if sessionID != nil {
		updates["session_id"] = *sessionID
	}
	result := s.db.WithContext(ctx).Model(&Chat{}).Where("chat_id = ? AND status = ?", chatID, expectedStatus).Updates(updates)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}
