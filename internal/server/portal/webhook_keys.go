package portal

import (
	"encoding/json"
	"net/http"

	"buildmax/internal/server/httputil"
)

type createWebhookKeyRequest struct {
	Name string `json:"name"`
}

type createWebhookKeyResponse struct {
	Key  string `json:"key"`
	KeyID string `json:"key_id"`
}

type listWebhookKeysResponse struct {
	Keys []webhookKeyMetaResponse `json:"keys"`
}

type webhookKeyMetaResponse struct {
	KeyID     string `json:"key_id"`
	Name      string `json:"name,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

func (h *Handler) createWebhookKeyHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.WorkspaceWebhookKeyStore, "webhook keys not configured") {
		return
	}
	var req createWebhookKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	plaintextKey, keyID, err := h.cfg.WorkspaceWebhookKeyStore.CreateKey(r.Context(), workspaceID, req.Name)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "create_webhook_key", "workspace_id", workspaceID)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, createWebhookKeyResponse{Key: plaintextKey, KeyID: keyID})
}

func (h *Handler) listWebhookKeysHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.WorkspaceWebhookKeyStore, "webhook keys not configured") {
		return
	}
	list, err := h.cfg.WorkspaceWebhookKeyStore.ListKeys(r.Context(), workspaceID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "list_webhook_keys", "workspace_id", workspaceID)
		return
	}
	out := make([]webhookKeyMetaResponse, len(list))
	for i := range list {
		out[i] = webhookKeyMetaResponse{KeyID: list[i].KeyID, Name: list[i].Name, CreatedAt: list[i].CreatedAt}
	}
	httputil.WriteJSON(w, http.StatusOK, listWebhookKeysResponse{Keys: out})
}

func (h *Handler) revokeWebhookKeyHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	keyID := r.PathValue("key_id")
	if keyID == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "key_id required")
		return
	}
	if !h.requireStore(w, h.cfg.WorkspaceWebhookKeyStore, "webhook keys not configured") {
		return
	}
	err := h.cfg.WorkspaceWebhookKeyStore.RevokeKey(r.Context(), workspaceID, keyID)
	if err != nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "webhook key not found or not in workspace")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
