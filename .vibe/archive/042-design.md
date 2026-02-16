# Design 042: Tasks belong to workspace

## Goal

Move task ownership from project to workspace. Add `workspace_id` to the task model, make `project_id` nullable. Replace project-scoped task API endpoints with workspace-scoped ones. Update portal to use the new endpoints.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/store** | Task model and persistence | Task struct (add `workspace_id`, nullable `project_id`); TaskStore interface (new signatures); Store implementation |
| **internal/server** | HTTP API handlers and routing | Workspace-scoped task handlers; remove project-scoped task routes; TaskResponse and OpenAPI |
| **portal/src/lib** | API client and types | `getTasks`, `createTask` (new URLs); `ApiTask` and `Task` type (nullable `project_id`) |
| **portal/src** | App wiring | `App.tsx`, `ProjectDashboard.tsx` — call sites use workspace-scoped API |

## Structure

### internal/store/store.go

**Task struct** — change two fields:

```go
type Task struct {
    ID           uint    `gorm:"primaryKey;autoIncrement" json:"-"`
    TaskID       string  `gorm:"type:varchar(36);uniqueIndex;not null" json:"task_id"`
    WorkspaceID  string  `gorm:"type:varchar(36);not null;index" json:"workspace_id"`    // NEW
    ProjectID    *string `gorm:"type:varchar(36);index" json:"project_id,omitempty"`      // CHANGED: nullable
    Status       string  `gorm:"type:varchar(32);not null" json:"status"`
    Input        string  `gorm:"type:text;not null" json:"input"`
    Output       *string `gorm:"type:text" json:"output,omitempty"`
    CreatedBy    string  `gorm:"type:varchar(36);not null" json:"created_by"`
    CreatedAt    int64   `gorm:"autoCreateTime" json:"created_at"`
    StartedAt    *int64  `gorm:"" json:"started_at,omitempty"`
    EndedAt      *int64  `gorm:"" json:"ended_at,omitempty"`
    ErrorMessage *string `gorm:"type:text" json:"error_message,omitempty"`
}
```

- `WorkspaceID` — new required field, indexed.
- `ProjectID` — changes from `string` (NOT NULL) to `*string` (nullable). GORM tag removes `not null`.

AutoMigrate handles adding the `workspace_id` column and making `project_id` nullable. Existing rows with `project_id` set will keep their value. Backfill of `workspace_id` happens after migrate (see below).

**TaskStore interface** — replace both methods:

```go
type TaskStore interface {
    ListTasksByWorkspace(ctx context.Context, workspaceID string, projectID *string) ([]Task, error)
    CreateTask(ctx context.Context, workspaceID string, projectID *string, input, createdBy string) (*Task, error)
}
```

- `ListTasksByWorkspace` — WHERE `workspace_id = ?`; if `projectID != nil`, also AND `project_id = ?`.
- `CreateTask` — inserts with `workspace_id` (required) and `project_id` (nullable).

**Store implementation** — replace `ListTasksByProject` and old `CreateTask`:

- `ListTasksByWorkspace(ctx, workspaceID, projectID)`:
  ```
  query := db.Where("workspace_id = ?", workspaceID)
  if projectID != nil {
      query = query.Where("project_id = ?", *projectID)
  }
  query.Order("created_at ASC").Find(&list)
  ```
- `CreateTask(ctx, workspaceID, projectID, input, createdBy)`:
  ```
  t := &Task{
      TaskID:      newUUID(),
      WorkspaceID: workspaceID,
      ProjectID:   projectID,
      Status:      "PENDING",
      Input:       input,
      CreatedBy:   createdBy,
      CreatedAt:   time.Now().Unix(),
  }
  db.Create(t)
  ```

**Backfill** — add `BackfillTaskWorkspaceID(ctx)` method on Store. Called once after `New()` in server bootstrap:

```go
func (s *Store) BackfillTaskWorkspaceID(ctx context.Context) error {
    return s.db.WithContext(ctx).Exec(`
        UPDATE task t
        JOIN project p ON t.project_id = p.project_id
        SET t.workspace_id = p.workspace_id
        WHERE t.workspace_id = '' OR t.workspace_id IS NULL
    `).Error
}
```

This fills `workspace_id` for any existing task rows that have a `project_id` but no `workspace_id` yet. Idempotent — safe to run on every startup.

### internal/server/tasks.go

**Remove** `listTasksHandler` and `createTaskHandler` (project-scoped).

**Add** two new handlers:

