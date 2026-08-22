package db

import (
	"context"
	"errors"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"time"

	"github.com/gougoujiang/buildmax/internal/util"
	"gorm.io/gorm"
)

type workflowRow struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	PublicID    string `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_workflow_public_id;not null"`
	TeamID      uint64 `gorm:"column:team_id;not null;index"`
	Name        string `gorm:"type:varchar(255);not null"`
	Description string `gorm:"type:text;not null"`
	Definition  string `gorm:"type:longtext;not null"`
	Status      string `gorm:"type:varchar(32);not null;default:'draft'"`
	Revision    int    `gorm:"column:revision;not null;default:1"`
	CreatedBy   uint64 `gorm:"column:created_by;not null"`
	CreatedAt   int64  `gorm:"autoCreateTime"`
	UpdatedAt   int64  `gorm:"autoUpdateTime"`
}

func (workflowRow) TableName() string { return "workflow" }

// workflowReadRow is the row plus the handles its references resolve to.
type workflowReadRow struct {
	Row               workflowRow `gorm:"embedded"`
	TeamPublicID      string      `gorm:"column:team_public_id"`
	CreatedByPublicID string      `gorm:"column:created_by_public_id"`
}

func (s *Store) workflowSelect(ctx context.Context) *gorm.DB {
	return workflowSelectTx(s.db.WithContext(ctx))
}

func workflowSelectTx(tx *gorm.DB) *gorm.DB {
	return tx.Model(&workflowRow{}).
		Select("workflow.*, t.public_id AS team_public_id, cb.public_id AS created_by_public_id").
		Joins("INNER JOIN team t ON t.id = workflow.team_id").
		Joins("INNER JOIN `user` cb ON cb.id = workflow.created_by")
}

// workflowRevisionRow is one recorded version of a workflow. Rows are appended,
// never updated or deleted.
type workflowRevisionRow struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	WorkflowID  uint64 `gorm:"column:workflow_id;not null;index:idx_workflow_revision,unique,priority:1"`
	Revision    int    `gorm:"column:revision;not null;index:idx_workflow_revision,unique,priority:2"`
	Name        string `gorm:"type:varchar(255);not null"`
	Description string `gorm:"type:text;not null"`
	Definition  string `gorm:"type:longtext;not null"`
	Status      string `gorm:"type:varchar(32);not null"`
	CreatedBy   uint64 `gorm:"column:created_by;not null"`
	CreatedAt   int64  `gorm:"autoCreateTime"`
}

func (workflowRevisionRow) TableName() string { return "workflow_revision" }

// workflowRevisionReadRow is the row plus the handles its references resolve
// to. Like an agent revision, it has none of its own.
type workflowRevisionReadRow struct {
	Row               workflowRevisionRow `gorm:"embedded"`
	WorkflowPublicID  string              `gorm:"column:workflow_public_id"`
	CreatedByPublicID string              `gorm:"column:created_by_public_id"`
}

func (s *Store) workflowRevisionSelect(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Model(&workflowRevisionRow{}).
		Select("workflow_revision.*, w.public_id AS workflow_public_id, cb.public_id AS created_by_public_id").
		Joins("INNER JOIN workflow w ON w.id = workflow_revision.workflow_id").
		Joins("INNER JOIN `user` cb ON cb.id = workflow_revision.created_by")
}

type workflowRunRow struct {
	ID               uint64  `gorm:"primaryKey;autoIncrement"`
	PublicID         string  `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_workflow_run_public_id;not null"`
	WorkflowID       uint64  `gorm:"column:workflow_id;not null;index:idx_workflow_run_workflow_created,priority:1"`
	WorkflowRevision int     `gorm:"column:workflow_revision;not null;default:0"`
	IssueID          *uint64 `gorm:"column:issue_id;index"`
	ConversationID   uint64  `gorm:"column:conversation_id;not null;index"`
	Status           string  `gorm:"type:varchar(32);not null"`
	CreatedBy        uint64  `gorm:"column:created_by;not null"`
	CreatedAt        int64   `gorm:"autoCreateTime;index:idx_workflow_run_workflow_created,priority:2"`
	StartedAt        *int64  `gorm:""`
	EndedAt          *int64  `gorm:""`
	ErrorMessage     *string `gorm:"type:text"`
}

