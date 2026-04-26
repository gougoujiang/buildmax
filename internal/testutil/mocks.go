package testutil

import (
	"context"
	"fmt"
	"io"
	"time"

	"buildmax/internal/storage/blob"
	"buildmax/internal/storage/entity"

	"gorm.io/gorm"
)

// MockIssueStore is an in-memory IssueStore for tests.
type MockIssueStore struct {
	Issues []entity.Issue
}

func (m *MockIssueStore) CreateIssue(_ context.Context, userID string, in entity.CreateIssueInput) (*entity.Issue, error) {
	return m.CreateIssueInTeam(context.Background(), "tm_personal", userID, in)
}

func (m *MockIssueStore) CreateIssueInTeam(_ context.Context, teamID, createdBy string, in entity.CreateIssueInput) (*entity.Issue, error) {
	issue := entity.Issue{
		IssueID:      fmt.Sprintf("i_mock_%d", len(m.Issues)+1),
		UserID:       createdBy,
		TeamID:       teamID,
		Title:        in.Title,
		Description:  in.Description,
		Status:       entity.IssueStatusTodo,
		CreatedBy:    createdBy,
		CreatedAt:    time.Now().Unix(),
		UpdatedAt:    time.Now().Unix(),
		AssigneeKind: nil,
		AssigneeID:   nil,
	}
	m.Issues = append(m.Issues, issue)
	return &m.Issues[len(m.Issues)-1], nil
}

func (m *MockIssueStore) ListIssuesByUser(_ context.Context, userID string, limit, offset int) ([]entity.Issue, int, error) {
	var filtered []entity.Issue
	for _, issue := range m.Issues {
		if issue.UserID == userID {
			filtered = append(filtered, issue)
		}
	}
	total := len(filtered)
	if offset > len(filtered) {
		return []entity.Issue{}, total, nil
	}
	end := offset + limit
	if limit <= 0 || end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], total, nil
}

func (m *MockIssueStore) ListIssuesByTeam(_ context.Context, teamID string, limit, offset int) ([]entity.Issue, int, error) {
	var filtered []entity.Issue
	for _, issue := range m.Issues {
		if issue.TeamID == teamID {
			filtered = append(filtered, issue)
		}
	}
	total := len(filtered)
	if offset > len(filtered) {
		return []entity.Issue{}, total, nil
	}
	end := offset + limit
	if limit <= 0 || end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], total, nil
}

func (m *MockIssueStore) GetIssue(_ context.Context, issueID string) (*entity.Issue, error) {
	for i := range m.Issues {
		if m.Issues[i].IssueID == issueID {
			return &m.Issues[i], nil
		}
	}
	return nil, nil
}

func (m *MockIssueStore) UpdateIssue(_ context.Context, issueID, userID string, in entity.UpdateIssueInput) (*entity.Issue, error) {
	for i := range m.Issues {
		if m.Issues[i].IssueID != issueID || m.Issues[i].UserID != userID {
			continue
		}
		return m.applyIssueUpdate(i, in), nil
	}
	return nil, nil
}

func (m *MockIssueStore) UpdateIssueInTeam(_ context.Context, issueID, teamID string, in entity.UpdateIssueInput) (*entity.Issue, error) {
	for i := range m.Issues {
		if m.Issues[i].IssueID != issueID || m.Issues[i].TeamID != teamID {
			continue
		}
		return m.applyIssueUpdate(i, in), nil
	}
	return nil, nil
}

func (m *MockIssueStore) applyIssueUpdate(i int, in entity.UpdateIssueInput) *entity.Issue {
	if in.Title != nil {
		m.Issues[i].Title = *in.Title
	}
	if in.Description != nil {
		m.Issues[i].Description = *in.Description
	}
	if in.Status != nil {
		m.Issues[i].Status = *in.Status
	}
	if in.AssigneeKind != nil {
		if *in.AssigneeKind == "" {
			m.Issues[i].AssigneeKind = nil
		} else {
			m.Issues[i].AssigneeKind = in.AssigneeKind
		}
	}
	if in.AssigneeID != nil {
		if *in.AssigneeID == "" {
			m.Issues[i].AssigneeID = nil
		} else {
			m.Issues[i].AssigneeID = in.AssigneeID
		}
	}
	m.Issues[i].UpdatedAt = time.Now().Unix()
	return &m.Issues[i]
}

