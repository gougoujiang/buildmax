package worker

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
	artifactsvc "github.com/gougoujiang/buildmax/internal/service/artifact"
)

const workerArtifactToken = "worker-token-123"

func artifactWorkerMux(t *testing.T, store *mock.MockArtifactStore, teamID string) *http.ServeMux {
	t.Helper()
	run := model.TaskRun{TaskRunID: "run-1", TaskID: "task-1", Status: "RUNNING", CreatedAt: 1}
	task := model.Task{TaskID: "task-1", ConversationID: "conv-1", TeamID: teamID, CreatedBy: "u1"}
	h := New(Config{
		WorkerToken: workerArtifactToken,
		TaskRuns:    &mock.MockTaskRunStore{Runs: []model.TaskRun{run}, TaskList: []model.Task{task}},
		Artifacts:   &artifactsvc.Service{Artifacts: store, Storage: mock.NewMockArtifactStorage()},
	})
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

func workerUpload(t *testing.T, mux *http.ServeMux, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", "result.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, "# done"); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// The run token names the run, the run names the task, and the task names the
// team. A worker never says which team it is writing to, so a stolen token
// cannot be pointed at another one.
func TestWorkerArtifactTakesTheTeamFromTheRun(t *testing.T) {
	store := &mock.MockArtifactStore{}
	mux := artifactWorkerMux(t, store, "tm_1")

	rec := workerUpload(t, mux, "/api/worker/task-runs/run-1/artifacts?title=Result", workerArtifactToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		ArtifactID    string `json:"artifact_id"`
		TeamID        string `json:"team_id"`
		SourceType    string `json:"source_type"`
		SourceID      string `json:"source_id"`
		CreatedByType string `json:"created_by_type"`
		CreatedByID   string `json:"created_by_id"`
		Title         string `json:"title"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.TeamID != "tm_1" {
		t.Errorf("team = %q, want the run's team", out.TeamID)
	}
	// An agent choosing to publish is a different fact from a run leaving files
	// in its output directory, and the provenance has to say which happened.
	if out.SourceType != model.ArtifactSourceAgent || out.SourceID != "run-1" {
		t.Errorf("provenance = %q/%q, want agent/run-1", out.SourceType, out.SourceID)
	}
	if out.CreatedByType != model.ArtifactCreatorAgent {
		t.Errorf("creator = %q, want %q", out.CreatedByType, model.ArtifactCreatorAgent)
	}
	if out.CreatedByID != "" {
		t.Errorf("creator id = %q, want empty — automated work gets no invented user", out.CreatedByID)
	}
	if out.Title != "Result" {
		t.Errorf("title = %q, want the query value", out.Title)
	}
}

func TestWorkerArtifactRequiresTheRunCredential(t *testing.T) {
	store := &mock.MockArtifactStore{}
	mux := artifactWorkerMux(t, store, "tm_1")
	if code := workerUpload(t, mux, "/api/worker/task-runs/run-1/artifacts", "").Code; code != http.StatusUnauthorized {
		t.Errorf("without a token: %d, want 401", code)
	}
	if code := workerUpload(t, mux, "/api/worker/task-runs/run-1/artifacts", "wrong").Code; code != http.StatusUnauthorized {
		t.Errorf("with the wrong token: %d, want 401", code)
	}
	if store.Count() != 0 {
		t.Error("an unauthenticated upload must not be recorded")
	}
}

func TestWorkerArtifactUnknownRunIsNotFound(t *testing.T) {
	store := &mock.MockArtifactStore{}
	mux := artifactWorkerMux(t, store, "tm_1")
	if code := workerUpload(t, mux, "/api/worker/task-runs/run-missing/artifacts", workerArtifactToken).Code; code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
}

// A run with no team has nowhere to keep an artifact, and inventing one would
// put a file outside every authorization boundary the product has.
func TestWorkerArtifactRefusesARunWithNoTeam(t *testing.T) {
	store := &mock.MockArtifactStore{}
	mux := artifactWorkerMux(t, store, "")
	if code := workerUpload(t, mux, "/api/worker/task-runs/run-1/artifacts", workerArtifactToken).Code; code != http.StatusConflict {
		t.Errorf("status = %d, want 409", code)
	}
	if store.Count() != 0 {
		t.Error("nothing should have been recorded")
	}
}

func TestWorkerArtifactUnconfiguredDeploymentRefuses(t *testing.T) {
	run := model.TaskRun{TaskRunID: "run-1", TaskID: "task-1", Status: "RUNNING", CreatedAt: 1}
	task := model.Task{TaskID: "task-1", TeamID: "tm_1", CreatedBy: "u1"}
	h := New(Config{
		WorkerToken: workerArtifactToken,
		TaskRuns:    &mock.MockTaskRunStore{Runs: []model.TaskRun{run}, TaskList: []model.Task{task}},
	})
	mux := http.NewServeMux()
	h.Register(mux)
	if code := workerUpload(t, mux, "/api/worker/task-runs/run-1/artifacts", workerArtifactToken).Code; code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", code)
	}
}
