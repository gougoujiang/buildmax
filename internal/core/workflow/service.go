package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"buildmax/internal/core/model"
	taskapp "buildmax/internal/core/task"
)

var (
	ErrWorkflowsNotConfigured     = errors.New("workflows not configured")
	ErrConversationsNotConfigured = errors.New("conversations not configured")
	ErrIssuesNotConfigured        = errors.New("issues not configured")
	ErrTasksNotConfigured         = errors.New("tasks not configured")
	ErrWorkflowNameRequired       = errors.New("workflow name required")
	ErrWorkflowDefinitionRequired = errors.New("workflow definition required")
	ErrWorkflowNotFound           = errors.New("workflow not found")
	ErrWorkflowRunNotFound        = errors.New("workflow run not found")
	ErrIssueNotFound              = errors.New("issue not found")
	ErrIssueWorkflowMismatch      = errors.New("issue not assigned to workflow")
	ErrInvalidDefinition          = errors.New("invalid workflow definition")
	ErrInvalidStepType            = errors.New("invalid workflow step type")
	ErrInvalidStepID              = errors.New("invalid workflow step_id")
	ErrInvalidTargetAgent         = errors.New("invalid target agent")
	ErrInvalidWorkflowStatus      = errors.New("invalid workflow status")
	ErrWorkflowNotPublished       = errors.New("workflow not published")
	ErrWorkflowArchived           = errors.New("workflow archived")
)

type Service struct {
	Workflows     model.WorkflowStore
	Agents        model.AgentStore
	Issues        model.IssueStore
	Conversations model.ConversationStore
	Task          *taskapp.Service
}

type Definition struct {
	Steps []DefinitionStep `json:"steps"`
}

type DefinitionStep struct {
	StepID        string `json:"step_id"`
	Type          string `json:"type"`
	TargetAgentID string `json:"target_agent_id"`
	Prompt        string `json:"prompt"`
}

type CreateWorkflowCmd struct {
	TeamID      string
	UserID      string
	Name        string
	Description string
	Definition  string
}

type UpdateWorkflowCmd struct {
	TeamID      string
	WorkflowID  string
	Name        *string
	Description *string
	Definition  *string
	Status      *string
}

type StartWorkflowRunCmd struct {
	TeamID     string
	UserID     string
	WorkflowID string
	IssueID    *string
}

// TaskRunTerminalInfo describes a task run that reached a terminal state.
type TaskRunTerminalInfo struct {
	TaskRunID      string
	TaskID         string
	ConversationID string
	UserID         string
	Status         string
	Output         *string
	ErrorMessage   *string
}

func (s *Service) ListWorkflows(ctx context.Context, teamID string) ([]model.Workflow, error) {
	if s.Workflows == nil {
		return nil, ErrWorkflowsNotConfigured
	}
	return s.Workflows.ListWorkflowsByTeam(ctx, teamID)
}

func (s *Service) CreateWorkflow(ctx context.Context, cmd CreateWorkflowCmd) (*model.Workflow, error) {
	if s.Workflows == nil {
		return nil, ErrWorkflowsNotConfigured
	}
	if strings.TrimSpace(cmd.Name) == "" {
		return nil, ErrWorkflowNameRequired
	}
	if strings.TrimSpace(cmd.Definition) == "" {
		return nil, ErrWorkflowDefinitionRequired
	}
	if _, err := s.parseAndValidateDefinition(ctx, cmd.TeamID, cmd.Definition); err != nil {
		return nil, err
	}
	return s.Workflows.CreateWorkflow(ctx, cmd.TeamID, cmd.UserID, strings.TrimSpace(cmd.Name), strings.TrimSpace(cmd.Description), cmd.Definition)
}

func (s *Service) GetWorkflow(ctx context.Context, teamID, workflowID string) (*model.Workflow, error) {
	if s.Workflows == nil {
		return nil, ErrWorkflowsNotConfigured
	}
	workflow, err := s.Workflows.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	if workflow == nil || workflow.TeamID != teamID {
		return nil, ErrWorkflowNotFound
	}
	return workflow, nil
}

func (s *Service) UpdateWorkflow(ctx context.Context, cmd UpdateWorkflowCmd) (*model.Workflow, error) {
	if s.Workflows == nil {
		return nil, ErrWorkflowsNotConfigured
	}
	in := model.UpdateWorkflowInput{
		Name:        cmd.Name,
		Description: cmd.Description,
		Definition:  cmd.Definition,
		Status:      nil,
	}
	if cmd.Definition != nil {
		if strings.TrimSpace(*cmd.Definition) == "" {
			return nil, ErrWorkflowDefinitionRequired
		}
		if _, err := s.parseAndValidateDefinition(ctx, cmd.TeamID, *cmd.Definition); err != nil {
			return nil, err
		}
	}
	if cmd.Status != nil {
		if !isValidWorkflowStatus(*cmd.Status) {
			return nil, ErrInvalidWorkflowStatus
		}
		in.Status = cmd.Status
	}
	workflow, err := s.Workflows.UpdateWorkflow(ctx, cmd.WorkflowID, cmd.TeamID, in)
	if err != nil {
		return nil, err
	}
	if workflow == nil {
		return nil, ErrWorkflowNotFound
	}
	return workflow, nil
}

