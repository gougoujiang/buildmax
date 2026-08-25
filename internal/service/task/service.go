package task

import (
	"context"
	"fmt"
	"github.com/gougoujiang/buildmax/internal/core/apierr"

	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/util"
)

const defaultTitleRunes = 50

var (
	ErrInputRequired         = apierr.New(apierr.KindInvalid, "input required")
	ErrAgentsNotConfigured   = apierr.New(apierr.KindNotConfigured, "agents not configured")
	ErrTasksNotConfigured    = apierr.New(apierr.KindNotConfigured, "tasks not configured")
	ErrTaskRunsNotConfigured = apierr.New(apierr.KindNotConfigured, "task runs not configured")
	ErrAgentNotFound         = apierr.New(apierr.KindInvalid, "agent not found")
	ErrTaskNotFound          = apierr.New(apierr.KindNotFound, "task not found")
	// ErrNoRunToRetry means the task has no finished run to repeat: it has
	// never run, or its only run is still in flight.
	ErrNoRunToRetry = apierr.New(apierr.KindConflict, "this task has no finished run to retry")
	// ErrRetryOfWorkflowStep means the task belongs to a workflow step. The
	// workflow owns that task's lifecycle — it reacts to the step run's
	// outcome — so a run started behind its back would mark a settled step
	// succeeded and dispatch the next step of a workflow run that is already
	// over.
	ErrRetryOfWorkflowStep = apierr.New(apierr.KindConflict, "this run belongs to a workflow step and cannot be retried on its own")
)

// WorkflowStepLookup answers whether a task is a workflow step's task. It is
// optional: a deployment with no workflow store has no workflow steps, so a nil
// lookup means nothing to protect rather than an unanswered question.
type WorkflowStepLookup interface {
	GetWorkflowStepRunByTaskID(ctx context.Context, taskID string) (*model.WorkflowStepRun, error)
}

// QuotaChecker is the narrow quota surface needed by task workflows.
type QuotaChecker interface {
	Check(ctx context.Context, teamID string, runsToAdd, tokensToAdd int) (allowed bool, reason string)
}

// Service owns task-related application workflows.
type Service struct {
	Agents         model.AgentStore
	Tasks          model.TaskStore
	TaskRuns       model.TaskRunStore
	QuotaChecker   QuotaChecker
	TitleGenerator llm.TitleGenerator
	// WorkflowSteps is only consulted by RetryRun. Callers that never retry
	// leave it nil.
	WorkflowSteps WorkflowStepLookup
}

// CreateTaskCmd creates a new task and its first run.
type CreateTaskCmd struct {
	ConversationID string
	UserID         string
	TeamID         string
	Input          string
	AgentID        *string
	IssueID        *string
	CreatedByType  string
	TriggerSource  string
	// SourceMessageID names the conversation message that asked for this task.
	SourceMessageID *string
}

// CreateRunCmd creates a new run on an existing task.
type CreateRunCmd struct {
	UserID        string
	TaskID        string
	Input         string
	CreatedByType string
	TriggerSource string
	// RetryOfTaskRunID names the run this one repeats, when it repeats one.
	RetryOfTaskRunID *string
	// SourceMessageID names the conversation message that asked for this run.
	SourceMessageID *string
}

// RetryRunCmd repeats a task's most recent run.
type RetryRunCmd struct {
	UserID string
	TaskID string
}

// RetryResult reports the new run and the one it repeats.
type RetryResult struct {
	Run        *model.TaskRun
	RetriedRun model.TaskRun
}

// StartBackgroundTaskResult is returned when a background task is created.
type StartBackgroundTaskResult struct {
	TaskID string
	RunID  string
}

