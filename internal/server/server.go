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

	"buildmax/internal/storage/blob"
	"buildmax/internal/storage/entity"
)

//go:embed static/openapi.json static/swagger.html
var staticFS embed.FS

const shutdownTimeout = 10 * time.Second

// Config holds server configuration.
type Config struct {
	Addr            string                      // Listen address (e.g. ":5678")
	UserStore       entity.UserStore            // Optional; required for login
	WorkspaceStore  entity.WorkspaceStore       // Optional; required for GET /api/workspaces
	ProjectStore    entity.ProjectStore         // Optional; required for project list/create
	TaskStore       entity.TaskStore            // Optional; required for task list/create
	TaskRunStore    entity.TaskRunStore         // Optional; required for POST runs and worker task-runs API
	ArtifactStore   entity.ArtifactStore        // Optional; required for GET /api/workspaces/{id}/artifacts and artifact content
	PersistStorage  blob.PersistStorage         // Optional; required for upload and Explore (files tree/content)
	ArtifactStorage blob.ArtifactStorage        // Optional; required for artifact content file read
	WorkspacesDir   string                      // Optional; overrides config.WorkspacesDir() for workspace file operations
	JWTSecret       string                      // Required for login when UserStore is set
	CORSOrigin      string                      // If set, enable CORS with this origin (e.g. "http://localhost:5173")
	WorkerToken     string                      // If set, required for /api/worker/* (worker-to-server auth)
}

// Server wraps the HTTP server and runs it.
type Server struct {
	srv *http.Server
	cfg Config
}

// New builds an HTTP server with routes for /healthz, /openapi.json, /swagger/, and POST /api/login.
func New(cfg Config) *Server {
	s := &Server{cfg: cfg}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthzHandler)
	mux.HandleFunc("GET /openapi.json", openAPIHandler)
	mux.HandleFunc("GET /swagger/", swaggerUIHandler)
	mux.HandleFunc("GET /swagger/index.html", swaggerUIHandler)
	mux.HandleFunc("GET /swagger", swaggerUIHandler)
	mux.HandleFunc("POST /api/otp/request", s.otpRequestHandler)
	mux.HandleFunc("POST /api/login", s.loginHandler)
	mux.HandleFunc("GET /api/workspaces", s.workspacesHandler)
	mux.HandleFunc("POST /api/workspaces", s.createWorkspaceHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/projects", s.listProjectsHandler)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/projects", s.createProjectHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/tasks", s.listWorkspaceTasksHandler)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/tasks", s.createWorkspaceTaskHandler)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/tasks/{task_id}/runs", s.createTaskRunHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/artifacts", s.listWorkspaceArtifactsHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/artifacts/{artifact_id}/items", s.listArtifactItemsHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/artifacts/{artifact_id}/content", s.artifactContentHandler)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/upload", s.uploadHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/files", s.filesTreeHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/files/{path...}", s.fileContentHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/tasks/{task_id}/conversation", s.getTaskConversationHandler)
	mux.Handle("GET /api/worker/task-runs/{run_id}", s.workerAuthMiddleware(http.HandlerFunc(s.getWorkerTaskRunHandler)))
	mux.Handle("PATCH /api/worker/task-runs/{run_id}", s.workerAuthMiddleware(http.HandlerFunc(s.patchWorkerTaskRunHandler)))

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

// serveStaticJSON reads path from staticFS and serves it as application/json. On read error, logs and writes 500.
func serveStaticJSON(w http.ResponseWriter, path string) {
	data, err := staticFS.ReadFile(path)
	if err != nil {
		slog.Error("static file read failed", "err", err, "path", path)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// serveStaticHTML reads path from staticFS and serves it as text/html. On read error, logs and writes 500.
func serveStaticHTML(w http.ResponseWriter, path string) {
	data, err := staticFS.ReadFile(path)
	if err != nil {
		slog.Error("static file read failed", "err", err, "path", path)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func openAPIHandler(w http.ResponseWriter, _ *http.Request) {
	serveStaticJSON(w, "static/openapi.json")
}

func swaggerUIHandler(w http.ResponseWriter, _ *http.Request) {
	serveStaticHTML(w, "static/swagger.html")
}
