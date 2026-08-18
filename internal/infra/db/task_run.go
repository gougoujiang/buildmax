package db

import (
	"context"
	"errors"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"time"

	"github.com/gougoujiang/buildmax/internal/util"
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
	TracePath        *string `gorm:"column:trace_path;type:varchar(512)"`
	// CancelRequestedAt is when someone asked this run to stop. It is a
	// separate column rather than a status because a started run stays RUNNING
	// until its own worker reports the outcome.
	CancelRequestedAt *int64  `gorm:"column:cancel_requested_at;index"`
	CancelRequestedBy *string `gorm:"column:cancel_requested_by;type:varchar(64)"`
	CreatedAt         int64   `gorm:"autoCreateTime"`
}

func (taskRunRow) TableName() string { return "task_run" }

type taskRunArtifactRow struct {
	ID           uint   `gorm:"primaryKey;autoIncrement"`
	TaskRunID    string `gorm:"type:varchar(64);not null;uniqueIndex:uq_task_run_artifact_run_path"`
	RelativePath string `gorm:"type:varchar(512);not null;uniqueIndex:uq_task_run_artifact_run_path"`
}

func (taskRunArtifactRow) TableName() string { return "task_run_artifact" }

func toTaskRun(row *taskRunRow) *model.TaskRun {
	if row == nil {
		return nil
	}
	return &model.TaskRun{
		ID:                row.ID,
		TaskRunID:         row.TaskRunID,
		TaskID:            row.TaskID,
		Input:             row.Input,
		CreatedBy:         row.CreatedBy,
		CreatedByType:     row.CreatedByType,
		TriggerSource:     row.TriggerSource,
		Status:            row.Status,
		Output:            row.Output,
		ErrorMessage:      row.ErrorMessage,
		StartedAt:         row.StartedAt,
		EndedAt:           row.EndedAt,
		SessionID:         row.SessionID,
		WorkerType:        row.WorkerType,
		K8sJobName:        row.K8sJobName,
		K8sJobCreatedAt:   row.K8sJobCreatedAt,
		PromptTokens:      row.PromptTokens,
		CompletionTokens:  row.CompletionTokens,
		TracePath:         row.TracePath,
		CancelRequestedAt: row.CancelRequestedAt,
		CancelRequestedBy: row.CancelRequestedBy,
		CreatedAt:         row.CreatedAt,
	}
}

func toTaskRunRow(m *model.TaskRun) *taskRunRow {
	if m == nil {
		return nil
	}
	return &taskRunRow{
		ID:                m.ID,
		TaskRunID:         m.TaskRunID,
		TaskID:            m.TaskID,
		Input:             m.Input,
		CreatedBy:         m.CreatedBy,
		CreatedByType:     m.CreatedByType,
		TriggerSource:     m.TriggerSource,
		Status:            m.Status,
		Output:            m.Output,
		ErrorMessage:      m.ErrorMessage,
		StartedAt:         m.StartedAt,
		EndedAt:           m.EndedAt,
		SessionID:         m.SessionID,
		WorkerType:        m.WorkerType,
		K8sJobName:        m.K8sJobName,
		K8sJobCreatedAt:   m.K8sJobCreatedAt,
		PromptTokens:      m.PromptTokens,
		CompletionTokens:  m.CompletionTokens,
		TracePath:         m.TracePath,
		CancelRequestedAt: m.CancelRequestedAt,
		CancelRequestedBy: m.CancelRequestedBy,
		CreatedAt:         m.CreatedAt,
	}
}

func toTaskRunArtifact(row *taskRunArtifactRow) *model.TaskRunArtifact {
	if row == nil {
		return nil
	}
	return &model.TaskRunArtifact{
		ID:           row.ID,
		TaskRunID:    row.TaskRunID,
		RelativePath: row.RelativePath,
	}
}

func toTaskRunArtifacts(rows []taskRunArtifactRow) []model.TaskRunArtifact {
	out := make([]model.TaskRunArtifact, len(rows))
	for i := range rows {
		out[i] = *toTaskRunArtifact(&rows[i])
	}
	return out
}

// taskRunUpdate is the set of columns a run status transition may write. A
// struct rather than positional parameters: four of the fields are *string, and
// transposing two of them would still compile.
type taskRunUpdate struct {
	status           string
	startedAt        *int64
	endedAt          *int64
	output           *string
	errorMessage     *string
	sessionID        *string
	tracePath        *string
	promptTokens     *int
	completionTokens *int
}

// buildTaskRunUpdates renders the update into GORM's column map. A nil field
// means "leave this column alone", so a later transition cannot blank a value
// an earlier one wrote.
func buildTaskRunUpdates(in taskRunUpdate) map[string]interface{} {
	updates := map[string]interface{}{"status": in.status}
	if in.startedAt != nil {
		updates["started_at"] = *in.startedAt
	}
	if in.endedAt != nil {
		updates["ended_at"] = *in.endedAt
	}
	if in.output != nil {
		updates["output"] = *in.output
	}
	if in.errorMessage != nil {
		updates["error_message"] = *in.errorMessage
	}
	if in.sessionID != nil {
		updates["session_id"] = *in.sessionID
	}
	if in.tracePath != nil {
		updates["trace_path"] = *in.tracePath
	}
	if in.promptTokens != nil {
		updates["prompt_tokens"] = *in.promptTokens
	}
	if in.completionTokens != nil {
		updates["completion_tokens"] = *in.completionTokens
	}
	return updates
}

