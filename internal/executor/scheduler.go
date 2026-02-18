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

// Scheduler polls the task store for PENDING tasks and runs the worker via the configured runner.
type Scheduler struct {
	tasks        entity.TaskStore
	runner       WorkerRunner
	pollInterval time.Duration
	stopCh       chan struct{}
	doneCh       chan struct{}
}

// NewScheduler creates a Scheduler that polls and runs the worker via the given runner. Call Start() to begin polling.
func NewScheduler(taskStore entity.TaskStore, runner WorkerRunner) (*Scheduler, error) {
	if taskStore == nil {
		return nil, errors.New("executor: taskStore must not be nil")
	}
	if runner == nil {
		return nil, errors.New("executor: runner must not be nil")
	}
	return &Scheduler{
		tasks:        taskStore,
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

// loop is the main poll loop: on each tick it fetches the next PENDING task, claims it (PENDING→SCHEDULED), runs the worker, and persists worker info on success.
// If run fails, the task is reverted to PENDING so the next poll retries.
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
			task, err := s.tasks.GetNextPendingTask(ctx)
			if err != nil {
				slog.Warn("scheduler: poll failed", "err", err)
				continue
			}
			if task == nil {
				continue
			}
			updated, err := s.tasks.UpdateTaskStatusIf(ctx, task.TaskID, "PENDING", "SCHEDULED", nil, nil, nil, nil, nil)
			if err != nil {
				slog.Warn("scheduler: claim failed", "task_id", task.TaskID, "err", err)
				continue
			}
			if !updated {
				continue // another scheduler claimed it
			}
			workerType, k8sName, k8sAt, err := s.runner.Run(ctx, *task)
			if err != nil {
				slog.Warn("scheduler: worker run failed, reverting task to PENDING", "task_id", task.TaskID, "err", err)
				if revertErr := s.tasks.UpdateTaskStatus(ctx, task.TaskID, "PENDING", nil, nil, nil, nil, nil); revertErr != nil {
					slog.Error("scheduler: failed to revert task to PENDING", "task_id", task.TaskID, "err", revertErr)
				}
				continue
			}
			if err := s.tasks.UpdateTaskWorkerInfo(ctx, task.TaskID, workerType, k8sName, k8sAt); err != nil {
				slog.Warn("scheduler: failed to persist worker info", "task_id", task.TaskID, "err", err)
			}
		}
	}
}
