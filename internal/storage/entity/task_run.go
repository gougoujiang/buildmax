package entity

import (
	"context"
	"errors"
	"time"

	"buildmax/internal/util"
	"gorm.io/gorm"
)

func buildTaskRunUpdates(status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string, promptTokens, completionTokens *int) map[string]interface{} {
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
	if promptTokens != nil {
		updates["prompt_tokens"] = *promptTokens
	}
	if completionTokens != nil {
		updates["completion_tokens"] = *completionTokens
	}
	return updates
}

// CreateTaskRun creates a new run (PENDING). Returns ErrRunInProgress if the task has any run in PENDING, SCHEDULED, or RUNNING.
func (s *Store) CreateTaskRun(ctx context.Context, taskID, input, createdBy string) (*TaskRun, error) {
	var inProgress int64
	err := s.db.WithContext(ctx).Model(&TaskRun{}).Where("task_id = ? AND status IN ?", taskID, []string{"PENDING", "SCHEDULED", "RUNNING"}).Count(&inProgress).Error
	if err != nil {
		return nil, err
	}
	if inProgress > 0 {
		return nil, ErrRunInProgress
	}
	run := &TaskRun{
		TaskRunID: util.NewPrefixedID(util.PrefixTaskRun),
		TaskID:    taskID,
		Input:     input,
		Status:    "PENDING",
		CreatedAt: time.Now().Unix(),
	}
	if err := s.db.WithContext(ctx).Create(run).Error; err != nil {
		return nil, err
	}
	return run, nil
}

