// Scheduler polls for pending tasks and spawns the buildmax-worker binary for each.
// It does not perform task execution; the worker process calls executor.RunTask.
package executor

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"buildmax/internal/storage/entity"
)

const defaultPollInterval = 5 * time.Second

// Scheduler polls the task store for PENDING tasks and spawns the worker binary (--task-id).
// It holds no blob storage or workspace paths; the worker fetches the task and runs it via RunTask.
type Scheduler struct {
	tasks        entity.TaskStore
	workerPath   string
	pollInterval time.Duration
	stopCh       chan struct{}
	doneCh       chan struct{}
}

// NewScheduler creates a Scheduler that polls and spawns the worker binary. Call Start() to begin polling.
func NewScheduler(taskStore entity.TaskStore, workerPath string) (*Scheduler, error) {
	if taskStore == nil {
		return nil, errors.New("executor: taskStore must not be nil")
	}
	if workerPath == "" {
		return nil, errors.New("executor: workerPath must not be empty")
	}
	return &Scheduler{
		tasks:        taskStore,
		workerPath:   workerPath,
		pollInterval: defaultPollInterval,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}, nil
}

// Start launches the poll loop in a background goroutine.
func (s *Scheduler) Start() {
	go s.loop()
	slog.Info("scheduler started", "poll_interval", s.pollInterval, "worker", s.workerPath)
}

// Stop signals the loop to exit and blocks until it has finished.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	<-s.doneCh
	slog.Info("scheduler stopped")
}

// loop is the main poll loop: on each tick it fetches the next PENDING task, claims it (PENDING→SCHEDULED), and spawns the worker.
// If spawn fails, the task is reverted to PENDING so the next poll retries.
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
			s.spawnWorker(ctx, *task)
		}
	}
}

// spawnWorker runs the worker binary with --task-id. If spawn fails, the task is reverted to PENDING.
func (s *Scheduler) spawnWorker(ctx context.Context, task entity.Task) {
	slog.Info("scheduler: spawning worker", "task_id", task.TaskID, "workspace_id", task.WorkspaceID)
	cmd := exec.CommandContext(ctx, s.workerPath, "--task-id", task.TaskID)
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		slog.Warn("scheduler: worker exited with error, reverting task to PENDING", "task_id", task.TaskID, "err", err)
		if revertErr := s.tasks.UpdateTaskStatus(ctx, task.TaskID, "PENDING", nil, nil, nil, nil, nil); revertErr != nil {
			slog.Error("scheduler: failed to revert task to PENDING", "task_id", task.TaskID, "err", revertErr)
		}
	}
}
