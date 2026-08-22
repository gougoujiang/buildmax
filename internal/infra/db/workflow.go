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
	ID          uint   `gorm:"primaryKey;autoIncrement"`
	WorkflowID  string `gorm:"column:workflow_id;type:varchar(64);uniqueIndex;not null"`
	TeamID      string `gorm:"column:team_id;type:varchar(64);not null;index"`
	Name        string `gorm:"type:varchar(255);not null"`
	Description string `gorm:"type:text;not null"`
	Definition  string `gorm:"type:longtext;not null"`
	Status      string `gorm:"type:varchar(32);not null;default:'draft'"`
	Revision    int    `gorm:"column:revision;not null;default:1"`
	CreatedBy   string `gorm:"type:varchar(64);not null"`
	CreatedAt   int64  `gorm:"autoCreateTime"`
	UpdatedAt   int64  `gorm:"autoUpdateTime"`
}

func (workflowRow) TableName() string { return "workflow" }

// workflowRevisionRow is one recorded version of a workflow. Rows are appended,
// never updated or deleted.
type workflowRevisionRow struct {
	ID          uint   `gorm:"primaryKey;autoIncrement"`
	WorkflowID  string `gorm:"column:workflow_id;type:varchar(64);not null;index:idx_workflow_revision,unique,priority:1"`
	Revision    int    `gorm:"column:revision;not null;index:idx_workflow_revision,unique,priority:2"`
	Name        string `gorm:"type:varchar(255);not null"`
	Description string `gorm:"type:text;not null"`
	Definition  string `gorm:"type:longtext;not null"`
	Status      string `gorm:"type:varchar(32);not null"`
	CreatedBy   string `gorm:"column:created_by;type:varchar(64);not null"`
	CreatedAt   int64  `gorm:"autoCreateTime"`
}

func (workflowRevisionRow) TableName() string { return "workflow_revision" }

type workflowRunRow struct {
	ID               uint    `gorm:"primaryKey;autoIncrement"`
	WorkflowRunID    string  `gorm:"column:workflow_run_id;type:varchar(64);uniqueIndex;not null"`
	WorkflowID       string  `gorm:"column:workflow_id;type:varchar(64);not null;index"`
	WorkflowRevision int     `gorm:"column:workflow_revision;not null;default:0"`
	IssueID          *string `gorm:"column:issue_id;type:varchar(64);index"`
	ConversationID   string  `gorm:"column:conversation_id;type:varchar(64);not null;index"`
	Status           string  `gorm:"type:varchar(32);not null"`
	CreatedBy        string  `gorm:"type:varchar(64);not null"`
	CreatedAt        int64   `gorm:"autoCreateTime"`
	StartedAt        *int64  `gorm:""`
	EndedAt          *int64  `gorm:""`
	ErrorMessage     *string `gorm:"type:text"`
}

func (workflowRunRow) TableName() string { return "workflow_run" }

type workflowStepRunRow struct {
	ID            uint    `gorm:"primaryKey;autoIncrement"`
	StepRunID     string  `gorm:"column:workflow_step_run_id;type:varchar(64);uniqueIndex;not null"`
	WorkflowRunID string  `gorm:"column:workflow_run_id;type:varchar(64);not null;index"`
	StepID        string  `gorm:"column:step_id;type:varchar(128);not null"`
	StepIndex     int     `gorm:"column:step_index;not null"`
	StepType      string  `gorm:"column:step_type;type:varchar(32);not null"`
	TargetAgentID *string `gorm:"column:target_agent_id;type:varchar(64);index"`
	// Agent definition captured when the run started; empty on rows written before
	// step runs snapshotted their agent.
	AgentName         string  `gorm:"column:agent_name;type:varchar(255);not null"`
	AgentDescription  string  `gorm:"column:agent_description;type:text;not null"`
	AgentInstructions string  `gorm:"column:agent_instructions;type:longtext;not null"`
	AgentRevision     int     `gorm:"column:agent_revision;not null;default:0"`
	Prompt            string  `gorm:"type:text;not null"`
	Status            string  `gorm:"type:varchar(32);not null"`
	TaskID            *string `gorm:"column:task_id;type:varchar(64);index"`
	TaskRunID         *string `gorm:"column:task_run_id;type:varchar(64);index"`
	OutputSummary     *string `gorm:"type:text"`
	ErrorMessage      *string `gorm:"type:text"`
	CreatedAt         int64   `gorm:"autoCreateTime"`
	StartedAt         *int64  `gorm:""`
	EndedAt           *int64  `gorm:""`
}

