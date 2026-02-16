# Code Smell Proposal: internal/server

**Scope:** `internal/server` package (HTTP server and handlers for BuildMax portal backend).  
**Date:** 2026-02-15.

---

## Summary

The server package is structured clearly with store interfaces and table-driven tests. The main refactor opportunities are: reducing repeated response and auth logic, clarifying responsibility for workspace directory creation, tightening status codes, and moving large inline assets out of `server.go`. No single change is critical; priorities are marked below.

---

## Proposals

### 1. Extract JSON error response helper

**Location:** All handler files — `login.go`, `workspaces.go`, `projects.go`, `tasks.go`.

**Current state:** Every error path repeats the same pattern: set `Content-Type: application/json`, call `WriteHeader(status)`, then `w.Write([]byte(`{"error":"..."}`))`. This appears 30+ times across the package. Any change to the error envelope (e.g. adding a code or request id) would require edits in many places.

**Proposed change:** Add a small helper in `server.go` (or a shared `response.go`):

- `writeJSONError(w http.ResponseWriter, status int, message string)` that sets Content-Type, status, and `{"error": "<message>"}`.
- Replace each existing error-response block with a single call to this helper.

**Benefit:** Single place for the error format, less duplication, easier to extend (e.g. correlation id or error codes) and to keep consistent with AGENTS.md (snake_case, clear messages).

**Priority:** Medium.

---

### 2. Centralize auth and ownership checks for protected handlers

**Location:** `projects.go` (listProjectsHandler, createProjectHandler), `tasks.go` (listTasksHandler, createTaskHandler).

**Current state:** Each handler repeats: (1) get user from JWT → 401 if missing, (2) read path param → 400 if empty, (3) check workspace/project ownership → 403 if forbidden, (4) check store nil → 503. The sequence and status codes are similar but implemented in full in every handler, so adding a new protected resource multiplies the same boilerplate.

**Proposed change:**

- Introduce a helper that returns `(userID string, ok bool)` and writes 401 and returns false if auth fails; handlers use it and return immediately when !ok.
- Optionally, a helper that given `(r, userID, workspaceID)` checks ownership and writes 403 if not owned (using existing or new store method). Similarly for “resolve project and check workspace ownership” used by task handlers.
- Handlers then become: auth → path param → ownership/store checks → business logic → success response.

**Benefit:** Shorter handlers, consistent status codes and error messages, one place to adjust auth or ownership behavior.

**Priority:** Medium.

---

### 3. Move workspace directory creation out of HTTP handler

**Location:** `workspaces.go`, `workspacesHandler` (lines 47–51).

**Current state:** The handler calls `config.WorkspacesDir()` and `os.MkdirAll(dir, 0755)` for each workspace in the list. HTTP layer is responsible for both returning workspace data and ensuring filesystem directories exist, mixing transport with filesystem setup.

**Proposed change:** Move “ensure workspace directories exist” into a lower layer: e.g. in `store` when creating or listing workspaces, or a small `workspacefs`/`workspace` package that is called from a single place (e.g. when the server starts or when a workspace is created). The handler then only reads from the store and returns JSON. If directory creation must stay “on first list,” it could be a dedicated service function called from the handler instead of inline `os.MkdirAll` in the handler loop.

**Benefit:** Aligns with “one place per responsibility” (AGENTS.md); handler stays focused on HTTP; easier to test and to change where/when directories are created.

**Priority:** Low.

---

### 4. Remove redundant method check in login handler

**Location:** `login.go`, `loginHandler` (lines 45–49).

**Current state:** The handler checks `if r.Method != http.MethodPost` and returns 405. With Go 1.22+ `mux.HandleFunc("POST /api/login", ...)`, only POST requests are routed here, so this branch is dead code.

**Proposed change:** Remove the method check and the associated `Content-Type`/status/body block.

**Benefit:** Less noise, no misleading suggestion that other methods are handled here.

**Priority:** Low.

---

### 5. Move OpenAPI spec and Swagger UI out of server.go

**Location:** `server.go`, constants `openAPISpec` (~260 lines) and `swaggerUIHTML` (~15 lines).

**Current state:** Large string literals live in the same file as server setup and middleware. `server.go` becomes long and mixes wiring with static content; editing the API spec requires scrolling through the same file as server logic.