// MockWorkflowStore is an in-memory WorkflowStore for tests.
type MockWorkflowStore struct {
	Workflows []entity.Workflow
	Runs      []entity.WorkflowRun
	StepRuns  []entity.WorkflowStepRun
}

func (m *MockWorkflowStore) ListWorkflowsByTeam(_ context.Context, teamID string) ([]entity.Workflow, error) {
	var out []entity.Workflow
	for _, workflow := range m.Workflows {
		if workflow.TeamID == teamID {
			out = append(out, workflow)
		}
	}
	return out, nil
}

func (m *MockWorkflowStore) CreateWorkflow(_ context.Context, teamID, createdBy, name, description, definition string) (*entity.Workflow, error) {
	workflow := entity.Workflow{
		WorkflowID:  fmt.Sprintf("w_mock_%d", len(m.Workflows)+1),
		TeamID:      teamID,
		Name:        name,
		Description: description,
		Definition:  definition,
		CreatedBy:   createdBy,
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	}
	m.Workflows = append(m.Workflows, workflow)
	return &m.Workflows[len(m.Workflows)-1], nil
}

func (m *MockWorkflowStore) GetWorkflow(_ context.Context, workflowID string) (*entity.Workflow, error) {
	for i := range m.Workflows {
		if m.Workflows[i].WorkflowID == workflowID {
			return &m.Workflows[i], nil
		}
	}
	return nil, nil
}

func (m *MockWorkflowStore) UpdateWorkflow(_ context.Context, workflowID, teamID string, in entity.UpdateWorkflowInput) (*entity.Workflow, error) {
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
		m.Workflows[i].UpdatedAt = time.Now().Unix()
		return &m.Workflows[i], nil
	}
	return nil, nil
}

