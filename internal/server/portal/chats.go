package portal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"buildmax/internal/conversation/adapter"
	"buildmax/internal/model"
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

func buildChatInputFromAgent(agent *model.Agent, userInput string) string {
	out := fmt.Sprintf("Agent: %s\nDescription: %s\nInstructions:\n%s", agent.Name, agent.Description, agent.Instructions)
	if userInput != "" {
		out = out + "\n\n" + userInput
	}
	return out
}

func chatToResponse(c model.Chat) ChatResponse {
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

func (h *Handler) getChatForWorkspace(w http.ResponseWriter, r *http.Request, workspaceID, chatID string) (*model.Chat, bool) {
	if !h.requireStore(w, h.cfg.ChatStore, "chats not configured") {
		return nil, false
	}
	chat, err := h.cfg.ChatStore.GetChat(r.Context(), chatID)
	if err != nil {
		writeInternalError(w, err, "handler", "get_chat", "chat_id", chatID)
		return nil, false
	}
	if chat == nil || chat.WorkspaceID != workspaceID {
		writeJSONError(w, http.StatusNotFound, "chat not found")
		return nil, false
	}
	return chat, true
}

type chatsListResponse struct {
	Chats []ChatResponse `json:"chats"`
	Total int            `json:"total"`
}

func (h *Handler) listWorkspaceChatsHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.ChatStore, "chats not configured") {
		return
	}
	q := r.URL.Query()
	usePaginated := q.Has("limit") || q.Has("offset") || q.Get("executed_only") == "true"
	if usePaginated {
		limit, offset := parseLimitOffset(q, "limit", "offset", 50, 200)
		executedOnly := q.Get("executed_only") == "true"
		list, total, err := h.cfg.ChatStore.ListChatsByWorkspacePaginated(r.Context(), workspaceID, executedOnly, limit, offset)
		if err != nil {
			writeInternalError(w, err, "handler", "list_chats", "workspace_id", workspaceID)
			return
		}
		out := make([]ChatResponse, len(list))
		for i := range list {
			out[i] = chatToResponse(list[i])
		}
		writeJSON(w, http.StatusOK, chatsListResponse{Chats: out, Total: total})
		return
	}
	order := q.Get("order")
	if order != "asc" && order != "desc" {
		order = "desc"
	}
	list, err := h.cfg.ChatStore.ListChatsByWorkspace(r.Context(), workspaceID, order)
	if err != nil {
		writeInternalError(w, err, "handler", "list_chats", "workspace_id", workspaceID)
		return
	}
	out := make([]ChatResponse, len(list))
	for i := range list {
		out[i] = chatToResponse(list[i])
	}
	writeJSON(w, http.StatusOK, out)
}

func truncateChatTitle(input string, maxRunes int) string {
	runes := []rune(input)
	if len(runes) <= maxRunes {
		return input
	}
	return string(runes[:maxRunes]) + "…"
}

func (h *Handler) createWorkspaceChatHandler(w http.ResponseWriter, r *http.Request) {
	userID, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.ChatStore, "chats not configured") {
		return
	}
	var req createChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var input string
	var agentID *string
	if req.AgentID != nil && *req.AgentID != "" {
		if h.cfg.AgentStore == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "agents not configured")
			return
		}
		agent, err := h.cfg.AgentStore.GetAgent(r.Context(), *req.AgentID)
		if err != nil {
			writeInternalError(w, err, "handler", "get_agent", "agent_id", *req.AgentID)
			return
		}
		if agent == nil || agent.WorkspaceID != workspaceID {
			writeJSONError(w, http.StatusBadRequest, "agent not found or not in workspace")
			return
		}
		if req.Input != "" {
			input = req.Input
		} else {
			input = buildChatInputFromAgent(agent, "")
		}
		agentID = req.AgentID
	} else {
		if req.Input == "" {
			writeJSONError(w, http.StatusBadRequest, "input required")
			return
		}
		input = req.Input
	}
	title := truncateChatTitle(input, 50)
	var titlePromptTokens, titleCompletionTokens int
	if h.cfg.ChatTitleGenerator != nil {
		if gen, usage, err := h.cfg.ChatTitleGenerator.GenerateChatTitle(r.Context(), input); err == nil && gen != "" {
			title = gen
			titlePromptTokens = usage.PromptTokens
			titleCompletionTokens = usage.CompletionTokens
		}
	}
	if h.cfg.QuotaChecker != nil {
		allowed, reason := h.cfg.QuotaChecker.Check(r.Context(), userID, 1, titlePromptTokens+titleCompletionTokens)
		if !allowed {
			writeQuotaExceeded(w, reason)
			return
		}
	}
	chat, err := h.cfg.ChatStore.CreateChat(r.Context(), workspaceID, input, title, userID, titlePromptTokens, titleCompletionTokens, agentID, nil)
	if err != nil {
		writeInternalError(w, err, "handler", "create_chat", "workspace_id", workspaceID)
		return
	}
	writeJSON(w, http.StatusCreated, chatToResponse(*chat))
}