**Proposed change:** Use `go:embed` to load `openapi.json` and (if desired) a small `swagger.html` from a subdirectory (e.g. `internal/server/static/` or `internal/server/spec/`). Keep only the wiring in `server.go` (register routes, set handler for `/openapi.json` and `/swagger/`). Validate or parse the spec at init if needed.

**Benefit:** Shorter, clearer `server.go`; spec and UI can be edited or generated separately; aligns with common practice for embedded assets.

**Priority:** Low.

---

### 6. Add WorkspaceBelongsToUser (or equivalent) to avoid full list load

**Location:** `internal/server/projects.go`, `tasks.go` — `userOwnsWorkspace`; `internal/store/store.go`.

**Current state:** `userOwnsWorkspace(r, userID, workspaceID)` is implemented by calling `ListWorkspacesByOwner(ctx, userID)` and scanning the slice for `workspaceID`. For project and task handlers we only need a boolean “does this user own this workspace?”; loading the full list is unnecessary work and couples the server to the full list shape.

**Proposed change:** Add to the store interface and implementation something like `WorkspaceBelongsToUser(ctx context.Context, workspaceID, userID string) (bool, error)`. Implement it with a single query (e.g. EXISTS or COUNT). In the server, replace `userOwnsWorkspace` with a call to this method (or a thin wrapper that writes 403 and returns false). Keep `ListWorkspacesByOwner` for the workspaces list endpoint.

**Benefit:** Clearer intent, better performance when only ownership is needed, less data passed around.

**Priority:** Medium.

---

### 7. Consistent 503 for “store not configured” in task handlers

**Location:** `tasks.go`, `listTasksHandler` and `createTaskHandler` (ProjectStore nil vs TaskStore nil).

**Current state:** When `ProjectStore` is nil, the handlers return 500 and `"internal error"`. When `TaskStore` is nil, they return 503 and `"tasks not configured"`. Other handlers (workspaces, projects) use 503 and “not configured” when their store is nil. So task handlers are inconsistent: ProjectStore nil is treated as a generic internal error rather than “projects not configured.”

**Proposed change:** When `ProjectStore` is nil in the task handlers, return 503 and a message like `"projects not configured"` (or `"projects not available"`) so that “store not configured” is consistently 503 across the API.

**Benefit:** Predictable status codes for clients; aligns with the rest of the server.

**Priority:** Low.

---

### 8. Shared test helpers and mocks

**Location:** `workspaces_test.go` (signJWT, mockWorkspaceStore), `projects_test.go` (uses signJWT, mockProjectStore, mockWorkspaceStore), `tasks_test.go` (uses signJWT, mockTaskStore, mockProjectStore, mockWorkspaceStore).

**Current state:** `signJWT` is defined in `workspaces_test.go` and reused in other `_test.go` files; long comment in `projects_test.go` explains that. Mock stores are defined in different test files and duplicated where multiple tests need the same mocks (e.g. tasks_test needs both mockWorkspaceStore and mockProjectStore). This works but scatters test-only types and helpers.

**Proposed change:** Move `signJWT` and all mock store implementations into a single test helper file (e.g. `server_test_helpers.go` or `testing.go` in package server, or a `testdata`/helper used by tests). Remove the long comment in `projects_test.go`. Optionally add a short doc on how to add new handlers and tests.

**Benefit:** One place for auth and store mocks; easier to add new handlers and tests; no reliance on “which _test.go file defines signJWT.”

**Priority:** Low.

---

## Changes for review (summary)

| Item | Files / area | Change |
|------|----------------|--------|
| JSON error helper | server.go or response.go | Add `writeJSONError`; use in login, workspaces, projects, tasks |
| Auth/ownership helpers | server.go or auth.go, projects.go, tasks.go | Optional middleware or helpers for auth + path + ownership; reduce handler boilerplate |
| Workspace dir creation | workspaces.go, store or new pkg | Move MkdirAll out of handler into store or dedicated service |
| Login method check | login.go | Remove dead POST check |
| OpenAPI/Swagger | server.go, internal/server/static/ (or spec/) | go:embed openapi.json and Swagger HTML |
| WorkspaceBelongsToUser | store/store.go, server (projects.go, tasks.go) | New store method; replace userOwnsWorkspace implementation |
| 503 for ProjectStore nil in tasks | tasks.go | Return 503 + “not configured” when ProjectStore is nil |
| Test helpers | New server test helper file, projects_test.go | Centralize signJWT and mocks; drop long comment |

---

*Proposal only; no code changes were made. Implement refactors in separate tasks as needed.*
