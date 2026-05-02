package bootstrap

import (
	"buildmax/internal/core/model"
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"

	"buildmax/internal/config"
	"buildmax/internal/core/quota"
	"buildmax/internal/execution/scheduler"
	"buildmax/internal/infra/db"
	"buildmax/internal/infra/k8s"
	llm "buildmax/internal/infra/llm"
	blob "buildmax/internal/infra/objectstore"
	httpserver "buildmax/internal/server"
	"buildmax/internal/session"
)

type serverBootstrap struct {
	workspacesDir   string
	store           *db.Store
	persistStorage  blob.PersistStorage
	artifactStorage blob.ArtifactStorage
	serverConfig    httpserver.Config
	runner          scheduler.WorkerRunner
}

// titleGenAdapter implements httpserver.TitleGenerator using session.GenerateTitleFromInput and an LLM client.
type titleGenAdapter struct {
	client *llm.Client
}

func (a *titleGenAdapter) GenerateTitle(ctx context.Context, input string) (string, httpserver.TokenUsage, error) {
	titleClient := session.TitleChatFunc(func(ctx context.Context, msgs []model.Message) (string, model.Usage, error) {
		content, _, usage, err := a.client.ChatWithTools(ctx, msgs, nil)
		return content, usage, err
	})
	title, usage, err := session.GenerateTitleFromInput(ctx, titleClient, input)
	if err != nil {
		return "", httpserver.TokenUsage{}, err
	}
	return title, httpserver.TokenUsage{PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens}, nil
}

// RunServer loads server env and workspaces dir, opens the DB, builds blob storage,
// creates and starts the task-run scheduler, then runs the HTTP server until shutdown.
// The port argument should already be resolved (e.g. via config.ResolveServerPort).
func RunServer(ctx context.Context, port int) error {
	bootstrap, err := buildServerBootstrap(ctx, port)
	if err != nil {
		return err
	}
	sched, err := scheduler.NewScheduler(bootstrap.store, bootstrap.runner)
	if err != nil {
		return fmt.Errorf("scheduler: %w", err)
	}
	sched.Start()
	defer sched.Stop()

	s := httpserver.New(bootstrap.serverConfig)
	slog.Info("server starting", "addr", bootstrap.serverConfig.Addr)
	err = s.Run()
	slog.Info("server stopped")
	if err != nil {
		return err
	}
	return nil
}

func buildServerBootstrap(ctx context.Context, port int) (*serverBootstrap, error) {
	serverEnv, err := config.LoadServerEnv()
	if err != nil {
		return nil, err
	}
	workspacesDir, err := resolveWorkspacesDir()
	if err != nil {
		return nil, err
	}
	store, err := openStore(ctx)
	if err != nil {
		return nil, err
	}
	persistStorage, artifactStorage, err := buildBlobStorage(ctx)
	if err != nil {
		return nil, err
	}
	serverConfig := buildHTTPServerConfig(port, serverEnv, workspacesDir, store, persistStorage, artifactStorage)
	runner, err := buildWorkerRunner()
	if err != nil {
		return nil, err
	}
	return &serverBootstrap{
		workspacesDir:   workspacesDir,
		store:           store,
		persistStorage:  persistStorage,
		artifactStorage: artifactStorage,
		serverConfig:    serverConfig,
		runner:          runner,
	}, nil
}

func resolveWorkspacesDir() (string, error) {
	workspacesDir := config.WorkspacesDir()
	if workspacesDir == "" {
		return "", fmt.Errorf("%s is required for server mode", config.EnvKeyBuildmaxWorkspacesDir)
	}
	if abs, err := filepath.Abs(workspacesDir); err == nil {
		workspacesDir = abs
	}
	return workspacesDir, nil
}

func openStore(ctx context.Context) (*db.Store, error) {
	st, err := db.New(ctx, config.MySQLDSN())
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	return st, nil
}