// GetNextPendingTaskRun returns the oldest run with status PENDING (by created_at), or (nil, nil) if none.
func (s *Store) GetNextPendingTaskRun(ctx context.Context) (*TaskRun, error) {
	var r TaskRun
	err := s.db.WithContext(ctx).Where("status = ?", "PENDING").Order("created_at ASC").First(&r).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

// GetTaskRun returns the run by task_run_id, or (nil, nil) if not found.
func (s *Store) GetTaskRun(ctx context.Context, taskRunID string) (*TaskRun, error) {
	var r TaskRun
	err := s.db.WithContext(ctx).Where("task_run_id = ?", taskRunID).First(&r).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

// GetTaskRunWithTask returns the run and its task, or (nil, nil, nil) if run not found.
func (s *Store) GetTaskRunWithTask(ctx context.Context, taskRunID string) (*TaskRun, *Task, error) {
	run, err := s.GetTaskRun(ctx, taskRunID)
	if err != nil || run == nil {
		return nil, nil, err
	}
	task, err := s.GetTask(ctx, run.TaskID)
	if err != nil || task == nil {
		return run, nil, err
	}
	return run, task, nil
}

// ClaimTaskRun atomically updates a run when current status matches ExpectedStatus.
func (s *Store) ClaimTaskRun(ctx context.Context, in ClaimTaskRunInput) (bool, error) {
	result := s.db.WithContext(ctx).Model(&TaskRun{}).Where("task_run_id = ? AND status = ?", in.TaskRunID, string(in.ExpectedStatus)).Updates(
		buildTaskRunUpdates(string(in.NewStatus), in.StartedAt, in.EndedAt, in.Output, in.ErrorMessage, in.SessionID, nil, nil),
	)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 1 && (in.NewStatus == RunStatusPending || in.NewStatus == RunStatusScheduled || in.NewStatus == RunStatusRunning) {
		_ = s.syncTaskStatusFromRun(ctx, in.TaskRunID, string(in.NewStatus), in.StartedAt, nil)
	}
	return result.RowsAffected == 1, nil
}

// UpdateRun updates a run's status and optional fields.
func (s *Store) UpdateRun(ctx context.Context, in UpdateTaskRunInput) error {
	if err := s.db.WithContext(ctx).Model(&TaskRun{}).Where("task_run_id = ?", in.TaskRunID).Updates(
		buildTaskRunUpdates(string(in.Status), in.StartedAt, in.EndedAt, in.Output, in.ErrorMessage, in.SessionID, in.PromptTokens, in.CompletionTokens),
	).Error; err != nil {
		return err
	}
	if in.Status == RunStatusPending || in.Status == RunStatusScheduled || in.Status == RunStatusRunning {
		_ = s.syncTaskStatusFromRun(ctx, in.TaskRunID, string(in.Status), in.StartedAt, in.EndedAt)
	}
	return nil
}

// syncTaskStatusFromRun updates the task row (denormalized status) for the run's task_id to match the run's status.
func (s *Store) syncTaskStatusFromRun(ctx context.Context, taskRunID, status string, startedAt, endedAt *int64) error {
	var run TaskRun
	if err := s.db.WithContext(ctx).Where("task_run_id = ?", taskRunID).Select("task_id").First(&run).Error; err != nil {
		return err
	}
	taskUpdates := map[string]interface{}{"status": status}
	if status == "PENDING" {
		taskUpdates["started_at"] = nil
		taskUpdates["ended_at"] = nil
	} else {
		if startedAt != nil {
			taskUpdates["started_at"] = *startedAt
		}
		if endedAt != nil {
			taskUpdates["ended_at"] = *endedAt
		}
	}
	return s.db.WithContext(ctx).Model(&Task{}).Where("task_id = ?", run.TaskID).Updates(taskUpdates).Error
}

// UpdateTaskRunWorkerInfo updates worker_type, k8s_job_name, k8s_job_created_at for the run.
func (s *Store) UpdateTaskRunWorkerInfo(ctx context.Context, taskRunID, workerType string, k8sJobName *string, k8sJobCreatedAt *int64) error {
	updates := map[string]interface{}{"worker_type": workerType}
	if k8sJobName != nil {
		updates["k8s_job_name"] = *k8sJobName
	}
	if k8sJobCreatedAt != nil {
		updates["k8s_job_created_at"] = *k8sJobCreatedAt
	}
	return s.db.WithContext(ctx).Model(&TaskRun{}).Where("task_run_id = ?", taskRunID).Updates(updates).Error
}

// OnRunComplete creates task_run_artifact rows (one per relativePath) and updates task denormalized fields.
func (s *Store) OnRunComplete(ctx context.Context, taskRunID string, relativePaths []string) error {
	if len(relativePaths) == 0 {
		relativePaths = []string{"result.md"}
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run TaskRun
		if err := tx.Where("task_run_id = ?", taskRunID).First(&run).Error; err != nil {
			return err
		}
		for _, relPath := range relativePaths {
			if err := tx.Create(&TaskRunArtifact{TaskRunID: taskRunID, RelativePath: relPath}).Error; err != nil {
				return err
			}
		}
		updates := map[string]interface{}{
			"last_run_id":   taskRunID,
			"status":        run.Status,
			"output":        run.Output,
			"started_at":    run.StartedAt,
			"ended_at":      run.EndedAt,
			"error_message": run.ErrorMessage,
		}
		if run.SessionID != nil {
			updates["session_id"] = *run.SessionID
		}
		return tx.Model(&Task{}).Where("task_id = ?", run.TaskID).Updates(updates).Error
	})
}

// SyncTaskFromRun updates task denormalized fields and last_run_id from the run. Use when run ends with FAILED (no artifact).
func (s *Store) SyncTaskFromRun(ctx context.Context, taskRunID string) error {
	var run TaskRun
	if err := s.db.WithContext(ctx).Where("task_run_id = ?", taskRunID).First(&run).Error; err != nil {
		return err
	}
	updates := map[string]interface{}{
		"last_run_id":   taskRunID,
		"status":        run.Status,
		"output":        run.Output,
		"started_at":    run.StartedAt,
		"ended_at":      run.EndedAt,
		"error_message": run.ErrorMessage,
	}
	if run.SessionID != nil {
		updates["session_id"] = *run.SessionID
	}
	return s.db.WithContext(ctx).Model(&Task{}).Where("task_id = ?", run.TaskID).Updates(updates).Error
}
