package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// spyTaskRunStore records UpdateRun and SyncTaskFromRun calls for tests.
type spyTaskRunStore struct {
	mu sync.Mutex

	// GetNextPendingTaskRun: return one run on first call, nil thereafter
	pendingRun   *model.TaskRun
	pendingCalls int

	// UpdateTaskRunStatusIf: return true for first claim (PENDING→SCHEDULED)
	statusIfCalls int

	// Recorded calls
	lastUpdateStatus *struct {
		taskRunID    string
		status       string
		endedAt      *int64
		errorMessage *string
	}
	syncTaskFromRunCalls []string
}

func newSpyTaskRunStore(taskRunID string) *spyTaskRunStore {
	return &spyTaskRunStore{
		pendingRun: &model.TaskRun{
			TaskRunID: taskRunID,
			TaskID:    "t_test",
			Input:     "input",
			Status:    "PENDING",
			CreatedAt: time.Now().Unix(),
		},
	}
}

func (s *spyTaskRunStore) CreateTaskRun(_ context.Context, _, _, _, _, _ string) (*model.TaskRun, error) {
	return nil, nil
}

func (s *spyTaskRunStore) GetNextPendingTaskRun(_ context.Context) (*model.TaskRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingCalls++
	if s.pendingCalls == 1 && s.pendingRun != nil {
		return s.pendingRun, nil
	}
	return nil, nil
}

func (s *spyTaskRunStore) GetTaskRun(_ context.Context, _ string) (*model.TaskRun, error) {
	return nil, nil
}

func (s *spyTaskRunStore) GetTaskRunWithTask(_ context.Context, _ string) (*model.TaskRun, *model.Task, error) {
	return nil, nil, nil
}

func (s *spyTaskRunStore) ClaimTaskRun(_ context.Context, in model.ClaimTaskRunInput) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statusIfCalls++
	if s.statusIfCalls == 1 && in.ExpectedStatus == model.RunStatusPending && in.NewStatus == model.RunStatusScheduled {
		return true, nil
	}
	return false, nil
}

func (s *spyTaskRunStore) UpdateRun(_ context.Context, in model.UpdateTaskRunInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUpdateStatus = &struct {
		taskRunID    string
		status       string
		endedAt      *int64
		errorMessage *string
	}{in.TaskRunID, string(in.Status), in.EndedAt, in.ErrorMessage}
	return nil
}

func (s *spyTaskRunStore) UpdateTaskRunWorkerInfo(_ context.Context, _ string, _ string, _ *string, _ *int64) error {
	return nil
}

func (s *spyTaskRunStore) OnRunComplete(_ context.Context, _ string, _ []string) error {
	return nil
}

func (s *spyTaskRunStore) SyncTaskFromRun(_ context.Context, taskRunID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncTaskFromRunCalls = append(s.syncTaskFromRunCalls, taskRunID)
	return nil
}

// failingRunner implements WorkerRunner and always returns an error.
type failingRunner struct{ err error }

func (f *failingRunner) Run(_ context.Context, _ model.TaskRun) (string, *string, *int64, error) {
	if f.err != nil {
		return "", nil, nil, f.err
	}
	return "", nil, nil, errSpawnFailed
}

var errSpawnFailed = errors.New("spawn failed for test")

func TestScheduler_Loop_SpawnFailure_MarksRunFailed(t *testing.T) {
	taskRunID := "r_spy123456789012345678"
	spy := newSpyTaskRunStore(taskRunID)
	runner := &failingRunner{err: errSpawnFailed}

	s, err := NewSchedulerWithPollInterval(spy, runner, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	s.Start()
	// Wait for at least one tick (10ms) so the loop picks the run and runner fails
	time.Sleep(25 * time.Millisecond)
	s.Stop()

	spy.mu.Lock()
	defer spy.mu.Unlock()

	// UpdateTaskRunStatus must have been called with FAILED, not PENDING
	if spy.lastUpdateStatus == nil {
		t.Fatal("UpdateTaskRunStatus was not called")
	}
	if spy.lastUpdateStatus.status != "FAILED" {
		t.Errorf("UpdateRun status = %q, want FAILED", spy.lastUpdateStatus.status)
	}
	if spy.lastUpdateStatus.taskRunID != taskRunID {
		t.Errorf("UpdateRun taskRunID = %q, want %q", spy.lastUpdateStatus.taskRunID, taskRunID)
	}
	if spy.lastUpdateStatus.endedAt == nil {
		t.Error("UpdateRun endedAt is nil, want set")
	}
	if spy.lastUpdateStatus.errorMessage == nil || *spy.lastUpdateStatus.errorMessage == "" {
		t.Error("UpdateRun errorMessage is nil or empty, want non-empty")
	}

	// SyncTaskFromRun must have been called with the same taskRunID.
	if len(spy.syncTaskFromRunCalls) == 0 {
		t.Fatal("SyncTaskFromRun was not called")
	}
	if spy.syncTaskFromRunCalls[0] != taskRunID {
		t.Errorf("SyncTaskFromRun taskRunID = %q, want %q", spy.syncTaskFromRunCalls[0], taskRunID)
	}
}
