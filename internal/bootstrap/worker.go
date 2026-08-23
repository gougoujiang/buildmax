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
	modelName := llm.Model
	if modelName == "" {
		// The gateway resolves an empty name to the deployment default. Naming
		// it here would be this worker choosing a model.
		slog.Info("using the deployment's default model")
	}
	// The entry says which model, and ManagedInference says where: a run's
	// transport is a property of the run, not of the entry.
	return config.ModelEntry{
			Model:         modelName,
			Name:          managedModelDisplayName(modelName),
			ContextWindow: llm.ContextWindow,
			CallTimeout:   llm.CallTimeout,
		}, taskrun.ManagedInference{
			ServerURL: serverURL,
			RunToken:  runToken,
		}, nil
}

// managedModelDisplayName is what the run's trace records as its model. The
// catalog name is the only model identity a run has — the upstream model stays
// on the server — so a trace that showed anything else would name something the
// run never chose.
func managedModelDisplayName(name string) string {
	if name == "" {
		return "deployment default"
	}
	return name
}

// RunWorker reads server.yaml for connection and storage config, fetches the task run
// from the server, marks it RUNNING, executes the agent, and uploads artifacts.
func RunWorker(ctx context.Context, taskRunID string) error {
	if taskRunID == "" {
		return fmt.Errorf("task-run-id is required")
	}

	// A worker process executes exactly one run, so its identity belongs on the
	// default logger rather than on each call. This reaches the packages a
	// worker drives -- the agent loop, the LLM client, the tools -- none of
	// which could be handed a run id any other way.
	slog.SetDefault(slog.Default().With("component", "worker", "task_run_id", taskRunID))

	sc, err := config.LoadServerConfig()
	if err != nil {
		return fmt.Errorf("server config: %w", err)
	}

	serverURL := sc.Worker.ServerURL
	if serverURL == "" {
		slog.Error("server_url not set in server.yaml")
		return fmt.Errorf("worker.server_url is required in server.yaml")
	}
	workspacesDir := sc.WorkspacesDir
	if workspacesDir == "" {
		slog.Error("workspaces_dir not set in server.yaml")
		return fmt.Errorf("workspaces_dir is required in server.yaml")
	}
	if abs, err := filepath.Abs(workspacesDir); err == nil {
		workspacesDir = abs
	}

	// Read now and cleared from the environment immediately. A worker executes
	// model-chosen shell commands, and the sandbox that would strip
	// secret-shaped variables from a child is off by default — so the only safe
	// place for a credential is this process's memory.
	//
	// It is the only credential this worker has for its own routes, and it names
	// the one run this process is executing. A run dispatched without one cannot
	// report anything, so it fails here rather than at its first callback — see
	// docs/design/worker-run-token.md.
	runToken := takeEnv(config.EnvKeyBuildmaxRunToken)
	if runToken == "" {
		slog.Error("no run token", "env", config.EnvKeyBuildmaxRunToken)
		return fmt.Errorf("this run was dispatched without %s", config.EnvKeyBuildmaxRunToken)
	}

	fetched, err := workerclient.GetWorkerTaskRun(ctx, workerclient.WorkerAPIClientConfig{BaseURL: serverURL, Token: runToken}, taskRunID)
	if err != nil {
		slog.Error("get run failed", "err", err)
		return fmt.Errorf("get run: %w", err)
	}
	if fetched == nil {
		slog.Error("run not found")
		return fmt.Errorf("run not found")
	}
	run, task := fetched.Run, fetched.Task
	runtimeModel, managed, err := resolveRunModel(sc, fetched.LLM, serverURL, runToken)
	if err != nil {
		slog.Error("cannot resolve a model for this run", "err", err)
		return err
	}
	if run.Status != string(model.RunStatusScheduled) {
		slog.Error("run not in SCHEDULED status", "status", run.Status)
		return fmt.Errorf("run not scheduled (status=%s)", run.Status)
	}
	apiCfg := workerclient.WorkerAPIClientConfig{BaseURL: serverURL, Token: runToken}
	updater := &workerclient.WorkerHTTPUpdater{BaseURL: serverURL, Token: runToken}
	// A cancel that landed between dispatch and now: the run is over before it
	// starts, and nothing below needs to be built for it.
	if fetched.CancelRequested {
		return reportCanceledBeforeStart(ctx, updater, taskRunID)
	}

	sessionID := session.NewID()
	if task.SessionID != nil {
		sessionID = *task.SessionID
	}
	now := time.Now().UTC()
	if err := updater.UpdateRunStatus(ctx, run.ID, &workerclient.PatchTaskRunRequest{
		Status:    string(model.RunStatusRunning),
		StartedAt: &now,
		SessionID: &sessionID,
	}); err != nil {
		if errors.Is(err, workerclient.ErrTaskRunAlreadyClaimed) {
			slog.Info("run already claimed by another worker")
			return ErrAlreadyClaimed
		}
		slog.Error("failed to mark run RUNNING", "err", err)
		return fmt.Errorf("mark RUNNING: %w", err)
	}

	wsCfg := toWorkspaceStorageConfig(sc.Storage)
	var s3Client blob.S3Client
	if wsCfg.PersistProvider == config.ProviderMinIO || wsCfg.ArtifactProvider == config.ProviderMinIO {
		var s3Err error
		s3Client, s3Err = BuildS3Client(ctx, wsCfg)
		if s3Err != nil {
			slog.Error("failed to build S3 client", "err", s3Err)
			return fmt.Errorf("S3 client: %w", s3Err)
		}
	}
	persistRoot := func(teamID string) string {
		return config.PersistentWorkspaceDir(workspacesDir, teamID)
	}
	persistStorage, err := BuildPersistStorage(wsCfg, persistRoot, s3Client)
	if err != nil {
		slog.Error("failed to build persist storage", "err", err)
		return fmt.Errorf("persist storage: %w", err)
	}
	artifactRoot := func(userID, conversationID, taskID, taskRunID string) string {
		return filepath.Join(workspacesDir, userID, "artifacts", conversationID, taskID, taskRunID)
	}
	runOutputStorage, err := BuildRunOutputStorage(wsCfg, artifactRoot, s3Client)
	if err != nil {
		slog.Error("failed to build artifact storage", "err", err)
		return fmt.Errorf("artifact storage: %w", err)
	}

	paths := taskrun.NewRuntimePathsFromRoot(workspacesDir)
	httpSender := &workerclient.WorkerHTTPStreamSender{BaseURL: serverURL, Token: runToken}
	streamSender := &workerclient.DebouncedStreamSender{Inner: httpSender}

	// The run's own context, so both ways it can be stopped reach the agent loop
	// with a cause attached. Cancelling by cause rather than plainly is what
	// lets RunTask tell "someone stopped this" from "this process is going
	// away", which arrive as the same dead context but are not the same
	// outcome.
	//
	// It deliberately does not inherit ctx's cancellation. A shutdown would
	// otherwise reach the run as a plain cancel first and win the race to set
	// the cause, leaving the run unable to say why it stopped.
	runCtx, cancelRun := context.WithCancelCause(context.WithoutCancel(ctx))
	defer cancelRun(nil)
	go workerclient.WatchCancel(runCtx, apiCfg, taskRunID, 0, func() { cancelRun(model.ErrRunCanceled) })
	interruptRunOnShutdown(ctx, runCtx, cancelRun)

	err = taskrun.RunTask(runCtx, taskrun.RunTaskInput{
		Task:                   task,
		Run:                    run,
		SessionID:              sessionID,
		Paths:                  paths,
		Persist:                persistStorage,
		RunOutputStorage:       runOutputStorage,
		Updater:                updater,
		StreamSender:           streamSender,
		Model:                  runtimeModel,
		Managed:                managed,
		WorkerAPI:              apiCfg,
		AdditionalSystemPrompt: fetched.AgentInstructions,
	})
	if errors.Is(err, model.ErrRunCanceled) {
		slog.Info("run canceled on request")
		return err
	}
	if errors.Is(err, model.ErrRunInterrupted) {
		slog.Info("run interrupted by shutdown")
		return err
	}
	if err != nil {
		slog.Error("run execution failed", "err", err)
		return err
	}
	return nil
}

