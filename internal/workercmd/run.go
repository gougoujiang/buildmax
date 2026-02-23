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
	"buildmax/internal/storage/setup"
	"buildmax/internal/workerapi"

	"github.com/google/uuid"
)

// ErrAlreadyClaimed is returned by RunWorker when the run was already claimed by another worker (server returned 409).
var ErrAlreadyClaimed = errors.New("chat run already claimed by another worker")

// RunWorker validates worker env, fetches run+chat from the server, marks run RUNNING,
// builds blob storage, then runs the run (materialize, optionally restore session, buildmax -p, update status).
// chatRunID must be non-empty; from --chat-run-id.
func RunWorker(ctx context.Context, chatRunID string) error {
	if chatRunID == "" {
		slog.Error("worker: chat-run-id is required")
		return fmt.Errorf("chat-run-id is required")
	}
	baseURL := config.WorkerServerURL()
	token := config.WorkerToken()
	if baseURL == "" {
		slog.Error("worker: server URL not set", "chat_run_id", chatRunID, "env", config.EnvKeyBuildmaxServerURL)
		return fmt.Errorf("%s is required", config.EnvKeyBuildmaxServerURL)
	}
	if token == "" {
		slog.Error("worker: worker token not set", "chat_run_id", chatRunID, "env", config.EnvKeyBuildmaxWorkerToken)
		return fmt.Errorf("%s is required", config.EnvKeyBuildmaxWorkerToken)
	}
	workspacesDir := config.WorkspacesDir()
	if workspacesDir == "" {
		slog.Error("worker: workspaces dir not set", "chat_run_id", chatRunID, "env", config.EnvKeyBuildmaxWorkspacesDir)
		return fmt.Errorf("%s is required for worker", config.EnvKeyBuildmaxWorkspacesDir)
	}
	if abs, err := filepath.Abs(workspacesDir); err == nil {
		workspacesDir = abs
	}

	run, chat, err := executor.GetWorkerChatRun(ctx, baseURL, token, chatRunID, nil)
	if err != nil {
		slog.Error("worker: get run failed", "chat_run_id", chatRunID, "err", err)
		return fmt.Errorf("get run: %w", err)
	}
	if run == nil || chat == nil {
		slog.Error("worker: run not found", "chat_run_id", chatRunID)
		return fmt.Errorf("run not found")
	}
	if run.Status != workerapi.StatusScheduled {
		slog.Error("worker: run not in SCHEDULED status", "chat_run_id", chatRunID, "status", run.Status)
		return fmt.Errorf("run not scheduled (status=%s)", run.Status)
	}

	sessionID := uuid.New().String()
	if chat.SessionID != nil {
		sessionID = *chat.SessionID
	}
	updater := &executor.WorkerHTTPUpdater{BaseURL: baseURL, Token: token}
	now := time.Now().Unix()
	if err := updater.UpdateRunStatus(ctx, run.ChatRunID, workerapi.StatusRunning, &now, nil, nil, nil, &sessionID, nil); err != nil {
		if errors.Is(err, executor.ErrChatRunAlreadyClaimed) {
			slog.Info("run already claimed by another worker", "chat_run_id", chatRunID)
			return ErrAlreadyClaimed
		}
		slog.Error("worker: failed to mark run RUNNING", "chat_run_id", chatRunID, "err", err)
		return fmt.Errorf("mark RUNNING: %w", err)
	}

	wsCfg := config.LoadWorkspaceStorageConfig()
	var s3Client blob.S3Client
	if wsCfg.PersistProvider == config.ProviderMinIO || wsCfg.ArtifactProvider == config.ProviderMinIO {
		var s3Err error
		s3Client, s3Err = setup.BuildS3Client(ctx, wsCfg)
		if s3Err != nil {
			slog.Error("worker: failed to build S3 client", "chat_run_id", chatRunID, "err", s3Err)
			return fmt.Errorf("S3 client: %w", s3Err)
		}
	}
	persistStorage, err := setup.BuildPersistStorage(wsCfg, config.PersistentWorkspaceDir, s3Client)
	if err != nil {
		slog.Error("worker: failed to build persist storage", "chat_run_id", chatRunID, "err", err)
		return fmt.Errorf("persist storage: %w", err)
	}
	artifactStorage, err := setup.BuildArtifactStorage(wsCfg, config.RunOutputDir, s3Client)
	if err != nil {
		slog.Error("worker: failed to build artifact storage", "chat_run_id", chatRunID, "err", err)
		return fmt.Errorf("artifact storage: %w", err)
	}

	paths := executor.NewWorkspacePathsFromRoot(workspacesDir)
	httpSender := &executor.WorkerHTTPStreamSender{BaseURL: baseURL, Token: token}
	streamSender := &executor.DebouncedStreamSender{Inner: httpSender}
	err = executor.RunTask(ctx, chat, run, sessionID, paths, persistStorage, artifactStorage, updater, streamSender)
	if err != nil {
		slog.Error("worker: run execution failed", "chat_run_id", chatRunID, "err", err)
		return err
	}
	return nil
}
