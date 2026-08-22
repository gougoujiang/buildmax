package bootstrap

import (
	"context"
	"fmt"
	"github.com/gougoujiang/buildmax/internal/server/handlers/admin"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/infra/db"
	"github.com/gougoujiang/buildmax/internal/infra/k8s"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
	httpserver "github.com/gougoujiang/buildmax/internal/server"
	"github.com/gougoujiang/buildmax/internal/server/authtoken"
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
	completion, err := a.client.ChatCompletionBlocking(ctx, msgs, nil)
	if err != nil {
		return "", 0, 0, err
	}
	return cleanTaskTitle(completion.Content), completion.Usage.PromptTokens, completion.Usage.CompletionTokens, nil
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

	if err := validateWorkerLLM(sc); err != nil {
		return err
	}

	workspacesDir, err := resolveWorkspacesDir(sc.WorkspacesDir)
	if err != nil {
		return err
	}

	store, err := openStore(ctx, sc.Database)
	if err != nil {
		return err
	}

	storage, err := buildBlobStorage(ctx, sc.Storage, workspacesDir)
	if err != nil {
		return err
	}

	serverConfig, err := buildHTTPServerConfig(port, jwtSecret, sc, workspacesDir, store, storage)
	if err != nil {
		return err
	}

	runner, err := buildWorkerRunner(sc.Worker)
	if err != nil {
		return err
	}

	sched, err := scheduler.NewScheduler(store, runner, runTokenMinter(sc, jwtSecret))
	if err != nil {
		return fmt.Errorf("scheduler: %w", err)
	}
	// So that work queued by an account an administrator has disabled does not
	// start after the disable.
	sched.WithUserStore(store).Start()
	defer sched.Stop()

	cleaner := scheduler.NewCredentialCleaner(store, 0)
	cleaner.Start()
	defer cleaner.Stop()

	reaper := scheduler.NewStaleRunReaper(store, sc.Worker.RunTimeout, 0)
	reaper.Start()
	defer reaper.Stop()

	// Nil unless the operator set a retention window, so a deployment that
	// never chose one keeps every event.
	retainer := scheduler.NewAuditRetainer(store, store, sc.Audit.RetentionDays, 0)
	retainer.Start()
	defer retainer.Stop()

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

// blobStorage is what a server needs from the object store: the team's mutable
// home, the reproducible output a run leaves, and the durable artifacts the
// team keeps. They are three key spaces, not one bucket with three names.
type blobStorage struct {
	Persist   blob.PersistStorage
	RunOutput blob.RunOutputStorage
	Artifacts blob.ArtifactStorage
}

func buildBlobStorage(ctx context.Context, sc config.ServerStorageConfig, workspacesDir string) (blobStorage, error) {
	wsCfg := toWorkspaceStorageConfig(sc)
	s3Client, err := buildOptionalS3Client(ctx, wsCfg)
	if err != nil {
		return blobStorage{}, err
	}
	persistRoot := func(teamID string) string {
		return config.PersistentWorkspaceDir(workspacesDir, teamID)
	}
	persistStorage, err := BuildPersistStorage(wsCfg, persistRoot, s3Client)
	if err != nil {
		return blobStorage{}, fmt.Errorf("persist storage: %w", err)
	}
	runOutputRoot := func(userID, conversationID, taskID, taskRunID string) string {
		return filepath.Join(workspacesDir, userID, "artifacts", conversationID, taskID, taskRunID)
	}
	runOutputStorage, err := BuildRunOutputStorage(wsCfg, runOutputRoot, s3Client)
	if err != nil {
		return blobStorage{}, fmt.Errorf("run output storage: %w", err)
	}
	// Under "teams" so it cannot collide with the run-output tree above, which
	// is keyed by user, or with a team's home directory.
	artifactRoot := func(teamID, artifactID string) string {
		return filepath.Join(workspacesDir, "teams", teamID, "artifacts", artifactID)
	}
	artifactStorage, err := BuildArtifactStorage(wsCfg, artifactRoot, s3Client)
	if err != nil {
		return blobStorage{}, fmt.Errorf("artifact storage: %w", err)
	}
	return blobStorage{Persist: persistStorage, RunOutput: runOutputStorage, Artifacts: artifactStorage}, nil
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

func buildHTTPServerConfig(port int, jwtSecret string, sc config.ServerConfig, workspacesDir string, st *db.Store, storage blobStorage) (httpserver.Config, error) {
	quotaService := &quota.Service{
		TeamStore:   st,
		UsageReader: st,
		TierStore:   st,
		DefaultTier: sc.DefaultQuotaTier,
		// So a team admin can see that the team approached or hit its limits
		// without anyone having to notice a 429 in a log.
		Audit: st,
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
			LLMCallStore:        st,
			RunOutputLister:     st,
			UserWebhookKeyStore: st,
			AuditStore:          st,
			SystemGrantStore:    st,
			SchemaStore:         st,
			LLMModelStore:       st,
			ArtifactStore:       st,
		},
		Storage: httpserver.StorageConfig{
			PersistStorage:   storage.Persist,
			RunOutputStorage: storage.RunOutput,
			ArtifactStorage:  storage.Artifacts,
			MaxArtifactBytes: int64(sc.Storage.MaxArtifactMB) << 20,
			WorkspacesDir:    workspacesDir,
		},
		Worker: httpserver.WorkerConfig{
			WorkerToken: sc.Worker.Token,
			LLM:         workerLLMDescriptor(sc.Worker.LLM),
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
		Readiness: readinessChecks(st, storage.Persist),
		// What the admin system status reports about this deployment, and the
		// operator-facing view of server.yaml. The redaction whitelist lives in
		// internal/config, next to the struct it describes.
		Deployment:     deploymentInfoFor(sc),
		RedactedConfig: sc.Redacted(),
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
// deploymentInfoFor describes a deployment from its configuration.
//
// It lives in bootstrap for the same reason readinessChecks does: the server
// layer does not know what a run mode or a model transport is, and keeping that
// mapping here is what stops configuration detail leaking into it.
//
// SandboxSurface is deliberately left empty. No worker path passes one today —
// internal/agentapp/taskrun leaves AppConfig.SandboxSurface unset — and
// reporting a boundary that is not applied would be worse than reporting none.
func deploymentInfoFor(sc config.ServerConfig) admin.DeploymentInfo {
	transport := sc.Worker.LLM.Transport
	if transport == "" {
		transport = config.TransportDirect
	}
	runMode := sc.Worker.RunMode
	if runMode == "" {
		runMode = "local_process"
	}
	return admin.DeploymentInfo{
		Version:            config.VersionString(),
		ModelAliases:       sc.LLM.Aliases,
		DefaultModelAlias:  sc.LLM.DefaultAlias,
		WorkerRunMode:      runMode,
		WorkerLLMTransport: transport,
		AllowSignup:        sc.AllowSignup,
	}
}

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

func wireLLM(cfg *httpserver.Config, sc config.ServerConfig, st *db.Store, quotaService *quota.Service) error {
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

// runTokenMinter returns the signer the scheduler gives each dispatched run.
//
// Every run gets one, not only a managed one. The token started as a gateway
// credential, but it is now what a worker uses to reach any of its own routes,
// so a direct-mode run needs it to report the work it did.
func runTokenMinter(sc config.ServerConfig, jwtSecret string) scheduler.MintRunToken {
	ttl := sc.Worker.RunTokenTTL
	if sc.Worker.LLM.Managed() {
		slog.Info("task runs use managed inference and hold no provider credential",
			"alias", sc.Worker.LLM.Alias, "run_token_ttl", ttl)
	}
	return func(claims authtoken.RunClaims) (string, error) {
		return authtoken.MintRun(jwtSecret, claims, ttl, time.Now())
	}
}

// workerLLMDescriptor is what a worker is told about models for its run. It
// carries an alias and nothing else — the endpoint, the upstream model, and the
// credential stay on the server.
//
// Nil means direct, so a deployment that has not enabled managed inference sends
// the field at all.
func workerLLMDescriptor(wc config.ServerWorkerLLMConfig) *workerclient.TaskRunLLM {
	if !wc.Managed() {
		return nil
	}
	return &workerclient.TaskRunLLM{
		Transport:     config.TransportBuildMax,
		Alias:         wc.Alias,
		ContextWindow: wc.ContextWindow,
		CallTimeout:   wc.CallTimeout,
	}
}

// validateWorkerLLM rejects a deployment that asks for managed worker inference
// it cannot serve.
//
// Without this, `worker.llm.transport: buildmax` with no `llm.aliases` parses
// cleanly and then fails every run at its first model call, which reads as a
// model outage rather than a configuration mistake.
func validateWorkerLLM(sc config.ServerConfig) error {
	if !sc.Worker.LLM.Managed() {
		return nil
	}
	if len(sc.LLM.Aliases) == 0 {
		return fmt.Errorf("worker.llm.transport is %q but llm.aliases is empty, so no team may call the gateway",
			config.TransportBuildMax)
	}
	if alias := sc.Worker.LLM.Alias; alias != "" {
		if _, ok := sc.LLM.Aliases[alias]; !ok {
			return fmt.Errorf("worker.llm.alias %q is not one of llm.aliases", alias)
		}
	} else if sc.LLM.DefaultAlias == "" && len(sc.LLM.Aliases) > 1 {
		return fmt.Errorf("worker.llm.alias is unset and llm.default_alias does not say which of %d aliases a run should use",
			len(sc.LLM.Aliases))
	}
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
			k8s.WorkerEnvFromEnviron(wc.LLM.Managed()),
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
		return scheduler.NewLocalRunner(wc.Binary, config.FilterWorkerEnv(os.Environ(), wc.LLM.Managed()), config.EnvKeyBuildmaxRunToken), nil
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
