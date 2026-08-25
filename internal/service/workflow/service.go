package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	convchannel "github.com/gougoujiang/buildmax/internal/service/conversation/channel"

	coreconv "github.com/gougoujiang/buildmax/internal/core/conversation"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/service/task"
	"github.com/gougoujiang/buildmax/internal/util"
)

var (
	ErrWorkflowsNotConfigured     = apierr.New(apierr.KindNotConfigured, "workflows not configured")
	ErrConversationsNotConfigured = apierr.New(apierr.KindNotConfigured, "conversations not configured")
	ErrIssuesNotConfigured        = apierr.New(apierr.KindNotConfigured, "issues not configured")
	ErrTasksNotConfigured         = apierr.New(apierr.KindNotConfigured, "tasks not configured")
	ErrWorkflowNameRequired       = apierr.New(apierr.KindInvalid, "workflow name required")
	ErrWorkflowDefinitionRequired = apierr.New(apierr.KindInvalid, "workflow definition required")
	ErrWorkflowNotFound           = apierr.New(apierr.KindNotFound, "workflow not found")
	ErrWorkflowRunNotFound        = apierr.New(apierr.KindNotFound, "workflow run not found")
	ErrWorkflowRevisionNotFound   = apierr.New(apierr.KindNotFound, "workflow revision not found")
	ErrIssueNotFound              = apierr.New(apierr.KindNotFound, "issue not found")
	ErrIssueWorkflowMismatch      = apierr.New(apierr.KindInvalid, "issue not assigned to workflow")
	ErrInvalidDefinition          = apierr.New(apierr.KindInvalid, "invalid workflow definition")
	ErrInvalidStepType            = apierr.New(apierr.KindInvalid, "invalid workflow step type")
	ErrInvalidStepID              = apierr.New(apierr.KindInvalid, "invalid workflow step_id")
	ErrInvalidTargetAgent         = apierr.New(apierr.KindInvalid, "invalid target agent")
	ErrInvalidWorkflowStatus      = apierr.New(apierr.KindInvalid, "invalid workflow status")
	ErrWorkflowNotPublished       = apierr.New(apierr.KindInvalid, "workflow not published")
	ErrWorkflowArchived           = apierr.New(apierr.KindInvalid, "workflow archived")
)

type Service struct {
	Workflows     model.WorkflowStore
	Agents        model.AgentStore
	Issues        model.IssueStore
	Conversations coreconv.Store
	TaskService   *task.Service
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
	UserID      string
	WorkflowID  string
	Name        *string
	Description *string
	Definition  *string
	Status      *string
}

// RestoreWorkflowRevisionCmd restores an earlier revision's content.
type RestoreWorkflowRevisionCmd struct {
	TeamID     string
	UserID     string
	WorkflowID string
	Revision   int
}

