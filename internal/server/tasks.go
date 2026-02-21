package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"buildmax/internal/model"
	"buildmax/internal/storage/blob"
	"buildmax/internal/storage/entity"
)

// TaskResponse is one task in the list/create response (snake_case).
type TaskResponse struct {
	ID               string  `json:"id"`
	WorkspaceID      string  `json:"workspace_id"`
	ProjectID        *string `json:"project_id,omitempty"`
	SessionID        *string `json:"session_id,omitempty"`
	Status           string  `json:"status"`
	Input            string  `json:"input"`
	Title            string  `json:"title,omitempty"`
	Output           *string `json:"output,omitempty"`
	CreatedBy        string  `json:"created_by"`
	CreatedAt        int64   `json:"created_at"`
	StartedAt        *int64  `json:"started_at,omitempty"`
	EndedAt          *int64  `json:"ended_at,omitempty"`
	ErrorMessage     *string `json:"error_message,omitempty"`
}

// createTaskRequest is the JSON body for POST /api/workspaces/{workspace_id}/tasks.
type createTaskRequest struct {
	Input     string `json:"input"`
	ProjectID string `json:"project_id"`
}

func taskToResponse(t model.Task) TaskResponse {
	return TaskResponse{
		ID:              t.TaskID,
		WorkspaceID:     t.WorkspaceID,
		ProjectID:       t.ProjectID,
		SessionID:       t.SessionID,
		Status:          t.Status,
		Input:           t.Input,
		Title:           t.Title,
		Output:          t.Output,
		CreatedBy:       t.CreatedBy,
		CreatedAt:       t.CreatedAt,
		StartedAt:       t.StartedAt,
		EndedAt:         t.EndedAt,
		ErrorMessage:    t.ErrorMessage,
	}
}

// getTaskForWorkspace loads the task by taskID and verifies it belongs to workspaceID.
// Writes 404 "task not found" if the task is missing or in another workspace. Returns (task, true) on success.
func (s *Server) getTaskForWorkspace(w http.ResponseWriter, r *http.Request, workspaceID, taskID string) (*model.Task, bool) {
	if !s.requireStore(w, s.cfg.TaskStore, "tasks not configured") {
		return nil, false
	}
	task, err := s.cfg.TaskStore.GetTask(r.Context(), taskID)
	if err != nil {
		writeInternalError(w, err, "handler", "get_task", "task_id", taskID)
		return nil, false
	}
	if task == nil || task.WorkspaceID != workspaceID {
		writeJSONError(w, http.StatusNotFound, "task not found")
		return nil, false
	}
	return task, true
}

// resolveProjectForWorkspace resolves projectID (optional). If empty, returns (nil, true).
// If non-empty, validates project exists and belongs to workspace; on success returns (project, true),
// on failure writes the appropriate error and returns (nil, false).
func (s *Server) resolveProjectForWorkspace(w http.ResponseWriter, r *http.Request, workspaceID, projectID string) (*model.Project, bool) {
	if projectID == "" {
		return nil, true
	}
	if !s.requireStore(w, s.cfg.ProjectStore, "projects not configured") {
		return nil, false
	}
	project, err := s.cfg.ProjectStore.GetProject(r.Context(), projectID)
	if err != nil {
		writeInternalError(w, err, "handler", "resolve_project", "project_id", projectID)
		return nil, false
	}
	if project == nil {
		writeJSONError(w, http.StatusNotFound, "project not found")
		return nil, false
	}
	if project.WorkspaceID != workspaceID {
		writeJSONError(w, http.StatusBadRequest, "project does not belong to workspace")
		return nil, false
	}
	return project, true
}

// tasksListResponse is the paginated response for GET .../tasks when limit/offset/executed_only are used (snake_case).
type tasksListResponse struct {
	Tasks []TaskResponse `json:"tasks"`
	Total int            `json:"total"`
}

// listWorkspaceTasksHandler handles GET /api/workspaces/{workspace_id}/tasks.
// Optional: project_id, limit, offset, executed_only (when true, only tasks that have been run), order (asc or desc; default desc = latest first).
// When limit, offset, or executed_only are set, response is { "tasks": [...], "total": N } ordered by created_at DESC.
func (s *Server) listWorkspaceTasksHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireStore(w, s.cfg.TaskStore, "tasks not configured") {
		return
	}
	var projectIDPtr *string
	if pid := r.URL.Query().Get("project_id"); pid != "" {
		project, ok := s.resolveProjectForWorkspace(w, r, workspaceID, pid)
		if !ok {
			return
		}
		if project != nil {
			projectIDPtr = &project.ProjectID
		}
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
		list, total, err := s.cfg.TaskStore.ListTasksByWorkspacePaginated(r.Context(), workspaceID, projectIDPtr, executedOnly, limit, offset)
		if err != nil {
			writeInternalError(w, err, "handler", "list_tasks", "workspace_id", workspaceID)
			return
		}
		out := make([]TaskResponse, len(list))
		for i := range list {
			out[i] = taskToResponse(list[i])
		}
		writeJSON(w, http.StatusOK, tasksListResponse{Tasks: out, Total: total})
		return
	}
	order := q.Get("order")
	if order != "asc" && order != "desc" {
		order = "desc"
	}
	list, err := s.cfg.TaskStore.ListTasksByWorkspace(r.Context(), workspaceID, projectIDPtr, order)
	if err != nil {
		writeInternalError(w, err, "handler", "list_tasks", "workspace_id", workspaceID)
		return
	}
	out := make([]TaskResponse, len(list))
	for i := range list {
		out[i] = taskToResponse(list[i])
	}
	writeJSON(w, http.StatusOK, out)
}

