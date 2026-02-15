# Design 038: Add project

## Goal

Add project table and persistence, expose `GET /api/workspaces/{workspace_id}/projects` and `POST /api/workspaces/{workspace_id}/projects` with JWT and workspace-ownership checks, and have the portal load and create projects via the API instead of mock data.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/store** | User, Workspace, Project persistence | User, Workspace, Project models; UserStore; WorkspaceStore; ProjectStore (list by workspace, create); AutoMigrate |
| **internal/server** | HTTP API and auth | Config (UserStore, WorkspaceStore, ProjectStore, JWTSecret); login, workspaces handlers; projects handlers (list, create); OpenAPI spec |
| **internal/cmd** | Server bootstrap | Pass Store as ProjectStore in server.Config |
| **portal (React)** | Projects from API + create flow | api.getProjects, api.createProject; AppContent fetches projects per workspace; WorkspaceHome uses API list + "New project" |

## Structure

**Backend**

- **internal/store/store.go**
  - Add **Project** model: `ID` (string, UUID, primary key), `WorkspaceID` (string, not null, index), `Name` (string, not null), `Description` (string, optional), `CreatedAt` (int64). GORM tags; `json:"snake_case"` (id, workspace_id, name, description, created_at). TableName `projects`.
  - In **New**: add `AutoMigrate(&Project{})` after Workspace.
  - New interface **ProjectStore**: `ListProjectsByWorkspace(ctx, workspaceID string) ([]Project, error)`; `CreateProject(ctx, workspaceID, name, description string) (*Project, error)`.
  - **Store** implements ProjectStore: add **ListProjectsByWorkspace** — return projects where workspace_id = workspaceID, order by created_at; **CreateProject** — insert row (id = uuid.New().String(), workspace_id, name, description, created_at = time.Now().Unix()), return the created project.

