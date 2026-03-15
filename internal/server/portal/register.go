package portal

import (
	"net/http"
)

// Handler serves authenticated portal API (workspaces, tasks, agents, artifacts, conversations, stream, files, upload, usage).
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
	mux.HandleFunc("GET /api/workspaces", h.workspacesHandler)
	mux.HandleFunc("POST /api/workspaces", h.createWorkspaceHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/agents", h.listAgentsHandler)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/agents", h.createAgentHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/agents/{agent_id}", h.getAgentHandler)
	mux.HandleFunc("PATCH /api/workspaces/{workspace_id}/agents/{agent_id}", h.patchAgentHandler)
	mux.HandleFunc("DELETE /api/workspaces/{workspace_id}/agents/{agent_id}", h.deleteAgentHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/tasks", h.listWorkspaceChatsHandler)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/tasks", h.createWorkspaceChatHandler)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/tasks/{task_id}/runs", h.createTaskRunHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/artifacts", h.listWorkspaceArtifactsHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/artifacts/{task_run_id}/items", h.listArtifactItemsHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/artifacts/{task_run_id}/content", h.artifactContentHandler)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/upload", h.uploadHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/files", h.filesTreeHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/files/{path...}", h.fileContentHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/tasks/{task_id}/conversation", h.getChatConversationHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/tasks/{task_id}/stream", h.getChatStreamHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/conversations", h.listConversationsHandler)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/conversations", h.createConversationHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/conversations/{conversation_id}/messages", h.getConversationMessagesHandler)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/conversations/{conversation_id}/messages", h.addConversationMessageHandler)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/webhook-keys", h.createWebhookKeyHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/webhook-keys", h.listWebhookKeysHandler)
	mux.HandleFunc("DELETE /api/workspaces/{workspace_id}/webhook-keys/{key_id}", h.revokeWebhookKeyHandler)
}