// CreateTask resolves input, applies title/quota rules, and persists a new task.
func (s *Service) CreateTask(ctx context.Context, cmd CreateTaskCmd) (*model.Task, error) {
	if s.Tasks == nil {
		return nil, ErrTasksNotConfigured
	}
	input, agentID, err := s.resolveInput(ctx, cmd.TeamID, cmd.UserID, cmd.Input, cmd.AgentID)
	if err != nil {
		return nil, err
	}
	createdByType, triggerSource := normalizeCreateTaskProvenance(cmd.CreatedByType, cmd.TriggerSource)
	title, promptTokens, completionTokens := s.resolveTitle(ctx, input)
	if err := s.checkQuota(ctx, cmd.TeamID, promptTokens+completionTokens); err != nil {
		return nil, err
	}
	return s.Tasks.CreateTask(ctx, &model.CreateTaskInput{
		ConversationID:            cmd.ConversationID,
		TeamID:                    cmd.TeamID,
		Input:                     input,
		Title:                     title,
		CreatedBy:                 cmd.UserID,
		InitialRunCreatedBy:       cmd.UserID,
		InitialRunCreatedByType:   createdByType,
		InitialRunTriggerSource:   triggerSource,
		InitialRunSourceMessageID: cmd.SourceMessageID,
		TitlePromptTokens:         promptTokens,
		TitleCompletionTokens:     completionTokens,
		AgentID:                   agentID,
		IssueID:                   cmd.IssueID,
	})
}

// CreateRun enforces basic run creation rules and delegates to TaskRunStore.
func (s *Service) CreateRun(ctx context.Context, cmd CreateRunCmd) (*model.TaskRun, error) {
	if s.TaskRuns == nil {
		return nil, ErrTaskRunsNotConfigured
	}
	if cmd.Input == "" {
		return nil, ErrInputRequired
	}
	if s.QuotaChecker != nil && s.Tasks != nil {
		task, err := s.Tasks.GetTask(ctx, cmd.TaskID)
		if err != nil {
			return nil, err
		}
		if task != nil {
			if err := s.checkQuota(ctx, task.TeamID, 0); err != nil {
				return nil, err
			}
		}
	}
	createdByType, triggerSource := normalizeCreateRunProvenance(cmd.CreatedByType, cmd.TriggerSource)
	return s.TaskRuns.CreateTaskRun(ctx, model.CreateTaskRunInput{
		TaskID:           cmd.TaskID,
		Input:            cmd.Input,
		CreatedBy:        cmd.UserID,
		CreatedByType:    createdByType,
		TriggerSource:    triggerSource,
		RetryOfTaskRunID: cmd.RetryOfTaskRunID,
		SourceMessageID:  cmd.SourceMessageID,
	})
}

// RetryRun repeats a task's most recent run with the same input.
//
// The input comes from the run rather than the task because a task's later runs
// can carry follow-up instructions, and retrying means running that again — not
// running whatever the task was first asked to do.
//
// A run still in flight is not retried: one task holds at most one active run,
// and the answer to "it is taking too long" is to stop it first.
func (s *Service) RetryRun(ctx context.Context, cmd RetryRunCmd) (*RetryResult, error) {
	if s.TaskRuns == nil {
		return nil, ErrTaskRunsNotConfigured
	}
	if s.Tasks == nil {
		return nil, ErrTasksNotConfigured
	}
	target, err := s.Tasks.GetTask(ctx, cmd.TaskID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, ErrTaskNotFound
	}
	if err := s.refuseWorkflowStepRetry(ctx, cmd.TaskID); err != nil {
		return nil, err
	}
	// Ask about an in-flight run before looking at the last finished one. The
	// store refuses a second active run anyway, but a task whose current run is
	// still going has a more useful answer than "nothing to retry" — and while
	// it runs, last_run_id still names the run before it.
	active, err := s.TaskRuns.GetActiveTaskRunByTask(ctx, cmd.TaskID)
	if err != nil {
		return nil, err
	}
	if active != nil {
		return nil, model.ErrRunInProgress
	}
	if target.LastRunID == nil {
		return nil, ErrNoRunToRetry
	}
	previous, err := s.TaskRuns.GetTaskRun(ctx, *target.LastRunID)
	if err != nil {
		return nil, err
	}
	if previous == nil || !model.RunStatusTerminal(previous.Status) {
		return nil, ErrNoRunToRetry
	}
	run, err := s.CreateRun(ctx, CreateRunCmd{
		UserID:           cmd.UserID,
		TaskID:           cmd.TaskID,
		Input:            previous.Input,
		CreatedByType:    model.RunCreatedByTypeUser,
		TriggerSource:    model.RunTriggerSourceTaskRetry,
		RetryOfTaskRunID: &previous.ID,
	})
	if err != nil {
		return nil, err
	}
	return &RetryResult{Run: run, RetriedRun: *previous}, nil
}

