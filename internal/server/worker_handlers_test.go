package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"buildmax/internal/storage/entity"
)

func TestGetWorkerTaskRunHandler_RequiresWorkerAuth(t *testing.T) {
	secret := "test-worker-secret"
	runID := "run-1"
	run := entity.TaskRun{RunID: runID, TaskID: "task-1", Input: "input", Status: "SCHEDULED", CreatedAt: 1}
	task := entity.Task{TaskID: "task-1", WorkspaceID: "ws1", CreatedBy: "u1"}
	mockRun := &mockTaskRunStore{runs: []entity.TaskRun{run}, taskList: []entity.Task{task}}
	cfg := Config{
		JWTSecret:    secret,
		TaskRunStore: mockRun,
		WorkerToken:  "worker-token-123",
	}
	s := New(cfg)
	handler := s.workerAuthMiddleware(http.HandlerFunc(s.getWorkerTaskRunHandler))

	// Without token: 401
	req := httptest.NewRequest(http.MethodGet, "/api/worker/task-runs/"+runID, nil)
	req.SetPathValue("run_id", runID)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("without token: got status %d, want 401", w.Code)
	}

	// With correct token: 200 and run+task body
	req = httptest.NewRequest(http.MethodGet, "/api/worker/task-runs/"+runID, nil)
	req.SetPathValue("run_id", runID)
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

func TestGetWorkerTaskRunHandler_NotFound(t *testing.T) {
	cfg := Config{
		JWTSecret:    "secret",
		TaskRunStore: &mockTaskRunStore{runs: []entity.TaskRun{}},
		WorkerToken:  "token",
	}
	s := New(cfg)
	handler := s.workerAuthMiddleware(http.HandlerFunc(s.getWorkerTaskRunHandler))
	req := httptest.NewRequest(http.MethodGet, "/api/worker/task-runs/nonexistent", nil)
	req.SetPathValue("run_id", "nonexistent")
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestPatchWorkerTaskRun_RUNNING_WhenScheduled_Returns200(t *testing.T) {
	runID := "run-scheduled"
	run := entity.TaskRun{RunID: runID, TaskID: "task1", Input: "input", Status: "SCHEDULED", CreatedAt: 1}
	task := entity.Task{TaskID: "task1", WorkspaceID: "ws1"}
	mockRun := &mockTaskRunStore{runs: []entity.TaskRun{run}, taskList: []entity.Task{task}}
	cfg := Config{
		JWTSecret:    "secret",
		TaskRunStore: mockRun,
		WorkerToken:  "token",
	}
	s := New(cfg)
	handler := s.workerAuthMiddleware(http.HandlerFunc(s.patchWorkerTaskRunHandler))

	body := map[string]interface{}{"status": "RUNNING", "session_id": "sess-1", "started_at": int64(123)}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/worker/task-runs/"+runID, bytes.NewReader(raw))
	req.SetPathValue("run_id", runID)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("PATCH RUNNING when SCHEDULED: got status %d, want 200", w.Code)
	}
	if len(mockRun.runs) != 1 || mockRun.runs[0].Status != "RUNNING" {
		t.Errorf("PATCH RUNNING when SCHEDULED: run status = %q, want RUNNING", mockRun.runs[0].Status)
	}
}

func TestPatchWorkerTaskRun_RUNNING_WhenPending_Returns409(t *testing.T) {
	runID := "run-pending"
	run := entity.TaskRun{RunID: runID, TaskID: "task1", Input: "input", Status: "PENDING", CreatedAt: 1}
	task := entity.Task{TaskID: "task1", WorkspaceID: "ws1"}
	mockRun := &mockTaskRunStore{runs: []entity.TaskRun{run}, taskList: []entity.Task{task}}
	cfg := Config{
		JWTSecret:    "secret",
		TaskRunStore: mockRun,
		WorkerToken:  "token",
	}
	s := New(cfg)
	handler := s.workerAuthMiddleware(http.HandlerFunc(s.patchWorkerTaskRunHandler))

	body := map[string]interface{}{"status": "RUNNING", "session_id": "sess-1"}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/worker/task-runs/"+runID, bytes.NewReader(raw))
	req.SetPathValue("run_id", runID)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("PATCH RUNNING when PENDING: got status %d, want 409", w.Code)
	}
	if len(mockRun.runs) != 1 || mockRun.runs[0].Status != "PENDING" {
		t.Errorf("PATCH RUNNING when PENDING: run status = %q, want PENDING (unchanged)", mockRun.runs[0].Status)
	}
}
