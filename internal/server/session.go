package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"buildmax/internal/config"
)

// SessionMessage is one message in a session (snake_case for API).
type SessionMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	ToolCalls  []SessionToolCall `json:"tool_calls,omitempty"`
}

// SessionToolCall is a tool invocation in an assistant message.
type SessionToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

// SessionResponse is the GET /api/sessions/{session_id} response (matches session file shape, snake_case).
type SessionResponse struct {
	ID        string           `json:"id"`
	Title     string           `json:"title,omitempty"`
	CreatedAt string           `json:"created_at"`
	Messages  []SessionMessage `json:"messages,omitempty"`
}

// getSessionHandler handles GET /api/sessions/{session_id}.
// Returns the agent session content (conversation) from BUILDMAX_HOME/sessions/<session_id>.json.
// Caller must be authenticated and own a task that has this session_id.
func (s *Server) getSessionHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r, s.cfg.JWTSecret)
	if !ok {
		return
	}
	sessionID := r.PathValue("session_id")
	if sessionID == "" {
		writeJSONError(w, http.StatusBadRequest, "session_id required")
		return
	}
	if s.cfg.SessionsDir == "" {
		writeJSONError(w, http.StatusServiceUnavailable, "sessions not configured")
		return
	}
	if s.cfg.TaskStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "tasks not configured")
		return
	}

	task, err := s.cfg.TaskStore.GetTaskBySessionID(r.Context(), sessionID)
	if err != nil {
		writeInternalError(w, err, "handler", "get_session", "session_id", sessionID)
		return
	}
	if task == nil {
		writeJSONError(w, http.StatusNotFound, "session not found")
		return
	}
	owned, err := s.userOwnsWorkspace(r, userID, task.WorkspaceID)
	if err != nil {
		writeInternalError(w, err, "handler", "get_session", "workspace_id", task.WorkspaceID)
		return
	}
	if !owned {
		writeJSONError(w, http.StatusForbidden, "forbidden")
		return
	}
	taskSessionPath := filepath.Join(config.RuntimeTaskBuildmaxDir(task.WorkspaceID, task.TaskID), "sessions", sessionID+".json")
	data, err := os.ReadFile(taskSessionPath)
	if err != nil && os.IsNotExist(err) && s.cfg.SessionsDir != "" {
		path := filepath.Join(s.cfg.SessionsDir, sessionID+".json")
		data, err = os.ReadFile(path)
	}
	if err != nil {
		if os.IsNotExist(err) {
			writeJSONError(w, http.StatusNotFound, "session file not found")
			return
		}
		writeInternalError(w, err, "handler", "get_session", "session_id", sessionID)
		return
	}
	var out SessionResponse
	if err := json.Unmarshal(data, &out); err != nil {
		writeInternalError(w, err, "handler", "get_session", "session_id", sessionID)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
