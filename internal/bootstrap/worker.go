package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"buildmax/internal/config"
	"buildmax/internal/core/model"
	execruntime "buildmax/internal/execution/runtime"
	execworker "buildmax/internal/execution/worker"
	blob "buildmax/internal/infra/objectstore"

	"github.com/google/uuid"
)

// ErrAlreadyClaimed is returned by RunWorker when the run was already claimed by another worker (server returned 409).
var ErrAlreadyClaimed = errors.New("task run already claimed by another worker")

// RunWorker validates worker env, fetches run+task from the server, marks run RUNNING,
// builds blob storage, then runs the run (materialize, optionally restore session, buildmax -p, update status).
// taskRunID must be non-empty; from --task-run-id.
func RunWorker(ctx context.Context, taskRunID string) error {
	if taskRunID == "" {
		slog.Error("worker: task-run-id is required")
		return fmt.Errorf("task-run-id is required")
	}
	baseURL := config.WorkerServerURL()
	token := config.WorkerToken()
	if baseURL == "" {
		slog.Error("worker: server URL not set", "task_run_id", taskRunID, "env", config.EnvKeyBuildmaxServerURL)
		return fmt.Errorf("%s is required", config.EnvKeyBuildmaxServerURL)
	}
	if token == "" {
		slog.Error("worker: worker token not set", "task_run_id", taskRunID, "env", config.EnvKeyBuildmaxWorkerToken)
		return fmt.Errorf("%s is required", config.EnvKeyBuildmaxWorkerToken)
	}
	workspacesDir := config.WorkspacesDir()
	if workspacesDir == "" {
		slog.Error("worker: workspaces dir not set", "task_run_id", taskRunID, "env", config.EnvKeyBuildmaxWorkspacesDir)
		return fmt.Errorf("%s is required for worker", config.EnvKeyBuildmaxWorkspacesDir)
	}
	if abs, err := filepath.Abs(workspacesDir); err == nil {
		workspacesDir = abs
	}

	run, task, err := execworker.GetWorkerTaskRun(ctx, execworker.WorkerAPIClientConfig{BaseURL: baseURL, Token: token}, taskRunID)
	if err != nil {
		slog.Error("worker: get run failed", "task_run_id", taskRunID, "err", err)
		return fmt.Errorf("get run: %w", err)
	}
	if run == nil || task == nil {
		slog.Error("worker: run not found", "task_run_id", taskRunID)
		return fmt.Errorf("run not found")
	}
	if run.Status != string(model.RunStatusScheduled) {
		slog.Error("worker: run not in SCHEDULED status", "task_run_id", taskRunID, "status", run.Status)
		return fmt.Errorf("run not scheduled (status=%s)", run.Status)
	}

	sessionID := uuid.New().String()
	if task.SessionID != nil {
		sessionID = *task.SessionID
	}
	updater := &execworker.WorkerHTTPUpdater{BaseURL: baseURL, Token: token}
	now := time.Now().Unix()
	if err := updater.UpdateRunStatus(ctx, run.TaskRunID, &execworker.PatchTaskRunRequest{
		Status:    string(model.RunStatusRunning),
		StartedAt: &now,
		SessionID: &sessionID,
	}); err != nil {
		if errors.Is(err, execworker.ErrTaskRunAlreadyClaimed) {
			slog.Info("run already claimed by another worker", "task_run_id", taskRunID)
			return ErrAlreadyClaimed
		}
		slog.Error("worker: failed to mark run RUNNING", "task_run_id", taskRunID, "err", err)
		return fmt.Errorf("mark RUNNING: %w", err)
	}

	wsCfg := config.LoadWorkspaceStorageConfig()
	var s3Client blob.S3Client
	if wsCfg.PersistProvider == config.ProviderMinIO || wsCfg.ArtifactProvider == config.ProviderMinIO {
		var s3Err error
		s3Client, s3Err = BuildS3Client(ctx, wsCfg)
		if s3Err != nil {
			slog.Error("worker: failed to build S3 client", "task_run_id", taskRunID, "err", s3Err)
			return fmt.Errorf("S3 client: %w", s3Err)
		}
	}
	persistStorage, err := BuildPersistStorage(wsCfg, config.PersistentWorkspaceDir, s3Client)
	if err != nil {
		slog.Error("worker: failed to build persist storage", "task_run_id", taskRunID, "err", err)
		return fmt.Errorf("persist storage: %w", err)
	}
	artifactStorage, err := BuildArtifactStorage(wsCfg, func(userID, conversationID, taskID, taskRunID string) string {
		return filepath.Join(workspacesDir, userID, "artifacts", conversationID, taskID, taskRunID)
	}, s3Client)
	if err != nil {
		slog.Error("worker: failed to build artifact storage", "task_run_id", taskRunID, "err", err)
		return fmt.Errorf("artifact storage: %w", err)
	}

	paths := execruntime.NewRuntimePathsFromRoot(workspacesDir)
	httpSender := &execworker.WorkerHTTPStreamSender{BaseURL: baseURL, Token: token}
	streamSender := &execworker.DebouncedStreamSender{Inner: httpSender}
	err = execruntime.RunTask(ctx, execruntime.RunTaskInput{
		Task:               task,
		Run:                run,
		SessionID:          sessionID,
		Paths:              paths,
		Persist:            persistStorage,
		ArtifactStorage:    artifactStorage,
		Updater:            updater,
		StreamSender:       streamSender,
		BuildmaxHomeEnvKey: config.EnvKeyBuildmaxHome,
	})
	if err != nil {
		slog.Error("worker: run execution failed", "task_run_id", taskRunID, "err", err)
		return err
	}
	return nil
}
