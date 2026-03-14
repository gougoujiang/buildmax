package webhook

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"buildmax/internal/conversation"
	"buildmax/internal/server/httputil"
	"buildmax/internal/storage/entity"
)

// Register adds the webhook route to mux: POST /api/workspaces/{workspace_id}/webhook.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/webhook", h.serveWebhook)
}

// Handler handles webhook HTTP requests.
type Handler struct {
	cfg Config
}

// NewHandler returns a new webhook handler.
func NewHandler(cfg Config) *Handler {
	return &Handler{cfg: cfg}
}

func (h *Handler) serveWebhook(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspace_id")
	if workspaceID == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "workspace_id required")
		return
	}
	key := h.extractKey(r)
	if key == "" {
		httputil.WriteJSONError(w, http.StatusUnauthorized, "missing webhook key (Authorization: Bearer <key> or X-Webhook-Key)")
		return
	}
	if h.cfg.KeyStore == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "webhook keys not configured")
		return
	}
	resolvedWorkspaceID, err := h.cfg.KeyStore.GetWorkspaceIDByKey(r.Context(), key)
	if err != nil {
		httputil.WriteInternalError(w, err, "webhook handler", "handler", "get_workspace_by_key")
		return
	}
	if resolvedWorkspaceID == "" {
		httputil.WriteJSONError(w, http.StatusUnauthorized, "invalid webhook key")
		return
	}
	if resolvedWorkspaceID != workspaceID {
		httputil.WriteJSONError(w, http.StatusForbidden, "webhook key does not match workspace")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		httputil.WriteInternalError(w, err, "webhook handler", "handler", "read_body")
		return
	}
	req := &conversation.WebhookRequest{
		Body:        body,
		Header:      r.Header.Clone(),
		WorkspaceID: workspaceID,
	}
	turn, err := h.cfg.Adapter.Receive(r.Context(), req)
	if err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.cfg.Engine.Process(r.Context(), workspaceID, "", turn)
	if err != nil {
		if errors.Is(err, entity.ErrRunInProgress) {
			httputil.WriteJSONError(w, http.StatusConflict, "chat has a run already in progress")
			return
		}
		httputil.WriteInternalError(w, err, "webhook handler", "handler", "process")
		return
	}
	if len(result.TaskIDs) == 0 {
		httputil.WriteInternalError(w, nil, "webhook handler", "handler", "no_task_id")
		return
	}
	httputil.WriteJSON(w, http.StatusAccepted, map[string]string{"task_id": result.TaskIDs[0]})
}

func (h *Handler) extractKey(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	if k := r.Header.Get("X-Webhook-Key"); k != "" {
		return strings.TrimSpace(k)
	}
	return ""
}
