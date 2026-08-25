package server

import (
	"context"
	"embed"
	"errors"
	"github.com/gougoujiang/buildmax/internal/server/handlers/admin"
	artifactsvc "github.com/gougoujiang/buildmax/internal/service/artifact"
	"log/slog"
	"net/http"
	"sync"
	"time"

	agentdef "github.com/gougoujiang/buildmax/internal/core/agentdef"
	coreartifact "github.com/gougoujiang/buildmax/internal/core/artifact"
	coreconv "github.com/gougoujiang/buildmax/internal/core/conversation"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	coregw "github.com/gougoujiang/buildmax/internal/core/llmgateway"
	"github.com/gougoujiang/buildmax/internal/core/model"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	coreworkflow "github.com/gougoujiang/buildmax/internal/core/workflow"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
	"github.com/gougoujiang/buildmax/internal/server/handlers"
	workroutes "github.com/gougoujiang/buildmax/internal/server/handlers/work"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	"github.com/gougoujiang/buildmax/internal/service/conversation"
	convchannel "github.com/gougoujiang/buildmax/internal/service/conversation/channel"
	"github.com/gougoujiang/buildmax/internal/service/issue"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
	pluginsvc "github.com/gougoujiang/buildmax/internal/service/plugin"
	"github.com/gougoujiang/buildmax/internal/service/quota"
	"github.com/gougoujiang/buildmax/internal/service/task"
	"github.com/gougoujiang/buildmax/internal/service/workflow"
)

//go:embed static/openapi.json static/swagger.html
var staticFS embed.FS

// readHeaderTimeout and idleTimeout bound connections that are doing nothing
// useful. Both matter to shutdown — see the http.Server literal in New.
const (
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 120 * time.Second
)

// AuthConfig holds auth and CORS settings plus optional quota for signup and create-chat/run.
type AuthConfig struct {
	JWTSecret        string         // Required for login when UserStore is set
	AllowSignup      bool           // Open POST /api/otp/request to self-registration; closed by default
	CORSOrigin       string         // If set, enable CORS with this origin (e.g. "http://localhost:5173")
	QuotaService     *quota.Service // Optional; when set, create chat/run enforce quota and return 429 when exceeded
	DefaultQuotaTier string         // Default quota tier for new users (e.g. signup); used when calling CreateUser
	// Token lifetimes; zero means the default in internal/core/model.
	AccessTokenTTL       time.Duration
	RefreshTokenTTL      time.Duration
	RefreshRotationGrace time.Duration
}

// StoresConfig holds entity store interfaces used by handlers.
type StoresConfig struct {
	UserStore         model.UserStore
	LoginCodeStore    model.LoginCodeStore
	PasswordStore     model.PasswordStore
	RefreshTokenStore model.RefreshTokenStore
	TeamStore         coreteam.Store
	WorkflowStore     coreworkflow.Store
	AgentStore        agentdef.Store
	IssueStore        model.IssueStore
	IssueCommentStore model.IssueCommentStore
	TaskStore         model.TaskStore
	TaskRunStore      model.TaskRunStore
	// TaskResultDeliveryStore records the reports the server owes finished
	// runs. Nil means a report that fails is not retried.
	TaskResultDeliveryStore model.TaskResultDeliveryStore
	LLMCallStore            coregw.CallStore
	RunOutputLister         workroutes.RunOutputLister
	UserWebhookKeyStore     model.UserWebhookKeyStore
	AuditStore              model.AuditStore
	SystemGrantStore        model.SystemGrantStore
	SchemaStore             model.SchemaStore
	LLMModelStore           coregw.ModelStore
	// ArtifactStore records durable files. Nil leaves the artifact routes
	// answering 503, which is what a deployment with no database has.
	ArtifactStore coreartifact.Store
}

// ServicesConfig holds application services the handlers reach through rather
// than a store directly.
type ServicesConfig struct {
	// Plugin publishes Marketplace releases and manages catalog entries. Nil
	// is a deployment with no Marketplace, which those routes report rather
	// than serving an empty catalog.
	Plugin *pluginsvc.Service
}

// StorageConfig holds blob storage and workspace paths.
type StorageConfig struct {
	PersistStorage   blob.PersistStorage
	RunOutputStorage blob.RunOutputStorage
	// ArtifactStorage holds artifact content. It is separate from
	// RunOutputStorage because they are different key spaces with different
	// lifetimes, not two names for one bucket.
	ArtifactStorage artifactsvc.ContentStore
	// MaxArtifactBytes caps one artifact. Zero uses the service default.
	MaxArtifactBytes int64
	WorkspacesDir    string // Overrides config.WorkspacesDir() for workspace file operations
}

