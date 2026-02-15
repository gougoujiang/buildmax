# Server

## Purpose

The `internal/server` package provides the HTTP API backend for the BuildMax Portal. It exposes REST endpoints for health, login (JWT), workspaces, projects, and tasks. The server is optional: it runs via `buildmax server` and requires store implementations and a JWT secret when login is used.

## Key Types and Interfaces

| Name | Kind | Role |
|------|------|------|
| **Server** | struct | Wraps `http.Server`; holds config and route handlers |
| **Config** | struct | Listen address, store interfaces (UserStore, WorkspaceStore, ProjectStore, TaskStore), JWT secret, CORS origin |
| **UserStore** | interface | From `internal/store`: `UserByEmail(ctx, email)` |
| **WorkspaceStore** | interface | From store: `EnsureDefaultWorkspaceForUser`, `ListWorkspacesByOwner` |
| **ProjectStore** | interface | From store: `GetProject`, `ListProjectsByWorkspace`, `CreateProject` |
| **TaskStore** | interface | From store: `ListTasksByProject`, `CreateTask` |

## Endpoints

- **GET /healthz** — Health check; returns 200.
- **POST /api/login** — Body: `{ "email": "..." }`. Returns JWT and user info when UserStore and JWTSecret are set.
- **GET /api/workspaces** — Requires `Authorization: Bearer <token>`. Returns workspaces for the authenticated user; uses WorkspaceStore.
- **GET /api/workspaces/:id/projects** — Lists projects for a workspace; requires auth.
- **POST /api/workspaces/:id/projects** — Creates a project; requires auth.
- **GET /api/projects/:id/tasks** — Lists tasks for a project; requires auth.
- **POST /api/projects/:id/tasks** — Creates a task; requires auth.
- **GET /openapi.json** — Serves the OpenAPI 3.0 spec for the API.

Auth is done via JWT: `userIDFromRequest` parses `Authorization: Bearer <token>` and validates the token; handlers that need auth call it and return 401 when missing or invalid.

## How It Works

1. **Creation**: `server.New(cfg)` or similar builds a `Server` with the given `Config`. Stores can be nil; endpoints that need them return 503 or 404 when not configured.
2. **Routing**: Handlers are registered on a single `http.ServeMux` (or equivalent). CORS middleware is applied when `Config.CORSOrigin` is set (e.g. `http://localhost:5173` for the Vite dev server).
3. **Run**: The server listens on `Config.Addr`, typically `:5678`. Graceful shutdown is handled via `ListenAndServe` and signal handling in the command that starts the server (e.g. `internal/cmd/server.go`).

## Dependencies

- **Uses**: `internal/store` (interfaces and optionally `*store.Store` for MySQL-backed impl), `github.com/golang-jwt/jwt/v5` for JWT.
- **Used by**: `cmd/buildmax` via the `buildmax server` subcommand, which wires config, store, and JWT secret from env or flags.

## Notes

- All API JSON uses snake_case per project convention.
- Table naming in the store is singular (`user`, `workspace`, `project`, `task`).
- See also: [Store](store.md), [CLI](cli.md), [Portal](portal.md).