func (workflowRunRow) TableName() string { return "workflow_run" }

// workflowRunReadRow is the row plus the handles its references resolve to. A
// pointer field is one a LEFT JOIN may leave NULL.
type workflowRunReadRow struct {
	Row                  workflowRunRow `gorm:"embedded"`
	WorkflowPublicID     string         `gorm:"column:workflow_public_id"`
	IssuePublicID        *string        `gorm:"column:issue_public_id"`
	ConversationPublicID string         `gorm:"column:conversation_public_id"`
	CreatedByPublicID    string         `gorm:"column:created_by_public_id"`
}

func (s *Store) workflowRunSelect(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Model(&workflowRunRow{}).
		Select("workflow_run.*, w.public_id AS workflow_public_id, i.public_id AS issue_public_id, " +
			"c.public_id AS conversation_public_id, cb.public_id AS created_by_public_id").
		Joins("INNER JOIN workflow w ON w.id = workflow_run.workflow_id").
		Joins("LEFT JOIN issue i ON i.id = workflow_run.issue_id").
		Joins("INNER JOIN conversation c ON c.id = workflow_run.conversation_id").
		Joins("INNER JOIN `user` cb ON cb.id = workflow_run.created_by")
}

type workflowStepRunRow struct {
	ID            uint64  `gorm:"primaryKey;autoIncrement"`
	PublicID      string  `gorm:"column:public_id;type:char(20) CHARACTER SET ascii COLLATE ascii_bin;uniqueIndex:uq_workflow_step_run_public_id;not null"`
	WorkflowRunID uint64  `gorm:"column:workflow_run_id;not null;index:idx_step_run_run_index,priority:1"`
	StepID        string  `gorm:"column:step_id;type:varchar(128);not null"`
	StepIndex     int     `gorm:"column:step_index;not null;index:idx_step_run_run_index,priority:2"`
	StepType      string  `gorm:"column:step_type;type:varchar(32);not null"`
	TargetAgentID *uint64 `gorm:"column:target_agent_id;index"`
	// Agent definition captured when the run started; empty on rows written before
	// step runs snapshotted their agent.
	AgentName         string  `gorm:"column:agent_name;type:varchar(255);not null"`
	AgentDescription  string  `gorm:"column:agent_description;type:text;not null"`
	AgentInstructions string  `gorm:"column:agent_instructions;type:longtext;not null"`
	AgentRevision     int     `gorm:"column:agent_revision;not null;default:0"`
	Prompt            string  `gorm:"type:text;not null"`
	Status            string  `gorm:"type:varchar(32);not null"`
	TaskID            *uint64 `gorm:"column:task_id;index"`
	TaskRunID         *uint64 `gorm:"column:task_run_id;index"`
	OutputSummary     *string `gorm:"type:text"`
	ErrorMessage      *string `gorm:"type:text"`
	CreatedAt         int64   `gorm:"autoCreateTime"`
	StartedAt         *int64  `gorm:""`
	EndedAt           *int64  `gorm:""`
}

func (workflowStepRunRow) TableName() string { return "workflow_step_run" }

// workflowStepRunReadRow is the row plus the handles its references resolve to.
// A pointer field is one a LEFT JOIN may leave NULL.
type workflowStepRunReadRow struct {
	Row                 workflowStepRunRow `gorm:"embedded"`
	WorkflowRunPublicID string             `gorm:"column:workflow_run_public_id"`
	TargetAgentPublicID *string            `gorm:"column:target_agent_public_id"`
	TaskPublicID        *string            `gorm:"column:task_public_id"`
	TaskRunPublicID     *string            `gorm:"column:task_run_public_id"`
}

func (s *Store) workflowStepRunSelect(ctx context.Context) *gorm.DB {
	return workflowStepRunSelectTx(s.db.WithContext(ctx))
}

