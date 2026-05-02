package executor

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"buildmax/internal/storage/entity"
)

// spyTaskRunStore records UpdateTaskRunStatus and SyncTaskFromRun calls for tests.
type spyTaskRunStore struct {
	mu sync.Mutex

	// GetNextPendingTaskRun: return one run on first call, nil thereafter
	pendingRun   *entity.TaskRun
	pendingCalls int

	// UpdateTaskRunStatusIf: return true for first claim (PENDING→SCHEDULED)
	statusIfCalls int

	// Recorded calls
	lastUpdateStatus *struct {
		chatRunID    string
		status       string
		endedAt      *int64
		errorMessage *string
	}
	syncChatFromRunCalls []string
}

func newSpyTaskRunStore(chatRunID string) *spyTaskRunStore {
	return &spyTaskRunStore{
		pendingRun: &entity.TaskRun{
			TaskRunID: chatRunID,
			TaskID:    "t_test",
			Input:     "input",
			Status:    "PENDING",
			CreatedAt: time.Now().Unix(),
		},
	}
}

func (s *spyTaskRunStore) CreateTaskRun(_ context.Context, _, _, _, _, _ string) (*entity.TaskRun, error) {
	return nil, nil
}

func (s *spyTaskRunStore) GetNextPendingTaskRun(_ context.Context) (*entity.TaskRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingCalls++
	if s.pendingCalls == 1 && s.pendingRun != nil {
		return s.pendingRun, nil
	}
	return nil, nil
}

func (s *spyTaskRunStore) GetTaskRun(_ context.Context, _ string) (*entity.TaskRun, error) {
	return nil, nil
}

func (s *spyTaskRunStore) GetTaskRunWithTask(_ context.Context, _ string) (*entity.TaskRun, *entity.Task, error) {
	return nil, nil, nil
}

func (s *spyTaskRunStore) ClaimTaskRun(_ context.Context, in entity.ClaimTaskRunInput) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statusIfCalls++
	if s.statusIfCalls == 1 && in.ExpectedStatus == entity.RunStatusPending && in.NewStatus == entity.RunStatusScheduled {
		return true, nil
	}
	return false, nil
}

func (s *spyTaskRunStore) UpdateRun(_ context.Context, in entity.UpdateTaskRunInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUpdateStatus = &struct {
		chatRunID    string
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

func (s *spyTaskRunStore) SyncTaskFromRun(_ context.Context, chatRunID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncChatFromRunCalls = append(s.syncChatFromRunCalls, chatRunID)
	return nil
}

// failingRunner implements WorkerRunner and always returns an error.
type failingRunner struct{ err error }

func (f *failingRunner) Run(_ context.Context, _ entity.TaskRun) (string, *string, *int64, error) {
	if f.err != nil {
		return "", nil, nil, f.err
	}
	return "", nil, nil, errSpawnFailed
}

var errSpawnFailed = errors.New("spawn failed for test")

func TestScheduler_Loop_SpawnFailure_MarksRunFailed(t *testing.T) {
	chatRunID := "r_spy123456789012345678"
	spy := newSpyTaskRunStore(chatRunID)
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
		t.Errorf("UpdateTaskRunStatus status = %q, want FAILED", spy.lastUpdateStatus.status)
	}
	if spy.lastUpdateStatus.chatRunID != chatRunID {
		t.Errorf("UpdateTaskRunStatus chatRunID = %q, want %q", spy.lastUpdateStatus.chatRunID, chatRunID)
	}
	if spy.lastUpdateStatus.endedAt == nil {
		t.Error("UpdateTaskRunStatus endedAt is nil, want set")
	}
	if spy.lastUpdateStatus.errorMessage == nil || *spy.lastUpdateStatus.errorMessage == "" {
		t.Error("UpdateTaskRunStatus errorMessage is nil or empty, want non-empty")
	}

	// SyncTaskFromRun must have been called with the same chatRunID
	if len(spy.syncChatFromRunCalls) == 0 {
		t.Fatal("SyncTaskFromRun was not called")
	}
	if spy.syncChatFromRunCalls[0] != chatRunID {
		t.Errorf("SyncTaskFromRun chatRunID = %q, want %q", spy.syncChatFromRunCalls[0], chatRunID)
	}
}
