package db

import (
	"buildmax/internal/core/model"
	"context"
	"errors"
	"time"

	"buildmax/internal/util"
	"gorm.io/gorm"
)

type taskRunRow struct {
	ID               uint    `gorm:"primaryKey;autoIncrement"`
	TaskRunID        string  `gorm:"column:task_run_id;type:varchar(64);uniqueIndex;not null"`
	TaskID           string  `gorm:"column:task_id;type:varchar(64);not null;index"`
	Input            string  `gorm:"type:text;not null"`
	CreatedBy        string  `gorm:"type:varchar(64);index"`
	CreatedByType    string  `gorm:"type:varchar(32)"`
	TriggerSource    string  `gorm:"type:varchar(64)"`
	Status           string  `gorm:"type:varchar(32);not null"`
	Output           *string `gorm:"type:text"`
	ErrorMessage     *string `gorm:"type:text"`
	StartedAt        *int64  `gorm:""`
	EndedAt          *int64  `gorm:""`
	SessionID        *string `gorm:"type:varchar(36)"`
	WorkerType       string  `gorm:"type:varchar(32)"`
	K8sJobName       *string `gorm:"type:varchar(128)"`
	K8sJobCreatedAt  *int64  `gorm:"column:k8s_job_created_at"`
	PromptTokens     *int    `gorm:""`
	CompletionTokens *int    `gorm:""`
	CreatedAt        int64   `gorm:"autoCreateTime"`
}

func (taskRunRow) TableName() string { return "task_run" }

type taskRunArtifactRow struct {
	ID           uint   `gorm:"primaryKey;autoIncrement"`
	TaskRunID    string `gorm:"type:varchar(64);not null;uniqueIndex:uq_task_run_artifact_run_path"`
	RelativePath string `gorm:"type:varchar(512);not null;uniqueIndex:uq_task_run_artifact_run_path"`
}

func (taskRunArtifactRow) TableName() string { return "task_run_artifact" }

func toModelTaskRun(row *taskRunRow) *model.TaskRun {
	if row == nil {
		return nil
	}
	return &model.TaskRun{
		ID:               row.ID,
		TaskRunID:        row.TaskRunID,
		TaskID:           row.TaskID,
		Input:            row.Input,
		CreatedBy:        row.CreatedBy,
		CreatedByType:    row.CreatedByType,
		TriggerSource:    row.TriggerSource,
		Status:           row.Status,
		Output:           row.Output,
		ErrorMessage:     row.ErrorMessage,
		StartedAt:        row.StartedAt,
		EndedAt:          row.EndedAt,
		SessionID:        row.SessionID,
		WorkerType:       row.WorkerType,
		K8sJobName:       row.K8sJobName,
		K8sJobCreatedAt:  row.K8sJobCreatedAt,
		PromptTokens:     row.PromptTokens,
		CompletionTokens: row.CompletionTokens,
		CreatedAt:        row.CreatedAt,
	}
}

func toModelTaskRuns(rows []taskRunRow) []model.TaskRun {
	out := make([]model.TaskRun, len(rows))
	for i := range rows {
		out[i] = *toModelTaskRun(&rows[i])
	}
	return out
}

func fromModelTaskRun(m *model.TaskRun) *taskRunRow {
	if m == nil {
		return nil
	}
	return &taskRunRow{
		ID:               m.ID,
		TaskRunID:        m.TaskRunID,
		TaskID:           m.TaskID,
		Input:            m.Input,
		CreatedBy:        m.CreatedBy,
		CreatedByType:    m.CreatedByType,
		TriggerSource:    m.TriggerSource,
		Status:           m.Status,
		Output:           m.Output,
		ErrorMessage:     m.ErrorMessage,
		StartedAt:        m.StartedAt,
		EndedAt:          m.EndedAt,
		SessionID:        m.SessionID,
		WorkerType:       m.WorkerType,
		K8sJobName:       m.K8sJobName,
		K8sJobCreatedAt:  m.K8sJobCreatedAt,
		PromptTokens:     m.PromptTokens,
		CompletionTokens: m.CompletionTokens,
		CreatedAt:        m.CreatedAt,
	}
}

func toModelTaskRunArtifact(row *taskRunArtifactRow) *model.TaskRunArtifact {
	if row == nil {
		return nil
	}
	return &model.TaskRunArtifact{
		ID:           row.ID,
		TaskRunID:    row.TaskRunID,
		RelativePath: row.RelativePath,
	}
}

func toModelTaskRunArtifacts(rows []taskRunArtifactRow) []model.TaskRunArtifact {
	out := make([]model.TaskRunArtifact, len(rows))
	for i := range rows {
		out[i] = *toModelTaskRunArtifact(&rows[i])
	}
	return out
}

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

