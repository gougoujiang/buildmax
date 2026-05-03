package db

import (
	"buildmax/internal/core/model"
	"context"
	"errors"
	"time"

	"buildmax/internal/util"
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
	CreatedBy   string `gorm:"type:varchar(64);not null"`
	CreatedAt   int64  `gorm:"autoCreateTime"`
	UpdatedAt   int64  `gorm:"autoUpdateTime"`
}

func (workflowRow) TableName() string { return "workflow" }

type workflowRunRow struct {
	ID             uint    `gorm:"primaryKey;autoIncrement"`
	WorkflowRunID  string  `gorm:"column:workflow_run_id;type:varchar(64);uniqueIndex;not null"`
	WorkflowID     string  `gorm:"column:workflow_id;type:varchar(64);not null;index"`
	IssueID        *string `gorm:"column:issue_id;type:varchar(64);index"`
	ConversationID string  `gorm:"column:conversation_id;type:varchar(64);not null;index"`
	Status         string  `gorm:"type:varchar(32);not null"`
	CreatedBy      string  `gorm:"type:varchar(64);not null"`
	CreatedAt      int64   `gorm:"autoCreateTime"`
	StartedAt      *int64  `gorm:""`
	EndedAt        *int64  `gorm:""`
	ErrorMessage   *string `gorm:"type:text"`
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
	Prompt        string  `gorm:"type:text;not null"`
	Status        string  `gorm:"type:varchar(32);not null"`
	TaskID        *string `gorm:"column:task_id;type:varchar(64);index"`
	TaskRunID     *string `gorm:"column:task_run_id;type:varchar(64);index"`
	OutputSummary *string `gorm:"type:text"`
	ErrorMessage  *string `gorm:"type:text"`
	CreatedAt     int64   `gorm:"autoCreateTime"`
	StartedAt     *int64  `gorm:""`
	EndedAt       *int64  `gorm:""`
}

func (workflowStepRunRow) TableName() string { return "workflow_step_run" }

func toWorkflow(row *workflowRow) *model.Workflow {
	if row == nil {
		return nil
	}
	return &model.Workflow{
		ID:          row.ID,
		WorkflowID:  row.WorkflowID,
		TeamID:      row.TeamID,
		Name:        row.Name,
		Description: row.Description,
		Definition:  row.Definition,
		Status:      row.Status,
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
		ID:          m.ID,
		WorkflowID:  m.WorkflowID,
		TeamID:      m.TeamID,
		Name:        m.Name,
		Description: m.Description,
		Definition:  m.Definition,
		Status:      m.Status,
		CreatedBy:   m.CreatedBy,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func toWorkflowRun(row *workflowRunRow) *model.WorkflowRun {
	if row == nil {
		return nil
	}
	return &model.WorkflowRun{
		ID:             row.ID,
		WorkflowRunID:  row.WorkflowRunID,
		WorkflowID:     row.WorkflowID,
		IssueID:        row.IssueID,
		ConversationID: row.ConversationID,
		Status:         row.Status,
		CreatedBy:      row.CreatedBy,
		CreatedAt:      row.CreatedAt,
		StartedAt:      row.StartedAt,
		EndedAt:        row.EndedAt,
		ErrorMessage:   row.ErrorMessage,
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
		ID:             m.ID,
		WorkflowRunID:  m.WorkflowRunID,
		WorkflowID:     m.WorkflowID,
		IssueID:        m.IssueID,
		ConversationID: m.ConversationID,
		Status:         m.Status,
		CreatedBy:      m.CreatedBy,
		CreatedAt:      m.CreatedAt,
		StartedAt:      m.StartedAt,
		EndedAt:        m.EndedAt,
		ErrorMessage:   m.ErrorMessage,
	}
}

func toWorkflowStepRun(row *workflowStepRunRow) *model.WorkflowStepRun {
	if row == nil {
		return nil
	}
	return &model.WorkflowStepRun{
		ID:            row.ID,
		StepRunID:     row.StepRunID,
		WorkflowRunID: row.WorkflowRunID,
		StepID:        row.StepID,
		StepIndex:     row.StepIndex,
		StepType:      row.StepType,
		TargetAgentID: row.TargetAgentID,
		Prompt:        row.Prompt,
		Status:        row.Status,
		TaskID:        row.TaskID,
		TaskRunID:     row.TaskRunID,
		OutputSummary: row.OutputSummary,
		ErrorMessage:  row.ErrorMessage,
		CreatedAt:     row.CreatedAt,
		StartedAt:     row.StartedAt,
		EndedAt:       row.EndedAt,
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
	workflow := &model.Workflow{
		WorkflowID:  util.NewPrefixedID(util.PrefixWorkflow),
		TeamID:      teamID,
		Name:        name,
		Description: description,
		Definition:  definition,
		Status:      model.WorkflowStatusDraft,
		CreatedBy:   createdBy,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.db.WithContext(ctx).Create(toWorkflowRow(workflow)).Error; err != nil {
		return nil, err
	}
	return workflow, nil
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
	updates := map[string]interface{}{
		"updated_at": time.Now().Unix(),
	}
	if in.Name != nil {
		updates["name"] = *in.Name
	}
	if in.Description != nil {
		updates["description"] = *in.Description
	}
	if in.Definition != nil {
		updates["definition"] = *in.Definition
	}
	if in.Status != nil {
		updates["status"] = *in.Status
	}
	if err := s.db.WithContext(ctx).Model(&workflowRow{}).Where("workflow_id = ?", workflowID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetWorkflow(ctx, workflowID)
}

func (s *Store) CreateWorkflowRun(ctx context.Context, in model.CreateWorkflowRunInput) (*model.WorkflowRun, error) {
	now := time.Now().Unix()
	run := &model.WorkflowRun{
		WorkflowRunID:  util.NewPrefixedID(util.PrefixWorkflowRun),
		WorkflowID:     in.WorkflowID,
		IssueID:        in.IssueID,
		ConversationID: in.ConversationID,
		Status:         in.Status,
		CreatedBy:      in.CreatedBy,
		CreatedAt:      now,
		StartedAt:      in.StartedAt,
	}
	if err := s.db.WithContext(ctx).Create(toWorkflowRunRow(run)).Error; err != nil {
		return nil, err
	}
	return run, nil
}

func (s *Store) ListWorkflowRunsByWorkflow(ctx context.Context, workflowID string, limit, offset int) ([]model.WorkflowRun, int, error) {
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
		rows[i] = workflowStepRunRow{
			StepRunID:     util.NewPrefixedID(util.PrefixWorkflowStepRun),
			WorkflowRunID: workflowRunID,
			StepID:        steps[i].StepID,
			StepIndex:     steps[i].StepIndex,
			StepType:      steps[i].StepType,
			TargetAgentID: steps[i].TargetAgentID,
			Prompt:        steps[i].Prompt,
			Status:        steps[i].Status,
			CreatedAt:     now,
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
