package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"buildmax/internal/config"
	"buildmax/internal/model"
)

// TaskResponse is one task in the list/create response (snake_case).
type TaskResponse struct {
	ID           string  `json:"id"`
	WorkspaceID  string  `json:"workspace_id"`
	ProjectID    *string `json:"project_id,omitempty"`
	SessionID    *string `json:"session_id,omitempty"`
	Status       string  `json:"status"`
	Input        string  `json:"input"`
	Output       *string `json:"output,omitempty"`
	CreatedBy    string  `json:"created_by"`
	CreatedAt    int64   `json:"created_at"`
	StartedAt    *int64  `json:"started_at,omitempty"`
	EndedAt      *int64  `json:"ended_at,omitempty"`
	ErrorMessage *string `json:"error_message,omitempty"`
}

// createTaskRequest is the JSON body for POST /api/workspaces/{workspace_id}/tasks.
type createTaskRequest struct {
	Input     string `json:"input"`
	ProjectID string `json:"project_id"`
}

func taskToResponse(t model.Task) TaskResponse {
	return TaskResponse{
		ID:           t.TaskID,
		WorkspaceID:  t.WorkspaceID,
		ProjectID:    t.ProjectID,
		SessionID:    t.SessionID,
		Status:       t.Status,
		Input:        t.Input,
		Output:       t.Output,
		CreatedBy:    t.CreatedBy,
		CreatedAt:    t.CreatedAt,
		StartedAt:    t.StartedAt,
		EndedAt:      t.EndedAt,
		ErrorMessage: t.ErrorMessage,
	}
}

// resolveProjectForWorkspace resolves projectID (optional). If empty, returns (nil, true).
// If non-empty, validates project exists and belongs to workspace; on success returns (project, true),
// on failure writes the appropriate error and returns (nil, false).
func (s *Server) resolveProjectForWorkspace(w http.ResponseWriter, r *http.Request, workspaceID, projectID string) (*model.Project, bool) {
	if projectID == "" {
		return nil, true
	}
	if !s.requireProjectStore(w) {
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

// listWorkspaceTasksHandler handles GET /api/workspaces/{workspace_id}/tasks.
// Optional query param project_id filters by project (validates project belongs to workspace).
func (s *Server) listWorkspaceTasksHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireTaskStore(w) {
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
	list, err := s.cfg.TaskStore.ListTasksByWorkspace(r.Context(), workspaceID, projectIDPtr)
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

// createWorkspaceTaskHandler handles POST /api/workspaces/{workspace_id}/tasks.
// Body: { "input": "…", "project_id": "optional" }.
func (s *Server) createWorkspaceTaskHandler(w http.ResponseWriter, r *http.Request) {
	userID, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireTaskStore(w) {
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
	task, err := s.cfg.TaskStore.CreateTask(r.Context(), workspaceID, projectIDPtr, req.Input, userID)
	if err != nil {
		writeInternalError(w, err, "handler", "create_task", "workspace_id", workspaceID)
		return
	}
	writeJSON(w, http.StatusCreated, taskToResponse(*task))
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
// Caller must own the workspace. Conversation is read from task's buildmax dir or global sessions dir.
func (s *Server) getTaskConversationHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireTaskStore(w) {
		return
	}
	taskID := r.PathValue("task_id")
	if taskID == "" {
		writeJSONError(w, http.StatusBadRequest, "task_id required")
		return
	}

	task, err := s.cfg.TaskStore.GetTask(r.Context(), taskID)
	if err != nil {
		writeInternalError(w, err, "handler", "get_conversation", "task_id", taskID)
		return
	}
	if task == nil {
		writeJSONError(w, http.StatusNotFound, "task not found")
		return
	}
	if task.WorkspaceID != workspaceID {
		writeJSONError(w, http.StatusNotFound, "task not found")
		return
	}
	if task.SessionID == nil || *task.SessionID == "" {
		writeJSONError(w, http.StatusNotFound, "conversation not found")
		return
	}
	sessionID := *task.SessionID

	taskSessionPath := filepath.Join(config.RuntimeTaskBuildmaxDir(task.WorkspaceID, task.TaskID), "sessions", sessionID+".json")
	data, err := os.ReadFile(taskSessionPath)
	if err != nil && os.IsNotExist(err) && s.cfg.SessionsDir != "" {
		path := filepath.Join(s.cfg.SessionsDir, sessionID+".json")
		data, err = os.ReadFile(path)
	}
	if err != nil {
		if os.IsNotExist(err) {
			writeJSONError(w, http.StatusNotFound, "conversation file not found")
			return
		}
		writeInternalError(w, err, "handler", "get_conversation", "task_id", taskID)
		return
	}
	var out ConversationResponse
	if err := json.Unmarshal(data, &out); err != nil {
		writeInternalError(w, err, "handler", "get_conversation", "task_id", taskID)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
