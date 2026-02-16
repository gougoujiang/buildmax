// Package executor runs pending tasks by spawning the buildmax CLI agent.
package executor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"buildmax/internal/store"

	"github.com/google/uuid"
)

const defaultPollInterval = 5 * time.Second

// TaskStore is the subset of store.TaskStore that the executor needs.
type TaskStore interface {
	GetNextPendingTask(ctx context.Context) (*store.Task, error)
	UpdateTaskStatus(ctx context.Context, taskID, status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) error
}

// Runner polls for pending tasks and executes them one at a time.
type Runner struct {
	store         TaskStore
	workspacesDir string
	pollInterval  time.Duration
	stopCh        chan struct{}
	doneCh        chan struct{}
}

// New creates a Runner. Call Start() to begin polling.
func New(store TaskStore, workspacesDir string) *Runner {
	return &Runner{
		store:         store,
		workspacesDir: workspacesDir,
		pollInterval:  defaultPollInterval,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
}

// Start launches the poll loop in a background goroutine.
func (r *Runner) Start() {
	go r.loop()
	slog.Info("executor started", "poll_interval", r.pollInterval)
}

// Stop signals the loop to exit and blocks until any in-flight task finishes.
func (r *Runner) Stop() {
	close(r.stopCh)
	<-r.doneCh
	slog.Info("executor stopped")
}

// loop is the main poll loop. It checks for pending tasks on each tick.
func (r *Runner) loop() {
	defer close(r.doneCh)
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			ctx := context.Background()
			task, err := r.store.GetNextPendingTask(ctx)
			if err != nil {
				slog.Warn("executor: poll failed", "err", err)
				continue
			}
			if task == nil {
				continue
			}
			r.executeTask(ctx, *task)
		}
	}
}

// executeTask runs the buildmax CLI for a single task and updates the DB with the result.
func (r *Runner) executeTask(ctx context.Context, task store.Task) {
	sessionID := uuid.New().String()
	slog.Info("executor: running task", "task_id", task.TaskID, "workspace_id", task.WorkspaceID, "session_id", sessionID)

	// Mark task as RUNNING and persist the generated session id.
	now := time.Now().Unix()
	if err := r.store.UpdateTaskStatus(ctx, task.TaskID, "RUNNING", &now, nil, nil, nil, &sessionID); err != nil {
		slog.Error("executor: failed to mark task RUNNING", "task_id", task.TaskID, "err", err)
		return
	}

	// Resolve workspace directory.
	wsDir := filepath.Join(r.workspacesDir, task.WorkspaceID)
	if err := os.MkdirAll(wsDir, 0755); err != nil {
		r.failTask(ctx, task.TaskID, fmt.Sprintf("failed to create workspace dir: %v", err))
		return
	}

	// Spawn buildmax -p "<input>" --session-id <uuid>.
	cmd := exec.CommandContext(ctx, "buildmax", "-p", task.Input, "--session-id", sessionID)
	cmd.Dir = wsDir

	output, err := cmd.CombinedOutput()
	endTime := time.Now().Unix()
	outputStr := string(output)

	// Write result file regardless of success/failure.
	resultPath := filepath.Join(wsDir, fmt.Sprintf("result-%s.md", task.TaskID))
	if writeErr := os.WriteFile(resultPath, output, 0644); writeErr != nil {
		slog.Warn("executor: failed to write result file", "task_id", task.TaskID, "err", writeErr)
	}

	if err != nil {
		errMsg := fmt.Sprintf("buildmax exited with error: %v", err)
		slog.Warn("executor: task failed", "task_id", task.TaskID, "err", err)
		if updateErr := r.store.UpdateTaskStatus(ctx, task.TaskID, "FAILED", nil, &endTime, &outputStr, &errMsg, nil); updateErr != nil {
			slog.Error("executor: failed to update task status", "task_id", task.TaskID, "err", updateErr)
		}
		return
	}

	slog.Info("executor: task succeeded", "task_id", task.TaskID)
	if updateErr := r.store.UpdateTaskStatus(ctx, task.TaskID, "SUCCEEDED", nil, &endTime, &outputStr, nil, nil); updateErr != nil {
		slog.Error("executor: failed to update task status", "task_id", task.TaskID, "err", updateErr)
	}
}

// failTask is a helper to mark a task as FAILED when execution cannot proceed.
func (r *Runner) failTask(ctx context.Context, taskID, errMsg string) {
	slog.Warn("executor: task failed", "task_id", taskID, "err", errMsg)
	endTime := time.Now().Unix()
	if err := r.store.UpdateTaskStatus(ctx, taskID, "FAILED", nil, &endTime, nil, &errMsg, nil); err != nil {
		slog.Error("executor: failed to update task status", "task_id", taskID, "err", err)
	}
}
