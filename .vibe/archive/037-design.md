# Design 037: Workspace management

## Goal

Add workspace persistence (table + directory layout), ensure one "Default" workspace per user on first use, expose `GET /api/workspaces` (JWT-protected), and have the portal load workspaces from the API instead of mock data.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/store** | User + Workspace persistence | User, Workspace models; UserStore; workspace methods (EnsureDefaultWorkspace, ListByOwner); AutoMigrate |
| **internal/config** | Paths and env | DataDir, SessionsDir, LogsDir; add WorkspacesDir |
| **internal/server** | HTTP API and auth | Config (UserStore, JWTSecret); login handler; JWT parse helper; workspaces handler; OpenAPI spec |
| **internal/cmd** | Server bootstrap | Unchanged: opens store, passes to server |
| **portal (React)** | Workspace list from API | api.getWorkspaces(token); AppContent fetches workspaces when authenticated; remove mock usage for list |

## Structure

**Backend**

- `internal/store/store.go`
  - Add **Workspace** model: `ID` (string, primary key, e.g. UUID), `OwnerUserID` (string), `Name` (string), `CreatedAt` (int64). GORM tags; `json:"snake_case"` (id, owner_user_id, name, created_at). TableName `workspaces`.
  - In **New**: add `AutoMigrate(&Workspace{})` after User.
  - New methods on **Store**: `EnsureDefaultWorkspaceForUser(ctx, userID) error`; `ListWorkspacesByOwner(ctx, userID) ([]Workspace, error)`.
  - **EnsureDefaultWorkspaceForUser**: if no row with owner_user_id = userID exists, insert one (id = new UUID, name = "Default", owner_user_id = userID, created_at = now). Otherwise no-op.
  - **ListWorkspacesByOwner**: return all workspaces where owner_user_id = userID, order by created_at.

- `internal/config/config.go`
  - Add **WorkspacesDir() string**: return `filepath.Join(DataDir(), "workspaces")`. Optional: if env `BUILDMAX_WORKSPACES_DIR` is set, return that (filepath.Clean). Callers do not create the dir; server creates it when creating a workspace directory.

- `internal/server/`
  - **auth.go** (new): helper `userIDFromRequest(r *http.Request, jwtSecret string) (string, bool)`. Read `Authorization` header; if missing or not "Bearer <token>", return "", false. Parse JWT with jwtSecret, validate exp; extract sub claim as user id; return userID, true. On any parse/validation error return "", false.
  - **workspaces.go** (new): `workspacesHandler(w, r)`. Call `userIDFromRequest(r, s.cfg.JWTSecret)`; if !ok respond 401 JSON `{"error":"unauthorized"}`. Call `s.cfg.UserStore` — we need **WorkspaceStore** in server: either add **WorkspaceStore** interface in store and implement on Store, or add **Store** to server Config (Store has UserByEmail and workspace methods). Prefer adding **WorkspaceStore** interface and pass Store (which implements both). So: server.Config gets **WorkspaceStore store.WorkspaceStore** (interface with EnsureDefaultWorkspaceForUser and ListWorkspacesByOwner). workspacesHandler: EnsureDefaultWorkspaceForUser(ctx, userID); list = ListWorkspacesByOwner(ctx, userID). For each workspace, ensure directory exists: `dir := filepath.Join(config.WorkspacesDir(), workspace.ID)`; `os.MkdirAll(dir, 0755)`. Write response JSON array of `{ "id", "name", "owner_user_id", "created_at" }` (snake_case). So server.Config needs WorkspaceStore. cmd already has Store; Store will implement WorkspaceStore; cmd passes same st as UserStore and as WorkspaceStore (or we add WorkspaceStore to Config and cmd passes st for both).
  - **server.go**: Config add `WorkspaceStore store.WorkspaceStore`. Register `GET /api/workspaces`, s.workspacesHandler. Extend openAPISpec with GET /api/workspaces (security: Bearer JWT; response 200 array of workspace, 401 unauthorized).

**Portal**

