// Package executor runs pending tasks by spawning the buildmax-worker binary (scheduler)
// and provides RunTask for the worker to execute a single task (materialize, buildmax -p, update via TaskUpdater).
package executor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"buildmax/internal/config"
	"buildmax/internal/storage/blob"
	"buildmax/internal/storage/entity"
	"buildmax/internal/util"
)

const defaultPollInterval = 5 * time.Second

// ArtifactPayload is passed to TaskUpdater when registering an artifact on success.
type ArtifactPayload struct {
	ArtifactID    string
	RelativePath  string
}

// TaskUpdater is used by the worker to update task status and register artifacts via HTTP (or other backend).
type TaskUpdater interface {
	// UpdateTaskStatus updates task status and optional fields. For SUCCEEDED with artifact, pass non-nil artifact.
	UpdateTaskStatus(ctx context.Context, taskID, status string, startedAt, endedAt *int64, output, errMsg, sessionID *string, artifact *ArtifactPayload) error
}

// Runner polls for pending tasks and spawns the worker binary for each (scheduler only; no storage).
type Runner struct {
	tasks        entity.TaskStore
	workerPath   string
	pollInterval time.Duration
	stopCh       chan struct{}
	doneCh       chan struct{}
}

// NewRunner creates a Runner that polls and spawns the worker binary. Call Start() to begin polling.
func NewRunner(taskStore entity.TaskStore, workerPath string) (*Runner, error) {
	if taskStore == nil {
		return nil, errors.New("executor: taskStore must not be nil")
	}
	if workerPath == "" {
		return nil, errors.New("executor: workerPath must not be empty")
	}
	return &Runner{
		tasks:        taskStore,
		workerPath:   workerPath,
		pollInterval: defaultPollInterval,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}, nil
}

// Start launches the poll loop in a background goroutine.
func (r *Runner) Start() {
	go r.loop()
	slog.Info("executor started", "poll_interval", r.pollInterval, "worker", r.workerPath)
}

// Stop signals the loop to exit and blocks until any in-flight task finishes.
func (r *Runner) Stop() {
	close(r.stopCh)
	<-r.doneCh
	slog.Info("executor stopped")
}

// loop is the main poll loop. It checks for pending tasks on each tick and spawns the worker.
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
			task, err := r.tasks.GetNextPendingTask(ctx)
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

// executeTask spawns the worker binary with --task-id and waits. The worker claims the task and performs execution.
func (r *Runner) executeTask(ctx context.Context, task entity.Task) {
	slog.Info("executor: spawning worker", "task_id", task.TaskID, "workspace_id", task.WorkspaceID)
	cmd := exec.CommandContext(ctx, r.workerPath, "--task-id", task.TaskID)
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		slog.Warn("executor: worker exited with error", "task_id", task.TaskID, "err", err)
	}
}

// RunTask runs a single task: materialize workspace, run buildmax -p, then update status and optional artifact via updater.
// Used by the buildmax-worker binary. task must not be nil; sessionID is the worker-generated session id.
func RunTask(ctx context.Context, task *entity.Task, sessionID string, paths WorkspacePaths, persist blob.PersistStorage, artifactStorage blob.ArtifactStorage, updater TaskUpdater) error {
	if task == nil {
		return errors.New("executor: task must not be nil")
	}
	if paths == nil || persist == nil || artifactStorage == nil || updater == nil {
		return errors.New("executor: paths, persist, artifactStorage and updater must not be nil")
	}

	taskDir := paths.RuntimeWorkspaceDir(task.WorkspaceID, task.TaskID)
	buildmaxDir := paths.RuntimeTaskBuildmaxDir(task.WorkspaceID, task.TaskID)
	wsDir := paths.RuntimeTaskWSDir(task.WorkspaceID, task.TaskID)

	if err := os.MkdirAll(buildmaxDir, 0755); err != nil {
		_ = updater.UpdateTaskStatus(ctx, task.TaskID, "FAILED", nil, ptrInt64(time.Now().Unix()), nil, ptrString(fmt.Sprintf("failed to create buildmax dir: %v", err)), nil, nil)
		return err
	}
	if err := os.MkdirAll(wsDir, 0755); err != nil {
		_ = updater.UpdateTaskStatus(ctx, task.TaskID, "FAILED", nil, ptrInt64(time.Now().Unix()), nil, ptrString(fmt.Sprintf("failed to create ws dir: %v", err)), nil, nil)
		return err
	}
	if err := persist.MaterializeToDir(ctx, task.WorkspaceID, wsDir); err != nil {
		_ = updater.UpdateTaskStatus(ctx, task.TaskID, "FAILED", nil, ptrInt64(time.Now().Unix()), nil, ptrString(fmt.Sprintf("failed to materialize workspace: %v", err)), nil, nil)
		return err
	}

	env := os.Environ()
	prefix := config.EnvKeyBuildmaxHome + "="
	filtered := make([]string, 0, len(env)+1)
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			filtered = append(filtered, e)
		}
	}
	cmd := exec.CommandContext(ctx, "buildmax", "-p", task.Input, "--session-id", sessionID)
	cmd.Dir = wsDir
	cmd.Env = append(filtered, prefix+buildmaxDir)

	output, err := cmd.CombinedOutput()
	endTime := time.Now().Unix()
	outputStr := string(output)

	resultFilename := fmt.Sprintf("result-%s.md", task.TaskID)
	resultPath := filepath.Join(taskDir, resultFilename)
	if writeErr := os.WriteFile(resultPath, output, 0644); writeErr != nil {
		slog.Warn("executor: failed to write result file", "task_id", task.TaskID, "err", writeErr)
	}

	if err != nil {
		errMsg := fmt.Sprintf("buildmax exited with error: %v", err)
		slog.Warn("executor: task failed", "task_id", task.TaskID, "err", err)
		if updateErr := updater.UpdateTaskStatus(ctx, task.TaskID, "FAILED", nil, &endTime, &outputStr, &errMsg, nil, nil); updateErr != nil {
			slog.Error("executor: failed to update task status", "task_id", task.TaskID, "err", updateErr)
		}
		return err
	}

	artifactID := util.NewID()
	if putErr := artifactStorage.PutResult(ctx, task.WorkspaceID, task.TaskID, artifactID, output); putErr != nil {
		slog.Warn("executor: failed to write result to artifact storage", "task_id", task.TaskID, "err", putErr)
	}
	if updateErr := updater.UpdateTaskStatus(ctx, task.TaskID, "SUCCEEDED", nil, &endTime, &outputStr, nil, nil, &ArtifactPayload{ArtifactID: artifactID, RelativePath: resultFilename}); updateErr != nil {
		slog.Error("executor: failed to update task status", "task_id", task.TaskID, "err", updateErr)
		return updateErr
	}

	slog.Info("executor: task succeeded", "task_id", task.TaskID)
	return nil
}

func ptrString(s string) *string { return &s }
func ptrInt64(n int64) *int64   { return &n }