type StartWorkflowRunCmd struct {
	TeamID     string
	UserID     string
	WorkflowID string
	IssueID    *string
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
	if _, _, err := s.parseAndValidateDefinition(ctx, cmd.TeamID, cmd.Definition); err != nil {
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
		UpdatedBy:   cmd.UserID,
	}
	if cmd.Definition != nil {
		if strings.TrimSpace(*cmd.Definition) == "" {
			return nil, ErrWorkflowDefinitionRequired
		}
		if _, _, err := s.parseAndValidateDefinition(ctx, cmd.TeamID, *cmd.Definition); err != nil {
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

func (s *Service) ListWorkflowRevisions(ctx context.Context, teamID, workflowID string, limit, offset int) ([]model.WorkflowRevision, int, error) {
	workflow, err := s.GetWorkflow(ctx, teamID, workflowID)
	if err != nil {
		return nil, 0, err
	}
	return s.Workflows.ListWorkflowRevisions(ctx, workflow.ID, limit, offset)
}

// RestoreWorkflowRevision writes an earlier revision's name, description, and
// definition back to the workflow, which appends a new revision rather than
// rewinding to the old one.
//
// Status is deliberately not restored. It is lifecycle state, not content:
// restoring the definition of a draft revision must not unpublish a workflow
// teams are running, and restoring a published one must not publish a draft
// without anyone deciding to. The definition is revalidated, so a revision
// whose agents have since been deleted is refused rather than restored into a
// plan that cannot run.
func (s *Service) RestoreWorkflowRevision(ctx context.Context, cmd RestoreWorkflowRevisionCmd) (*model.Workflow, error) {
	if s.Workflows == nil {
		return nil, ErrWorkflowsNotConfigured
	}
	workflow, err := s.GetWorkflow(ctx, cmd.TeamID, cmd.WorkflowID)
	if err != nil {
		return nil, err
	}
	revision, err := s.Workflows.GetWorkflowRevision(ctx, workflow.ID, cmd.Revision)
	if err != nil {
		return nil, err
	}
	if revision == nil {
		return nil, ErrWorkflowRevisionNotFound
	}
	return s.UpdateWorkflow(ctx, UpdateWorkflowCmd{
		TeamID:      cmd.TeamID,
		UserID:      cmd.UserID,
		WorkflowID:  workflow.ID,
		Name:        &revision.Name,
		Description: &revision.Description,
		Definition:  &revision.Definition,
	})
}

// PublishedWorkflowsUsingAgent returns the team's published workflows whose
// definition names agentID.
//
// It exists so deleting an agent can be refused while a workflow that can still
// be run depends on it. Draft and archived workflows do not count: neither can
// start a run, and publishing one revalidates its agents.
//
// A published workflow whose definition no longer parses is skipped rather than
// treated as a reference. It cannot run either way, and blocking an unrelated
// delete on it would leave no way forward.
func (s *Service) PublishedWorkflowsUsingAgent(ctx context.Context, teamID, agentID string) ([]model.Workflow, error) {
	if s.Workflows == nil {
		return nil, ErrWorkflowsNotConfigured
	}
	workflows, err := s.Workflows.ListWorkflowsByTeam(ctx, teamID)
	if err != nil {
		return nil, err
	}
	var using []model.Workflow
	for i := range workflows {
		if workflows[i].Status != model.WorkflowStatusPublished {
			continue
		}
		def, err := parseDefinition(workflows[i].Definition)
		if err != nil {
			continue
		}
		for j := range def.Steps {
			if def.Steps[j].TargetAgentID == agentID {
				using = append(using, workflows[i])
				break
			}
		}
	}
	return using, nil
}

func (s *Service) ListWorkflowRuns(ctx context.Context, teamID, workflowID string, limit, offset int) ([]model.WorkflowRun, int, error) {
	workflow, err := s.GetWorkflow(ctx, teamID, workflowID)
	if err != nil {
		return nil, 0, err
	}
	return s.Workflows.ListWorkflowRunsByWorkflow(ctx, workflow.ID, limit, offset)
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
	if s.TaskService == nil || s.TaskService.Tasks == nil {
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
	def, agents, err := s.parseAndValidateDefinition(ctx, cmd.TeamID, workflow.Definition)
	if err != nil {
		return nil, nil, err
	}
	if err := s.validateIssueForRun(ctx, cmd.TeamID, workflow.ID, cmd.IssueID); err != nil {
		return nil, nil, err
	}
	conv, err := s.Conversations.CreateConversationInTeam(ctx, cmd.TeamID, cmd.UserID, convchannel.ChannelWorkflow, cmd.UserID)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	run, err := s.Workflows.CreateWorkflowRun(ctx, model.CreateWorkflowRunInput{
		WorkflowID:       workflow.ID,
		WorkflowRevision: workflow.Revision,
		IssueID:          cmd.IssueID,
		ConversationID:   conv.ID,
		Status:           model.WorkflowRunStatusRunning,
		CreatedBy:        cmd.UserID,
		StartedAt:        &now,
	})
	if err != nil {
		return nil, nil, err
	}
	stepsIn := make([]model.CreateWorkflowStepRunInput, len(def.Steps))
	for i := range def.Steps {
		target := def.Steps[i].TargetAgentID
		agent := agents[target]
		stepsIn[i] = model.CreateWorkflowStepRunInput{
			StepID:            def.Steps[i].StepID,
			StepIndex:         i,
			StepType:          def.Steps[i].Type,
			TargetAgentID:     &target,
			AgentName:         agent.Name,
			AgentDescription:  agent.Description,
			AgentInstructions: agent.Instructions,
			AgentRevision:     agent.Revision,
			Prompt:            def.Steps[i].Prompt,
			Status:            model.WorkflowStepRunStatusPending,
		}
	}
	stepRuns, err := s.Workflows.CreateWorkflowStepRuns(ctx, run.ID, stepsIn)
	if err != nil {
		return nil, nil, err
	}
	if _, err := s.dispatchNextStep(ctx, cmd.TeamID, cmd.UserID, run, stepRuns); err != nil {
		return nil, nil, err
	}
	stepRuns, err = s.Workflows.ListWorkflowStepRuns(ctx, run.ID)
	if err != nil {
		return nil, nil, err
	}
	run, err = s.Workflows.GetWorkflowRun(ctx, run.ID)
	if err != nil {
		return nil, nil, err
	}
	return run, stepRuns, nil
}

func (s *Service) HandleTaskRunTerminal(ctx context.Context, info model.TaskRunTerminalInfo) error {
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
	now := time.Now().UTC()
	if info.Status == string(model.RunStatusSucceeded) {
		summary := summarizeOutput(info.Output)
		status := model.WorkflowStepRunStatusSucceeded
		if _, err := s.Workflows.UpdateWorkflowStepRun(ctx, stepRun.ID, model.UpdateWorkflowStepRunInput{
			Status:        &status,
			TaskRunID:     &info.TaskRunID,
			OutputSummary: summary,
			EndedAt:       &now,
		}); err != nil {
			return err
		}
		steps, err := s.Workflows.ListWorkflowStepRuns(ctx, run.ID)
		if err != nil {
			return err
		}
		if _, err := s.dispatchNextStep(ctx, "", info.UserID, run, steps); err != nil {
			return err
		}
		return nil
	}
	// A canceled step stops the run the same way a failed one does, but it is
	// not a failure: someone stopped this work on purpose, and a run labelled
	// failed would send whoever reads it looking for a fault that never
	// happened.
	stepStatus := model.WorkflowStepRunStatusFailed
	runStatus := model.WorkflowRunStatusFailed
	if info.Status == string(model.RunStatusCanceled) {
		stepStatus = model.WorkflowStepRunStatusCanceled
		runStatus = model.WorkflowRunStatusCanceled
	}
	if _, err := s.Workflows.UpdateWorkflowStepRun(ctx, stepRun.ID, model.UpdateWorkflowStepRunInput{
		Status:       &stepStatus,
		TaskRunID:    &info.TaskRunID,
		ErrorMessage: info.ErrorMessage,
		EndedAt:      &now,
	}); err != nil {
		return err
	}
	steps, err := s.Workflows.ListWorkflowStepRuns(ctx, run.ID)
	if err != nil {
		return err
	}
	blocked := model.WorkflowStepRunStatusBlocked
	for i := range steps {
		if steps[i].StepIndex > stepRun.StepIndex && steps[i].Status == model.WorkflowStepRunStatusPending {
			if _, err := s.Workflows.UpdateWorkflowStepRun(ctx, steps[i].ID, model.UpdateWorkflowStepRunInput{Status: &blocked}); err != nil {
				return err
			}
		}
	}
	_, err = s.Workflows.UpdateWorkflowRun(ctx, run.ID, model.UpdateWorkflowRunInput{
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
		startedAt := time.Now().UTC()
		running := model.WorkflowStepRunStatusRunning
		taskItem, taskRunID, err := s.createStepTask(ctx, teamID, userID, run.ConversationID, steps[i])
		if err != nil {
			failed := model.WorkflowRunStatusFailed
			_, _ = s.Workflows.UpdateWorkflowRun(ctx, run.ID, model.UpdateWorkflowRunInput{
				Status:       failed,
				EndedAt:      &startedAt,
				ErrorMessage: ptrError(err),
			})
			stepFailed := model.WorkflowStepRunStatusFailed
			_, _ = s.Workflows.UpdateWorkflowStepRun(ctx, steps[i].ID, model.UpdateWorkflowStepRunInput{
				Status:       &stepFailed,
				ErrorMessage: ptrError(err),
				StartedAt:    &startedAt,
				EndedAt:      &startedAt,
			})
			return nil, err
		}
		_, err = s.Workflows.UpdateWorkflowStepRun(ctx, steps[i].ID, model.UpdateWorkflowStepRunInput{
			Status:    &running,
			TaskID:    &taskItem.ID,
			TaskRunID: &taskRunID,
			StartedAt: &startedAt,
		})
		if err != nil {
			return nil, err
		}
		return &steps[i], nil
	}
	endedAt := time.Now().UTC()
	status := model.WorkflowRunStatusSucceeded
	_, err := s.Workflows.UpdateWorkflowRun(ctx, run.ID, model.UpdateWorkflowRunInput{
		Status:  status,
		EndedAt: &endedAt,
	})
	return nil, err
}

// stepAgent returns the agent definition a step must run with. Steps recorded since
// runs snapshot their agent carry it on the step run itself, so an edit to the agent
// while the run is in flight cannot change what a later step sends. Steps written
// before that fall back to the agent definition as it stands now, deleted or not:
// the run was authorized when it started, and refusing to finish it because the
// agent has since been deleted would strand it half done.
func (s *Service) stepAgent(ctx context.Context, teamID, agentID string, step model.WorkflowStepRun) (*model.Agent, error) {
	if step.AgentName != "" || step.AgentInstructions != "" {
		return &model.Agent{
			ID:           agentID,
			TeamID:       teamID,
			Name:         step.AgentName,
			Description:  step.AgentDescription,
			Instructions: step.AgentInstructions,
			Revision:     step.AgentRevision,
		}, nil
	}
	if s.Agents == nil {
		return nil, ErrInvalidTargetAgent
	}
	agent, err := s.Agents.GetAgentIncludingDeleted(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if agent == nil || agent.TeamID != teamID {
		return nil, ErrInvalidTargetAgent
	}
	return agent, nil
}

func (s *Service) createStepTask(ctx context.Context, teamID, userID, conversationID string, step model.WorkflowStepRun) (*model.Task, string, error) {
	agentID := ""
	if step.TargetAgentID != nil {
		agentID = *step.TargetAgentID
	}
	if agentID == "" {
		return nil, "", ErrInvalidTargetAgent
	}
	agent, err := s.stepAgent(ctx, teamID, agentID, step)
	if err != nil {
		return nil, "", err
	}
	input := buildWorkflowTaskInput(agent, step.Prompt)
	taskItem, err := s.TaskService.CreateTask(ctx, task.CreateTaskCmd{
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
	if taskItem.LastRunID != nil {
		runID = *taskItem.LastRunID
	}
	return taskItem, runID, nil
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

// parseAndValidateDefinition parses raw, checks every step's target agent, and returns
// the resolved agents keyed by agent ID so a caller can snapshot them.
func (s *Service) parseAndValidateDefinition(ctx context.Context, teamID, raw string) (*model.WorkflowDefinition, map[string]model.Agent, error) {
	def, err := parseDefinition(raw)
	if err != nil {
		return nil, nil, err
	}
	agents, err := s.resolveDefinitionAgents(ctx, teamID, def)
	if err != nil {
		return nil, nil, err
	}
	return def, agents, nil
}

// parseDefinition unmarshals raw JSON into a WorkflowDefinition and validates structural fields
// (step count, unique IDs, required type/agent/prompt). Does not touch the database.
func parseDefinition(raw string) (*model.WorkflowDefinition, error) {
	var def model.WorkflowDefinition
	if err := json.Unmarshal([]byte(raw), &def); err != nil {
		return nil, apierr.Detail(ErrInvalidDefinition, "%v", err)
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
	}
	return &def, nil
}

// resolveDefinitionAgents checks that every step's target agent is live and belongs
// to teamID, and returns those agents keyed by agent ID.
//
// A deleted agent is refused. This runs when a workflow is written and again when a
// run starts, so a plan cannot take a new dependency on a deleted agent, and a
// workflow that lost one is refused at the start of a run rather than partway
// through it.
func (s *Service) resolveDefinitionAgents(ctx context.Context, teamID string, def *model.WorkflowDefinition) (map[string]model.Agent, error) {
	if s.Agents == nil {
		return nil, ErrInvalidTargetAgent
	}
	agents := make(map[string]model.Agent, len(def.Steps))
	for i := range def.Steps {
		agentID := def.Steps[i].TargetAgentID
		if _, ok := agents[agentID]; ok {
			continue
		}
		agent, err := s.Agents.GetAgent(ctx, agentID)
		if err != nil {
			return nil, err
		}
		if agent == nil || agent.TeamID != teamID {
			return nil, ErrInvalidTargetAgent
		}
		agents[agentID] = *agent
	}
	return agents, nil
}

func isValidWorkflowStatus(status string) bool {
	switch status {
	case model.WorkflowStatusDraft, model.WorkflowStatusPublished, model.WorkflowStatusArchived:
		return true
	default:
		return false
	}
}

func summarizeOutput(output *string) *string {
	if output == nil {
		return nil
	}
	value := strings.TrimSpace(*output)
	if value == "" {
		return nil
	}
	value = util.ClipRunes(value, 500)
	return util.Ptr(value)
}

func ptrError(err error) *string {
	if err == nil {
		return nil
	}
	return util.Ptr(err.Error())
}

func buildWorkflowTaskInput(agent *model.Agent, prompt string) string {
	base := fmt.Sprintf("Agent: %s\nDescription: %s\nInstructions:\n%s", agent.Name, agent.Description, agent.Instructions)
	if strings.TrimSpace(prompt) == "" {
		return base
	}
	return base + "\n\n" + prompt
}
