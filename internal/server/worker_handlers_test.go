package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"buildmax/internal/storage/entity"
)

func TestGetWorkerTaskHandler_RequiresWorkerAuth(t *testing.T) {
	secret := "test-worker-secret"
	taskID := "task-1"
	task := entity.Task{
		TaskID: taskID, WorkspaceID: "ws1", Status: "PENDING", Input: "input",
		CreatedBy: "u1", CreatedAt: 1,
	}
	mockTask := &mockTaskStore{list: []entity.Task{task}}
	cfg := Config{
		JWTSecret:   secret,
		TaskStore:   mockTask,
		WorkerToken: "worker-token-123",
	}
	s := New(cfg)
	handler := s.workerAuthMiddleware(http.HandlerFunc(s.getWorkerTaskHandler))

	// Without token: 401
	req := httptest.NewRequest(http.MethodGet, "/api/worker/tasks/"+taskID, nil)
	req.SetPathValue("task_id", taskID)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("without token: got status %d, want 401", w.Code)
	}

	// With wrong token: 401
	req = httptest.NewRequest(http.MethodGet, "/api/worker/tasks/"+taskID, nil)
	req.SetPathValue("task_id", taskID)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("wrong token: got status %d, want 401", w.Code)
	}

	// With correct token: 200 and task body
	req = httptest.NewRequest(http.MethodGet, "/api/worker/tasks/"+taskID, nil)
	req.SetPathValue("task_id", taskID)
	req.Header.Set("Authorization", "Bearer worker-token-123")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("with token: got status %d, want 200", w.Code)
	}
	if body := w.Body.String(); body == "" || len(body) < 10 {
		t.Errorf("with token: body too short: %q", body)
	}
}

func TestGetWorkerTaskHandler_NotFound(t *testing.T) {
	cfg := Config{
		JWTSecret:   "secret",
		TaskStore:   &mockTaskStore{list: []entity.Task{}},
		WorkerToken: "token",
	}
	s := New(cfg)
	handler := s.workerAuthMiddleware(http.HandlerFunc(s.getWorkerTaskHandler))
	req := httptest.NewRequest(http.MethodGet, "/api/worker/tasks/nonexistent", nil)
	req.SetPathValue("task_id", "nonexistent")
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestPatchWorkerTask_RUNNING_WhenScheduled_Returns200(t *testing.T) {
	taskID := "task-scheduled"
	task := entity.Task{
		TaskID: taskID, WorkspaceID: "ws1", Status: "SCHEDULED", Input: "input",
		CreatedBy: "u1", CreatedAt: 1,
	}
	mockTask := &mockTaskStore{list: []entity.Task{task}}
	cfg := Config{
		JWTSecret:   "secret",
		TaskStore:   mockTask,
		WorkerToken: "token",
	}
	s := New(cfg)
	handler := s.workerAuthMiddleware(http.HandlerFunc(s.patchWorkerTaskHandler))

	body := map[string]interface{}{"status": "RUNNING", "session_id": "sess-1", "started_at": int64(123)}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/worker/tasks/"+taskID, bytes.NewReader(raw))
	req.SetPathValue("task_id", taskID)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("PATCH RUNNING when SCHEDULED: got status %d, want 200", w.Code)
	}
	if len(mockTask.list) != 1 || mockTask.list[0].Status != "RUNNING" {
		t.Errorf("PATCH RUNNING when SCHEDULED: task status = %q, want RUNNING", mockTask.list[0].Status)
	}
}

func TestPatchWorkerTask_RUNNING_WhenPending_Returns409(t *testing.T) {
	taskID := "task-pending"
	task := entity.Task{
		TaskID: taskID, WorkspaceID: "ws1", Status: "PENDING", Input: "input",
		CreatedBy: "u1", CreatedAt: 1,
	}
	mockTask := &mockTaskStore{list: []entity.Task{task}}
	cfg := Config{
		JWTSecret:   "secret",
		TaskStore:   mockTask,
		WorkerToken: "token",
	}
	s := New(cfg)
	handler := s.workerAuthMiddleware(http.HandlerFunc(s.patchWorkerTaskHandler))

	body := map[string]interface{}{"status": "RUNNING", "session_id": "sess-1"}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/worker/tasks/"+taskID, bytes.NewReader(raw))
	req.SetPathValue("task_id", taskID)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("PATCH RUNNING when PENDING: got status %d, want 409", w.Code)
	}
	if len(mockTask.list) != 1 || mockTask.list[0].Status != "PENDING" {
		t.Errorf("PATCH RUNNING when PENDING: task status = %q, want PENDING (unchanged)", mockTask.list[0].Status)
	}
}

func TestPatchWorkerTask_RUNNING_WhenRunning_Returns409(t *testing.T) {
	taskID := "task-running"
	task := entity.Task{
		TaskID: taskID, WorkspaceID: "ws1", Status: "RUNNING", Input: "input",
		CreatedBy: "u1", CreatedAt: 1,
	}
	mockTask := &mockTaskStore{list: []entity.Task{task}}
	cfg := Config{
		JWTSecret:   "secret",
		TaskStore:   mockTask,
		WorkerToken: "token",
	}
	s := New(cfg)
	handler := s.workerAuthMiddleware(http.HandlerFunc(s.patchWorkerTaskHandler))

	body := map[string]interface{}{"status": "RUNNING"}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/worker/tasks/"+taskID, bytes.NewReader(raw))
	req.SetPathValue("task_id", taskID)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("PATCH RUNNING when RUNNING: got status %d, want 409", w.Code)
	}
	if len(mockTask.list) != 1 || mockTask.list[0].Status != "RUNNING" {
		t.Errorf("PATCH RUNNING when RUNNING: task status = %q, want RUNNING (unchanged)", mockTask.list[0].Status)
	}
}
