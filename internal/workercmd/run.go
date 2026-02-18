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

// ErrAlreadyClaimed is returned by RunWorker when the task was already claimed by another worker (server returned 409).
// The main program should exit with code 2 when this is returned.
var ErrAlreadyClaimed = errors.New("task already claimed by another worker")

// RunWorker validates worker env, fetches the task from the server, marks it RUNNING,
// builds blob storage, then runs the task (materialize, buildmax -p, update status).
// taskID must be non-empty; typically from --task-id.
func RunWorker(ctx context.Context, taskID string) error {
	if taskID == "" {
		return fmt.Errorf("task-id is required")
	}
	baseURL := config.WorkerServerURL()
	token := config.WorkerToken()
	if baseURL == "" {
		return fmt.Errorf("%s is required", config.EnvKeyBuildmaxServerURL)
	}
	if token == "" {
		return fmt.Errorf("%s is required", config.EnvKeyBuildmaxWorkerToken)
	}
	workspacesDir := config.WorkspacesDir()
	if workspacesDir == "" {
		return fmt.Errorf("%s is required for worker", config.EnvKeyBuildmaxWorkspacesDir)
	}
	if abs, err := filepath.Abs(workspacesDir); err == nil {
		workspacesDir = abs
	}

	task, err := executor.GetWorkerTask(ctx, baseURL, token, taskID, nil)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found")
	}
	if task.Status != "SCHEDULED" {
		return fmt.Errorf("task not scheduled (status=%s)", task.Status)
	}

	sessionID := uuid.New().String()
	updater := &executor.WorkerHTTPUpdater{BaseURL: baseURL, Token: token}
	now := time.Now().Unix()
	if err := updater.UpdateTaskStatus(ctx, taskID, "RUNNING", &now, nil, nil, nil, &sessionID, nil); err != nil {
		if errors.Is(err, executor.ErrTaskAlreadyClaimed) {
			slog.Info("task already claimed by another worker", "task_id", taskID)
			return ErrAlreadyClaimed
		}
		return fmt.Errorf("mark RUNNING: %w", err)
	}

	wsCfg := config.LoadWorkspaceStorageConfig()
	var s3Client blob.S3Client
	if wsCfg.PersistProvider == config.ProviderMinIO || wsCfg.ArtifactProvider == config.ProviderMinIO {
		var s3Err error
		s3Client, s3Err = config.BuildS3Client(ctx, wsCfg)
		if s3Err != nil {
			return fmt.Errorf("S3 client: %w", s3Err)
		}
	}
	persistStorage, err := config.BuildPersistStorage(wsCfg, config.PersistentWorkspaceDir, s3Client)
	if err != nil {
		return fmt.Errorf("persist storage: %w", err)
	}
	artifactStorage, err := config.BuildArtifactStorage(wsCfg, config.ArtifactDir, s3Client)
	if err != nil {
		return fmt.Errorf("artifact storage: %w", err)
	}

	paths := executor.NewWorkspacePathsFromRoot(workspacesDir)
	return executor.RunTask(ctx, task, sessionID, paths, persistStorage, artifactStorage, updater)
}
