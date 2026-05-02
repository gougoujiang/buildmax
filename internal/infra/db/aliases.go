package db

import "buildmax/internal/core/model"

type (
	Agent                      = model.Agent
	AgentStore                 = model.AgentStore
	ArtifactWithTask           = model.ArtifactWithTask
	ClaimTaskInput             = model.ClaimTaskInput
	ClaimTaskRunInput          = model.ClaimTaskRunInput
	Conversation               = model.Conversation
	ConversationMessage        = model.ConversationMessage
	ConversationMessageStore   = model.ConversationMessageStore
	ConversationStore          = model.ConversationStore
	CreateIssueInput           = model.CreateIssueInput
	CreateTaskInput            = model.CreateTaskInput
	CreateWorkflowRunInput     = model.CreateWorkflowRunInput
	CreateWorkflowStepRunInput = model.CreateWorkflowStepRunInput
	Issue                      = model.Issue
	IssueStore                 = model.IssueStore
	QuotaTier                  = model.QuotaTier
	QuotaTierStore             = model.QuotaTierStore
	RunStatus                  = model.RunStatus
	Task                       = model.Task
	TaskRun                    = model.TaskRun
	TaskRunArtifact            = model.TaskRunArtifact
	TaskRunStore               = model.TaskRunStore
	TaskStore                  = model.TaskStore
	Team                       = model.Team
	TeamMember                 = model.TeamMember
	TeamStore                  = model.TeamStore
	UpdateIssueInput           = model.UpdateIssueInput
	UpdateTaskInput            = model.UpdateTaskInput
	UpdateTaskRunInput         = model.UpdateTaskRunInput
	UpdateWorkflowInput        = model.UpdateWorkflowInput
	UpdateWorkflowRunInput     = model.UpdateWorkflowRunInput
	UpdateWorkflowStepRunInput = model.UpdateWorkflowStepRunInput
	UsageInWindowReader        = model.UsageInWindowReader
	User                       = model.User
	UserStore                  = model.UserStore
	UserWebhookKey             = model.UserWebhookKey
	UserWebhookKeyStore        = model.UserWebhookKeyStore
	WebhookKeyMeta             = model.WebhookKeyMeta
	Workflow                   = model.Workflow
	WorkflowRun                = model.WorkflowRun
	WorkflowStepRun            = model.WorkflowStepRun
	WorkflowStore              = model.WorkflowStore
)

const (
	DefaultPersonalTeamName            = model.DefaultPersonalTeamName
	IssueAssigneeAgent                 = model.IssueAssigneeAgent
	IssueAssigneePerson                = model.IssueAssigneePerson
	IssueAssigneeWorkflow              = model.IssueAssigneeWorkflow
	IssueStatusDone                    = model.IssueStatusDone
	IssueStatusInProgress              = model.IssueStatusInProgress
	IssueStatusTodo                    = model.IssueStatusTodo
	RunCreatedByTypeSystem             = model.RunCreatedByTypeSystem
	RunCreatedByTypeUser               = model.RunCreatedByTypeUser
	RunCreatedByTypeWebhook            = model.RunCreatedByTypeWebhook
	RunStatusFailed                    = model.RunStatusFailed
	RunStatusPending                   = model.RunStatusPending
	RunStatusRunning                   = model.RunStatusRunning
	RunStatusScheduled                 = model.RunStatusScheduled
	RunStatusSucceeded                 = model.RunStatusSucceeded
	RunTriggerSourceIssueAgentRun      = model.RunTriggerSourceIssueAgentRun
	RunTriggerSourcePortalConversation = model.RunTriggerSourcePortalConversation
	RunTriggerSourcePortalTaskCreate   = model.RunTriggerSourcePortalTaskCreate
	RunTriggerSourcePortalTaskRerun    = model.RunTriggerSourcePortalTaskRerun
	RunTriggerSourceTaskCreate         = model.RunTriggerSourceTaskCreate
	RunTriggerSourceTaskRerun          = model.RunTriggerSourceTaskRerun
	RunTriggerSourceWebhook            = model.RunTriggerSourceWebhook
	RunTriggerSourceWorkflowStep       = model.RunTriggerSourceWorkflowStep
	TeamRoleAdmin                      = model.TeamRoleAdmin
	TeamRoleMember                     = model.TeamRoleMember
	TeamRoleOwner                      = model.TeamRoleOwner
	WorkflowRunStatusCanceled          = model.WorkflowRunStatusCanceled
	WorkflowRunStatusFailed            = model.WorkflowRunStatusFailed
	WorkflowRunStatusPending           = model.WorkflowRunStatusPending
	WorkflowRunStatusRunning           = model.WorkflowRunStatusRunning
	WorkflowRunStatusSucceeded         = model.WorkflowRunStatusSucceeded
	WorkflowStatusArchived             = model.WorkflowStatusArchived
	WorkflowStatusDraft                = model.WorkflowStatusDraft
	WorkflowStatusPublished            = model.WorkflowStatusPublished
	WorkflowStepRunStatusBlocked       = model.WorkflowStepRunStatusBlocked
	WorkflowStepRunStatusFailed        = model.WorkflowStepRunStatusFailed
	WorkflowStepRunStatusPending       = model.WorkflowStepRunStatusPending
	WorkflowStepRunStatusRunning       = model.WorkflowStepRunStatusRunning
	WorkflowStepRunStatusSucceeded     = model.WorkflowStepRunStatusSucceeded
	WorkflowStepTypeAgentTask          = model.WorkflowStepTypeAgentTask
)

var (
	ErrEmailExists   = model.ErrEmailExists
	ErrRunInProgress = model.ErrRunInProgress
)