func (workflowStepRunRow) TableName() string { return "workflow_step_run" }

func toWorkflow(row *workflowRow) *model.Workflow {
	if row == nil {
		return nil
	}
	return &model.Workflow{
		ID:          row.WorkflowID,
		TeamID:      row.TeamID,
		Name:        row.Name,
		Description: row.Description,
		Definition:  row.Definition,
		Status:      row.Status,
		Revision:    row.Revision,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func toWorkflows(rows []workflowRow) []model.Workflow {
	out := make([]model.Workflow, len(rows))
	for i := range rows {
		out[i] = *toWorkflow(&rows[i])
	}
	return out
}

func toWorkflowRow(m *model.Workflow) *workflowRow {
	if m == nil {
		return nil
	}
	return &workflowRow{
		WorkflowID:  m.ID,
		TeamID:      m.TeamID,
		Name:        m.Name,
		Description: m.Description,
		Definition:  m.Definition,
		Status:      m.Status,
		Revision:    m.Revision,
		CreatedBy:   m.CreatedBy,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func toWorkflowRevision(row *workflowRevisionRow) *model.WorkflowRevision {
	if row == nil {
		return nil
	}
	return &model.WorkflowRevision{
		WorkflowID:  row.WorkflowID,
		Revision:    row.Revision,
		Name:        row.Name,
		Description: row.Description,
		Definition:  row.Definition,
		Status:      row.Status,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt,
	}
}

func toWorkflowRevisions(rows []workflowRevisionRow) []model.WorkflowRevision {
	out := make([]model.WorkflowRevision, len(rows))
	for i := range rows {
		out[i] = *toWorkflowRevision(&rows[i])
	}
	return out
}

func toWorkflowRun(row *workflowRunRow) *model.WorkflowRun {
	if row == nil {
		return nil
	}
	return &model.WorkflowRun{
		ID:               row.WorkflowRunID,
		WorkflowID:       row.WorkflowID,
		WorkflowRevision: row.WorkflowRevision,
		IssueID:          row.IssueID,
		ConversationID:   row.ConversationID,
		Status:           row.Status,
		CreatedBy:        row.CreatedBy,
		CreatedAt:        row.CreatedAt,
		StartedAt:        row.StartedAt,
		EndedAt:          row.EndedAt,
		ErrorMessage:     row.ErrorMessage,
	}
}

func toWorkflowRuns(rows []workflowRunRow) []model.WorkflowRun {
	out := make([]model.WorkflowRun, len(rows))
	for i := range rows {
		out[i] = *toWorkflowRun(&rows[i])
	}
	return out
}

func toWorkflowRunRow(m *model.WorkflowRun) *workflowRunRow {
	if m == nil {
		return nil
	}
	return &workflowRunRow{
		WorkflowRunID:    m.ID,
		WorkflowID:       m.WorkflowID,
		WorkflowRevision: m.WorkflowRevision,
		IssueID:          m.IssueID,
		ConversationID:   m.ConversationID,
		Status:           m.Status,
		CreatedBy:        m.CreatedBy,
		CreatedAt:        m.CreatedAt,
		StartedAt:        m.StartedAt,
		EndedAt:          m.EndedAt,
		ErrorMessage:     m.ErrorMessage,
	}
}

func toWorkflowStepRun(row *workflowStepRunRow) *model.WorkflowStepRun {
	if row == nil {
		return nil
	}
	return &model.WorkflowStepRun{
		ID:                row.StepRunID,
		WorkflowRunID:     row.WorkflowRunID,
		StepID:            row.StepID,
		StepIndex:         row.StepIndex,
		StepType:          row.StepType,
		TargetAgentID:     row.TargetAgentID,
		AgentName:         row.AgentName,
		AgentDescription:  row.AgentDescription,
		AgentInstructions: row.AgentInstructions,
		AgentRevision:     row.AgentRevision,
		Prompt:            row.Prompt,
		Status:            row.Status,
		TaskID:            row.TaskID,
		TaskRunID:         row.TaskRunID,
		OutputSummary:     row.OutputSummary,
		ErrorMessage:      row.ErrorMessage,
		CreatedAt:         row.CreatedAt,
		StartedAt:         row.StartedAt,
		EndedAt:           row.EndedAt,
	}
}

func toWorkflowStepRuns(rows []workflowStepRunRow) []model.WorkflowStepRun {
	out := make([]model.WorkflowStepRun, len(rows))
	for i := range rows {
		out[i] = *toWorkflowStepRun(&rows[i])
	}
	return out
}

func (s *Store) ListWorkflowsByTeam(ctx context.Context, teamID string) ([]model.Workflow, error) {
	var list []workflowRow
	err := s.db.WithContext(ctx).Where("team_id = ?", teamID).Order("created_at ASC").Find(&list).Error
	return toWorkflows(list), err
}

func (s *Store) CreateWorkflow(ctx context.Context, teamID, createdBy, name, description, definition string) (*model.Workflow, error) {
	now := time.Now().Unix()
	publicID, err := util.NewPublicID()
	if err != nil {
		return nil, err
	}
	workflow := &model.Workflow{
		ID:          publicID,
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
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(toWorkflowRow(workflow)).Error; err != nil {
			return err
		}
		return appendWorkflowRevision(tx, workflow, createdBy)
	})
	if err != nil {
		return nil, err
	}
	return workflow, nil
}

// appendWorkflowRevision records the workflow's current content as its
// revision. It runs in the same transaction as the write it describes, and the
// unique (workflow_id, revision) index makes a concurrent second write fail
// rather than record two definitions under one number.
func appendWorkflowRevision(tx *gorm.DB, w *model.Workflow, createdBy string) error {
	return tx.Create(&workflowRevisionRow{
		WorkflowID:  w.ID,
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
	var workflow workflowRow
	err := s.db.WithContext(ctx).Where("workflow_id = ?", workflowID).First(&workflow).Error
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
		if err := tx.Model(&workflowRow{}).Where("workflow_id = ?", workflowID).Updates(updates).Error; err != nil {
			return err
		}
		return appendWorkflowRevision(tx, &updated, in.UpdatedBy)
	})
	if err != nil {
		return nil, err
	}
	return s.GetWorkflow(ctx, workflowID)
}

// ListWorkflowRevisions returns a workflow's revisions, newest first.
func (s *Store) ListWorkflowRevisions(ctx context.Context, workflowID string, limit, offset int) ([]model.WorkflowRevision, int, error) {
	limit, offset = capPage(limit, offset)
	rows, total, err := listRevisions[workflowRevisionRow](ctx, s.db, "workflow_id", workflowID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return toWorkflowRevisions(rows), total, nil
}

// GetWorkflowRevision returns one revision, or (nil, nil) when there is no such revision.
func (s *Store) GetWorkflowRevision(ctx context.Context, workflowID string, revision int) (*model.WorkflowRevision, error) {
	row, err := getRevision[workflowRevisionRow](ctx, s.db, "workflow_id", workflowID, revision)
	if err != nil || row == nil {
		return nil, err
	}
	return toWorkflowRevision(row), nil
}

func (s *Store) CreateWorkflowRun(ctx context.Context, in model.CreateWorkflowRunInput) (*model.WorkflowRun, error) {
	now := time.Now().Unix()
	publicID, err := util.NewPublicID()
	if err != nil {
		return nil, err
	}
	run := &model.WorkflowRun{
		ID:               publicID,
		WorkflowID:       in.WorkflowID,
		WorkflowRevision: in.WorkflowRevision,
		IssueID:          in.IssueID,
		ConversationID:   in.ConversationID,
		Status:           in.Status,
		CreatedBy:        in.CreatedBy,
		CreatedAt:        now,
		StartedAt:        in.StartedAt,
	}
	if err := s.db.WithContext(ctx).Create(toWorkflowRunRow(run)).Error; err != nil {
		return nil, err
	}
	return run, nil
}

func (s *Store) ListWorkflowRunsByWorkflow(ctx context.Context, workflowID string, limit, offset int) ([]model.WorkflowRun, int, error) {
	limit, offset = capPage(limit, offset)
	q := s.db.WithContext(ctx).Model(&workflowRunRow{}).Where("workflow_id = ?", workflowID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []workflowRunRow
	q = s.db.WithContext(ctx).Where("workflow_id = ?", workflowID).Order("created_at DESC")
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
	q := s.db.WithContext(ctx).Model(&workflowRunRow{}).Where("issue_id = ?", issueID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []workflowRunRow
	q = s.db.WithContext(ctx).Where("issue_id = ?", issueID).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return toWorkflowRuns(list), int(total), nil
}

func (s *Store) GetWorkflowRun(ctx context.Context, workflowRunID string) (*model.WorkflowRun, error) {
	var run workflowRunRow
	err := s.db.WithContext(ctx).Where("workflow_run_id = ?", workflowRunID).First(&run).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toWorkflowRun(&run), nil
}

func (s *Store) ListWorkflowStepRuns(ctx context.Context, workflowRunID string) ([]model.WorkflowStepRun, error) {
	var list []workflowStepRunRow
	err := s.db.WithContext(ctx).
		Where("workflow_run_id = ?", workflowRunID).
		Order("step_index ASC, created_at ASC").
		Find(&list).Error
	return toWorkflowStepRuns(list), err
}

func (s *Store) CreateWorkflowStepRuns(ctx context.Context, workflowRunID string, steps []model.CreateWorkflowStepRunInput) ([]model.WorkflowStepRun, error) {
	if len(steps) == 0 {
		return []model.WorkflowStepRun{}, nil
	}
	now := time.Now().Unix()
	rows := make([]workflowStepRunRow, len(steps))
	for i := range steps {
		publicID, err := util.NewPublicID()
		if err != nil {
			return nil, err
		}
		rows[i] = workflowStepRunRow{
			StepRunID:         publicID,
			WorkflowRunID:     workflowRunID,
			StepID:            steps[i].StepID,
			StepIndex:         steps[i].StepIndex,
			StepType:          steps[i].StepType,
			TargetAgentID:     steps[i].TargetAgentID,
			AgentName:         steps[i].AgentName,
			AgentDescription:  steps[i].AgentDescription,
			AgentInstructions: steps[i].AgentInstructions,
			AgentRevision:     steps[i].AgentRevision,
			Prompt:            steps[i].Prompt,
			Status:            steps[i].Status,
			CreatedAt:         now,
		}
	}
	if err := s.db.WithContext(ctx).Create(&rows).Error; err != nil {
		return nil, err
	}
	return toWorkflowStepRuns(rows), nil
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
	if err := s.db.WithContext(ctx).Model(&workflowRunRow{}).Where("workflow_run_id = ?", workflowRunID).Updates(updates).Error; err != nil {
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
			updates["task_id"] = *in.TaskID
		}
	}
	if in.TaskRunID != nil {
		if *in.TaskRunID == "" {
			updates["task_run_id"] = nil
		} else {
			updates["task_run_id"] = *in.TaskRunID
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
	if err := s.db.WithContext(ctx).Model(&workflowStepRunRow{}).Where("workflow_step_run_id = ?", stepRunID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.getWorkflowStepRun(ctx, stepRunID)
}

func (s *Store) GetWorkflowStepRunByTaskID(ctx context.Context, taskID string) (*model.WorkflowStepRun, error) {
	return s.getWorkflowStepRunByColumn(ctx, "task_id", taskID)
}

func (s *Store) GetWorkflowStepRunByTaskRunID(ctx context.Context, taskRunID string) (*model.WorkflowStepRun, error) {
	return s.getWorkflowStepRunByColumn(ctx, "task_run_id", taskRunID)
}

func (s *Store) getWorkflowStepRunByColumn(ctx context.Context, col, value string) (*model.WorkflowStepRun, error) {
	var step workflowStepRunRow
	err := s.db.WithContext(ctx).Where(col+" = ?", value).First(&step).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toWorkflowStepRun(&step), nil
}

func (s *Store) getWorkflowStepRun(ctx context.Context, stepRunID string) (*model.WorkflowStepRun, error) {
	var step workflowStepRunRow
	err := s.db.WithContext(ctx).Where("workflow_step_run_id = ?", stepRunID).First(&step).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toWorkflowStepRun(&step), nil
}
