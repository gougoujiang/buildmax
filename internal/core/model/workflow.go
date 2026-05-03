package model

import "context"

const (
	WorkflowStatusDraft     = "draft"
	WorkflowStatusPublished = "published"
	WorkflowStatusArchived  = "archived"

	WorkflowRunStatusPending   = "pending"
	WorkflowRunStatusRunning   = "running"
	WorkflowRunStatusSucceeded = "succeeded"
	WorkflowRunStatusFailed    = "failed"
	WorkflowRunStatusCanceled  = "canceled"

	WorkflowStepTypeAgentTask = "agent_task"

	WorkflowStepRunStatusPending   = "pending"
	WorkflowStepRunStatusRunning   = "running"
	WorkflowStepRunStatusSucceeded = "succeeded"
	WorkflowStepRunStatusFailed    = "failed"
	WorkflowStepRunStatusBlocked   = "blocked"
)

// Workflow is a reusable team-scoped execution plan.
type Workflow struct {
	ID          uint   `json:"-"`
	WorkflowID  string `json:"workflow_id"`
	TeamID      string `json:"team_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Definition  string `json:"definition"`
	Status      string `json:"status"`
	CreatedBy   string `json:"created_by"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// WorkflowRun is one execution attempt of a workflow.
type WorkflowRun struct {
	ID             uint    `json:"-"`
	WorkflowRunID  string  `json:"workflow_run_id"`
	WorkflowID     string  `json:"workflow_id"`
	IssueID        *string `json:"issue_id,omitempty"`
	ConversationID string  `json:"conversation_id"`
	Status         string  `json:"status"`
	CreatedBy      string  `json:"created_by"`
	CreatedAt      int64   `json:"created_at"`
	StartedAt      *int64  `json:"started_at,omitempty"`
	EndedAt        *int64  `json:"ended_at,omitempty"`
	ErrorMessage   *string `json:"error_message,omitempty"`
}

// WorkflowStepRun is one durable step execution record under a workflow run.
type WorkflowStepRun struct {
	ID            uint    `json:"-"`
	StepRunID     string  `json:"workflow_step_run_id"`
	WorkflowRunID string  `json:"workflow_run_id"`
	StepID        string  `json:"step_id"`
	StepIndex     int     `json:"step_index"`
	StepType      string  `json:"step_type"`
	TargetAgentID *string `json:"target_agent_id,omitempty"`
	Prompt        string  `json:"prompt"`
	Status        string  `json:"status"`
	TaskID        *string `json:"task_id,omitempty"`
	TaskRunID     *string `json:"task_run_id,omitempty"`
	OutputSummary *string `json:"output_summary,omitempty"`
	ErrorMessage  *string `json:"error_message,omitempty"`
	CreatedAt     int64   `json:"created_at"`
	StartedAt     *int64  `json:"started_at,omitempty"`
	EndedAt       *int64  `json:"ended_at,omitempty"`
}

// WorkflowDefinition is the parsed structure of a workflow definition JSON.
type WorkflowDefinition struct {
	Steps []WorkflowDefinitionStep `json:"steps"`
}

// WorkflowDefinitionStep describes one step in a workflow definition.
type WorkflowDefinitionStep struct {
	StepID        string `json:"step_id"`
	Type          string `json:"type"`
	TargetAgentID string `json:"target_agent_id"`
	Prompt        string `json:"prompt"`
}

type CreateWorkflowRunInput struct {
	WorkflowID     string
	IssueID        *string
	ConversationID string
	Status         string
	CreatedBy      string
	StartedAt      *int64
}

type UpdateWorkflowInput struct {
	Name        *string
	Description *string
	Definition  *string
	Status      *string
}

type CreateWorkflowStepRunInput struct {
	StepID        string
	StepIndex     int
	StepType      string
	TargetAgentID *string
	Prompt        string
	Status        string
}

type UpdateWorkflowRunInput struct {
	Status       string
	StartedAt    *int64
	EndedAt      *int64
	ErrorMessage *string
}

type UpdateWorkflowStepRunInput struct {
	Status        *string
	TaskID        *string
	TaskRunID     *string
	OutputSummary *string
	ErrorMessage  *string
	StartedAt     *int64
	EndedAt       *int64
}

// WorkflowStore provides workflow and workflow execution persistence.
type WorkflowStore interface {
	ListWorkflowsByTeam(ctx context.Context, teamID string) ([]Workflow, error)
	CreateWorkflow(ctx context.Context, teamID, createdBy, name, description, definition string) (*Workflow, error)
	GetWorkflow(ctx context.Context, workflowID string) (*Workflow, error)
	UpdateWorkflow(ctx context.Context, workflowID, teamID string, in UpdateWorkflowInput) (*Workflow, error)
	CreateWorkflowRun(ctx context.Context, in CreateWorkflowRunInput) (*WorkflowRun, error)
	ListWorkflowRunsByWorkflow(ctx context.Context, workflowID string, limit, offset int) ([]WorkflowRun, int, error)
	ListWorkflowRunsByIssue(ctx context.Context, issueID string, limit, offset int) ([]WorkflowRun, int, error)
	GetWorkflowRun(ctx context.Context, workflowRunID string) (*WorkflowRun, error)
	ListWorkflowStepRuns(ctx context.Context, workflowRunID string) ([]WorkflowStepRun, error)
	CreateWorkflowStepRuns(ctx context.Context, workflowRunID string, steps []CreateWorkflowStepRunInput) ([]WorkflowStepRun, error)
	UpdateWorkflowRun(ctx context.Context, workflowRunID string, in UpdateWorkflowRunInput) (*WorkflowRun, error)
	UpdateWorkflowStepRun(ctx context.Context, stepRunID string, in UpdateWorkflowStepRunInput) (*WorkflowStepRun, error)
	GetWorkflowStepRunByTaskID(ctx context.Context, taskID string) (*WorkflowStepRun, error)
	GetWorkflowStepRunByTaskRunID(ctx context.Context, taskRunID string) (*WorkflowStepRun, error)
}