func (s *Service) ListWorkflowRuns(ctx context.Context, teamID, workflowID string, limit, offset int) ([]model.WorkflowRun, int, error) {
	workflow, err := s.GetWorkflow(ctx, teamID, workflowID)
	if err != nil {
		return nil, 0, err
	}
	return s.Workflows.ListWorkflowRunsByWorkflow(ctx, workflow.WorkflowID, limit, offset)
}

func (s *Service) GetWorkflowRunDetail(ctx context.Context, teamID, workflowRunID string) (*model.WorkflowRun, []model.WorkflowStepRun, error) {
	if s.Workflows == nil {
		return nil, nil, ErrWorkflowsNotConfigured
	}
	run, err := s.Workflows.GetWorkflowRun(ctx, workflowRunID)
	if err != nil {
		return nil, nil, err
	}
	if run == nil {
		return nil, nil, ErrWorkflowRunNotFound
	}
	workflow, err := s.GetWorkflow(ctx, teamID, run.WorkflowID)
	if err != nil {
		return nil, nil, err
	}
	if workflow == nil {
		return nil, nil, ErrWorkflowNotFound
	}
	steps, err := s.Workflows.ListWorkflowStepRuns(ctx, workflowRunID)
	if err != nil {
		return nil, nil, err
	}
	return run, steps, nil
}

func (s *Service) StartWorkflowRun(ctx context.Context, cmd StartWorkflowRunCmd) (*model.WorkflowRun, []model.WorkflowStepRun, error) {
	if s.Workflows == nil {
		return nil, nil, ErrWorkflowsNotConfigured
	}
	if s.Conversations == nil {
		return nil, nil, ErrConversationsNotConfigured
	}
	if s.Task == nil || s.Task.Tasks == nil {
		return nil, nil, ErrTasksNotConfigured
	}
	workflow, err := s.GetWorkflow(ctx, cmd.TeamID, cmd.WorkflowID)
	if err != nil {
		return nil, nil, err
	}
	if workflow.Status == model.WorkflowStatusArchived {
		return nil, nil, ErrWorkflowArchived
	}
	if workflow.Status != model.WorkflowStatusPublished {
		return nil, nil, ErrWorkflowNotPublished
	}
	def, err := s.parseAndValidateDefinition(ctx, cmd.TeamID, workflow.Definition)
	if err != nil {
		return nil, nil, err
	}
	if err := s.validateIssueForRun(ctx, cmd.TeamID, workflow.WorkflowID, cmd.IssueID); err != nil {
		return nil, nil, err
	}
	conv, err := s.Conversations.CreateConversationInTeam(ctx, cmd.TeamID, cmd.UserID, "workflow", cmd.UserID)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().Unix()
	run, err := s.Workflows.CreateWorkflowRun(ctx, model.CreateWorkflowRunInput{
		WorkflowID:     workflow.WorkflowID,
		IssueID:        cmd.IssueID,
		ConversationID: conv.ConversationID,
		Status:         model.WorkflowRunStatusRunning,
		CreatedBy:      cmd.UserID,
		StartedAt:      &now,
	})
	if err != nil {
		return nil, nil, err
	}
	stepsIn := make([]model.CreateWorkflowStepRunInput, len(def.Steps))
	for i := range def.Steps {
		target := def.Steps[i].TargetAgentID
		stepsIn[i] = model.CreateWorkflowStepRunInput{
			StepID:        def.Steps[i].StepID,
			StepIndex:     i,
			StepType:      def.Steps[i].Type,
			TargetAgentID: &target,
			Prompt:        def.Steps[i].Prompt,
			Status:        model.WorkflowStepRunStatusPending,
		}
	}
	stepRuns, err := s.Workflows.CreateWorkflowStepRuns(ctx, run.WorkflowRunID, stepsIn)
	if err != nil {
		return nil, nil, err
	}
	if _, err := s.dispatchNextStep(ctx, cmd.TeamID, cmd.UserID, run, stepRuns); err != nil {
		return nil, nil, err
	}
	stepRuns, err = s.Workflows.ListWorkflowStepRuns(ctx, run.WorkflowRunID)
	if err != nil {
		return nil, nil, err
	}
	run, err = s.Workflows.GetWorkflowRun(ctx, run.WorkflowRunID)
	if err != nil {
		return nil, nil, err
	}
	return run, stepRuns, nil
}