func workflowStepRunSelectTx(tx *gorm.DB) *gorm.DB {
	return tx.Model(&workflowStepRunRow{}).
		Select("workflow_step_run.*, wr.public_id AS workflow_run_public_id, a.public_id AS target_agent_public_id, " +
			"t.public_id AS task_public_id, r.public_id AS task_run_public_id").
		Joins("INNER JOIN workflow_run wr ON wr.id = workflow_step_run.workflow_run_id").
		Joins("LEFT JOIN agent a ON a.id = workflow_step_run.target_agent_id").
		Joins("LEFT JOIN task t ON t.id = workflow_step_run.task_id").
		Joins("LEFT JOIN task_run r ON r.id = workflow_step_run.task_run_id")
}

func toWorkflow(row *workflowReadRow) *model.Workflow {
	if row == nil {
		return nil
	}
	return &model.Workflow{
		ID:          row.Row.PublicID,
		TeamID:      row.TeamPublicID,
		Name:        row.Row.Name,
		Description: row.Row.Description,
		Definition:  row.Row.Definition,
		Status:      row.Row.Status,
		Revision:    row.Row.Revision,
		CreatedBy:   row.CreatedByPublicID,
		CreatedAt:   row.Row.CreatedAt,
		UpdatedAt:   row.Row.UpdatedAt,
	}
}

func toWorkflows(rows []workflowReadRow) []model.Workflow {
	out := make([]model.Workflow, len(rows))
	for i := range rows {
		out[i] = *toWorkflow(&rows[i])
	}
	return out
}

func toWorkflowRevision(row *workflowRevisionReadRow) *model.WorkflowRevision {
	if row == nil {
		return nil
	}
	return &model.WorkflowRevision{
		WorkflowID:  row.WorkflowPublicID,
		Revision:    row.Row.Revision,
		Name:        row.Row.Name,
		Description: row.Row.Description,
		Definition:  row.Row.Definition,
		Status:      row.Row.Status,
		CreatedBy:   row.CreatedByPublicID,
		CreatedAt:   row.Row.CreatedAt,
	}
}

func toWorkflowRevisions(rows []workflowRevisionReadRow) []model.WorkflowRevision {
	out := make([]model.WorkflowRevision, len(rows))
	for i := range rows {
		out[i] = *toWorkflowRevision(&rows[i])
	}
	return out
}

func toWorkflowRun(row *workflowRunReadRow) *model.WorkflowRun {
	if row == nil {
		return nil
	}
	out := &model.WorkflowRun{
		ID:               row.Row.PublicID,
		WorkflowID:       row.WorkflowPublicID,
		WorkflowRevision: row.Row.WorkflowRevision,
		ConversationID:   row.ConversationPublicID,
		Status:           row.Row.Status,
		CreatedBy:        row.CreatedByPublicID,
		CreatedAt:        row.Row.CreatedAt,
		StartedAt:        row.Row.StartedAt,
		EndedAt:          row.Row.EndedAt,
		ErrorMessage:     row.Row.ErrorMessage,
	}
	if row.Row.IssueID != nil {
		issue := derefPublicID(row.IssuePublicID)
		out.IssueID = &issue
	}
	return out
}

func toWorkflowRuns(rows []workflowRunReadRow) []model.WorkflowRun {
	out := make([]model.WorkflowRun, len(rows))
	for i := range rows {
		out[i] = *toWorkflowRun(&rows[i])
	}
	return out
}

func toWorkflowStepRun(row *workflowStepRunReadRow) *model.WorkflowStepRun {
	if row == nil {
		return nil
	}
	out := &model.WorkflowStepRun{
		ID:                row.Row.PublicID,
		WorkflowRunID:     row.WorkflowRunPublicID,
		StepID:            row.Row.StepID,
		StepIndex:         row.Row.StepIndex,
		StepType:          row.Row.StepType,
		AgentName:         row.Row.AgentName,
		AgentDescription:  row.Row.AgentDescription,
		AgentInstructions: row.Row.AgentInstructions,
		AgentRevision:     row.Row.AgentRevision,
		Prompt:            row.Row.Prompt,
		Status:            row.Row.Status,
		OutputSummary:     row.Row.OutputSummary,
		ErrorMessage:      row.Row.ErrorMessage,
		CreatedAt:         row.Row.CreatedAt,
		StartedAt:         row.Row.StartedAt,
		EndedAt:           row.Row.EndedAt,
	}
	if row.Row.TargetAgentID != nil {
		agent := derefPublicID(row.TargetAgentPublicID)
		out.TargetAgentID = &agent
	}
	if row.Row.TaskID != nil {
		task := derefPublicID(row.TaskPublicID)
		out.TaskID = &task
	}
	if row.Row.TaskRunID != nil {
		run := derefPublicID(row.TaskRunPublicID)
		out.TaskRunID = &run
	}
	return out
}