// WorkerConfig holds what a worker is told about models for its run. Worker
// authentication is not here: it is the run token the scheduler mints, verified
// with the deployment's JWT secret.
type WorkerConfig struct {
	// LLM tells a worker how to reach a model. Nil means direct.
	LLM *workerclient.TaskRunLLM
}

// ConversationConfig holds Tier 1 conversation stores and LLM wiring.
type ConversationConfig struct {
	TitleGenerator           llm.TitleGenerator
	ConversationStore        coreconv.Store
	ConversationMessageStore coreconv.MessageStore
	ConversationLLMClient    llm.LLMClient
	// LLMGateway serves managed inference to authenticated clients. Nil leaves
	// the /llm routes answering 503.
	LLMGateway *llmgateway.Service
}

// WebhookConfig holds webhook handler options (message path and optional user ID for created runs).
type WebhookConfig struct {
	MessagePath string // JSON path for message in body (default "message")
	UserID      string // CreatedBy for webhook runs (default "webhook")
}

// Config holds server configuration. Grouped fields document what is required for auth, storage, worker, and conversation.
type Config struct {
	Addr     string // Listen address (e.g. ":5678")
	Auth     AuthConfig
	Stores   StoresConfig
	Services ServicesConfig
	Storage  StorageConfig
	Worker   WorkerConfig
	Conv     ConversationConfig
	Webhook  WebhookConfig
	// Audit records sensitive actions. Nil discards them.
	Audit *audit.Recorder
	// Deployment describes this deployment for the admin system status.
	Deployment admin.DeploymentInfo
	// RedactedConfig is the operator-facing view of server.yaml. Nil means the
	// admin configuration route answers 503.
	RedactedConfig any
	// Readiness lists the dependency probes GET /readyz runs. Empty means the
	// endpoint reports ready without verifying anything, and says so by
	// returning an empty check list.
	Readiness []ReadinessCheck
}

// Server wraps the HTTP server and runs it.
type Server struct {
	srv *http.Server
	cfg Config
	// handlers is kept so the server can run and stop the background work the
	// API surface owns — see StartBackground.
	handlers *handlers.Handler
	// drain is closed once this server is going away. It is a channel rather
	// than a flag because watcher streams block on it: a Portal tab following a
	// run has to be told, not polled. See docs/design/graceful-shutdown.md §5.
	drain     chan struct{}
	drainOnce sync.Once
}

// New builds an HTTP server with routes for healthz, readyz, openapi, swagger, and all API handlers.
func New(cfg Config) *Server {
	s := &Server{cfg: cfg, drain: make(chan struct{})}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthzHandler)
	mux.HandleFunc("GET /readyz", s.readyzHandler)
	mux.HandleFunc("GET /openapi.json", openAPIHandler)
	mux.HandleFunc("GET /swagger/", swaggerUIHandler)
	mux.HandleFunc("GET /swagger/index.html", swaggerUIHandler)
	mux.HandleFunc("GET /swagger", swaggerUIHandler)

	s.handlers = handlers.NewHandler(buildHandlersConfig(cfg, s.drain))
	s.handlers.Register(mux)

	handler := http.Handler(mux)
	if cfg.Auth.CORSOrigin != "" {
		handler = corsMiddleware(handler, cfg.Auth.CORSOrigin)
	}
	handler = requestLoggingMiddleware(handler)
	s.srv = &http.Server{
		Addr:    cfg.Addr,
		Handler: handler,
		// A connection that never completes its header is neither idle nor
		// active, so Shutdown neither closes it nor finishes waiting for it.
		// It is also the slowloris surface.
		ReadHeaderTimeout: readHeaderTimeout,
		// Bounds how many keep-alive connections are still open when draining
		// starts. Shutdown closes idle ones immediately, so fewer here is a
		// shorter drain.
		IdleTimeout: idleTimeout,
		// ReadTimeout and WriteTimeout stay unset on purpose. An artifact
		// upload is legitimately slow to read, and a write deadline would
		// truncate every SSE stream and every long managed model call.
	}
	return s
}