func buildBlobStorage(ctx context.Context) (blob.PersistStorage, blob.ArtifactStorage, error) {
	wsCfg := config.LoadWorkspaceStorageConfig()
	s3Client, err := buildOptionalS3Client(ctx, wsCfg)
	if err != nil {
		return nil, nil, err
	}
	persistStorage, err := BuildPersistStorage(wsCfg, config.PersistentWorkspaceDir, s3Client)
	if err != nil {
		return nil, nil, fmt.Errorf("persist storage: %w", err)
	}
	artifactStorage, err := BuildArtifactStorage(wsCfg, func(userID, conversationID, taskID, taskRunID string) string {
		return filepath.Join(config.WorkspacesDir(), userID, "artifacts", conversationID, taskID, taskRunID)
	}, s3Client)
	if err != nil {
		return nil, nil, fmt.Errorf("artifact storage: %w", err)
	}
	return persistStorage, artifactStorage, nil
}

func buildOptionalS3Client(ctx context.Context, wsCfg config.WorkspaceStorageConfig) (blob.S3Client, error) {
	if wsCfg.PersistProvider != config.ProviderMinIO && wsCfg.ArtifactProvider != config.ProviderMinIO {
		return nil, nil
	}
	s3Client, err := BuildS3Client(ctx, wsCfg)
	if err != nil {
		return nil, fmt.Errorf("S3 client: %w", err)
	}
	return s3Client, nil
}

func buildHTTPServerConfig(port int, serverEnv config.ServerEnv, workspacesDir string, st *db.Store, persistStorage blob.PersistStorage, artifactStorage blob.ArtifactStorage) httpserver.Config {
	defaultQuotaTier := config.DefaultQuotaTier()
	quotaChecker := quota.NewChecker(st, st, st, defaultQuotaTier)
	cfg := httpserver.Config{
		Addr: ":" + strconv.Itoa(port),
		Auth: httpserver.AuthConfig{
			JWTSecret:        serverEnv.JWTSecret,
			CORSOrigin:       serverEnv.CORSOrigin,
			QuotaChecker:     quotaChecker,
			DefaultQuotaTier: defaultQuotaTier,
		},
		Stores: httpserver.StoresConfig{
			UserStore:           st,
			TeamStore:           st,
			WorkflowStore:       st,
			AgentStore:          st,
			IssueStore:          st,
			TaskStore:           st,
			TaskRunStore:        st,
			RunOutputLister:     st,
			UserWebhookKeyStore: st,
		},
		Storage: httpserver.StorageConfig{
			PersistStorage:  persistStorage,
			ArtifactStorage: artifactStorage,
			WorkspacesDir:   workspacesDir,
		},
		Worker: httpserver.WorkerConfig{
			WorkerToken: config.WorkerToken(),
		},
		Conv: httpserver.ConversationConfig{
			ConversationStore:        st,
			ConversationMessageStore: st,
		},
		Webhook: httpserver.WebhookConfig{
			MessagePath: config.WebhookMessagePath(),
			UserID:      config.WebhookUserID(),
		},
	}
	wireOptionalLLM(&cfg)
	return cfg
}

func wireOptionalLLM(cfg *httpserver.Config) {
	llmCfg := config.LoadLLM()
	if llmCfg.APIKey == "" {
		return
	}
	llmClient := llm.NewClient(llm.Config{APIKey: llmCfg.APIKey, BaseURL: llmCfg.BaseURL, Model: llmCfg.Model})
	cfg.Conv.TitleGenerator = &titleGenAdapter{client: llmClient}
	cfg.Conv.ConversationLLMCaller = llmClient
}

func buildWorkerRunner() (scheduler.WorkerRunner, error) {
	switch config.WorkerRunMode() {
	case "k8s_job":
		jobClient, err := k8s.BuildK8sJobCreator()
		if err != nil {
			return nil, fmt.Errorf("k8s job creator: %w", err)
		}
		return k8s.NewK8sJobRunner(
			config.WorkerJobNamespace(),
			config.WorkerImage(),
			k8s.WorkerEnvFromEnviron(),
			jobClient,
		), nil
	default:
		workerPath := config.WorkerBinaryPath()
		if workerPath == "" {
			return nil, fmt.Errorf("%s is required for local_process mode", config.EnvKeyBuildmaxWorkerBinary)
		}
		return scheduler.NewLocalRunner(workerPath), nil
	}
}