func toWorkflowStepRuns(rows []workflowStepRunReadRow) []model.WorkflowStepRun {
	out := make([]model.WorkflowStepRun, len(rows))
	for i := range rows {
		out[i] = *toWorkflowStepRun(&rows[i])
	}
	return out
}

func (s *Store) ListWorkflowsByTeam(ctx context.Context, teamID string) ([]model.Workflow, error) {
	id, ok := util.CanonicalPublicID(teamID)
	if !ok {
		return nil, nil
	}
	var list []workflowReadRow
	err := s.workflowSelect(ctx).Where("t.public_id = ?", id).Order("workflow.created_at ASC").Find(&list).Error
	return toWorkflows(list), err
}

func (s *Store) CreateWorkflow(ctx context.Context, teamID, createdBy, name, description, definition string) (*model.Workflow, error) {
	now := time.Now().Unix()
	workflow := &model.Workflow{
		TeamID:      teamID,
		Name:        name,
		Description: description,
		Definition:  definition,
		Status:      model.WorkflowStatusDraft,
		Revision:    1,
		CreatedBy:   createdBy,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	row := &workflowRow{
		Name:        name,
		Description: description,
		Definition:  definition,
		Status:      model.WorkflowStatusDraft,
		Revision:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		teamKey, err := lookupKey(ctx, tx, "team", teamID)
		if err != nil {
			return err
		}
		row.TeamID = teamKey
		creator, err := lookupKey(ctx, tx, "user", createdBy)
		if err != nil {
			return err
		}
		row.CreatedBy = creator
		if err := createWithPublicID(ctx, tx, "uq_workflow_public_id",
			func(id string) { row.PublicID = id }, row); err != nil {
			return err
		}
		return appendWorkflowRevision(tx, row.ID, creator, workflow)
	})
	if err != nil {
		return nil, err
	}
	workflow.ID = row.PublicID
	return workflow, nil
}

// appendWorkflowRevision records the workflow's current content as its
// revision. It runs in the same transaction as the write it describes, and the
// unique (workflow_id, revision) index makes a concurrent second write fail
// rather than record two definitions under one number.
func appendWorkflowRevision(tx *gorm.DB, workflowKey, createdBy uint64, w *model.Workflow) error {
	return tx.Create(&workflowRevisionRow{
		WorkflowID:  workflowKey,
		Revision:    w.Revision,
		Name:        w.Name,
		Description: w.Description,
		Definition:  w.Definition,
		Status:      w.Status,
		CreatedBy:   createdBy,
		CreatedAt:   time.Now().Unix(),
	}).Error
}

func (s *Store) GetWorkflow(ctx context.Context, workflowID string) (*model.Workflow, error) {
	id, ok := util.CanonicalPublicID(workflowID)
	if !ok {
		return nil, nil
	}
	var workflow workflowReadRow
	err := s.workflowSelect(ctx).Where("workflow.public_id = ?", id).Take(&workflow).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toWorkflow(&workflow), nil
}

