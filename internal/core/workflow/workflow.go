package workflow

import (
	"context"
	"time"
)

const (
	StatusDraft     = "draft"
	StatusPublished = "published"
	StatusArchived  = "archived"

	RunStatusPending   = "pending"
	RunStatusRunning   = "running"
	RunStatusSucceeded = "succeeded"
	RunStatusFailed    = "failed"
	RunStatusCanceled  = "canceled"

	StepTypeAgentTask = "agent_task"

	StepRunStatusPending   = "pending"
	StepRunStatusRunning   = "running"
	StepRunStatusSucceeded = "succeeded"
	StepRunStatusFailed    = "failed"
	StepRunStatusCanceled  = "canceled"
	StepRunStatusBlocked   = "blocked"
)

// Workflow is a reusable team-scoped execution plan.
type Workflow struct {
	ID          string `json:"id"`
	TeamID      string `json:"team_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Definition  string `json:"definition"`
	Status      string `json:"status"`
	// Revision numbers the workflow_revision row holding this content. It
	// starts at 1 and advances every time the name, description, definition,
	// or status changes.
	Revision  int       `json:"revision"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Revision is one recorded version of a workflow.
//
// Revisions are append-only: an edit adds one, nothing rewrites or deletes one,
// and restoring an older revision is itself an edit that appends a new one.
type Revision struct {
	WorkflowID  string    `json:"workflow_id"`
	Revision    int       `json:"revision"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Definition  string    `json:"definition"`
	Status      string    `json:"status"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

// Run is one execution attempt of a workflow.
type Run struct {
	ID         string `json:"id"`
	WorkflowID string `json:"workflow_id"`
	// WorkflowRevision is the workflow revision this run expanded. It is 0 for
	// runs started before workflows recorded revisions.
	WorkflowRevision int        `json:"workflow_revision,omitempty"`
	IssueID          *string    `json:"issue_id,omitempty"`
	ConversationID   string     `json:"conversation_id"`
	Status           string     `json:"status"`
	CreatedBy        string     `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	EndedAt          *time.Time `json:"ended_at,omitempty"`
	ErrorMessage     *string    `json:"error_message,omitempty"`
}

// StepRun is one durable step execution record under a workflow run.
type StepRun struct {
	ID            string  `json:"id"`
	WorkflowRunID string  `json:"workflow_run_id"`
	StepID        string  `json:"step_id"`
	StepIndex     int     `json:"step_index"`
	StepType      string  `json:"step_type"`
	TargetAgentID *string `json:"target_agent_id,omitempty"`
	// AgentName, AgentDescription, and AgentInstructions capture the target agent
	// definition as it was when the run started, so later edits to the agent cannot
	// change what a step in flight sends to the model.
	AgentName         string     `json:"agent_name,omitempty"`
	AgentDescription  string     `json:"agent_description,omitempty"`
	AgentInstructions string     `json:"agent_instructions,omitempty"`
	AgentRevision     int        `json:"agent_revision,omitempty"`
	Prompt            string     `json:"prompt"`
	Status            string     `json:"status"`
	TaskID            *string    `json:"task_id,omitempty"`
	TaskRunID         *string    `json:"task_run_id,omitempty"`
	OutputSummary     *string    `json:"output_summary,omitempty"`
	ErrorMessage      *string    `json:"error_message,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	EndedAt           *time.Time `json:"ended_at,omitempty"`
}

// Definition is the parsed structure of a workflow definition JSON.
type Definition struct {
	Steps []DefinitionStep `json:"steps"`
}

// DefinitionStep describes one step in a workflow definition.
type DefinitionStep struct {
	StepID        string `json:"step_id"`
	Type          string `json:"type"`
	TargetAgentID string `json:"target_agent_id"`
	Prompt        string `json:"prompt"`
}

type CreateRunInput struct {
	WorkflowID       string
	WorkflowRevision int
	IssueID          *string
	ConversationID   string
	Status           string
	CreatedBy        string
	StartedAt        *time.Time
}

type UpdateInput struct {
	Name        *string
	Description *string
	Definition  *string
	Status      *string
	// UpdatedBy is recorded as the author of the revision this update appends.
	UpdatedBy string
}

type CreateStepRunInput struct {
	StepID            string
	StepIndex         int
	StepType          string
	TargetAgentID     *string
	AgentName         string
	AgentDescription  string
	AgentInstructions string
	AgentRevision     int
	Prompt            string
	Status            string
}

type UpdateRunInput struct {
	Status       string
	StartedAt    *time.Time
	EndedAt      *time.Time
	ErrorMessage *string
}

type UpdateStepRunInput struct {
	Status        *string
	TaskID        *string
	TaskRunID     *string
	OutputSummary *string
	ErrorMessage  *string
	StartedAt     *time.Time
	EndedAt       *time.Time
}

// Store provides workflow and workflow execution persistence.
type Store interface {
	ListWorkflowsByTeam(ctx context.Context, teamID string) ([]Workflow, error)
	CreateWorkflow(ctx context.Context, teamID, createdBy, name, description, definition string) (*Workflow, error)
	GetWorkflow(ctx context.Context, workflowID string) (*Workflow, error)
	UpdateWorkflow(ctx context.Context, workflowID, teamID string, in UpdateInput) (*Workflow, error)
	CreateWorkflowRun(ctx context.Context, in CreateRunInput) (*Run, error)
	ListWorkflowRunsByWorkflow(ctx context.Context, workflowID string, limit, offset int) ([]Run, int, error)
	ListWorkflowRunsByIssue(ctx context.Context, issueID string, limit, offset int) ([]Run, int, error)
	GetWorkflowRun(ctx context.Context, workflowRunID string) (*Run, error)
	ListWorkflowStepRuns(ctx context.Context, workflowRunID string) ([]StepRun, error)
	CreateWorkflowStepRuns(ctx context.Context, workflowRunID string, steps []CreateStepRunInput) ([]StepRun, error)
	UpdateWorkflowRun(ctx context.Context, workflowRunID string, in UpdateRunInput) (*Run, error)
	UpdateWorkflowStepRun(ctx context.Context, stepRunID string, in UpdateStepRunInput) (*StepRun, error)
	GetWorkflowStepRunByTaskID(ctx context.Context, taskID string) (*StepRun, error)
	GetWorkflowStepRunByTaskRunID(ctx context.Context, taskRunID string) (*StepRun, error)
	// ListWorkflowRevisions returns a workflow's revisions, newest first, with
	// the total count.
	ListWorkflowRevisions(ctx context.Context, workflowID string, limit, offset int) ([]Revision, int, error)
	// GetWorkflowRevision returns one revision, or nil when the workflow has no
	// such revision number.
	GetWorkflowRevision(ctx context.Context, workflowID string, revision int) (*Revision, error)
}