func buildHandlersConfig(cfg Config, drain <-chan struct{}) handlers.Config {
	msgPath := cfg.Webhook.MessagePath
	if msgPath == "" {
		msgPath = "message"
	}
	webhookUserID := cfg.Webhook.UserID
	if webhookUserID == "" {
		webhookUserID = convchannel.DefaultWebhookUserID
	}
	var webhookAdapter convchannel.Adapter
	var webhookEngine conversation.TurnEngine
	if cfg.Stores.UserWebhookKeyStore != nil {
		taskSvc := &task.Service{
			Agents:         cfg.Stores.AgentStore,
			Tasks:          cfg.Stores.TaskStore,
			TaskRuns:       cfg.Stores.TaskRunStore,
			QuotaChecker:   cfg.Auth.QuotaService,
			TitleGenerator: nil,
		}
		webhookAdapter = convchannel.NewWebhookAdapter(msgPath, webhookUserID)
		webhookEngine = &conversation.WebhookEngine{TaskService: taskSvc, Conversations: cfg.Conv.ConversationStore}
	}
	return handlers.Config{
		JWTSecret:                cfg.Auth.JWTSecret,
		AllowSignup:              cfg.Auth.AllowSignup,
		CORSOrigin:               cfg.Auth.CORSOrigin,
		AccessTokenTTL:           cfg.Auth.AccessTokenTTL,
		RefreshTokenTTL:          cfg.Auth.RefreshTokenTTL,
		RefreshRotationGrace:     cfg.Auth.RefreshRotationGrace,
		WorkerLLM:                cfg.Worker.LLM,
		UserStore:                cfg.Stores.UserStore,
		AuditStore:               cfg.Stores.AuditStore,
		SystemGrantStore:         cfg.Stores.SystemGrantStore,
		SchemaStore:              cfg.Stores.SchemaStore,
		LLMModelStore:            cfg.Stores.LLMModelStore,
		ArtifactStore:            cfg.Stores.ArtifactStore,
		PluginService:            cfg.Services.Plugin,
		Deployment:               cfg.Deployment,
		DependencyProbes:         dependencyProbes(cfg.Readiness),
		RedactedConfig:           cfg.RedactedConfig,
		Audit:                    cfg.Audit,
		LoginCodeStore:           cfg.Stores.LoginCodeStore,
		PasswordStore:            cfg.Stores.PasswordStore,
		RefreshTokenStore:        cfg.Stores.RefreshTokenStore,
		TeamStore:                cfg.Stores.TeamStore,
		WorkflowStore:            cfg.Stores.WorkflowStore,
		AgentStore:               cfg.Stores.AgentStore,
		IssueStore:               cfg.Stores.IssueStore,
		IssueCommentStore:        cfg.Stores.IssueCommentStore,
		TaskStore:                cfg.Stores.TaskStore,
		TaskRunStore:             cfg.Stores.TaskRunStore,
		TaskResultDeliveries:     cfg.Stores.TaskResultDeliveryStore,
		LLMCallStore:             cfg.Stores.LLMCallStore,
		RunOutputLister:          cfg.Stores.RunOutputLister,
		UserWebhookKeyStore:      cfg.Stores.UserWebhookKeyStore,
		ConversationStore:        cfg.Conv.ConversationStore,
		ConversationMessageStore: cfg.Conv.ConversationMessageStore,
		PersistStorage:           cfg.Storage.PersistStorage,
		RunOutputStorage:         cfg.Storage.RunOutputStorage,
		ArtifactStorage:          cfg.Storage.ArtifactStorage,
		MaxArtifactBytes:         cfg.Storage.MaxArtifactBytes,
		WorkspacesDir:            cfg.Storage.WorkspacesDir,
		DefaultQuotaTier:         cfg.Auth.DefaultQuotaTier,
		QuotaService:             cfg.Auth.QuotaService,
		TitleGenerator:           cfg.Conv.TitleGenerator,
		ConversationLLMClient:    cfg.Conv.ConversationLLMClient,
		LLMGateway:               cfg.Conv.LLMGateway,
		WebhookAdapter:           webhookAdapter,
		WebhookEngine:            webhookEngine,
		WebhookMessagePath:       msgPath,
		OnTaskRunTerminal:        buildOnTaskRunTerminal(cfg),
		Drain:                    drain,
	}
}