func (s *Store) UpdateWorkflow(ctx context.Context, workflowID, teamID string, in model.UpdateWorkflowInput) (*model.Workflow, error) {
	workflow, err := s.GetWorkflow(ctx, workflowID)
	if err != nil || workflow == nil {
		return nil, err
	}
	if workflow.TeamID != teamID {
		return nil, nil
	}
	updated := *workflow
	if in.Name != nil {
		updated.Name = *in.Name
	}
	if in.Description != nil {
		updated.Description = *in.Description
	}
	if in.Definition != nil {
		updated.Definition = *in.Definition
	}
	if in.Status != nil {
		updated.Status = *in.Status
	}
	// A save that changes nothing is not a revision. Status counts as content
	// here: publishing is the act that lets a workflow run, so the record of who
	// published which definition is exactly what history is for.
	if updated.Name == workflow.Name && updated.Description == workflow.Description &&
		updated.Definition == workflow.Definition && updated.Status == workflow.Status {
		return workflow, nil
	}
	updated.Revision = nextRevision(workflow.Revision)
	updated.UpdatedAt = time.Now().Unix()
	updates := map[string]interface{}{
		"name":        updated.Name,
		"description": updated.Description,
		"definition":  updated.Definition,
		"status":      updated.Status,
		"revision":    updated.Revision,
		"updated_at":  updated.UpdatedAt,
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		workflowKey, err := lookupKey(ctx, tx, "workflow", workflowID)
		if err != nil {
			return err
		}
		if err := tx.Model(&workflowRow{}).Where("id = ?", workflowKey).Updates(updates).Error; err != nil {
			return err
		}
		updatedBy, err := lookupKey(ctx, tx, "user", in.UpdatedBy)
		if err != nil {
			return err
		}
		return appendWorkflowRevision(tx, workflowKey, updatedBy, &updated)
	})
	if err != nil {
		return nil, err
	}
	return s.GetWorkflow(ctx, workflowID)
}