- `portal/src/lib/api.ts`
  - Add **getWorkspaces(token: string): Promise<Workspace[]>**: `fetch(getApiBase() + '/api/workspaces', { headers: { 'Authorization': 'Bearer ' + token } })`; if !res.ok throw or return []; else return res.json(). Type **Workspace** from API: id (string), name (string), owner_user_id (string), created_at (number). Portal can use same shape as current Workspace (id, name) and ignore owner_user_id/created_at for display.
- `portal/src/App.tsx` (AppContent)
  - When token exists, instead of `listWorkspaces()` from mockData, call API: use state for workspaces (e.g. `workspaces`, `setWorkspaces`), useEffect that calls `getWorkspaces(token)` and sets result; while loading show a loading state or keep empty; on success set workspaces. Default workspace = workspaces[0]; redirect and list logic unchanged. Remove usage of `listWorkspaces`, `getWorkspaceById` for the list (getWorkspaceById for validation can become: find in the fetched workspaces list). Remove `createWorkspace` for "New workspace" in MVP or hide the button (task says MVP only Default — so we can hide "New workspace" when using API, or show disabled; spec says "remove or bypass mock workspace list for logged-in users" and "use first workspace as default". So: fetch workspaces from API; use that list; do not call createWorkspace — hide or disable "New workspace" for MVP).
- `portal/src/data/mockData.ts`
  - Keep mock data for projects/tasks/artifacts (still mock); only workspace list is from API when logged in. No change to mockData except we may export getWorkspaceById for non-auth fallback or remove from App usage.

## Method design

| Receiver / Package | Method / Function | Signature | Responsibility |
|--------------------|-------------------|-----------|----------------|
| **store.Store** | EnsureDefaultWorkspaceForUser | `(ctx context.Context, userID string) error` | If no workspace with owner_user_id = userID, insert one (id = uuid.NewString(), name = "Default", owner_user_id = userID, created_at = time.Now().Unix()). |
| **store.Store** | ListWorkspacesByOwner | `(ctx context.Context, userID string) ([]store.Workspace, error)` | Return workspaces where owner_user_id = userID, ordered by created_at. |
| **config** | WorkspacesDir | `() string` | Return DataDir()/workspaces, or BUILDMAX_WORKSPACES_DIR if set. |
| **server** | userIDFromRequest | `(r *http.Request, jwtSecret string) (userID string, ok bool)` | Authorization: Bearer <token>; parse and validate JWT; return sub claim and true, or "", false. |
| **server** | workspacesHandler | `(w http.ResponseWriter, r *http.Request)` | userIDFromRequest; if !ok return 401. EnsureDefaultWorkspaceForUser(ctx, userID); ListWorkspacesByOwner(ctx, userID). For each workspace MkdirAll(WorkspacesDir()/id). Write JSON array of workspace (snake_case). |
| **api (portal)** | getWorkspaces | `(token: string) => Promise<Workspace[]>` | GET /api/workspaces with Authorization: Bearer token; return JSON array or throw. |

## Interfaces

- **store.WorkspaceStore** (new interface in store package):
  - `EnsureDefaultWorkspaceForUser(ctx context.Context, userID string) error`
  - `ListWorkspacesByOwner(ctx context.Context, userID string) ([]Workspace, error)`
- **store.Store** implements both UserStore and WorkspaceStore (same struct, add the two methods).

## How they work together

**GET /api/workspaces flow**

1. Client sends GET with header `Authorization: Bearer <jwt>`.
2. workspacesHandler calls userIDFromRequest(r, cfg.JWTSecret). If !ok → 401.
3. Handler calls cfg.WorkspaceStore.EnsureDefaultWorkspaceForUser(ctx, userID) so user has at least one workspace.
4. Handler calls ListWorkspacesByOwner(ctx, userID).
5. For each workspace, handler ensures directory exists: `os.MkdirAll(filepath.Join(config.WorkspacesDir(), workspace.ID), 0755)`.
6. Handler responds 200 with JSON array of workspaces (id, name, owner_user_id, created_at).

**Portal load**

1. AppContent has token (useAuth). useEffect with [token]: if token, call getWorkspaces(token), set state workspaces.
2. defaultWorkspaceId = workspaces[0]?.id ?? "". If workspaces empty (loading or error), show loading or single empty state; once loaded, redirect to first workspace if needed.
3. getWorkspaceById(route.workspaceId) for validation: can be replaced by workspaces.find(w => w.id === route.workspaceId) from state.
4. "New workspace" button: hide or disable for MVP (only Default workspace supported).

