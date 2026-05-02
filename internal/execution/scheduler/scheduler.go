// Scheduler polls for pending task runs and runs the worker via a WorkerRunner (local process or k8s Job).
// It does not perform run execution; the worker process calls executor.RunTask.
package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"buildmax/internal/infra/db"
)

const (
	defaultPollInterval   = 5 * time.Second
	maxErrorMessageLength = 500
)

// Scheduler polls the task run store for PENDING runs and runs the worker via the configured runner.
type Scheduler struct {
	chatRuns     db.TaskRunStore
	runner       WorkerRunner
	pollInterval time.Duration
	stopCh       chan struct{}
	doneCh       chan struct{}
}

// NewScheduler creates a Scheduler that polls for pending task runs and runs the worker via the given runner. Call Start() to begin polling.
func NewScheduler(chatRunStore db.TaskRunStore, runner WorkerRunner) (*Scheduler, error) {
	return NewSchedulerWithPollInterval(chatRunStore, runner, defaultPollInterval)
}

// NewSchedulerWithPollInterval is like NewScheduler but allows setting the poll interval (e.g. for tests). Use 0 for default.
func NewSchedulerWithPollInterval(chatRunStore db.TaskRunStore, runner WorkerRunner, pollInterval time.Duration) (*Scheduler, error) {
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
			run, err := s.chatRuns.GetNextPendingTaskRun(ctx)
			if err != nil {
				slog.Warn("scheduler: poll failed", "err", err)
				continue
			}
			if run == nil {
				continue
			}
			updated, err := s.chatRuns.ClaimTaskRun(ctx, db.ClaimTaskRunInput{
				TaskRunID:      run.TaskRunID,
				ExpectedStatus: db.RunStatusPending,
				NewStatus:      db.RunStatusScheduled,
			})
			if err != nil {
				slog.Warn("scheduler: claim failed", "task_run_id", run.TaskRunID, "err", err)
				continue
			}
			if !updated {
				continue // another scheduler claimed it
			}
			workerType, k8sName, k8sAt, err := s.runner.Run(ctx, *run)
			if err != nil {
				slog.Warn("scheduler: worker spawn failed, marking run as FAILED", "task_run_id", run.TaskRunID, "err", err)
				errorMsg := err.Error()
				if len(errorMsg) > maxErrorMessageLength {
					errorMsg = errorMsg[:maxErrorMessageLength]
				}
				endedAt := time.Now().Unix()
				if updateErr := s.chatRuns.UpdateRun(ctx, db.UpdateTaskRunInput{
					TaskRunID:    run.TaskRunID,
					Status:       db.RunStatusFailed,
					EndedAt:      &endedAt,
					ErrorMessage: &errorMsg,
				}); updateErr != nil {
					slog.Error("scheduler: failed to set run to FAILED", "task_run_id", run.TaskRunID, "err", updateErr)
					continue
				}
				if syncErr := s.chatRuns.SyncTaskFromRun(ctx, run.TaskRunID); syncErr != nil {
					slog.Warn("scheduler: failed to sync chat from run", "task_run_id", run.TaskRunID, "err", syncErr)
				}
				continue
			}
			if err := s.chatRuns.UpdateTaskRunWorkerInfo(ctx, run.TaskRunID, workerType, k8sName, k8sAt); err != nil {
				slog.Warn("scheduler: failed to persist worker info", "task_run_id", run.TaskRunID, "err", err)
			}
		}
	}
}
