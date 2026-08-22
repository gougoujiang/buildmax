package server

import (
	"context"
	"embed"
	"errors"
	"github.com/gougoujiang/buildmax/internal/server/handlers/admin"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/model"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
	"github.com/gougoujiang/buildmax/internal/server/handlers"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	"github.com/gougoujiang/buildmax/internal/service/conversation"
	convchannel "github.com/gougoujiang/buildmax/internal/service/conversation/channel"
	"github.com/gougoujiang/buildmax/internal/service/issue"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
	"github.com/gougoujiang/buildmax/internal/service/quota"
	"github.com/gougoujiang/buildmax/internal/service/task"
	"github.com/gougoujiang/buildmax/internal/service/workflow"
)

//go:embed static/openapi.json static/swagger.html
var staticFS embed.FS

const shutdownTimeout = 10 * time.Second

// RunOutputLister lists run outputs (artifacts) by conversation and gets output files for a run.
type RunOutputLister interface {
	ListRunOutputsByConversation(ctx context.Context, conversationID string, taskID *string) ([]model.ArtifactWithTask, error)
	GetTaskRunOutputFiles(ctx context.Context, taskRunID string) ([]model.TaskRunArtifact, error)
}

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
	UserStore           model.UserStore
	LoginCodeStore      model.LoginCodeStore
	PasswordStore       model.PasswordStore
	RefreshTokenStore   model.RefreshTokenStore
	TeamStore           model.TeamStore
	WorkflowStore       model.WorkflowStore
	AgentStore          model.AgentStore
	IssueStore          model.IssueStore
	IssueCommentStore   model.IssueCommentStore
	TaskStore           model.TaskStore
	TaskRunStore        model.TaskRunStore
	LLMCallStore        model.LLMCallStore
	RunOutputLister     RunOutputLister
	UserWebhookKeyStore model.UserWebhookKeyStore
	AuditStore          model.AuditStore
	SystemGrantStore    model.SystemGrantStore
	SchemaStore         model.SchemaStore
	LLMModelStore       model.LLMModelStore
	// ArtifactStore records durable files. Nil leaves the artifact routes
	// answering 503, which is what a deployment with no database has.
	ArtifactStore model.ArtifactStore
}

// StorageConfig holds blob storage and workspace paths.
type StorageConfig struct {
	PersistStorage   blob.PersistStorage
	RunOutputStorage blob.RunOutputStorage
	// ArtifactStorage holds artifact content. It is separate from
	// RunOutputStorage because they are different key spaces with different
	// lifetimes, not two names for one bucket.
	ArtifactStorage blob.ArtifactStorage
	// MaxArtifactBytes caps one artifact. Zero uses the service default.
	MaxArtifactBytes int64
	WorkspacesDir    string // Overrides config.WorkspacesDir() for workspace file operations
}

// WorkerConfig holds worker-to-server auth and what a worker is told about
// models for its run.
type WorkerConfig struct {
	WorkerToken string // If set, required for /api/worker/*
	// LLM tells a worker how to reach a model. Nil means direct.
	LLM *workerclient.TaskRunLLM
}

// ConversationConfig holds Tier 1 conversation stores and LLM wiring.
type ConversationConfig struct {
	TitleGenerator           llm.TitleGenerator
	ConversationStore        model.ConversationStore
	ConversationMessageStore model.ConversationMessageStore
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
	Addr    string // Listen address (e.g. ":5678")
	Auth    AuthConfig
	Stores  StoresConfig
	Storage StorageConfig
	Worker  WorkerConfig
	Conv    ConversationConfig
	Webhook WebhookConfig
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
}

// New builds an HTTP server with routes for healthz, readyz, openapi, swagger, and all API handlers.
func New(cfg Config) *Server {
	s := &Server{cfg: cfg}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthzHandler)
	mux.HandleFunc("GET /readyz", s.readyzHandler)
	mux.HandleFunc("GET /openapi.json", openAPIHandler)
	mux.HandleFunc("GET /swagger/", swaggerUIHandler)
	mux.HandleFunc("GET /swagger/index.html", swaggerUIHandler)
	mux.HandleFunc("GET /swagger", swaggerUIHandler)

	handlers.NewHandler(buildHandlersConfig(cfg)).Register(mux)

	handler := http.Handler(mux)
	if cfg.Auth.CORSOrigin != "" {
		handler = corsMiddleware(handler, cfg.Auth.CORSOrigin)
	}
	handler = requestLoggingMiddleware(handler)
	s.srv = &http.Server{
		Addr:    cfg.Addr,
		Handler: handler,
	}
	return s
}

func buildHandlersConfig(cfg Config) handlers.Config {
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
		WorkerToken:              cfg.Worker.WorkerToken,
		WorkerLLM:                cfg.Worker.LLM,
		UserStore:                cfg.Stores.UserStore,
		AuditStore:               cfg.Stores.AuditStore,
		SystemGrantStore:         cfg.Stores.SystemGrantStore,
		SchemaStore:              cfg.Stores.SchemaStore,
		LLMModelStore:            cfg.Stores.LLMModelStore,
		ArtifactStore:            cfg.Stores.ArtifactStore,
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

// Run starts the server and blocks until shutdown (SIGINT/SIGTERM). Returns nil or the shutdown error.
func (s *Server) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := s.srv.Shutdown(shutdownCtx); err != nil {
			slog.Warn("server shutdown", "err", err)
		}
	}()

	err := s.srv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	slog.Info("server stopped")
	return nil
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
