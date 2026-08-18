package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
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

// takeEnv reads a variable and removes it, so nothing this process spawns can
// find it.
func takeEnv(key string) string {
	value := os.Getenv(key)
	_ = os.Unsetenv(key)
	return value
}

// resolveRunModel decides how this run reaches a model.
//
// The server states the transport; the worker supplies only the credential it
// was given. A managed run therefore gets an entry with an alias and no API key,
// and a direct run gets the server's own model exactly as before.
//
// A managed run with no token fails here rather than at its first prompt. The
// two transports never substitute for one another: a run that cannot
// authenticate to the gateway must not quietly fall back to a provider key that
// happens to be lying around.
func resolveRunModel(sc config.ServerConfig, llm *workerclient.TaskRunLLM, serverURL, runToken string) (config.ModelEntry, taskrun.ManagedInference, error) {
	if llm == nil || llm.Transport != config.TransportBuildMax {
		return sc.Conversation.Model.RuntimeModelEntry(), taskrun.ManagedInference{}, nil
	}
	if runToken == "" {
		return config.ModelEntry{}, taskrun.ManagedInference{},
			fmt.Errorf("this run uses managed inference but was given no %s", config.EnvKeyBuildmaxRunToken)
	}
	alias := llm.Alias
	if alias == "" {
		// The gateway resolves an empty alias to the team default. Naming it
		// here would be this worker choosing a model.
		slog.Info("worker: using the team's default model alias")
	}
	return config.ModelEntry{
			Model:         alias,
			Name:          managedModelDisplayName(alias),
			Transport:     config.TransportBuildMax,
			ServerURL:     serverURL,
			ContextWindow: llm.ContextWindow,
			CallTimeout:   llm.CallTimeout,
		}, taskrun.ManagedInference{
			ServerURL: serverURL,
			RunToken:  runToken,
		}, nil
}

// managedModelDisplayName is what the run's trace records as its model. The
// alias is the only model identity a run has — the upstream model stays on the
// server — so a trace that showed anything else would name something the run
// never chose.
func managedModelDisplayName(alias string) string {
	if alias == "" {
		return "team default"
	}
	return alias
}

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
	workspacesDir := sc.WorkspacesDir
	if workspacesDir == "" {
		slog.Error("worker: workspaces_dir not set in server.yaml", "task_run_id", taskRunID)
		return fmt.Errorf("workspaces_dir is required in server.yaml")
	}
	if abs, err := filepath.Abs(workspacesDir); err == nil {
		workspacesDir = abs
	}

	// Both credentials are read now and cleared from the environment immediately.
	// A worker executes model-chosen shell commands, and the sandbox that would
	// strip secret-shaped variables from a child is off by default — so the only
	// safe place for a credential is this process's memory.
	runToken := takeEnv(config.EnvKeyBuildmaxRunToken)
	_ = os.Unsetenv(config.EnvKeyBuildmaxWorkerToken)

	// The run token is what this worker presents on its own routes: it names the
	// one run this process is executing. worker.token is the fallback for the
	// upgrade window where an older server dispatched this run without minting
	// one — see docs/design/worker-run-token.md.
	token := runToken
	if token == "" {
		token = sc.Worker.Token
		if token == "" {
			slog.Error("worker: no run token and no worker.token", "task_run_id", taskRunID)
			return fmt.Errorf("this run was dispatched without %s, and worker.token is not set either",
				config.EnvKeyBuildmaxRunToken)
		}
		slog.Warn("worker: falling back to the deployment-wide worker token",
			"task_run_id", taskRunID, "why", "this run was dispatched without a run token")
	}

	fetched, err := workerclient.GetWorkerTaskRun(ctx, workerclient.WorkerAPIClientConfig{BaseURL: serverURL, Token: token}, taskRunID)
	if err != nil {
		slog.Error("worker: get run failed", "task_run_id", taskRunID, "err", err)
		return fmt.Errorf("get run: %w", err)
	}
	if fetched == nil {
		slog.Error("worker: run not found", "task_run_id", taskRunID)
		return fmt.Errorf("run not found")
	}
	run, task := fetched.Run, fetched.Task
	runtimeModel, managed, err := resolveRunModel(sc, fetched.LLM, serverURL, runToken)
	if err != nil {
		slog.Error("worker: cannot resolve a model for this run", "task_run_id", taskRunID, "err", err)
		return err
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
		Model:           runtimeModel,
		Managed:         managed,
	})
	if err != nil {
		slog.Error("worker: run execution failed", "task_run_id", taskRunID, "err", err)
		return err
	}
	return nil
}
