package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/infra/db"
	"github.com/gougoujiang/buildmax/internal/infra/k8s"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	httpserver "github.com/gougoujiang/buildmax/internal/server"
	"github.com/gougoujiang/buildmax/internal/server/scheduler"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
	"github.com/gougoujiang/buildmax/internal/service/quota"
)

const taskTitlePrompt = `Generate a short task title (3-5 words) from this user request. Return ONLY the title, no quotes or punctuation.`

// titleGenAdapter implements llm.TitleGenerator using an LLM client. It holds
// the core interface, not a provider client, so the model router decides which
// implementation generates titles.
type titleGenAdapter struct {
	client cllm.LLMClient
}

func (a *titleGenAdapter) GenerateTitle(ctx context.Context, input string) (string, int, int, error) {
	if input == "" {
		return "", 0, 0, nil
	}
	msgs := []cllm.Message{
		{Role: "system", Content: taskTitlePrompt},
		{Role: "user", Content: input},
	}
	content, _, usage, err := a.client.ChatCompletionBlocking(ctx, msgs, nil)
	if err != nil {
		return "", 0, 0, err
	}
	return cleanTaskTitle(content), usage.PromptTokens, usage.CompletionTokens, nil
}

func cleanTaskTitle(s string) string {
	s = strings.TrimSpace(s)
	for _, q := range []string{`"`, `'`, "`"} {
		if len(s) >= 2 && strings.HasPrefix(s, q) && strings.HasSuffix(s, q) {
			s = s[len(q) : len(s)-len(q)]
		}
	}
	return strings.TrimSpace(s)
}

// RunServer loads server.yaml, resolves the listen port (flag overrides config),
// opens the DB, builds blob storage, starts the scheduler, and runs the HTTP server.
// portOverride > 0 takes priority over the port in server.yaml.
func RunServer(ctx context.Context, portOverride int) error {
	sc, err := config.LoadServerConfig()
	if err != nil {
		return fmt.Errorf("server config: %w", err)
	}

	// JWT secret: server.yaml jwt_secret field, overridable by BUILDMAX_JWT_SECRET env var.
	jwtSecret := sc.JWTSecret
	if jwtSecret == "" {
		return fmt.Errorf("jwt_secret is required (set in server.yaml or %s env var)", config.EnvKeyBuildmaxJWTSecret)
	}

	slog.Info("login accepts a password or a single-use code",
		"set_password_with", "buildmax-server user set-password <email>",
		"issue_code_with", "buildmax-server user login-code <email>")
	// Self-registration and a reachable server together mean anyone can create
	// an account. Say so at startup rather than in a document nobody opened.
	if sc.AllowSignup {
		slog.Warn("open signup is enabled — anyone who can reach this server can create an account",
			"config", "allow_signup")
	}

	port := sc.Port
	if portOverride > 0 {
		port = portOverride
	}
	if port == 0 {
		port = 5678
	}

	workspacesDir, err := resolveWorkspacesDir(sc.WorkspacesDir)
	if err != nil {
		return err
	}

	store, err := openStore(ctx, sc.Database)
	if err != nil {
		return err
	}

	persistStorage, artifactStorage, err := buildBlobStorage(ctx, sc.Storage, workspacesDir)
	if err != nil {
		return err
	}

	serverConfig, err := buildHTTPServerConfig(port, jwtSecret, sc, workspacesDir, store, persistStorage, artifactStorage)
	if err != nil {
		return err
	}

	runner, err := buildWorkerRunner(sc.Worker)
	if err != nil {
		return err
	}

	sched, err := scheduler.NewScheduler(store, runner)
	if err != nil {
		return fmt.Errorf("scheduler: %w", err)
	}
	sched.Start()
	defer sched.Stop()

	cleaner := scheduler.NewCredentialCleaner(store, 0)
	cleaner.Start()
	defer cleaner.Stop()

	s := httpserver.New(serverConfig)
	slog.Info("server starting",
		"addr", serverConfig.Addr,
		"version", config.Version,
		"commit", config.Commit,
	)
	err = s.Run()
	slog.Info("server stopped")
	return err
}

