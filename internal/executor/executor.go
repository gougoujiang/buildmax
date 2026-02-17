// Package executor runs pending tasks by spawning the buildmax CLI agent.
package executor

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"buildmax/internal/config"
	"buildmax/internal/id"
	"buildmax/internal/store"

	"github.com/google/uuid"
)

const defaultPollInterval = 5 * time.Second

// TaskStore is the subset of store.TaskStore that the executor needs (including artifact creation).
type TaskStore interface {
	GetNextPendingTask(ctx context.Context) (*store.Task, error)
	UpdateTaskStatus(ctx context.Context, taskID, status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) error
	IncrementTaskSeq(ctx context.Context, taskID string) (newSeq int, err error)
	CreateArtifactWithItem(ctx context.Context, taskID, artifactID string, seq int, relativePath string) error
}

// Runner polls for pending tasks and executes them one at a time.
type Runner struct {
	store        TaskStore
	pollInterval time.Duration
	stopCh       chan struct{}
	doneCh       chan struct{}
}

// New creates a Runner. Call Start() to begin polling.
func New(store TaskStore) *Runner {
	return &Runner{
		store:        store,
		pollInterval: defaultPollInterval,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
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

	persistDir := config.PersistentWorkspaceDir(task.WorkspaceID)
	runtimeDir := config.RuntimeWorkspaceDir(task.WorkspaceID, task.TaskID)

	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		r.failTask(ctx, task.TaskID, fmt.Sprintf("failed to create runtime dir: %v", err))
		return
	}
	if err := copyWorkspaceContents(persistDir, runtimeDir); err != nil {
		r.failTask(ctx, task.TaskID, fmt.Sprintf("failed to copy workspace contents: %v", err))
		return
	}

	cmd := exec.CommandContext(ctx, "buildmax", "-p", task.Input, "--session-id", sessionID)
	cmd.Dir = runtimeDir

	output, err := cmd.CombinedOutput()
	endTime := time.Now().Unix()
	outputStr := string(output)

	resultFilename := fmt.Sprintf("result-%s.md", task.TaskID)
	resultPath := filepath.Join(runtimeDir, resultFilename)
	if writeErr := os.WriteFile(resultPath, output, 0644); writeErr != nil {
		slog.Warn("executor: failed to write result file", "task_id", task.TaskID, "err", writeErr)
	}

	// Copy result file to persistent workspace so user sees it in file tree.
	if copyErr := copyResultToPersist(runtimeDir, persistDir, resultFilename); copyErr != nil {
		slog.Warn("executor: failed to copy result to persistent workspace", "task_id", task.TaskID, "err", copyErr)
	}

	if err != nil {
		errMsg := fmt.Sprintf("buildmax exited with error: %v", err)
		slog.Warn("executor: task failed", "task_id", task.TaskID, "err", err)
		if updateErr := r.store.UpdateTaskStatus(ctx, task.TaskID, "FAILED", nil, &endTime, &outputStr, &errMsg, nil); updateErr != nil {
			slog.Error("executor: failed to update task status", "task_id", task.TaskID, "err", updateErr)
		}
		return
	}

	// Artifact creation (best-effort): increment seq, write result to artifact dir, insert artifact + item and set last_artifact_id.
	newSeq, seqErr := r.store.IncrementTaskSeq(ctx, task.TaskID)
	if seqErr != nil {
		slog.Warn("executor: IncrementTaskSeq failed, skipping artifact", "task_id", task.TaskID, "err", seqErr)
	} else {
		artifactID := id.New()
		artifactDir := config.ArtifactDir(task.WorkspaceID, task.TaskID, artifactID)
		if mkdirErr := os.MkdirAll(artifactDir, 0755); mkdirErr != nil {
			slog.Warn("executor: failed to create artifact dir, skipping artifact", "task_id", task.TaskID, "err", mkdirErr)
		} else {
			resultSrc := filepath.Join(runtimeDir, resultFilename)
			resultDst := filepath.Join(artifactDir, "result.md")
			if copyErr := copyFile(resultSrc, resultDst); copyErr != nil {
				slog.Warn("executor: failed to copy result to artifact dir, skipping DB insert", "task_id", task.TaskID, "err", copyErr)
			} else if createErr := r.store.CreateArtifactWithItem(ctx, task.TaskID, artifactID, newSeq, resultFilename); createErr != nil {
				slog.Warn("executor: CreateArtifactWithItem failed", "task_id", task.TaskID, "err", createErr)
			}
		}
	}

	slog.Info("executor: task succeeded", "task_id", task.TaskID)
	if updateErr := r.store.UpdateTaskStatus(ctx, task.TaskID, "SUCCEEDED", nil, &endTime, &outputStr, nil, nil); updateErr != nil {
		slog.Error("executor: failed to update task status", "task_id", task.TaskID, "err", updateErr)
	}
}

// copyWorkspaceContents copies files and directories from src to dst recursively.
// If src is missing or not a directory, returns nil (no-op). Copies only regular files and dirs; skips symlinks.
func copyWorkspaceContents(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		destPath := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(path, destPath)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// copyResultToPersist copies the result file from runtimeDir to persistDir.
// Ensures persistDir exists. Best-effort; logs on failure.
func copyResultToPersist(runtimeDir, persistDir, resultFilename string) error {
	src := filepath.Join(runtimeDir, resultFilename)
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(persistDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(persistDir, resultFilename), data, 0644)
}

// failTask is a helper to mark a task as FAILED when execution cannot proceed.
func (r *Runner) failTask(ctx context.Context, taskID, errMsg string) {
	slog.Warn("executor: task failed", "task_id", taskID, "err", errMsg)
	endTime := time.Now().Unix()
	if err := r.store.UpdateTaskStatus(ctx, taskID, "FAILED", nil, &endTime, nil, &errMsg, nil); err != nil {
		slog.Error("executor: failed to update task status", "task_id", taskID, "err", err)
	}
}
