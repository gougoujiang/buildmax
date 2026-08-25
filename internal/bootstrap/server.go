package bootstrap

import (
	"context"
	"fmt"
	"github.com/gougoujiang/buildmax/internal/server/handlers/admin"
	artifactsvc "github.com/gougoujiang/buildmax/internal/service/artifact"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	coregw "github.com/gougoujiang/buildmax/internal/core/llmgateway"
	"github.com/gougoujiang/buildmax/internal/infra/db"
	"github.com/gougoujiang/buildmax/internal/infra/k8s"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
	httpserver "github.com/gougoujiang/buildmax/internal/server"
	"github.com/gougoujiang/buildmax/internal/server/authtoken"
	"github.com/gougoujiang/buildmax/internal/server/scheduler"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
	pluginsvc "github.com/gougoujiang/buildmax/internal/service/plugin"
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
	completion, err := a.client.ChatCompletionBlocking(ctx, cllm.Request{Messages: msgs, Profile: cllm.ProfileTitle})
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

	// The budget is resolved before anything starts, because the scheduler's
	// runner needs the worker's share of it at construction: how long a worker
	// gets to report is part of how it is spawned.
	budget := httpserver.NewShutdownBudget(sc.ShutdownGrace)

	runner, err := buildWorkerRunner(sc.Worker, budget.Workers)
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

	cleaner := scheduler.NewCredentialCleaner(store, 0)
	cleaner.Start()

	reaper := scheduler.NewStaleRunReaper(store, sc.Worker.RunTimeout, 0)
	reaper.Start()

	// Nil unless the operator set a retention window, so a deployment that
	// never chose one keeps every event.
	retainer := scheduler.NewAuditRetainer(store, store, sc.Audit.RetentionDays, 0)
	retainer.Start()

	s := httpserver.New(serverConfig)
	s.StartBackground()
	slog.Info("server starting",
		"addr", serverConfig.Addr,
		"version", config.Version,
		"commit", config.Commit,
	)

	signalCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	serveErr := make(chan error, 1)
	go func() { serveErr <- s.ListenAndServe() }()

	select {
	case err := <-serveErr:
		// The listener failed before any signal — a taken port, a bad address.
		// Nothing has started serving, so there is nothing to drain.
		shutdownServer(context.Background(), targetsFor(s, sched, cleaner, reaper, retainer), budget)
		return err
	case <-signalCtx.Done():
	}

	// Stop listening for signals before the ladder starts: a second Ctrl-C
	// should abort a shutdown that is taking too long, not be swallowed by the
	// handler that is already running one.
	stopSignals()
	slog.Info("shutdown requested", "grace", sc.ShutdownGrace)
	shutdownServer(ctx, targetsFor(s, sched, cleaner, reaper, retainer), budget)

	slog.Info("server stopped")
	return <-serveErr
}

// serverLifecycle is the shutdown surface of the HTTP server. An interface
// because the order below is the whole point of this code, and a test that
// cannot observe the order cannot defend it.
type serverLifecycle interface {
	Drain()
	Shutdown(ctx context.Context) error
	StopBackground(ctx context.Context)
}

// namedStop is a component's stop and the name it is reported under when it
// overruns its share of the budget. The context is the budget: a stop that can
// observe it stops early rather than being abandoned.
type namedStop struct {
	name string
	stop func(context.Context)
}

// shutdownTargets is everything RunServer started, in one value so the ladder
// below reads as a sequence rather than as argument plumbing.
type shutdownTargets struct {
	server serverLifecycle
	// scheduler stops before the HTTP server, not after: what it dispatches
	// reports back over the API.
	scheduler namedStop
	// loops are the sweeps that need nothing from anyone, so they stop last.
	loops []namedStop
}

// targetsFor names what RunServer started in the order the ladder stops it.
func targetsFor(s *httpserver.Server, sched *scheduler.Scheduler, cleaner *scheduler.CredentialCleaner, reaper *scheduler.StaleRunReaper, retainer *scheduler.AuditRetainer) shutdownTargets {
	return shutdownTargets{
		server:    s,
		scheduler: namedStop{name: "scheduler", stop: func(ctx context.Context) { sched.Stop(ctx) }},
		loops: []namedStop{
			{name: "audit retainer", stop: ignoringContext(retainer.Stop)},
			{name: "stale run reaper", stop: ignoringContext(reaper.Stop)},
			{name: "credential cleaner", stop: ignoringContext(cleaner.Stop)},
		},
	}
}

