package db

import (
	"context"
	"errors"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/core/session"
	"github.com/gougoujiang/buildmax/internal/util"

	"gorm.io/gorm"
)

type taskRow struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement"`
	PublicID string `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_task_public_id;not null"`
	// The team index carries created_at: a team's task list is always ordered
	// by it, and the single-column index the string model left could not serve
	// the sort.
	ConversationID        uint64     `gorm:"column:conversation_id;not null;index"`
	TeamID                uint64     `gorm:"column:team_id;index:idx_task_team_created,priority:1"`
	IssueID               *uint64    `gorm:"column:issue_id;index"`
	Status                string     `gorm:"type:varchar(32);not null"`
	Input                 string     `gorm:"type:text;not null"`
	Title                 string     `gorm:"type:varchar(256)"`
	TitlePromptTokens     int        `gorm:""`
	TitleCompletionTokens int        `gorm:""`
	Output                *string    `gorm:"type:text"`
	CreatedBy             uint64     `gorm:"column:created_by;not null"`
	CreatedAt             time.Time  `gorm:"autoCreateTime;index:idx_task_team_created,priority:2"`
	StartedAt             *time.Time `gorm:""`
	EndedAt               *time.Time `gorm:""`
	ErrorMessage          *string    `gorm:"type:text"`
	SessionID             *string    `gorm:"type:varchar(36)"`
	LastRunID             *uint64    `gorm:"column:last_run_id;index"`
	AgentID               *uint64    `gorm:"column:agent_id;index"`
}

func (taskRow) TableName() string { return "task" }

// taskReadRow is the row plus the handles its converted references resolve to.
// A pointer field is one a LEFT JOIN may leave NULL.
type taskReadRow struct {
	Row                  taskRow `gorm:"embedded"`
	ConversationPublicID string  `gorm:"column:conversation_public_id"`
	TeamPublicID         *string `gorm:"column:team_public_id"`
	CreatedByPublicID    string  `gorm:"column:created_by_public_id"`
	LastRunPublicID      *string `gorm:"column:last_run_public_id"`
	IssuePublicID        *string `gorm:"column:issue_public_id"`
	AgentPublicID        *string `gorm:"column:agent_public_id"`
}

// taskSelect is the one place the join set for a task read is written down, so
// the detail read, the listings, and the run-output read cannot drift apart.
// Every join is a primary-key lookup, which is what keeps a listing one query.
func (s *Store) taskSelect(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Model(&taskRow{}).
		Select("task.*, c.public_id AS conversation_public_id, t.public_id AS team_public_id, " +
			"cb.public_id AS created_by_public_id, lr.public_id AS last_run_public_id, " +
			"i.public_id AS issue_public_id, a.public_id AS agent_public_id").
		Joins("INNER JOIN conversation c ON c.id = task.conversation_id").
		Joins("LEFT JOIN team t ON t.id = task.team_id").
		Joins("INNER JOIN `user` cb ON cb.id = task.created_by").
		Joins("LEFT JOIN task_run lr ON lr.id = task.last_run_id").
		Joins("LEFT JOIN issue i ON i.id = task.issue_id").
		Joins("LEFT JOIN agent a ON a.id = task.agent_id")
}

func toTask(row *taskReadRow) *model.Task {
	if row == nil {
		return nil
	}
	out := &model.Task{
		ID:                    row.Row.PublicID,
		ConversationID:        row.ConversationPublicID,
		TeamID:                derefPublicID(row.TeamPublicID),
		Status:                row.Row.Status,
		Input:                 row.Row.Input,
		Title:                 row.Row.Title,
		TitlePromptTokens:     row.Row.TitlePromptTokens,
		TitleCompletionTokens: row.Row.TitleCompletionTokens,
		Output:                row.Row.Output,
		CreatedBy:             row.CreatedByPublicID,
		CreatedAt:             row.Row.CreatedAt,
		StartedAt:             row.Row.StartedAt,
		EndedAt:               row.Row.EndedAt,
		ErrorMessage:          row.Row.ErrorMessage,
		SessionID:             row.Row.SessionID,
	}
	if row.Row.LastRunID != nil {
		lastRun := derefPublicID(row.LastRunPublicID)
		out.LastRunID = &lastRun
	}
	if row.Row.IssueID != nil {
		issue := derefPublicID(row.IssuePublicID)
		out.IssueID = &issue
	}
	if row.Row.AgentID != nil {
		agent := derefPublicID(row.AgentPublicID)
		out.AgentID = &agent
	}
	return out
}

