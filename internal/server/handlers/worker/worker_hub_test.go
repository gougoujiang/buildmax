package worker

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	"github.com/gougoujiang/buildmax/internal/mock"
	streamhub "github.com/gougoujiang/buildmax/internal/server/websocket"
)

func TestPostWorkerStreamHandler_AppendsToHub(t *testing.T) {
	taskRunID := "r_run1"
	taskID := "t_task1"
	hub := streamhub.NewStreamHub()
	cfg := Config{
		JWTSecret: workerTestSecret,
		TaskRuns: &mock.MockTaskRunStore{
			Runs:     []coretask.Run{{ID: taskRunID, TaskID: taskID}},
			TaskList: []coretask.Task{{ID: taskID}},
		},
		Hub: hub,
	}
	h := New(cfg)
	mux := http.NewServeMux()
	h.Register(mux)

	body := bytes.NewReader([]byte(`{"delta":"hello "}`))
	req := httptest.NewRequest(http.MethodPost, "/api/worker/task-runs/"+taskRunID+"/stream", body)
	req.SetPathValue("task_run_id", taskRunID)
	req.Header.Set("Authorization", "Bearer "+runTokenFor(t, taskRunID, taskID))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST stream: got status %d, want 200", w.Code)
	}
	if got := hub.Buffer(taskID); got != "hello " {
		t.Errorf("hub buffer: got %q, want \"hello \"", got)
	}

	body2 := bytes.NewReader([]byte(`{"delta":"world"}`))
	req2 := httptest.NewRequest(http.MethodPost, "/api/worker/task-runs/"+taskRunID+"/stream", body2)
	req2.SetPathValue("task_run_id", taskRunID)
	req2.Header.Set("Authorization", "Bearer "+runTokenFor(t, taskRunID, taskID))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("POST stream second: got status %d, want 200", w2.Code)
	}
	if got := hub.Buffer(taskID); got != "hello world" {
		t.Errorf("hub buffer after second: got %q, want \"hello world\"", got)
	}
}
