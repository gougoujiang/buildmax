// Package server provides the HTTP server for BuildMax (backend for portal).
package server

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const shutdownTimeout = 10 * time.Second

// Config holds server configuration.
type Config struct {
	Addr string // Listen address (e.g. ":5678")
}

// Server wraps the HTTP server and runs it.
type Server struct {
	srv *http.Server
}

// openAPISpec is the minimal OpenAPI 3.0 spec for the server endpoints.
const openAPISpec = `{
  "openapi": "3.0.3",
  "info": { "title": "BuildMax API", "version": "0.0.4" },
  "paths": {
    "/healthz": {
      "get": {
        "summary": "Health check",
        "description": "Returns 200 when the server is alive.",
        "responses": { "200": { "description": "OK" } }
      }
    }
  }
}`

// swaggerUIHTML loads Swagger UI from CDN and points it at /openapi.json.
const swaggerUIHTML = `<!DOCTYPE html>
<html>
<head>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.9.0/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5.9.0/swagger-ui-bundle.js"></script>
  <script>
    SwaggerUIBundle({ url: "/openapi.json", dom_id: "#swagger-ui" });
  </script>
</body>
</html>
`

// New builds an HTTP server with routes for /healthz, /openapi.json, and /swagger/.
func New(cfg Config) *Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthzHandler)
	mux.HandleFunc("GET /openapi.json", openAPIHandler)
	mux.HandleFunc("GET /swagger/", swaggerUIHandler)
	mux.HandleFunc("GET /swagger/index.html", swaggerUIHandler)
	mux.HandleFunc("GET /swagger", swaggerUIHandler)

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: mux,
	}
	return &Server{srv: srv}
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func openAPIHandler(w http.ResponseWriter, _ *http.Request) {
	// Validate that the constant is valid JSON at init would be possible; for simplicity we write it directly.
	// Pretty-print not required; the raw constant is already readable.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(openAPISpec))
}

func swaggerUIHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(swaggerUIHTML))
}
