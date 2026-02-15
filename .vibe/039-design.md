# Design 039: Add task

## Goal

Add task table and persistence, expose `GET /api/projects/{project_id}/tasks` and `POST /api/projects/{project_id}/tasks` with JWT and project ownership checks (user must own the project’s workspace), and have the portal load and create tasks via the API instead of mock data. Task execution is out of scope.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/store** | User, Workspace, Project, Task persistence | Task model; ProjectStore extended with GetProject; TaskStore (list by project, create); AutoMigrate Task |
| **internal/server** | HTTP API and auth | Config adds TaskStore; tasks handlers (list, create); project resolution via GetProject + userOwnsWorkspace; OpenAPI task paths |
| **internal/cmd** | Server bootstrap | Pass Store as TaskStore in server.Config |
| **portal (React)** | Tasks from API + create flow | api.getTasks, api.createTask, apiTaskToTask; App fetches tasks for current project; ProjectDashboard uses API tasks + "New task" |

## Structure

**Backend**

- **internal/store/store.go**
  - Add **Task** model: `ID` (uint, primary key, auto-increment), `TaskID` (string, UUID, unique, not null), `ProjectID` (string, not null, index), `Status` (string, e.g. PENDING/RUNNING/SUCCEEDED/FAILED/CANCELED), `Input` (string, text), `Output` (string, text, nullable), `CreatedBy` (string, not null), `CreatedAt` (int64), `StartedAt` (int64, nullable), `EndedAt` (int64, nullable), `ErrorMessage` (string, nullable). GORM tags; `json:"snake_case"` (id omitted or internal-only; API exposes task_id as id: task_id, project_id, status, input, output, created_by, created_at, started_at, ended_at, error_message). TableName `tasks`.
  - Extend **ProjectStore** interface: add `GetProject(ctx context.Context, projectID string) (*Project, error)`. Store implements it: First by id; return (nil, nil) when not found; otherwise return project.
  - In **New**: add `AutoMigrate(&Task{})` after Project.
  - New interface **TaskStore**: `ListTasksByProject(ctx context.Context, projectID string) ([]Task, error)`; `CreateTask(ctx context.Context, projectID, input, createdBy string) (*Task, error)`.
  - **Store** implements TaskStore: **ListTasksByProject** — return tasks where project_id = projectID, order by created_at; **CreateTask** — insert row (task_id = uuid.New().String(), project_id, status = "PENDING", input, created_by, created_at = time.Now().Unix(); output, started_at, ended_at, error_message null/zero), return the created task.

- **internal/server/tasks.go** (new)
  - **listTasksHandler(w, r)**: userIDFromRequest; if !ok → 401. projectID = r.PathValue("project_id"); if empty → 400. cfg.ProjectStore.GetProject(ctx, projectID); if project == nil → 404. userOwnsWorkspace(r, userID, project.WorkspaceID) else → 403. If cfg.TaskStore == nil → 503. ListTasksByProject(ctx, projectID). Respond 200 with JSON array; each item: id = task.TaskID, project_id, status, input, output, created_by, created_at, started_at, ended_at, error_message.
  - **createTaskHandler(w, r)**: same JWT and project_id extraction; same GetProject + userOwnsWorkspace → 404/403. Decode body `{ "input": "…" }`; if input missing or empty → 400. CreateTask(ctx, projectID, input, userID). Respond 201 with created task JSON (same shape; id = task_id).
  - Response type **TaskResponse** with snake_case JSON: id (from task_id), project_id, status, input, output, created_by, created_at, started_at, ended_at, error_message. Helper **taskToResponse(t store.Task) TaskResponse**.

- **internal/server/server.go**
  - Config add `TaskStore store.TaskStore`.
  - Register `GET /api/projects/{project_id}/tasks`, s.listTasksHandler; `POST /api/projects/{project_id}/tasks`, s.createTaskHandler.
  - Extend openAPISpec: add path `/api/projects/{project_id}/tasks` with get (summary: List tasks; description: Returns tasks for the project; caller must own the project’s workspace; 200 array, 401/403/404/503) and post (summary: Create task; body { input }; 201 created task, 400/401/403/404/503).

**Portal**

- **portal/src/lib/api.ts**
  - Add **ApiTask**: id (string), project_id (string), status (string), input (string), output (string | null), created_by (string), created_at (number), started_at (number | null), ended_at (number | null), error_message (string | null).
  - **getTasks(projectId: string, token: string): Promise<ApiTask[]>**: GET `${getApiBase()}/api/projects/${projectId}/tasks` with `Authorization: Bearer ${token}`; if !res.ok throw; return res.json().
  - **createTask(projectId: string, body: { input: string }, token: string): Promise<ApiTask>**: POST same base path with JSON body, same auth; if !res.ok throw; return res.json().
  - **apiTaskToTask(api: ApiTask): Task**: Map to portal Task type: id = api.id, projectId = api.project_id, title = truncate(api.input) or api.input, status = mapStatus(api.status) (PENDING/RUNNING → "running", SUCCEEDED → "success", FAILED → "failed", CANCELED → "canceled"), timeLabel from api.created_at or api.ended_at (e.g. "Created today" / "Ended …"), summary = api.output || api.input or truncated input.