func toTasks(rows []taskReadRow) []model.Task {
	out := make([]model.Task, len(rows))
	for i := range rows {
		out[i] = *toTask(&rows[i])
	}
	return out
}

// ListTasksByConversation returns tasks in the conversation, ordered by created_at.
// order is "asc" (oldest first) or "desc" (latest first); default "desc".
func (s *Store) ListTasksByConversation(ctx context.Context, conversationID string, order string) ([]model.Task, error) {
	id, ok := util.CanonicalPublicID(conversationID)
	if !ok {
		return nil, nil
	}
	var list []taskReadRow
	q := s.taskSelect(ctx).Where("c.public_id = ?", id)
	if order == "asc" {
		q = q.Order("task.created_at ASC")
	} else {
		q = q.Order("task.created_at DESC")
	}
	err := q.Find(&list).Error
	return toTasks(list), err
}

// ListTasksByConversationPaginated returns tasks with optional executed_only filter, ordered by created_at DESC.
// executedOnly: when true, only tasks that have been run (last_run_id IS NOT NULL) are returned.
// total is the total number of matching tasks (ignoring limit/offset).
func (s *Store) ListTasksByConversationPaginated(ctx context.Context, conversationID string, executedOnly bool, limit, offset int) ([]model.Task, int, error) {
	limit, offset = capPage(limit, offset)
	convKey, err := lookupKey(ctx, s.db, "conversation", conversationID)
	if errors.Is(err, model.ErrNotFound) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	count := s.db.WithContext(ctx).Model(&taskRow{}).Where("conversation_id = ?", convKey)
	if executedOnly {
		count = count.Where("last_run_id IS NOT NULL")
	}
	var total int64
	if err := count.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q := s.taskSelect(ctx).Where("task.conversation_id = ?", convKey)
	if executedOnly {
		q = q.Where("task.last_run_id IS NOT NULL")
	}
	var list []taskReadRow
	err = q.Order("task.created_at DESC").Limit(limit).Offset(offset).Find(&list).Error
	return toTasks(list), int(total), err
}

func (s *Store) ListTasksByIssue(ctx context.Context, issueID string, limit, offset int) ([]model.Task, int, error) {
	limit, offset = capPage(limit, offset)
	issueKey, err := lookupKey(ctx, s.db, "issue", issueID)
	if errors.Is(err, model.ErrNotFound) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := s.db.WithContext(ctx).Model(&taskRow{}).Where("issue_id = ?", issueKey).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q := s.taskSelect(ctx).Where("task.issue_id = ?", issueKey).Order("task.created_at DESC")
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	var list []taskReadRow
	err = q.Find(&list).Error
	return toTasks(list), int(total), err
}