// ignoringContext adapts a stop that ends its own loop promptly and has nothing
// to shorten. stopWithin still bounds the wait.
func ignoringContext(stop func()) func(context.Context) {
	return func(context.Context) { stop() }
}

// shutdownServer walks the shutdown ladder from docs/design/graceful-shutdown.md §3.
//
// The order is not stylistic. A worker reports its outcome over this server's
// own HTTP API, so the listener has to outlive the runs — which is why the
// scheduler stops above the HTTP shutdown rather than in a defer below it.
// Every rung is bounded: a stop that hangs is worse than one that loses a
// little work.
func shutdownServer(ctx context.Context, t shutdownTargets, budget httpserver.ShutdownBudget) {
	// Rung 1: out of the load balancer, and watcher streams told to go
	// elsewhere. Immediate, and everything below is quieter for it.
	t.server.Drain()

	// Rungs 2-3: no new run is claimed, and the runs already dispatched are
	// asked to stop and given their window to report — while the API they
	// report to is still listening. That is what the whole order is for.
	stopWithin(ctx, budget.Workers, t.scheduler.name, t.scheduler.stop)

	// Rungs 4-6: streams have already been told (rung 1) and get their moment
	// to return before ordinary requests are drained and the listener closes.
	sleepWithin(ctx, budget.Streams)
	requestCtx, cancelRequests := context.WithTimeout(ctx, budget.Requests)
	defer cancelRequests()
	if err := t.server.Shutdown(requestCtx); err != nil {
		slog.Warn("some requests did not finish before shutdown", "err", err)
	}

	// Rung 7: last, because everything above could still have enqueued work
	// here — a run reported during rung 3 fires terminal callbacks.
	backgroundCtx, cancelBackground := context.WithTimeout(ctx, budget.Background)
	defer cancelBackground()
	t.server.StopBackground(backgroundCtx)
	for _, loop := range t.loops {
		stopWithin(ctx, budget.Background, loop.name, loop.stop)
	}
}

// stopWithin runs stop, which blocks until its loop has finished, and gives up
// waiting after limit. Giving up leaks the goroutine, which is acceptable
// exactly here: the process is about to exit anyway, and the alternative is a
// shutdown that never completes.
func stopWithin(ctx context.Context, limit time.Duration, name string, stop func(context.Context)) {
	stopCtx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	done := make(chan struct{})
	go func() {
		stop(stopCtx)
		close(done)
	}()
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		slog.Warn("component did not stop within its shutdown budget", "component", name, "budget", limit)
	case <-ctx.Done():
		slog.Warn("shutdown abandoned while stopping a component", "component", name)
	}
}

// sleepWithin waits out a phase that has nothing to wait on — the moment
// watcher streams need to notice the drain and return.
func sleepWithin(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
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

// blobStorage is what one deployment stores and where.
//
// The four are separate key spaces rather than one bucket with four names: a
// team's mutable home, the reproducible output a run leaves, the durable
// artifacts the team keeps, and plugin packages.
type blobStorage struct {
	persist   blob.PersistStorage
	runOutput blob.RunOutputStorage
	artifact  artifactsvc.ContentStore
	packages  pluginsvc.PackageStore
	// packageKeyPrefix scopes package keys inside whichever backend holds them.
	packageKeyPrefix string
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
	packages, packagePrefix := BuildPluginPackageStorage(wsCfg, workspacesDir, s3Client)
	return blobStorage{
		persist:          persistStorage,
		runOutput:        runOutputStorage,
		artifact:         artifactStorage,
		packages:         packages,
		packageKeyPrefix: packagePrefix,
	}, nil
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
	pluginService := &pluginsvc.Service{
		Catalog:     st,
		Activations: st,
		Teams:       st,
		Packages:    storage.packages,
		KeyPrefix:   storage.packageKeyPrefix,
		Audit:       audit.NewRecorder(st),
	}
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
			UserStore:               st,
			LoginCodeStore:          st,
			PasswordStore:           st,
			RefreshTokenStore:       st,
			TeamStore:               st,
			WorkflowStore:           st,
			AgentStore:              st,
			IssueStore:              st,
			IssueCommentStore:       st,
			TaskStore:               st,
			TaskRunStore:            st,
			TaskResultDeliveryStore: st,
			LLMCallStore:            st,
			RunOutputLister:         st,
			UserWebhookKeyStore:     st,
			AuditStore:              st,
			SystemGrantStore:        st,
			SchemaStore:             st,
			LLMModelStore:           st,
			ArtifactStore:           st,
		},
		Services: httpserver.ServicesConfig{Plugin: pluginService},
		Storage: httpserver.StorageConfig{
			PersistStorage:   storage.persist,
			RunOutputStorage: storage.runOutput,
			ArtifactStorage:  storage.artifact,
			MaxArtifactBytes: int64(sc.Storage.MaxArtifactMB) << 20,
			WorkspacesDir:    workspacesDir,
		},
		Worker: httpserver.WorkerConfig{
			LLM: workerLLMDescriptor(sc.Worker.LLM),
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
		Readiness: readinessChecks(st, storage.persist),
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
		DefaultModel:       sc.LLM.DefaultModel,
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
	var models coregw.ModelStore
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
	// Startup, not first call: a name in server.yaml that resolves to nothing is
	// a configuration mistake, and it should stop the server rather than surface
	// later as a model outage.
	if err := validateConfiguredModels(context.Background(), routing, sc); err != nil {
		return err
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
			"model", sc.Worker.LLM.Model, "run_token_ttl", ttl)
	}
	return func(claims authtoken.RunClaims) (string, error) {
		return authtoken.MintRun(jwtSecret, claims, ttl, time.Now())
	}
}

