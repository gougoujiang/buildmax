package store

import (
	"context"
	"errors"
	"time"

	"buildmax/internal/util"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ListTasksByWorkspace returns tasks in the workspace, ordered by created_at.
// If projectID is non-nil, only tasks with that project_id are returned.
func (s *Store) ListTasksByWorkspace(ctx context.Context, workspaceID string, projectID *string) ([]Task, error) {
	var list []Task
	q := s.db.WithContext(ctx).Where("workspace_id = ?", workspaceID)
	if projectID != nil {
		q = q.Where("project_id = ?", *projectID)
	}
	err := q.Order("created_at ASC").Find(&list).Error
	return list, err
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

// CreateTask inserts a new task with status PENDING and returns it.
// projectID is optional (nil = task with no project).
func (s *Store) CreateTask(ctx context.Context, workspaceID string, projectID *string, input, createdBy string) (*Task, error) {
	t := &Task{
		TaskID:      util.NewID(),
		WorkspaceID: workspaceID,
		ProjectID:   projectID,
		Status:      "PENDING",
		Input:       input,
		CreatedBy:   createdBy,
		CreatedAt:   time.Now().Unix(),
	}
	if err := s.db.WithContext(ctx).Create(t).Error; err != nil {
		return nil, err
	}
	return t, nil
}

// GetNextPendingTask returns the oldest task with status PENDING (by created_at), or (nil, nil) if none.
func (s *Store) GetNextPendingTask(ctx context.Context) (*Task, error) {
	var t Task
	err := s.db.WithContext(ctx).Where("status = ?", "PENDING").Order("created_at ASC").First(&t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
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
