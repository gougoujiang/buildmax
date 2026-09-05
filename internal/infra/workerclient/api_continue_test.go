package workerclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A Task already projects the current run when its worker starts. The client
// must preserve the immutable predecessor carried by the run or session
// restoration has no source to fetch.
func TestGetWorkerTaskRunPreservesSessionPredecessor(t *testing.T) {
	previousRunID := "run-previous"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/worker/task-runs/run-current" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(GetTaskRunResponse{
			Run: TaskRunRun{
				ID: "run-current", TaskID: "task-1", PreviousTaskRunID: &previousRunID,
				Input: "continue", Status: "SCHEDULED", CreatedAt: time.Unix(1, 0).UTC(),
			},
			Task: TaskRunTask{ID: "task-1", TeamID: "team-1", UserID: "user-1"},
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	got, err := GetWorkerTaskRun(context.Background(), WorkerAPIClientConfig{BaseURL: server.URL}, "run-current")
	if err != nil {
		t.Fatalf("GetWorkerTaskRun: %v", err)
	}
	if got.Run.PreviousTaskRunID == nil || *got.Run.PreviousTaskRunID != previousRunID {
		t.Errorf("previous_task_run_id = %v, want %q", got.Run.PreviousTaskRunID, previousRunID)
	}
}
