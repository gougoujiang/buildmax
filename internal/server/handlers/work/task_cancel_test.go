package work

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	coreconv "github.com/gougoujiang/buildmax/internal/core/conversation"
	"github.com/gougoujiang/buildmax/internal/core/model"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/testsupport"
	"github.com/gougoujiang/buildmax/internal/util"
)

const (
	cancelSecret  = "test-cancel-secret"
	cancelTeam    = "tm_personal_u1"
	cancelConv    = "conv_cancel"
	cancelUser    = "u1"
	cancelTaskID  = "t_cancel"
	cancelRunID   = "tr_cancel"
	cancelOtherID = "tr_other"
)

// cancelFixture wires the two stores a cancel touches: the task is read for
// authorization, the run is what actually moves.
func cancelFixture(t *testing.T, run model.TaskRun) (*http.ServeMux, *mock.MockTaskRunStore) {
	t.Helper()
	task := model.Task{
		ID:             cancelTaskID,
		ConversationID: cancelConv,
		TeamID:         cancelTeam,
		Status:         run.Status,
		CreatedBy:      cancelUser,
	}
	runs := &mock.MockTaskRunStore{Runs: []model.TaskRun{run}, TaskList: []model.Task{task}}
	h := New(Config{
		JWTSecret: cancelSecret,
		Teams: &mock.MockTeamStore{
			Teams:   []coreteam.Team{{ID: cancelTeam, Name: "My Space", PersonalForUserID: util.Ptr(cancelUser), CreatedBy: cancelUser}},
			Members: []coreteam.Member{{TeamID: cancelTeam, UserID: cancelUser, Role: coreteam.RoleOwner}},
		},
		Tasks:    &mock.MockTaskStore{List: []model.Task{task}},
		TaskRuns: runs,
		Conversations: &mock.MockConversationStore{Conversations: []coreconv.Conversation{
			{ID: cancelConv, UserID: cancelUser, TeamID: cancelTeam, Channel: "portal", CreatedBy: cancelUser},
		}},
	})
	mux := http.NewServeMux()
	h.Register(mux)
	return mux, runs
}

func postCancel(t *testing.T, mux *http.ServeMux) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/teams/"+cancelTeam+"/tasks/"+cancelTaskID+"/cancel", nil)
	req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT(cancelUser, cancelSecret))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func decodeCancel(t *testing.T, rec *httptest.ResponseRecorder) cancelTaskResponse {
	t.Helper()
	var got cancelTaskResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return got
}

// A run nobody has picked up is over the moment it is canceled: no worker holds
// it, so there is nothing to wait for and nothing that could still report.
func TestCancelTaskFinishesAnUndispatchedRun(t *testing.T) {
	mux, runs := cancelFixture(t, model.TaskRun{
		ID: cancelRunID, TaskID: cancelTaskID, Status: string(model.RunStatusPending),
	})

	rec := postCancel(t, mux)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	got := decodeCancel(t, rec)
	if got.Status != string(model.RunStatusCanceled) {
		t.Errorf("status = %q, want CANCELED", got.Status)
	}
	if got.CancelRequested {
		t.Error("cancel_requested is true for a run that is already over")
	}
	if runs.Runs[0].Status != string(model.RunStatusCanceled) {
		t.Errorf("stored run status = %q, want CANCELED", runs.Runs[0].Status)
	}
	if runs.Runs[0].EndedAt == nil {
		t.Error("a finished run has no ended_at")
	}
	// The task's denormalized status has to follow, or Portal keeps showing
	// work in progress that has already stopped.
	if runs.TaskList[0].Status != string(model.RunStatusCanceled) {
		t.Errorf("task status = %q, want CANCELED", runs.TaskList[0].Status)
	}
	if runs.Runs[0].CancelRequestedBy == nil || *runs.Runs[0].CancelRequestedBy != cancelUser {
		t.Errorf("cancel_requested_by = %v, want %q", runs.Runs[0].CancelRequestedBy, cancelUser)
	}
}

// A started run belongs to its worker. The cancel is recorded and the run keeps
// its status, because marking it over here would describe a process that is
// still executing.
func TestCancelTaskRequestsStopForARunningRun(t *testing.T) {
	mux, runs := cancelFixture(t, model.TaskRun{
		ID: cancelRunID, TaskID: cancelTaskID, Status: string(model.RunStatusRunning),
	})

	rec := postCancel(t, mux)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body = %s", rec.Code, rec.Body.String())
	}
	got := decodeCancel(t, rec)
	if !got.CancelRequested {
		t.Error("cancel_requested is false for a run that is still executing")
	}
	if got.Status != string(model.RunStatusRunning) {
		t.Errorf("status = %q, want the run's current status RUNNING", got.Status)
	}
	if runs.Runs[0].Status != string(model.RunStatusRunning) {
		t.Errorf("stored run status = %q, want it left alone until its worker reports", runs.Runs[0].Status)
	}
	if runs.Runs[0].CancelRequestedAt == nil {
		t.Error("no cancel request was recorded, so no worker will ever see it")
	}
}

// Pressing stop twice is normal — the first press has no visible effect until
// the worker reacts. The second must not read as an error.
func TestCancelTaskIsIdempotentWhileTheRunIsStopping(t *testing.T) {
	mux, runs := cancelFixture(t, model.TaskRun{
		ID: cancelRunID, TaskID: cancelTaskID, Status: string(model.RunStatusRunning),
	})

	first := postCancel(t, mux)
	second := postCancel(t, mux)

	if first.Code != http.StatusAccepted || second.Code != http.StatusAccepted {
		t.Fatalf("statuses = %d then %d, want 202 both times", first.Code, second.Code)
	}
	if !decodeCancel(t, second).CancelRequested {
		t.Error("the second cancel does not report the run as stopping")
	}
	firstAsk := runs.Runs[0].CancelRequestedAt
	if firstAsk == nil {
		t.Fatal("no cancel request was recorded")
	}
}

func TestCancelTaskRefusesAFinishedRun(t *testing.T) {
	mux, _ := cancelFixture(t, model.TaskRun{
		ID: cancelRunID, TaskID: cancelTaskID, Status: string(model.RunStatusSucceeded),
	})

	rec := postCancel(t, mux)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "no run in progress") {
		t.Errorf("body = %s, want it to say there is nothing to cancel", rec.Body.String())
	}
}

// The run belongs to a different task, so this task has nothing in flight.
func TestCancelTaskRefusesWhenTheTaskHasNoActiveRun(t *testing.T) {
	mux, _ := cancelFixture(t, model.TaskRun{
		ID: cancelOtherID, TaskID: "t_somebody_else", Status: string(model.RunStatusRunning),
	})

	rec := postCancel(t, mux)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rec.Code, rec.Body.String())
	}
}
