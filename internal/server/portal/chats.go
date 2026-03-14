package portal

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	chatapp "buildmax/internal/app/chat"
	convapp "buildmax/internal/app/conversation"
	"buildmax/internal/server/httputil"
	"buildmax/internal/storage/blob"
	"buildmax/internal/storage/entity"
)

type ChatResponse struct {
	ID             string  `json:"id"`
	WorkspaceID    string  `json:"workspace_id"`
	SessionID      *string `json:"session_id,omitempty"`
	Status         string  `json:"status"`
	Input          string  `json:"input"`
	Title          string  `json:"title,omitempty"`
	Output         *string `json:"output,omitempty"`
	CreatedBy      string  `json:"created_by"`
	CreatedAt      int64   `json:"created_at"`
	StartedAt      *int64  `json:"started_at,omitempty"`
	EndedAt        *int64  `json:"ended_at,omitempty"`
	ErrorMessage   *string `json:"error_message,omitempty"`
	AgentID        *string `json:"agent_id,omitempty"`
	ConversationID *string `json:"conversation_id,omitempty"`
}

type createChatRequest struct {
	Input   string  `json:"input"`
	AgentID *string `json:"agent_id,omitempty"`
}

func chatToResponse(c entity.Chat) ChatResponse {
	return ChatResponse{
		ID:             c.ChatID,
		WorkspaceID:    c.WorkspaceID,
		SessionID:      c.SessionID,
		Status:         c.Status,
		Input:          c.Input,
		Title:          c.Title,
		Output:         c.Output,
		CreatedBy:      c.CreatedBy,
		CreatedAt:      c.CreatedAt,
		StartedAt:      c.StartedAt,
		EndedAt:        c.EndedAt,
		ErrorMessage:   c.ErrorMessage,
		AgentID:        c.AgentID,
		ConversationID: c.ConversationID,
	}
}

func (h *Handler) getChatForWorkspace(w http.ResponseWriter, r *http.Request, workspaceID, chatID string) (*entity.Chat, bool) {
	if !h.requireStore(w, h.cfg.ChatStore, "chats not configured") {
		return nil, false
	}
	chat, err := h.cfg.ChatStore.GetChat(r.Context(), chatID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "get_chat", "chat_id", chatID)
		return nil, false
	}
	if chat == nil || chat.WorkspaceID != workspaceID {
		httputil.WriteJSONError(w, http.StatusNotFound, "chat not found")
		return nil, false
	}
	return chat, true
}

type chatsListResponse struct {
	Chats []ChatResponse `json:"chats"`
	Total int            `json:"total"`
}

func (h *Handler) listWorkspaceChatsHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.withWorkspaceAndStore(w, r, "workspace_id", h.cfg.ChatStore, "chats not configured")
	if !ok {
		return
	}
	q := r.URL.Query()
	usePaginated := q.Has("limit") || q.Has("offset") || q.Get("executed_only") == "true"
	if usePaginated {
		limit, offset := parseLimitOffset(q, "limit", "offset", 50, 200)
		executedOnly := q.Get("executed_only") == "true"
		list, total, err := h.cfg.ChatStore.ListChatsByWorkspacePaginated(r.Context(), workspaceID, executedOnly, limit, offset)
		if err != nil {
			httputil.WriteInternalError(w, err, "portal handler error", "handler", "list_chats", "workspace_id", workspaceID)
			return
		}
		out := make([]ChatResponse, len(list))
		for i := range list {
			out[i] = chatToResponse(list[i])
		}
		httputil.WriteJSON(w, http.StatusOK, chatsListResponse{Chats: out, Total: total})
		return
	}
	order := q.Get("order")
	if order != "asc" && order != "desc" {
		order = "desc"
	}
	list, err := h.cfg.ChatStore.ListChatsByWorkspace(r.Context(), workspaceID, order)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "list_chats", "workspace_id", workspaceID)
		return
	}
	out := make([]ChatResponse, len(list))
	for i := range list {
		out[i] = chatToResponse(list[i])
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) createWorkspaceChatHandler(w http.ResponseWriter, r *http.Request) {
	userID, workspaceID, ok := h.withWorkspaceAndStore(w, r, "workspace_id", h.cfg.ChatStore, "chats not configured")
	if !ok {
		return
	}
	var req createChatRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	chat, err := h.chatService().CreateChat(r.Context(), chatapp.CreateChatCmd{
		WorkspaceID: workspaceID,
		UserID:      userID,
		Input:       req.Input,
		AgentID:     req.AgentID,
	})
	if err != nil {
		if h.writeChatServiceError(w, r, err, req.AgentID) {
			return
		}
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "create_chat", "workspace_id", workspaceID)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, chatToResponse(*chat))
}

