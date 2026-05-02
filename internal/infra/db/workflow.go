package db

import (
	"buildmax/internal/core/model"
	"context"
	"errors"
	"time"

	"buildmax/internal/util"
	"gorm.io/gorm"
)

func (s *Store) ListWorkflowsByTeam(ctx context.Context, teamID string) ([]model.Workflow, error) {
	var list []workflowRow
	err := s.db.WithContext(ctx).Where("team_id = ?", teamID).Order("created_at ASC").Find(&list).Error
	return toModelWorkflows(list), err
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
	if err := s.db.WithContext(ctx).Create(fromModelWorkflow(workflow)).Error; err != nil {
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
	return toModelWorkflow(&workflow), nil
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
	if err := s.db.WithContext(ctx).Create(fromModelWorkflowRun(run)).Error; err != nil {
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
	return toModelWorkflowRuns(list), int(total), nil
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
	return toModelWorkflowRuns(list), int(total), nil
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
	return toModelWorkflowRun(&run), nil
}

func (s *Store) ListWorkflowStepRuns(ctx context.Context, workflowRunID string) ([]model.WorkflowStepRun, error) {
	var list []workflowStepRunRow
	err := s.db.WithContext(ctx).
		Where("workflow_run_id = ?", workflowRunID).
		Order("step_index ASC, created_at ASC").
		Find(&list).Error
	return toModelWorkflowStepRuns(list), err
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
	return toModelWorkflowStepRuns(rows), nil
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
	return toModelWorkflowStepRun(&step), nil
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
	return toModelWorkflowStepRun(&step), nil
}
