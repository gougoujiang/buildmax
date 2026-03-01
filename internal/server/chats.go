package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"buildmax/internal/model"
	"buildmax/internal/storage/blob"
	"buildmax/internal/storage/entity"
)

// ChatResponse is one chat in the list/create response (snake_case). ID is the chat_id.
type ChatResponse struct {
	ID           string  `json:"id"`
	WorkspaceID  string  `json:"workspace_id"`
	SessionID    *string `json:"session_id,omitempty"`
	Status       string  `json:"status"`
	Input        string  `json:"input"`
	Title        string  `json:"title,omitempty"`
	Output       *string `json:"output,omitempty"`
	CreatedBy    string  `json:"created_by"`
	CreatedAt    int64   `json:"created_at"`
	StartedAt    *int64  `json:"started_at,omitempty"`
	EndedAt      *int64  `json:"ended_at,omitempty"`
	ErrorMessage *string `json:"error_message,omitempty"`
	AgentID      *string `json:"agent_id,omitempty"`
}

// createChatRequest is the JSON body for POST /api/workspaces/{workspace_id}/chats.
type createChatRequest struct {
	Input   string  `json:"input"`
	AgentID *string `json:"agent_id,omitempty"`
}

// buildChatInputFromAgent builds the chat input string from an agent's name, description, and instructions.
// If userInput is non-empty, it is appended after a blank line.
func buildChatInputFromAgent(agent *model.Agent, userInput string) string {
	out := fmt.Sprintf("Agent: %s\nDescription: %s\nInstructions:\n%s", agent.Name, agent.Description, agent.Instructions)
	if userInput != "" {
		out = out + "\n\n" + userInput
	}
	return out
}

func chatToResponse(c model.Chat) ChatResponse {
	return ChatResponse{
		ID:           c.ChatID,
		WorkspaceID:  c.WorkspaceID,
		SessionID:    c.SessionID,
		Status:       c.Status,
		Input:        c.Input,
		Title:        c.Title,
		Output:       c.Output,
		CreatedBy:    c.CreatedBy,
		CreatedAt:    c.CreatedAt,
		StartedAt:    c.StartedAt,
		EndedAt:      c.EndedAt,
		ErrorMessage: c.ErrorMessage,
		AgentID:      c.AgentID,
	}
}

