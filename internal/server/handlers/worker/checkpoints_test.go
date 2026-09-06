package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
	"github.com/gougoujiang/buildmax/internal/mock"
	workspacesvc "github.com/gougoujiang/buildmax/internal/service/workspace"
)

// fakeWorkspaceRuns is the base-read and restore-record half of the worker
// checkpoint API.
type fakeWorkspaceRuns struct {
	base          *coretask.WorkspaceCheckpoint
	restoreStatus coretask.WorkspaceRestoreStatus
	restoreErr    *string
	restoreCalls  int
}

func (f *fakeWorkspaceRuns) GetRunWorkspaceBase(context.Context, string) (*coretask.WorkspaceCheckpoint, error) {
	return f.base, nil
}

func (f *fakeWorkspaceRuns) RecordWorkspaceRestore(_ context.Context, _ string, status coretask.WorkspaceRestoreStatus, errMessage *string) error {
	f.restoreCalls++
	f.restoreStatus = status
	f.restoreErr = errMessage
	return nil
}

// seedMetadata is a MetadataStore that returns a fixed checkpoint, so a seed
// finalize can be driven without a database.
type seedMetadata struct {
	out *coretask.WorkspaceCheckpoint
	got coretask.FinalizeCheckpointInput
}

func (m *seedMetadata) FinalizeWorkspaceCheckpoint(_ context.Context, in coretask.FinalizeCheckpointInput) (*coretask.WorkspaceCheckpoint, error) {
	m.got = in
	return m.out, nil
}

type seedPayloads struct{ exists bool }

func (p *seedPayloads) Exists(context.Context, string, string) (bool, error) { return p.exists, nil }
func (p *seedPayloads) Key(spaceID, sha string) (string, error) {
	return spaceID + "/workspace/blobs/sha256/" + sha, nil
}

func runningRunAndTask(taskRunID, taskID, spaceID string) *mock.MockTaskRunStore {
	return &mock.MockTaskRunStore{
		Runs:     []coretask.Run{{ID: taskRunID, TaskID: taskID, Status: string(coretask.RunStatusRunning)}},
		TaskList: []coretask.Task{{ID: taskID, ConversationID: "c1", SpaceID: spaceID, CreatedBy: "u1", Status: string(coretask.RunStatusRunning)}},
	}
}

const seedDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestGetWorkspaceBase_DescriptorOr204(t *testing.T) {
	taskRunID := "run-base"
	// With a base: the descriptor comes back, and never a storage key.
	ws := &fakeWorkspaceRuns{base: &coretask.WorkspaceCheckpoint{
		ID: "cp1", PayloadFormat: coretask.PayloadFormatTarZstV1, PayloadSHA256: seedDigest,
		SizeBytes: 10, UncompressedBytes: 20, EntryCount: 3,
	}}
	h := New(Config{JWTSecret: workerTestSecret, TaskRuns: runningRunAndTask(taskRunID, "t1", "tm_1"), WorkspaceRuns: ws})
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/worker/task-runs/"+taskRunID+"/workspace-base", nil)
	req.Header.Set("Authorization", "Bearer "+runTokenFor(t, taskRunID, "t1"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var got workerclient.WorkspaceBaseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.CheckpointID != "cp1" || got.PayloadSHA256 != seedDigest {
		t.Errorf("descriptor = %+v", got)
	}
	if strings.Contains(rec.Body.String(), "storage_key") {
		t.Error("the base descriptor leaked a storage key")
	}

	// Without a base: 204, the first run of a Task.
	ws.base = nil
	req = httptest.NewRequest(http.MethodGet, "/api/worker/task-runs/"+taskRunID+"/workspace-base", nil)
	req.Header.Set("Authorization", "Bearer "+runTokenFor(t, taskRunID, "t1"))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestPostWorkspaceRestore_RecordsOutcome(t *testing.T) {
	taskRunID := "run-restore"
	ws := &fakeWorkspaceRuns{}
	h := New(Config{JWTSecret: workerTestSecret, TaskRuns: runningRunAndTask(taskRunID, "t1", "tm_1"), WorkspaceRuns: ws})
	mux := http.NewServeMux()
	h.Register(mux)

	post := func(body string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/worker/task-runs/"+taskRunID+"/workspace-restore", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+runTokenFor(t, taskRunID, "t1"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := post(`{"status":"failed","error":"tar was truncated"}`); code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", code)
	}
	if ws.restoreStatus != coretask.WorkspaceRestoreFailed || ws.restoreErr == nil || *ws.restoreErr != "tar was truncated" {
		t.Errorf("recorded status=%q err=%v", ws.restoreStatus, ws.restoreErr)
	}
	if code := post(`{"status":"bogus"}`); code != http.StatusBadRequest {
		t.Errorf("a bogus status returned %d, want 400", code)
	}
}

func TestPostWorkspaceCheckpoint_FinalizesSeed(t *testing.T) {
	taskRunID := "run-seed"
	meta := &seedMetadata{out: &coretask.WorkspaceCheckpoint{ID: "cp_seed", Kind: coretask.CheckpointKindSeed}}
	svc := workspacesvc.New(meta, &seedPayloads{exists: true})
	h := New(Config{JWTSecret: workerTestSecret, TaskRuns: runningRunAndTask(taskRunID, "t1", "tm_1"), Checkpoints: svc})
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{"payload_format":"tar.zst.v1","payload_sha256":"` + seedDigest + `","size_bytes":10,"uncompressed_bytes":20,"entry_count":3}`
	req := httptest.NewRequest(http.MethodPost, "/api/worker/task-runs/"+taskRunID+"/workspace-checkpoints", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+runTokenFor(t, taskRunID, "t1"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var got workerclient.SeedCheckpointResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.CheckpointID != "cp_seed" {
		t.Errorf("checkpoint id = %q", got.CheckpointID)
	}
	// The server derived space, task, and kind; the worker sent none of them.
	if meta.got.SpaceID != "tm_1" || meta.got.TaskID != "t1" || meta.got.Kind != coretask.CheckpointKindSeed {
		t.Errorf("finalize input = %+v", meta.got)
	}
	if meta.got.StorageKey == "" {
		t.Error("the server did not derive a storage key")
	}
}

func TestPostWorkspaceCheckpoint_MissingBytesConflicts(t *testing.T) {
	taskRunID := "run-seed-missing"
	svc := workspacesvc.New(&seedMetadata{}, &seedPayloads{exists: false})
	h := New(Config{JWTSecret: workerTestSecret, TaskRuns: runningRunAndTask(taskRunID, "t1", "tm_1"), Checkpoints: svc})
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{"payload_format":"tar.zst.v1","payload_sha256":"` + seedDigest + `","size_bytes":10}`
	req := httptest.NewRequest(http.MethodPost, "/api/worker/task-runs/"+taskRunID+"/workspace-checkpoints", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+runTokenFor(t, taskRunID, "t1"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 for un-uploaded bytes; body = %s", rec.Code, rec.Body.String())
	}
}

// TestPatchTerminal_CommitsSuccessfulResultCheckpoint pins that a successful
// terminal report carrying a result descriptor commits a successful checkpoint,
// with space, task, kind, and the run's base all derived by the server.
func TestPatchTerminal_CommitsSuccessfulResultCheckpoint(t *testing.T) {
	taskRunID := "run-result"
	meta := &seedMetadata{out: &coretask.WorkspaceCheckpoint{ID: "cp_result", Kind: coretask.CheckpointKindSuccessful}}
	svc := workspacesvc.New(meta, &seedPayloads{exists: true})
	runs := runningRunAndTask(taskRunID, "t1", "tm_1")
	base := "cp_base"
	runs.Runs[0].WorkspaceBaseCheckpointID = &base
	h := New(Config{JWTSecret: workerTestSecret, TaskRuns: runs, Checkpoints: svc})
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{"status":"SUCCEEDED","output":"ok","workspace_checkpoint":{"payload_format":"tar.zst.v1","payload_sha256":"` + seedDigest + `","size_bytes":10,"uncompressed_bytes":20,"entry_count":3}}`
	req := httptest.NewRequest(http.MethodPatch, "/api/worker/task-runs/"+taskRunID, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+runTokenFor(t, taskRunID, "t1"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if meta.got.Kind != coretask.CheckpointKindSuccessful {
		t.Errorf("checkpoint kind = %q, want successful", meta.got.Kind)
	}
	if meta.got.SpaceID != "tm_1" || meta.got.TaskID != "t1" {
		t.Errorf("finalize input = %+v; server did not derive owners", meta.got)
	}
	if meta.got.BaseCheckpointID == nil || *meta.got.BaseCheckpointID != "cp_base" {
		t.Errorf("base checkpoint = %v, want cp_base", meta.got.BaseCheckpointID)
	}
	if meta.got.StorageKey == "" {
		t.Error("the server did not derive a storage key")
	}
}

// TestPatchTerminal_ResultCheckpointFailureIsFailOpen pins that a result
// checkpoint the server cannot commit (its bytes never reached the store) does
// not fail the run: the terminal outcome is already accepted.
func TestPatchTerminal_ResultCheckpointFailureIsFailOpen(t *testing.T) {
	taskRunID := "run-result-missing"
	svc := workspacesvc.New(&seedMetadata{}, &seedPayloads{exists: false})
	h := New(Config{JWTSecret: workerTestSecret, TaskRuns: runningRunAndTask(taskRunID, "t1", "tm_1"), Checkpoints: svc})
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{"status":"SUCCEEDED","output":"ok","workspace_checkpoint":{"payload_format":"tar.zst.v1","payload_sha256":"` + seedDigest + `","size_bytes":10}}`
	req := httptest.NewRequest(http.MethodPatch, "/api/worker/task-runs/"+taskRunID, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+runTokenFor(t, taskRunID, "t1"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("a checkpoint that cannot commit must not fail the run, got %d; body = %s", rec.Code, rec.Body.String())
	}
}