func isValidWorkflowStatus(status string) bool {
	switch status {
	case model.WorkflowStatusDraft, model.WorkflowStatusPublished, model.WorkflowStatusArchived:
		return true
	default:
		return false
	}
}

func (s *Service) HandleTaskRunTerminal(ctx context.Context, info TaskRunTerminalInfo) error {
	if s.Workflows == nil {
		return nil
	}
	stepRun, err := s.Workflows.GetWorkflowStepRunByTaskRunID(ctx, info.TaskRunID)
	if err != nil {
		return err
	}
	if stepRun == nil {
		stepRun, err = s.Workflows.GetWorkflowStepRunByTaskID(ctx, info.TaskID)
		if err != nil || stepRun == nil {
			return err
		}
	}
	run, err := s.Workflows.GetWorkflowRun(ctx, stepRun.WorkflowRunID)
	if err != nil || run == nil {
		if err == nil {
			return ErrWorkflowRunNotFound
		}
		return err
	}
	now := time.Now().Unix()
	if info.Status == string(model.RunStatusSucceeded) {
		summary := summarizeOutput(info.Output)
		status := model.WorkflowStepRunStatusSucceeded
		if _, err := s.Workflows.UpdateWorkflowStepRun(ctx, stepRun.StepRunID, model.UpdateWorkflowStepRunInput{
			Status:        &status,
			TaskRunID:     &info.TaskRunID,
			OutputSummary: summary,
			EndedAt:       &now,
		}); err != nil {
			return err
		}
		steps, err := s.Workflows.ListWorkflowStepRuns(ctx, run.WorkflowRunID)
		if err != nil {
			return err
		}
		if _, err := s.dispatchNextStep(ctx, "", info.UserID, run, steps); err != nil {
			return err
		}
		return nil
	}
	stepStatus := model.WorkflowStepRunStatusFailed
	if _, err := s.Workflows.UpdateWorkflowStepRun(ctx, stepRun.StepRunID, model.UpdateWorkflowStepRunInput{
		Status:       &stepStatus,
		TaskRunID:    &info.TaskRunID,
		ErrorMessage: info.ErrorMessage,
		EndedAt:      &now,
	}); err != nil {
		return err
	}
	steps, err := s.Workflows.ListWorkflowStepRuns(ctx, run.WorkflowRunID)
	if err != nil {
		return err
	}
	blocked := model.WorkflowStepRunStatusBlocked
	for i := range steps {
		if steps[i].StepIndex > stepRun.StepIndex && steps[i].Status == model.WorkflowStepRunStatusPending {
			if _, err := s.Workflows.UpdateWorkflowStepRun(ctx, steps[i].StepRunID, model.UpdateWorkflowStepRunInput{Status: &blocked}); err != nil {
				return err
			}
		}
	}
	runStatus := model.WorkflowRunStatusFailed
	_, err = s.Workflows.UpdateWorkflowRun(ctx, run.WorkflowRunID, model.UpdateWorkflowRunInput{
		Status:       runStatus,
		EndedAt:      &now,
		ErrorMessage: info.ErrorMessage,
	})
	return err
}

func (s *Service) dispatchNextStep(ctx context.Context, teamID, userID string, run *model.WorkflowRun, steps []model.WorkflowStepRun) (*model.WorkflowStepRun, error) {
	for i := range steps {
		if steps[i].Status != model.WorkflowStepRunStatusPending {
			continue
		}
		if teamID == "" {
			workflow, err := s.Workflows.GetWorkflow(ctx, run.WorkflowID)
			if err != nil {
				return nil, err
			}
			if workflow == nil {
				return nil, ErrWorkflowNotFound
			}
			teamID = workflow.TeamID
		}
		startedAt := time.Now().Unix()
		running := model.WorkflowStepRunStatusRunning
		task, taskRunID, err := s.createStepTask(ctx, teamID, userID, run.ConversationID, steps[i])
		if err != nil {
			failed := model.WorkflowRunStatusFailed
			_, _ = s.Workflows.UpdateWorkflowRun(ctx, run.WorkflowRunID, model.UpdateWorkflowRunInput{
				Status:       failed,
				EndedAt:      &startedAt,
				ErrorMessage: ptrError(err),
			})
			stepFailed := model.WorkflowStepRunStatusFailed
			_, _ = s.Workflows.UpdateWorkflowStepRun(ctx, steps[i].StepRunID, model.UpdateWorkflowStepRunInput{
				Status:       &stepFailed,
				ErrorMessage: ptrError(err),
				StartedAt:    &startedAt,
				EndedAt:      &startedAt,
			})
			return nil, err
		}
		_, err = s.Workflows.UpdateWorkflowStepRun(ctx, steps[i].StepRunID, model.UpdateWorkflowStepRunInput{
			Status:    &running,
			TaskID:    &task.TaskID,
			TaskRunID: &taskRunID,
			StartedAt: &startedAt,
		})
		if err != nil {
			return nil, err
		}
		return &steps[i], nil
	}
	endedAt := time.Now().Unix()
	status := model.WorkflowRunStatusSucceeded
	_, err := s.Workflows.UpdateWorkflowRun(ctx, run.WorkflowRunID, model.UpdateWorkflowRunInput{
		Status:  status,
		EndedAt: &endedAt,
	})
	return nil, err
}

