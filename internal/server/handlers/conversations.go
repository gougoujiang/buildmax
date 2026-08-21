package handlers

import (
	"encoding/json"
	"errors"
	"github.com/gougoujiang/buildmax/internal/server/turnqueue"
	"net/http"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	"github.com/gougoujiang/buildmax/internal/service/conversation"
	"github.com/gougoujiang/buildmax/internal/service/task"
)

type conversationListResponse struct {
	Conversations []conversationResponse `json:"conversations"`
	Total         int                    `json:"total"`
}

type conversationResponse struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	TeamID    string `json:"team_id,omitempty"`
	Channel   string `json:"channel"`
	Title     string `json:"title,omitempty"`
	CreatedAt int64  `json:"created_at"`
	CreatedBy string `json:"created_by"`
}

type createConversationRequest struct {
	Channel string `json:"channel"`
	Message string `json:"message"`
}

type createConversationResponse struct {
	ConversationID string `json:"conversation_id"`
	Reply          string `json:"reply,omitempty"`
}

type conversationMessageResponse struct {
	ID        string  `json:"id"`
	Role      string  `json:"role"`
	Content   string  `json:"content"`
	Channel   *string `json:"channel,omitempty"`
	CreatedAt int64   `json:"created_at"`
}

type messagesResponse struct {
	Messages []conversationMessageResponse `json:"messages"`
}

type addMessageRequest struct {
	Content string `json:"content"`
}

type addMessageResponse struct {
	Reply string `json:"reply"`
}

func isVisibleConversationMessage(m model.ConversationMessage) bool {
	return m.Role == "user" || m.Role == "assistant"
}

type sseSink struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func (s *sseSink) OnDelta(delta string) {
	writeSSE(s.w, delta)
	if s.flusher != nil {
		s.flusher.Flush()
	}
}

type runConversationTurnInput struct {
	conversationID       string
	message              string
	channel              string
	userID               string
	stream               bool
	streamStatus         int
	streamInitialPayload string
}

func (h *Handler) conversationService() *conversation.Service {
	return &conversation.Service{
		TaskService:       h.taskService(),
		ConversationStore: h.cfg.ConversationStore,
		MessageStore:      h.cfg.ConversationMessageStore,
		LLMClient:         h.cfg.ConversationLLMClient,
		TitleGenerator:    h.cfg.TitleGenerator,
		AgentStore:        h.cfg.AgentStore,
	}
}

func (h *Handler) writeConversationServiceError(w http.ResponseWriter, r *http.Request, err error, agentID *string) bool {
	if h.writeTaskServiceError(w, r, err, agentID) {
		return true
	}
	// turnqueue.ErrQueueFull is this package's own, and its text names the queue depth,
	// so it is answered here rather than given a Kind.
	if errors.Is(err, turnqueue.ErrQueueFull) {
		httputil.WriteJSONError(w, http.StatusTooManyRequests, err.Error())
		return true
	}
	var quotaErr *task.QuotaExceededError
	if errors.As(err, &quotaErr) {
		httputil.WriteQuotaExceeded(w, quotaErr.Reason)
		return true
	}
	return false
}

// runConversationTurn runs one turn for the conversation, streaming it when asked.
//
// The turn goes through the server's turn registry, so a request that arrives while
// the conversation is busy waits for its turn instead of racing the running one
// through the same message history. A streaming request writes its headers first,
// so the client sees the stream open while it waits.
func (h *Handler) runConversationTurn(w http.ResponseWriter, r *http.Request, in runConversationTurnInput) (reply string, err error) {
	cmd := conversation.HandleTurnCmd{
		UserID:         in.userID,
		Channel:        in.channel,
		Message:        in.message,
		ConversationID: in.conversationID,
	}
	if in.stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(in.streamStatus)
		flusher, _ := w.(http.Flusher)
		if in.streamInitialPayload != "" {
			writeSSE(w, in.streamInitialPayload)
			if flusher != nil {
				flusher.Flush()
			}
		}
		cmd.StreamSink = &sseSink{w: w, flusher: flusher}
		var turnErr error
		waitErr := h.turns.RunSync(r.Context(), in.conversationID, func() {
			_, turnErr = h.conversationService().HandleTurn(r.Context(), cmd)
		})
		if turnErr == nil {
			turnErr = waitErr
		}
		if turnErr != nil {
			errJSON, _ := json.Marshal(turnErr.Error())
			writeSSE(w, `{"error":`+string(errJSON)+`}`)
		}
		writeSSE(w, "done")
		if flusher != nil {
			flusher.Flush()
		}
		return "", nil
	}
	var result conversation.ConversationResult
	var turnErr error
	if waitErr := h.turns.RunSync(r.Context(), in.conversationID, func() {
		result, turnErr = h.conversationService().HandleTurn(r.Context(), cmd)
	}); waitErr != nil {
		return "", waitErr
	}
	if turnErr != nil {
		return "", turnErr
	}
	return result.Reply, nil
}

