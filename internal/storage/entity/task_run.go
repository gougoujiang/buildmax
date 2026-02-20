package entity

import (
	"context"
	"errors"
	"time"

	"buildmax/internal/util"
	"gorm.io/gorm"
)

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
		RunID:    util.NewULID(),
		TaskID:   taskID,
		Input:    input,
		Status:   "PENDING",
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

// GetTaskRun returns the run by run_id, or (nil, nil) if not found.
func (s *Store) GetTaskRun(ctx context.Context, runID string) (*TaskRun, error) {
	var r TaskRun
	err := s.db.WithContext(ctx).Where("run_id = ?", runID).First(&r).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

// GetTaskRunWithTask returns the run and its task, or (nil, nil, nil) if run not found.
func (s *Store) GetTaskRunWithTask(ctx context.Context, runID string) (*TaskRun, *Task, error) {
	run, err := s.GetTaskRun(ctx, runID)
	if err != nil || run == nil {
		return nil, nil, err
	}
	task, err := s.GetTask(ctx, run.TaskID)
	if err != nil || task == nil {
		return run, nil, err
	}
	return run, task, nil
}

// UpdateTaskRunStatusIf atomically updates run status when current status equals expectedStatus. Returns updated.
func (s *Store) UpdateTaskRunStatusIf(ctx context.Context, runID, expectedStatus, newStatus string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) (bool, error) {
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
	result := s.db.WithContext(ctx).Model(&TaskRun{}).Where("run_id = ? AND status = ?", runID, expectedStatus).Updates(updates)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// UpdateTaskRunStatus updates a run's status and optional fields.
func (s *Store) UpdateTaskRunStatus(ctx context.Context, runID, status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) error {
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
	return s.db.WithContext(ctx).Model(&TaskRun{}).Where("run_id = ?", runID).Updates(updates).Error
}

// UpdateTaskRunWorkerInfo updates worker_type, k8s_job_name, k8s_job_created_at for the run.
func (s *Store) UpdateTaskRunWorkerInfo(ctx context.Context, runID, workerType string, k8sJobName *string, k8sJobCreatedAt *int64) error {
	updates := map[string]interface{}{"worker_type": workerType}
	if k8sJobName != nil {
		updates["k8s_job_name"] = *k8sJobName
	}
	if k8sJobCreatedAt != nil {
		updates["k8s_job_created_at"] = *k8sJobCreatedAt
	}
	return s.db.WithContext(ctx).Model(&TaskRun{}).Where("run_id = ?", runID).Updates(updates).Error
}

// OnRunComplete creates the artifact with task_run_id, updates task denormalized fields (last_run_id, status, output, etc.) and task.session_id if the run set it.
func (s *Store) OnRunComplete(ctx context.Context, runID, artifactID, relativePath string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run TaskRun
		if err := tx.Where("run_id = ?", runID).First(&run).Error; err != nil {
			return err
		}
		// Create artifact + item
		art := Artifact{
			TaskID:     run.TaskID,
			TaskRunID:  run.RunID,
			ArtifactID: artifactID,
			CreatedAt:  time.Now().Unix(),
			Seq:        0,
		}
		var count int64
		if err := tx.Model(&Artifact{}).Where("task_id = ?", run.TaskID).Count(&count).Error; err != nil {
			return err
		}
		art.Seq = int(count) + 1
		if err := tx.Create(&art).Error; err != nil {
			return err
		}
		if err := tx.Create(&ArtifactItem{ArtifactID: artifactID, RelativePath: relativePath}).Error; err != nil {
			return err
		}
		// Update task denormalized from run (worker_type, k8s_* live only on task_run)
		updates := map[string]interface{}{
			"last_run_id":     runID,
			"status":         run.Status,
			"output":         run.Output,
			"started_at":     run.StartedAt,
			"ended_at":       run.EndedAt,
			"error_message":  run.ErrorMessage,
			"last_artifact_id": artifactID,
		}
		if run.SessionID != nil {
			updates["session_id"] = *run.SessionID
		}
		return tx.Model(&Task{}).Where("task_id = ?", run.TaskID).Updates(updates).Error
	})
}

// SyncTaskFromRun updates task denormalized fields and last_run_id from the run. Use when run ends with FAILED (no artifact).
func (s *Store) SyncTaskFromRun(ctx context.Context, runID string) error {
	var run TaskRun
	if err := s.db.WithContext(ctx).Where("run_id = ?", runID).First(&run).Error; err != nil {
		return err
	}
	updates := map[string]interface{}{
		"last_run_id":    runID,
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