// GetTask returns the task by task_id, or (nil, nil) if not found.
func (s *Store) GetTask(ctx context.Context, taskID string) (*model.Task, error) {
	id, ok := util.CanonicalPublicID(taskID)
	if !ok {
		return nil, nil
	}
	var task taskReadRow
	err := s.taskSelect(ctx).Where("task.public_id = ?", id).Take(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toTask(&task), nil
}

// GetTaskBySessionID returns the task with the given session_id, or (nil, nil) if not found.
func (s *Store) GetTaskBySessionID(ctx context.Context, sessionID string) (*model.Task, error) {
	var task taskReadRow
	err := s.taskSelect(ctx).Where("task.session_id = ?", sessionID).Take(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toTask(&task), nil
}

// CreateTask creates a new task and its first model.TaskRun (PENDING) in one transaction. Returns the task with last_run_id and session_id set.
func (s *Store) CreateTask(ctx context.Context, in *model.CreateTaskInput) (*model.Task, error) {
	if in == nil {
		return nil, errors.New("model.CreateTaskInput is required")
	}
	now := time.Now().UTC()
	sessionID := session.NewID() // UUID for buildmax CLI (session not exposed to user)
	taskDB := &taskRow{
		Status:                "PENDING",
		Input:                 in.Input,
		Title:                 in.Title,
		TitlePromptTokens:     in.TitlePromptTokens,
		TitleCompletionTokens: in.TitleCompletionTokens,
		CreatedAt:             now,
		SessionID:             &sessionID,
	}
	runDB := &taskRunRow{
		Input:         in.Input,
		CreatedBy:     defaultString(in.InitialRunCreatedBy, in.CreatedBy),
		CreatedByType: defaultString(in.InitialRunCreatedByType, model.RunCreatedByTypeUser),
		TriggerSource: defaultString(in.InitialRunTriggerSource, model.RunTriggerSourceTaskCreate),
		Status:        "PENDING",
		CreatedAt:     now,
	}
	teamID := in.TeamID
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The conversation is read once and its team taken from the row already
		// in hand, rather than resolved a second time from the caller's handle.
		var conv conversationRow
		convID, ok := util.CanonicalPublicID(in.ConversationID)
		if !ok {
			return model.ErrNotFound
		}
		if err := tx.Where("public_id = ?", convID).First(&conv).Error; err != nil {
			return err
		}
		taskDB.ConversationID = conv.ID
		taskDB.TeamID = conv.TeamID
		if teamID != "" {
			teamKey, err := lookupKey(ctx, tx, "team", teamID)
			if err != nil {
				return err
			}
			taskDB.TeamID = teamKey
		}
		creator, err := lookupKey(ctx, tx, "user", in.CreatedBy)
		if err != nil {
			return err
		}
		taskDB.CreatedBy = creator
		if in.AgentID != nil && *in.AgentID != "" {
			key, err := lookupKey(ctx, tx, "agent", *in.AgentID)
			if err != nil {
				return err
			}
			taskDB.AgentID = &key
		}
		if in.IssueID != nil && *in.IssueID != "" {
			key, err := lookupKey(ctx, tx, "issue", *in.IssueID)
			if err != nil {
				return err
			}
			taskDB.IssueID = &key
		}
		// See CreateTaskRun: an unresolvable message leaves the run
		// unattributed rather than refusing to create the task.
		sourceKey, err := optionalKey(ctx, tx, "conversation_message", in.InitialRunSourceMessageID)
		if err != nil && !errors.Is(err, model.ErrNotFound) {
			return err
		}
		runDB.SourceMessageID = sourceKey
		if err := createWithPublicID(ctx, tx, "uq_task_public_id",
			func(id string) { taskDB.PublicID = id }, taskDB); err != nil {
			return err
		}
		runDB.TaskID = taskDB.ID
		if err := createWithPublicID(ctx, tx, "uq_task_run_public_id",
			func(id string) { runDB.PublicID = id }, runDB); err != nil {
			return err
		}
		// The task names its latest run, so the run has to exist first.
		return tx.Model(&taskRow{}).Where("id = ?", taskDB.ID).
			Update("last_run_id", runDB.ID).Error
	})
	if err != nil {
		return nil, err
	}
	taskDB.LastRunID = &runDB.ID
	if teamID == "" {
		resolved, err := publicIDForKey(ctx, s.db, "team", taskDB.TeamID)
		if err != nil {
			return nil, err
		}
		teamID = resolved
	}
	return toTask(&taskReadRow{
		Row:                  *taskDB,
		ConversationPublicID: canonicalPublicID(in.ConversationID),
		TeamPublicID:         optionalCanonicalPublicID(&teamID),
		CreatedByPublicID:    canonicalPublicID(in.CreatedBy),
		LastRunPublicID:      &runDB.PublicID,
		IssuePublicID:        optionalCanonicalPublicID(in.IssueID),
		AgentPublicID:        optionalCanonicalPublicID(in.AgentID),
	}), nil
}

func defaultString(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}

func buildTaskUpdates(status string, startedAt, endedAt *time.Time, output, errorMessage, sessionID *string) map[string]interface{} {
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
	id, ok := util.CanonicalPublicID(in.TaskID)
	if !ok {
		return model.ErrNotFound
	}
	return s.db.WithContext(ctx).Model(&taskRow{}).Where("public_id = ?", id).Updates(
		buildTaskUpdates(in.Status, in.StartedAt, in.EndedAt, in.Output, in.ErrorMessage, in.SessionID),
	).Error
}

// ClaimTask updates a task's status and optional fields only when current status equals expectedStatus.
// Returns updated = (exactly one row was updated). Used for atomic claim (e.g. PENDING→SCHEDULED, SCHEDULED→RUNNING).
func (s *Store) ClaimTask(ctx context.Context, in model.ClaimTaskInput) (bool, error) {
	id, ok := util.CanonicalPublicID(in.TaskID)
	if !ok {
		return false, nil
	}
	result := s.db.WithContext(ctx).Model(&taskRow{}).Where("public_id = ? AND status = ?", id, in.ExpectedStatus).Updates(
		buildTaskUpdates(in.NewStatus, in.StartedAt, in.EndedAt, in.Output, in.ErrorMessage, in.SessionID),
	)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}