func (h *Handler) listConversationsHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.ConversationStore, "conversations not configured")
	if !ok {
		return
	}
	limit, offset := httputil.LimitOffset(r.URL.Query(), "limit", "offset", httputil.ListPageDefault, httputil.ListPageMax)
	list, total, err := h.cfg.ConversationStore.ListConversationsByTeam(r.Context(), teamID, limit, offset)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_conversations", "user_id", userID, "team_id", teamID)
		return
	}
	out := make([]conversationResponse, len(list))
	for i := range list {
		out[i] = conversationResponse{
			ID:        list[i].ConversationID,
			UserID:    list[i].UserID,
			TeamID:    list[i].TeamID,
			Channel:   list[i].Channel,
			Title:     list[i].Title,
			CreatedAt: list[i].CreatedAt,
			CreatedBy: list[i].CreatedBy,
		}
	}
	httputil.WriteJSON(w, http.StatusOK, conversationListResponse{Conversations: out, Total: total})
}

func (h *Handler) createConversationHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.ConversationStore, "conversations not configured")
	if !ok {
		return
	}
	if !httputil.RequireStore(w, h.cfg.ConversationMessageStore, "conversation messages not configured") {
		return
	}
	var req createConversationRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	if req.Channel == "" {
		req.Channel = "portal"
	}
	conv, err := h.cfg.ConversationStore.CreateConversationInTeam(r.Context(), teamID, userID, req.Channel, userID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "create_conversation", "user_id", userID, "team_id", teamID)
		return
	}
	if req.Message == "" || h.cfg.ConversationLLMClient == nil {
		httputil.WriteJSON(w, http.StatusCreated, createConversationResponse{ConversationID: conv.ConversationID, Reply: ""})
		return
	}
	streamRequested := r.URL.Query().Get("stream") == "1"
	initialPayload := ""
	if streamRequested {
		initialPayload = `{"conversation_id":"` + conv.ConversationID + `"}`
	}
	reply, err := h.runConversationTurn(w, r, runConversationTurnInput{
		conversationID:       conv.ConversationID,
		message:              req.Message,
		channel:              req.Channel,
		userID:               userID,
		stream:               streamRequested,
		streamStatus:         http.StatusCreated,
		streamInitialPayload: initialPayload,
	})
	if err != nil {
		if h.writeConversationServiceError(w, r, err, nil) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "conversation_loop", "conversation_id", conv.ConversationID)
		return
	}
	if !streamRequested {
		httputil.WriteJSON(w, http.StatusCreated, createConversationResponse{ConversationID: conv.ConversationID, Reply: reply})
	}
}

func (h *Handler) getConversationMessagesHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.ConversationStore, "conversations not configured")
	if !ok {
		return
	}
	conversationID, ok := httputil.PathValue(w, r, "conversation_id")
	if !ok {
		return
	}
	if !httputil.RequireStore(w, h.cfg.ConversationStore, "conversations not configured") {
		return
	}
	if !httputil.RequireStore(w, h.cfg.ConversationMessageStore, "conversation messages not configured") {
		return
	}
	conv, err := h.cfg.ConversationStore.GetConversation(r.Context(), conversationID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_conversation", "conversation_id", conversationID)
		return
	}
	if conv == nil || conv.TeamID != teamID {
		httputil.WriteJSONError(w, http.StatusNotFound, "conversation not found")
		return
	}
	msgs, err := h.cfg.ConversationMessageStore.ListMessages(r.Context(), conversationID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_messages", "conversation_id", conversationID)
		return
	}
	out := make([]conversationMessageResponse, 0, len(msgs))
	for i := range msgs {
		if !isVisibleConversationMessage(msgs[i]) {
			continue
		}
		out = append(out, conversationMessageResponse{
			ID:        msgs[i].ConversationMessageID,
			Role:      msgs[i].Role,
			Content:   msgs[i].Content,
			Channel:   msgs[i].Channel,
			CreatedAt: msgs[i].CreatedAt,
		})
	}
	httputil.WriteJSON(w, http.StatusOK, messagesResponse{Messages: out})
}

func (h *Handler) addConversationMessageHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.ConversationStore, "conversations not configured")
	if !ok {
		return
	}
	conversationID, ok := httputil.PathValue(w, r, "conversation_id")
	if !ok {
		return
	}
	conv, ok := h.getConversationForTeam(w, r, teamID, conversationID)
	if !ok {
		return
	}
	if !httputil.RequireStore(w, h.cfg.ConversationMessageStore, "conversation messages not configured") {
		return
	}
	if h.cfg.ConversationLLMClient == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "conversation LLM not configured")
		return
	}
	var req addMessageRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	if req.Content == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "content required")
		return
	}
	streamRequested := r.URL.Query().Get("stream") == "1"
	reply, err := h.runConversationTurn(w, r, runConversationTurnInput{
		conversationID: conversationID,
		message:        req.Content,
		channel:        conv.Channel,
		userID:         userID,
		stream:         streamRequested,
		streamStatus:   http.StatusOK,
	})
	if err != nil {
		if h.writeConversationServiceError(w, r, err, nil) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "conversation_loop", "conversation_id", conversationID)
		return
	}
	if !streamRequested {
		httputil.WriteJSON(w, http.StatusOK, addMessageResponse{Reply: reply})
	}
}

func (h *Handler) getConversationForTeam(w http.ResponseWriter, r *http.Request, teamID, conversationID string) (*model.Conversation, bool) {
	if !httputil.RequireStore(w, h.cfg.ConversationStore, "conversations not configured") {
		return nil, false
	}
	conv, err := h.cfg.ConversationStore.GetConversation(r.Context(), conversationID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_conversation", "conversation_id", conversationID)
		return nil, false
	}
	if conv == nil || conv.TeamID != teamID {
		httputil.WriteJSONError(w, http.StatusNotFound, "conversation not found")
		return nil, false
	}
	return conv, true
}
