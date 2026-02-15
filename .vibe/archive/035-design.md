# Design 035 — Buildmax server mode

## Goal

Add the **`buildmax server`** subcommand that starts a long-lived HTTP server (default port 5678), exposes **GET /healthz**, serves an **OpenAPI spec** at **/openapi.json**, and serves **Swagger UI** at **/swagger/** for local development; other APIs and portal integration are out of scope.

## Modules

| Module | Responsibility | Owns |
|--------|----------------|------|
| **internal/server** | HTTP server lifecycle, routes, healthz, OpenAPI spec, Swagger UI HTML | Server, Config, handlers, embedded or inlined spec and Swagger UI page |
| **internal/cmd** | `server` subcommand, port from flag/env, run server | server.go, registration in root |

## Structure

**New package: `internal/server`**

- **Config** — server configuration (listen address, e.g. `:5678`).
- **Server** — wraps `*http.Server` and route registration; created from Config.
- **Routes:**
  - `GET /healthz` — health check (200 + JSON `{"status":"ok"}`).
  - `GET /openapi.json` — OpenAPI 3.0 spec (at least path `/healthz`).
  - `GET /swagger/`, `GET /swagger/index.html` — Swagger UI: single HTML page that loads Swagger UI from a CDN (e.g. unpkg) and points `url` at `/openapi.json` (no embedding of swagger-ui-dist in Go to avoid large assets and extra tooling).

**Modified: `internal/cmd`**

- **server.go** (new) — `newServerCommand() *cobra.Command`: `Use: "server"`, `Short: "Start the HTTP server (backend for portal)"`. Flags: `--port` (int, default 0 meaning “use env or 5678”). RunE: resolve port (flag > env `BUILDMAX_SERVER_PORT` > 5678), build `server.Config{Addr: ":<port>"}`, create server, call `server.Run()` (blocks until shutdown). Log start and shutdown via `slog` (existing `internal/log`; no stdout/stderr).
- **root.go** — `root.AddCommand(newServerCommand())` next to `newVersionCommand()`.

**Port resolution order**

1. If `--port` > 0, use it.
2. Else if `BUILDMAX_SERVER_PORT` is set and parseable, use it.
3. Else use default **5678**.

Listen address: `:<port>` (e.g. `:5678`) so the server listens on all interfaces. Design does not add a separate “host” flag; can be added later if needed.

## Method design

### internal/server

| Symbol | Signature / Type | Responsibility |
|--------|------------------|----------------|
| **Config** | `struct { Addr string }` | Listen address (e.g. `:5678`). |
| **Server** | `struct { srv *http.Server }` (or equivalent) | Holds the HTTP server and runs it. |
| **New** | `func(cfg Config) *Server` | Builds `http.Server` with a `ServeMux`; registers `GET /healthz`, `GET /openapi.json`, `GET /swagger/` (and `/swagger/index.html` if needed). Returns Server that wraps it. |
| **Run** | `func(s *Server) error` | Starts the server (ListenAndServe or equivalent); blocks. Listens for OS signals (SIGINT, SIGTERM) and calls `Shutdown(context.WithTimeout(..., 10*time.Second))`. On shutdown, logs “server stopped” and returns nil or the shutdown error. |
| **healthzHandler** | `func(http.ResponseWriter, *http.Request)` | Writes `Content-Type: application/json`, status 200, body `{"status":"ok"}`. |
| **openAPIHandler** | `func(http.ResponseWriter, *http.Request)` | Writes `Content-Type: application/json` and the OpenAPI 3.0 spec bytes (inlined constant or embed). |
| **swaggerUIHandler** | `func(http.ResponseWriter, *http.Request)` | Writes `Content-Type: text/html` and an HTML page that loads Swagger UI from CDN (e.g. `https://unpkg.com/swagger-ui-dist@5/...`) and sets spec URL to `/openapi.json`. |

OpenAPI spec content: minimal valid OpenAPI 3.0 with `openapi: "3.0.x"`, `info.title` (e.g. "BuildMax API"), `paths./healthz.get` with summary/description and response 200. No auth. Spec can be a raw string or `//go:embed openapi.json` in the same package.

### internal/cmd (server command)

| Symbol | Signature | Responsibility |
|--------|------------|----------------|
| **newServerCommand** | `func() *cobra.Command` | Returns Cobra command: `Use: "server"`, Short/Long describe HTTP server and port. Flag `--port` (int, default 0). RunE: resolve port (flag → BUILDMAX_SERVER_PORT → 5678), build `server.Config{Addr: ":<port>"}`, create `server.New(cfg)`, log “server starting” with address, call `s.Run()`, log “server stopped” on return. |
| **resolveServerPort** | `func(cmd *cobra.Command) (int, error)` | Gets `--port`; if 0, parses `os.Getenv("BUILDMAX_SERVER_PORT")`; if still 0, return 5678. Returns error if env is set but not a valid number. |

## How they work together

1. User runs `buildmax server` (optionally `--port 8080` or with `BUILDMAX_SERVER_PORT=8080`).
2. Root command dispatches to the server command’s RunE. No TUI or prompt mode.
3. RunE resolves port, builds `server.Config{Addr: ":<port>"}`, calls `server.New(cfg)` which creates an `http.Server` with a mux registering `/healthz`, `/openapi.json`, and `/swagger/` (and `/swagger/index.html`).
4. RunE logs “server starting” with the chosen address (using existing slog; log.Init() already done in main).
5. `s.Run()` starts the server and blocks. Run installs a signal handler; on SIGINT/SIGTERM it calls `http.Server.Shutdown(ctx)` with a timeout (e.g. 10s), then returns.
6. RunE logs “server stopped” and returns the error from `Run()` (if any).
7. `GET /healthz` returns 200 and `{"status":"ok"}`. `GET /openapi.json` returns the static spec. `GET /swagger/` returns HTML that loads Swagger UI and fetches `/openapi.json`.

## Graceful shutdown

- On SIGINT (Ctrl+C) or SIGTERM, stop accepting new requests and shut down the HTTP server with a context timeout (e.g. 10 seconds). Log “server stopped” after shutdown. If shutdown times out, document that the process may exit with a non-zero code or log a warning; implementation may return the shutdown error from `Run()`.

## Dependencies

- **Standard library:** `net/http`, `context`, `os`, `os/signal`, `strconv`, etc. No new third-party HTTP framework required; use `net/http.ServeMux`.
- **Swagger UI:** No Go dependency. Serve a single HTML page that loads Swagger UI from a CDN (e.g. unpkg) and points to `/openapi.json`. Keeps the binary and build simple.
- **OpenAPI:** No code generation. Hand-written minimal JSON (or YAML converted to JSON) for `/healthz` only; can be a string constant or an embedded file in `internal/server`.

## Changes for review

- **New** `internal/server/server.go` — Config, Server, New(cfg), Run(s), healthzHandler, openAPIHandler, swaggerUIHandler; route registration in New.
- **New** `internal/server/openapi.go` or inline in server.go — OpenAPI 3.0 spec (string or embed) for `GET /healthz`; used by openAPIHandler.
- **New** `internal/cmd/server.go` — newServerCommand(), resolveServerPort(); register routes and run server.
- **Modified** `internal/cmd/root.go` — add `root.AddCommand(newServerCommand())`.
- **Optional** `internal/server/openapi.json` — if using `//go:embed`, one minimal OpenAPI 3.0 file; otherwise spec can live as a constant in server.go.

No changes to `cmd/buildmax/main.go` (log.Init() and root.Execute() remain); no changes to portal or other packages. Tests: add tests for `internal/server` (healthz response, openapi.json returns valid JSON, swagger UI returns HTML); add test for port resolution (flag, env, default) in cmd if desired. `go build ./...` and `go test ./...` must pass.