// ListWorkflowRevisions returns a workflow's revisions, newest first.
func (s *Store) ListWorkflowRevisions(ctx context.Context, workflowID string, limit, offset int) ([]model.WorkflowRevision, int, error) {
	limit, offset = capPage(limit, offset)
	workflowKey, err := lookupKey(ctx, s.db, "workflow", workflowID)
	if errors.Is(err, model.ErrNotFound) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	rows, total, err := listRevisions[workflowRevisionReadRow](ctx, s.db, s.workflowRevisionSelect(ctx),
		"workflow_revision", "workflow_id", workflowKey, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return toWorkflowRevisions(rows), total, nil
}

// GetWorkflowRevision returns one revision, or (nil, nil) when there is no such revision.
func (s *Store) GetWorkflowRevision(ctx context.Context, workflowID string, revision int) (*model.WorkflowRevision, error) {
	workflowKey, err := lookupKey(ctx, s.db, "workflow", workflowID)
	if errors.Is(err, model.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row, err := getRevision[workflowRevisionReadRow](s.workflowRevisionSelect(ctx),
		"workflow_revision", "workflow_id", workflowKey, revision)
	if err != nil || row == nil {
		return nil, err
	}
	return toWorkflowRevision(row), nil
}

func (s *Store) CreateWorkflowRun(ctx context.Context, in model.CreateWorkflowRunInput) (*model.WorkflowRun, error) {
	now := time.Now().Unix()
	run := &model.WorkflowRun{
		WorkflowID:       in.WorkflowID,
		WorkflowRevision: in.WorkflowRevision,
		IssueID:          in.IssueID,
		ConversationID:   in.ConversationID,
		Status:           in.Status,
		CreatedBy:        in.CreatedBy,
		CreatedAt:        now,
		StartedAt:        in.StartedAt,
	}
	row := &workflowRunRow{
		WorkflowRevision: in.WorkflowRevision,
		Status:           in.Status,
		CreatedAt:        now,
		StartedAt:        in.StartedAt,
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		workflowKey, err := lookupKey(ctx, tx, "workflow", in.WorkflowID)
		if err != nil {
			return err
		}
		row.WorkflowID = workflowKey
		convKey, err := lookupKey(ctx, tx, "conversation", in.ConversationID)
		if err != nil {
			return err
		}
		row.ConversationID = convKey
		creator, err := lookupKey(ctx, tx, "user", in.CreatedBy)
		if err != nil {
			return err
		}
		row.CreatedBy = creator
		if in.IssueID != nil && *in.IssueID != "" {
			issueKey, err := lookupKey(ctx, tx, "issue", *in.IssueID)
			if err != nil {
				return err
			}
			row.IssueID = &issueKey
		}
		return createWithPublicID(ctx, tx, "uq_workflow_run_public_id",
			func(id string) { row.PublicID = id }, row)
	}); err != nil {
		return nil, err
	}
	run.ID = row.PublicID
	return run, nil
}

func (s *Store) ListWorkflowRunsByWorkflow(ctx context.Context, workflowID string, limit, offset int) ([]model.WorkflowRun, int, error) {
	limit, offset = capPage(limit, offset)
	workflowKey, err := lookupKey(ctx, s.db, "workflow", workflowID)
	if errors.Is(err, model.ErrNotFound) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := s.db.WithContext(ctx).Model(&workflowRunRow{}).Where("workflow_id = ?", workflowKey).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []workflowRunReadRow
	q := s.workflowRunSelect(ctx).Where("workflow_run.workflow_id = ?", workflowKey).Order("workflow_run.created_at DESC")
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return toWorkflowRuns(list), int(total), nil
}

func (s *Store) ListWorkflowRunsByIssue(ctx context.Context, issueID string, limit, offset int) ([]model.WorkflowRun, int, error) {
	limit, offset = capPage(limit, offset)
	issueKey, err := lookupKey(ctx, s.db, "issue", issueID)
	if errors.Is(err, model.ErrNotFound) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := s.db.WithContext(ctx).Model(&workflowRunRow{}).Where("issue_id = ?", issueKey).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []workflowRunReadRow
	q := s.workflowRunSelect(ctx).Where("workflow_run.issue_id = ?", issueKey).Order("workflow_run.created_at DESC")
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return toWorkflowRuns(list), int(total), nil
}

func (s *Store) GetWorkflowRun(ctx context.Context, workflowRunID string) (*model.WorkflowRun, error) {
	id, ok := util.CanonicalPublicID(workflowRunID)
	if !ok {
		return nil, nil
	}
	var run workflowRunReadRow
	err := s.workflowRunSelect(ctx).Where("workflow_run.public_id = ?", id).Take(&run).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toWorkflowRun(&run), nil
}

func (s *Store) ListWorkflowStepRuns(ctx context.Context, workflowRunID string) ([]model.WorkflowStepRun, error) {
	runKey, err := lookupKey(ctx, s.db, "workflow_run", workflowRunID)
	if errors.Is(err, model.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []workflowStepRunReadRow
	err = s.workflowStepRunSelect(ctx).
		Where("workflow_step_run.workflow_run_id = ?", runKey).
		Order("workflow_step_run.step_index ASC, workflow_step_run.created_at ASC").
		Find(&list).Error
	return toWorkflowStepRuns(list), err
}

func (s *Store) CreateWorkflowStepRuns(ctx context.Context, workflowRunID string, steps []model.CreateWorkflowStepRunInput) ([]model.WorkflowStepRun, error) {
	if len(steps) == 0 {
		return []model.WorkflowStepRun{}, nil
	}
	now := time.Now().Unix()
	runKey, err := lookupKey(ctx, s.db, "workflow_run", workflowRunID)
	if err != nil {
		return nil, err
	}
	out := make([]model.WorkflowStepRun, len(steps))
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range steps {
			row := &workflowStepRunRow{
				WorkflowRunID:     runKey,
				StepID:            steps[i].StepID,
				StepIndex:         steps[i].StepIndex,
				StepType:          steps[i].StepType,
				AgentName:         steps[i].AgentName,
				AgentDescription:  steps[i].AgentDescription,
				AgentInstructions: steps[i].AgentInstructions,
				AgentRevision:     steps[i].AgentRevision,
				Prompt:            steps[i].Prompt,
				Status:            steps[i].Status,
				CreatedAt:         now,
			}
			if steps[i].TargetAgentID != nil && *steps[i].TargetAgentID != "" {
				agentKey, err := lookupKey(ctx, tx, "agent", *steps[i].TargetAgentID)
				if err != nil {
					return err
				}
				row.TargetAgentID = &agentKey
			}
			if err := createWithPublicID(ctx, tx, "uq_workflow_step_run_public_id",
				func(id string) { row.PublicID = id }, row); err != nil {
				return err
			}
			out[i] = *toWorkflowStepRun(&workflowStepRunReadRow{
				Row:                 *row,
				WorkflowRunPublicID: canonicalPublicID(workflowRunID),
				TargetAgentPublicID: optionalCanonicalPublicID(steps[i].TargetAgentID),
			})
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) UpdateWorkflowRun(ctx context.Context, workflowRunID string, in model.UpdateWorkflowRunInput) (*model.WorkflowRun, error) {
	updates := map[string]interface{}{
		"status": in.Status,
	}
	if in.StartedAt != nil {
		updates["started_at"] = *in.StartedAt
	}
	if in.EndedAt != nil {
		updates["ended_at"] = *in.EndedAt
	}
	if in.ErrorMessage != nil {
		updates["error_message"] = *in.ErrorMessage
	}
	id, ok := util.CanonicalPublicID(workflowRunID)
	if !ok {
		return nil, nil
	}
	if err := s.db.WithContext(ctx).Model(&workflowRunRow{}).Where("public_id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetWorkflowRun(ctx, workflowRunID)
}

func (s *Store) UpdateWorkflowStepRun(ctx context.Context, stepRunID string, in model.UpdateWorkflowStepRunInput) (*model.WorkflowStepRun, error) {
	updates := map[string]interface{}{}
	if in.Status != nil {
		updates["status"] = *in.Status
	}
	if in.TaskID != nil {
		if *in.TaskID == "" {
			updates["task_id"] = nil
		} else {
			key, err := lookupKey(ctx, s.db, "task", *in.TaskID)
			if err != nil {
				return nil, err
			}
			updates["task_id"] = key
		}
	}
	if in.TaskRunID != nil {
		if *in.TaskRunID == "" {
			updates["task_run_id"] = nil
		} else {
			key, err := lookupKey(ctx, s.db, "task_run", *in.TaskRunID)
			if err != nil {
				return nil, err
			}
			updates["task_run_id"] = key
		}
	}
	if in.OutputSummary != nil {
		if *in.OutputSummary == "" {
			updates["output_summary"] = nil
		} else {
			updates["output_summary"] = *in.OutputSummary
		}
	}
	if in.ErrorMessage != nil {
		if *in.ErrorMessage == "" {
			updates["error_message"] = nil
		} else {
			updates["error_message"] = *in.ErrorMessage
		}
	}
	if in.StartedAt != nil {
		updates["started_at"] = *in.StartedAt
	}
	if in.EndedAt != nil {
		updates["ended_at"] = *in.EndedAt
	}
	if len(updates) == 0 {
		return s.getWorkflowStepRun(ctx, stepRunID)
	}
	id, ok := util.CanonicalPublicID(stepRunID)
	if !ok {
		return nil, nil
	}
	if err := s.db.WithContext(ctx).Model(&workflowStepRunRow{}).Where("public_id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.getWorkflowStepRun(ctx, stepRunID)
}

func (s *Store) GetWorkflowStepRunByTaskID(ctx context.Context, taskID string) (*model.WorkflowStepRun, error) {
	return s.getWorkflowStepRunByOwner(ctx, "task", "workflow_step_run.task_id", taskID)
}

func (s *Store) GetWorkflowStepRunByTaskRunID(ctx context.Context, taskRunID string) (*model.WorkflowStepRun, error) {
	return s.getWorkflowStepRunByOwner(ctx, "task_run", "workflow_step_run.task_run_id", taskRunID)
}

// getWorkflowStepRunByOwner finds the step run that produced a task or a run.
//
// table and col reach a query as text, so both stay constants from this package
// and never values from a request.
func (s *Store) getWorkflowStepRunByOwner(ctx context.Context, table, col, publicID string) (*model.WorkflowStepRun, error) {
	key, err := lookupKey(ctx, s.db, table, publicID)
	if errors.Is(err, model.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var step workflowStepRunReadRow
	err = s.workflowStepRunSelect(ctx).Where(col+" = ?", key).Take(&step).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toWorkflowStepRun(&step), nil
}

func (s *Store) getWorkflowStepRun(ctx context.Context, stepRunID string) (*model.WorkflowStepRun, error) {
	id, ok := util.CanonicalPublicID(stepRunID)
	if !ok {
		return nil, nil
	}
	var step workflowStepRunReadRow
	err := s.workflowStepRunSelect(ctx).Where("workflow_step_run.public_id = ?", id).Take(&step).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toWorkflowStepRun(&step), nil
}