// truncateTaskTitle returns the first maxRunes runes of input; if longer, appends "…".
func truncateTaskTitle(input string, maxRunes int) string {
	runes := []rune(input)
	if len(runes) <= maxRunes {
		return input
	}
	return string(runes[:maxRunes]) + "…"
}

// createWorkspaceTaskHandler handles POST /api/workspaces/{workspace_id}/tasks.
// Body: { "input": "…", "project_id": "optional" }.
func (s *Server) createWorkspaceTaskHandler(w http.ResponseWriter, r *http.Request) {
	userID, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireStore(w, s.cfg.TaskStore, "tasks not configured") {
		return
	}
	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Input == "" {
		writeJSONError(w, http.StatusBadRequest, "input required")
		return
	}
	title := truncateTaskTitle(req.Input, 50)
	if s.cfg.TaskTitleGenerator != nil {
		if gen, err := s.cfg.TaskTitleGenerator.GenerateTaskTitle(r.Context(), req.Input); err == nil && gen != "" {
			title = gen
		}
	}
	var projectIDPtr *string
	if req.ProjectID != "" {
		project, ok := s.resolveProjectForWorkspace(w, r, workspaceID, req.ProjectID)
		if !ok {
			return
		}
		if project != nil {
			projectIDPtr = &project.ProjectID
		}
	}
	task, err := s.cfg.TaskStore.CreateTask(r.Context(), workspaceID, projectIDPtr, req.Input, title, userID)
	if err != nil {
		writeInternalError(w, err, "handler", "create_task", "workspace_id", workspaceID)
		return
	}
	writeJSON(w, http.StatusCreated, taskToResponse(*task))
}

// createTaskRunRequest is the JSON body for POST /api/workspaces/{workspace_id}/tasks/{task_id}/runs.
type createTaskRunRequest struct {
	Input string `json:"input"`
}

// createTaskRunHandler handles POST /api/workspaces/{workspace_id}/tasks/{task_id}/runs. Creates a new run (follow-up). Returns 409 if a run is already in progress.
func (s *Server) createTaskRunHandler(w http.ResponseWriter, r *http.Request) {
	userID, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireStore(w, s.cfg.TaskRunStore, "task runs not configured") {
		return
	}
	taskID := r.PathValue("task_id")
	if taskID == "" {
		writeJSONError(w, http.StatusBadRequest, "task_id required")
		return
	}
	_, ok = s.getTaskForWorkspace(w, r, workspaceID, taskID)
	if !ok {
		return
	}
	var req createTaskRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Input == "" {
		writeJSONError(w, http.StatusBadRequest, "input required")
		return
	}
	run, err := s.cfg.TaskRunStore.CreateTaskRun(r.Context(), taskID, req.Input, userID)
	if err != nil {
		if errors.Is(err, entity.ErrRunInProgress) {
			writeJSONError(w, http.StatusConflict, "a run is already in progress for this task")
			return
		}
		writeInternalError(w, err, "handler", "create_run", "task_id", taskID)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"run_id": run.RunID, "task_id": taskID})
}

// Conversation types for GET .../tasks/{task_id}/conversation (snake_case for API).
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

// getTaskConversationHandler handles GET /api/workspaces/{workspace_id}/tasks/{task_id}/conversation.
// Returns the agent conversation for that task. Session is an implementation detail; the task stores session_id when run.
// Caller must own the workspace. Conversation is read from persist storage or server workspace dir.
func (s *Server) getTaskConversationHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	taskID := r.PathValue("task_id")
	if taskID == "" {
		writeJSONError(w, http.StatusBadRequest, "task_id required")
		return
	}
	task, ok := s.getTaskForWorkspace(w, r, workspaceID, taskID)
	if !ok {
		return
	}
	if task.SessionID == nil || *task.SessionID == "" {
		writeJSONError(w, http.StatusNotFound, "conversation not found")
		return
	}
	if task.LastRunID == nil || *task.LastRunID == "" {
		writeJSONError(w, http.StatusNotFound, "conversation not found")
		return
	}
	sessionID := *task.SessionID
	lastRunID := *task.LastRunID
	data, err := s.loadTaskConversationData(r.Context(), task, lastRunID, sessionID)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, blob.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "conversation file not found")
			return
		}
		writeInternalError(w, err, "handler", "get_conversation", "task_id", task.TaskID)
		return
	}
	var out ConversationResponse
	if err := json.Unmarshal(data, &out); err != nil {
		writeInternalError(w, err, "handler", "get_conversation", "task_id", task.TaskID)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// loadTaskConversationData returns the raw session JSON for the task run. Tries PersistStorage first, then local path under server's workspaces dir.
func (s *Server) loadTaskConversationData(ctx context.Context, task *model.Task, lastRunID, sessionID string) ([]byte, error) {
	relPath := "sessions/" + sessionID + ".json"
	if s.cfg.PersistStorage != nil {
		data, err := s.cfg.PersistStorage.GetTaskBuildmax(ctx, task.WorkspaceID, task.TaskID, lastRunID, relPath)
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, blob.ErrNotFound) {
			return nil, err
		}
	}
	localPath := filepath.Join(s.workspacesDir(), task.WorkspaceID, "tasks", task.TaskID, lastRunID, "buildmax", "sessions", sessionID+".json")
	return os.ReadFile(localPath)
}