// interruptRunOnShutdown ends the run with ErrRunInterrupted when the process
// is asked to stop, and stops watching once the run is over.
//
// The signal has to be turned into a cause the run can read, which is why the
// process context is watched rather than inherited: an inherited cancellation
// would reach the run first as a plain one, and a context's cause is set once —
// leaving the run unable to say why it stopped.
func interruptRunOnShutdown(processCtx, runCtx context.Context, cancelRun context.CancelCauseFunc) {
	go func() {
		select {
		case <-processCtx.Done():
			slog.Info("worker asked to stop; interrupting the run")
			cancelRun(model.ErrRunInterrupted)
		case <-runCtx.Done():
		}
	}()
}

// reportCanceledBeforeStart finishes a run that was canceled between dispatch
// and start. Nothing ran, so there is no output and no artifact to register —
// only a record that says why the run stopped instead of what it produced.
func reportCanceledBeforeStart(ctx context.Context, updater taskrun.TaskRunUpdater, taskRunID string) error {
	slog.Info("this run was canceled before it started")
	endedAt := time.Now().UTC()
	message := "this run was canceled before it started"
	if err := updater.UpdateRunStatus(ctx, taskRunID, &workerclient.PatchTaskRunRequest{
		Status:       string(model.RunStatusCanceled),
		EndedAt:      &endedAt,
		ErrorMessage: &message,
	}); err != nil {
		slog.Error("could not report a cancel", "err", err)
		return err
	}
	return model.ErrRunCanceled
}
