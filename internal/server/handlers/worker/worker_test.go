package worker

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/util"
)

func TestGetWorkerTaskRunHandler_RequiresWorkerAuth(t *testing.T) {
	taskRunID := "run-1"
	run := model.TaskRun{ID: taskRunID, TaskID: "task-1", Input: "input", Status: "SCHEDULED", CreatedAt: time.Unix(1, 0).UTC()}
	task := model.Task{ID: "task-1", ConversationID: "conv-1", TeamID: "tm_1", CreatedBy: "u1"}
	mockRun := &mock.MockTaskRunStore{Runs: []model.TaskRun{run}, TaskList: []model.Task{task}}
	cfg := Config{
		WorkerToken: "worker-token-123",
		TaskRuns:    mockRun,
	}
	h := New(cfg)
	mux := http.NewServeMux()
	h.Register(mux)

	// Without token: 401
	req := httptest.NewRequest(http.MethodGet, "/api/worker/task-runs/"+taskRunID, nil)
	req.SetPathValue("task_run_id", taskRunID)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("without token: got status %d, want 401", w.Code)
	}

	// With correct token: 200 and run+chat body
	req = httptest.NewRequest(http.MethodGet, "/api/worker/task-runs/"+taskRunID, nil)
	req.SetPathValue("task_run_id", taskRunID)
	req.Header.Set("Authorization", "Bearer worker-token-123")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("with token: got status %d, want 200", w.Code)
	}
	if body := w.Body.String(); body == "" || len(body) < 10 {
		t.Errorf("with token: body too short: %q", body)
	}
	if body := w.Body.String(); !strings.Contains(body, `"team_id":"tm_1"`) {
		t.Errorf("with token: body missing team_id: %q", body)
	}
}

// The worker polls this route while it executes, so the run's cancel request
// has to be visible in it. Nothing else can reach a started run.
func TestGetWorkerTaskRunHandler_ReportsACancelRequest(t *testing.T) {
	taskRunID := "run-cancel"
	askedAt := time.Unix(1_800_000_000, 0).UTC()
	runs := &mock.MockTaskRunStore{
		Runs: []model.TaskRun{{
			ID: taskRunID, TaskID: "task-1", Input: "input",
			Status: string(model.RunStatusRunning), CancelRequestedAt: &askedAt,
		}},
		TaskList: []model.Task{{ID: "task-1", ConversationID: "conv-1", TeamID: "tm_1", CreatedBy: "u1"}},
	}
	h := New(Config{WorkerToken: "worker-token-123", TaskRuns: runs})
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/worker/task-runs/"+taskRunID, nil)
	req.Header.Set("Authorization", "Bearer worker-token-123")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got workerclient.GetTaskRunResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Run.CancelRequested {
		t.Errorf("cancel_requested = false for a run that was asked to stop; body = %s", w.Body.String())
	}
}