// getChatForWorkspace loads the chat by chatID and verifies it belongs to workspaceID.
// Writes 404 "chat not found" if the chat is missing or in another workspace. Returns (chat, true) on success.
func (s *Server) getChatForWorkspace(w http.ResponseWriter, r *http.Request, workspaceID, chatID string) (*model.Chat, bool) {
	if !s.requireStore(w, s.cfg.ChatStore, "chats not configured") {
		return nil, false
	}
	chat, err := s.cfg.ChatStore.GetChat(r.Context(), chatID)
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

// chatsListResponse is the paginated response for GET .../chats when limit/offset/executed_only are used (snake_case).
type chatsListResponse struct {
	Chats []ChatResponse `json:"chats"`
	Total int            `json:"total"`
}

// listWorkspaceChatsHandler handles GET /api/workspaces/{workspace_id}/chats.
// Optional: limit, offset, executed_only (when true, only chats that have been run), order (asc or desc; default desc = latest first).
// When limit, offset, or executed_only are set, response is { "chats": [...], "total": N } ordered by created_at DESC.
func (s *Server) listWorkspaceChatsHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireStore(w, s.cfg.ChatStore, "chats not configured") {
		return
	}
	q := r.URL.Query()
	usePaginated := q.Has("limit") || q.Has("offset") || q.Get("executed_only") == "true"
	if usePaginated {
		limit := 50
		if l := q.Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}
		offset := 0
		if o := q.Get("offset"); o != "" {
			if n, err := strconv.Atoi(o); err == nil && n >= 0 {
				offset = n
			}
		}
		executedOnly := q.Get("executed_only") == "true"
		list, total, err := s.cfg.ChatStore.ListChatsByWorkspacePaginated(r.Context(), workspaceID, executedOnly, limit, offset)
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
	list, err := s.cfg.ChatStore.ListChatsByWorkspace(r.Context(), workspaceID, order)
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

// truncateChatTitle returns the first maxRunes runes of input; if longer, appends "…".
func truncateChatTitle(input string, maxRunes int) string {
	runes := []rune(input)
	if len(runes) <= maxRunes {
		return input
	}
	return string(runes[:maxRunes]) + "…"
}

// createWorkspaceChatHandler handles POST /api/workspaces/{workspace_id}/chats.
// Body: { "input"?: "…", "agent_id"?: "…" }. When agent_id is present, input is optional and the stored input is composed from the agent; when agent_id is absent, input is required.
func (s *Server) createWorkspaceChatHandler(w http.ResponseWriter, r *http.Request) {
	userID, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireStore(w, s.cfg.ChatStore, "chats not configured") {
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
		if s.cfg.AgentStore == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "agents not configured")
			return
		}
		agent, err := s.cfg.AgentStore.GetAgent(r.Context(), *req.AgentID)
		if err != nil {
			writeInternalError(w, err, "handler", "get_agent", "agent_id", *req.AgentID)
			return
		}
		if agent == nil || agent.WorkspaceID != workspaceID {
			writeJSONError(w, http.StatusBadRequest, "agent not found or not in workspace")
			return
		}
		if req.Input != "" {
			input = req.Input // use client-provided input directly (e.g. user-edited instructions)
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
	if s.cfg.ChatTitleGenerator != nil {
		if gen, usage, err := s.cfg.ChatTitleGenerator.GenerateChatTitle(r.Context(), input); err == nil && gen != "" {
			title = gen
			titlePromptTokens = usage.PromptTokens
			titleCompletionTokens = usage.CompletionTokens
		}
	}
	if s.cfg.QuotaChecker != nil {
		allowed, reason := s.cfg.QuotaChecker.Check(r.Context(), userID, 1, titlePromptTokens+titleCompletionTokens)
		if !allowed {
			writeQuotaExceeded(w, reason)
			return
		}
	}
	chat, err := s.cfg.ChatStore.CreateChat(r.Context(), workspaceID, input, title, userID, titlePromptTokens, titleCompletionTokens, agentID)
	if err != nil {
		writeInternalError(w, err, "handler", "create_chat", "workspace_id", workspaceID)
		return
	}
	writeJSON(w, http.StatusCreated, chatToResponse(*chat))
}

// createChatRunRequest is the JSON body for POST /api/workspaces/{workspace_id}/chats/{chat_id}/runs.
type createChatRunRequest struct {
	Input string `json:"input"`
}

// createChatRunHandler handles POST /api/workspaces/{workspace_id}/chats/{chat_id}/runs. Creates a new run (follow-up). Returns 409 if a run is already in progress.
func (s *Server) createChatRunHandler(w http.ResponseWriter, r *http.Request) {
	userID, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireStore(w, s.cfg.ChatRunStore, "chat runs not configured") {
		return
	}
	chatID := r.PathValue("chat_id")
	if chatID == "" {
		writeJSONError(w, http.StatusBadRequest, "chat_id required")
		return
	}
	_, ok = s.getChatForWorkspace(w, r, workspaceID, chatID)
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
	if s.cfg.QuotaChecker != nil {
		allowed, reason := s.cfg.QuotaChecker.Check(r.Context(), userID, 1, 0)
		if !allowed {
			writeQuotaExceeded(w, reason)
			return
		}
	}
	if s.cfg.ConversationEngine != nil && s.cfg.PortalAdapter != nil {
		input := &PortalTurnInput{
			WorkspaceID: workspaceID,
			ChatID:      chatID,
			UserID:      userID,
			Message:     req.Input,
		}
		turn, err := s.cfg.PortalAdapter.Receive(r.Context(), input)
		if err != nil {
			writeInternalError(w, err, "handler", "receive_turn", "chat_id", chatID)
			return
		}
		result, err := s.cfg.ConversationEngine.Process(r.Context(), workspaceID, chatID, turn)
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
	run, err := s.cfg.ChatRunStore.CreateChatRun(r.Context(), chatID, req.Input, userID)
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

// Conversation types for GET .../chats/{chat_id}/conversation (snake_case for API).
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

// getChatConversationHandler handles GET /api/workspaces/{workspace_id}/chats/{chat_id}/conversation.
// Returns the agent conversation for that chat. Session is an implementation detail; the chat stores session_id when run.
// Caller must own the workspace. Conversation is read from persist storage or server workspace dir.
func (s *Server) getChatConversationHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	chatID := r.PathValue("chat_id")
	if chatID == "" {
		writeJSONError(w, http.StatusBadRequest, "chat_id required")
		return
	}
	chat, ok := s.getChatForWorkspace(w, r, workspaceID, chatID)
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
	data, err := s.loadChatConversationData(r.Context(), chat, lastRunID, sessionID)
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

// loadChatConversationData returns the raw session JSON for the chat run. Tries PersistStorage first, then local path under server's workspaces dir.
func (s *Server) loadChatConversationData(ctx context.Context, chat *model.Chat, lastRunID, sessionID string) ([]byte, error) {
	relPath := "sessions/" + sessionID + ".json"
	if s.cfg.PersistStorage != nil {
		data, err := s.cfg.PersistStorage.GetChatGlobal(ctx, chat.WorkspaceID, chat.ChatID, lastRunID, relPath)
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, blob.ErrNotFound) {
			return nil, err
		}
	}
	localPath := filepath.Join(s.workspacesDir(), chat.WorkspaceID, "chats", chat.ChatID, lastRunID, "global", "sessions", sessionID+".json")
	return os.ReadFile(localPath)
}