// buildOnTaskRunTerminal composes what should happen when a worker run reaches
// a terminal status: advance the workflow it belongs to, and report it on the
// issue it belongs to.
//
// Each half is wired independently and each failure is logged rather than
// propagated. A deployment without workflows still reports runs on issues, and
// a comment store that is down does not stop a workflow from advancing.
func buildOnTaskRunTerminal(cfg Config) func(ctx context.Context, info model.TaskRunTerminalInfo) {
	var workflowSvc *workflow.Service
	if cfg.Stores.WorkflowStore != nil {
		taskSvc := &task.Service{
			Agents:         cfg.Stores.AgentStore,
			Tasks:          cfg.Stores.TaskStore,
			TaskRuns:       cfg.Stores.TaskRunStore,
			QuotaChecker:   cfg.Auth.QuotaService,
			TitleGenerator: nil,
		}
		workflowSvc = &workflow.Service{
			Workflows:     cfg.Stores.WorkflowStore,
			Agents:        cfg.Stores.AgentStore,
			Issues:        cfg.Stores.IssueStore,
			Conversations: cfg.Conv.ConversationStore,
			TaskService:   taskSvc,
		}
	}
	var runReporter *issue.RunReporter
	if cfg.Stores.IssueCommentStore != nil && cfg.Stores.TaskStore != nil {
		runReporter = &issue.RunReporter{
			Tasks:    cfg.Stores.TaskStore,
			Comments: cfg.Stores.IssueCommentStore,
		}
	}
	if workflowSvc == nil && runReporter == nil {
		return nil
	}
	return func(ctx context.Context, info model.TaskRunTerminalInfo) {
		terminal := info
		if workflowSvc != nil {
			if err := workflowSvc.HandleTaskRunTerminal(ctx, terminal); err != nil && !errors.Is(err, workflow.ErrWorkflowRunNotFound) {
				slog.Warn("workflow terminal callback failed", "task_run_id", info.TaskRunID, "task_id", info.TaskID, "err", err)
			}
		}
		if runReporter != nil {
			if err := runReporter.ReportRunTerminal(ctx, terminal); err != nil {
				slog.Warn("issue run comment not written", "task_run_id", info.TaskRunID, "task_id", info.TaskID, "err", err)
			}
		}
	}
}

// Handler returns the HTTP handler for use in tests.
func (s *Server) Handler() http.Handler {
	return s.srv.Handler
}

// ListenAndServe serves until Shutdown is called, and reports nil for the
// orderly stop that causes.
//
// It does not install a signal handler. Stopping this process is a sequence
// that reaches further than the HTTP surface — see docs/design/graceful-shutdown.md
// — and the layer that assembles the scheduler and the background loops is the
// only one that can walk it in the right order.
func (s *Server) ListenAndServe() error {
	if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// StartBackground launches the background work the API surface owns.
//
// Called by whoever runs the server rather than by New, so that building a
// handler — which a test does freely — never starts a goroutine.
func (s *Server) StartBackground() { s.handlers.StartBackground() }

// StopBackground stops that work and waits for it, bounded by ctx.
func (s *Server) StopBackground(ctx context.Context) { s.handlers.StopBackground(ctx) }

// Drain marks this server as going away: /readyz starts answering 503 so the
// load balancer stops sending it new work, and watcher streams return so they
// stop holding the drain open. It is idempotent and does not wait.
//
// Requests in flight are untouched. Ending them is Shutdown's job, one rung
// further down.
func (s *Server) Drain() {
	s.drainOnce.Do(func() {
		close(s.drain)
		s.handlers.BeginDrain()
		slog.Info("server draining")
	})
}

// Draining reports whether Drain has been called.
func (s *Server) Draining() bool {
	select {
	case <-s.drain:
		return true
	default:
		return false
	}
}

// Shutdown stops accepting connections and waits for the work in flight,
// bounded by ctx. It also drains, so a caller that stops the server without
// walking the whole ladder still takes it out of the load balancer first.
//
// Conversation turns are waited for first and explicitly: one reached over a
// WebSocket lives on a hijacked connection, which http.Server.Shutdown returns
// without waiting for.
func (s *Server) Shutdown(ctx context.Context) error {
	s.Drain()
	s.handlers.WaitTurns(ctx)
	return s.srv.Shutdown(ctx)
}

func serveStatic(w http.ResponseWriter, path, contentType string) {
	data, err := staticFS.ReadFile(path)
	if err != nil {
		slog.Error("static file read failed", "err", err, "path", path)
		httputil.WriteJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func openAPIHandler(w http.ResponseWriter, _ *http.Request) {
	serveStatic(w, "static/openapi.json", "application/json")
}

func swaggerUIHandler(w http.ResponseWriter, _ *http.Request) {
	serveStatic(w, "static/swagger.html", "text/html; charset=utf-8")
}

// dependencyProbes converts readiness checks into the shape the admin API
// reports. Both are a name and a probe; the conversion exists so that the
// handler package does not import this one, which imports it.
func dependencyProbes(checks []ReadinessCheck) []admin.DependencyProbe {
	out := make([]admin.DependencyProbe, 0, len(checks))
	for _, check := range checks {
		out = append(out, admin.DependencyProbe{Name: check.Name, Probe: check.Probe})
	}
	return out
}
