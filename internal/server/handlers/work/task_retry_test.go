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
	coreworkflow "github.com/gougoujiang/buildmax/internal/core/workflow"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/testsupport"
	"github.com/gougoujiang/buildmax/internal/util"
)

const (
	retrySecret = "test-retry-secret"
	retryTeam   = "tm_personal_u1"
	retryConv   = "conv_retry"
	retryUser   = "u1"
	retryTaskID = "t_retry"
	retryRunID  = "tr_retry"
)

// retryFixture wires a task whose last run is the given one. The workflow store
// is only set by the test that needs a step to exist.
func retryFixture(t *testing.T, run model.TaskRun, workflows *mock.MockWorkflowStore) (*http.ServeMux, *mock.MockTaskRunStore) {
	t.Helper()
	target := model.Task{
		ID:             retryTaskID,
		ConversationID: retryConv,
		TeamID:         retryTeam,
		Status:         run.Status,
		Input:          "the task's original input",
		CreatedBy:      retryUser,
	}
	var runs *mock.MockTaskRunStore
	if run.ID == "" {
		runs = &mock.MockTaskRunStore{TaskList: []model.Task{target}}
	} else {
		target.LastRunID = util.Ptr(run.ID)
		runs = &mock.MockTaskRunStore{Runs: []model.TaskRun{run}, TaskList: []model.Task{target}}
	}
	cfg := Config{
		JWTSecret: retrySecret,
		Teams: &mock.MockTeamStore{
			Teams:   []coreteam.Team{{ID: retryTeam, Name: "My Space", PersonalForUserID: util.Ptr(retryUser), CreatedBy: retryUser}},
			Members: []coreteam.Member{{TeamID: retryTeam, UserID: retryUser, Role: coreteam.RoleOwner}},
		},
		Tasks:    &mock.MockTaskStore{List: []model.Task{target}},
		TaskRuns: runs,
		Conversations: &mock.MockConversationStore{Conversations: []coreconv.Conversation{
			{ID: retryConv, UserID: retryUser, TeamID: retryTeam, Channel: "portal", CreatedBy: retryUser},
		}},
	}
	if workflows != nil {
		cfg.Workflows = workflows
	}
	mux := http.NewServeMux()
	New(cfg).Register(mux)
	return mux, runs
}

func postRetry(t *testing.T, mux *http.ServeMux) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/teams/"+retryTeam+"/tasks/"+retryTaskID+"/retry", nil)
	req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT(retryUser, retrySecret))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// The point of retry is that recovering from a dead worker does not make
// someone retype the instructions — so the new run carries the old input, and
// says which run it repeats.
func TestRetryTaskRepeatsTheRunWithItsOwnInput(t *testing.T) {
	mux, runs := retryFixture(t, model.TaskRun{
		ID:     retryRunID,
		TaskID: retryTaskID,
		Input:  "review the migration plan",
		Status: string(model.RunStatusFailed),
	}, nil)

	rec := postRetry(t, mux)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	var got retryTaskResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if got.RetryOfTaskRunID != retryRunID {
		t.Errorf("retry_of_task_run_id = %q, want %q", got.RetryOfTaskRunID, retryRunID)
	}
	if got.TaskRunID == retryRunID || got.TaskRunID == "" {
		t.Errorf("task_run_id = %q, want a new run", got.TaskRunID)
	}
	if got.Status != string(model.RunStatusPending) {
		t.Errorf("status = %q, want PENDING", got.Status)
	}
	if len(runs.Runs) != 2 {
		t.Fatalf("run count = %d, want 2", len(runs.Runs))
	}
	created := runs.Runs[1]
	if created.Input != "review the migration plan" {
		t.Errorf("input = %q, want the retried run's input", created.Input)
	}
	if created.TriggerSource != model.RunTriggerSourceTaskRetry {
		t.Errorf("trigger_source = %q, want %q", created.TriggerSource, model.RunTriggerSourceTaskRetry)
	}
}

func TestRetryTaskWithoutAFinishedRunConflicts(t *testing.T) {
	mux, runs := retryFixture(t, model.TaskRun{}, nil)

	rec := postRetry(t, mux)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rec.Code, rec.Body.String())
	}
	if len(runs.Runs) != 0 {
		t.Errorf("run count = %d, want none created", len(runs.Runs))
	}
}

// One task holds at most one run. A retry asked for while the previous one is
// still going is a conflict, and the answer names the run in flight rather than
// claiming there is nothing to repeat.
func TestRetryTaskWhileARunIsInFlightConflicts(t *testing.T) {
	mux, runs := retryFixture(t, model.TaskRun{
		ID:     retryRunID,
		TaskID: retryTaskID,
		Input:  "review the migration plan",
		Status: string(model.RunStatusRunning),
	}, nil)

	rec := postRetry(t, mux)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "already in progress") {
		t.Errorf("body = %s, want it to name the run in progress", body)
	}
	if len(runs.Runs) != 1 {
		t.Errorf("run count = %d, want no run created", len(runs.Runs))
	}
}

// A workflow owns its step tasks: it advances or fails the workflow run from
// their outcomes. Retrying one from the task API would report a second outcome
// for a step that is already settled.
func TestRetryTaskRefusesAWorkflowStep(t *testing.T) {
	mux, runs := retryFixture(t, model.TaskRun{
		ID:     retryRunID,
		TaskID: retryTaskID,
		Input:  "review the migration plan",
		Status: string(model.RunStatusFailed),
	}, &mock.MockWorkflowStore{StepRuns: []coreworkflow.StepRun{{
		ID:            "wsr_1",
		WorkflowRunID: "wr_1",
		TaskID:        util.Ptr(retryTaskID),
		Status:        coreworkflow.StepRunStatusFailed,
	}}})

	rec := postRetry(t, mux)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "workflow step") {
		t.Errorf("body = %s, want it to name the workflow step", body)
	}
	if len(runs.Runs) != 1 {
		t.Errorf("run count = %d, want no run created", len(runs.Runs))
	}
}
