package executor

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"buildmax/internal/storage/entity"
)

// spyChatRunStore records UpdateChatRunStatus and SyncChatFromRun calls for tests.
type spyChatRunStore struct {
	mu sync.Mutex

	// GetNextPendingChatRun: return one run on first call, nil thereafter
	pendingRun   *entity.ChatRun
	pendingCalls int

	// UpdateChatRunStatusIf: return true for first claim (PENDING→SCHEDULED)
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

func newSpyChatRunStore(chatRunID string) *spyChatRunStore {
	return &spyChatRunStore{
		pendingRun: &entity.ChatRun{
			ChatRunID: chatRunID,
			ChatID:    "c_test",
			Input:     "input",
			Status:    "PENDING",
			CreatedAt: time.Now().Unix(),
		},
	}
}

func (s *spyChatRunStore) CreateChatRun(_ context.Context, _, _, _ string) (*entity.ChatRun, error) {
	return nil, nil
}

func (s *spyChatRunStore) GetNextPendingChatRun(_ context.Context) (*entity.ChatRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingCalls++
	if s.pendingCalls == 1 && s.pendingRun != nil {
		return s.pendingRun, nil
	}
	return nil, nil
}

func (s *spyChatRunStore) GetChatRun(_ context.Context, _ string) (*entity.ChatRun, error) {
	return nil, nil
}

func (s *spyChatRunStore) GetChatRunWithChat(_ context.Context, _ string) (*entity.ChatRun, *entity.Chat, error) {
	return nil, nil, nil
}

func (s *spyChatRunStore) ClaimChatRun(_ context.Context, in entity.ClaimChatRunInput) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statusIfCalls++
	if s.statusIfCalls == 1 && in.ExpectedStatus == entity.RunStatusPending && in.NewStatus == entity.RunStatusScheduled {
		return true, nil
	}
	return false, nil
}

func (s *spyChatRunStore) UpdateRun(_ context.Context, in entity.UpdateChatRunInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUpdateStatus = &struct {
		chatRunID    string
		status       string
		endedAt      *int64
		errorMessage *string
	}{in.ChatRunID, string(in.Status), in.EndedAt, in.ErrorMessage}
	return nil
}

func (s *spyChatRunStore) UpdateChatRunWorkerInfo(_ context.Context, _ string, _ string, _ *string, _ *int64) error {
	return nil
}

func (s *spyChatRunStore) OnRunComplete(_ context.Context, _ string, _ []string) error {
	return nil
}

func (s *spyChatRunStore) SyncChatFromRun(_ context.Context, chatRunID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncChatFromRunCalls = append(s.syncChatFromRunCalls, chatRunID)
	return nil
}

// failingRunner implements WorkerRunner and always returns an error.
type failingRunner struct{ err error }

func (f *failingRunner) Run(_ context.Context, _ entity.ChatRun) (string, *string, *int64, error) {
	if f.err != nil {
		return "", nil, nil, f.err
	}
	return "", nil, nil, errSpawnFailed
}

var errSpawnFailed = errors.New("spawn failed for test")

func TestScheduler_Loop_SpawnFailure_MarksRunFailed(t *testing.T) {
	chatRunID := "r_spy123456789012345678"
	spy := newSpyChatRunStore(chatRunID)
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

	// UpdateChatRunStatus must have been called with FAILED, not PENDING
	if spy.lastUpdateStatus == nil {
		t.Fatal("UpdateChatRunStatus was not called")
	}
	if spy.lastUpdateStatus.status != "FAILED" {
		t.Errorf("UpdateChatRunStatus status = %q, want FAILED", spy.lastUpdateStatus.status)
	}
	if spy.lastUpdateStatus.chatRunID != chatRunID {
		t.Errorf("UpdateChatRunStatus chatRunID = %q, want %q", spy.lastUpdateStatus.chatRunID, chatRunID)
	}
	if spy.lastUpdateStatus.endedAt == nil {
		t.Error("UpdateChatRunStatus endedAt is nil, want set")
	}
	if spy.lastUpdateStatus.errorMessage == nil || *spy.lastUpdateStatus.errorMessage == "" {
		t.Error("UpdateChatRunStatus errorMessage is nil or empty, want non-empty")
	}

	// SyncChatFromRun must have been called with the same chatRunID
	if len(spy.syncChatFromRunCalls) == 0 {
		t.Fatal("SyncChatFromRun was not called")
	}
	if spy.syncChatFromRunCalls[0] != chatRunID {
		t.Errorf("SyncChatFromRun chatRunID = %q, want %q", spy.syncChatFromRunCalls[0], chatRunID)
	}
}
