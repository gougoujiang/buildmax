package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp/taskrun"
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/core/session"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
)

// ErrAlreadyClaimed is returned by RunWorker when the run was already claimed by another worker.
var ErrAlreadyClaimed = errors.New("task run already claimed by another worker")

// RunWorker reads server.yaml for connection and storage config, fetches the task run
// from the server, marks it RUNNING, executes the agent, and uploads artifacts.
func RunWorker(ctx context.Context, taskRunID string) error {
	if taskRunID == "" {
		return fmt.Errorf("task-run-id is required")
	}

	sc, err := config.LoadServerConfig()
	if err != nil {
		return fmt.Errorf("server config: %w", err)
	}

	serverURL := sc.Worker.ServerURL
	if serverURL == "" {
		slog.Error("worker: server_url not set in server.yaml", "task_run_id", taskRunID)
		return fmt.Errorf("worker.server_url is required in server.yaml")
	}
	token := sc.Worker.Token
	if token == "" {
		slog.Error("worker: worker token not set in server.yaml", "task_run_id", taskRunID)
		return fmt.Errorf("worker.token is required in server.yaml")
	}
	workspacesDir := sc.WorkspacesDir
	if workspacesDir == "" {
		slog.Error("worker: workspaces_dir not set in server.yaml", "task_run_id", taskRunID)
		return fmt.Errorf("workspaces_dir is required in server.yaml")
	}
	if abs, err := filepath.Abs(workspacesDir); err == nil {
		workspacesDir = abs
	}

	run, task, err := workerclient.GetWorkerTaskRun(ctx, workerclient.WorkerAPIClientConfig{BaseURL: serverURL, Token: token}, taskRunID)
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

	sessionID := session.NewID()
	if task.SessionID != nil {
		sessionID = *task.SessionID
	}
	updater := &workerclient.WorkerHTTPUpdater{BaseURL: serverURL, Token: token}
	now := time.Now().Unix()
	if err := updater.UpdateRunStatus(ctx, run.TaskRunID, &workerclient.PatchTaskRunRequest{
		Status:    string(model.RunStatusRunning),
		StartedAt: &now,
		SessionID: &sessionID,
	}); err != nil {
		if errors.Is(err, workerclient.ErrTaskRunAlreadyClaimed) {
			slog.Info("run already claimed by another worker", "task_run_id", taskRunID)
			return ErrAlreadyClaimed
		}
		slog.Error("worker: failed to mark run RUNNING", "task_run_id", taskRunID, "err", err)
		return fmt.Errorf("mark RUNNING: %w", err)
	}

	wsCfg := toWorkspaceStorageConfig(sc.Storage)
	var s3Client blob.S3Client
	if wsCfg.PersistProvider == config.ProviderMinIO || wsCfg.ArtifactProvider == config.ProviderMinIO {
		var s3Err error
		s3Client, s3Err = BuildS3Client(ctx, wsCfg)
		if s3Err != nil {
			slog.Error("worker: failed to build S3 client", "task_run_id", taskRunID, "err", s3Err)
			return fmt.Errorf("S3 client: %w", s3Err)
		}
	}
	persistRoot := func(teamID string) string {
		return config.PersistentWorkspaceDir(workspacesDir, teamID)
	}
	persistStorage, err := BuildPersistStorage(wsCfg, persistRoot, s3Client)
	if err != nil {
		slog.Error("worker: failed to build persist storage", "task_run_id", taskRunID, "err", err)
		return fmt.Errorf("persist storage: %w", err)
	}
	artifactRoot := func(userID, conversationID, taskID, taskRunID string) string {
		return filepath.Join(workspacesDir, userID, "artifacts", conversationID, taskID, taskRunID)
	}
	artifactStorage, err := BuildArtifactStorage(wsCfg, artifactRoot, s3Client)
	if err != nil {
		slog.Error("worker: failed to build artifact storage", "task_run_id", taskRunID, "err", err)
		return fmt.Errorf("artifact storage: %w", err)
	}

	paths := taskrun.NewRuntimePathsFromRoot(workspacesDir)
	httpSender := &workerclient.WorkerHTTPStreamSender{BaseURL: serverURL, Token: token}
	streamSender := &workerclient.DebouncedStreamSender{Inner: httpSender}
	err = taskrun.RunTask(ctx, taskrun.RunTaskInput{
		Task:            task,
		Run:             run,
		SessionID:       sessionID,
		Paths:           paths,
		Persist:         persistStorage,
		ArtifactStorage: artifactStorage,
		Updater:         updater,
		StreamSender:    streamSender,
		Model:           sc.Conversation.Model.RuntimeModelEntry(),
	})
	if err != nil {
		slog.Error("worker: run execution failed", "task_run_id", taskRunID, "err", err)
		return err
	}
	return nil
}
