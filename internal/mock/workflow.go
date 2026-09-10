package mock

import (
	"context"
	"fmt"
	"time"

	coreworkflow "github.com/gougoujiang/buildmax/internal/core/workflow"
)

// MockWorkflowStore is an in-memory WorkflowStore for tests. It records
// revisions the way the database store does, so a test can assert on history.
type MockWorkflowStore struct {
	Workflows []coreworkflow.Workflow
	Revisions []coreworkflow.Revision
	Runs      []coreworkflow.Run
	StepRuns  []coreworkflow.StepRun
}

func (m *MockWorkflowStore) appendRevision(w *coreworkflow.Workflow, createdBy string) {
	m.Revisions = append(m.Revisions, coreworkflow.Revision{
		WorkflowID:  w.ID,
		Revision:    w.Revision,
		Name:        w.Name,
		Description: w.Description,
		Definition:  w.Definition,
		Status:      w.Status,
		CreatedBy:   createdBy,
		CreatedAt:   time.Now().UTC(),
	})
}

func (m *MockWorkflowStore) ListWorkflowsBySpace(_ context.Context, spaceID string) ([]coreworkflow.Workflow, error) {
	var out []coreworkflow.Workflow
	for _, workflow := range m.Workflows {
		if workflow.SpaceID == spaceID {
			out = append(out, workflow)
		}
	}
	return out, nil
}

