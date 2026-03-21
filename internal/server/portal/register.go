package portal

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
	mux.HandleFunc("GET /api/agents", h.listAgentsHandler)
	mux.HandleFunc("POST /api/agents", h.createAgentHandler)
	mux.HandleFunc("GET /api/agents/{agent_id}", h.getAgentHandler)
	mux.HandleFunc("PATCH /api/agents/{agent_id}", h.patchAgentHandler)
	mux.HandleFunc("DELETE /api/agents/{agent_id}", h.deleteAgentHandler)
	mux.HandleFunc("POST /api/upload", h.uploadHandler)
	mux.HandleFunc("GET /api/files", h.filesTreeHandler)
	mux.HandleFunc("GET /api/files/{path...}", h.fileContentHandler)
	mux.HandleFunc("POST /api/webhook-keys", h.createWebhookKeyHandler)
	mux.HandleFunc("GET /api/webhook-keys", h.listWebhookKeysHandler)
	mux.HandleFunc("DELETE /api/webhook-keys/{key_id}", h.revokeWebhookKeyHandler)
	mux.HandleFunc("GET /api/conversations", h.listConversationsHandler)
	mux.HandleFunc("POST /api/conversations", h.createConversationHandler)
	mux.HandleFunc("GET /api/conversations/{conversation_id}/messages", h.getConversationMessagesHandler)
	mux.HandleFunc("POST /api/conversations/{conversation_id}/messages", h.addConversationMessageHandler)
	mux.HandleFunc("GET /api/conversations/{conversation_id}/tasks", h.listConversationTasksHandler)
	mux.HandleFunc("POST /api/conversations/{conversation_id}/tasks", h.createConversationTaskHandler)
	mux.HandleFunc("GET /api/tasks/{task_id}", h.getTaskHandler)
	mux.HandleFunc("POST /api/tasks/{task_id}/runs", h.createTaskRunHandler)
	mux.HandleFunc("GET /api/tasks/{task_id}/conversation", h.getTaskConversationHandler)
	mux.HandleFunc("GET /api/tasks/{task_id}/stream", h.getChatStreamHandler)
	mux.HandleFunc("GET /api/tasks/{task_id}/artifacts", h.listTaskArtifactsHandler)
	mux.HandleFunc("GET /api/task-runs/{task_run_id}/artifacts/items", h.listArtifactItemsHandler)
	mux.HandleFunc("GET /api/task-runs/{task_run_id}/artifacts/content", h.artifactContentHandler)
	mux.HandleFunc("GET /api/ws", h.wsUpgradeHandler)
}
