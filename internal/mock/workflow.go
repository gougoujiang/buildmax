package mock

import (
	"context"
	"fmt"
	"time"

	"buildmax/internal/core/model"
)

// MockWorkflowStore is an in-memory WorkflowStore for tests.
type MockWorkflowStore struct {
	Workflows []model.Workflow
	Runs      []model.WorkflowRun
	StepRuns  []model.WorkflowStepRun
}

func (m *MockWorkflowStore) ListWorkflowsByTeam(_ context.Context, teamID string) ([]model.Workflow, error) {
	var out []model.Workflow
	for _, workflow := range m.Workflows {
		if workflow.TeamID == teamID {
			out = append(out, workflow)
		}
	}
	return out, nil
}

func (m *MockWorkflowStore) CreateWorkflow(_ context.Context, teamID, createdBy, name, description, definition string) (*model.Workflow, error) {
	workflow := model.Workflow{
		WorkflowID:  fmt.Sprintf("w_mock_%d", len(m.Workflows)+1),
		TeamID:      teamID,
		Name:        name,
		Description: description,
		Definition:  definition,
		Status:      model.WorkflowStatusDraft,
		CreatedBy:   createdBy,
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	}
	m.Workflows = append(m.Workflows, workflow)
	return &m.Workflows[len(m.Workflows)-1], nil
}

func (m *MockWorkflowStore) GetWorkflow(_ context.Context, workflowID string) (*model.Workflow, error) {
	for i := range m.Workflows {
		if m.Workflows[i].WorkflowID == workflowID {
			return &m.Workflows[i], nil
		}
	}
	return nil, nil
}

func (m *MockWorkflowStore) UpdateWorkflow(_ context.Context, workflowID, teamID string, in model.UpdateWorkflowInput) (*model.Workflow, error) {
	for i := range m.Workflows {
		if m.Workflows[i].WorkflowID != workflowID || m.Workflows[i].TeamID != teamID {
			continue
		}
		if in.Name != nil {
			m.Workflows[i].Name = *in.Name
		}
		if in.Description != nil {
			m.Workflows[i].Description = *in.Description
		}
		if in.Definition != nil {
			m.Workflows[i].Definition = *in.Definition
		}
		if in.Status != nil {
			m.Workflows[i].Status = *in.Status
		}
		m.Workflows[i].UpdatedAt = time.Now().Unix()
		return &m.Workflows[i], nil
	}
	return nil, nil
}

func (m *MockWorkflowStore) CreateWorkflowRun(_ context.Context, in model.CreateWorkflowRunInput) (*model.WorkflowRun, error) {
	run := model.WorkflowRun{
		WorkflowRunID:  fmt.Sprintf("wr_mock_%d", len(m.Runs)+1),
		WorkflowID:     in.WorkflowID,
		IssueID:        in.IssueID,
		ConversationID: in.ConversationID,
		Status:         in.Status,
		CreatedBy:      in.CreatedBy,
		CreatedAt:      time.Now().Unix(),
		StartedAt:      in.StartedAt,
	}
	m.Runs = append(m.Runs, run)
	return &m.Runs[len(m.Runs)-1], nil
}

func (m *MockWorkflowStore) ListWorkflowRunsByWorkflow(_ context.Context, workflowID string, limit, offset int) ([]model.WorkflowRun, int, error) {
	var out []model.WorkflowRun
	for _, run := range m.Runs {
		if run.WorkflowID == workflowID {
			out = append(out, run)
		}
	}
	total := len(out)
	if offset > total {
		return []model.WorkflowRun{}, total, nil
	}
	if limit <= 0 || offset+limit > total {
		limit = total - offset
	}
	return out[offset : offset+limit], total, nil
}

func (m *MockWorkflowStore) ListWorkflowRunsByIssue(_ context.Context, issueID string, limit, offset int) ([]model.WorkflowRun, int, error) {
	var out []model.WorkflowRun
	for _, run := range m.Runs {
		if run.IssueID != nil && *run.IssueID == issueID {
			out = append(out, run)
		}
	}
	total := len(out)
	if offset > total {
		return []model.WorkflowRun{}, total, nil
	}
	if limit <= 0 || offset+limit > total {
		limit = total - offset
	}
	return out[offset : offset+limit], total, nil
}

func (m *MockWorkflowStore) GetWorkflowRun(_ context.Context, workflowRunID string) (*model.WorkflowRun, error) {
	for i := range m.Runs {
		if m.Runs[i].WorkflowRunID == workflowRunID {
			return &m.Runs[i], nil
		}
	}
	return nil, nil
}