func (s *Service) refuseWorkflowStepRetry(ctx context.Context, taskID string) error {
	if s.WorkflowSteps == nil {
		return nil
	}
	step, err := s.WorkflowSteps.GetWorkflowStepRunByTaskID(ctx, taskID)
	if err != nil {
		return err
	}
	if step != nil {
		return ErrRetryOfWorkflowStep
	}
	return nil
}

// StartBackgroundTask creates a task and returns its task/run ids.
func (s *Service) StartBackgroundTask(ctx context.Context, cmd CreateTaskCmd) (*StartBackgroundTaskResult, error) {
	task, err := s.CreateTask(ctx, cmd)
	if err != nil {
		return nil, err
	}
	runID := ""
	if task.LastRunID != nil {
		runID = *task.LastRunID
	}
	return &StartBackgroundTaskResult{
		TaskID: task.ID,
		RunID:  runID,
	}, nil
}

func (s *Service) resolveInput(ctx context.Context, teamID, userID, input string, agentID *string) (string, *string, error) {
	if agentID == nil || *agentID == "" {
		if input == "" {
			return "", nil, ErrInputRequired
		}
		return input, nil, nil
	}
	if s.Agents == nil {
		return "", nil, ErrAgentsNotConfigured
	}
	// With an input the caller already rendered, the agent is provenance and a
	// deleted one still names it truthfully — that is how a workflow run whose
	// agent was deleted mid-flight finishes its remaining steps. With no input
	// the agent is the source of the prompt, so it has to be live.
	var agent *model.Agent
	var err error
	if input != "" {
		agent, err = s.Agents.GetAgentIncludingDeleted(ctx, *agentID)
	} else {
		agent, err = s.Agents.GetAgent(ctx, *agentID)
	}
	if err != nil {
		return "", nil, err
	}
	if agent == nil || agent.TeamID != teamID {
		return "", nil, ErrAgentNotFound
	}
	if input != "" {
		return input, agentID, nil
	}
	return buildTaskInputFromAgent(agent, ""), agentID, nil
}

func (s *Service) resolveTitle(ctx context.Context, input string) (string, int, int) {
	title := truncateTaskTitle(input, defaultTitleRunes)
	if s.TitleGenerator == nil {
		return title, 0, 0
	}
	genTitle, promptTokens, completionTokens, err := s.TitleGenerator.GenerateTitle(ctx, input)
	if err != nil || genTitle == "" {
		return title, 0, 0
	}
	return genTitle, promptTokens, completionTokens
}

func (s *Service) checkQuota(ctx context.Context, teamID string, tokens int) error {
	if s.QuotaChecker == nil {
		return nil
	}
	if teamID == "" {
		return nil
	}
	allowed, reason := s.QuotaChecker.Check(ctx, teamID, 1, tokens)
	if allowed {
		return nil
	}
	// The quota service's reason is already the whole sentence a caller should
	// read, so it is the message rather than a detail appended to one. The Kind
	// is what carries the 429; no transport needs to know this package's types.
	return apierr.New(apierr.KindQuotaExceeded, reason)
}

func buildTaskInputFromAgent(agent *model.Agent, userInput string) string {
	out := fmt.Sprintf("Agent: %s\nDescription: %s\nInstructions:\n%s", agent.Name, agent.Description, agent.Instructions)
	if userInput != "" {
		out = out + "\n\n" + userInput
	}
	return out
}

func truncateTaskTitle(input string, maxRunes int) string {
	return util.TruncateRunes(input, maxRunes)
}

func normalizeCreateTaskProvenance(createdByType, triggerSource string) (string, string) {
	if createdByType == "" {
		createdByType = model.RunCreatedByTypeUser
	}
	if triggerSource == "" {
		triggerSource = model.RunTriggerSourceTaskCreate
	}
	return createdByType, triggerSource
}

func normalizeCreateRunProvenance(createdByType, triggerSource string) (string, string) {
	if createdByType == "" {
		createdByType = model.RunCreatedByTypeUser
	}
	if triggerSource == "" {
		triggerSource = model.RunTriggerSourceTaskRerun
	}
	return createdByType, triggerSource
}
