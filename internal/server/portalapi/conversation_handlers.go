package portalapi

import (
	"encoding/json"
	"net/http"

	convapp "buildmax/internal/core/conversation"
	"buildmax/internal/core/model"
	"buildmax/internal/server/httputil"
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

type runConversationTurnInput struct {
	conversationID       string
	message              string
	channel              string
	userID               string
	stream               bool
	streamStatus         int
	streamInitialPayload string
}

func (s *sseSink) OnDelta(delta string) {
	writeSSE(s.w, delta)
	if s.flusher != nil {
		s.flusher.Flush()
	}
}

// runConversationTurn runs the conversation loop (stream or non-stream). When stream is true it sets SSE headers, optionally writes streamInitialPayload, runs RunLoopStream, and writes "done". When stream is false it runs RunLoop and returns (reply, err).
func (h *Handler) runConversationTurn(w http.ResponseWriter, r *http.Request, in runConversationTurnInput) (reply string, err error) {
	cmd := convapp.HandleTurnCmd{
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
		if in.streamInitialPayload != "" {
			writeSSE(w, in.streamInitialPayload)
			if flusher, _ := w.(http.Flusher); flusher != nil {
				flusher.Flush()
			}
		}
		flusher, _ := w.(http.Flusher)
		sink := &sseSink{w: w, flusher: flusher}
		cmd.StreamSink = sink
		_, err := h.conversationService().HandleTurn(r.Context(), cmd)
		if err != nil {
			errJSON, _ := json.Marshal(err.Error())
			writeSSE(w, `{"error":`+string(errJSON)+`}`)
		}
		writeSSE(w, "done")
		if flusher != nil {
			flusher.Flush()
		}
		return "", nil
	}
	result, err := h.conversationService().HandleTurn(r.Context(), cmd)
	if err != nil {
		return "", err
	}
	return result.Reply, nil
}

func (h *Handler) listConversationsHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.ConversationStore, "conversations not configured")
	if !ok {
		return
	}
	limit, offset := parseLimitOffset(r.URL.Query(), "limit", "offset", 50, 100)
	list, total, err := h.cfg.ConversationStore.ListConversationsByTeam(r.Context(), teamID, limit, offset)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "list_conversations", "user_id", userID, "team_id", teamID)
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
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.ConversationStore, "conversations not configured")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.ConversationMessageStore, "conversation messages not configured") {
		return
	}
	var req createConversationRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Channel == "" {
		req.Channel = "portal"
	}
	conv, err := h.cfg.ConversationStore.CreateConversationInTeam(r.Context(), teamID, userID, req.Channel, userID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "create_conversation", "user_id", userID, "team_id", teamID)
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
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "conversation_loop", "conversation_id", conv.ConversationID)
		return
	}
	if !streamRequested {
		httputil.WriteJSON(w, http.StatusCreated, createConversationResponse{ConversationID: conv.ConversationID, Reply: reply})
	}
}

func (h *Handler) getConversationMessagesHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.ConversationStore, "conversations not configured")
	if !ok {
		return
	}
	conversationID, ok := pathValueRequired(w, r, "conversation_id")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.ConversationStore, "conversations not configured") {
		return
	}
	if !h.requireStore(w, h.cfg.ConversationMessageStore, "conversation messages not configured") {
		return
	}
	conv, err := h.cfg.ConversationStore.GetConversation(r.Context(), conversationID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "get_conversation", "conversation_id", conversationID)
		return
	}
	if conv == nil || conv.TeamID != teamID {
		httputil.WriteJSONError(w, http.StatusNotFound, "conversation not found")
		return
	}
	msgs, err := h.cfg.ConversationMessageStore.ListMessages(r.Context(), conversationID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "list_messages", "conversation_id", conversationID)
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
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.ConversationStore, "conversations not configured")
	if !ok {
		return
	}
	conversationID, ok := pathValueRequired(w, r, "conversation_id")
	if !ok {
		return
	}
	conv, ok := h.getConversationForTeam(w, r, teamID, conversationID)
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.ConversationMessageStore, "conversation messages not configured") {
		return
	}
	if h.cfg.ConversationLLMClient == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "conversation LLM not configured")
		return
	}
	var req addMessageRequest
	if !decodeJSONBody(w, r, &req) {
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
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "conversation_loop", "conversation_id", conversationID)
		return
	}
	if !streamRequested {
		httputil.WriteJSON(w, http.StatusOK, addMessageResponse{Reply: reply})
	}
}

func (h *Handler) getConversationForTeam(w http.ResponseWriter, r *http.Request, teamID, conversationID string) (*model.Conversation, bool) {
	if !h.requireStore(w, h.cfg.ConversationStore, "conversations not configured") {
		return nil, false
	}
	conv, err := h.cfg.ConversationStore.GetConversation(r.Context(), conversationID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "get_conversation", "conversation_id", conversationID)
		return nil, false
	}
	if conv == nil || conv.TeamID != teamID {
		httputil.WriteJSONError(w, http.StatusNotFound, "conversation not found")
		return nil, false
	}
	return conv, true
}
