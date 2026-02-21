package servercmd

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"

	"buildmax/internal/config"
	"buildmax/internal/executor"
	"buildmax/internal/llm"
	"buildmax/internal/server"
	"buildmax/internal/session"
	"buildmax/internal/storage/blob"
	"buildmax/internal/storage/entity"
	"buildmax/internal/storage/setup"
)

// taskTitleGenAdapter implements server.TaskTitleGenerator using session.GenerateTitleFromInput and an LLM client.
type taskTitleGenAdapter struct {
	client *llm.Client
}

func (a *taskTitleGenAdapter) GenerateTaskTitle(ctx context.Context, input string) (string, error) {
	titleClient := session.TitleChatFunc(func(ctx context.Context, msgs []llm.Message) (string, error) {
		content, _, err := a.client.ChatWithTools(ctx, msgs, nil)
		return content, err
	})
	return session.GenerateTitleFromInput(ctx, titleClient, input)
}

// RunServer loads server env and workspaces dir, opens the DB, builds blob storage,
// creates and starts the task executor, then runs the HTTP server until shutdown.
// The port argument should already be resolved (e.g. via config.ResolveServerPort).
func RunServer(ctx context.Context, port int) error {
	serverEnv, err := config.LoadServerEnv()
	if err != nil {
		return err
	}
	workspacesDir := config.WorkspacesDir()
	if workspacesDir == "" {
		return fmt.Errorf("%s is required for server mode", config.EnvKeyBuildmaxWorkspacesDir)
	}
	if abs, err := filepath.Abs(workspacesDir); err == nil {
		workspacesDir = abs
	}
	dsn := config.MySQLDSN()
	st, err := entity.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	if err := st.BackfillTaskWorkspaceID(ctx); err != nil {
		slog.Warn("backfill task workspace_id", "err", err)
	}

	wsCfg := config.LoadWorkspaceStorageConfig()
	var s3Client blob.S3Client
	if wsCfg.PersistProvider == config.ProviderMinIO || wsCfg.ArtifactProvider == config.ProviderMinIO {
		var s3Err error
		s3Client, s3Err = setup.BuildS3Client(ctx, wsCfg)
		if s3Err != nil {
			return fmt.Errorf("S3 client: %w", s3Err)
		}
	}
	persistStorage, err := setup.BuildPersistStorage(wsCfg, config.PersistentWorkspaceDir, s3Client)
	if err != nil {
		return fmt.Errorf("persist storage: %w", err)
	}
	artifactStorage, err := setup.BuildArtifactStorage(wsCfg, config.ArtifactDir, s3Client)
	if err != nil {
		return fmt.Errorf("artifact storage: %w", err)
	}

	cfg := server.Config{
		Addr:             ":" + strconv.Itoa(port),
		UserStore:        st,
		WorkspaceStore:   st,
		ProjectStore:     st,
		AgentStore:       st,
		TaskStore:        st,
		TaskRunStore:     st,
		ArtifactStore:    st,
		PersistStorage:   persistStorage,
		ArtifactStorage:  artifactStorage,
		WorkspacesDir:    workspacesDir,
		JWTSecret:        serverEnv.JWTSecret,
		CORSOrigin:       serverEnv.CORSOrigin,
		WorkerToken:      config.WorkerToken(),
	}
	if llmCfg := config.LoadLLM(); llmCfg.APIKey != "" {
		cfg.TaskTitleGenerator = &taskTitleGenAdapter{client: llm.NewClient(llmCfg)}
	}
	var runner executor.WorkerRunner
	switch config.WorkerRunMode() {
	case "k8s_job":
		jobClient, err := executor.BuildK8sJobCreator()
		if err != nil {
			return fmt.Errorf("k8s job creator: %w", err)
		}
		runner = executor.NewK8sJobRunner(
			config.WorkerJobNamespace(),
			config.WorkerImage(),
			executor.WorkerEnvFromEnviron(),
			jobClient,
		)
	default:
		workerPath := config.WorkerBinaryPath()
		if workerPath == "" {
			return fmt.Errorf("%s is required for local_process mode", config.EnvKeyBuildmaxWorkerBinary)
		}
		runner = executor.NewLocalRunner(workerPath)
	}
	scheduler, err := executor.NewScheduler(st, runner)
	if err != nil {
		return fmt.Errorf("executor: %w", err)
	}
	scheduler.Start()
	defer scheduler.Stop()

	s := server.New(cfg)
	slog.Info("server starting", "addr", cfg.Addr)
	err = s.Run()
	slog.Info("server stopped")
	if err != nil {
		return err
	}
	return nil
}
