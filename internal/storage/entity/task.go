package entity

import (
	"context"
	"errors"
	"time"

	"buildmax/internal/util"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ListTasksByConversation returns tasks in the conversation, ordered by created_at.
// order is "asc" (oldest first) or "desc" (latest first); default "desc".
func (s *Store) ListTasksByConversation(ctx context.Context, conversationID string, order string) ([]Task, error) {
	var list []Task
	q := s.db.WithContext(ctx).Where("conversation_id = ?", conversationID)
	if order == "asc" {
		q = q.Order("created_at ASC")
	} else {
		q = q.Order("created_at DESC")
	}
	err := q.Find(&list).Error
	return list, err
}

// ListTasksByConversationPaginated returns tasks with optional executed_only filter, ordered by created_at DESC.
// executedOnly: when true, only tasks that have been run (last_run_id IS NOT NULL) are returned.
// total is the total number of matching tasks (ignoring limit/offset).
func (s *Store) ListTasksByConversationPaginated(ctx context.Context, conversationID string, executedOnly bool, limit, offset int) ([]Task, int, error) {
	q := s.db.WithContext(ctx).Model(&Task{}).Where("conversation_id = ?", conversationID)
	if executedOnly {
		q = q.Where("last_run_id IS NOT NULL AND last_run_id != ''")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []Task
	q = s.db.WithContext(ctx).Where("conversation_id = ?", conversationID)
	if executedOnly {
		q = q.Where("last_run_id IS NOT NULL AND last_run_id != ''")
	}
	err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&list).Error
	return list, int(total), err
}

// GetTask returns the task by task_id, or (nil, nil) if not found.
func (s *Store) GetTask(ctx context.Context, taskID string) (*Task, error) {
	var task Task
	err := s.db.WithContext(ctx).Where("task_id = ?", taskID).First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &task, nil
}

// GetTaskBySessionID returns the task with the given session_id, or (nil, nil) if not found.
func (s *Store) GetTaskBySessionID(ctx context.Context, sessionID string) (*Task, error) {
	var task Task
	err := s.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &task, nil
}

// CreateTaskInput is the input for CreateTask.
type CreateTaskInput struct {
	ConversationID        string
	Input                 string
	Title                 string
	CreatedBy             string
	TitlePromptTokens     int
	TitleCompletionTokens int
	AgentID               *string
}

// CreateTask creates a new task and its first TaskRun (PENDING) in one transaction. Returns the task with last_run_id and session_id set.
func (s *Store) CreateTask(ctx context.Context, in *CreateTaskInput) (*Task, error) {
	if in == nil {
		return nil, errors.New("CreateTaskInput is required")
	}
	now := time.Now().Unix()
	taskID := util.NewPrefixedID(util.PrefixChat)
	taskRunID := util.NewPrefixedID(util.PrefixTaskRun)
	sessionID := uuid.New().String() // UUID for buildmax CLI (session not exposed to user)
	task := &Task{
		TaskID:                taskID,
		ConversationID:        in.ConversationID,
		Status:                "PENDING",
		Input:                 in.Input,
		Title:                 in.Title,
		TitlePromptTokens:     in.TitlePromptTokens,
		TitleCompletionTokens: in.TitleCompletionTokens,
		CreatedBy:             in.CreatedBy,
		CreatedAt:             now,
		LastRunID:             &taskRunID,
		SessionID:             &sessionID,
		AgentID:               in.AgentID,
	}
	run := &TaskRun{
		TaskRunID: taskRunID,
		TaskID:    taskID,
		Input:     in.Input,
		Status:    "PENDING",
		CreatedAt: now,
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		return tx.Create(run).Error
	})
	if err != nil {
		return nil, err
	}
	return task, nil
}

func buildTaskUpdates(status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) map[string]interface{} {
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
	return updates
}

// UpdateTask updates a task's status and optional fields.
// Only non-nil pointer fields are written; status is always set.
func (s *Store) UpdateTask(ctx context.Context, in UpdateTaskInput) error {
	return s.db.WithContext(ctx).Model(&Task{}).Where("task_id = ?", in.TaskID).Updates(
		buildTaskUpdates(in.Status, in.StartedAt, in.EndedAt, in.Output, in.ErrorMessage, in.SessionID),
	).Error
}

// ClaimTask updates a task's status and optional fields only when current status equals expectedStatus.
// Returns updated = (exactly one row was updated). Used for atomic claim (e.g. PENDING→SCHEDULED, SCHEDULED→RUNNING).
func (s *Store) ClaimTask(ctx context.Context, in ClaimTaskInput) (bool, error) {
	result := s.db.WithContext(ctx).Model(&Task{}).Where("task_id = ? AND status = ?", in.TaskID, in.ExpectedStatus).Updates(
		buildTaskUpdates(in.NewStatus, in.StartedAt, in.EndedAt, in.Output, in.ErrorMessage, in.SessionID),
	)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}