// CreateTaskRun creates a new run (PENDING). Returns model.ErrRunInProgress if the task has any run in PENDING, SCHEDULED, or RUNNING.
func (s *Store) CreateTaskRun(ctx context.Context, taskID, input, createdBy, createdByType, triggerSource string) (*model.TaskRun, error) {
	var inProgress int64
	err := s.db.WithContext(ctx).Model(&taskRunRow{}).Where("task_id = ? AND status IN ?", taskID, []string{"PENDING", "SCHEDULED", "RUNNING"}).Count(&inProgress).Error
	if err != nil {
		return nil, err
	}
	if inProgress > 0 {
		return nil, model.ErrRunInProgress
	}
	run := &model.TaskRun{
		TaskRunID:     util.NewPrefixedID(util.PrefixTaskRun),
		TaskID:        taskID,
		Input:         input,
		CreatedBy:     createdBy,
		CreatedByType: defaultString(createdByType, model.RunCreatedByTypeUser),
		TriggerSource: defaultString(triggerSource, model.RunTriggerSourceTaskRerun),
		Status:        "PENDING",
		CreatedAt:     time.Now().Unix(),
	}
	if err := s.db.WithContext(ctx).Create(fromModelTaskRun(run)).Error; err != nil {
		return nil, err
	}
	return run, nil
}

// GetNextPendingTaskRun returns the oldest run with status PENDING (by created_at), or (nil, nil) if none.
func (s *Store) GetNextPendingTaskRun(ctx context.Context) (*model.TaskRun, error) {
	var r taskRunRow
	err := s.db.WithContext(ctx).Where("status = ?", "PENDING").Order("created_at ASC").First(&r).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toModelTaskRun(&r), nil
}

// GetTaskRun returns the run by task_run_id, or (nil, nil) if not found.
func (s *Store) GetTaskRun(ctx context.Context, taskRunID string) (*model.TaskRun, error) {
	var r taskRunRow
	err := s.db.WithContext(ctx).Where("task_run_id = ?", taskRunID).First(&r).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toModelTaskRun(&r), nil
}

// GetTaskRunWithTask returns the run and its task, or (nil, nil, nil) if run not found.
func (s *Store) GetTaskRunWithTask(ctx context.Context, taskRunID string) (*model.TaskRun, *model.Task, error) {
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
func (s *Store) ClaimTaskRun(ctx context.Context, in model.ClaimTaskRunInput) (bool, error) {
	result := s.db.WithContext(ctx).Model(&taskRunRow{}).Where("task_run_id = ? AND status = ?", in.TaskRunID, string(in.ExpectedStatus)).Updates(
		buildTaskRunUpdates(string(in.NewStatus), in.StartedAt, in.EndedAt, in.Output, in.ErrorMessage, in.SessionID, nil, nil),
	)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 1 && (in.NewStatus == model.RunStatusPending || in.NewStatus == model.RunStatusScheduled || in.NewStatus == model.RunStatusRunning) {
		_ = s.syncTaskStatusFromRun(ctx, in.TaskRunID, string(in.NewStatus), in.StartedAt, nil)
	}
	return result.RowsAffected == 1, nil
}

// UpdateRun updates a run's status and optional fields.
func (s *Store) UpdateRun(ctx context.Context, in model.UpdateTaskRunInput) error {
	if err := s.db.WithContext(ctx).Model(&taskRunRow{}).Where("task_run_id = ?", in.TaskRunID).Updates(
		buildTaskRunUpdates(string(in.Status), in.StartedAt, in.EndedAt, in.Output, in.ErrorMessage, in.SessionID, in.PromptTokens, in.CompletionTokens),
	).Error; err != nil {
		return err
	}
	if in.Status == model.RunStatusPending || in.Status == model.RunStatusScheduled || in.Status == model.RunStatusRunning {
		_ = s.syncTaskStatusFromRun(ctx, in.TaskRunID, string(in.Status), in.StartedAt, in.EndedAt)
	}
	return nil
}

// syncTaskStatusFromRun updates the task row (denormalized status) for the run's task_id to match the run's status.
func (s *Store) syncTaskStatusFromRun(ctx context.Context, taskRunID, status string, startedAt, endedAt *int64) error {
	var run taskRunRow
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
	return s.db.WithContext(ctx).Model(&taskRow{}).Where("task_id = ?", run.TaskID).Updates(taskUpdates).Error
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
	return s.db.WithContext(ctx).Model(&taskRunRow{}).Where("task_run_id = ?", taskRunID).Updates(updates).Error
}

// OnRunComplete creates task_run_artifact rows (one per relativePath) and updates task denormalized fields.
func (s *Store) OnRunComplete(ctx context.Context, taskRunID string, relativePaths []string) error {
	if len(relativePaths) == 0 {
		relativePaths = []string{"result.md"}
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run taskRunRow
		if err := tx.Where("task_run_id = ?", taskRunID).First(&run).Error; err != nil {
			return err
		}
		for _, relPath := range relativePaths {
			if err := tx.Create(&taskRunArtifactRow{TaskRunID: taskRunID, RelativePath: relPath}).Error; err != nil {
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
		return tx.Model(&taskRow{}).Where("task_id = ?", run.TaskID).Updates(updates).Error
	})
}

// SyncTaskFromRun updates task denormalized fields and last_run_id from the run. Use when run ends with FAILED (no artifact).
func (s *Store) SyncTaskFromRun(ctx context.Context, taskRunID string) error {
	var run taskRunRow
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
	return s.db.WithContext(ctx).Model(&taskRow{}).Where("task_id = ?", run.TaskID).Updates(updates).Error
}
