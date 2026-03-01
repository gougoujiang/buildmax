// HTTP handlers for Tier 1 conversations and conversation messages.
package server

import (
	"encoding/json"
	"net/http"

	"buildmax/internal/conversation"
	"buildmax/internal/storage/entity"
)

// conversationListResponse is the response for GET .../conversations (snake_case).
type conversationListResponse struct {
	Conversations []conversationResponse `json:"conversations"`
	Total         int                   `json:"total"`
}

type conversationResponse struct {
	ID             string `json:"id"`
	WorkspaceID    string `json:"workspace_id"`
	Channel        string `json:"channel"`
	CreatedAt      int64  `json:"created_at"`
	CreatedBy      string `json:"created_by"`
}

// createConversationRequest is the body for POST .../conversations.
type createConversationRequest struct {
	Channel  string `json:"channel"`
	Message  string `json:"message"`
}

// createConversationResponse is the response for POST .../conversations.
type createConversationResponse struct {
	ConversationID string `json:"conversation_id"`
	Reply          string `json:"reply,omitempty"`
}

// conversationMessageResponse is one message in GET .../messages (snake_case).
type conversationMessageResponse struct {
	ID        string  `json:"id"`
	Role      string  `json:"role"`
	Content   string  `json:"content"`
	Channel   *string `json:"channel,omitempty"`
	CreatedAt int64   `json:"created_at"`
}

// messagesResponse is the response for GET .../conversations/{id}/messages.
type messagesResponse struct {
	Messages []conversationMessageResponse `json:"messages"`
}

// addMessageRequest is the body for POST .../conversations/{id}/messages.
type addMessageRequest struct {
	Content string `json:"content"`
}

// addMessageResponse is the response for POST .../conversations/{id}/messages.
type addMessageResponse struct {
	Reply string `json:"reply"`
}

// sseSink writes content deltas as SSE events and flushes. Used when stream=1.
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

func (s *Server) listConversationsHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireStore(w, s.cfg.ConversationStore, "conversations not configured") {
		return
	}
	limit, offset := parseLimitOffset(r.URL.Query(), "limit", "offset", 50, 100)
	list, total, err := s.cfg.ConversationStore.ListConversationsByWorkspace(r.Context(), workspaceID, limit, offset)
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
			CreatedAt:   list[i].CreatedAt,
			CreatedBy:   list[i].CreatedBy,
		}
	}
	writeJSON(w, http.StatusOK, conversationListResponse{Conversations: out, Total: total})
}

func (s *Server) createConversationHandler(w http.ResponseWriter, r *http.Request) {
	userID, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireStore(w, s.cfg.ConversationStore, "conversations not configured") {
		return
	}
	if !s.requireStore(w, s.cfg.ConversationMessageStore, "conversation messages not configured") {
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
	conv, err := s.cfg.ConversationStore.CreateConversation(r.Context(), workspaceID, req.Channel, userID)
	if err != nil {
		writeInternalError(w, err, "handler", "create_conversation", "workspace_id", workspaceID)
		return
	}
	streamRequested := r.URL.Query().Get("stream") == "1"
	if streamRequested && req.Message != "" && s.cfg.ConversationLLMCaller != nil {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusCreated)
		flusher, _ := w.(http.Flusher)
		writeSSE(w, `{"conversation_id":"`+conv.ConversationID+`"}`)
		if flusher != nil {
			flusher.Flush()
		}
		sink := &sseSink{w: w, flusher: flusher}
		reply, err := conversation.RunLoopStream(r.Context(), s.cfg.ConversationStore, s.cfg.ConversationMessageStore,
			s.cfg.ConversationLLMCaller, conv.ConversationID, req.Message, req.Channel, nil, workspaceID, userID,
			s.startChatRunner(workspaceID, userID, conv.ConversationID), sink)
		if err != nil {
			errJSON, _ := json.Marshal(err.Error())
			writeSSE(w, `{"error":`+string(errJSON)+`}`)
		}
		writeSSE(w, "done")
		if flusher != nil {
			flusher.Flush()
		}
		_ = reply
		return
	}
	reply := ""
	if req.Message != "" && s.cfg.ConversationLLMCaller != nil {
		reply, err = conversation.RunLoop(r.Context(), s.cfg.ConversationStore, s.cfg.ConversationMessageStore,
			s.cfg.ConversationLLMCaller, conv.ConversationID, req.Message, req.Channel, nil, workspaceID, userID,
			s.startChatRunner(workspaceID, userID, conv.ConversationID))
		if err != nil {
			writeInternalError(w, err, "handler", "conversation_loop", "conversation_id", conv.ConversationID)
			return
		}
	}
	writeJSON(w, http.StatusCreated, createConversationResponse{
		ConversationID: conv.ConversationID,
		Reply:          reply,
	})
}