func resolveWorkspacesDir(fromConfig string) (string, error) {
	if fromConfig == "" {
		return "", fmt.Errorf("workspaces_dir is required in server.yaml")
	}
	abs, err := filepath.Abs(fromConfig)
	if err != nil {
		return fromConfig, nil
	}
	return abs, nil
}

func openStore(ctx context.Context, db_ config.ServerDBConfig) (*db.Store, error) {
	st, err := db.New(ctx, db_.DSN())
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	return st, nil
}

func buildBlobStorage(ctx context.Context, sc config.ServerStorageConfig, workspacesDir string) (blob.PersistStorage, blob.ArtifactStorage, error) {
	wsCfg := toWorkspaceStorageConfig(sc)
	s3Client, err := buildOptionalS3Client(ctx, wsCfg)
	if err != nil {
		return nil, nil, err
	}
	persistRoot := func(teamID string) string {
		return config.PersistentWorkspaceDir(workspacesDir, teamID)
	}
	persistStorage, err := BuildPersistStorage(wsCfg, persistRoot, s3Client)
	if err != nil {
		return nil, nil, fmt.Errorf("persist storage: %w", err)
	}
	artifactRoot := func(userID, conversationID, taskID, taskRunID string) string {
		return filepath.Join(workspacesDir, userID, "artifacts", conversationID, taskID, taskRunID)
	}
	artifactStorage, err := BuildArtifactStorage(wsCfg, artifactRoot, s3Client)
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

func buildHTTPServerConfig(port int, jwtSecret string, sc config.ServerConfig, workspacesDir string, st *db.Store, persistStorage blob.PersistStorage, artifactStorage blob.ArtifactStorage) (httpserver.Config, error) {
	quotaService := &quota.QuotaService{
		TeamStore:   st,
		UsageReader: st,
		TierStore:   st,
		DefaultTier: sc.DefaultQuotaTier,
	}
	cfg := httpserver.Config{
		Addr: fmt.Sprintf(":%d", port),
		Auth: httpserver.AuthConfig{
			JWTSecret:            jwtSecret,
			AllowSignup:          sc.AllowSignup,
			CORSOrigin:           sc.CORSOrigin,
			QuotaService:         quotaService,
			DefaultQuotaTier:     sc.DefaultQuotaTier,
			AccessTokenTTL:       sc.AccessTokenTTL,
			RefreshTokenTTL:      sc.RefreshTokenTTL,
			RefreshRotationGrace: sc.RefreshRotationGrace,
		},
		Stores: httpserver.StoresConfig{
			UserStore:           st,
			LoginCodeStore:      st,
			PasswordStore:       st,
			RefreshTokenStore:   st,
			TeamStore:           st,
			WorkflowStore:       st,
			AgentStore:          st,
			IssueStore:          st,
			IssueCommentStore:   st,
			TaskStore:           st,
			TaskRunStore:        st,
			RunOutputLister:     st,
			UserWebhookKeyStore: st,
			AuditStore:          st,
		},
		Storage: httpserver.StorageConfig{
			PersistStorage:  persistStorage,
			ArtifactStorage: artifactStorage,
			WorkspacesDir:   workspacesDir,
		},
		Worker: httpserver.WorkerConfig{
			WorkerToken: sc.Worker.Token,
		},
		Conv: httpserver.ConversationConfig{
			ConversationStore:        st,
			ConversationMessageStore: st,
		},
		Webhook: httpserver.WebhookConfig{
			MessagePath: sc.Webhook.MessagePath,
			UserID:      sc.Webhook.UserID,
		},
		Audit:     audit.NewRecorder(st),
		Readiness: readinessChecks(st, persistStorage),
	}
	if err := wireLLM(&cfg, sc, st, quotaService); err != nil {
		return httpserver.Config{}, err
	}
	return cfg, nil
}

// wireLLM builds the model router, routes Tier 1 conversation through it in
// process, and exposes the managed gateway to authenticated clients.
//
// The server resolves a catalog target it owns for its own inference: it does
// not call its own HTTP listener and is not subject to team model policy.
// readinessProbeTeam is a team id no team can have, so the storage probe reads
// nothing real. It exercises the configured backend — reachability,
// credentials, and bucket or directory access — without depending on any
// tenant's data existing.
const readinessProbeTeam = "_readiness_probe"

// readinessChecks are the dependencies the server cannot serve traffic without.
//
// Names are what an unauthenticated caller sees, so they say which dependency
// without saying where it lives.
func readinessChecks(st *db.Store, persist blob.PersistStorage) []httpserver.ReadinessCheck {
	return []httpserver.ReadinessCheck{
		{
			Name:  "database",
			Probe: st.Ping,
		},
		{
			Name: "object_storage",
			Probe: func(ctx context.Context) error {
				_, err := persist.ListFiles(ctx, readinessProbeTeam)
				return err
			},
		},
	}
}

func wireLLM(cfg *httpserver.Config, sc config.ServerConfig, st *db.Store, quotaService *quota.QuotaService) error {
	// A nil *db.Store put straight into an interface parameter is a non-nil
	// interface holding a nil pointer, so the absence of a store has to be
	// stated rather than passed along.
	var models model.LLMModelStore
	if st != nil {
		models = st
	}

	routing, err := buildLLMRouting(sc, models)
	if err != nil {
		return err
	}
	if routing == nil {
		return nil
	}

	// The gateway needs a ledger. Without a store there is nowhere to account
	// managed calls, so it stays off rather than serving unmetered inference.
	if st != nil {
		cfg.Conv.LLMGateway = &llmgateway.Service{
			Router: routing.Router,
			Ledger: st,
			Quota:  quotaService,
		}
	}

	if routing.Tier1TargetID == "" {
		return nil
	}
	routed, err := routing.Router.ClientForTarget(context.Background(), routing.Tier1TargetID, llmgateway.BaselineCapabilities())
	if err != nil {
		return fmt.Errorf("conversation model %q: %w", routing.Tier1TargetID, err)
	}
	cfg.Conv.TitleGenerator = &titleGenAdapter{client: routed.Client}
	cfg.Conv.ConversationLLMClient = routed.Client
	return nil
}

func buildWorkerRunner(wc config.ServerWorkerConfig) (scheduler.WorkerRunner, error) {
	switch wc.RunMode {
	case "k8s_job":
		jobClient, err := k8s.BuildK8sJobCreator()
		if err != nil {
			return nil, fmt.Errorf("k8s job creator: %w", err)
		}
		// Worker pods read the same server.yaml the server does: the ConfigMap
		// supplies the file, the inherited BUILDMAX_* environment supplies the
		// credentials that must not be written into it.
		return k8s.NewK8sJobRunner(
			wc.K8s.Namespace,
			wc.K8s.Image,
			k8s.WorkerEnvFromEnviron(),
			k8s.PodConfig{
				ConfigMapName: wc.K8s.ConfigMap,
				HomeDir:       wc.K8s.HomeDir,
				RunAsUser:     wc.K8s.RunAsUser,
				Resources: k8s.PodResources{
					CPURequest:    wc.K8s.Resources.CPURequest,
					CPULimit:      wc.K8s.Resources.CPULimit,
					MemoryRequest: wc.K8s.Resources.MemoryRequest,
					MemoryLimit:   wc.K8s.Resources.MemoryLimit,
				},
			},
			jobClient,
		), nil
	default:
		if wc.Binary == "" {
			return nil, fmt.Errorf("worker.binary is required in server.yaml for local_process mode")
		}
		return scheduler.NewLocalRunner(wc.Binary, config.FilterWorkerEnv(os.Environ())), nil
	}
}

// toWorkspaceStorageConfig converts ServerStorageConfig to the shared WorkspaceStorageConfig
// used by the objectstore builders.
func toWorkspaceStorageConfig(sc config.ServerStorageConfig) config.WorkspaceStorageConfig {
	return config.WorkspaceStorageConfig{
		PersistProvider:  sc.PersistBackend,
		ArtifactProvider: sc.ArtifactBackend,
		Endpoint:         sc.MinIO.Endpoint,
		Region:           sc.MinIO.Region,
		AccessKey:        sc.MinIO.AccessKey,
		SecretKey:        sc.MinIO.SecretKey,
		Bucket:           sc.MinIO.Bucket,
		Prefix:           sc.MinIO.Prefix,
		PathStyle:        sc.MinIO.PathStyle,
	}
}
