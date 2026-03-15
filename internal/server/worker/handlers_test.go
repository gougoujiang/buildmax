package worker

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"buildmax/internal/storage/entity"
	"buildmax/internal/testutil"
)

func TestGetWorkerTaskRunHandler_RequiresWorkerAuth(t *testing.T) {
	chatRunID := "run-1"
	run := entity.TaskRun{TaskRunID: chatRunID, TaskID: "chat-1", Input: "input", Status: "SCHEDULED", CreatedAt: 1}
	task := entity.Task{TaskID: "chat-1", ConversationID: "conv-1", CreatedBy: "u1"}
	mockRun := &testutil.MockTaskRunStore{Runs: []entity.TaskRun{run}, TaskList: []entity.Task{task}}
	cfg := Config{
		Token:        "worker-token-123",
		TaskRunStore: mockRun,
	}
	h := NewHandler(cfg)
	mux := http.NewServeMux()
	h.Register(mux)

	// Without token: 401
	req := httptest.NewRequest(http.MethodGet, "/api/worker/task-runs/"+chatRunID, nil)
	req.SetPathValue("task_run_id", chatRunID)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("without token: got status %d, want 401", w.Code)
	}

	// With correct token: 200 and run+chat body
	req = httptest.NewRequest(http.MethodGet, "/api/worker/task-runs/"+chatRunID, nil)
	req.SetPathValue("task_run_id", chatRunID)
	req.Header.Set("Authorization", "Bearer worker-token-123")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("with token: got status %d, want 200", w.Code)
	}
	if body := w.Body.String(); body == "" || len(body) < 10 {
		t.Errorf("with token: body too short: %q", body)
	}
}

func TestGetWorkerTaskRunHandler_NotFound(t *testing.T) {
	cfg := Config{
		Token:        "token",
		TaskRunStore: &testutil.MockTaskRunStore{Runs: []entity.TaskRun{}},
	}
	h := NewHandler(cfg)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/worker/task-runs/nonexistent", nil)
	req.SetPathValue("task_run_id", "nonexistent")
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestPatchWorkerTaskRun_RUNNING_WhenScheduled_Returns200(t *testing.T) {
	chatRunID := "run-scheduled"
	run := entity.TaskRun{TaskRunID: chatRunID, TaskID: "chat1", Input: "input", Status: "SCHEDULED", CreatedAt: 1}
	task := entity.Task{TaskID: "chat1", ConversationID: "conv-1"}
	mockRun := &testutil.MockTaskRunStore{Runs: []entity.TaskRun{run}, TaskList: []entity.Task{task}}
	cfg := Config{
		Token:        "token",
		TaskRunStore: mockRun,
	}
	h := NewHandler(cfg)
	mux := http.NewServeMux()
	h.Register(mux)

	body := map[string]interface{}{"status": "RUNNING", "session_id": "sess-1", "started_at": int64(123)}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/worker/task-runs/"+chatRunID, bytes.NewReader(raw))
	req.SetPathValue("task_run_id", chatRunID)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("PATCH RUNNING when SCHEDULED: got status %d, want 200", w.Code)
	}
	if len(mockRun.Runs) != 1 || mockRun.Runs[0].Status != "RUNNING" {
		t.Errorf("PATCH RUNNING when SCHEDULED: run status = %q, want RUNNING", mockRun.Runs[0].Status)
	}
}

func TestPatchWorkerTaskRun_RUNNING_WhenPending_Returns409(t *testing.T) {
	chatRunID := "run-pending"
	run := entity.TaskRun{TaskRunID: chatRunID, TaskID: "chat1", Input: "input", Status: "PENDING", CreatedAt: 1}
	task := entity.Task{TaskID: "chat1", ConversationID: "conv-1"}
	mockRun := &testutil.MockTaskRunStore{Runs: []entity.TaskRun{run}, TaskList: []entity.Task{task}}
	cfg := Config{
		Token:        "token",
		TaskRunStore: mockRun,
	}
	h := NewHandler(cfg)
	mux := http.NewServeMux()
	h.Register(mux)

	body := map[string]interface{}{"status": "RUNNING", "session_id": "sess-1"}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/worker/task-runs/"+chatRunID, bytes.NewReader(raw))
	req.SetPathValue("task_run_id", chatRunID)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("PATCH RUNNING when PENDING: got status %d, want 409", w.Code)
	}
	if len(mockRun.Runs) != 1 || mockRun.Runs[0].Status != "PENDING" {
		t.Errorf("PATCH RUNNING when PENDING: run status = %q, want PENDING (unchanged)", mockRun.Runs[0].Status)
	}
}
