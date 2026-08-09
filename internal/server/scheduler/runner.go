// Package scheduler provides task run scheduling.
//
// WorkerRunner abstracts how a worker is started for a task run (local process or k8s Job).
package scheduler

import (
	"context"
	"log/slog"
	"os"
	"os/exec"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// WorkerRunner starts a worker for a task run. On success returns worker info to persist; on failure returns an error (caller should revert run to PENDING).
type WorkerRunner interface {
	Run(ctx context.Context, run model.TaskRun) (workerType string, k8sJobName *string, k8sJobCreatedAt *int64, err error)
}

// LocalRunner runs the worker binary as a local process (blocks until exit).
type LocalRunner struct {
	workerPath string
}

// NewLocalRunner returns a runner that exec's the worker binary with --task-run-id.
func NewLocalRunner(workerPath string) *LocalRunner {
	return &LocalRunner{workerPath: workerPath}
}

// Run executes the worker process; on success returns ("local_process", nil, nil, nil). On failure returns error.
func (r *LocalRunner) Run(ctx context.Context, run model.TaskRun) (workerType string, k8sJobName *string, k8sJobCreatedAt *int64, err error) {
	slog.Info("scheduler: spawning worker", "task_run_id", run.TaskRunID, "task_id", run.TaskID)
	cmd := exec.CommandContext(ctx, r.workerPath, "--task-run-id", run.TaskRunID)
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		return "", nil, nil, err
	}
	return "local_process", nil, nil, nil
}
