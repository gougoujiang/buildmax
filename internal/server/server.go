package server

import (
	"context"
	"embed"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	convapp "buildmax/internal/app/conversation"
	taskapp "buildmax/internal/app/task"
	"buildmax/internal/conversation"
	"buildmax/internal/llm"
	"buildmax/internal/quota"
	"buildmax/internal/server/auth"
	"buildmax/internal/server/httputil"
	"buildmax/internal/server/portal"
	"buildmax/internal/server/webhook"
	"buildmax/internal/server/worker"
	"buildmax/internal/storage/blob"
	"buildmax/internal/storage/entity"
	"buildmax/internal/streamhub"
)

//go:embed static/openapi.json static/swagger.html
var staticFS embed.FS

const shutdownTimeout = 10 * time.Second

// ChatTitleGenerator generates a short title from chat input. Optional; when nil, create-chat uses truncated input.
// Usage is returned for metering (billing). When the generator is used, the server stores title token usage on the chat.
type ChatTitleGenerator interface {
	GenerateChatTitle(ctx context.Context, input string) (title string, usage TokenUsage, err error)
}

// TokenUsage holds prompt and completion token counts for a single LLM call.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
}

// RunOutputLister lists run outputs (artifacts) by conversation and gets output files for a run.
type RunOutputLister interface {
	ListRunOutputsByConversation(ctx context.Context, conversationID string, taskID *string) ([]entity.ArtifactWithTask, error)
	GetTaskRunOutputFiles(ctx context.Context, taskRunID string) ([]entity.TaskRunArtifact, error)
}

// AuthConfig holds auth and CORS settings plus optional quota for signup and create-chat/run.
type AuthConfig struct {
	JWTSecret        string         // Required for login when UserStore is set
	CORSOrigin       string         // If set, enable CORS with this origin (e.g. "http://localhost:5173")
	QuotaChecker     *quota.Checker // Optional; when set, create chat/run enforce quota and return 429 when exceeded
	DefaultQuotaTier string         // Default quota tier for new users (e.g. signup); used when calling CreateUser
}

// StoresConfig holds entity store interfaces used by handlers.
type StoresConfig struct {
	UserStore           entity.UserStore
	AgentStore          entity.AgentStore
	TaskStore           entity.TaskStore
	TaskRunStore        entity.TaskRunStore
	RunOutputLister     RunOutputLister
	UserWebhookKeyStore entity.UserWebhookKeyStore
}

// StorageConfig holds blob storage and workspace paths.
type StorageConfig struct {
	PersistStorage  blob.PersistStorage
	ArtifactStorage blob.ArtifactStorage
	WorkspacesDir   string // Overrides config.WorkspacesDir() for workspace file operations
}

// WorkerConfig holds worker-to-server auth.
type WorkerConfig struct {
	WorkerToken string // If set, required for /api/worker/*
}

// ConversationConfig holds Tier 1 conversation stores and LLM wiring.
type ConversationConfig struct {
	ChatTitleGenerator       ChatTitleGenerator
	ConversationStore        entity.ConversationStore
	ConversationMessageStore entity.ConversationMessageStore
	ConversationLLMCaller    llm.LLMCaller
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
}

// Server wraps the HTTP server and runs it.
type Server struct {
	srv *http.Server
	cfg Config
	hub streamhub.StreamHub // in-memory stream buffer per run (Phase 1); nil if not used
}

// chatTitleGenAdapter adapts server ChatTitleGenerator to portal.ChatTitleGenerator (TokenUsage type).
type chatTitleGenAdapter struct{ gen ChatTitleGenerator }

func (a chatTitleGenAdapter) GenerateChatTitle(ctx context.Context, input string) (string, portal.TokenUsage, error) {
	if a.gen == nil {
		return "", portal.TokenUsage{}, nil
	}
	title, usage, err := a.gen.GenerateChatTitle(ctx, input)
	return title, portal.TokenUsage{PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens}, err
}

