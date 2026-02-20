// Package executor provides task run scheduling and execution.
//
//   - Scheduler: polls for PENDING task runs, spawns worker with --task-run-id.
//   - RunTask: runs a single run (materialize workspace, optionally restore session, run buildmax -p, update run via TaskRunUpdater).
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

// ArtifactPayload is passed to TaskRunUpdater when registering an artifact on success.
type ArtifactPayload struct {
	ArtifactID   string
	RelativePath string
}

// TaskRunUpdater is used by the worker to update run status and register artifacts via HTTP.
// When status is SUCCEEDED with artifact, server creates artifact and syncs task denormalized fields.
// When status is FAILED, server syncs task denormalized from run.
type TaskRunUpdater interface {
	UpdateRunStatus(ctx context.Context, runID, status string, startedAt, endedAt *int64, output, errMsg, sessionID *string, artifact *ArtifactPayload) error
}

// RunTask runs a single task run: materialize workspace, optionally restore session from previous run, run buildmax -p, upload buildmax, update run and task via updater.
func RunTask(ctx context.Context, task *entity.Task, run *entity.TaskRun, sessionID string, paths WorkspacePaths, persist blob.PersistStorage, artifactStorage blob.ArtifactStorage, updater TaskRunUpdater) error {
	if task == nil || run == nil {
		return errors.New("executor: task and run must not be nil")
	}
	if paths == nil || persist == nil || artifactStorage == nil || updater == nil {
		return errors.New("executor: paths, persist, artifactStorage and updater must not be nil")
	}

	buildmaxDir := paths.RuntimeTaskRunBuildmaxDir(task.WorkspaceID, task.TaskID, run.RunID)
	wsDir := paths.RuntimeTaskWSDir(task.WorkspaceID, task.TaskID)

	if err := os.MkdirAll(buildmaxDir, 0755); err != nil {
		slog.Error("executor: failed to create buildmax dir", "run_id", run.RunID, "path", buildmaxDir, "err", err)
		_ = updater.UpdateRunStatus(ctx, run.RunID, "FAILED", nil, ptrInt64(time.Now().Unix()), nil, ptrString(fmt.Sprintf("failed to create buildmax dir: %v", err)), nil, nil)
		return err
	}
	if err := os.MkdirAll(wsDir, 0755); err != nil {
		slog.Error("executor: failed to create ws dir", "run_id", run.RunID, "path", wsDir, "err", err)
		_ = updater.UpdateRunStatus(ctx, run.RunID, "FAILED", nil, ptrInt64(time.Now().Unix()), nil, ptrString(fmt.Sprintf("failed to create ws dir: %v", err)), nil, nil)
		return err
	}
	// Restore session from previous run's buildmax if this is a follow-up run.
	if task.SessionID != nil && task.LastRunID != nil && *task.LastRunID != run.RunID {
		relPath := "sessions/" + *task.SessionID + ".json"
		data, err := persist.GetTaskBuildmax(ctx, task.WorkspaceID, task.TaskID, *task.LastRunID, relPath)
		if err == nil {
			sessionsDir := filepath.Join(buildmaxDir, "sessions")
			if err := os.MkdirAll(sessionsDir, 0755); err == nil {
				_ = os.WriteFile(filepath.Join(sessionsDir, *task.SessionID+".json"), data, 0644)
			}
		}
	}
	if err := persist.MaterializeToDir(ctx, task.WorkspaceID, wsDir); err != nil {
		slog.Error("executor: failed to materialize workspace", "run_id", run.RunID, "workspace_id", task.WorkspaceID, "err", err)
		_ = updater.UpdateRunStatus(ctx, run.RunID, "FAILED", nil, ptrInt64(time.Now().Unix()), nil, ptrString(fmt.Sprintf("failed to materialize workspace: %v", err)), nil, nil)
		return err
	}

	effectiveSessionID := sessionID
	if task.SessionID != nil {
		effectiveSessionID = *task.SessionID
	}
	env := os.Environ()
	prefix := config.EnvKeyBuildmaxHome + "="
	filtered := make([]string, 0, len(env)+1)
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			filtered = append(filtered, e)
		}
	}
	cmd := exec.CommandContext(ctx, "buildmax", "-p", run.Input, "--session-id", effectiveSessionID)
	cmd.Dir = wsDir
	cmd.Env = append(filtered, prefix+buildmaxDir)

	output, err := cmd.CombinedOutput()
	endTime := time.Now().Unix()
	outputStr := string(output)

	resultFilename := fmt.Sprintf("result-%s.md", run.RunID)
	runDir := filepath.Dir(buildmaxDir)
	resultPath := filepath.Join(runDir, resultFilename)
	if writeErr := os.WriteFile(resultPath, output, 0644); writeErr != nil {
		slog.Error("executor: failed to write result file", "run_id", run.RunID, "path", resultPath, "err", writeErr)
	}

	uploadTaskBuildmax(ctx, buildmaxDir, task.WorkspaceID, task.TaskID, run.RunID, persist)

	if err != nil {
		errMsg := fmt.Sprintf("buildmax exited with error: %v", err)
		slog.Error("executor: run failed", "run_id", run.RunID, "err", err, "output_len", len(outputStr))
		if updateErr := updater.UpdateRunStatus(ctx, run.RunID, "FAILED", nil, &endTime, &outputStr, &errMsg, nil, nil); updateErr != nil {
			slog.Error("executor: failed to update run status to FAILED", "run_id", run.RunID, "err", updateErr)
		}
		return err
	}

	artifactID := util.NewULID()
	if putErr := artifactStorage.PutResult(ctx, task.WorkspaceID, task.TaskID, run.RunID, artifactID, output); putErr != nil {
		slog.Error("executor: failed to write result to artifact storage", "run_id", run.RunID, "err", putErr)
	}
	if updateErr := updater.UpdateRunStatus(ctx, run.RunID, "SUCCEEDED", nil, &endTime, &outputStr, nil, nil, &ArtifactPayload{ArtifactID: artifactID, RelativePath: resultFilename}); updateErr != nil {
		slog.Error("executor: failed to update run status to SUCCEEDED", "run_id", run.RunID, "err", updateErr)
		return updateErr
	}

	slog.Info("executor: run succeeded", "run_id", run.RunID)
	return nil
}

// uploadTaskBuildmax uploads buildmax dir files (logs, sessions, settings) to persist storage for the run.
func uploadTaskBuildmax(ctx context.Context, buildmaxDir, workspaceID, taskID, runID string, persist blob.PersistStorage) {
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
			slog.Warn("executor: upload task buildmax open failed", "run_id", runID, "rel_path", relPath, "err", err)
			continue
		}
		putErr := persist.PutTaskBuildmax(ctx, workspaceID, taskID, runID, filepath.ToSlash(relPath), f)
		f.Close()
		if putErr != nil {
			slog.Warn("executor: upload task buildmax put failed", "run_id", runID, "rel_path", relPath, "err", putErr)
		}
	}
}

func ptrString(s string) *string { return &s }
func ptrInt64(n int64) *int64    { return &n }