func (s *Service) createStepTask(ctx context.Context, teamID, userID, conversationID string, step model.WorkflowStepRun) (*model.Task, string, error) {
	agentID := ""
	if step.TargetAgentID != nil {
		agentID = *step.TargetAgentID
	}
	if agentID == "" {
		return nil, "", ErrInvalidTargetAgent
	}
	agent, err := s.Agents.GetAgent(ctx, agentID)
	if err != nil {
		return nil, "", err
	}
	if agent == nil || agent.TeamID != teamID {
		return nil, "", ErrInvalidTargetAgent
	}
	input := buildWorkflowTaskInput(agent, step.Prompt)
	task, err := s.Task.CreateTask(ctx, taskapp.CreateTaskCmd{
		ConversationID: conversationID,
		UserID:         userID,
		TeamID:         teamID,
		Input:          input,
		AgentID:        &agentID,
		CreatedByType:  model.RunCreatedByTypeUser,
		TriggerSource:  model.RunTriggerSourceWorkflowStep,
	})
	if err != nil {
		return nil, "", err
	}
	runID := ""
	if task.LastRunID != nil {
		runID = *task.LastRunID
	}
	return task, runID, nil
}

func (s *Service) validateIssueForRun(ctx context.Context, teamID, workflowID string, issueID *string) error {
	if issueID == nil || *issueID == "" {
		return nil
	}
	if s.Issues == nil {
		return ErrIssuesNotConfigured
	}
	issue, err := s.Issues.GetIssue(ctx, *issueID)
	if err != nil {
		return err
	}
	if issue == nil || issue.TeamID != teamID {
		return ErrIssueNotFound
	}
	if issue.AssigneeKind == nil || issue.AssigneeID == nil || *issue.AssigneeKind != model.IssueAssigneeWorkflow || *issue.AssigneeID != workflowID {
		return ErrIssueWorkflowMismatch
	}
	return nil
}

func (s *Service) parseAndValidateDefinition(ctx context.Context, teamID, raw string) (*Definition, error) {
	var def Definition
	if err := json.Unmarshal([]byte(raw), &def); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidDefinition, err)
	}
	if len(def.Steps) == 0 {
		return nil, ErrInvalidDefinition
	}
	seen := make(map[string]struct{}, len(def.Steps))
	for i := range def.Steps {
		step := &def.Steps[i]
		step.StepID = strings.TrimSpace(step.StepID)
		step.Type = strings.TrimSpace(step.Type)
		step.TargetAgentID = strings.TrimSpace(step.TargetAgentID)
		step.Prompt = strings.TrimSpace(step.Prompt)
		if step.StepID == "" {
			return nil, ErrInvalidStepID
		}
		if _, ok := seen[step.StepID]; ok {
			return nil, ErrInvalidStepID
		}
		seen[step.StepID] = struct{}{}
		if step.Type != model.WorkflowStepTypeAgentTask {
			return nil, ErrInvalidStepType
		}
		if step.TargetAgentID == "" || step.Prompt == "" {
			return nil, ErrInvalidDefinition
		}
		if s.Agents == nil {
			return nil, ErrInvalidTargetAgent
		}
		agent, err := s.Agents.GetAgent(ctx, step.TargetAgentID)
		if err != nil {
			return nil, err
		}
		if agent == nil || agent.TeamID != teamID {
			return nil, ErrInvalidTargetAgent
		}
	}
	return &def, nil
}

func summarizeOutput(output *string) *string {
	if output == nil {
		return nil
	}
	value := strings.TrimSpace(*output)
	if value == "" {
		return nil
	}
	if len(value) > 500 {
		value = value[:500]
	}
	return &value
}

func ptrError(err error) *string {
	if err == nil {
		return nil
	}
	msg := err.Error()
	return &msg
}

func buildWorkflowTaskInput(agent *model.Agent, prompt string) string {
	base := fmt.Sprintf("Agent: %s\nDescription: %s\nInstructions:\n%s", agent.Name, agent.Description, agent.Instructions)
	if strings.TrimSpace(prompt) == "" {
		return base
	}
	return base + "\n\n" + prompt
}