// buildPortalConfig builds portal.Config from server config, the shared stream hub, and the connection registry.
func buildPortalConfig(cfg Config, hub streamhub.StreamHub, registry *portal.ConnRegistry) portal.Config {
	return portal.Config{
		JWTSecret:                cfg.Auth.JWTSecret,
		CORSOrigin:               cfg.Auth.CORSOrigin,
		AgentStore:               cfg.Stores.AgentStore,
		TaskStore:                cfg.Stores.TaskStore,
		TaskRunStore:             cfg.Stores.TaskRunStore,
		RunOutputLister:          cfg.Stores.RunOutputLister,
		PersistStorage:           cfg.Storage.PersistStorage,
		ArtifactStorage:          cfg.Storage.ArtifactStorage,
		WorkspacesDir:            cfg.Storage.WorkspacesDir,
		QuotaChecker:             cfg.Auth.QuotaChecker,
		ChatTitleGenerator:       chatTitleGenAdapter{cfg.Conv.ChatTitleGenerator},
		ConversationStore:        cfg.Conv.ConversationStore,
		ConversationMessageStore: cfg.Conv.ConversationMessageStore,
		ConversationLLMCaller:    cfg.Conv.ConversationLLMCaller,
		Hub:                      hub,
		UserWebhookKeyStore:      cfg.Stores.UserWebhookKeyStore,
		ConnRegistry:             registry,
	}
}

// New builds an HTTP server with routes for healthz, openapi, swagger, auth (login/OTP), portal (user API), and worker.
func New(cfg Config) *Server {
	s := &Server{cfg: cfg, hub: streamhub.NewStreamHub()}
	connRegistry := portal.NewConnRegistry()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthzHandler)
	mux.HandleFunc("GET /openapi.json", openAPIHandler)
	mux.HandleFunc("GET /swagger/", swaggerUIHandler)
	mux.HandleFunc("GET /swagger/index.html", swaggerUIHandler)
	mux.HandleFunc("GET /swagger", swaggerUIHandler)
	auth.NewHandler(auth.Config{
		UserStore:        cfg.Stores.UserStore,
		JWTSecret:        cfg.Auth.JWTSecret,
		DefaultQuotaTier: cfg.Auth.DefaultQuotaTier,
	}).Register(mux)
	portal.NewHandler(buildPortalConfig(cfg, s.hub, connRegistry)).Register(mux)
	worker.NewHandler(worker.Config{
		Token:        cfg.Worker.WorkerToken,
		TaskRunStore: cfg.Stores.TaskRunStore,
		Hub:          s.hub,
		OnTaskRunTerminal: connRegistry.OnTaskRunTerminal,
	}).Register(mux)
	if cfg.Stores.UserWebhookKeyStore != nil {
		msgPath := cfg.Webhook.MessagePath
		if msgPath == "" {
			msgPath = "message"
		}
		userID := cfg.Webhook.UserID
		if userID == "" {
			userID = conversation.DefaultWebhookUserID
		}
		taskSvc := &taskapp.Service{
			Agents:         cfg.Stores.AgentStore,
			Tasks:          cfg.Stores.TaskStore,
			TaskRuns:       cfg.Stores.TaskRunStore,
			QuotaChecker:   cfg.Auth.QuotaChecker,
			TitleGenerator: nil,
		}
		webhookHandler := webhook.NewHandler(webhook.Config{
			Adapter:           conversation.NewWebhookAdapter(msgPath, userID),
			Engine:            &convapp.RuleBasedEngine{Task: taskSvc},
			ConversationStore: cfg.Conv.ConversationStore,
			KeyStore:          cfg.Stores.UserWebhookKeyStore,
			MessagePath:       msgPath,
		})
		webhookHandler.Register(mux)
	}

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

func healthzHandler(w http.ResponseWriter, _ *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// serveStatic reads path from staticFS and serves it with the given Content-Type. On read error, logs and writes 500.
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
