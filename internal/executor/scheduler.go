// Scheduler polls for pending chat runs and runs the worker via a WorkerRunner (local process or k8s Job).
// It does not perform run execution; the worker process calls executor.RunTask.
package executor

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"buildmax/internal/storage/entity"
)

const (
	defaultPollInterval   = 5 * time.Second
	maxErrorMessageLength = 500
)

// Scheduler polls the chat run store for PENDING runs and runs the worker via the configured runner.
type Scheduler struct {
	chatRuns     entity.ChatRunStore
	runner       WorkerRunner
	pollInterval time.Duration
	stopCh       chan struct{}
	doneCh       chan struct{}
}

// NewScheduler creates a Scheduler that polls for pending chat runs and runs the worker via the given runner. Call Start() to begin polling.
func NewScheduler(chatRunStore entity.ChatRunStore, runner WorkerRunner) (*Scheduler, error) {
	return NewSchedulerWithPollInterval(chatRunStore, runner, defaultPollInterval)
}

// NewSchedulerWithPollInterval is like NewScheduler but allows setting the poll interval (e.g. for tests). Use 0 for default.
func NewSchedulerWithPollInterval(chatRunStore entity.ChatRunStore, runner WorkerRunner, pollInterval time.Duration) (*Scheduler, error) {
	if chatRunStore == nil {
		return nil, errors.New("executor: chatRunStore must not be nil")
	}
	if runner == nil {
		return nil, errors.New("executor: runner must not be nil")
	}
	if pollInterval == 0 {
		pollInterval = defaultPollInterval
	}
	return &Scheduler{
		chatRuns:     chatRunStore,
		runner:       runner,
		pollInterval: pollInterval,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}, nil
}

// Start launches the poll loop in a background goroutine.
func (s *Scheduler) Start() {
	go s.loop()
	slog.Info("scheduler started", "poll_interval", s.pollInterval)
}

// Stop signals the loop to exit and blocks until it has finished.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	<-s.doneCh
	slog.Info("scheduler stopped")
}

// loop is the main poll loop: on each tick it fetches the next PENDING run, claims it (PENDING→SCHEDULED), runs the worker, and persists worker info on success.
// State machine: PENDING → SCHEDULED → RUNNING → SUCCEEDED/FAILED. If spawn fails, run is set to FAILED (no revert to PENDING).
func (s *Scheduler) loop() {
	defer close(s.doneCh)
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			ctx := context.Background()
			run, err := s.chatRuns.GetNextPendingChatRun(ctx)
			if err != nil {
				slog.Warn("scheduler: poll failed", "err", err)
				continue
			}
			if run == nil {
				continue
			}
			updated, err := s.chatRuns.ClaimChatRun(ctx, entity.ClaimChatRunInput{
				ChatRunID:      run.ChatRunID,
				ExpectedStatus: entity.RunStatusPending,
				NewStatus:      entity.RunStatusScheduled,
			})
			if err != nil {
				slog.Warn("scheduler: claim failed", "chat_run_id", run.ChatRunID, "err", err)
				continue
			}
			if !updated {
				continue // another scheduler claimed it
			}
			workerType, k8sName, k8sAt, err := s.runner.Run(ctx, *run)
			if err != nil {
				slog.Warn("scheduler: worker spawn failed, marking run as FAILED", "chat_run_id", run.ChatRunID, "err", err)
				errorMsg := err.Error()
				if len(errorMsg) > maxErrorMessageLength {
					errorMsg = errorMsg[:maxErrorMessageLength]
				}
				endedAt := time.Now().Unix()
				if updateErr := s.chatRuns.UpdateRun(ctx, entity.UpdateChatRunInput{
					ChatRunID:    run.ChatRunID,
					Status:       entity.RunStatusFailed,
					EndedAt:      &endedAt,
					ErrorMessage: &errorMsg,
				}); updateErr != nil {
					slog.Error("scheduler: failed to set run to FAILED", "chat_run_id", run.ChatRunID, "err", updateErr)
					continue
				}
				if syncErr := s.chatRuns.SyncChatFromRun(ctx, run.ChatRunID); syncErr != nil {
					slog.Warn("scheduler: failed to sync chat from run", "chat_run_id", run.ChatRunID, "err", syncErr)
				}
				continue
			}
			if err := s.chatRuns.UpdateChatRunWorkerInfo(ctx, run.ChatRunID, workerType, k8sName, k8sAt); err != nil {
				slog.Warn("scheduler: failed to persist worker info", "chat_run_id", run.ChatRunID, "err", err)
			}
		}
	}
}
