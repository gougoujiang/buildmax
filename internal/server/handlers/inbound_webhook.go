package handlers

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"buildmax/internal/core/conversation"
	"buildmax/internal/core/model"
	"buildmax/internal/server/httputil"
)

func (h *Handler) serveWebhook(w http.ResponseWriter, r *http.Request) {
	if h.cfg.WebhookEngine == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "webhook not configured")
		return
	}
	key := h.extractWebhookKey(r)
	if key == "" {
		httputil.WriteJSONError(w, http.StatusUnauthorized, "missing webhook key (Authorization: Bearer <key> or X-Webhook-Key)")
		return
	}
	if h.cfg.UserWebhookKeyStore == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "webhook keys not configured")
		return
	}
	resolvedUserID, err := h.cfg.UserWebhookKeyStore.GetUserIDByKey(r.Context(), key)
	if err != nil {
		httputil.WriteInternalError(w, err, "webhook handler", "handler", "get_user_by_key")
		return
	}
	if resolvedUserID == "" {
		httputil.WriteJSONError(w, http.StatusUnauthorized, "invalid webhook key")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		httputil.WriteInternalError(w, err, "webhook handler", "handler", "read_body")
		return
	}
	req := &conversation.WebhookRequest{
		Body:   body,
		Header: map[string][]string(r.Header.Clone()),
	}
	turn, err := h.cfg.WebhookAdapter.Receive(r.Context(), req)
	if err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if turn.UserID == "" {
		turn.UserID = resolvedUserID
	}
	if h.cfg.ConversationStore == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "conversations not configured")
		return
	}
	conv, err := h.cfg.ConversationStore.CreateConversation(r.Context(), resolvedUserID, conversation.ChannelWebhook, turn.UserID)
	if err != nil {
		httputil.WriteInternalError(w, err, "webhook handler", "handler", "create_conversation")
		return
	}
	result, err := h.cfg.WebhookEngine.Process(r.Context(), conv.ConversationID, "", turn)
	if err != nil {
		if errors.Is(err, model.ErrRunInProgress) {
			httputil.WriteJSONError(w, http.StatusConflict, "task has a run already in progress")
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

func (h *Handler) extractWebhookKey(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	if k := r.Header.Get("X-Webhook-Key"); k != "" {
		return strings.TrimSpace(k)
	}
	return ""
}
