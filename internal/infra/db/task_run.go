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
	ID       uint64 `gorm:"primaryKey;autoIncrement"`
	PublicID string `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_task_run_public_id;not null"`
	TaskID   uint64 `gorm:"column:task_id;not null;index:idx_task_run_task_created,priority:1"`
	Input    string `gorm:"type:text;not null"`
	// CreatedBy stays an opaque handle: created_by_type admits "webhook" and
	// "system", neither of which is a user row.
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
	CancelRequestedBy *uint64 `gorm:"column:cancel_requested_by"`
	// RetryOfTaskRunID names the run this one repeats. A nullable column rather
	// than a trigger_source detail because the question a reader asks is which
	// run this repeated, and a source string cannot answer it.
	RetryOfTaskRunID *uint64 `gorm:"column:retry_of_task_run_id;index"`
	// SourceMessageID names the conversation message this run was asked for in.
	// Nullable because most origins are not a message: a workflow step, an
	// issue agent run, a retry, and a task created from the API all have none.
	SourceMessageID *uint64 `gorm:"column:source_message_id;index"`
	// AgentRevision numbers the agent definition served to this run's worker.
	// Not a reference to agent_revision.id: the pair (task.agent_id, this) is
	// what addresses a revision, and the task already holds the agent.
	AgentRevision *int  `gorm:"column:agent_revision"`
	CreatedAt     int64 `gorm:"autoCreateTime;index:idx_task_run_task_created,priority:2"`
}

func (taskRunRow) TableName() string { return "task_run" }

type taskRunArtifactRow struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	TaskRunID    uint64 `gorm:"column:task_run_id;not null;uniqueIndex:uq_task_run_artifact_run_path"`
	RelativePath string `gorm:"type:varchar(512);not null;uniqueIndex:uq_task_run_artifact_run_path"`
}

func (taskRunArtifactRow) TableName() string { return "task_run_artifact" }

// taskRunReadRow is the row plus the handles its references resolve to. A
// pointer field is one a LEFT JOIN may leave NULL.
type taskRunReadRow struct {
	Row                   taskRunRow `gorm:"embedded"`
	TaskPublicID          string     `gorm:"column:task_public_id"`
	RetryOfPublicID       *string    `gorm:"column:retry_of_public_id"`
	CancelRequestedByPub  *string    `gorm:"column:cancel_requested_by_public_id"`
	SourceMessagePublicID *string    `gorm:"column:source_message_public_id"`
}

func (s *Store) taskRunSelect(ctx context.Context) *gorm.DB {
	return taskRunSelectTx(s.db.WithContext(ctx))
}

func taskRunSelectTx(tx *gorm.DB) *gorm.DB {
	return tx.Model(&taskRunRow{}).
		Select("task_run.*, t.public_id AS task_public_id, ro.public_id AS retry_of_public_id, " +
			"cb.public_id AS cancel_requested_by_public_id, sm.public_id AS source_message_public_id").
		Joins("INNER JOIN task t ON t.id = task_run.task_id").
		Joins("LEFT JOIN task_run ro ON ro.id = task_run.retry_of_task_run_id").
		Joins("LEFT JOIN `user` cb ON cb.id = task_run.cancel_requested_by").
		Joins("LEFT JOIN conversation_message sm ON sm.id = task_run.source_message_id")
}

func toTaskRun(row *taskRunReadRow) *model.TaskRun {
	if row == nil {
		return nil
	}
	out := &model.TaskRun{
		ID:                row.Row.PublicID,
		TaskID:            row.TaskPublicID,
		Input:             row.Row.Input,
		CreatedBy:         row.Row.CreatedBy,
		CreatedByType:     row.Row.CreatedByType,
		TriggerSource:     row.Row.TriggerSource,
		Status:            row.Row.Status,
		Output:            row.Row.Output,
		ErrorMessage:      row.Row.ErrorMessage,
		StartedAt:         row.Row.StartedAt,
		EndedAt:           row.Row.EndedAt,
		SessionID:         row.Row.SessionID,
		WorkerType:        row.Row.WorkerType,
		K8sJobName:        row.Row.K8sJobName,
		K8sJobCreatedAt:   row.Row.K8sJobCreatedAt,
		PromptTokens:      row.Row.PromptTokens,
		CompletionTokens:  row.Row.CompletionTokens,
		TracePath:         row.Row.TracePath,
		AgentRevision:     row.Row.AgentRevision,
		CancelRequestedAt: row.Row.CancelRequestedAt,
		CreatedAt:         row.Row.CreatedAt,
	}
	if row.Row.CancelRequestedBy != nil {
		by := derefPublicID(row.CancelRequestedByPub)
		out.CancelRequestedBy = &by
	}
	if row.Row.RetryOfTaskRunID != nil {
		of := derefPublicID(row.RetryOfPublicID)
		out.RetryOfTaskRunID = &of
	}
	if row.Row.SourceMessageID != nil {
		from := derefPublicID(row.SourceMessagePublicID)
		out.SourceMessageID = &from
	}
	return out
}

