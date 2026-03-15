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

type TaskResponse struct {
	ID             string  `json:"id"`
	ConversationID string  `json:"conversation_id"`
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
}

type createTaskRequest struct {
	Input   string  `json:"input"`
	AgentID *string `json:"agent_id,omitempty"`
}

func taskToResponse(task entity.Chat) TaskResponse {
	return TaskResponse{
		ID:             task.ChatID,
		ConversationID: task.ConversationID,
		SessionID:      task.SessionID,
		Status:         task.Status,
		Input:          task.Input,
		Title:          task.Title,
		Output:         task.Output,
		CreatedBy:      task.CreatedBy,
		CreatedAt:      task.CreatedAt,
		StartedAt:      task.StartedAt,
		EndedAt:        task.EndedAt,
		ErrorMessage:   task.ErrorMessage,
		AgentID:        task.AgentID,
	}
}

func (h *Handler) getTaskForConversation(w http.ResponseWriter, r *http.Request, conversationID, taskID string) (*entity.Chat, bool) {
	if !h.requireStore(w, h.cfg.TaskStore, "tasks not configured") {
		return nil, false
	}
	task, err := h.cfg.TaskStore.GetChat(r.Context(), taskID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "get_task", "task_id", taskID)
		return nil, false
	}
	if task == nil || task.ConversationID != conversationID {
		httputil.WriteJSONError(w, http.StatusNotFound, "task not found")
		return nil, false
	}
	return task, true
}

func (h *Handler) getTaskForUser(w http.ResponseWriter, r *http.Request, userID, taskID string) (*entity.Chat, *entity.Conversation, bool) {
	task, err := h.cfg.TaskStore.GetChat(r.Context(), taskID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "get_task", "task_id", taskID)
		return nil, nil, false
	}
	if task == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "task not found")
		return nil, nil, false
	}
	conv, ok := h.getConversationForUser(w, r, userID, task.ConversationID)
	if !ok {
		return nil, nil, false
	}
	return task, conv, true
}

type tasksListResponse struct {
	Tasks []TaskResponse `json:"tasks"`
	Total int            `json:"total"`
}

func (h *Handler) listConversationTasksHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.withUserAndStore(w, r, h.cfg.TaskStore, "tasks not configured")
	if !ok {
		return
	}
	conversationID, ok := pathValueRequired(w, r, "conversation_id")
	if !ok {
		return
	}
	if _, ok = h.getConversationForUser(w, r, userID, conversationID); !ok {
		return
	}
	q := r.URL.Query()
	usePaginated := q.Has("limit") || q.Has("offset") || q.Get("executed_only") == "true"
	if usePaginated {
		limit, offset := parseLimitOffset(q, "limit", "offset", 50, 200)
		executedOnly := q.Get("executed_only") == "true"
		list, total, err := h.cfg.TaskStore.ListChatsByConversationPaginated(r.Context(), conversationID, executedOnly, limit, offset)
		if err != nil {
			httputil.WriteInternalError(w, err, "portal handler error", "handler", "list_tasks", "conversation_id", conversationID)
			return
		}
		out := make([]TaskResponse, len(list))
		for i := range list {
			out[i] = taskToResponse(list[i])
		}
		httputil.WriteJSON(w, http.StatusOK, tasksListResponse{Tasks: out, Total: total})
		return
	}
	order := q.Get("order")
	if order != "asc" && order != "desc" {
		order = "desc"
	}
	list, err := h.cfg.TaskStore.ListChatsByConversation(r.Context(), conversationID, order)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "list_tasks", "conversation_id", conversationID)
		return
	}
	out := make([]TaskResponse, len(list))
	for i := range list {
		out[i] = taskToResponse(list[i])
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) createConversationTaskHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.withUserAndStore(w, r, h.cfg.TaskStore, "tasks not configured")
	if !ok {
		return
	}
	conversationID, ok := pathValueRequired(w, r, "conversation_id")
	if !ok {
		return
	}
	if _, ok = h.getConversationForUser(w, r, userID, conversationID); !ok {
		return
	}
	var req createTaskRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	task, err := h.taskService().CreateChat(r.Context(), chatapp.CreateChatCmd{
		ConversationID: conversationID,
		UserID:         userID,
		Input:          req.Input,
		AgentID:        req.AgentID,
	})
	if err != nil {
		if h.writeTaskServiceError(w, r, err, req.AgentID) {
			return
		}
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "create_task", "conversation_id", conversationID)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, taskToResponse(*task))
}