// workerLLMDescriptor is what a worker is told about models for its run. It
// carries a model name and nothing else — the endpoint, the upstream model, and
// the credential stay on the server.
//
// Nil means direct, so a deployment that has not enabled managed inference sends
// the field at all.
func workerLLMDescriptor(wc config.ServerWorkerLLMConfig) *workerclient.TaskRunLLM {
	if !wc.Managed() {
		return nil
	}
	return &workerclient.TaskRunLLM{
		Transport:     config.TransportBuildMax,
		Model:         wc.Model,
		ContextWindow: wc.ContextWindow,
		CallTimeout:   wc.CallTimeout,
	}
}

// validateConfiguredModels rejects a server.yaml that names a model the catalog
// does not have.
//
// It runs against the assembled catalog rather than against configuration alone
// because the catalog is a table: only a lookup can tell a real name from a
// typo. An empty catalog is not an error — an operator may add rows after
// starting the server — but a name that was written down and resolves to
// nothing is, since it parses cleanly and would then fail every call that
// relied on it.
func validateConfiguredModels(ctx context.Context, routing *llmRouting, sc config.ServerConfig) error {
	if routing == nil || routing.Router == nil || routing.Router.Resolver == nil {
		return nil
	}
	catalog := routing.Router.Resolver.Catalog
	check := func(field, name string) error {
		if name == "" {
			return nil
		}
		if _, err := catalog.TargetByName(ctx, name); err != nil {
			return fmt.Errorf("%s names %q, which is not in the model catalog: %w", field, name, err)
		}
		return nil
	}
	if err := check("llm.default_model", sc.LLM.DefaultModel); err != nil {
		return err
	}
	if sc.Worker.LLM.Managed() {
		return check("worker.llm.model", sc.Worker.LLM.Model)
	}
	return nil
}

func buildWorkerRunner(wc config.ServerWorkerConfig, stopGrace time.Duration) (scheduler.WorkerRunner, error) {
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
		// The two windows have to nest: a worker asked to stop reports over
		// this server's API, so it must finish inside the window the server
		// spends waiting for it. Derived from one budget rather than configured
		// separately, so an operator raising shutdown_grace moves both.
		env := config.FilterWorkerEnv(os.Environ(), wc.LLM.Managed())
		env = append(env, fmt.Sprintf("%s=%s", config.EnvKeyBuildmaxRunInterruptGrace, workerReportWindow(stopGrace)))
		return scheduler.NewLocalRunner(wc.Binary, env, config.EnvKeyBuildmaxRunToken, stopGrace), nil
	}
}

// workerReportWindow is how long a worker gets to report, given how long the
// server will wait for it. The margin covers the signal reaching the process and
// the process exiting after its last write.
func workerReportWindow(stopGrace time.Duration) time.Duration {
	return stopGrace * 80 / 100
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
