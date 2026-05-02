package entity

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
