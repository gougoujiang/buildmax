// Package executor runs pending tasks by spawning the buildmax CLI agent.
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

	"github.com/google/uuid"
)

const defaultPollInterval = 5 * time.Second

// Runner polls for pending tasks and executes them one at a time.
type Runner struct {
	tasks           entity.TaskStore
	artifacts       entity.ArtifactStore
	paths           WorkspacePaths
	persist         blob.PersistStorage
	artifactStorage blob.ArtifactStorage
	pollInterval    time.Duration
	stopCh          chan struct{}
	doneCh          chan struct{}
}

// New creates a Runner. Call Start() to begin polling.
func New(taskStore entity.TaskStore, artifactStore entity.ArtifactStore, paths WorkspacePaths, persist blob.PersistStorage, artifactStorage blob.ArtifactStorage) (*Runner, error) {
	if taskStore == nil {
		return nil, errors.New("executor: taskStore must not be nil")
	}
	if artifactStore == nil {
		return nil, errors.New("executor: artifactStore must not be nil")
	}
	if paths == nil {
		return nil, errors.New("executor: paths must not be nil")
	}
	if persist == nil {
		return nil, errors.New("executor: persist storage must not be nil")
	}
	if artifactStorage == nil {
		return nil, errors.New("executor: artifact storage must not be nil")
	}
	return &Runner{
		tasks:           taskStore,
		artifacts:       artifactStore,
		paths:           paths,
		persist:         persist,
		artifactStorage: artifactStorage,
		pollInterval:    defaultPollInterval,
		stopCh:          make(chan struct{}),
		doneCh:          make(chan struct{}),
	}, nil
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

// executeTask runs the buildmax CLI for a single task and updates the DB with the result.
func (r *Runner) executeTask(ctx context.Context, task entity.Task) {
	sessionID := uuid.New().String()
	slog.Info("executor: running task", "task_id", task.TaskID, "workspace_id", task.WorkspaceID, "session_id", sessionID)

	// Mark task as RUNNING and persist the generated session id.
	now := time.Now().Unix()
	if err := r.tasks.UpdateTaskStatus(ctx, task.TaskID, "RUNNING", &now, nil, nil, nil, &sessionID); err != nil {
		slog.Error("executor: failed to mark task RUNNING", "task_id", task.TaskID, "err", err)
		return
	}

	taskDir := r.paths.RuntimeWorkspaceDir(task.WorkspaceID, task.TaskID)
	buildmaxDir := r.paths.RuntimeTaskBuildmaxDir(task.WorkspaceID, task.TaskID)
	wsDir := r.paths.RuntimeTaskWSDir(task.WorkspaceID, task.TaskID)

	if err := os.MkdirAll(buildmaxDir, 0755); err != nil {
		r.failTask(ctx, task.TaskID, fmt.Sprintf("failed to create buildmax dir: %v", err))
		return
	}
	if err := os.MkdirAll(wsDir, 0755); err != nil {
		r.failTask(ctx, task.TaskID, fmt.Sprintf("failed to create ws dir: %v", err))
		return
	}
	if err := r.persist.MaterializeToDir(ctx, task.WorkspaceID, wsDir); err != nil {
		r.failTask(ctx, task.TaskID, fmt.Sprintf("failed to materialize workspace: %v", err))
		return
	}

	// Use absolute path for BUILDMAX_HOME so the child process writes sessions/logs under
	// the task's buildmax dir regardless of its CWD (wsDir). If buildmaxDir is relative
	// (e.g. when BUILDMAX_WORKSPACES_DIR is relative), a relative BUILDMAX_HOME would
	// be resolved against CWD and would point under ws/, producing wrong paths.
	buildmaxDirAbs, err := filepath.Abs(buildmaxDir)
	if err != nil {
		r.failTask(ctx, task.TaskID, fmt.Sprintf("failed to resolve buildmax dir: %v", err))
		return
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
	cmd.Env = append(filtered, prefix+buildmaxDirAbs)

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
		if updateErr := r.tasks.UpdateTaskStatus(ctx, task.TaskID, "FAILED", nil, &endTime, &outputStr, &errMsg, nil); updateErr != nil {
			slog.Error("executor: failed to update task status", "task_id", task.TaskID, "err", updateErr)
		}
		return
	}

	// Artifact creation (best-effort): increment seq, write result to artifact storage, insert artifact + item and set last_artifact_id.
	newSeq, seqErr := r.tasks.IncrementTaskSeq(ctx, task.TaskID)
	if seqErr != nil {
		slog.Warn("executor: IncrementTaskSeq failed, skipping artifact", "task_id", task.TaskID, "err", seqErr)
	} else {
		artifactID := util.NewID()
		if putErr := r.artifactStorage.PutResult(ctx, task.WorkspaceID, task.TaskID, artifactID, output); putErr != nil {
			slog.Warn("executor: failed to write result to artifact storage, skipping DB insert", "task_id", task.TaskID, "err", putErr)
		} else if createErr := r.artifacts.CreateArtifactWithItem(ctx, task.TaskID, artifactID, newSeq, resultFilename); createErr != nil {
			slog.Warn("executor: CreateArtifactWithItem failed", "task_id", task.TaskID, "err", createErr)
		}
	}

	slog.Info("executor: task succeeded", "task_id", task.TaskID)
	if updateErr := r.tasks.UpdateTaskStatus(ctx, task.TaskID, "SUCCEEDED", nil, &endTime, &outputStr, nil, nil); updateErr != nil {
		slog.Error("executor: failed to update task status", "task_id", task.TaskID, "err", updateErr)
	}
}

// failTask is a helper to mark a task as FAILED when execution cannot proceed.
func (r *Runner) failTask(ctx context.Context, taskID, errMsg string) {
	slog.Warn("executor: task failed", "task_id", taskID, "err", errMsg)
	endTime := time.Now().Unix()
	if err := r.tasks.UpdateTaskStatus(ctx, taskID, "FAILED", nil, &endTime, nil, &errMsg, nil); err != nil {
		slog.Error("executor: failed to update task status", "task_id", taskID, "err", err)
	}
}