func (h *Handler) getTaskHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.withUserAndStore(w, r, h.cfg.TaskStore, "tasks not configured")
	if !ok {
		return
	}
	taskID, ok := pathValueRequired(w, r, "task_id")
	if !ok {
		return
	}
	task, _, ok := h.getTaskForUser(w, r, userID, taskID)
	if !ok {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, taskToResponse(*task))
}

type createTaskRunRequest struct {
	Input string `json:"input"`
}

// createTaskRunViaConversation handles the Tier 1 conversation path; returns true if it wrote a response.
func (h *Handler) createTaskRunViaConversation(w http.ResponseWriter, r *http.Request, userID, conversationID, taskID, input string) bool {
	result, err := h.conversationService().HandleTurn(r.Context(), convapp.HandleTurnCmd{
		UserID:  userID,
		Channel: "portal",
		Message: input,
		ChatID:  taskID,
	})
	if err != nil {
		if h.writeConversationServiceError(w, r, err, nil) {
			return true
		}
		if errors.Is(err, entity.ErrRunInProgress) {
			httputil.WriteJSONError(w, http.StatusConflict, "a run is already in progress for this task")
			return true
		}
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "create_run", "task_id", taskID)
		return true
	}
	if len(result.TaskIDs) == 0 {
		httputil.WriteJSONError(w, http.StatusInternalServerError, "no run created")
		return true
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]string{"task_run_id": result.TaskIDs[0], "task_id": taskID})
	return true
}

func (h *Handler) createTaskRunHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.withUserAndStore(w, r, h.cfg.TaskRunStore, "task runs not configured")
	if !ok {
		return
	}
	taskID, ok := pathValueRequired(w, r, "task_id")
	if !ok {
		return
	}
	task, conv, ok := h.getTaskForUser(w, r, userID, taskID)
	if !ok {
		return
	}
	var req createTaskRunRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Input == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "input required")
		return
	}
	h.createTaskRunViaConversation(w, r, userID, conv.ConversationID, task.ChatID, req.Input)
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

func (h *Handler) getTaskConversationHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.withUserAndStore(w, r, h.cfg.TaskStore, "tasks not configured")
	if !ok {
		return
	}
	taskID := r.PathValue("task_id")
	if taskID == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "task_id required")
		return
	}
	task, _, ok := h.getTaskForUser(w, r, userID, taskID)
	if !ok {
		return
	}
	if task.SessionID == nil || *task.SessionID == "" {
		httputil.WriteJSONError(w, http.StatusNotFound, "conversation not found")
		return
	}
	if task.LastRunID == nil || *task.LastRunID == "" {
		httputil.WriteJSONError(w, http.StatusNotFound, "conversation not found")
		return
	}
	sessionID := *task.SessionID
	lastRunID := *task.LastRunID
	data, err := h.loadTaskConversationData(r.Context(), task, lastRunID, sessionID)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, blob.ErrNotFound) {
			httputil.WriteJSONError(w, http.StatusNotFound, "conversation file not found")
			return
		}
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "get_conversation", "task_id", task.ChatID)
		return
	}
	var out ConversationResponse
	if err := json.Unmarshal(data, &out); err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "get_conversation", "task_id", task.ChatID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) loadTaskConversationData(ctx context.Context, task *entity.Chat, lastRunID, sessionID string) ([]byte, error) {
	relPath := "sessions/" + sessionID + ".json"
	if h.cfg.PersistStorage != nil {
		data, err := h.cfg.PersistStorage.GetChatGlobal(ctx, blob.RunObjectRef{
			UserID:         task.CreatedBy,
			ConversationID: task.ConversationID,
			ChatID:         task.ChatID,
			TaskRunID:      lastRunID,
			RelPath:        relPath,
		})
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, blob.ErrNotFound) {
			return nil, err
		}
	}
	localPath := filepath.Join(h.workspacesDir(), task.CreatedBy, "conversations", task.ConversationID, "tasks", task.ChatID, lastRunID, "global", "sessions", sessionID+".json")
	return os.ReadFile(localPath)
}
