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

	"buildmax/internal/store"
)

const shutdownTimeout = 10 * time.Second

// Config holds server configuration.
type Config struct {
	Addr           string                // Listen address (e.g. ":5678")
	UserStore      store.UserStore       // Optional; required for login
	WorkspaceStore store.WorkspaceStore  // Optional; required for GET /api/workspaces
	ProjectStore   store.ProjectStore   // Optional; required for project list/create
	TaskStore      store.TaskStore       // Optional; required for task list/create
	JWTSecret      string                // Required for login when UserStore is set
	CORSOrigin     string                // If set, enable CORS with this origin (e.g. "http://localhost:5173")
}

// Server wraps the HTTP server and runs it.
type Server struct {
	srv *http.Server
	cfg Config
}

// openAPISpec is the minimal OpenAPI 3.0 spec for the server endpoints.
const openAPISpec = `{
  "openapi": "3.0.3",
  "info": { "title": "BuildMax API", "version": "0.0.5" },
  "components": {
    "securitySchemes": {
      "bearerAuth": {
        "type": "http",
        "scheme": "bearer",
        "bearerFormat": "JWT"
      }
    }
  },
  "paths": {
    "/healthz": {
      "get": {
        "summary": "Health check",
        "description": "Returns 200 when the server is alive.",
        "responses": { "200": { "description": "OK" } }
      }
    },
    "/api/login": {
      "post": {
        "summary": "Login",
        "description": "Authenticate by email. Returns JWT and user info on success.",
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["email"],
                "properties": { "email": { "type": "string" } }
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "OK",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "token": { "type": "string" },
                    "user": {
                      "type": "object",
                      "properties": {
                        "id": { "type": "string" },
                        "email": { "type": "string" },
                        "name": { "type": "string" }
                      }
                    }
                  }
                }
              }
            }
          },
          "401": { "description": "User not found" },
          "400": { "description": "Invalid request body" }
        }
      }
    },
    "/api/workspaces": {
      "get": {
        "summary": "List workspaces",
        "description": "Returns workspaces for the authenticated user. Creates a Default workspace if none exist. Requires Bearer JWT.",
        "security": [{ "bearerAuth": [] }],
        "responses": {
          "200": {
            "description": "OK",
            "content": {
              "application/json": {
                "schema": {
                  "type": "array",
                  "items": {
                    "type": "object",
                    "properties": {
                      "id": { "type": "string" },
                      "name": { "type": "string" },
                      "owner_user_id": { "type": "string" },
                      "created_at": { "type": "integer" }
                    }
                  }
                }
              }
            }
          },
          "401": { "description": "Unauthorized" }
        }
      }
    },
    "/api/workspaces/{workspace_id}/projects": {
      "get": {
        "summary": "List projects",
        "description": "Returns projects for the given workspace. Caller must own the workspace. Requires Bearer JWT.",
        "security": [{ "bearerAuth": [] }],
        "parameters": [
          { "name": "workspace_id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": {
          "200": {
            "description": "OK",
            "content": {
              "application/json": {
                "schema": {
                  "type": "array",
                  "items": {
                    "type": "object",
                    "properties": {
                      "id": { "type": "string" },
                      "workspace_id": { "type": "string" },
                      "name": { "type": "string" },
                      "description": { "type": "string" },
                      "created_at": { "type": "integer" }
                    }
                  }
                }
              }
            }
          },
          "401": { "description": "Unauthorized" },
          "403": { "description": "Forbidden (workspace not owned)" }
        }
      },
      "post": {
        "summary": "Create project",
        "description": "Creates a project under the given workspace. Caller must own the workspace. Requires Bearer JWT.",
        "security": [{ "bearerAuth": [] }],
        "parameters": [
          { "name": "workspace_id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["name"],
                "properties": {
                  "name": { "type": "string" },
                  "description": { "type": "string" }
                }
              }
            }
          }
        },
        "responses": {
          "201": {
            "description": "Created",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "id": { "type": "string" },
                    "workspace_id": { "type": "string" },
                    "name": { "type": "string" },
                    "description": { "type": "string" },
                    "created_at": { "type": "integer" }
                  }
                }
              }
            }
          },
          "400": { "description": "Bad request (name missing)" },
          "401": { "description": "Unauthorized" },
          "403": { "description": "Forbidden" }
        }
      }
    },
    "/api/projects/{project_id}/tasks": {
      "get": {
        "summary": "List tasks",
        "description": "Returns tasks for the project. Caller must own the project's workspace. Requires Bearer JWT.",
        "security": [{ "bearerAuth": [] }],
        "parameters": [
          { "name": "project_id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": {
          "200": {
            "description": "OK",
            "content": {
              "application/json": {
                "schema": {
                  "type": "array",
                  "items": {
                    "type": "object",
                    "properties": {
                      "id": { "type": "string" },
                      "project_id": { "type": "string" },
                      "status": { "type": "string" },
                      "input": { "type": "string" },
                      "output": { "type": "string" },
                      "created_by": { "type": "string" },
                      "created_at": { "type": "integer" },
                      "started_at": { "type": "integer" },
                      "ended_at": { "type": "integer" },
                      "error_message": { "type": "string" }
                    }
                  }
                }
              }
            }
          },
          "401": { "description": "Unauthorized" },
          "403": { "description": "Forbidden" },
          "404": { "description": "Project not found" },
          "503": { "description": "Tasks not configured" }
        }
      },
      "post": {
        "summary": "Create task",
        "description": "Creates a task under the project. Caller must own the project's workspace. Requires Bearer JWT.",
        "security": [{ "bearerAuth": [] }],
        "parameters": [
          { "name": "project_id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["input"],
                "properties": { "input": { "type": "string" } }
              }
            }
          }
        },
        "responses": {
          "201": {
            "description": "Created",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "id": { "type": "string" },
                    "project_id": { "type": "string" },
                    "status": { "type": "string" },
                    "input": { "type": "string" },
                    "output": { "type": "string" },
                    "created_by": { "type": "string" },
                    "created_at": { "type": "integer" },
                    "started_at": { "type": "integer" },
                    "ended_at": { "type": "integer" },
                    "error_message": { "type": "string" }
                  }
                }
              }
            }
          },
          "400": { "description": "Bad request (input missing)" },
          "401": { "description": "Unauthorized" },
          "403": { "description": "Forbidden" },
          "404": { "description": "Project not found" },
          "503": { "description": "Tasks not configured" }
        }
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

// New builds an HTTP server with routes for /healthz, /openapi.json, /swagger/, and POST /api/login.
func New(cfg Config) *Server {
	s := &Server{cfg: cfg}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthzHandler)
	mux.HandleFunc("GET /openapi.json", openAPIHandler)
	mux.HandleFunc("GET /swagger/", swaggerUIHandler)
	mux.HandleFunc("GET /swagger/index.html", swaggerUIHandler)
	mux.HandleFunc("GET /swagger", swaggerUIHandler)
	mux.HandleFunc("POST /api/login", s.loginHandler)
	mux.HandleFunc("GET /api/workspaces", s.workspacesHandler)
	mux.HandleFunc("GET /api/workspaces/{workspace_id}/projects", s.listProjectsHandler)
	mux.HandleFunc("POST /api/workspaces/{workspace_id}/projects", s.createProjectHandler)
	mux.HandleFunc("GET /api/projects/{project_id}/tasks", s.listTasksHandler)
	mux.HandleFunc("POST /api/projects/{project_id}/tasks", s.createTaskHandler)

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

// requestLoggingMiddleware logs each request (method, path, remote) for trace/debugging.
func requestLoggingMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
		}
		if r.URL.RawQuery != "" {
			attrs = append(attrs, "query", r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "" {
			attrs = append(attrs, "auth", "present")
		}
		slog.Info("request", attrs...)
		h.ServeHTTP(w, r)
	})
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
