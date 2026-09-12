package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	agentdef "github.com/icloudbb/buildmax/internal/core/agentdef"
	"github.com/icloudbb/buildmax/internal/core/apierr"
	coreissue "github.com/icloudbb/buildmax/internal/core/issue"
	coretask "github.com/icloudbb/buildmax/internal/core/task"
	coreworkflow "github.com/icloudbb/buildmax/internal/core/workflow"
	"github.com/icloudbb/buildmax/internal/service/task"
	"github.com/icloudbb/buildmax/internal/util"
)

var (
	ErrWorkflowsNotConfigured     = apierr.New(apierr.KindNotConfigured, "workflows not configured")
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
	Workflows   coreworkflow.Store
	Agents      agentdef.Store
	Issues      coreissue.Store
	TaskService *task.Service
}

type CreateWorkflowCmd struct {
	SpaceID     string
	UserID      string
	Name        string
	Description string
	Definition  string
}

type UpdateWorkflowCmd struct {
	SpaceID     string
	UserID      string
	WorkflowID  string
	Name        *string
	Description *string
	Definition  *string
	Status      *string
}

// RestoreWorkflowRevisionCmd restores an earlier revision's content.
type RestoreWorkflowRevisionCmd struct {
	SpaceID    string
	UserID     string
	WorkflowID string
	Revision   int
}

type StartWorkflowRunCmd struct {
	SpaceID    string
	UserID     string
	WorkflowID string
	IssueID    *string
}

func (s *Service) ListWorkflows(ctx context.Context, spaceID string) ([]coreworkflow.Workflow, error) {
	if s.Workflows == nil {
		return nil, ErrWorkflowsNotConfigured
	}
	return s.Workflows.ListWorkflowsBySpace(ctx, spaceID)
}

func (s *Service) CreateWorkflow(ctx context.Context, cmd CreateWorkflowCmd) (*coreworkflow.Workflow, error) {
	if s.Workflows == nil {
		return nil, ErrWorkflowsNotConfigured
	}
	if strings.TrimSpace(cmd.Name) == "" {
		return nil, ErrWorkflowNameRequired
	}
	if strings.TrimSpace(cmd.Definition) == "" {
		return nil, ErrWorkflowDefinitionRequired
	}
	if _, _, err := s.parseAndValidateDefinition(ctx, cmd.SpaceID, cmd.Definition); err != nil {
		return nil, err
	}
	return s.Workflows.CreateWorkflow(ctx, cmd.SpaceID, cmd.UserID, strings.TrimSpace(cmd.Name), strings.TrimSpace(cmd.Description), cmd.Definition)
}

func (s *Service) GetWorkflow(ctx context.Context, spaceID, workflowID string) (*coreworkflow.Workflow, error) {
	if s.Workflows == nil {
		return nil, ErrWorkflowsNotConfigured
	}
	workflow, err := s.Workflows.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	if workflow == nil || workflow.SpaceID != spaceID {
		return nil, ErrWorkflowNotFound
	}
	return workflow, nil
}

func (s *Service) UpdateWorkflow(ctx context.Context, cmd UpdateWorkflowCmd) (*coreworkflow.Workflow, error) {
	if s.Workflows == nil {
		return nil, ErrWorkflowsNotConfigured
	}
	in := coreworkflow.UpdateInput{
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
		if _, _, err := s.parseAndValidateDefinition(ctx, cmd.SpaceID, *cmd.Definition); err != nil {
			return nil, err
		}
	}
	if cmd.Status != nil {
		if !isValidWorkflowStatus(*cmd.Status) {
			return nil, ErrInvalidWorkflowStatus
		}
		in.Status = cmd.Status
	}
	workflow, err := s.Workflows.UpdateWorkflow(ctx, cmd.WorkflowID, cmd.SpaceID, in)
	if err != nil {
		return nil, err
	}
	if workflow == nil {
		return nil, ErrWorkflowNotFound
	}
	return workflow, nil
}

func (s *Service) ListWorkflowRevisions(ctx context.Context, spaceID, workflowID string, limit, offset int) ([]coreworkflow.Revision, int, error) {
	workflow, err := s.GetWorkflow(ctx, spaceID, workflowID)
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
// spaces are running, and restoring a published one must not publish a draft
// without anyone deciding to. The definition is revalidated, so a revision
// whose agents have since been deleted is refused rather than restored into a
// plan that cannot run.
func (s *Service) RestoreWorkflowRevision(ctx context.Context, cmd RestoreWorkflowRevisionCmd) (*coreworkflow.Workflow, error) {
	if s.Workflows == nil {
		return nil, ErrWorkflowsNotConfigured
	}
	workflow, err := s.GetWorkflow(ctx, cmd.SpaceID, cmd.WorkflowID)
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
		SpaceID:     cmd.SpaceID,
		UserID:      cmd.UserID,
		WorkflowID:  workflow.ID,
		Name:        &revision.Name,
		Description: &revision.Description,
		Definition:  &revision.Definition,
	})
}