func (m *MockWorkflowStore) CreateWorkflow(_ context.Context, spaceID, createdBy, name, description, definition string) (*coreworkflow.Workflow, error) {
	workflow := coreworkflow.Workflow{
		ID:          fmt.Sprintf("w_mock_%d", len(m.Workflows)+1),
		SpaceID:     spaceID,
		Name:        name,
		Description: description,
		Definition:  definition,
		Status:      coreworkflow.StatusDraft,
		Revision:    1,
		CreatedBy:   createdBy,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	m.Workflows = append(m.Workflows, workflow)
	created := &m.Workflows[len(m.Workflows)-1]
	m.appendRevision(created, createdBy)
	return created, nil
}

func (m *MockWorkflowStore) GetWorkflow(_ context.Context, workflowID string) (*coreworkflow.Workflow, error) {
	for i := range m.Workflows {
		if m.Workflows[i].ID == workflowID {
			return &m.Workflows[i], nil
		}
	}
	return nil, nil
}

func (m *MockWorkflowStore) UpdateWorkflow(_ context.Context, workflowID, spaceID string, in coreworkflow.UpdateInput) (*coreworkflow.Workflow, error) {
	for i := range m.Workflows {
		if m.Workflows[i].ID != workflowID || m.Workflows[i].SpaceID != spaceID {
			continue
		}
		updated := m.Workflows[i]
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
		if updated.Name == m.Workflows[i].Name && updated.Description == m.Workflows[i].Description &&
			updated.Definition == m.Workflows[i].Definition && updated.Status == m.Workflows[i].Status {
			return &m.Workflows[i], nil
		}
		if updated.Revision < 1 {
			updated.Revision = 1
		}
		updated.Revision++
		updated.UpdatedAt = time.Now().UTC()
		m.Workflows[i] = updated
		m.appendRevision(&m.Workflows[i], in.UpdatedBy)
		return &m.Workflows[i], nil
	}
	return nil, nil
}

func (m *MockWorkflowStore) ListWorkflowRevisions(_ context.Context, workflowID string, limit, offset int) ([]coreworkflow.Revision, int, error) {
	var all []coreworkflow.Revision
	for i := len(m.Revisions) - 1; i >= 0; i-- {
		if m.Revisions[i].WorkflowID == workflowID {
			all = append(all, m.Revisions[i])
		}
	}
	return pageRevisions(all, limit, offset), len(all), nil
}

func (m *MockWorkflowStore) GetWorkflowRevision(_ context.Context, workflowID string, revision int) (*coreworkflow.Revision, error) {
	for i := range m.Revisions {
		if m.Revisions[i].WorkflowID == workflowID && m.Revisions[i].Revision == revision {
			return &m.Revisions[i], nil
		}
	}
	return nil, nil
}

func (m *MockWorkflowStore) CreateWorkflowRun(_ context.Context, in coreworkflow.CreateRunInput) (*coreworkflow.Run, error) {
	run := coreworkflow.Run{
		ID:               fmt.Sprintf("wr_mock_%d", len(m.Runs)+1),
		WorkflowID:       in.WorkflowID,
		WorkflowRevision: in.WorkflowRevision,
		IssueID:          in.IssueID,
		Status:           in.Status,
		CreatedBy:        in.CreatedBy,
		CreatedAt:        time.Now().UTC(),
		StartedAt:        in.StartedAt,
	}
	m.Runs = append(m.Runs, run)
	return &m.Runs[len(m.Runs)-1], nil
}

func (m *MockWorkflowStore) ListWorkflowRunsByWorkflow(_ context.Context, workflowID string, limit, offset int) ([]coreworkflow.Run, int, error) {
	var out []coreworkflow.Run
	for _, run := range m.Runs {
		if run.WorkflowID == workflowID {
			out = append(out, run)
		}
	}
	total := len(out)
	if offset > total {
		return []coreworkflow.Run{}, total, nil
	}
	if limit <= 0 || offset+limit > total {
		limit = total - offset
	}
	return out[offset : offset+limit], total, nil
}

func (m *MockWorkflowStore) ListWorkflowRunsByIssue(_ context.Context, issueID string, limit, offset int) ([]coreworkflow.Run, int, error) {
	var out []coreworkflow.Run
	for _, run := range m.Runs {
		if run.IssueID != nil && *run.IssueID == issueID {
			out = append(out, run)
		}
	}
	total := len(out)
	if offset > total {
		return []coreworkflow.Run{}, total, nil
	}
	if limit <= 0 || offset+limit > total {
		limit = total - offset
	}
	return out[offset : offset+limit], total, nil
}

func (m *MockWorkflowStore) GetWorkflowRun(_ context.Context, workflowRunID string) (*coreworkflow.Run, error) {
	for i := range m.Runs {
		if m.Runs[i].ID == workflowRunID {
			return &m.Runs[i], nil
		}
	}
	return nil, nil
}

func (m *MockWorkflowStore) ListWorkflowStepRuns(_ context.Context, workflowRunID string) ([]coreworkflow.StepRun, error) {
	var out []coreworkflow.StepRun
	for _, step := range m.StepRuns {
		if step.WorkflowRunID == workflowRunID {
			out = append(out, step)
		}
	}
	return out, nil
}

func (m *MockWorkflowStore) CreateWorkflowStepRuns(_ context.Context, workflowRunID string, steps []coreworkflow.CreateStepRunInput) ([]coreworkflow.StepRun, error) {
	out := make([]coreworkflow.StepRun, len(steps))
	for i := range steps {
		out[i] = coreworkflow.StepRun{
			ID:                fmt.Sprintf("wsr_mock_%d", len(m.StepRuns)+1),
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
			CreatedAt:         time.Now().UTC(),
		}
		m.StepRuns = append(m.StepRuns, out[i])
	}
	return out, nil
}

func (m *MockWorkflowStore) TransitionWorkflowRun(_ context.Context, in coreworkflow.TransitionRunInput) (bool, error) {
	if !coreworkflow.ValidRunStatusTransition(in.ExpectedStatus, in.NewStatus) {
		return false, fmt.Errorf("%w: %s -> %s", coreworkflow.ErrInvalidRunTransition, in.ExpectedStatus, in.NewStatus)
	}
	for i := range m.Runs {
		if m.Runs[i].ID != in.WorkflowRunID {
			continue
		}
		if m.Runs[i].Status != string(in.ExpectedStatus) {
			return false, nil
		}
		m.Runs[i].Status = string(in.NewStatus)
		if in.StartedAt != nil {
			m.Runs[i].StartedAt = in.StartedAt
		}
		if in.EndedAt != nil {
			m.Runs[i].EndedAt = in.EndedAt
		}
		if in.ErrorMessage != nil {
			m.Runs[i].ErrorMessage = in.ErrorMessage
		}
		return true, nil
	}
	return false, nil
}

func (m *MockWorkflowStore) TransitionWorkflowStepRun(_ context.Context, in coreworkflow.TransitionStepRunInput) (bool, error) {
	if !coreworkflow.ValidStepRunTransition(in.ExpectedStatus, in.NewStatus) {
		return false, fmt.Errorf("%w: %s -> %s", coreworkflow.ErrInvalidStepRunTransition, in.ExpectedStatus, in.NewStatus)
	}
	for i := range m.StepRuns {
		if m.StepRuns[i].ID != in.StepRunID {
			continue
		}
		if m.StepRuns[i].Status != string(in.ExpectedStatus) {
			return false, nil
		}
		m.StepRuns[i].Status = string(in.NewStatus)
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
		return true, nil
	}
	return false, nil
}

func (m *MockWorkflowStore) FinalizeFailedWorkflowRun(_ context.Context, in coreworkflow.FinalizeFailedRunInput) (bool, error) {
	if !coreworkflow.ValidStepRunTransition(in.StepExpected, in.StepStatus) {
		return false, fmt.Errorf("%w: %s -> %s", coreworkflow.ErrInvalidStepRunTransition, in.StepExpected, in.StepStatus)
	}
	if !coreworkflow.ValidRunStatusTransition(in.RunExpected, in.RunStatus) {
		return false, fmt.Errorf("%w: %s -> %s", coreworkflow.ErrInvalidRunTransition, in.RunExpected, in.RunStatus)
	}
	stepApplied := false
	for i := range m.StepRuns {
		if m.StepRuns[i].ID != in.StepRunID {
			continue
		}
		if m.StepRuns[i].Status != string(in.StepExpected) {
			return false, nil
		}
		m.StepRuns[i].Status = string(in.StepStatus)
		if in.TaskRunID != nil && *in.TaskRunID != "" {
			m.StepRuns[i].TaskRunID = in.TaskRunID
		}
		if in.ErrorMessage != nil {
			m.StepRuns[i].ErrorMessage = in.ErrorMessage
		}
		if in.StartedAt != nil {
			m.StepRuns[i].StartedAt = in.StartedAt
		}
		if in.EndedAt != nil {
			m.StepRuns[i].EndedAt = in.EndedAt
		}
		stepApplied = true
		break
	}
	if !stepApplied {
		return false, nil
	}
	for i := range m.StepRuns {
		if m.StepRuns[i].WorkflowRunID == in.WorkflowRunID &&
			m.StepRuns[i].StepIndex > in.StepIndex &&
			m.StepRuns[i].Status == string(coreworkflow.StepRunStatusPending) {
			m.StepRuns[i].Status = string(coreworkflow.StepRunStatusBlocked)
		}
	}
	for i := range m.Runs {
		if m.Runs[i].ID != in.WorkflowRunID {
			continue
		}
		if m.Runs[i].Status == string(in.RunExpected) {
			m.Runs[i].Status = string(in.RunStatus)
			if in.EndedAt != nil {
				m.Runs[i].EndedAt = in.EndedAt
			}
			if in.ErrorMessage != nil {
				m.Runs[i].ErrorMessage = in.ErrorMessage
			}
		}
		break
	}
	return true, nil
}

func (m *MockWorkflowStore) GetWorkflowStepRunByTaskID(_ context.Context, taskID string) (*coreworkflow.StepRun, error) {
	for i := range m.StepRuns {
		if m.StepRuns[i].TaskID != nil && *m.StepRuns[i].TaskID == taskID {
			return &m.StepRuns[i], nil
		}
	}
	return nil, nil
}

func (m *MockWorkflowStore) GetWorkflowStepRunByTaskRunID(_ context.Context, taskRunID string) (*coreworkflow.StepRun, error) {
	for i := range m.StepRuns {
		if m.StepRuns[i].TaskRunID != nil && *m.StepRuns[i].TaskRunID == taskRunID {
			return &m.StepRuns[i], nil
		}
	}
	return nil, nil
}
