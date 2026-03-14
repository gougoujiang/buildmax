package portal

import (
	"encoding/json"
	"net/http"

	convapp "buildmax/internal/app/conversation"
	"buildmax/internal/storage/entity"
)

type conversationListResponse struct {
	Conversations []conversationResponse `json:"conversations"`
	Total         int                    `json:"total"`
}

type conversationResponse struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Channel     string `json:"channel"`
	Title       string `json:"title,omitempty"`
	CreatedAt   int64  `json:"created_at"`
	CreatedBy   string `json:"created_by"`
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

// runConversationTurn runs the conversation loop (stream or non-stream). When stream is true it sets SSE headers, optionally writes streamInitialPayload, runs RunLoopStream, and writes "done". When stream is false it runs RunLoop and returns (reply, err).
func (h *Handler) runConversationTurn(r *http.Request, conversationID, message, channel, workspaceID, userID string, stream bool, w http.ResponseWriter, streamStatus int, streamInitialPayload string) (reply string, err error) {
	cmd := convapp.HandleTurnCmd{
		WorkspaceID:    workspaceID,
		UserID:         userID,
		Channel:        channel,
		Message:        message,
		ConversationID: conversationID,
	}
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(streamStatus)
		if streamInitialPayload != "" {
			writeSSE(w, streamInitialPayload)
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
	_, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.ConversationStore, "conversations not configured") {
		return
	}
	limit, offset := parseLimitOffset(r.URL.Query(), "limit", "offset", 50, 100)
	list, total, err := h.cfg.ConversationStore.ListConversationsByWorkspace(r.Context(), workspaceID, limit, offset)
	if err != nil {
		writeInternalError(w, err, "handler", "list_conversations", "workspace_id", workspaceID)
		return
	}
	out := make([]conversationResponse, len(list))
	for i := range list {
		out[i] = conversationResponse{
			ID:          list[i].ConversationID,
			WorkspaceID: list[i].WorkspaceID,
			Channel:     list[i].Channel,
			Title:       list[i].Title,
			CreatedAt:   list[i].CreatedAt,
			CreatedBy:   list[i].CreatedBy,
		}
	}
	writeJSON(w, http.StatusOK, conversationListResponse{Conversations: out, Total: total})
}

func (h *Handler) createConversationHandler(w http.ResponseWriter, r *http.Request) {
	userID, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.ConversationStore, "conversations not configured") {
		return
	}
	if !h.requireStore(w, h.cfg.ConversationMessageStore, "conversation messages not configured") {
		return
	}
	var req createConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Channel == "" {
		req.Channel = "portal"
	}
	conv, err := h.cfg.ConversationStore.CreateConversation(r.Context(), workspaceID, req.Channel, userID)
	if err != nil {
		writeInternalError(w, err, "handler", "create_conversation", "workspace_id", workspaceID)
		return
	}
	if req.Message == "" || h.cfg.ConversationLLMCaller == nil {
		writeJSON(w, http.StatusCreated, createConversationResponse{ConversationID: conv.ConversationID, Reply: ""})
		return
	}
	streamRequested := r.URL.Query().Get("stream") == "1"
	initialPayload := ""
	if streamRequested {
		initialPayload = `{"conversation_id":"` + conv.ConversationID + `"}`
	}
	reply, err := h.runConversationTurn(r, conv.ConversationID, req.Message, req.Channel, workspaceID, userID, streamRequested, w, http.StatusCreated, initialPayload)
	if err != nil {
		if h.writeConversationServiceError(w, r, err, nil) {
			return
		}
		writeInternalError(w, err, "handler", "conversation_loop", "conversation_id", conv.ConversationID)
		return
	}
	if !streamRequested {
		writeJSON(w, http.StatusCreated, createConversationResponse{ConversationID: conv.ConversationID, Reply: reply})
	}
}

func (h *Handler) getConversationMessagesHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
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
		writeInternalError(w, err, "handler", "get_conversation", "conversation_id", conversationID)
		return
	}
	if conv == nil || conv.WorkspaceID != workspaceID {
		writeJSONError(w, http.StatusNotFound, "conversation not found")
		return
	}
	msgs, err := h.cfg.ConversationMessageStore.ListMessages(r.Context(), conversationID)
	if err != nil {
		writeInternalError(w, err, "handler", "list_messages", "conversation_id", conversationID)
		return
	}
	out := make([]conversationMessageResponse, len(msgs))
	for i := range msgs {
		out[i] = conversationMessageResponse{
			ID:        msgs[i].ConversationMessageID,
			Role:      msgs[i].Role,
			Content:   msgs[i].Content,
			Channel:   msgs[i].Channel,
			CreatedAt: msgs[i].CreatedAt,
		}
	}
	writeJSON(w, http.StatusOK, messagesResponse{Messages: out})
}

func (h *Handler) addConversationMessageHandler(w http.ResponseWriter, r *http.Request) {
	userID, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	conversationID, ok := pathValueRequired(w, r, "conversation_id")
	if !ok {
		return
	}
	conv, ok := h.getConversationForWorkspace(w, r, workspaceID, conversationID)
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.ConversationMessageStore, "conversation messages not configured") {
		return
	}
	if h.cfg.ConversationLLMCaller == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "conversation LLM not configured")
		return
	}
	var req addMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Content == "" {
		writeJSONError(w, http.StatusBadRequest, "content required")
		return
	}
	streamRequested := r.URL.Query().Get("stream") == "1"
	reply, err := h.runConversationTurn(r, conversationID, req.Content, conv.Channel, workspaceID, userID, streamRequested, w, http.StatusOK, "")
	if err != nil {
		if h.writeConversationServiceError(w, r, err, nil) {
			return
		}
		writeInternalError(w, err, "handler", "conversation_loop", "conversation_id", conversationID)
		return
	}
	if !streamRequested {
		writeJSON(w, http.StatusOK, addMessageResponse{Reply: reply})
	}
}

func (h *Handler) getConversationForWorkspace(w http.ResponseWriter, r *http.Request, workspaceID, conversationID string) (*entity.Conversation, bool) {
	if !h.requireStore(w, h.cfg.ConversationStore, "conversations not configured") {
		return nil, false
	}
	conv, err := h.cfg.ConversationStore.GetConversation(r.Context(), conversationID)
	if err != nil {
		writeInternalError(w, err, "handler", "get_conversation", "conversation_id", conversationID)
		return nil, false
	}
	if conv == nil || conv.WorkspaceID != workspaceID {
		writeJSONError(w, http.StatusNotFound, "conversation not found")
		return nil, false
	}
	return conv, true
}
