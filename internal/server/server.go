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

	"buildmax/internal/conversation"
	"buildmax/internal/llm"
	"buildmax/internal/quota"
	"buildmax/internal/server/worker"
	"buildmax/internal/streamhub"
	"buildmax/internal/storage/blob"
	"buildmax/internal/storage/entity"
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

// RunOutputLister lists run outputs (artifacts) by workspace and gets output files for a run.
type RunOutputLister interface {
	ListRunOutputsByWorkspace(ctx context.Context, workspaceID string, chatID *string) ([]entity.ArtifactWithChat, error)
	GetChatRunOutputFiles(ctx context.Context, chatRunID string) ([]entity.ChatRunArtifact, error)
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
	UserStore       entity.UserStore
	WorkspaceStore  entity.WorkspaceStore
	AgentStore      entity.AgentStore
	ChatStore       entity.ChatStore
	ChatRunStore    entity.ChatRunStore
	RunOutputLister RunOutputLister
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

// ConversationConfig holds Tier 1 conversation engine, adapter, stores, and LLM.
type ConversationConfig struct {
	ChatTitleGenerator       ChatTitleGenerator
	ConversationEngine       conversation.ConversationEngine
	PortalAdapter            conversation.ChannelAdapter
	ConversationStore        entity.ConversationStore
	ConversationMessageStore entity.ConversationMessageStore
	ConversationLLMCaller    llm.LLMCaller
}

// Config holds server configuration. Grouped fields document what is required for auth, storage, worker, and conversation.
type Config struct {
	Addr    string // Listen address (e.g. ":5678")
	Auth    AuthConfig
	Stores  StoresConfig
	Storage StorageConfig
	Worker  WorkerConfig
	Conv    ConversationConfig
}

// Server wraps the HTTP server and runs it.
type Server struct {
	srv *http.Server
	cfg Config
	hub streamhub.StreamHub // in-memory stream buffer per run (Phase 1); nil if not used
}

// New builds an HTTP server with routes for /healthz, /openapi.json, /swagger/, and POST /api/login.
func New(cfg Config) *Server {
	s := &Server{cfg: cfg, hub: streamhub.NewStreamHub()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthzHandler)
	mux.HandleFunc("GET /openapi.json", openAPIHandler)
	mux.HandleFunc("GET /swagger/", swaggerUIHandler)
	mux.HandleFunc("GET /swagger/index.html", swaggerUIHandler)
	mux.HandleFunc("GET /swagger", swaggerUIHandler)
	mux.HandleFunc("POST /api/otp/request", s.otpRequestHandler)
	mux.HandleFunc("POST /api/login", s.loginHandler)
	mux.HandleFunc("GET /api/usage", s.usageHandler)
	mux.HandleFunc("GET /api/workspaces", s.workspacesHandler)
	mux.HandleFunc("POST /api/workspaces", s.createWorkspaceHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/agents", s.listAgentsHandler)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/agents", s.createAgentHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/agents/{agent_id}", s.getAgentHandler)
	mux.HandleFunc("PATCH /api/workspaces/{workspace_id}/agents/{agent_id}", s.patchAgentHandler)
	mux.HandleFunc("DELETE /api/workspaces/{workspace_id}/agents/{agent_id}", s.deleteAgentHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/chats", s.listWorkspaceChatsHandler)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/chats", s.createWorkspaceChatHandler)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/chats/{chat_id}/runs", s.createChatRunHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/artifacts", s.listWorkspaceArtifactsHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/artifacts/{chat_run_id}/items", s.listArtifactItemsHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/artifacts/{chat_run_id}/content", s.artifactContentHandler)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/upload", s.uploadHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/files", s.filesTreeHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/files/{path...}", s.fileContentHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/chats/{chat_id}/conversation", s.getChatConversationHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/chats/{chat_id}/stream", s.getChatStreamHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/conversations", s.listConversationsHandler)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/conversations", s.createConversationHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/conversations/{conversation_id}/messages", s.getConversationMessagesHandler)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/conversations/{conversation_id}/messages", s.addConversationMessageHandler)
	worker.NewHandler(worker.Config{
		Token:        cfg.Worker.WorkerToken,
		ChatRunStore: cfg.Stores.ChatRunStore,
		Hub:          s.hub,
	}).Register(mux)

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
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// serveStatic reads path from staticFS and serves it with the given Content-Type. On read error, logs and writes 500.
func serveStatic(w http.ResponseWriter, path, contentType string) {
	data, err := staticFS.ReadFile(path)
	if err != nil {
		slog.Error("static file read failed", "err", err, "path", path)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
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