func (m *MockWorkflowStore) ListWorkflowStepRuns(_ context.Context, workflowRunID string) ([]model.WorkflowStepRun, error) {
	var out []model.WorkflowStepRun
	for _, step := range m.StepRuns {
		if step.WorkflowRunID == workflowRunID {
			out = append(out, step)
		}
	}
	return out, nil
}

func (m *MockWorkflowStore) CreateWorkflowStepRuns(_ context.Context, workflowRunID string, steps []model.CreateWorkflowStepRunInput) ([]model.WorkflowStepRun, error) {
	out := make([]model.WorkflowStepRun, len(steps))
	for i := range steps {
		out[i] = model.WorkflowStepRun{
			StepRunID:     fmt.Sprintf("wsr_mock_%d", len(m.StepRuns)+1),
			WorkflowRunID: workflowRunID,
			StepID:        steps[i].StepID,
			StepIndex:     steps[i].StepIndex,
			StepType:      steps[i].StepType,
			TargetAgentID: steps[i].TargetAgentID,
			Prompt:        steps[i].Prompt,
			Status:        steps[i].Status,
			CreatedAt:     time.Now().Unix(),
		}
		m.StepRuns = append(m.StepRuns, out[i])
	}
	return out, nil
}

func (m *MockWorkflowStore) UpdateWorkflowRun(_ context.Context, workflowRunID string, in model.UpdateWorkflowRunInput) (*model.WorkflowRun, error) {
	for i := range m.Runs {
		if m.Runs[i].WorkflowRunID != workflowRunID {
			continue
		}
		m.Runs[i].Status = in.Status
		if in.StartedAt != nil {
			m.Runs[i].StartedAt = in.StartedAt
		}
		if in.EndedAt != nil {
			m.Runs[i].EndedAt = in.EndedAt
		}
		if in.ErrorMessage != nil {
			m.Runs[i].ErrorMessage = in.ErrorMessage
		}
		return &m.Runs[i], nil
	}
	return nil, nil
}

func (m *MockWorkflowStore) UpdateWorkflowStepRun(_ context.Context, stepRunID string, in model.UpdateWorkflowStepRunInput) (*model.WorkflowStepRun, error) {
	for i := range m.StepRuns {
		if m.StepRuns[i].StepRunID != stepRunID {
			continue
		}
		if in.Status != nil {
			m.StepRuns[i].Status = *in.Status
		}
		if in.TaskID != nil {
			if *in.TaskID == "" {
				m.StepRuns[i].TaskID = nil
			} else {
				m.StepRuns[i].TaskID = in.TaskID
			}
		}
		if in.TaskRunID != nil {
			if *in.TaskRunID == "" {
				m.StepRuns[i].TaskRunID = nil
			} else {
				m.StepRuns[i].TaskRunID = in.TaskRunID
			}
		}
		if in.OutputSummary != nil {
			if *in.OutputSummary == "" {
				m.StepRuns[i].OutputSummary = nil
			} else {
				m.StepRuns[i].OutputSummary = in.OutputSummary
			}
		}
		if in.ErrorMessage != nil {
			if *in.ErrorMessage == "" {
				m.StepRuns[i].ErrorMessage = nil
			} else {
				m.StepRuns[i].ErrorMessage = in.ErrorMessage
			}
		}
		if in.StartedAt != nil {
			m.StepRuns[i].StartedAt = in.StartedAt
		}
		if in.EndedAt != nil {
			m.StepRuns[i].EndedAt = in.EndedAt
		}
		return &m.StepRuns[i], nil
	}
	return nil, nil
}

func (m *MockWorkflowStore) GetWorkflowStepRunByTaskID(_ context.Context, taskID string) (*model.WorkflowStepRun, error) {
	for i := range m.StepRuns {
		if m.StepRuns[i].TaskID != nil && *m.StepRuns[i].TaskID == taskID {
			return &m.StepRuns[i], nil
		}
	}
	return nil, nil
}

func (m *MockWorkflowStore) GetWorkflowStepRunByTaskRunID(_ context.Context, taskRunID string) (*model.WorkflowStepRun, error) {
	for i := range m.StepRuns {
		if m.StepRuns[i].TaskRunID != nil && *m.StepRuns[i].TaskRunID == taskRunID {
			return &m.StepRuns[i], nil
		}
	}
	return nil, nil
}