- **portal/src/App.tsx**
  - Add state: tasks (Task[]), loadingTasks (boolean). When route has project (route.name === "project" and route.projectId) and token exists, useEffect([token, route.projectId]) calling getTasks(route.projectId, token), set tasks (and loading/error). Map ApiTask → Task via apiTaskToTask. Pass tasks to ProjectDashboard when rendering project page. Add refetchTasks callback (same as refetch but for getTasks(route.projectId, token)); pass to ProjectDashboard so "New task" can refetch after create.
  - In renderPage, for fallbackProject: pass tasks={api-backed tasks for this project} (from state) instead of listTasksForProject(project.id). When loadingTasks, can pass [] or show loading in dashboard.
  - getTaskById for task detail route: when switching to API-backed tasks, getTaskById(projectId, taskId) can be tasks.find(t => t.id === taskId) when tasks are for that project; else keep mock getTaskById for now for task detail page (or derive from fetched tasks). Spec says "load tasks from the API" for project view; task detail can still resolve from the same tasks list (tasks state is for current project).

- **portal/src/pages/ProjectDashboard.tsx**
  - Add "New task" control (e.g. button or inline form with input). On submit: call createTask(project.id, { input: userInput }, token); on success, call refetchTasks() (passed from App) and optionally clear input or navigate to the new task. Token and refetchTasks passed as props from App.

## Method design

| Receiver / Package | Method / Function | Signature | Responsibility |
|--------------------|-------------------|-----------|----------------|
| **store.Store** | GetProject | `(ctx context.Context, projectID string) (*Project, error)` | Return project by id; (nil, nil) when not found. |
| **store.Store** | ListTasksByProject | `(ctx context.Context, projectID string) ([]Task, error)` | Return tasks where project_id = projectID, order by created_at. |
| **store.Store** | CreateTask | `(ctx context.Context, projectID, input, createdBy string) (*Task, error)` | Insert Task (task_id=uuid, project_id, status=PENDING, input, created_by, created_at=now); return created task. |
| **server** | listTasksHandler | `(w http.ResponseWriter, r *http.Request)` | JWT → userID; PathValue("project_id"); GetProject → 404 if nil; userOwnsWorkspace(userID, project.WorkspaceID) → 403; ListTasksByProject; 200 JSON array. 401/403/404/503. |
| **server** | createTaskHandler | `(w http.ResponseWriter, r *http.Request)` | JWT → userID; PathValue("project_id"); GetProject + ownership; decode body { input }; validate input non-empty; CreateTask; 201 JSON. 400/401/403/404/503. |
| **api (portal)** | getTasks | `(projectId: string, token: string) => Promise<ApiTask[]>` | GET /api/projects/{projectId}/tasks with Bearer token; return JSON or throw. |
| **api (portal)** | createTask | `(projectId: string, body: { input: string }, token: string) => Promise<ApiTask>` | POST /api/projects/{projectId}/tasks with body and Bearer token; return JSON or throw. |
| **api (portal)** | apiTaskToTask | `(api: ApiTask) => Task` | Map API task to UI Task (id, projectId, title, status, timeLabel, summary). |

## Interfaces

- **store.ProjectStore** (extended): add `GetProject(ctx context.Context, projectID string) (*Project, error)`.
- **store.TaskStore** (new): `ListTasksByProject(ctx context.Context, projectID string) ([]Task, error)`; `CreateTask(ctx context.Context, projectID, input, createdBy string) (*Task, error)`.
- **store.Store** implements ProjectStore (add GetProject) and TaskStore (add ListTasksByProject, CreateTask).

## How they work together

**GET /api/projects/{project_id}/tasks**

1. Client sends GET with path and `Authorization: Bearer <jwt>`.
2. listTasksHandler: userIDFromRequest(r, cfg.JWTSecret) → userID; if !ok → 401.
3. projectID = r.PathValue("project_id"); if projectID == "" → 400.
4. GetProject(ctx, projectID) → project; if project == nil → 404.
5. userOwnsWorkspace(r, userID, project.WorkspaceID); if false → 403.
6. TaskStore nil → 503. ListTasksByProject(ctx, projectID) → list. Respond 200 with JSON array (id = task_id, project_id, status, input, output, created_by, created_at, started_at, ended_at, error_message).

**POST /api/projects/{project_id}/tasks**

1. Client sends POST with path, body { "input": "…" }, Bearer JWT.
2. createTaskHandler: same JWT and project_id; GetProject → 404 if nil; userOwnsWorkspace → 403; decode body; input missing/empty → 400.
3. CreateTask(ctx, projectID, input, userID) → task. Respond 201 with task JSON (same shape).

**Portal**

