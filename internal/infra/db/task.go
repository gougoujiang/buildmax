package db

import (
	"context"
	"errors"
	"time"

	"buildmax/internal/core/model"
	"buildmax/internal/session"
	"buildmax/internal/util"

	"gorm.io/gorm"
)

type taskRow struct {
	ID                    uint    `gorm:"primaryKey;autoIncrement"`
	TaskID                string  `gorm:"column:task_id;type:varchar(64);uniqueIndex;not null"`
	ConversationID        string  `gorm:"type:varchar(64);not null;index"`
	TeamID                string  `gorm:"type:varchar(64);index"`
	IssueID               *string `gorm:"column:issue_id;type:varchar(64);index"`
	Status                string  `gorm:"type:varchar(32);not null"`
	Input                 string  `gorm:"type:text;not null"`
	Title                 string  `gorm:"type:varchar(256)"`
	TitlePromptTokens     int     `gorm:""`
	TitleCompletionTokens int     `gorm:""`
	Output                *string `gorm:"type:text"`
	CreatedBy             string  `gorm:"type:varchar(64);not null"`
	CreatedAt             int64   `gorm:"autoCreateTime"`
	StartedAt             *int64  `gorm:""`
	EndedAt               *int64  `gorm:""`
	ErrorMessage          *string `gorm:"type:text"`
	SessionID             *string `gorm:"type:varchar(36)"`
	LastRunID             *string `gorm:"type:varchar(64);index"`
	AgentID               *string `gorm:"type:varchar(64);index"`
}

func (taskRow) TableName() string { return "task" }

func toModelTask(row *taskRow) *model.Task {
	if row == nil {
		return nil
	}
	return &model.Task{
		ID:                    row.ID,
		TaskID:                row.TaskID,
		ConversationID:        row.ConversationID,
		TeamID:                row.TeamID,
		IssueID:               row.IssueID,
		Status:                row.Status,
		Input:                 row.Input,
		Title:                 row.Title,
		TitlePromptTokens:     row.TitlePromptTokens,
		TitleCompletionTokens: row.TitleCompletionTokens,
		Output:                row.Output,
		CreatedBy:             row.CreatedBy,
		CreatedAt:             row.CreatedAt,
		StartedAt:             row.StartedAt,
		EndedAt:               row.EndedAt,
		ErrorMessage:          row.ErrorMessage,
		SessionID:             row.SessionID,
		LastRunID:             row.LastRunID,
		AgentID:               row.AgentID,
	}
}

func toModelTasks(rows []taskRow) []model.Task {
	out := make([]model.Task, len(rows))
	for i := range rows {
		out[i] = *toModelTask(&rows[i])
	}
	return out
}

func fromModelTask(m *model.Task) *taskRow {
	if m == nil {
		return nil
	}
	return &taskRow{
		ID:                    m.ID,
		TaskID:                m.TaskID,
		ConversationID:        m.ConversationID,
		TeamID:                m.TeamID,
		IssueID:               m.IssueID,
		Status:                m.Status,
		Input:                 m.Input,
		Title:                 m.Title,
		TitlePromptTokens:     m.TitlePromptTokens,
		TitleCompletionTokens: m.TitleCompletionTokens,
		Output:                m.Output,
		CreatedBy:             m.CreatedBy,
		CreatedAt:             m.CreatedAt,
		StartedAt:             m.StartedAt,
		EndedAt:               m.EndedAt,
		ErrorMessage:          m.ErrorMessage,
		SessionID:             m.SessionID,
		LastRunID:             m.LastRunID,
		AgentID:               m.AgentID,
	}
}

// ListTasksByConversation returns tasks in the conversation, ordered by created_at.
// order is "asc" (oldest first) or "desc" (latest first); default "desc".
func (s *Store) ListTasksByConversation(ctx context.Context, conversationID string, order string) ([]model.Task, error) {
	var list []taskRow
	q := s.db.WithContext(ctx).Where("conversation_id = ?", conversationID)
	if order == "asc" {
		q = q.Order("created_at ASC")
	} else {
		q = q.Order("created_at DESC")
	}
	err := q.Find(&list).Error
	return toModelTasks(list), err
}

