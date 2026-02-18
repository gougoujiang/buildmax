package server

import (
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
