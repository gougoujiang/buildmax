package model

const (
	// TeamRoleOwner is the initial role for the user who creates a team.
	TeamRoleOwner = "owner"
	// TeamRoleAdmin can manage shared automation assets but not membership ownership.
	TeamRoleAdmin = "admin"
	// TeamRoleMember is the basic collaboration role for invited members.
	TeamRoleMember = "member"
	// DefaultPersonalTeamName is the initial UX-facing name for a user's own space.
	DefaultPersonalTeamName = "My Space"
)

const (
	IssueStatusTodo       = "todo"
	IssueStatusInProgress = "in_progress"
	IssueStatusDone       = "done"
	IssueAssigneePerson   = "person"
	IssueAssigneeAgent    = "agent"
	IssueAssigneeWorkflow = "workflow"
)

type CreateIssueInput struct {
	Title       string
	Description string
}

type UpdateIssueInput struct {
	Title        *string
	Description  *string
	Status       *string
	AssigneeKind *string
	AssigneeID   *string
}

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

const (
	RunCreatedByTypeUser    = "user"
	RunCreatedByTypeWebhook = "webhook"
	RunCreatedByTypeSystem  = "system"
)

const (
	RunTriggerSourceTaskCreate         = "task_create"
	RunTriggerSourceTaskRerun          = "task_rerun"
	RunTriggerSourcePortalConversation = "portal_conversation"
	RunTriggerSourcePortalTaskCreate   = "portal_task_create"
	RunTriggerSourcePortalTaskRerun    = "portal_task_rerun"
	RunTriggerSourceIssueAgentRun      = "issue_agent_run"
	RunTriggerSourceWorkflowStep       = "workflow_step"
	RunTriggerSourceWebhook            = "webhook"
)

// CreateTaskInput is the input for CreateTask.
type CreateTaskInput struct {
	ConversationID          string
	TeamID                  string
	Input                   string
	Title                   string
	CreatedBy               string
	InitialRunCreatedBy     string
	InitialRunCreatedByType string
	InitialRunTriggerSource string
	TitlePromptTokens       int
	TitleCompletionTokens   int
	AgentID                 *string
	IssueID                 *string
}
