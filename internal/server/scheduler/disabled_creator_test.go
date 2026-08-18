package scheduler

import (
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
)

// runnerCalls reads how many workers the runner was asked to spawn.
func runnerCalls(r *recordingRunner) int {
	calls, _ := r.observed()
	return calls
}

// TestSchedulerDoesNotDispatchForADisabledAccount.
//
// Disabling an account has to stop work it queued, or "disable" means "stops
// signing in" rather than "stops acting". The run fails at dispatch rather than
// being left PENDING: a run nobody will ever pick up, sitting in a queue with
// no explanation, is worse than a terminal one that says why.
func TestSchedulerDoesNotDispatchForADisabledAccount(t *testing.T) {
	taskRunID := "r_disabled12345678901234"
	spy := newSpyTaskRunStore(taskRunID)
	spy.pendingRun.CreatedBy = "u_disabled"

	users := &mock.MockUserStore{}
	disabledAt := int64(1)
	users.ByID = map[string]*model.User{
		"u_disabled": {UserID: "u_disabled", Email: "gone@example.com", DisabledAt: &disabledAt},
	}
	runner := &recordingRunner{}

	s, err := NewSchedulerWithPollInterval(spy, runner, nil, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	s.WithUserStore(users).Start()
	time.Sleep(25 * time.Millisecond)
	s.Stop()

	if runnerCalls(runner) != 0 {
		t.Errorf("a worker was spawned for a disabled account: %d", runnerCalls(runner))
	}

	spy.mu.Lock()
	defer spy.mu.Unlock()
	if spy.lastUpdateStatus == nil {
		t.Fatal("the run was left PENDING with no explanation")
	}
	if spy.lastUpdateStatus.status != "FAILED" {
		t.Errorf("status = %q, want FAILED", spy.lastUpdateStatus.status)
	}
	if spy.lastUpdateStatus.errorMessage == nil || !strings.Contains(*spy.lastUpdateStatus.errorMessage, "disabled") {
		t.Errorf("the run should say why it did not start, got %v", spy.lastUpdateStatus.errorMessage)
	}
}

// TestSchedulerDispatchesForAnEnabledAccount is the other half: the guard must
// not stop ordinary work, including when the deployment wires no user store at
// all.
func TestSchedulerDispatchesForAnEnabledAccount(t *testing.T) {
	for _, tc := range []struct {
		name  string
		users model.UserStore
	}{
		{"enabled account", &mock.MockUserStore{ByID: map[string]*model.User{
			"u_active": {UserID: "u_active", Email: "here@example.com"},
		}}},
		{"no user store", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spy := newSpyTaskRunStore("r_active123456789012345")
			spy.pendingRun.CreatedBy = "u_active"
			runner := &recordingRunner{}

			s, err := NewSchedulerWithPollInterval(spy, runner, nil, 10*time.Millisecond)
			if err != nil {
				t.Fatal(err)
			}
			s.WithUserStore(tc.users).Start()
			time.Sleep(25 * time.Millisecond)
			s.Stop()

			if runnerCalls(runner) == 0 {
				t.Error("the run was not dispatched")
			}
		})
	}
}
