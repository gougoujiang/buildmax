// Package server provides the HTTP server for BuildMax (backend for portal).
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

// Config holds server configuration.
type Config struct {
	Addr               string                 // Listen address (e.g. ":5678")
	UserStore          entity.UserStore       // Optional; required for login
	WorkspaceStore     entity.WorkspaceStore  // Optional; required for GET /api/workspaces
	AgentStore         entity.AgentStore      // Optional; required for GET/POST /api/workspaces/{id}/agents
	ChatStore          entity.ChatStore       // Optional; required for chat list/create
	ChatRunStore       entity.ChatRunStore    // Optional; required for POST runs and worker chat-runs API
	RunOutputLister    RunOutputLister        // Optional; required for GET /api/workspaces/{id}/artifacts and artifact content
	PersistStorage     blob.PersistStorage    // Optional; required for upload and Explore (files tree/content)
	ArtifactStorage    blob.ArtifactStorage   // Optional; required for artifact content file read
	WorkspacesDir      string                 // Optional; overrides config.WorkspacesDir() for workspace file operations
	JWTSecret          string                 // Required for login when UserStore is set
	CORSOrigin         string                 // If set, enable CORS with this origin (e.g. "http://localhost:5173")
	WorkerToken        string                 // If set, required for /api/worker/* (worker-to-server auth)
	ChatTitleGenerator ChatTitleGenerator     // Optional; when set, used to generate chat title from input at create time
	QuotaChecker       *quota.Checker         // Optional; when set, create chat/run enforce quota and return 429 when exceeded
	DefaultQuotaTier   string                 // Default quota tier for new users (e.g. signup); used when calling CreateUser
	// Tier 1 conversation (optional). When set, create-run uses adapter + engine instead of direct CreateChatRun.
	ConversationEngine conversation.ConversationEngine // Optional; when set, createChatRunHandler uses Tier 1
	PortalAdapter      conversation.ChannelAdapter    // Optional; when ConversationEngine is set, used to build turn from request
	// Tier 1 conversation entity API: conversations and messages. When set, handlers serve /api/workspaces/{id}/conversations.
	ConversationStore        entity.ConversationStore        // Optional; required for conversation list/create
	ConversationMessageStore  entity.ConversationMessageStore // Optional; required for conversation messages
	ConversationLLMCaller     llm.LLMCaller                   // Optional; when set, run conversation LLM loop (stream and non-stream)
}

// Server wraps the HTTP server and runs it.
type Server struct {
	srv *http.Server
	cfg Config
	hub StreamHub // in-memory stream buffer per run (Phase 1); nil if not used
}

// New builds an HTTP server with routes for /healthz, /openapi.json, /swagger/, and POST /api/login.
func New(cfg Config) *Server {
	s := &Server{cfg: cfg, hub: NewStreamHub()}
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
	mux.Handle("GET /api/worker/chat-runs/{chat_run_id}", s.workerAuthMiddleware(http.HandlerFunc(s.getWorkerChatRunHandler)))
	mux.Handle("PATCH /api/worker/chat-runs/{chat_run_id}", s.workerAuthMiddleware(http.HandlerFunc(s.patchWorkerChatRunHandler)))
	mux.Handle("POST /api/worker/chat-runs/{chat_run_id}/stream", s.workerAuthMiddleware(http.HandlerFunc(s.postWorkerStreamHandler)))

	handler := http.Handler(mux)
	if cfg.CORSOrigin != "" {
		handler = corsMiddleware(handler, cfg.CORSOrigin)
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