type createChatRunRequest struct {
	Input string `json:"input"`
}

func (h *Handler) createChatRunHandler(w http.ResponseWriter, r *http.Request) {
	userID, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.ChatRunStore, "chat runs not configured") {
		return
	}
	chatID := r.PathValue("chat_id")
	if chatID == "" {
		writeJSONError(w, http.StatusBadRequest, "chat_id required")
		return
	}
	_, ok = h.getChatForWorkspace(w, r, workspaceID, chatID)
	if !ok {
		return
	}
	var req createChatRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Input == "" {
		writeJSONError(w, http.StatusBadRequest, "input required")
		return
	}
	if h.cfg.QuotaChecker != nil {
		allowed, reason := h.cfg.QuotaChecker.Check(r.Context(), userID, 1, 0)
		if !allowed {
			writeQuotaExceeded(w, reason)
			return
		}
	}
	if h.cfg.ConversationEngine != nil && h.cfg.PortalAdapter != nil {
		input := &adapter.PortalTurnInput{
			WorkspaceID: workspaceID,
			ChatID:      chatID,
			UserID:      userID,
			Message:     req.Input,
		}
		turn, err := h.cfg.PortalAdapter.Receive(r.Context(), input)
		if err != nil {
			writeInternalError(w, err, "handler", "receive_turn", "chat_id", chatID)
			return
		}
		result, err := h.cfg.ConversationEngine.Process(r.Context(), workspaceID, chatID, turn)
		if err != nil {
			if errors.Is(err, entity.ErrRunInProgress) {
				writeJSONError(w, http.StatusConflict, "a run is already in progress for this chat")
				return
			}
			writeInternalError(w, err, "handler", "create_run", "chat_id", chatID)
			return
		}
		if len(result.TaskIDs) == 0 {
			writeJSONError(w, http.StatusInternalServerError, "no run created")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"chat_run_id": result.TaskIDs[0], "chat_id": chatID})
		return
	}
	run, err := h.cfg.ChatRunStore.CreateChatRun(r.Context(), chatID, req.Input, userID)
	if err != nil {
		if errors.Is(err, entity.ErrRunInProgress) {
			writeJSONError(w, http.StatusConflict, "a run is already in progress for this chat")
			return
		}
		writeInternalError(w, err, "handler", "create_run", "chat_id", chatID)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"chat_run_id": run.ChatRunID, "chat_id": chatID})
}

type SessionMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
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
		writeJSONError(w, http.StatusBadRequest, "chat_id required")
		return
	}
	chat, ok := h.getChatForWorkspace(w, r, workspaceID, chatID)
	if !ok {
		return
	}
	if chat.SessionID == nil || *chat.SessionID == "" {
		writeJSONError(w, http.StatusNotFound, "conversation not found")
		return
	}
	if chat.LastRunID == nil || *chat.LastRunID == "" {
		writeJSONError(w, http.StatusNotFound, "conversation not found")
		return
	}
	sessionID := *chat.SessionID
	lastRunID := *chat.LastRunID
	data, err := h.loadChatConversationData(r.Context(), chat, lastRunID, sessionID)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, blob.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "conversation file not found")
			return
		}
		writeInternalError(w, err, "handler", "get_conversation", "chat_id", chat.ChatID)
		return
	}
	var out ConversationResponse
	if err := json.Unmarshal(data, &out); err != nil {
		writeInternalError(w, err, "handler", "get_conversation", "chat_id", chat.ChatID)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) loadChatConversationData(ctx context.Context, chat *model.Chat, lastRunID, sessionID string) ([]byte, error) {
	relPath := "sessions/" + sessionID + ".json"
	if h.cfg.PersistStorage != nil {
		data, err := h.cfg.PersistStorage.GetChatGlobal(ctx, chat.WorkspaceID, chat.ChatID, lastRunID, relPath)
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
