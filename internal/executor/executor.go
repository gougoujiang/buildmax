// Package executor provides task scheduling and task execution.
//
//   - Scheduler (scheduler.go): polls for PENDING tasks and spawns the buildmax-worker binary.
//   - Executor: RunTask runs a single task (materialize workspace, run buildmax -p, update status via TaskUpdater).
//     Used by the worker binary; the scheduler does not call RunTask.
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

// ArtifactPayload is passed to TaskUpdater when registering an artifact on success.
type ArtifactPayload struct {
	ArtifactID   string
	RelativePath string
}

// TaskUpdater is used by the worker to update task status and register artifacts via HTTP (or other backend).
type TaskUpdater interface {
	// UpdateTaskStatus updates task status and optional fields. For SUCCEEDED with artifact, pass non-nil artifact.
	UpdateTaskStatus(ctx context.Context, taskID, status string, startedAt, endedAt *int64, output, errMsg, sessionID *string, artifact *ArtifactPayload) error
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
		slog.Error("executor: failed to create buildmax dir", "task_id", task.TaskID, "path", buildmaxDir, "err", err)
		_ = updater.UpdateTaskStatus(ctx, task.TaskID, "FAILED", nil, ptrInt64(time.Now().Unix()), nil, ptrString(fmt.Sprintf("failed to create buildmax dir: %v", err)), nil, nil)
		return err
	}
	if err := os.MkdirAll(wsDir, 0755); err != nil {
		slog.Error("executor: failed to create ws dir", "task_id", task.TaskID, "path", wsDir, "err", err)
		_ = updater.UpdateTaskStatus(ctx, task.TaskID, "FAILED", nil, ptrInt64(time.Now().Unix()), nil, ptrString(fmt.Sprintf("failed to create ws dir: %v", err)), nil, nil)
		return err
	}
	if err := persist.MaterializeToDir(ctx, task.WorkspaceID, wsDir); err != nil {
		slog.Error("executor: failed to materialize workspace", "task_id", task.TaskID, "workspace_id", task.WorkspaceID, "err", err)
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
		slog.Error("executor: failed to write result file", "task_id", task.TaskID, "path", resultPath, "err", writeErr)
	}

	uploadTaskBuildmax(ctx, buildmaxDir, task.WorkspaceID, task.TaskID, persist)

	if err != nil {
		errMsg := fmt.Sprintf("buildmax exited with error: %v", err)
		slog.Error("executor: task failed", "task_id", task.TaskID, "err", err, "output_len", len(outputStr))
		if updateErr := updater.UpdateTaskStatus(ctx, task.TaskID, "FAILED", nil, &endTime, &outputStr, &errMsg, nil, nil); updateErr != nil {
			slog.Error("executor: failed to update task status to FAILED", "task_id", task.TaskID, "err", updateErr)
		}
		return err
	}

	artifactID := util.NewULID()
	if putErr := artifactStorage.PutResult(ctx, task.WorkspaceID, task.TaskID, artifactID, output); putErr != nil {
		slog.Error("executor: failed to write result to artifact storage", "task_id", task.TaskID, "err", putErr)
	}
	if updateErr := updater.UpdateTaskStatus(ctx, task.TaskID, "SUCCEEDED", nil, &endTime, &outputStr, nil, nil, &ArtifactPayload{ArtifactID: artifactID, RelativePath: resultFilename}); updateErr != nil {
		slog.Error("executor: failed to update task status to SUCCEEDED", "task_id", task.TaskID, "err", updateErr)
		return updateErr
	}

	slog.Info("executor: task succeeded", "task_id", task.TaskID)
	return nil
}

// uploadTaskBuildmax uploads buildmax dir files (logs, sessions, settings) to persist storage.
// Best-effort: missing files or PutTaskBuildmax errors are logged and skipped.
func uploadTaskBuildmax(ctx context.Context, buildmaxDir, workspaceID, taskID string, persist blob.PersistStorage) {
	relPaths := []string{"logs/buildmax.log", "logs/buildmax-worker.log", "settings.json"}
	sessionsDir := filepath.Join(buildmaxDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			relPaths = append(relPaths, "sessions/"+e.Name())
		}
	}
	for _, relPath := range relPaths {
		fullPath := filepath.Join(buildmaxDir, filepath.FromSlash(relPath))
		info, err := os.Stat(fullPath)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		f, err := os.Open(fullPath)
		if err != nil {
			slog.Warn("executor: upload task buildmax open failed", "task_id", taskID, "rel_path", relPath, "err", err)
			continue
		}
		putErr := persist.PutTaskBuildmax(ctx, workspaceID, taskID, filepath.ToSlash(relPath), f)
		f.Close()
		if putErr != nil {
			slog.Warn("executor: upload task buildmax put failed", "task_id", taskID, "rel_path", relPath, "err", putErr)
		}
	}
}

func ptrString(s string) *string { return &s }
func ptrInt64(n int64) *int64    { return &n }
