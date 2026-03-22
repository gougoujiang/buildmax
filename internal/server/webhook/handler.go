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

// Register adds the webhook route to mux: POST /api/webhook.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/webhook", h.serveWebhook)
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
	key := h.extractKey(r)
	if key == "" {
		httputil.WriteJSONError(w, http.StatusUnauthorized, "missing webhook key (Authorization: Bearer <key> or X-Webhook-Key)")
		return
	}
	if h.cfg.KeyStore == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "webhook keys not configured")
		return
	}
	resolvedUserID, err := h.cfg.KeyStore.GetUserIDByKey(r.Context(), key)
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
		Header: r.Header.Clone(),
	}
	turn, err := h.cfg.Adapter.Receive(r.Context(), req)
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
	result, err := h.cfg.Engine.Process(r.Context(), conv.ConversationID, "", turn)
	if err != nil {
		if errors.Is(err, entity.ErrRunInProgress) {
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

func (h *Handler) extractKey(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	if k := r.Header.Get("X-Webhook-Key"); k != "" {
		return strings.TrimSpace(k)
	}
	return ""
}
