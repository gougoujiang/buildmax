package entity

import (
	"context"
	"errors"
	"time"

	"buildmax/internal/util"
	"gorm.io/gorm"
)

// CreateChatRun creates a new run (PENDING). Returns ErrRunInProgress if the chat has any run in PENDING, SCHEDULED, or RUNNING.
func (s *Store) CreateChatRun(ctx context.Context, chatID, input, createdBy string) (*ChatRun, error) {
	var inProgress int64
	err := s.db.WithContext(ctx).Model(&ChatRun{}).Where("chat_id = ? AND status IN ?", chatID, []string{"PENDING", "SCHEDULED", "RUNNING"}).Count(&inProgress).Error
	if err != nil {
		return nil, err
	}
	if inProgress > 0 {
		return nil, ErrRunInProgress
	}
	run := &ChatRun{
		ChatRunID: util.NewPrefixedID(util.PrefixChatRun),
		ChatID:    chatID,
		Input:     input,
		Status:    "PENDING",
		CreatedAt: time.Now().Unix(),
	}
	if err := s.db.WithContext(ctx).Create(run).Error; err != nil {
		return nil, err
	}
	return run, nil
}

// GetNextPendingChatRun returns the oldest run with status PENDING (by created_at), or (nil, nil) if none.
func (s *Store) GetNextPendingChatRun(ctx context.Context) (*ChatRun, error) {
	var r ChatRun
	err := s.db.WithContext(ctx).Where("status = ?", "PENDING").Order("created_at ASC").First(&r).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

// GetChatRun returns the run by chat_run_id, or (nil, nil) if not found.
func (s *Store) GetChatRun(ctx context.Context, chatRunID string) (*ChatRun, error) {
	var r ChatRun
	err := s.db.WithContext(ctx).Where("chat_run_id = ?", chatRunID).First(&r).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

// GetChatRunWithChat returns the run and its chat, or (nil, nil, nil) if run not found.
func (s *Store) GetChatRunWithChat(ctx context.Context, chatRunID string) (*ChatRun, *Chat, error) {
	run, err := s.GetChatRun(ctx, chatRunID)
	if err != nil || run == nil {
		return nil, nil, err
	}
	chat, err := s.GetChat(ctx, run.ChatID)
	if err != nil || chat == nil {
		return run, nil, err
	}
	return run, chat, nil
}

// ClaimChatRun atomically updates a run when current status matches ExpectedStatus.
func (s *Store) ClaimChatRun(ctx context.Context, in ClaimChatRunInput) (bool, error) {
	updates := map[string]interface{}{"status": string(in.NewStatus)}
	if in.StartedAt != nil {
		updates["started_at"] = *in.StartedAt
	}
	if in.EndedAt != nil {
		updates["ended_at"] = *in.EndedAt
	}
	if in.Output != nil {
		updates["output"] = *in.Output
	}
	if in.ErrorMessage != nil {
		updates["error_message"] = *in.ErrorMessage
	}
	if in.SessionID != nil {
		updates["session_id"] = *in.SessionID
	}
	result := s.db.WithContext(ctx).Model(&ChatRun{}).Where("chat_run_id = ? AND status = ?", in.ChatRunID, string(in.ExpectedStatus)).Updates(updates)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 1 && (in.NewStatus == RunStatusPending || in.NewStatus == RunStatusScheduled || in.NewStatus == RunStatusRunning) {
		_ = s.syncChatStatusFromRun(ctx, in.ChatRunID, string(in.NewStatus), in.StartedAt, nil)
	}
	return result.RowsAffected == 1, nil
}

// UpdateRun updates a run's status and optional fields.
func (s *Store) UpdateRun(ctx context.Context, in UpdateChatRunInput) error {
	updates := map[string]interface{}{"status": string(in.Status)}
	if in.StartedAt != nil {
		updates["started_at"] = *in.StartedAt
	}
	if in.EndedAt != nil {
		updates["ended_at"] = *in.EndedAt
	}
	if in.Output != nil {
		updates["output"] = *in.Output
	}
	if in.ErrorMessage != nil {
		updates["error_message"] = *in.ErrorMessage
	}
	if in.SessionID != nil {
		updates["session_id"] = *in.SessionID
	}
	if in.PromptTokens != nil {
		updates["prompt_tokens"] = *in.PromptTokens
	}
	if in.CompletionTokens != nil {
		updates["completion_tokens"] = *in.CompletionTokens
	}
	if err := s.db.WithContext(ctx).Model(&ChatRun{}).Where("chat_run_id = ?", in.ChatRunID).Updates(updates).Error; err != nil {
		return err
	}
	if in.Status == RunStatusPending || in.Status == RunStatusScheduled || in.Status == RunStatusRunning {
		_ = s.syncChatStatusFromRun(ctx, in.ChatRunID, string(in.Status), in.StartedAt, in.EndedAt)
	}
	return nil
}

// syncChatStatusFromRun updates the chat row (denormalized status) for the run's chat_id to match the run's status.
func (s *Store) syncChatStatusFromRun(ctx context.Context, chatRunID, status string, startedAt, endedAt *int64) error {
	var run ChatRun
	if err := s.db.WithContext(ctx).Where("chat_run_id = ?", chatRunID).Select("chat_id").First(&run).Error; err != nil {
		return err
	}
	chatUpdates := map[string]interface{}{"status": status}
	if status == "PENDING" {
		chatUpdates["started_at"] = nil
		chatUpdates["ended_at"] = nil
	} else {
		if startedAt != nil {
			chatUpdates["started_at"] = *startedAt
		}
		if endedAt != nil {
			chatUpdates["ended_at"] = *endedAt
		}
	}
	return s.db.WithContext(ctx).Model(&Chat{}).Where("chat_id = ?", run.ChatID).Updates(chatUpdates).Error
}

// UpdateChatRunWorkerInfo updates worker_type, k8s_job_name, k8s_job_created_at for the run.
func (s *Store) UpdateChatRunWorkerInfo(ctx context.Context, chatRunID, workerType string, k8sJobName *string, k8sJobCreatedAt *int64) error {
	updates := map[string]interface{}{"worker_type": workerType}
	if k8sJobName != nil {
		updates["k8s_job_name"] = *k8sJobName
	}
	if k8sJobCreatedAt != nil {
		updates["k8s_job_created_at"] = *k8sJobCreatedAt
	}
	return s.db.WithContext(ctx).Model(&ChatRun{}).Where("chat_run_id = ?", chatRunID).Updates(updates).Error
}

// OnRunComplete creates chat_run_artifact rows (one per relativePath) and updates chat denormalized fields.
func (s *Store) OnRunComplete(ctx context.Context, chatRunID string, relativePaths []string) error {
	if len(relativePaths) == 0 {
		relativePaths = []string{"result.md"}
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run ChatRun
		if err := tx.Where("chat_run_id = ?", chatRunID).First(&run).Error; err != nil {
			return err
		}
		for _, relPath := range relativePaths {
			if err := tx.Create(&ChatRunArtifact{ChatRunID: chatRunID, RelativePath: relPath}).Error; err != nil {
				return err
			}
		}
		updates := map[string]interface{}{
			"last_run_id":   chatRunID,
			"status":        run.Status,
			"output":        run.Output,
			"started_at":    run.StartedAt,
			"ended_at":      run.EndedAt,
			"error_message": run.ErrorMessage,
		}
		if run.SessionID != nil {
			updates["session_id"] = *run.SessionID
		}
		return tx.Model(&Chat{}).Where("chat_id = ?", run.ChatID).Updates(updates).Error
	})
}

// SyncChatFromRun updates chat denormalized fields and last_run_id from the run. Use when run ends with FAILED (no artifact).
func (s *Store) SyncChatFromRun(ctx context.Context, chatRunID string) error {
	var run ChatRun
	if err := s.db.WithContext(ctx).Where("chat_run_id = ?", chatRunID).First(&run).Error; err != nil {
		return err
	}
	updates := map[string]interface{}{
		"last_run_id":   chatRunID,
		"status":        run.Status,
		"output":        run.Output,
		"started_at":    run.StartedAt,
		"ended_at":      run.EndedAt,
		"error_message": run.ErrorMessage,
	}
	if run.SessionID != nil {
		updates["session_id"] = *run.SessionID
	}
	return s.db.WithContext(ctx).Model(&Chat{}).Where("chat_id = ?", run.ChatID).Updates(updates).Error
}