func (m *MockWorkflowStore) CreateWorkflowRun(_ context.Context, in entity.CreateWorkflowRunInput) (*entity.WorkflowRun, error) {
	run := entity.WorkflowRun{
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

func (m *MockWorkflowStore) ListWorkflowRunsByWorkflow(_ context.Context, workflowID string, limit, offset int) ([]entity.WorkflowRun, int, error) {
	var out []entity.WorkflowRun
	for _, run := range m.Runs {
		if run.WorkflowID == workflowID {
			out = append(out, run)
		}
	}
	total := len(out)
	if offset > total {
		return []entity.WorkflowRun{}, total, nil
	}
	if limit <= 0 || offset+limit > total {
		limit = total - offset
	}
	return out[offset : offset+limit], total, nil
}

func (m *MockWorkflowStore) ListWorkflowRunsByIssue(_ context.Context, issueID string, limit, offset int) ([]entity.WorkflowRun, int, error) {
	var out []entity.WorkflowRun
	for _, run := range m.Runs {
		if run.IssueID != nil && *run.IssueID == issueID {
			out = append(out, run)
		}
	}
	total := len(out)
	if offset > total {
		return []entity.WorkflowRun{}, total, nil
	}
	if limit <= 0 || offset+limit > total {
		limit = total - offset
	}
	return out[offset : offset+limit], total, nil
}

func (m *MockWorkflowStore) GetWorkflowRun(_ context.Context, workflowRunID string) (*entity.WorkflowRun, error) {
	for i := range m.Runs {
		if m.Runs[i].WorkflowRunID == workflowRunID {
			return &m.Runs[i], nil
		}
	}
	return nil, nil
}

func (m *MockWorkflowStore) ListWorkflowStepRuns(_ context.Context, workflowRunID string) ([]entity.WorkflowStepRun, error) {
	var out []entity.WorkflowStepRun
	for _, step := range m.StepRuns {
		if step.WorkflowRunID == workflowRunID {
			out = append(out, step)
		}
	}
	return out, nil
}

func (m *MockWorkflowStore) CreateWorkflowStepRuns(_ context.Context, workflowRunID string, steps []entity.CreateWorkflowStepRunInput) ([]entity.WorkflowStepRun, error) {
	out := make([]entity.WorkflowStepRun, len(steps))
	for i := range steps {
		out[i] = entity.WorkflowStepRun{
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

func (m *MockWorkflowStore) UpdateWorkflowRun(_ context.Context, workflowRunID string, in entity.UpdateWorkflowRunInput) (*entity.WorkflowRun, error) {
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

func (m *MockWorkflowStore) UpdateWorkflowStepRun(_ context.Context, stepRunID string, in entity.UpdateWorkflowStepRunInput) (*entity.WorkflowStepRun, error) {
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

func (m *MockWorkflowStore) GetWorkflowStepRunByTaskID(_ context.Context, taskID string) (*entity.WorkflowStepRun, error) {
	for i := range m.StepRuns {
		if m.StepRuns[i].TaskID != nil && *m.StepRuns[i].TaskID == taskID {
			return &m.StepRuns[i], nil
		}
	}
	return nil, nil
}

func (m *MockWorkflowStore) GetWorkflowStepRunByTaskRunID(_ context.Context, taskRunID string) (*entity.WorkflowStepRun, error) {
	for i := range m.StepRuns {
		if m.StepRuns[i].TaskRunID != nil && *m.StepRuns[i].TaskRunID == taskRunID {
			return &m.StepRuns[i], nil
		}
	}
	return nil, nil
}

// MockTaskStore is an in-memory TaskStore for tests.
type MockTaskStore struct {
	List      []entity.Task
	ListErr   error
	Create    *entity.Task
	CreateErr error
}

func (m *MockTaskStore) ListTasksByConversation(_ context.Context, conversationID string, order string) ([]entity.Task, error) {
	list, _, err := m.ListTasksByConversationPaginated(context.Background(), conversationID, false, 0, 0)
	if err != nil {
		return nil, err
	}
	if order == "asc" {
		for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
			list[i], list[j] = list[j], list[i]
		}
	}
	return list, nil
}

func (m *MockTaskStore) ListTasksByConversationPaginated(_ context.Context, conversationID string, executedOnly bool, limit, offset int) ([]entity.Task, int, error) {
	if m.ListErr != nil {
		return nil, 0, m.ListErr
	}
	var filtered []entity.Task
	for _, c := range m.List {
		if c.ConversationID != conversationID {
			continue
		}
		if executedOnly && (c.LastRunID == nil || *c.LastRunID == "") {
			continue
		}
		filtered = append(filtered, c)
	}
	for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	}
	total := len(filtered)
	if offset > len(filtered) {
		return []entity.Task{}, total, nil
	}
	end := offset + limit
	if limit <= 0 {
		end = len(filtered)
	}
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], total, nil
}

func (m *MockTaskStore) ListTasksByIssue(_ context.Context, issueID string, limit, offset int) ([]entity.Task, int, error) {
	if m.ListErr != nil {
		return nil, 0, m.ListErr
	}
	var filtered []entity.Task
	for _, task := range m.List {
		if task.IssueID != nil && *task.IssueID == issueID {
			filtered = append(filtered, task)
		}
	}
	for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	}
	total := len(filtered)
	if offset > len(filtered) {
		return []entity.Task{}, total, nil
	}
	end := offset + limit
	if limit <= 0 || end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], total, nil
}

func (m *MockTaskStore) CreateTask(_ context.Context, in *entity.CreateTaskInput) (*entity.Task, error) {
	if m.CreateErr != nil {
		return nil, m.CreateErr
	}
	if m.Create != nil {
		return m.Create, nil
	}
	if in == nil {
		return nil, nil
	}
	id := len(m.List) + 1
	taskID := fmt.Sprintf("t_mock_%d", id)
	lastRunID := fmt.Sprintf("r_mock_%d", id)
	task := &entity.Task{
		TaskID:         taskID,
		ConversationID: in.ConversationID,
		Status:         "PENDING",
		Input:          in.Input,
		Title:          in.Title,
		CreatedBy:      in.CreatedBy,
		CreatedAt:      12345,
		AgentID:        in.AgentID,
		IssueID:        in.IssueID,
	}
	task.LastRunID = &lastRunID
	m.List = append(m.List, *task)
	return task, nil
}

func (m *MockTaskStore) GetTaskBySessionID(_ context.Context, sessionID string) (*entity.Task, error) {
	for i := range m.List {
		if m.List[i].SessionID != nil && *m.List[i].SessionID == sessionID {
			return &m.List[i], nil
		}
	}
	return nil, nil
}

func (m *MockTaskStore) UpdateTask(_ context.Context, in entity.UpdateTaskInput) error {
	for i := range m.List {
		if m.List[i].TaskID == in.TaskID {
			m.List[i].Status = in.Status
			if in.StartedAt != nil {
				m.List[i].StartedAt = in.StartedAt
			}
			if in.EndedAt != nil {
				m.List[i].EndedAt = in.EndedAt
			}
			if in.Output != nil {
				m.List[i].Output = in.Output
			}
			if in.ErrorMessage != nil {
				m.List[i].ErrorMessage = in.ErrorMessage
			}
			if in.SessionID != nil {
				m.List[i].SessionID = in.SessionID
			}
			return nil
		}
	}
	return nil
}

func (m *MockTaskStore) ClaimTask(_ context.Context, in entity.ClaimTaskInput) (bool, error) {
	for i := range m.List {
		if m.List[i].TaskID == in.TaskID && m.List[i].Status == in.ExpectedStatus {
			m.List[i].Status = in.NewStatus
			if in.StartedAt != nil {
				m.List[i].StartedAt = in.StartedAt
			}
			if in.EndedAt != nil {
				m.List[i].EndedAt = in.EndedAt
			}
			if in.Output != nil {
				m.List[i].Output = in.Output
			}
			if in.ErrorMessage != nil {
				m.List[i].ErrorMessage = in.ErrorMessage
			}
			if in.SessionID != nil {
				m.List[i].SessionID = in.SessionID
			}
			return true, nil
		}
	}
	return false, nil
}

func (m *MockTaskStore) GetTask(_ context.Context, taskID string) (*entity.Task, error) {
	for i := range m.List {
		if m.List[i].TaskID == taskID {
			return &m.List[i], nil
		}
	}
	return nil, nil
}

// MockTaskRunStore is an in-memory TaskRunStore for tests.
type MockTaskRunStore struct {
	Runs     []entity.TaskRun
	TaskList []entity.Task
}

func (m *MockTaskRunStore) CreateTaskRun(_ context.Context, taskID, input, createdBy string) (*entity.TaskRun, error) {
	return nil, nil
}
func (m *MockTaskRunStore) GetNextPendingTaskRun(_ context.Context) (*entity.TaskRun, error) {
	return nil, nil
}
func (m *MockTaskRunStore) GetTaskRun(_ context.Context, taskRunID string) (*entity.TaskRun, error) {
	for i := range m.Runs {
		if m.Runs[i].TaskRunID == taskRunID {
			return &m.Runs[i], nil
		}
	}
	return nil, nil
}
func (m *MockTaskRunStore) GetTaskRunWithTask(_ context.Context, taskRunID string) (*entity.TaskRun, *entity.Task, error) {
	var run *entity.TaskRun
	for i := range m.Runs {
		if m.Runs[i].TaskRunID == taskRunID {
			run = &m.Runs[i]
			break
		}
	}
	if run == nil {
		return nil, nil, nil
	}
	var task *entity.Task
	for i := range m.TaskList {
		if m.TaskList[i].TaskID == run.TaskID {
			task = &m.TaskList[i]
			break
		}
	}
	return run, task, nil
}
func (m *MockTaskRunStore) ClaimTaskRun(ctx context.Context, in entity.ClaimTaskRunInput) (bool, error) {
	for i := range m.Runs {
		if m.Runs[i].TaskRunID == in.TaskRunID && m.Runs[i].Status == string(in.ExpectedStatus) {
			m.Runs[i].Status = string(in.NewStatus)
			if in.StartedAt != nil {
				m.Runs[i].StartedAt = in.StartedAt
			}
			if in.SessionID != nil {
				m.Runs[i].SessionID = in.SessionID
			}
			return true, nil
		}
	}
	return false, nil
}
func (m *MockTaskRunStore) UpdateRun(ctx context.Context, in entity.UpdateTaskRunInput) error {
	for i := range m.Runs {
		if m.Runs[i].TaskRunID == in.TaskRunID {
			m.Runs[i].Status = string(in.Status)
			if in.StartedAt != nil {
				m.Runs[i].StartedAt = in.StartedAt
			}
			if in.EndedAt != nil {
				m.Runs[i].EndedAt = in.EndedAt
			}
			if in.Output != nil {
				m.Runs[i].Output = in.Output
			}
			if in.ErrorMessage != nil {
				m.Runs[i].ErrorMessage = in.ErrorMessage
			}
			if in.SessionID != nil {
				m.Runs[i].SessionID = in.SessionID
			}
			if in.PromptTokens != nil {
				m.Runs[i].PromptTokens = in.PromptTokens
			}
			if in.CompletionTokens != nil {
				m.Runs[i].CompletionTokens = in.CompletionTokens
			}
			return nil
		}
	}
	return nil
}
func (m *MockTaskRunStore) UpdateTaskRunWorkerInfo(_ context.Context, taskRunID, workerType string, k8sJobName *string, k8sJobCreatedAt *int64) error {
	return nil
}
func (m *MockTaskRunStore) OnRunComplete(_ context.Context, taskRunID string, relativePaths []string) error {
	return nil
}
func (m *MockTaskRunStore) SyncTaskFromRun(_ context.Context, taskRunID string) error {
	return nil
}

// MockRunOutputLister is an in-memory RunOutputLister for tests.
type MockRunOutputLister struct {
	List        []entity.ArtifactWithTask
	ListErr     error
	OutputFiles map[string][]entity.TaskRunArtifact
}

func (m *MockRunOutputLister) ListRunOutputsByConversation(_ context.Context, conversationID string, taskID *string) ([]entity.ArtifactWithTask, error) {
	if m.ListErr != nil {
		return nil, m.ListErr
	}
	return m.List, nil
}

func (m *MockRunOutputLister) GetTaskRunOutputFiles(_ context.Context, chatRunID string) ([]entity.TaskRunArtifact, error) {
	if m.OutputFiles != nil {
		return m.OutputFiles[chatRunID], nil
	}
	return nil, nil
}

// MockAgentStore is an in-memory AgentStore for tests.
type MockAgentStore struct {
	Agents []entity.Agent
}

func (m *MockAgentStore) ListAgentsByUser(_ context.Context, userID string) ([]entity.Agent, error) {
	var out []entity.Agent
	for _, a := range m.Agents {
		if a.UserID == userID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *MockAgentStore) ListAgentsByTeam(_ context.Context, teamID string) ([]entity.Agent, error) {
	var out []entity.Agent
	for _, a := range m.Agents {
		if a.TeamID == teamID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *MockAgentStore) GetAgent(_ context.Context, agentID string) (*entity.Agent, error) {
	for i := range m.Agents {
		if m.Agents[i].AgentID == agentID {
			return &m.Agents[i], nil
		}
	}
	return nil, nil
}

func (m *MockAgentStore) CreateAgent(_ context.Context, userID, name, description, instructions string) (*entity.Agent, error) {
	return m.CreateAgentInTeam(context.Background(), "tm_personal", userID, name, description, instructions)
}

func (m *MockAgentStore) CreateAgentInTeam(_ context.Context, teamID, userID, name, description, instructions string) (*entity.Agent, error) {
	a := entity.Agent{
		AgentID:      fmt.Sprintf("a_%d", len(m.Agents)+1),
		UserID:       userID,
		TeamID:       teamID,
		Name:         name,
		Description:  description,
		Instructions: instructions,
		CreatedAt:    time.Now().Unix(),
	}
	m.Agents = append(m.Agents, a)
	return &m.Agents[len(m.Agents)-1], nil
}

func (m *MockAgentStore) UpdateAgent(_ context.Context, agentID, userID, name, description, instructions string) (*entity.Agent, error) {
	for i := range m.Agents {
		if m.Agents[i].AgentID == agentID && m.Agents[i].UserID == userID {
			m.Agents[i].Name = name
			m.Agents[i].Description = description
			m.Agents[i].Instructions = instructions
			return &m.Agents[i], nil
		}
	}
	return nil, nil
}

func (m *MockAgentStore) UpdateAgentInTeam(_ context.Context, agentID, teamID, name, description, instructions string) (*entity.Agent, error) {
	for i := range m.Agents {
		if m.Agents[i].AgentID == agentID && m.Agents[i].TeamID == teamID {
			m.Agents[i].Name = name
			m.Agents[i].Description = description
			m.Agents[i].Instructions = instructions
			return &m.Agents[i], nil
		}
	}
	return nil, nil
}

func (m *MockAgentStore) DeleteAgent(_ context.Context, agentID, userID string) error {
	for i := range m.Agents {
		if m.Agents[i].AgentID == agentID && m.Agents[i].UserID == userID {
			m.Agents = append(m.Agents[:i], m.Agents[i+1:]...)
			return nil
		}
	}
	return gorm.ErrRecordNotFound
}

func (m *MockAgentStore) DeleteAgentInTeam(_ context.Context, agentID, teamID string) error {
	for i := range m.Agents {
		if m.Agents[i].AgentID == agentID && m.Agents[i].TeamID == teamID {
			m.Agents = append(m.Agents[:i], m.Agents[i+1:]...)
			return nil
		}
	}
	return gorm.ErrRecordNotFound
}

// MockConversationStore is an in-memory ConversationStore for tests.
type MockConversationStore struct {
	Conversations []entity.Conversation
}

func (m *MockConversationStore) CreateConversation(_ context.Context, userID, channel, createdBy string) (*entity.Conversation, error) {
	return m.CreateConversationInTeam(context.Background(), "tm_personal", userID, channel, createdBy)
}

func (m *MockConversationStore) CreateConversationInTeam(_ context.Context, teamID, userID, channel, createdBy string) (*entity.Conversation, error) {
	conv := entity.Conversation{
		ConversationID: fmt.Sprintf("v_%d", len(m.Conversations)+1),
		UserID:         userID,
		TeamID:         teamID,
		Channel:        channel,
		CreatedBy:      createdBy,
		CreatedAt:      time.Now().Unix(),
	}
	m.Conversations = append(m.Conversations, conv)
	return &m.Conversations[len(m.Conversations)-1], nil
}

func (m *MockConversationStore) GetConversation(_ context.Context, conversationID string) (*entity.Conversation, error) {
	for i := range m.Conversations {
		if m.Conversations[i].ConversationID == conversationID {
			return &m.Conversations[i], nil
		}
	}
	return nil, nil
}

func (m *MockConversationStore) ListConversationsByUser(_ context.Context, userID string, limit, offset int) ([]entity.Conversation, int, error) {
	var out []entity.Conversation
	for _, conv := range m.Conversations {
		if conv.UserID == userID {
			out = append(out, conv)
		}
	}
	total := len(out)
	if offset > total {
		return []entity.Conversation{}, total, nil
	}
	if limit <= 0 || offset+limit > total {
		limit = total - offset
	}
	return out[offset : offset+limit], total, nil
}

func (m *MockConversationStore) ListConversationsByTeam(_ context.Context, teamID string, limit, offset int) ([]entity.Conversation, int, error) {
	var out []entity.Conversation
	for _, conv := range m.Conversations {
		if conv.TeamID == teamID {
			out = append(out, conv)
		}
	}
	total := len(out)
	if offset > total {
		return []entity.Conversation{}, total, nil
	}
	if limit <= 0 || offset+limit > total {
		limit = total - offset
	}
	return out[offset : offset+limit], total, nil
}

func (m *MockConversationStore) UpdateConversationTitle(_ context.Context, conversationID, title string) error {
	for i := range m.Conversations {
		if m.Conversations[i].ConversationID == conversationID {
			m.Conversations[i].Title = title
			return nil
		}
	}
	return nil
}

// MockArtifactStorage is an in-memory blob.ArtifactStorage for tests.
type MockArtifactStorage struct {
	Results map[string][]byte
	Files   map[string][]byte
}

// NewMockArtifactStorage returns a MockArtifactStorage ready for use.
func NewMockArtifactStorage() *MockArtifactStorage {
	return &MockArtifactStorage{Results: make(map[string][]byte)}
}

func (m *MockArtifactStorage) PutResult(_ context.Context, ref blob.RunRef, data []byte) error {
	m.Results[ref.UserID+"/"+ref.ConversationID+"/"+ref.TaskID+"/"+ref.TaskRunID] = append([]byte(nil), data...)
	return nil
}

func (m *MockArtifactStorage) GetResult(_ context.Context, ref blob.RunRef) ([]byte, error) {
	key := ref.UserID + "/" + ref.ConversationID + "/" + ref.TaskID + "/" + ref.TaskRunID
	if data, ok := m.Results[key]; ok {
		return data, nil
	}
	return nil, blob.ErrNotFound
}

func (m *MockArtifactStorage) PutArtifactFile(_ context.Context, ref blob.RunObjectRef, r io.Reader) error {
	if m.Files == nil {
		m.Files = make(map[string][]byte)
	}
	key := ref.UserID + "/" + ref.ConversationID + "/" + ref.TaskID + "/" + ref.TaskRunID + "/" + ref.RelPath
	data, _ := io.ReadAll(r)
	m.Files[key] = data
	return nil
}

func (m *MockArtifactStorage) GetArtifactFile(_ context.Context, ref blob.RunObjectRef) ([]byte, error) {
	if m.Files == nil {
		return nil, blob.ErrNotFound
	}
	key := ref.UserID + "/" + ref.ConversationID + "/" + ref.TaskID + "/" + ref.TaskRunID + "/" + ref.RelPath
	if data, ok := m.Files[key]; ok {
		return data, nil
	}
	return nil, blob.ErrNotFound
}