// ListTasksByConversationPaginated returns tasks with optional executed_only filter, ordered by created_at DESC.
// executedOnly: when true, only tasks that have been run (last_run_id IS NOT NULL) are returned.
// total is the total number of matching tasks (ignoring limit/offset).
func (s *Store) ListTasksByConversationPaginated(ctx context.Context, conversationID string, executedOnly bool, limit, offset int) ([]model.Task, int, error) {
	q := s.db.WithContext(ctx).Model(&taskRow{}).Where("conversation_id = ?", conversationID)
	if executedOnly {
		q = q.Where("last_run_id IS NOT NULL AND last_run_id != ''")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []taskRow
	q = s.db.WithContext(ctx).Where("conversation_id = ?", conversationID)
	if executedOnly {
		q = q.Where("last_run_id IS NOT NULL AND last_run_id != ''")
	}
	err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&list).Error
	return toModelTasks(list), int(total), err
}

func (s *Store) ListTasksByIssue(ctx context.Context, issueID string, limit, offset int) ([]model.Task, int, error) {
	q := s.db.WithContext(ctx).Model(&taskRow{}).Where("issue_id = ?", issueID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []taskRow
	q = s.db.WithContext(ctx).Where("issue_id = ?", issueID).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	err := q.Find(&list).Error
	return toModelTasks(list), int(total), err
}

// GetTask returns the task by task_id, or (nil, nil) if not found.
func (s *Store) GetTask(ctx context.Context, taskID string) (*model.Task, error) {
	var task taskRow
	err := s.db.WithContext(ctx).Where("task_id = ?", taskID).First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toModelTask(&task), nil
}

// GetTaskBySessionID returns the task with the given session_id, or (nil, nil) if not found.
func (s *Store) GetTaskBySessionID(ctx context.Context, sessionID string) (*model.Task, error) {
	var task taskRow
	err := s.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toModelTask(&task), nil
}

// CreateTask creates a new task and its first model.TaskRun (PENDING) in one transaction. Returns the task with last_run_id and session_id set.
func (s *Store) CreateTask(ctx context.Context, in *model.CreateTaskInput) (*model.Task, error) {
	if in == nil {
		return nil, errors.New("model.CreateTaskInput is required")
	}
	now := time.Now().Unix()
	taskID := util.NewPrefixedID(util.PrefixTask)
	taskRunID := util.NewPrefixedID(util.PrefixTaskRun)
	sessionID := session.NewID() // UUID for buildmax CLI (session not exposed to user)
	task := &model.Task{
		TaskID:                taskID,
		ConversationID:        in.ConversationID,
		TeamID:                in.TeamID,
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
		IssueID:               in.IssueID,
	}
	run := &model.TaskRun{
		TaskRunID:     taskRunID,
		TaskID:        taskID,
		Input:         in.Input,
		CreatedBy:     defaultString(in.InitialRunCreatedBy, in.CreatedBy),
		CreatedByType: defaultString(in.InitialRunCreatedByType, model.RunCreatedByTypeUser),
		TriggerSource: defaultString(in.InitialRunTriggerSource, model.RunTriggerSourceTaskCreate),
		Status:        "PENDING",
		CreatedAt:     now,
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conv conversationRow
		if err := tx.Where("conversation_id = ?", in.ConversationID).First(&conv).Error; err != nil {
			return err
		}
		if task.TeamID == "" {
			task.TeamID = conv.TeamID
		}
		if err := tx.Create(fromModelTask(task)).Error; err != nil {
			return err
		}
		return tx.Create(fromModelTaskRun(run)).Error
	})
	if err != nil {
		return nil, err
	}
	return task, nil
}

func defaultString(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
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
func (s *Store) UpdateTask(ctx context.Context, in model.UpdateTaskInput) error {
	return s.db.WithContext(ctx).Model(&taskRow{}).Where("task_id = ?", in.TaskID).Updates(
		buildTaskUpdates(in.Status, in.StartedAt, in.EndedAt, in.Output, in.ErrorMessage, in.SessionID),
	).Error
}

// ClaimTask updates a task's status and optional fields only when current status equals expectedStatus.
// Returns updated = (exactly one row was updated). Used for atomic claim (e.g. PENDING→SCHEDULED, SCHEDULED→RUNNING).
func (s *Store) ClaimTask(ctx context.Context, in model.ClaimTaskInput) (bool, error) {
	result := s.db.WithContext(ctx).Model(&taskRow{}).Where("task_id = ? AND status = ?", in.TaskID, in.ExpectedStatus).Updates(
		buildTaskUpdates(in.NewStatus, in.StartedAt, in.EndedAt, in.Output, in.ErrorMessage, in.SessionID),
	)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}
