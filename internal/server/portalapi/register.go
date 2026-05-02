package portalapi

import (
	"net/http"
)

// Handler serves authenticated portal API (tasks, agents, artifacts, conversations, stream, files, upload, usage).
type Handler struct {
	cfg Config
}

// NewHandler returns a handler for portal routes.
func NewHandler(cfg Config) *Handler {
	return &Handler{cfg: cfg}
}

// Register adds all portal routes to mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/usage", h.usageHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/usage", h.teamUsageHandler)
	mux.HandleFunc("GET /api/teams", h.listTeamsHandler)
	mux.HandleFunc("POST /api/teams", h.createTeamHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/members", h.listTeamMembersHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/members", h.addTeamMemberHandler)
	mux.HandleFunc("DELETE /api/teams/{team_id}/members/{user_id}", h.removeTeamMemberHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/agents", h.listAgentsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/agents", h.createAgentHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/agents/{agent_id}", h.getAgentHandler)
	mux.HandleFunc("PATCH /api/teams/{team_id}/agents/{agent_id}", h.patchAgentHandler)
	mux.HandleFunc("DELETE /api/teams/{team_id}/agents/{agent_id}", h.deleteAgentHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/issues", h.listIssuesHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/issues", h.createIssueHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/issues/{issue_id}", h.getIssueHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/issues/{issue_id}/flow", h.getIssueFlowHandler)
	mux.HandleFunc("PATCH /api/teams/{team_id}/issues/{issue_id}", h.patchIssueHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/issues/{issue_id}/agent-runs", h.createIssueAgentRunHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/issues/{issue_id}/workflow-runs", h.createIssueWorkflowRunHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/workflows", h.listWorkflowsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/workflows", h.createWorkflowHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/workflows/{workflow_id}", h.getWorkflowHandler)
	mux.HandleFunc("PATCH /api/teams/{team_id}/workflows/{workflow_id}", h.patchWorkflowHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/workflows/{workflow_id}/runs", h.listWorkflowRunsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/workflows/{workflow_id}/runs", h.createWorkflowRunHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/workflow-runs/{workflow_run_id}", h.getWorkflowRunHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/upload", h.uploadHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/files", h.filesTreeHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/files/{path...}", h.fileContentHandler)
	mux.HandleFunc("POST /api/webhook-keys", h.createWebhookKeyHandler)
	mux.HandleFunc("GET /api/webhook-keys", h.listWebhookKeysHandler)
	mux.HandleFunc("DELETE /api/webhook-keys/{key_id}", h.revokeWebhookKeyHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/conversations", h.listConversationsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/conversations", h.createConversationHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/conversations/{conversation_id}/messages", h.getConversationMessagesHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/conversations/{conversation_id}/messages", h.addConversationMessageHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/conversations/{conversation_id}/tasks", h.listConversationTasksHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/conversations/{conversation_id}/tasks", h.createConversationTaskHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/tasks/{task_id}", h.getTaskHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/tasks/{task_id}/runs", h.createTaskRunHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/tasks/{task_id}/conversation", h.getTaskConversationHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/tasks/{task_id}/stream", h.getChatStreamHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/tasks/{task_id}/artifacts", h.listTaskArtifactsHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/task-runs/{task_run_id}/artifacts/items", h.listArtifactItemsHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/task-runs/{task_run_id}/artifacts/content", h.artifactContentHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/ws", h.wsUpgradeHandler)
}