1. AppContent: when token and route.projectId (project page) are set, useEffect fetches getTasks(route.projectId, token), stores in state as tasks, maps via apiTaskToTask for display.
2. ProjectDashboard receives tasks (and token, refetchTasks). Renders task list from props. "New task": user enters input; createTask(project.id, { input }, token); on success refetchTasks() so list updates.
3. getTaskById(projectId, taskId) for task detail: when tasks state holds the current project’s tasks, can use tasks.find(t => t.id === taskId); App can pass tasks and project so TaskDetail receives task from the list, or App resolves task from tasks state when route is "task".

## Data structures

- **Task** (store): ID uint, TaskID string (UUID), ProjectID string, Status string, Input string, Output string, CreatedBy string, CreatedAt int64, StartedAt int64, EndedAt int64, ErrorMessage string. JSON tags snake_case; API response uses task_id as "id".
- **TaskResponse** (server): Id (json "id"), ProjectID, Status, Input, Output, CreatedBy, CreatedAt, StartedAt, EndedAt, ErrorMessage — all snake_case.
- **ApiTask** (portal): id, project_id, status, input, output, created_by, created_at, started_at, ended_at, error_message. Map to **Task** (types): id, projectId, title (from input), status (mapped), timeLabel (from created_at/ended_at), summary (from output or input).

## OpenAPI

Add to openAPISpec paths:

- **GET /api/projects/{project_id}/tasks**
  - summary: List tasks
  - description: Returns tasks for the project. Caller must own the project’s workspace. Requires Bearer JWT.
  - security: [{ bearerAuth: [] }]
  - parameters: path project_id (required, string)
  - responses: 200 — array of task (id, project_id, status, input, output, created_by, created_at, started_at, ended_at, error_message); 401 — unauthorized; 403 — forbidden; 404 — project not found; 503 — tasks not configured
- **POST /api/projects/{project_id}/tasks**
  - summary: Create task
  - description: Creates a task under the project. Caller must own the project’s workspace. Requires Bearer JWT.
  - security: [{ bearerAuth: [] }]
  - parameters: path project_id (required, string)
  - requestBody: application/json { input (required) }
  - responses: 201 — created task object; 400 — bad request (input missing); 401 — unauthorized; 403 — forbidden; 404 — project not found; 503 — tasks not configured

Use existing components.securitySchemes.bearerAuth.

## Testing

- **internal/server/tasks_test.go**
  - Mock ProjectStore with GetProject (return project or nil). Mock TaskStore with ListTasksByProject and CreateTask. Table-driven: listTasksHandler — no auth → 401; valid JWT, project not found (GetProject nil) → 404; valid JWT, project exists but user does not own workspace → 403; valid JWT, owned project, empty list → 200 []; valid JWT, owned project, store returns tasks → 200 with items. createTaskHandler — no auth → 401; project not found → 404; not owned → 403; body missing input or empty → 400; valid body and owned project → 201 and body contains id (task_id), status PENDING, created_by, input.
- **internal/store**
  - Test GetProject returns project by id and (nil, nil) when not found. Test ListTasksByProject returns only tasks for that project; test CreateTask inserts task with PENDING, task_id set, created_at set. Use test DB or in-memory if available.

## Path routing (Go 1.22)

Use `GET /api/projects/{project_id}/tasks` and `POST /api/projects/{project_id}/tasks`. Read project_id in handlers via `r.PathValue("project_id")`.

## Changes for review

- **Modified** `internal/store/store.go` — Add Task struct and table; ProjectStore extended with GetProject; TaskStore interface and Store implementation (ListTasksByProject, CreateTask); New adds AutoMigrate(Task).
- **New** `internal/server/tasks.go` — listTasksHandler; createTaskHandler; TaskResponse and taskToResponse; project resolution via GetProject + userOwnsWorkspace.
- **Modified** `internal/server/server.go` — Config add TaskStore; register GET and POST `/api/projects/{project_id}/tasks`; openAPISpec add both paths and request/response schemas.
- **Modified** `internal/cmd` (server bootstrap) — Pass TaskStore: st in server.Config.
- **Modified** `portal/src/lib/api.ts` — ApiTask type; getTasks(projectId, token); createTask(projectId, body, token); apiTaskToTask(api).
- **Modified** `portal/src/App.tsx` — Fetch tasks for current project when route has projectId (useEffect); tasks state and loadingTasks; refetchTasks callback; pass tasks and refetchTasks to ProjectDashboard; resolve task for task detail from tasks when available.
- **Modified** `portal/src/pages/ProjectDashboard.tsx` — Accept token and refetchTasks props; add "New task" UI (input + submit); on submit call createTask(project.id, { input }, token), then refetchTasks().
- **New** `internal/server/tasks_test.go` — Tests for 401, 404, 403, 400, 200, 201 as above; mock ProjectStore (with GetProject) and TaskStore.
- **Optional** `internal/store` — Unit tests for GetProject, ListTasksByProject, CreateTask.