type createChatRunRequest struct {
	Input string `json:"input"`
}

// createChatRunViaConversation handles the Tier 1 conversation path; returns true if it wrote a response.
func (h *Handler) createChatRunViaConversation(w http.ResponseWriter, r *http.Request, userID, workspaceID, chatID, input string) bool {
	result, err := h.conversationService().HandleTurn(r.Context(), convapp.HandleTurnCmd{
		WorkspaceID: workspaceID,
		UserID:      userID,
		Channel:     "portal",
		Message:     input,
		ChatID:      chatID,
	})
	if err != nil {
		if h.writeConversationServiceError(w, r, err, nil) {
			return true
		}
		if errors.Is(err, entity.ErrRunInProgress) {
			httputil.WriteJSONError(w, http.StatusConflict, "a run is already in progress for this chat")
			return true
		}
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "create_run", "chat_id", chatID)
		return true
	}
	if len(result.TaskIDs) == 0 {
		httputil.WriteJSONError(w, http.StatusInternalServerError, "no run created")
		return true
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]string{"chat_run_id": result.TaskIDs[0], "chat_id": chatID})
	return true
}

func (h *Handler) createChatRunHandler(w http.ResponseWriter, r *http.Request) {
	userID, workspaceID, ok := h.withWorkspaceAndStore(w, r, "workspace_id", h.cfg.ChatRunStore, "chat runs not configured")
	if !ok {
		return
	}
	chatID, ok := pathValueRequired(w, r, "chat_id")
	if !ok {
		return
	}
	_, ok = h.getChatForWorkspace(w, r, workspaceID, chatID)
	if !ok {
		return
	}
	var req createChatRunRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Input == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "input required")
		return
	}
	h.createChatRunViaConversation(w, r, userID, workspaceID, chatID, req.Input)
}

type SessionMessage struct {
	Role       string            `json:"role"`
	Content    string            `json:"content,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
	ToolCalls  []SessionToolCall `json:"tool_calls,omitempty"`
}

type SessionToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

type ConversationResponse struct {
	ID        string           `json:"id"`
	Title     string           `json:"title,omitempty"`
	CreatedAt string           `json:"created_at"`
	Messages  []SessionMessage `json:"messages,omitempty"`
}

func (h *Handler) getChatConversationHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	chatID := r.PathValue("chat_id")
	if chatID == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "chat_id required")
		return
	}
	chat, ok := h.getChatForWorkspace(w, r, workspaceID, chatID)
	if !ok {
		return
	}
	if chat.SessionID == nil || *chat.SessionID == "" {
		httputil.WriteJSONError(w, http.StatusNotFound, "conversation not found")
		return
	}
	if chat.LastRunID == nil || *chat.LastRunID == "" {
		httputil.WriteJSONError(w, http.StatusNotFound, "conversation not found")
		return
	}
	sessionID := *chat.SessionID
	lastRunID := *chat.LastRunID
	data, err := h.loadChatConversationData(r.Context(), chat, lastRunID, sessionID)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, blob.ErrNotFound) {
			httputil.WriteJSONError(w, http.StatusNotFound, "conversation file not found")
			return
		}
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "get_conversation", "chat_id", chat.ChatID)
		return
	}
	var out ConversationResponse
	if err := json.Unmarshal(data, &out); err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "get_conversation", "chat_id", chat.ChatID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) loadChatConversationData(ctx context.Context, chat *entity.Chat, lastRunID, sessionID string) ([]byte, error) {
	relPath := "sessions/" + sessionID + ".json"
	if h.cfg.PersistStorage != nil {
		data, err := h.cfg.PersistStorage.GetChatGlobal(ctx, blob.RunObjectRef{
			WorkspaceID: chat.WorkspaceID,
			ChatID:      chat.ChatID,
			ChatRunID:   lastRunID,
			RelPath:     relPath,
		})
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, blob.ErrNotFound) {
			return nil, err
		}
	}
	localPath := filepath.Join(h.workspacesDir(), chat.WorkspaceID, "chats", chat.ChatID, lastRunID, "global", "sessions", sessionID+".json")
	return os.ReadFile(localPath)
}
