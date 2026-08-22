package scheduler

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/authtoken"
)

// recordingRunner captures what the scheduler handed a worker.
type recordingRunner struct {
	mu     sync.Mutex
	calls  int
	token  string
	runIDs []string
}

func (r *recordingRunner) Run(_ context.Context, run model.TaskRun, runToken string) (string, *string, *int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.token = runToken
	r.runIDs = append(r.runIDs, run.ID)
	return "local_process", nil, nil, nil
}

func (r *recordingRunner) observed() (calls int, token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, r.token
}

// runOnce drives the loop long enough to dispatch the single pending run.
func runOnce(t *testing.T, spy *spyTaskRunStore, runner WorkerRunner, mint MintRunToken) {
	t.Helper()
	s, err := NewSchedulerWithPollInterval(spy, runner, mint, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	s.Start()
	time.Sleep(40 * time.Millisecond)
	s.Stop()
}

// TestRunTokenClaimsComeFromTheTask is the property the whole credential rests
// on: a worker's identity is assembled from what the server already knows about
// the run, so nothing a worker or its model does can change who a call is
// attributed to or whose models it may reach.
func TestRunTokenClaimsComeFromTheTask(t *testing.T) {
	spy := newSpyTaskRunStore("r_token12345678901234")
	runner := &recordingRunner{}

	var got authtoken.RunClaims
	runOnce(t, spy, runner, func(c authtoken.RunClaims) (string, error) {
		got = c
		return "signed-token", nil
	})

	calls, token := runner.observed()
	if calls != 1 {
		t.Fatalf("runner ran %d times, want 1", calls)
	}
	if token != "signed-token" {
		t.Errorf("worker received token %q", token)
	}
	want := authtoken.RunClaims{
		UserID:    "u_test",
		TeamID:    "tm_test",
		TaskRunID: "r_token12345678901234",
		TaskID:    "t_test",
	}
	if got != want {
		t.Errorf("claims = %+v, want %+v", got, want)
	}
}

// TestNoMinterMeansNoToken records that a direct-mode deployment is untouched:
// its workers reach a provider themselves and have nothing to authenticate to.
func TestNoMinterMeansNoToken(t *testing.T) {
	spy := newSpyTaskRunStore("r_direct123456789012")
	runner := &recordingRunner{}

	runOnce(t, spy, runner, nil)

	calls, token := runner.observed()
	if calls != 1 {
		t.Fatalf("runner ran %d times, want 1", calls)
	}
	if token != "" {
		t.Errorf("a deployment with no minter handed the worker %q", token)
	}
}

// TestRunTokenFailureStopsDispatch keeps an unauthenticated worker from
// starting. Letting it run would surface the missing credential as a failed
// model call partway through the work, which reads as a model problem rather
// than a dispatch one.
func TestRunTokenFailureStopsDispatch(t *testing.T) {
	mintFailed := errors.New("no signing secret")

	tests := []struct {
		name    string
		task    *model.Task
		mint    MintRunToken
		wantMsg string
	}{
		{
			name:    "the minter refuses",
			task:    &model.Task{ID: "t_test", TeamID: "tm_test", CreatedBy: "u_test"},
			mint:    func(authtoken.RunClaims) (string, error) { return "", mintFailed },
			wantMsg: mintFailed.Error(),
		},
		{
			name:    "the run has no task to attribute it to",
			task:    nil,
			mint:    func(authtoken.RunClaims) (string, error) { return "signed", nil },
			wantMsg: "has no task",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spy := newSpyTaskRunStore("r_fail1234567890123456")
			spy.task = tc.task
			runner := &recordingRunner{}

			runOnce(t, spy, runner, tc.mint)

			if calls, _ := runner.observed(); calls != 0 {
				t.Errorf("a worker was started %d times without a credential", calls)
			}

			spy.mu.Lock()
			defer spy.mu.Unlock()
			if spy.lastUpdateStatus == nil {
				t.Fatal("the run was left in SCHEDULED with no worker coming for it")
			}
			if spy.lastUpdateStatus.status != string(model.RunStatusFailed) {
				t.Errorf("run status = %q, want FAILED", spy.lastUpdateStatus.status)
			}
			if spy.lastUpdateStatus.errorMessage == nil {
				t.Fatal("the failure recorded no cause")
			}
			if msg := *spy.lastUpdateStatus.errorMessage; !strings.Contains(msg, tc.wantMsg) {
				t.Errorf("error message %q does not mention %q", msg, tc.wantMsg)
			}
		})
	}
}
