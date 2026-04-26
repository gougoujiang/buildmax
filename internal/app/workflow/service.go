package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	taskapp "buildmax/internal/app/task"
	"buildmax/internal/server/worker"
	"buildmax/internal/storage/entity"
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
)

type Service struct {
	Workflows     entity.WorkflowStore
	Agents        entity.AgentStore
	Issues        entity.IssueStore
	Conversations entity.ConversationStore
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
}

type StartWorkflowRunCmd struct {
	TeamID     string
	UserID     string
	WorkflowID string
	IssueID    *string
}

func (s *Service) ListWorkflows(ctx context.Context, teamID string) ([]entity.Workflow, error) {
	if s.Workflows == nil {
		return nil, ErrWorkflowsNotConfigured
	}
	return s.Workflows.ListWorkflowsByTeam(ctx, teamID)
}

func (s *Service) CreateWorkflow(ctx context.Context, cmd CreateWorkflowCmd) (*entity.Workflow, error) {
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

func (s *Service) GetWorkflow(ctx context.Context, teamID, workflowID string) (*entity.Workflow, error) {
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

func (s *Service) UpdateWorkflow(ctx context.Context, cmd UpdateWorkflowCmd) (*entity.Workflow, error) {
	if s.Workflows == nil {
		return nil, ErrWorkflowsNotConfigured
	}
	in := entity.UpdateWorkflowInput{
		Name:        cmd.Name,
		Description: cmd.Description,
		Definition:  cmd.Definition,
	}
	if cmd.Definition != nil {
		if strings.TrimSpace(*cmd.Definition) == "" {
			return nil, ErrWorkflowDefinitionRequired
		}
		if _, err := s.parseAndValidateDefinition(ctx, cmd.TeamID, *cmd.Definition); err != nil {
			return nil, err
		}
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

func (s *Service) ListWorkflowRuns(ctx context.Context, teamID, workflowID string, limit, offset int) ([]entity.WorkflowRun, int, error) {
	workflow, err := s.GetWorkflow(ctx, teamID, workflowID)
	if err != nil {
		return nil, 0, err
	}
	return s.Workflows.ListWorkflowRunsByWorkflow(ctx, workflow.WorkflowID, limit, offset)
}

func (s *Service) GetWorkflowRunDetail(ctx context.Context, teamID, workflowRunID string) (*entity.WorkflowRun, []entity.WorkflowStepRun, error) {
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

func (s *Service) StartWorkflowRun(ctx context.Context, cmd StartWorkflowRunCmd) (*entity.WorkflowRun, []entity.WorkflowStepRun, error) {
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
	run, err := s.Workflows.CreateWorkflowRun(ctx, entity.CreateWorkflowRunInput{
		WorkflowID:     workflow.WorkflowID,
		IssueID:        cmd.IssueID,
		ConversationID: conv.ConversationID,
		Status:         entity.WorkflowRunStatusRunning,
		CreatedBy:      cmd.UserID,
		StartedAt:      &now,
	})
	if err != nil {
		return nil, nil, err
	}
	stepsIn := make([]entity.CreateWorkflowStepRunInput, len(def.Steps))
	for i := range def.Steps {
		target := def.Steps[i].TargetAgentID
		stepsIn[i] = entity.CreateWorkflowStepRunInput{
			StepID:        def.Steps[i].StepID,
			StepIndex:     i,
			StepType:      def.Steps[i].Type,
			TargetAgentID: &target,
			Prompt:        def.Steps[i].Prompt,
			Status:        entity.WorkflowStepRunStatusPending,
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

func (s *Service) HandleTaskRunTerminal(ctx context.Context, info worker.TaskRunTerminalInfo) error {
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
	if info.Status == string(entity.RunStatusSucceeded) {
		summary := summarizeOutput(info.Output)
		status := entity.WorkflowStepRunStatusSucceeded
		if _, err := s.Workflows.UpdateWorkflowStepRun(ctx, stepRun.StepRunID, entity.UpdateWorkflowStepRunInput{
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
	stepStatus := entity.WorkflowStepRunStatusFailed
	if _, err := s.Workflows.UpdateWorkflowStepRun(ctx, stepRun.StepRunID, entity.UpdateWorkflowStepRunInput{
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
	blocked := entity.WorkflowStepRunStatusBlocked
	for i := range steps {
		if steps[i].StepIndex > stepRun.StepIndex && steps[i].Status == entity.WorkflowStepRunStatusPending {
			if _, err := s.Workflows.UpdateWorkflowStepRun(ctx, steps[i].StepRunID, entity.UpdateWorkflowStepRunInput{Status: &blocked}); err != nil {
				return err
			}
		}
	}
	runStatus := entity.WorkflowRunStatusFailed
	_, err = s.Workflows.UpdateWorkflowRun(ctx, run.WorkflowRunID, entity.UpdateWorkflowRunInput{
		Status:       runStatus,
		EndedAt:      &now,
		ErrorMessage: info.ErrorMessage,
	})
	return err
}

func (s *Service) dispatchNextStep(ctx context.Context, teamID, userID string, run *entity.WorkflowRun, steps []entity.WorkflowStepRun) (*entity.WorkflowStepRun, error) {
	for i := range steps {
		if steps[i].Status != entity.WorkflowStepRunStatusPending {
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
		running := entity.WorkflowStepRunStatusRunning
		task, taskRunID, err := s.createStepTask(ctx, teamID, userID, run.ConversationID, steps[i])
		if err != nil {
			failed := entity.WorkflowRunStatusFailed
			_, _ = s.Workflows.UpdateWorkflowRun(ctx, run.WorkflowRunID, entity.UpdateWorkflowRunInput{
				Status:       failed,
				EndedAt:      &startedAt,
				ErrorMessage: ptrError(err),
			})
			stepFailed := entity.WorkflowStepRunStatusFailed
			_, _ = s.Workflows.UpdateWorkflowStepRun(ctx, steps[i].StepRunID, entity.UpdateWorkflowStepRunInput{
				Status:       &stepFailed,
				ErrorMessage: ptrError(err),
				StartedAt:    &startedAt,
				EndedAt:      &startedAt,
			})
			return nil, err
		}
		_, err = s.Workflows.UpdateWorkflowStepRun(ctx, steps[i].StepRunID, entity.UpdateWorkflowStepRunInput{
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
	status := entity.WorkflowRunStatusSucceeded
	_, err := s.Workflows.UpdateWorkflowRun(ctx, run.WorkflowRunID, entity.UpdateWorkflowRunInput{
		Status:  status,
		EndedAt: &endedAt,
	})
	return nil, err
}

func (s *Service) createStepTask(ctx context.Context, teamID, userID, conversationID string, step entity.WorkflowStepRun) (*entity.Task, string, error) {
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
	if issue.AssigneeKind == nil || issue.AssigneeID == nil || *issue.AssigneeKind != entity.IssueAssigneeWorkflow || *issue.AssigneeID != workflowID {
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
		if step.Type != entity.WorkflowStepTypeAgentTask {
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

func buildWorkflowTaskInput(agent *entity.Agent, prompt string) string {
	base := fmt.Sprintf("Agent: %s\nDescription: %s\nInstructions:\n%s", agent.Name, agent.Description, agent.Instructions)
	if strings.TrimSpace(prompt) == "" {
		return base
	}
	return base + "\n\n" + prompt
}