func toTaskRunArtifact(row *taskRunArtifactRow) *model.TaskRunArtifact {
	if row == nil {
		return nil
	}
	return &model.TaskRunArtifact{
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
func (s *Store) CreateTaskRun(ctx context.Context, in model.CreateTaskRunInput) (*model.TaskRun, error) {
	taskKey, err := lookupKey(ctx, s.db, "task", in.TaskID)
	if err != nil {
		return nil, err
	}
	var inProgress int64
	err = s.db.WithContext(ctx).Model(&taskRunRow{}).Where("task_id = ? AND status IN ?", taskKey, model.ActiveRunStatuses).Count(&inProgress).Error
	if err != nil {
		return nil, err
	}
	if inProgress > 0 {
		return nil, model.ErrRunInProgress
	}
	row := &taskRunRow{
		TaskID:        taskKey,
		Input:         in.Input,
		CreatedBy:     in.CreatedBy,
		CreatedByType: defaultString(in.CreatedByType, model.RunCreatedByTypeUser),
		TriggerSource: defaultString(in.TriggerSource, model.RunTriggerSourceTaskRerun),
		Status:        "PENDING",
		CreatedAt:     time.Now().Unix(),
	}
	var retryOf *string
	if in.RetryOfTaskRunID != nil && *in.RetryOfTaskRunID != "" {
		key, err := lookupKey(ctx, s.db, "task_run", *in.RetryOfTaskRunID)
		if err != nil {
			return nil, err
		}
		row.RetryOfTaskRunID = &key
		retryOf = optionalCanonicalPublicID(in.RetryOfTaskRunID)
	}
	// A message that cannot be resolved leaves the run unattributed rather than
	// refusing it. Losing the provenance of work someone asked for is bad;
	// refusing to do the work because its provenance would not resolve is worse.
	sourceKey, err := optionalKey(ctx, s.db, "conversation_message", in.SourceMessageID)
	if err != nil && !errors.Is(err, model.ErrNotFound) {
		return nil, err
	}
	row.SourceMessageID = sourceKey
	if err := createWithPublicID(ctx, s.db, "uq_task_run_public_id",
		func(id string) { row.PublicID = id }, row); err != nil {
		return nil, err
	}
	var sourceMessage *string
	if sourceKey != nil {
		sourceMessage = optionalCanonicalPublicID(in.SourceMessageID)
	}
	return toTaskRun(&taskRunReadRow{
		Row:                   *row,
		TaskPublicID:          canonicalPublicID(in.TaskID),
		RetryOfPublicID:       retryOf,
		SourceMessagePublicID: sourceMessage,
	}), nil
}

// RecordTaskRunAgentRevision stores which agent definition a run was given.
//
// The `agent_revision IS NULL` guard is what makes the first write win. A worker
// polls its run repeatedly, and an agent edited mid-run would otherwise rewrite
// the record of what actually ran on the next poll.
func (s *Store) RecordTaskRunAgentRevision(ctx context.Context, taskRunID string, revision int) error {
	id, ok := util.CanonicalPublicID(taskRunID)
	if !ok {
		return model.ErrNotFound
	}
	return s.db.WithContext(ctx).Model(&taskRunRow{}).
		Where("public_id = ? AND agent_revision IS NULL", id).
		Update("agent_revision", revision).Error
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
	var r taskRunReadRow
	err := s.taskRunSelect(ctx).Where("task_run.status = ?", "PENDING").Order("task_run.created_at ASC").Take(&r).Error
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
	id, ok := util.CanonicalPublicID(taskRunID)
	if !ok {
		return nil, nil
	}
	var r taskRunReadRow
	err := s.taskRunSelect(ctx).Where("task_run.public_id = ?", id).Take(&r).Error
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
	taskKey, err := lookupKey(ctx, s.db, "task", taskID)
	if errors.Is(err, model.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r taskRunReadRow
	err = s.taskRunSelect(ctx).
		Where("task_run.task_id = ? AND task_run.status IN ?", taskKey, model.ActiveRunStatuses).
		Order("task_run.created_at DESC").
		Take(&r).Error
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
	id, ok := util.CanonicalPublicID(taskRunID)
	if !ok {
		return false, nil
	}
	requester, err := lookupKey(ctx, s.db, "user", requestedBy)
	if err != nil {
		return false, err
	}
	result := s.db.WithContext(ctx).Model(&taskRunRow{}).
		Where("public_id = ? AND status IN ? AND cancel_requested_at IS NULL", id, model.ActiveRunStatuses).
		Updates(map[string]interface{}{
			"cancel_requested_at": requestedAt,
			"cancel_requested_by": requester,
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
	var rows []taskRunReadRow
	err := s.taskRunSelect(ctx).
		Where("task_run.status IN ?", model.ActiveRunStatuses).
		Where("task_run.cancel_requested_at IS NOT NULL AND task_run.cancel_requested_at <= ?", cutoffUnix).
		Order("task_run.cancel_requested_at ASC").
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
	id, ok := util.CanonicalPublicID(in.TaskRunID)
	if !ok {
		return false, nil
	}
	result := s.db.WithContext(ctx).Model(&taskRunRow{}).Where("public_id = ? AND status = ?", id, string(in.ExpectedStatus)).Updates(
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
	id, ok := util.CanonicalPublicID(in.TaskRunID)
	if !ok {
		return model.ErrNotFound
	}
	if err := s.db.WithContext(ctx).Model(&taskRunRow{}).Where("public_id = ?", id).Updates(
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
	id, ok := util.CanonicalPublicID(taskRunID)
	if !ok {
		return model.ErrNotFound
	}
	var run taskRunRow
	if err := s.db.WithContext(ctx).Where("public_id = ?", id).Select("task_id").First(&run).Error; err != nil {
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
	return s.db.WithContext(ctx).Model(&taskRow{}).Where("id = ?", run.TaskID).Updates(taskUpdates).Error
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
	id, ok := util.CanonicalPublicID(taskRunID)
	if !ok {
		return model.ErrNotFound
	}
	return s.db.WithContext(ctx).Model(&taskRunRow{}).Where("public_id = ?", id).Updates(updates).Error
}

// ListTaskRunIDsByTasks groups run IDs by their task, newest first.
func (s *Store) ListTaskRunIDsByTasks(ctx context.Context, taskIDs []string) (map[string][]string, error) {
	if len(taskIDs) == 0 {
		return map[string][]string{}, nil
	}
	// Both handles come back through the join rather than being resolved one
	// task at a time; the grouping is still on the row key.
	ids := make([]string, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		if id, ok := util.CanonicalPublicID(taskID); ok {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return map[string][]string{}, nil
	}
	var rows []struct {
		TaskPublicID    string
		TaskRunPublicID string
	}
	err := s.db.WithContext(ctx).Model(&taskRunRow{}).
		Select("t.public_id AS task_public_id, task_run.public_id AS task_run_public_id").
		Joins("INNER JOIN task t ON t.id = task_run.task_id").
		Where("t.public_id IN ?", ids).
		Order("task_run.created_at DESC, task_run.id DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string][]string, len(taskIDs))
	for _, row := range rows {
		out[row.TaskPublicID] = append(out[row.TaskPublicID], row.TaskRunPublicID)
	}
	return out, nil
}

// OnRunComplete creates task_run_artifact rows (one per relativePath) and updates task denormalized fields.
func (s *Store) OnRunComplete(ctx context.Context, taskRunID string, relativePaths []string) error {
	if len(relativePaths) == 0 {
		relativePaths = []string{"result.md"}
	}
	id, ok := util.CanonicalPublicID(taskRunID)
	if !ok {
		return model.ErrNotFound
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run taskRunRow
		if err := tx.Where("public_id = ?", id).First(&run).Error; err != nil {
			return err
		}
		for _, relPath := range relativePaths {
			if err := tx.Create(&taskRunArtifactRow{TaskRunID: run.ID, RelativePath: relPath}).Error; err != nil {
				return err
			}
		}
		updates := map[string]interface{}{
			"last_run_id":   run.ID,
			"status":        run.Status,
			"output":        run.Output,
			"started_at":    run.StartedAt,
			"ended_at":      run.EndedAt,
			"error_message": run.ErrorMessage,
		}
		if run.SessionID != nil {
			updates["session_id"] = *run.SessionID
		}
		return tx.Model(&taskRow{}).Where("id = ?", run.TaskID).Updates(updates).Error
	})
}

// SyncTaskFromRun updates task denormalized fields and last_run_id from the run. Use when run ends with FAILED (no artifact).
func (s *Store) SyncTaskFromRun(ctx context.Context, taskRunID string) error {
	id, ok := util.CanonicalPublicID(taskRunID)
	if !ok {
		return model.ErrNotFound
	}
	var run taskRunRow
	if err := s.db.WithContext(ctx).Where("public_id = ?", id).First(&run).Error; err != nil {
		return err
	}
	updates := map[string]interface{}{
		"last_run_id":   run.ID,
		"status":        run.Status,
		"output":        run.Output,
		"started_at":    run.StartedAt,
		"ended_at":      run.EndedAt,
		"error_message": run.ErrorMessage,
	}
	if run.SessionID != nil {
		updates["session_id"] = *run.SessionID
	}
	return s.db.WithContext(ctx).Model(&taskRow{}).Where("id = ?", run.TaskID).Updates(updates).Error
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
	var rows []taskRunReadRow
	err := s.taskRunSelect(ctx).
		Where("task_run.status IN ?", []string{string(model.RunStatusScheduled), string(model.RunStatusRunning)}).
		Where("task_run.created_at <= ?", cutoffUnix).
		Order("task_run.created_at ASC").
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