// CreateTaskRun creates a new run (PENDING). Returns model.ErrRunInProgress if the task has any run in PENDING, SCHEDULED, or RUNNING.
func (s *Store) CreateTaskRun(ctx context.Context, taskID, input, createdBy, createdByType, triggerSource string) (*model.TaskRun, error) {
	var inProgress int64
	err := s.db.WithContext(ctx).Model(&taskRunRow{}).Where("task_id = ? AND status IN ?", taskID, model.ActiveRunStatuses).Count(&inProgress).Error
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
	if err := s.db.WithContext(ctx).Create(toTaskRunRow(run)).Error; err != nil {
		return nil, err
	}
	return run, nil
}

// CountTaskRunsByStatus implements model.TaskRunStore.
func (s *Store) CountTaskRunsByStatus(ctx context.Context) (map[string]int, error) {
	var rows []struct {
		Status string
		N      int
	}
	if err := s.db.WithContext(ctx).
		Model(&taskRunRow{}).
		Select("status, count(*) as n").
		Group("status").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.Status] = row.N
	}
	return out, nil
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
	return toTaskRun(&r), nil
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
	return toTaskRun(&r), nil
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

// GetActiveTaskRunByTask returns the task's run in PENDING, SCHEDULED, or
// RUNNING, or (nil, nil) when the task has none.
//
// CreateTaskRun refuses a second run while one is active, so at most one row
// can match; the ordering only makes the answer deterministic if that ever
// stops being true.
func (s *Store) GetActiveTaskRunByTask(ctx context.Context, taskID string) (*model.TaskRun, error) {
	var r taskRunRow
	err := s.db.WithContext(ctx).
		Where("task_id = ? AND status IN ?", taskID, model.ActiveRunStatuses).
		Order("created_at DESC").
		First(&r).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toTaskRun(&r), nil
}

// RequestTaskRunCancel records a cancel request on a run that is still active
// and does not already carry one.
//
// The status stays where it is. A started run belongs to its worker until that
// worker reports an outcome, so writing CANCELED here would describe a run that
// is still executing, and the worker's own report would overwrite it moments
// later.
func (s *Store) RequestTaskRunCancel(ctx context.Context, taskRunID, requestedBy string, requestedAt int64) (bool, error) {
	result := s.db.WithContext(ctx).Model(&taskRunRow{}).
		Where("task_run_id = ? AND status IN ? AND cancel_requested_at IS NULL", taskRunID, model.ActiveRunStatuses).
		Updates(map[string]interface{}{
			"cancel_requested_at": requestedAt,
			"cancel_requested_by": requestedBy,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// ListCancelRequestedTaskRuns returns runs that were asked to stop before
// cutoffUnix and are still active.
//
// A cancel is honored by the run's worker, and a worker can be gone — killed,
// evicted, or built before cancellation existed. Nothing else would ever move
// those runs, which is why this query exists; see StaleRunReaper.
func (s *Store) ListCancelRequestedTaskRuns(ctx context.Context, cutoffUnix int64, limit int) ([]model.TaskRun, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows []taskRunRow
	err := s.db.WithContext(ctx).
		Where("status IN ?", model.ActiveRunStatuses).
		Where("cancel_requested_at IS NOT NULL AND cancel_requested_at <= ?", cutoffUnix).
		Order("cancel_requested_at ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]model.TaskRun, 0, len(rows))
	for i := range rows {
		out = append(out, *toTaskRun(&rows[i]))
	}
	return out, nil
}

// ClaimTaskRun atomically updates a run when current status matches ExpectedStatus.
func (s *Store) ClaimTaskRun(ctx context.Context, in model.ClaimTaskRunInput) (bool, error) {
	result := s.db.WithContext(ctx).Model(&taskRunRow{}).Where("task_run_id = ? AND status = ?", in.TaskRunID, string(in.ExpectedStatus)).Updates(
		buildTaskRunUpdates(taskRunUpdate{
			status:       string(in.NewStatus),
			startedAt:    in.StartedAt,
			endedAt:      in.EndedAt,
			output:       in.Output,
			errorMessage: in.ErrorMessage,
			sessionID:    in.SessionID,
		}),
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
		buildTaskRunUpdates(taskRunUpdate{
			status:           string(in.Status),
			startedAt:        in.StartedAt,
			endedAt:          in.EndedAt,
			output:           in.Output,
			errorMessage:     in.ErrorMessage,
			sessionID:        in.SessionID,
			tracePath:        in.TracePath,
			promptTokens:     in.PromptTokens,
			completionTokens: in.CompletionTokens,
		}),
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

// ListStaleTaskRuns returns runs that were dispatched before cutoffUnix and have
// not reached a terminal status.
//
// A run leaves SCHEDULED or RUNNING when its worker reports the outcome, so a
// run still in one of those states long after dispatch has no worker coming for
// it: the process died, the pod was evicted, or its credential expired before it
// could report. Nothing else notices, which is why this query exists.
func (s *Store) ListStaleTaskRuns(ctx context.Context, cutoffUnix int64, limit int) ([]model.TaskRun, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows []taskRunRow
	err := s.db.WithContext(ctx).
		Where("status IN ?", []string{string(model.RunStatusScheduled), string(model.RunStatusRunning)}).
		Where("created_at <= ?", cutoffUnix).
		Order("created_at ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]model.TaskRun, 0, len(rows))
	for i := range rows {
		out = append(out, *toTaskRun(&rows[i]))
	}
	return out, nil
}