- **internal/server/**
  - **projects.go** (new): two handlers.
    - **listProjectsHandler(w, r)**: userIDFromRequest; if !ok → 401. workspaceID = r.PathValue("workspace_id"); if empty → 400. Call cfg.WorkspaceStore.ListWorkspacesByOwner(ctx, userID); if workspaceID not in that list (no workspace with id == workspaceID) → 403. Call cfg.ProjectStore.ListProjectsByWorkspace(ctx, workspaceID). Respond 200 with JSON array of projects (id, workspace_id, name, description, created_at).
    - **createProjectHandler(w, r)**: userIDFromRequest; if !ok → 401. workspaceID = r.PathValue("workspace_id"); if empty → 400. Same ownership check: ListWorkspacesByOwner, check workspaceID in list → 403 if not. Decode body JSON { "name", "description" }; if name missing or empty string → 400. Call cfg.ProjectStore.CreateProject(ctx, workspaceID, name, description). Respond 201 with created project JSON (same shape as list item).
  - **server.go**: Config add `ProjectStore store.ProjectStore`. Register `GET /api/workspaces/{workspace_id}/projects`, s.listProjectsHandler; `POST /api/workspaces/{workspace_id}/projects`, s.createProjectHandler. Extend openAPISpec with both paths (security: bearerAuth; list 200 + 401/403, create 201 + 400/401/403; request body for POST with name, optional description).

**Portal**

- **portal/src/lib/api.ts**
  - Add **ApiProject**: id (string), workspace_id (string), name (string), description (string), created_at (number).
  - **getProjects(workspaceId: string, token: string): Promise<ApiProject[]>**: GET `${getApiBase()}/api/workspaces/${workspaceId}/projects` with `Authorization: Bearer ${token}`; if !res.ok throw; return res.json().
  - **createProject(workspaceId: string, body: { name: string; description?: string }, token: string): Promise<ApiProject>**: POST same base path with JSON body, same auth; if !res.ok throw; return res.json().
- **portal/src/App.tsx** (AppContent)
  - Add state: projects (ApiProject[] or Project[]), loadingProjects, and optionally error. When route.workspaceId is set and token exists, useEffect([token, route.workspaceId]) calling getProjects(route.workspaceId, token), set projects (and loading/error). Map ApiProject → Project for existing components: id, workspaceId = workspace_id, name; status = "active", updatedAtLabel = e.g. "Created …" from created_at (format as needed).
  - Pass projects to WorkspaceHome and to route resolution (getProjectById becomes: find in projects by id). Remove or bypass mock listProjectsForWorkspace / getProjectById when API data is used (i.e. when authenticated, use API-backed projects for current workspace).
- **portal/src/pages/WorkspaceHome.tsx**
  - Add "New project" control (button or link). On click: prompt for name (and optional description) or open a small form/modal; call createProject(workspaceId, { name, description }, token); on success, either refetch projects (parent refetches or callback) or navigate to new project (navigate to project route with new id). Parent (App) holds token and can pass onCreateProject callback that refetches projects and optionally navigates.
- **portal/src/data/mockData.ts**
  - Keep MOCK_PROJECTS, listProjectsForWorkspace, getProjectById for fallback or non-workspace routes if any; App will prefer API when token and workspaceId are set. Alternatively, App always uses API for projects when logged in and overwrites list per workspace — then getProjectById in App can be implemented as projects.find(p => p.id === id) from state.

## Method design

| Receiver / Package | Method / Function | Signature | Responsibility |
|--------------------|-------------------|-----------|----------------|
| **store.Store** | ListProjectsByWorkspace | `(ctx context.Context, workspaceID string) ([]Project, error)` | Return projects where workspace_id = workspaceID, order by created_at. |
| **store.Store** | CreateProject | `(ctx context.Context, workspaceID, name, description string) (*Project, error)` | Insert Project (id=uuid, workspace_id, name, description, created_at=now); return pointer to created row. |
| **server** | listProjectsHandler | `(w http.ResponseWriter, r *http.Request)` | JWT → userID; PathValue("workspace_id"); ensure user owns workspace (ListWorkspacesByOwner, then check id in list); ListProjectsByWorkspace; 200 JSON array. 401/403/400 as specified. |
| **server** | createProjectHandler | `(w http.ResponseWriter, r *http.Request)` | JWT → userID; PathValue("workspace_id"); same ownership check; decode body { name, description }; validate name non-empty; CreateProject; 201 JSON body. 401/403/400 as specified. |
| **api (portal)** | getProjects | `(workspaceId: string, token: string) => Promise<ApiProject[]>` | GET /api/workspaces/{workspaceId}/projects with Bearer token; return JSON or throw. |
| **api (portal)** | createProject | `(workspaceId: string, body: { name: string; description?: string }, token: string) => Promise<ApiProject>` | POST /api/workspaces/{workspaceId}/projects with body and Bearer token; return JSON or throw. |

## Interfaces

- **store.ProjectStore** (new interface in store package):
  - `ListProjectsByWorkspace(ctx context.Context, workspaceID string) ([]Project, error)`
  - `CreateProject(ctx context.Context, workspaceID, name, description string) (*Project, error)`
- **store.Store** implements UserStore, WorkspaceStore, and ProjectStore (add the two project methods).

## How they work together

**GET /api/workspaces/{workspace_id}/projects**

1. Client sends GET with path and `Authorization: Bearer <jwt>`.
2. listProjectsHandler: userIDFromRequest(r, cfg.JWTSecret) → userID; if !ok → 401.
3. workspaceID = r.PathValue("workspace_id"); if workspaceID == "" → 400.
4. ListWorkspacesByOwner(ctx, userID). If no workspace in list has id == workspaceID → 403.
5. ListProjectsByWorkspace(ctx, workspaceID) → list. Respond 200 with JSON array (id, workspace_id, name, description, created_at).

**POST /api/workspaces/{workspace_id}/projects**

1. Client sends POST with path, body { "name": "…", "description": "…" }, Bearer JWT.
2. createProjectHandler: same JWT and workspace_id extraction and ownership check; 401/403/400 as above.
3. Decode JSON body; if name missing or empty → 400.
4. CreateProject(ctx, workspaceID, name, description) → project. Respond 201 with project JSON.

**Portal**

1. AppContent: when token and route.workspaceId are set, useEffect fetches getProjects(route.workspaceId, token), stores in state, maps to Project (id, workspaceId, name, status, updatedAtLabel).
2. WorkspaceHome receives projects and renders list; getProjectById(route.projectId) for project/task/artifact routes is projects.find(p => p.id === route.projectId).
3. "New project" on WorkspaceHome: user enters name (and optional description); createProject(workspaceId, { name, description }, token); on success, parent refetches projects (or add new project to state) and optionally navigate to project page.

**Server startup (cmd)**

- runServer already passes Store as UserStore and WorkspaceStore. Add ProjectStore: st to server.Config (same st implements ProjectStore).

## Data structures

- **Project** (store): ID string, WorkspaceID string, Name string, Description string, CreatedAt int64. JSON: id, workspace_id, name, description, created_at.
- **API response** (list and create): single object or array of `{ "id", "workspace_id", "name", "description", "created_at" }`.
- **Portal ApiProject**: id, workspace_id, name, description, created_at. Map to **Project** (types): id, workspaceId, name, status ("active"), updatedAtLabel (e.g. from created_at).

## OpenAPI

Add to openAPISpec paths:

- **GET /api/workspaces/{workspace_id}/projects**
  - summary: List projects
  - description: Returns projects for the given workspace. Caller must own the workspace. Requires Bearer JWT.
  - security: [{ bearerAuth: [] }]
  - parameters: path workspace_id (required, string)
  - responses: 200 — array of project (id, workspace_id, name, description, created_at); 401 — unauthorized; 403 — forbidden (workspace not owned)
- **POST /api/workspaces/{workspace_id}/projects**
  - summary: Create project
  - description: Creates a project under the given workspace. Caller must own the workspace. Requires Bearer JWT.
  - security: [{ bearerAuth: [] }]
  - parameters: path workspace_id (required, string)
  - requestBody: application/json { name (required), description (optional) }
  - responses: 201 — created project object; 400 — bad request (name missing); 401 — unauthorized; 403 — forbidden

Use existing components.securitySchemes.bearerAuth.

## Testing

- **internal/server/projects_test.go**
  - Table-driven: listProjectsHandler — (1) no Authorization → 401; (2) valid JWT, workspace not in user's list → 403; (3) valid JWT, workspace owned, empty list → 200 []; (4) valid JWT, workspace owned, store returns one project → 200 with one item. createProjectHandler — (1) no token → 401; (2) workspace not owned → 403; (3) body missing name or name empty → 400; (4) valid body and owned workspace → 201 and body contains id, workspace_id, name, description, created_at. Use mock ProjectStore and WorkspaceStore (e.g. in-memory or interface mocks).
- **internal/store**
  - Test ListProjectsByWorkspace returns only projects for that workspace; test CreateProject inserts and returns project with id and created_at set. Use test DB or in-memory SQLite if available.

## Path routing (Go 1.22)

Use Go 1.22 ServeMux path pattern: `GET /api/workspaces/{workspace_id}/projects` and `POST /api/workspaces/{workspace_id}/projects`. Read workspace_id in handlers via `r.PathValue("workspace_id")`.

## Changes for review

- **Modified** `internal/store/store.go` — Add Project struct and table; ProjectStore interface; Store implements it; New adds AutoMigrate(Project); ListProjectsByWorkspace; CreateProject.
- **New** `internal/server/projects.go` — listProjectsHandler; createProjectHandler; shared ownership check (userID + workspaceID → ListWorkspacesByOwner, then check workspaceID in list); response types with snake_case JSON.
- **Modified** `internal/server/server.go` — Config add ProjectStore; register GET and POST `/api/workspaces/{workspace_id}/projects`; openAPISpec add both paths and request/response schemas.
- **Modified** `internal/cmd/server.go` (or equivalent) — Pass ProjectStore: st in server.Config.
- **Modified** `portal/src/lib/api.ts` — ApiProject type; getProjects(workspaceId, token); createProject(workspaceId, body, token).
- **Modified** `portal/src/App.tsx` — Fetch projects for current workspace from API when authenticated; map ApiProject → Project; use fetched list for WorkspaceHome and getProjectById; pass callback or refetch so WorkspaceHome can trigger refresh after create.
- **Modified** `portal/src/pages/WorkspaceHome.tsx` — Add "New project" button/form; on submit call createProject; on success refetch projects (via callback from App) and optionally navigate to new project.
- **New** `internal/server/projects_test.go` — Tests for 401, 403, 400, 200, 201 as above.
- **Optional** `internal/store/project_test.go` or store_test.go — Tests for ListProjectsByWorkspace and CreateProject.