func (s *Server) getConversationMessagesHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	conversationID := r.PathValue("conversation_id")
	if conversationID == "" {
		writeJSONError(w, http.StatusBadRequest, "conversation_id required")
		return
	}
	if !s.requireStore(w, s.cfg.ConversationStore, "conversations not configured") {
		return
	}
	if !s.requireStore(w, s.cfg.ConversationMessageStore, "conversation messages not configured") {
		return
	}
	conv, err := s.cfg.ConversationStore.GetConversation(r.Context(), conversationID)
	if err != nil {
		writeInternalError(w, err, "handler", "get_conversation", "conversation_id", conversationID)
		return
	}
	if conv == nil || conv.WorkspaceID != workspaceID {
		writeJSONError(w, http.StatusNotFound, "conversation not found")
		return
	}
	msgs, err := s.cfg.ConversationMessageStore.ListMessages(r.Context(), conversationID)
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

func (s *Server) addConversationMessageHandler(w http.ResponseWriter, r *http.Request) {
	userID, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	conversationID := r.PathValue("conversation_id")
	if conversationID == "" {
		writeJSONError(w, http.StatusBadRequest, "conversation_id required")
		return
	}
	if !s.requireStore(w, s.cfg.ConversationStore, "conversations not configured") {
		return
	}
	if !s.requireStore(w, s.cfg.ConversationMessageStore, "conversation messages not configured") {
		return
	}
	if s.cfg.ConversationLLMCaller == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "conversation LLM not configured")
		return
	}
	conv, err := s.cfg.ConversationStore.GetConversation(r.Context(), conversationID)
	if err != nil {
		writeInternalError(w, err, "handler", "get_conversation", "conversation_id", conversationID)
		return
	}
	if conv == nil || conv.WorkspaceID != workspaceID {
		writeJSONError(w, http.StatusNotFound, "conversation not found")
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
	caller := s.cfg.ConversationLLMCaller
	if streamRequested && caller != nil {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		sink := &sseSink{w: w, flusher: flusher}
		reply, err := conversation.RunLoopStream(r.Context(), s.cfg.ConversationStore, s.cfg.ConversationMessageStore,
			caller, conversationID, req.Content, conv.Channel, nil, workspaceID, userID,
			s.startChatRunner(workspaceID, userID, conv.ConversationID), sink)
		if err != nil {
			errJSON, _ := json.Marshal(err.Error())
			writeSSE(w, `{"error":`+string(errJSON)+`}`)
		}
		writeSSE(w, "done")
		if flusher != nil {
			flusher.Flush()
		}
		_ = reply
		return
	}
	reply, err := conversation.RunLoop(r.Context(), s.cfg.ConversationStore, s.cfg.ConversationMessageStore,
		caller, conversationID, req.Content, conv.Channel, nil, workspaceID, userID,
		s.startChatRunner(workspaceID, userID, conv.ConversationID))
	if err != nil {
		writeInternalError(w, err, "handler", "conversation_loop", "conversation_id", conversationID)
		return
	}
	writeJSON(w, http.StatusOK, addMessageResponse{Reply: reply})
}

// getConversationForWorkspace loads the conversation and verifies it belongs to the workspace.
// Writes 404 if missing or wrong workspace. Returns (conv, true) on success.
func (s *Server) getConversationForWorkspace(w http.ResponseWriter, r *http.Request, workspaceID, conversationID string) (*entity.Conversation, bool) {
	if !s.requireStore(w, s.cfg.ConversationStore, "conversations not configured") {
		return nil, false
	}
	conv, err := s.cfg.ConversationStore.GetConversation(r.Context(), conversationID)
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
