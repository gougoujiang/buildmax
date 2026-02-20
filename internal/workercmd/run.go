package workercmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"buildmax/internal/config"
	"buildmax/internal/executor"
	"buildmax/internal/storage/blob"

	"github.com/google/uuid"
)

// ErrAlreadyClaimed is returned by RunWorker when the run was already claimed by another worker (server returned 409).
var ErrAlreadyClaimed = errors.New("task run already claimed by another worker")

// RunWorker validates worker env, fetches run+task from the server, marks run RUNNING,
// builds blob storage, then runs the run (materialize, optionally restore session, buildmax -p, update status).
// runID must be non-empty; from --task-run-id.
func RunWorker(ctx context.Context, runID string) error {
	if runID == "" {
		slog.Error("worker: task-run-id is required")
		return fmt.Errorf("task-run-id is required")
	}
	baseURL := config.WorkerServerURL()
	token := config.WorkerToken()
	if baseURL == "" {
		slog.Error("worker: server URL not set", "run_id", runID, "env", config.EnvKeyBuildmaxServerURL)
		return fmt.Errorf("%s is required", config.EnvKeyBuildmaxServerURL)
	}
	if token == "" {
		slog.Error("worker: worker token not set", "run_id", runID, "env", config.EnvKeyBuildmaxWorkerToken)
		return fmt.Errorf("%s is required", config.EnvKeyBuildmaxWorkerToken)
	}
	workspacesDir := config.WorkspacesDir()
	if workspacesDir == "" {
		slog.Error("worker: workspaces dir not set", "run_id", runID, "env", config.EnvKeyBuildmaxWorkspacesDir)
		return fmt.Errorf("%s is required for worker", config.EnvKeyBuildmaxWorkspacesDir)
	}
	if abs, err := filepath.Abs(workspacesDir); err == nil {
		workspacesDir = abs
	}

	run, task, err := executor.GetWorkerTaskRun(ctx, baseURL, token, runID, nil)
	if err != nil {
		slog.Error("worker: get run failed", "run_id", runID, "err", err)
		return fmt.Errorf("get run: %w", err)
	}
	if run == nil || task == nil {
		slog.Error("worker: run not found", "run_id", runID)
		return fmt.Errorf("run not found")
	}
	if run.Status != "SCHEDULED" {
		slog.Error("worker: run not in SCHEDULED status", "run_id", runID, "status", run.Status)
		return fmt.Errorf("run not scheduled (status=%s)", run.Status)
	}

	sessionID := uuid.New().String()
	if task.SessionID != nil {
		sessionID = *task.SessionID
	}
	updater := &executor.WorkerHTTPUpdater{BaseURL: baseURL, Token: token}
	now := time.Now().Unix()
	if err := updater.UpdateRunStatus(ctx, run.RunID, "RUNNING", &now, nil, nil, nil, &sessionID, nil); err != nil {
		if errors.Is(err, executor.ErrTaskAlreadyClaimed) {
			slog.Info("run already claimed by another worker", "run_id", runID)
			return ErrAlreadyClaimed
		}
		slog.Error("worker: failed to mark run RUNNING", "run_id", runID, "err", err)
		return fmt.Errorf("mark RUNNING: %w", err)
	}

	wsCfg := config.LoadWorkspaceStorageConfig()
	var s3Client blob.S3Client
	if wsCfg.PersistProvider == config.ProviderMinIO || wsCfg.ArtifactProvider == config.ProviderMinIO {
		var s3Err error
		s3Client, s3Err = config.BuildS3Client(ctx, wsCfg)
		if s3Err != nil {
			slog.Error("worker: failed to build S3 client", "run_id", runID, "err", s3Err)
			return fmt.Errorf("S3 client: %w", s3Err)
		}
	}
	persistStorage, err := config.BuildPersistStorage(wsCfg, config.PersistentWorkspaceDir, s3Client)
	if err != nil {
		slog.Error("worker: failed to build persist storage", "run_id", runID, "err", err)
		return fmt.Errorf("persist storage: %w", err)
	}
	artifactStorage, err := config.BuildArtifactStorage(wsCfg, config.ArtifactDir, s3Client)
	if err != nil {
		slog.Error("worker: failed to build artifact storage", "run_id", runID, "err", err)
		return fmt.Errorf("artifact storage: %w", err)
	}

	paths := executor.NewWorkspacePathsFromRoot(workspacesDir)
	err = executor.RunTask(ctx, task, run, sessionID, paths, persistStorage, artifactStorage, updater)
	if err != nil {
		slog.Error("worker: run execution failed", "run_id", runID, "err", err)
		return err
	}
	return nil
}