// PublishedWorkflowsUsingAgent returns the space's published workflows whose
// definition names agentID.
//
// It exists so deleting an agent can be refused while a workflow that can still
// be run depends on it. Draft and archived workflows do not count: neither can
// start a run, and publishing one revalidates its agents.
//
// A published workflow whose definition no longer parses is skipped rather than
// treated as a reference. It cannot run either way, and blocking an unrelated
// delete on it would leave no way forward.
func (s *Service) PublishedWorkflowsUsingAgent(ctx context.Context, spaceID, agentID string) ([]coreworkflow.Workflow, error) {
	if s.Workflows == nil {
		return nil, ErrWorkflowsNotConfigured
	}
	workflows, err := s.Workflows.ListWorkflowsBySpace(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	var using []coreworkflow.Workflow
	for i := range workflows {
		if workflows[i].Status != coreworkflow.StatusPublished {
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

func (s *Service) ListWorkflowRuns(ctx context.Context, spaceID, workflowID string, limit, offset int) ([]coreworkflow.Run, int, error) {
	workflow, err := s.GetWorkflow(ctx, spaceID, workflowID)
	if err != nil {
		return nil, 0, err
	}
	return s.Workflows.ListWorkflowRunsByWorkflow(ctx, workflow.ID, limit, offset)
}

func (s *Service) GetWorkflowRunDetail(ctx context.Context, spaceID, workflowRunID string) (*coreworkflow.Run, []coreworkflow.StepRun, error) {
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
	workflow, err := s.GetWorkflow(ctx, spaceID, run.WorkflowID)
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

func (s *Service) StartWorkflowRun(ctx context.Context, cmd StartWorkflowRunCmd) (*coreworkflow.Run, []coreworkflow.StepRun, error) {
	if s.Workflows == nil {
		return nil, nil, ErrWorkflowsNotConfigured
	}
	if s.TaskService == nil || s.TaskService.Tasks == nil {
		return nil, nil, ErrTasksNotConfigured
	}
	workflow, err := s.GetWorkflow(ctx, cmd.SpaceID, cmd.WorkflowID)
	if err != nil {
		return nil, nil, err
	}
	if workflow.Status == coreworkflow.StatusArchived {
		return nil, nil, ErrWorkflowArchived
	}
	if workflow.Status != coreworkflow.StatusPublished {
		return nil, nil, ErrWorkflowNotPublished
	}
	def, agents, err := s.parseAndValidateDefinition(ctx, cmd.SpaceID, workflow.Definition)
	if err != nil {
		return nil, nil, err
	}
	if err := s.validateIssueForRun(ctx, cmd.SpaceID, workflow.ID, cmd.IssueID); err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	run, err := s.Workflows.CreateWorkflowRun(ctx, coreworkflow.CreateRunInput{
		WorkflowID:       workflow.ID,
		WorkflowRevision: workflow.Revision,
		IssueID:          cmd.IssueID,
		Status:           string(coreworkflow.RunStatusRunning),
		CreatedBy:        cmd.UserID,
		StartedAt:        &now,
	})
	if err != nil {
		return nil, nil, err
	}
	stepsIn := make([]coreworkflow.CreateStepRunInput, len(def.Steps))
	for i := range def.Steps {
		target := def.Steps[i].TargetAgentID
		agent := agents[target]
		stepsIn[i] = coreworkflow.CreateStepRunInput{
			StepID:            def.Steps[i].StepID,
			StepIndex:         i,
			StepType:          def.Steps[i].Type,
			TargetAgentID:     &target,
			AgentName:         agent.Name,
			AgentDescription:  agent.Description,
			AgentInstructions: agent.Instructions,
			AgentRevision:     agent.Revision,
			Prompt:            def.Steps[i].Prompt,
			Status:            string(coreworkflow.StepRunStatusPending),
		}
	}
	stepRuns, err := s.Workflows.CreateWorkflowStepRuns(ctx, run.ID, stepsIn)
	if err != nil {
		return nil, nil, err
	}
	if _, err := s.dispatchNextStep(ctx, cmd.SpaceID, cmd.UserID, run, stepRuns); err != nil {
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

func (s *Service) HandleTaskRunTerminal(ctx context.Context, info coretask.RunTerminalInfo) error {
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
	if info.Status == string(coretask.RunStatusSucceeded) {
		summary := summarizeOutput(info.Output)
		applied, err := s.Workflows.TransitionWorkflowStepRun(ctx, coreworkflow.TransitionStepRunInput{
			StepRunID:      stepRun.ID,
			ExpectedStatus: coreworkflow.StepRunStatusRunning,
			NewStatus:      coreworkflow.StepRunStatusSucceeded,
			TaskRunID:      &info.TaskRunID,
			OutputSummary:  summary,
			EndedAt:        &now,
		})
		if err != nil {
			return err
		}
		if !applied {
			// The step was no longer running -- a concurrent cancel already
			// finished it and the run. Nothing more to dispatch.
			return nil
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
	stepStatus := coreworkflow.StepRunStatusFailed
	runStatus := coreworkflow.RunStatusFailed
	if info.Status == string(coretask.RunStatusCanceled) {
		stepStatus = coreworkflow.StepRunStatusCanceled
		runStatus = coreworkflow.RunStatusCanceled
	}
	// One transaction ends the run: the step goes terminal, every later step
	// still pending is blocked, and the run goes terminal -- so a crash cannot
	// leave a failed step under a run that still reads as running.
	_, err = s.Workflows.FinalizeFailedWorkflowRun(ctx, coreworkflow.FinalizeFailedRunInput{
		WorkflowRunID: run.ID,
		StepRunID:     stepRun.ID,
		StepIndex:     stepRun.StepIndex,
		StepExpected:  coreworkflow.StepRunStatusRunning,
		StepStatus:    stepStatus,
		RunExpected:   coreworkflow.RunStatusRunning,
		RunStatus:     runStatus,
		TaskRunID:     &info.TaskRunID,
		ErrorMessage:  info.ErrorMessage,
		EndedAt:       &now,
	})
	return err
}

func (s *Service) dispatchNextStep(ctx context.Context, spaceID, userID string, run *coreworkflow.Run, steps []coreworkflow.StepRun) (*coreworkflow.StepRun, error) {
	for i := range steps {
		if steps[i].Status != string(coreworkflow.StepRunStatusPending) {
			continue
		}
		if spaceID == "" {
			workflow, err := s.Workflows.GetWorkflow(ctx, run.WorkflowID)
			if err != nil {
				return nil, err
			}
			if workflow == nil {
				return nil, ErrWorkflowNotFound
			}
			spaceID = workflow.SpaceID
		}
		startedAt := time.Now().UTC()
		taskItem, taskRunID, err := s.createStepTask(ctx, spaceID, userID, steps[i])
		if err != nil {
			// The step never started, so it fails from pending and the run ends
			// with it -- one transaction, the same path a running step's failure
			// takes.
			_, _ = s.Workflows.FinalizeFailedWorkflowRun(ctx, coreworkflow.FinalizeFailedRunInput{
				WorkflowRunID: run.ID,
				StepRunID:     steps[i].ID,
				StepIndex:     steps[i].StepIndex,
				StepExpected:  coreworkflow.StepRunStatusPending,
				StepStatus:    coreworkflow.StepRunStatusFailed,
				RunExpected:   coreworkflow.RunStatusRunning,
				RunStatus:     coreworkflow.RunStatusFailed,
				ErrorMessage:  ptrError(err),
				StartedAt:     &startedAt,
				EndedAt:       &startedAt,
			})
			return nil, err
		}
		if _, err := s.Workflows.TransitionWorkflowStepRun(ctx, coreworkflow.TransitionStepRunInput{
			StepRunID:      steps[i].ID,
			ExpectedStatus: coreworkflow.StepRunStatusPending,
			NewStatus:      coreworkflow.StepRunStatusRunning,
			TaskID:         &taskItem.ID,
			TaskRunID:      &taskRunID,
			StartedAt:      &startedAt,
		}); err != nil {
			return nil, err
		}
		return &steps[i], nil
	}
	endedAt := time.Now().UTC()
	if _, err := s.Workflows.TransitionWorkflowRun(ctx, coreworkflow.TransitionRunInput{
		WorkflowRunID:  run.ID,
		ExpectedStatus: coreworkflow.RunStatusRunning,
		NewStatus:      coreworkflow.RunStatusSucceeded,
		EndedAt:        &endedAt,
	}); err != nil {
		return nil, err
	}
	return nil, nil
}

// stepAgent returns the agent definition a step must run with. Steps recorded since
// runs snapshot their agent carry it on the step run itself, so an edit to the agent
// while the run is in flight cannot change what a later step sends. Steps written
// before that fall back to the agent definition as it stands now, deleted or not:
// the run was authorized when it started, and refusing to finish it because the
// agent has since been deleted would strand it half done.
func (s *Service) stepAgent(ctx context.Context, spaceID, agentID string, step coreworkflow.StepRun) (*agentdef.Agent, error) {
	if step.AgentName != "" || step.AgentInstructions != "" {
		return &agentdef.Agent{
			ID:           agentID,
			SpaceID:      spaceID,
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
	if agent == nil || agent.SpaceID != spaceID {
		return nil, ErrInvalidTargetAgent
	}
	return agent, nil
}

func (s *Service) createStepTask(ctx context.Context, spaceID, userID string, step coreworkflow.StepRun) (*coretask.Task, string, error) {
	agentID := ""
	if step.TargetAgentID != nil {
		agentID = *step.TargetAgentID
	}
	if agentID == "" {
		return nil, "", ErrInvalidTargetAgent
	}
	agent, err := s.stepAgent(ctx, spaceID, agentID, step)
	if err != nil {
		return nil, "", err
	}
	input := buildWorkflowTaskInput(agent, step.Prompt)
	taskItem, err := s.TaskService.CreateTask(ctx, task.CreateTaskCmd{
		UserID:        userID,
		SpaceID:       spaceID,
		Input:         input,
		AgentID:       &agentID,
		CreatedByType: coretask.RunCreatedByTypeUser,
		TriggerSource: coretask.RunTriggerSourceWorkflowStep,
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

func (s *Service) validateIssueForRun(ctx context.Context, spaceID, workflowID string, issueID *string) error {
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
	if issue == nil || issue.SpaceID != spaceID {
		return ErrIssueNotFound
	}
	if issue.ExecutorKind == nil || issue.ExecutorID == nil || *issue.ExecutorKind != coreissue.ExecutorWorkflow || *issue.ExecutorID != workflowID {
		return ErrIssueWorkflowMismatch
	}
	return nil
}

// parseAndValidateDefinition parses raw, checks every step's target agent, and returns
// the resolved agents keyed by agent ID so a caller can snapshot them.
func (s *Service) parseAndValidateDefinition(ctx context.Context, spaceID, raw string) (*coreworkflow.Definition, map[string]agentdef.Agent, error) {
	def, err := parseDefinition(raw)
	if err != nil {
		return nil, nil, err
	}
	agents, err := s.resolveDefinitionAgents(ctx, spaceID, def)
	if err != nil {
		return nil, nil, err
	}
	return def, agents, nil
}

// parseDefinition unmarshals raw JSON into a WorkflowDefinition and validates structural fields
// (step count, unique IDs, required type/agent/prompt). Does not touch the database.
func parseDefinition(raw string) (*coreworkflow.Definition, error) {
	var def coreworkflow.Definition
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
		if step.Type != coreworkflow.StepTypeAgentTask {
			return nil, ErrInvalidStepType
		}
		if step.TargetAgentID == "" || step.Prompt == "" {
			return nil, ErrInvalidDefinition
		}
	}
	return &def, nil
}

// resolveDefinitionAgents checks that every step's target agent is live and belongs
// to spaceID, and returns those agents keyed by agent ID.
//
// A deleted agent is refused. This runs when a workflow is written and again when a
// run starts, so a plan cannot take a new dependency on a deleted agent, and a
// workflow that lost one is refused at the start of a run rather than partway
// through it.
func (s *Service) resolveDefinitionAgents(ctx context.Context, spaceID string, def *coreworkflow.Definition) (map[string]agentdef.Agent, error) {
	if s.Agents == nil {
		return nil, ErrInvalidTargetAgent
	}
	agents := make(map[string]agentdef.Agent, len(def.Steps))
	for i := range def.Steps {
		agentID := def.Steps[i].TargetAgentID
		if _, ok := agents[agentID]; ok {
			continue
		}
		agent, err := s.Agents.GetAgent(ctx, agentID)
		if err != nil {
			return nil, err
		}
		if agent == nil || agent.SpaceID != spaceID {
			return nil, ErrInvalidTargetAgent
		}
		agents[agentID] = *agent
	}
	return agents, nil
}

func isValidWorkflowStatus(status string) bool {
	switch status {
	case coreworkflow.StatusDraft, coreworkflow.StatusPublished, coreworkflow.StatusArchived:
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

func buildWorkflowTaskInput(agent *agentdef.Agent, prompt string) string {
	base := fmt.Sprintf("Agent: %s\nDescription: %s\nInstructions:\n%s", agent.Name, agent.Description, agent.Instructions)
	if strings.TrimSpace(prompt) == "" {
		return base
	}
	return base + "\n\n" + prompt
}
