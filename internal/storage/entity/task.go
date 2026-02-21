package entity

import (
	"context"
	"errors"
	"time"

	"buildmax/internal/util"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ListTasksByWorkspace returns tasks in the workspace, ordered by created_at.
// order is "asc" (oldest first) or "desc" (latest first); default "desc".
// If projectID is non-nil, only tasks with that project_id are returned.
func (s *Store) ListTasksByWorkspace(ctx context.Context, workspaceID string, projectID *string, order string) ([]Task, error) {
	var list []Task
	q := s.db.WithContext(ctx).Where("workspace_id = ?", workspaceID)
	if projectID != nil {
		q = q.Where("project_id = ?", *projectID)
	}
	if order == "asc" {
		q = q.Order("created_at ASC")
	} else {
		q = q.Order("created_at DESC")
	}
	err := q.Find(&list).Error
	return list, err
}

// ListTasksByWorkspacePaginated returns tasks with optional executed_only filter, ordered by created_at DESC.
// executedOnly: when true, only tasks that have been run (last_run_id IS NOT NULL) are returned.
// total is the total number of matching tasks (ignoring limit/offset).
func (s *Store) ListTasksByWorkspacePaginated(ctx context.Context, workspaceID string, projectID *string, executedOnly bool, limit, offset int) ([]Task, int, error) {
	q := s.db.WithContext(ctx).Model(&Task{}).Where("workspace_id = ?", workspaceID)
	if projectID != nil {
		q = q.Where("project_id = ?", *projectID)
	}
	if executedOnly {
		q = q.Where("last_run_id IS NOT NULL AND last_run_id != ''")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []Task
	q = s.db.WithContext(ctx).Where("workspace_id = ?", workspaceID)
	if projectID != nil {
		q = q.Where("project_id = ?", *projectID)
	}
	if executedOnly {
		q = q.Where("last_run_id IS NOT NULL AND last_run_id != ''")
	}
	err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&list).Error
	return list, int(total), err
}

// GetTask returns the task by task_id, or (nil, nil) if not found.
func (s *Store) GetTask(ctx context.Context, taskID string) (*Task, error) {
	var t Task
	err := s.db.WithContext(ctx).Where("task_id = ?", taskID).First(&t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

// GetTaskBySessionID returns the task with the given session_id, or (nil, nil) if not found.
func (s *Store) GetTaskBySessionID(ctx context.Context, sessionID string) (*Task, error) {
	var t Task
	err := s.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

// CreateTask creates a new task and its first TaskRun (PENDING) in one transaction. Returns the task with last_run_id set.
func (s *Store) CreateTask(ctx context.Context, workspaceID string, projectID *string, input, title, createdBy string) (*Task, error) {
	now := time.Now().Unix()
	taskID := util.NewULID()
	runID := util.NewULID()
	t := &Task{
		TaskID:      taskID,
		WorkspaceID: workspaceID,
		ProjectID:   projectID,
		Status:      "PENDING",
		Input:       input,
		Title:       title,
		CreatedBy:   createdBy,
		CreatedAt:   now,
		LastRunID:   &runID,
	}
	run := &TaskRun{
		RunID:     runID,
		TaskID:    taskID,
		Input:     input,
		Status:    "PENDING",
		CreatedAt: now,
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(t).Error; err != nil {
			return err
		}
		return tx.Create(run).Error
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

// UpdateTaskStatus updates a task's status and optional fields.
// Only non-nil pointer fields are written; status is always set.
func (s *Store) UpdateTaskStatus(ctx context.Context, taskID, status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) error {
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
	return s.db.WithContext(ctx).Model(&Task{}).Where("task_id = ?", taskID).Updates(updates).Error
}

// UpdateTaskStatusIf updates a task's status and optional fields only when current status equals expectedStatus.
// Returns updated = (exactly one row was updated). Used for atomic claim (e.g. PENDING→SCHEDULED, SCHEDULED→RUNNING).
func (s *Store) UpdateTaskStatusIf(ctx context.Context, taskID, expectedStatus, newStatus string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) (bool, error) {
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
	result := s.db.WithContext(ctx).Model(&Task{}).Where("task_id = ? AND status = ?", taskID, expectedStatus).Updates(updates)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// IncrementTaskSeq atomically increments the task's artifact_seq and returns the new value.
func (s *Store) IncrementTaskSeq(ctx context.Context, taskID string) (newSeq int, err error) {
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var t Task
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("task_id = ?", taskID).First(&t).Error; err != nil {
			return err
		}
		newSeq = t.ArtifactSeq + 1
		return tx.Model(&Task{}).Where("task_id = ?", taskID).Update("artifact_seq", newSeq).Error
	})
	return newSeq, err
}
