package worker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	"github.com/gougoujiang/buildmax/internal/mock"
)

// livenessFixture serves one RUNNING run through the real route table, so the
// stamp is observed where a worker's poll actually lands.
func livenessFixture(t *testing.T) (*mock.MockTaskRunStore, http.Handler, string) {
	t.Helper()
	const taskRunID, taskID = "run-live", "task-live"
	runs := &mock.MockTaskRunStore{
		Runs:     []coretask.Run{{ID: taskRunID, TaskID: taskID, Input: "input", Status: string(coretask.RunStatusRunning)}},
		TaskList: []coretask.Task{{ID: taskID, ConversationID: "conv-1", TeamID: "tm_1", CreatedBy: "u1"}},
	}
	mux := http.NewServeMux()
	New(Config{JWTSecret: workerTestSecret, TaskRuns: runs}).Register(mux)
	return runs, mux, taskRunID
}

func pollRun(t *testing.T, mux http.Handler, taskRunID string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/worker/task-runs/"+taskRunID, nil)
	req.Header.Set("Authorization", "Bearer "+runTokenFor(t, taskRunID, "task-live"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("poll status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

// TestPollingTheRunRecordsLiveness is what lets the reaper tell a killed worker
// from a slow one in minutes rather than at the run timeout. The signal is the
// cancel poll a worker already makes for the whole time it is RUNNING, so
// nothing new has to be sent for the server to know the process is alive.
func TestPollingTheRunRecordsLiveness(t *testing.T) {
	runs, mux, taskRunID := livenessFixture(t)

	before := time.Now().UTC()
	pollRun(t, mux, taskRunID)

	got := runs.Runs[0].LastSeenAt
	if got == nil {
		t.Fatal("a worker polled its run and nothing recorded that it is alive")
	}
	if got.Before(before) {
		t.Errorf("last_seen_at = %v, want at or after %v", got, before)
	}
}

// failingSeenStore is a store whose liveness write never lands.
type failingSeenStore struct {
	*mock.MockTaskRunStore
}

func (failingSeenStore) MarkTaskRunSeen(context.Context, string, time.Time) error {
	return errors.New("database is away")
}

// A worker that cannot be told it was canceled is worse than one whose liveness
// went unrecorded: the run keeps working either way, but a refused poll is how
// a cancel gets lost. The run timeout still closes a run this leaves unseen.
func TestPollingSucceedsWhenLivenessCannotBeRecorded(t *testing.T) {
	const taskRunID, taskID = "run-live", "task-live"
	runs := &mock.MockTaskRunStore{
		Runs:     []coretask.Run{{ID: taskRunID, TaskID: taskID, Input: "input", Status: string(coretask.RunStatusRunning)}},
		TaskList: []coretask.Task{{ID: taskID, ConversationID: "conv-1", TeamID: "tm_1", CreatedBy: "u1"}},
	}
	mux := http.NewServeMux()
	New(Config{JWTSecret: workerTestSecret, TaskRuns: failingSeenStore{runs}}).Register(mux)

	pollRun(t, mux, taskRunID)

	if runs.Runs[0].LastSeenAt != nil {
		t.Error("the fixture recorded a stamp; this test proves nothing")
	}
}