**Server startup (cmd)**

- runServer already creates Store and passes UserStore. Add to server.Config: WorkspaceStore: st (same st implements WorkspaceStore). No new env or flags.

**Dependencies**

- server depends on store (UserStore, WorkspaceStore, Workspace type) and config (WorkspacesDir).
- store depends only on GORM/MySQL; no dependency on config (directory creation is in server).

## Data structures

- **Workspace** (store): ID string, OwnerUserID string, Name string, CreatedAt int64. JSON: id, owner_user_id, name, created_at.
- **API response GET /api/workspaces**: `[{ "id": "uuid", "name": "Default", "owner_user_id": "user-uuid", "created_at": 1234567890 }]`.
- **Portal Workspace** (types): already `{ id: string, name: string }`; API can return more fields; portal uses id and name only.

## OpenAPI

Add to openAPISpec paths:

- **GET /api/workspaces**
  - summary: List workspaces
  - description: Returns workspaces for the authenticated user. Creates a Default workspace if none exist. Requires Bearer JWT.
  - security: [{ bearerAuth: [] }]
  - responses: 200 — array of workspace (id, name, owner_user_id, created_at); 401 — unauthorized

Add components.securitySchemes.bearerAuth (type http, scheme bearer, bearerFormat JWT) and reference in path.

## Testing

- **internal/server**
  - **auth_test.go** or in workspaces_test.go: Test_userIDFromRequest — no token → false; invalid token → false; valid JWT with sub → userID, true. Use same JWT secret and signing as login.
  - **workspaces_test.go**: Test workspacesHandler with mock WorkspaceStore. Case 1: no Authorization → 401. Case 2: valid JWT, mock returns one workspace → 200, body is JSON array with one workspace. Case 3: valid JWT, EnsureDefaultWorkspaceForUser + ListWorkspacesByOwner return list → 200; verify MkdirAll is called (or test with real config.WorkspacesDir in temp dir). Prefer table-driven; mock WorkspaceStore that implements EnsureDefaultWorkspaceForUser and ListWorkspacesByOwner.
- **internal/store**
  - Test EnsureDefaultWorkspaceForUser: first call creates one row; second call does not duplicate. Test ListWorkspacesByOwner: returns only workspaces for that user. Use in-memory SQLite if available for isolation, or integration test with test DB.

## Changes for review

- **Modified** `internal/store/store.go` — Add Workspace struct and table; WorkspaceStore interface; Store implements it; New adds AutoMigrate(Workspace); EnsureDefaultWorkspaceForUser; ListWorkspacesByOwner. Use uuid (e.g. google/uuid or standard library) for new workspace id.
- **Modified** `internal/config/config.go` — Add WorkspacesDir().
- **New** `internal/server/auth.go` — userIDFromRequest(r, jwtSecret) using Authorization Bearer and jwt.ParseWithClaims.
- **New** `internal/server/workspaces.go` — workspacesHandler; response type with snake_case JSON.
- **Modified** `internal/server/server.go` — Config add WorkspaceStore; register GET /api/workspaces; openAPISpec add GET /api/workspaces and bearerAuth security scheme.
- **Modified** `internal/cmd/server.go` — Pass WorkspaceStore: st in server.Config (st implements both UserStore and WorkspaceStore).
- **Modified** `portal/src/lib/api.ts` — getWorkspaces(token). Optional: add ApiWorkspace type (id, name, owner_user_id, created_at).
- **Modified** `portal/src/App.tsx` — Fetch workspaces from API when authenticated (useEffect getWorkspaces(token)); use fetched list for workspaces state; defaultWorkspaceId from workspaces[0]; validate route.workspaceId against fetched list. Hide or disable "New workspace" for MVP.
- **New** `internal/server/workspaces_test.go` — Tests for 401 without token and 200 with valid token + mock store.
- **Optional** `internal/store/workspace_test.go` — Tests for EnsureDefaultWorkspaceForUser and ListWorkspacesByOwner.