- `listWorkspaceTasksHandler(w, r)`:
  1. `requireAuth` → `userID`; 401 if not ok.
  2. `workspace_id` from `r.PathValue("workspace_id")`; 400 if empty.
  3. `userOwnsWorkspace(r, userID, workspaceID)` → 403 if not owned.
  4. `cfg.TaskStore` nil → 503.
  5. Read optional `project_id` from `r.URL.Query().Get("project_id")`. If non-empty: validate project exists (`cfg.ProjectStore.GetProject`) → 404 if nil; validate `project.WorkspaceID == workspaceID` → 400 if mismatch. Pass `&projectID` to store. If empty: pass `nil`.
  6. `ListTasksByWorkspace(ctx, workspaceID, projectIDPtr)` → respond 200 JSON array.

- `createWorkspaceTaskHandler(w, r)`:
  1. `requireAuth` → `userID`; 401 if not ok.
  2. `workspace_id` from path; 400 if empty.
  3. `userOwnsWorkspace` → 403 if not owned.
  4. `cfg.TaskStore` nil → 503.
  5. Decode body `{ "input": "…", "project_id": "optional" }`.
  6. `input` empty → 400.
  7. If `project_id` in body is non-empty: validate project exists → 404 if nil; validate `project.WorkspaceID == workspaceID` → 400 ("project does not belong to workspace"). Pass `&projectID`.
  8. `CreateTask(ctx, workspaceID, projectIDPtr, input, userID)` → respond 201.

**TaskResponse** — add `workspace_id`, make `project_id` a pointer:

```go
type TaskResponse struct {
    ID           string  `json:"id"`
    WorkspaceID  string  `json:"workspace_id"`
    ProjectID    *string `json:"project_id,omitempty"`
    Status       string  `json:"status"`
    Input        string  `json:"input"`
    Output       *string `json:"output,omitempty"`
    CreatedBy    string  `json:"created_by"`
    CreatedAt    int64   `json:"created_at"`
    StartedAt    *int64  `json:"started_at,omitempty"`
    EndedAt      *int64  `json:"ended_at,omitempty"`
    ErrorMessage *string `json:"error_message,omitempty"`
}
```

**createTaskRequest** — add optional `project_id`:

```go
type createTaskRequest struct {
    Input     string `json:"input"`
    ProjectID string `json:"project_id"`
}
```

**taskToResponse** — update to map new fields.

### internal/server/server.go

**Routes** — remove old, add new:

```go
// Remove:
// mux.HandleFunc("GET /api/projects/{project_id}/tasks", s.listTasksHandler)
// mux.HandleFunc("POST /api/projects/{project_id}/tasks", s.createTaskHandler)

// Add:
mux.HandleFunc("GET /api/workspaces/{workspace_id}/tasks", s.listWorkspaceTasksHandler)
mux.HandleFunc("POST /api/workspaces/{workspace_id}/tasks", s.createWorkspaceTaskHandler)
```

### internal/cmd/server.go

After `store.New(ctx, dsn)`, call `st.BackfillTaskWorkspaceID(ctx)`:

```go
st, err := store.New(ctx, dsn)
if err != nil { ... }
if err := st.BackfillTaskWorkspaceID(ctx); err != nil {
    slog.Warn("backfill task workspace_id", "err", err)
}
```

Non-fatal — log warning on error, don't block server start.

### internal/server/static/openapi.json

- **Remove** the `/api/projects/{project_id}/tasks` path entry (both GET and POST).
- **Add** `/api/workspaces/{workspace_id}/tasks` with:
  - **GET**: summary "List tasks"; params: path `workspace_id` (required), query `project_id` (optional); responses 200 (array of task), 401, 403, 503. Task schema: `id`, `workspace_id`, `project_id` (nullable), `status`, `input`, `output`, `created_by`, `created_at`, `started_at`, `ended_at`, `error_message`.
  - **POST**: summary "Create task"; params: path `workspace_id` (required); body `{ input (required), project_id (optional) }`; responses 201, 400, 401, 403, 503.

### portal/src/lib/types.ts

**Task** — make `projectId` optional:

```typescript
export interface Task {
  id: string
  projectId?: string       // CHANGED: optional
  title: string
  status: "running" | "success" | "failed" | "canceled"
  timeLabel: string
  summary: string
}
```

### portal/src/lib/api.ts

**ApiTask** — add `workspace_id`, make `project_id` nullable:

```typescript
export interface ApiTask {
  id: string
  workspace_id: string
  project_id: string | null    // CHANGED: nullable
  status: string
  input: string
  output: string | null
  created_by: string
  created_at: number
  started_at: number | null
  ended_at: number | null
  error_message: string | null
}
```

**getTasks** — change signature and URL:

```typescript
export async function getTasks(
  workspaceId: string,
  token: string,
  projectId?: string
): Promise<ApiTask[]> {
  let url = `${getApiBase()}/api/workspaces/${workspaceId}/tasks`
  if (projectId) {
    url += `?project_id=${encodeURIComponent(projectId)}`
  }
  // ... same fetch pattern ...
}
```

**createTask** — change signature and URL:

```typescript
export async function createTask(
  workspaceId: string,
  body: { input: string; project_id?: string },
  token: string
): Promise<ApiTask> {
  const res = await fetch(`${getApiBase()}/api/workspaces/${workspaceId}/tasks`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
    body: JSON.stringify(body),
  })
  // ... same error handling ...
}
```

**apiTaskToTask** — handle nullable `project_id`:

```typescript
export function apiTaskToTask(api: ApiTask): Task {
  return {
    id: api.id,
    projectId: api.project_id ?? undefined,
    // ... rest unchanged ...
  }
}
```

### portal/src/App.tsx

**refetchTasks** and the useEffect that fetches tasks — change from `getTasks(route.projectId, token)` to `getTasks(route.workspaceId, token, route.projectId)`:

```typescript
const refetchTasks = useCallback(() => {
  if (!token || !route.projectId) return
  setLoadingTasks(true)
  getTasks(route.workspaceId, token, route.projectId)
    .then((list) => setTasks(list.map(apiTaskToTask)))
    .catch(() => setTasks([]))
    .finally(() => setLoadingTasks(false))
}, [token, route.workspaceId, route.projectId])
```

Same change in the useEffect below it.

### portal/src/pages/ProjectDashboard.tsx

**handleRun** — change `createTask` call:

```typescript
await createTask(workspaceId, { input, project_id: project.id }, token)
```

The `workspaceId` prop is already passed to `ProjectDashboard`.

## Method design

| Receiver / Package | Method / Function | Signature | Responsibility |
|--------------------|-------------------|-----------|----------------|
| **store.Store** | BackfillTaskWorkspaceID | `(ctx context.Context) error` | Raw SQL: UPDATE task JOIN project SET workspace_id where workspace_id is empty/null. Idempotent. |
| **store.Store** | ListTasksByWorkspace | `(ctx context.Context, workspaceID string, projectID *string) ([]Task, error)` | WHERE workspace_id = ?; optionally AND project_id = ?; ORDER BY created_at. |
| **store.Store** | CreateTask | `(ctx context.Context, workspaceID string, projectID *string, input, createdBy string) (*Task, error)` | Insert task (task_id=uuid, workspace_id, project_id nullable, status=PENDING, input, created_by, created_at=now). |
| **server** | listWorkspaceTasksHandler | `(w http.ResponseWriter, r *http.Request)` | JWT → userID; PathValue("workspace_id"); ownership check; optional query project_id → validate project exists and belongs to workspace; ListTasksByWorkspace; 200 JSON array. |
| **server** | createWorkspaceTaskHandler | `(w http.ResponseWriter, r *http.Request)` | JWT → userID; PathValue("workspace_id"); ownership check; decode body {input, project_id?}; validate input; if project_id: validate project in workspace; CreateTask; 201 JSON. |
| **api (portal)** | getTasks | `(workspaceId: string, token: string, projectId?: string) => Promise<ApiTask[]>` | GET /api/workspaces/{wid}/tasks[?project_id=X] with Bearer token. |
| **api (portal)** | createTask | `(workspaceId: string, body: {input, project_id?}, token: string) => Promise<ApiTask>` | POST /api/workspaces/{wid}/tasks with body and Bearer token. |

## How they work together

**GET /api/workspaces/{workspace_id}/tasks**

1. Client sends GET with Bearer JWT.
2. `listWorkspaceTasksHandler`: requireAuth → userID (401 if missing).
3. workspace_id from path (400 if empty).
4. userOwnsWorkspace → 403 if not owned.
5. TaskStore nil → 503.
6. Optional query `project_id`: if present, GetProject → 404 if nil, check project.WorkspaceID == workspace_id → 400 if mismatch, pass `&projectID`. If absent, pass `nil`.
7. ListTasksByWorkspace(ctx, workspaceID, projectIDPtr) → tasks.
8. Respond 200 with JSON array (id, workspace_id, project_id, status, ...).

**POST /api/workspaces/{workspace_id}/tasks**

1. Client sends POST with body `{ "input": "…", "project_id": "…" }` and Bearer JWT.
2. `createWorkspaceTaskHandler`: requireAuth → userID (401).
3. workspace_id from path (400 if empty).
4. userOwnsWorkspace → 403.
5. TaskStore nil → 503.
6. Decode body; input empty → 400.
7. If project_id non-empty: GetProject → 404 if nil; project.WorkspaceID != workspace_id → 400.
8. CreateTask(ctx, workspaceID, projectIDPtr, input, userID) → task.
9. Respond 201 with task JSON.

**Portal — Project Dashboard**

1. App: when route has projectId, useEffect calls `getTasks(route.workspaceId, token, route.projectId)`.
2. ProjectDashboard: handleRun calls `createTask(workspaceId, { input, project_id: project.id }, token)`.
3. Same UX — tasks displayed for the selected project.

## Testing

