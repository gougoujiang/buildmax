// Scheduler polls for pending tasks and runs the worker via a WorkerRunner (local process or k8s Job).
// It does not perform task execution; the worker process calls executor.RunTask.
package executor

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"buildmax/internal/storage/entity"
)

const defaultPollInterval = 5 * time.Second

// Scheduler polls the task run store for PENDING runs and runs the worker via the configured runner.
type Scheduler struct {
	taskRuns     entity.TaskRunStore
	runner       WorkerRunner
	pollInterval time.Duration
	stopCh       chan struct{}
	doneCh       chan struct{}
}

// NewScheduler creates a Scheduler that polls for pending task runs and runs the worker via the given runner. Call Start() to begin polling.
func NewScheduler(taskRunStore entity.TaskRunStore, runner WorkerRunner) (*Scheduler, error) {
	if taskRunStore == nil {
		return nil, errors.New("executor: taskRunStore must not be nil")
	}
	if runner == nil {
		return nil, errors.New("executor: runner must not be nil")
	}
	return &Scheduler{
		taskRuns:     taskRunStore,
		runner:       runner,
		pollInterval: defaultPollInterval,
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
// If run fails, the run is reverted to PENDING so the next poll retries.
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
			run, err := s.taskRuns.GetNextPendingTaskRun(ctx)
			if err != nil {
				slog.Warn("scheduler: poll failed", "err", err)
				continue
			}
			if run == nil {
				continue
			}
			updated, err := s.taskRuns.UpdateTaskRunStatusIf(ctx, run.RunID, "PENDING", "SCHEDULED", nil, nil, nil, nil, nil)
			if err != nil {
				slog.Warn("scheduler: claim failed", "run_id", run.RunID, "err", err)
				continue
			}
			if !updated {
				continue // another scheduler claimed it
			}
			workerType, k8sName, k8sAt, err := s.runner.Run(ctx, *run)
			if err != nil {
				slog.Warn("scheduler: worker run failed, reverting run to PENDING", "run_id", run.RunID, "err", err)
				if revertErr := s.taskRuns.UpdateTaskRunStatus(ctx, run.RunID, "PENDING", nil, nil, nil, nil, nil); revertErr != nil {
					slog.Error("scheduler: failed to revert run to PENDING", "run_id", run.RunID, "err", revertErr)
				}
				continue
			}
			if err := s.taskRuns.UpdateTaskRunWorkerInfo(ctx, run.RunID, workerType, k8sName, k8sAt); err != nil {
				slog.Warn("scheduler: failed to persist worker info", "run_id", run.RunID, "err", err)
			}
		}
	}
}
