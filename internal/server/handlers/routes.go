package handlers

import (
	"net/http"
)

// Register adds all routes to mux: auth (unauthenticated), user API (JWT), worker API (worker token), inbound webhook.
func (h *Handler) Register(mux *http.ServeMux) {
	// Auth — unauthenticated
	mux.HandleFunc("POST /api/otp/request", h.otpRequestHandler)
	mux.HandleFunc("POST /api/login", h.loginHandler)
	mux.HandleFunc("POST /api/token/refresh", h.refreshHandler)
	mux.HandleFunc("POST /api/logout", h.logoutHandler)

	// Password — authenticated; sets or changes the caller's own password
	mux.HandleFunc("POST /api/password", h.setPasswordHandler)

	// Managed LLM gateway
	mux.HandleFunc("GET /api/teams/{team_id}/llm/models", h.listLLMModelsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/llm/completions", h.llmCompletionsHandler)

	// Usage
	mux.HandleFunc("GET /api/usage", h.usageHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/audit-events", h.listAuditEventsHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/audit-events/export", h.exportAuditEventsHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/usage", h.teamUsageHandler)

	// Teams and members
	mux.HandleFunc("GET /api/teams", h.listTeamsHandler)
	mux.HandleFunc("POST /api/teams", h.createTeamHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/members", h.listTeamMembersHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/members", h.addTeamMemberHandler)
	mux.HandleFunc("DELETE /api/teams/{team_id}/members/{user_id}", h.removeTeamMemberHandler)

	// Agents
	mux.HandleFunc("GET /api/teams/{team_id}/agents", h.listAgentsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/agents", h.createAgentHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/agents/{agent_id}", h.getAgentHandler)
	mux.HandleFunc("PATCH /api/teams/{team_id}/agents/{agent_id}", h.patchAgentHandler)
	mux.HandleFunc("DELETE /api/teams/{team_id}/agents/{agent_id}", h.deleteAgentHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/agents/{agent_id}/revisions", h.listAgentRevisionsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/agents/{agent_id}/revisions/{revision}/restore", h.restoreAgentRevisionHandler)

	// Issues
	mux.HandleFunc("GET /api/teams/{team_id}/issues", h.listIssuesHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/issues", h.createIssueHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/issues/{issue_id}", h.getIssueHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/issues/{issue_id}/flow", h.getIssueFlowHandler)
	mux.HandleFunc("PATCH /api/teams/{team_id}/issues/{issue_id}", h.patchIssueHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/issues/{issue_id}/comments", h.listIssueCommentsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/issues/{issue_id}/comments", h.createIssueCommentHandler)
	mux.HandleFunc("PATCH /api/teams/{team_id}/issues/{issue_id}/comments/{comment_id}", h.patchIssueCommentHandler)
	mux.HandleFunc("DELETE /api/teams/{team_id}/issues/{issue_id}/comments/{comment_id}", h.deleteIssueCommentHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/issues/{issue_id}/agent-runs", h.createIssueAgentRunHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/issues/{issue_id}/workflow-runs", h.createIssueWorkflowRunHandler)

	// Workflows
	mux.HandleFunc("GET /api/teams/{team_id}/workflows", h.listWorkflowsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/workflows", h.createWorkflowHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/workflows/{workflow_id}", h.getWorkflowHandler)
	mux.HandleFunc("PATCH /api/teams/{team_id}/workflows/{workflow_id}", h.patchWorkflowHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/workflows/{workflow_id}/revisions", h.listWorkflowRevisionsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/workflows/{workflow_id}/revisions/{revision}/restore", h.restoreWorkflowRevisionHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/workflows/{workflow_id}/runs", h.listWorkflowRunsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/workflows/{workflow_id}/runs", h.createWorkflowRunHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/workflow-runs/{workflow_run_id}", h.getWorkflowRunHandler)

	// Files
	mux.HandleFunc("POST /api/teams/{team_id}/upload", h.uploadHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/files", h.filesTreeHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/files/{path...}", h.fileContentHandler)

	// Webhook keys
	mux.HandleFunc("POST /api/webhook-keys", h.createWebhookKeyHandler)
	mux.HandleFunc("GET /api/webhook-keys", h.listWebhookKeysHandler)
	mux.HandleFunc("DELETE /api/webhook-keys/{key_id}", h.revokeWebhookKeyHandler)

	// Conversations
	mux.HandleFunc("GET /api/teams/{team_id}/conversations", h.listConversationsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/conversations", h.createConversationHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/conversations/{conversation_id}/messages", h.getConversationMessagesHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/conversations/{conversation_id}/messages", h.addConversationMessageHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/conversations/{conversation_id}/tasks", h.listConversationTasksHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/conversations/{conversation_id}/tasks", h.createConversationTaskHandler)

	// Tasks and task runs
	mux.HandleFunc("GET /api/teams/{team_id}/tasks/{task_id}", h.getTaskHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/tasks/{task_id}/runs", h.createTaskRunHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/tasks/{task_id}/cancel", h.cancelTaskHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/tasks/{task_id}/retry", h.retryTaskHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/tasks/{task_id}/conversation", h.getTaskConversationHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/tasks/{task_id}/stream", h.getChatStreamHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/tasks/{task_id}/artifacts", h.listTaskArtifactsHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/task-runs/{task_run_id}/artifacts/items", h.listArtifactItemsHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/task-runs/{task_run_id}/artifacts/content", h.artifactContentHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/task-runs/{task_run_id}/trace", h.getTaskRunTraceHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/task-runs/{task_run_id}/llm-calls", h.listTaskRunLLMCallsHandler)

	// WebSocket
	mux.HandleFunc("GET /api/teams/{team_id}/ws", h.wsUpgradeHandler)

	// System administration — deployment-scoped, never team-scoped. Every route
	// here requires a system_admin grant and none takes a {team_id}: an admin
	// route that looked team-scoped would invite exactly the confusion the
	// boundary exists to prevent. See docs/design/system-administration.md.
	mux.HandleFunc("GET /api/admin/me", h.adminMeHandler)
	mux.HandleFunc("GET /api/admin/grants", h.listAdminGrantsHandler)
	mux.HandleFunc("POST /api/admin/grants", h.createAdminGrantHandler)
	mux.HandleFunc("DELETE /api/admin/grants/{user_id}", h.deleteAdminGrantHandler)
	mux.HandleFunc("GET /api/admin/users", h.listAdminUsersHandler)
	mux.HandleFunc("POST /api/admin/users", h.createAdminUserHandler)
	mux.HandleFunc("GET /api/admin/users/{user_id}", h.getAdminUserHandler)
	mux.HandleFunc("POST /api/admin/users/{user_id}/login-code", h.issueAdminLoginCodeHandler)
	mux.HandleFunc("POST /api/admin/users/{user_id}/disable", h.setAdminUserDisabledHandler(true))
	mux.HandleFunc("POST /api/admin/users/{user_id}/enable", h.setAdminUserDisabledHandler(false))
	mux.HandleFunc("DELETE /api/admin/users/{user_id}/sessions", h.revokeAdminUserSessionsHandler)
	mux.HandleFunc("GET /api/admin/system", h.adminSystemHandler)
	mux.HandleFunc("GET /api/admin/config", h.adminConfigHandler)
	mux.HandleFunc("GET /api/admin/audit-events", h.listAdminAuditEventsHandler)
	mux.HandleFunc("GET /api/admin/audit-events/export", h.exportAdminAuditEventsHandler)
	mux.HandleFunc("GET /api/admin/teams", h.listAdminTeamsHandler)
	mux.HandleFunc("GET /api/admin/teams/{team_id}", h.getAdminTeamHandler)
	mux.HandleFunc("GET /api/admin/llm/models", h.listAdminModelsHandler)
	mux.HandleFunc("POST /api/admin/llm/models/{model_id}/enable", h.setAdminModelEnabledHandler(true))
	mux.HandleFunc("POST /api/admin/llm/models/{model_id}/disable", h.setAdminModelEnabledHandler(false))

	// Worker API — every route is scoped to one run, so every route authenticates
	// with that run's token. The deployment-wide worker token is still accepted
	// here for one release; see docs/design/worker-run-token.md.
	mux.Handle("GET /api/worker/task-runs/{task_run_id}", h.runScopedWorkerMiddleware(http.HandlerFunc(h.getTaskRun)))
	mux.Handle("PATCH /api/worker/task-runs/{task_run_id}", h.runScopedWorkerMiddleware(http.HandlerFunc(h.patchTaskRun)))
	mux.Handle("POST /api/worker/task-runs/{task_run_id}/stream", h.runScopedWorkerMiddleware(http.HandlerFunc(h.postStream)))
	// Managed inference takes the run token only. It never accepted the shared
	// worker token, so it has no upgrade window to keep open and no reason to
	// grow a fallback the other three are already shedding.
	mux.HandleFunc("POST /api/worker/task-runs/{task_run_id}/llm/completions", h.workerLLMCompletionsHandler)

	// Inbound webhook
	mux.HandleFunc("POST /api/webhook", h.serveWebhook)
}