### internal/server/tasks_test.go (rewrite)

Mock stores unchanged except mockTaskStore methods match new interface:

```go
type mockTaskStore struct {
    list      []store.Task
    listErr   error
    create    *store.Task
    createErr error
}

func (m *mockTaskStore) ListTasksByWorkspace(_ context.Context, workspaceID string, projectID *string) ([]store.Task, error) {
    if m.listErr != nil { return nil, m.listErr }
    var out []store.Task
    for _, t := range m.list {
        if t.WorkspaceID != workspaceID { continue }
        if projectID != nil && (t.ProjectID == nil || *t.ProjectID != *projectID) { continue }
        out = append(out, t)
    }
    return out, nil
}

func (m *mockTaskStore) CreateTask(_ context.Context, workspaceID string, projectID *string, input, createdBy string) (*store.Task, error) {
    if m.createErr != nil { return nil, m.createErr }
    if m.create != nil { return m.create, nil }
    return &store.Task{
        TaskID: "task-uuid-1", WorkspaceID: workspaceID, ProjectID: projectID,
        Status: "PENDING", Input: input, CreatedBy: createdBy, CreatedAt: 12345,
    }, nil
}
```

Test cases for **listWorkspaceTasksHandler**:

| Case | Setup | Expected |
|------|-------|----------|
| No auth | no Authorization header | 401 |
| Workspace not owned | user u1, workspace ws-other | 403 |
| No project_id filter, empty list | owned workspace, no tasks | 200 `[]` |
| No project_id filter, with tasks | owned workspace, tasks exist | 200 array with tasks |
| With project_id filter, project not found | ?project_id=bad | 404 |
| With project_id filter, project in different workspace | ?project_id=proj-other | 400 |
| With project_id filter, valid | ?project_id=proj1 | 200 filtered list |
| TaskStore nil | store not configured | 503 |

Test cases for **createWorkspaceTaskHandler**:

| Case | Setup | Expected |
|------|-------|----------|
| No auth | no Authorization header | 401 |
| Workspace not owned | user u1, workspace ws-other | 403 |
| Missing input | body `{}` | 400 |
| Empty input | body `{"input":""}` | 400 |
| No project_id in body | body `{"input":"Do X"}` | 201, project_id null |
| With project_id, project not found | body `{"input":"…","project_id":"bad"}` | 404 |
| With project_id, project in different workspace | body `{"input":"…","project_id":"proj-other"}` | 400 |
| With project_id, valid | body `{"input":"…","project_id":"proj1"}` | 201, project_id set, workspace_id set |

All test paths use `/api/workspaces/{wid}/tasks` instead of `/api/projects/{pid}/tasks`.

## Changes for review

| Action | File | What changes |
|--------|------|-------------|
| **Modified** | `internal/store/store.go` | Task struct: add `WorkspaceID`, change `ProjectID` to `*string`; TaskStore interface: replace `ListTasksByProject`/`CreateTask` with `ListTasksByWorkspace`/new `CreateTask`; Store implementation: new query methods; add `BackfillTaskWorkspaceID`. |
| **Modified** | `internal/server/tasks.go` | Remove `listTasksHandler`, `createTaskHandler`; add `listWorkspaceTasksHandler`, `createWorkspaceTaskHandler`; TaskResponse adds `workspace_id`, `project_id` becomes `*string`; createTaskRequest adds `project_id`. |
| **Modified** | `internal/server/server.go` | Remove project-scoped task routes; add workspace-scoped task routes. |
| **Modified** | `internal/server/static/openapi.json` | Remove `/api/projects/{project_id}/tasks`; add `/api/workspaces/{workspace_id}/tasks` (GET with optional project_id query, POST with optional project_id body). |
| **Modified** | `internal/cmd/server.go` | Call `st.BackfillTaskWorkspaceID(ctx)` after store init. |
| **Modified** | `internal/server/helpers_test.go` | mockTaskStore: update methods to match new TaskStore interface. |
| **Modified** | `internal/server/tasks_test.go` | Rewrite tests for workspace-scoped endpoints (new paths, new test cases for project_id validation). |
| **Modified** | `portal/src/lib/types.ts` | Task.projectId becomes optional (`projectId?: string`). |
| **Modified** | `portal/src/lib/api.ts` | ApiTask: add `workspace_id`, `project_id` nullable; `getTasks(workspaceId, token, projectId?)` → new URL; `createTask(workspaceId, body, token)` → new URL and body shape; `apiTaskToTask` handles nullable project_id. |
| **Modified** | `portal/src/App.tsx` | `refetchTasks` and useEffect: call `getTasks(route.workspaceId, token, route.projectId)`. |
| **Modified** | `portal/src/pages/ProjectDashboard.tsx` | `handleRun`: call `createTask(workspaceId, { input, project_id: project.id }, token)`. |