// A canceled run is registered like a finished one: its artifacts are kept and
// its task follows it out of "running". Losing either would make cancelling
// cost more than waiting.
func TestPatchWorkerTaskRun_CanceledKeepsArtifactsAndSyncsTheTask(t *testing.T) {
	taskRunID := "run-canceled"
	runs := &mock.MockTaskRunStore{
		Runs:     []model.TaskRun{{ID: taskRunID, TaskID: "task-1", Status: string(model.RunStatusRunning)}},
		TaskList: []model.Task{{ID: "task-1", ConversationID: "conv-1", TeamID: "tm_1", CreatedBy: "u1", Status: string(model.RunStatusRunning)}},
	}
	h := New(Config{WorkerToken: "worker-token-123", TaskRuns: runs})
	mux := http.NewServeMux()
	h.Register(mux)

	endedAt := time.Unix(1_800_000_010, 0).UTC()
	body, err := json.Marshal(workerclient.PatchTaskRunRequest{
		Status:   string(model.RunStatusCanceled),
		EndedAt:  &endedAt,
		Output:   util.Ptr("as far as I got"),
		Artifact: &workerclient.ArtifactPayload{RelativePaths: []string{"result.md", "notes.md"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/worker/task-runs/"+taskRunID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer worker-token-123")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if runs.Runs[0].Status != string(model.RunStatusCanceled) {
		t.Errorf("run status = %q, want CANCELED", runs.Runs[0].Status)
	}
	if runs.TaskList[0].Status != string(model.RunStatusCanceled) {
		t.Errorf("task status = %q, want CANCELED", runs.TaskList[0].Status)
	}
	if got := runs.Artifacts[taskRunID]; len(got) != 2 {
		t.Errorf("registered artifacts = %v, want both files the run wrote", got)
	}
}

func TestGetWorkerTaskRunHandler_NotFound(t *testing.T) {
	cfg := Config{
		WorkerToken: "token",
		TaskRuns:    &mock.MockTaskRunStore{Runs: []model.TaskRun{}},
	}
	h := New(cfg)
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
	taskRunID := "run-scheduled"
	run := model.TaskRun{ID: taskRunID, TaskID: "task1", Input: "input", Status: "SCHEDULED", CreatedAt: time.Unix(1, 0).UTC()}
	task := model.Task{ID: "task1", ConversationID: "conv-1"}
	mockRun := &mock.MockTaskRunStore{Runs: []model.TaskRun{run}, TaskList: []model.Task{task}}
	cfg := Config{
		WorkerToken: "token",
		TaskRuns:    mockRun,
	}
	h := New(cfg)
	mux := http.NewServeMux()
	h.Register(mux)

	body := map[string]interface{}{"status": "RUNNING", "session_id": "sess-1", "started_at": time.Unix(123, 0).UTC().Format(time.RFC3339)}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/worker/task-runs/"+taskRunID, bytes.NewReader(raw))
	req.SetPathValue("task_run_id", taskRunID)
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
	taskRunID := "run-pending"
	run := model.TaskRun{ID: taskRunID, TaskID: "task1", Input: "input", Status: "PENDING", CreatedAt: time.Unix(1, 0).UTC()}
	task := model.Task{ID: "task1", ConversationID: "conv-1"}
	mockRun := &mock.MockTaskRunStore{Runs: []model.TaskRun{run}, TaskList: []model.Task{task}}
	cfg := Config{
		WorkerToken: "token",
		TaskRuns:    mockRun,
	}
	h := New(cfg)
	mux := http.NewServeMux()
	h.Register(mux)

	body := map[string]interface{}{"status": "RUNNING", "session_id": "sess-1"}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/worker/task-runs/"+taskRunID, bytes.NewReader(raw))
	req.SetPathValue("task_run_id", taskRunID)
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

// TestGetWorkerTaskRunHandler_TellsTheRunHowToReachAModel covers the descriptor
// a worker uses to decide its transport. The server states it so the worker —
// which executes model-chosen code — does not choose its own model, and it
// states only the alias, because endpoint, upstream identifier, and credential
// stay on this side.
func TestGetWorkerTaskRunHandler_TellsTheRunHowToReachAModel(t *testing.T) {
	store := &mock.MockTaskRunStore{
		Runs:     []model.TaskRun{{ID: "run-1", TaskID: "task-1", Input: "input", Status: "SCHEDULED", CreatedAt: time.Unix(1, 0).UTC()}},
		TaskList: []model.Task{{ID: "task-1", ConversationID: "conv-1", TeamID: "tm_1", CreatedBy: "u1"}},
	}

	get := func(t *testing.T, cfg Config) workerclient.GetTaskRunResponse {
		t.Helper()
		mux := http.NewServeMux()
		New(cfg).Register(mux)
		req := httptest.NewRequest(http.MethodGet, "/api/worker/task-runs/run-1", nil)
		req.Header.Set("Authorization", "Bearer worker-token-123")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		var got workerclient.GetTaskRunResponse
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return got
	}

	t.Run("managed", func(t *testing.T) {
		got := get(t, Config{
			WorkerToken: "worker-token-123",
			TaskRuns:    store,
			WorkerLLM:   &workerclient.TaskRunLLM{Transport: "buildmax", Alias: "deep", ContextWindow: 128000},
		})
		if got.LLM == nil {
			t.Fatal("a managed deployment told the run nothing about models")
		}
		if got.LLM.Transport != "buildmax" || got.LLM.Alias != "deep" {
			t.Errorf("llm = %+v", *got.LLM)
		}
	})

	// Absent means direct, so a worker built before this field behaves as it
	// always did.
	t.Run("direct omits the field entirely", func(t *testing.T) {
		mux := http.NewServeMux()
		New(Config{WorkerToken: "worker-token-123", TaskRuns: store}).Register(mux)
		req := httptest.NewRequest(http.MethodGet, "/api/worker/task-runs/run-1", nil)
		req.Header.Set("Authorization", "Bearer worker-token-123")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if strings.Contains(w.Body.String(), `"llm"`) {
			t.Errorf("a direct deployment sent an llm field: %s", w.Body.String())
		}
	})
}
