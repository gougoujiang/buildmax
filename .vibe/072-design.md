# Design 072: Remove project related in backend

## Goal

Remove the project concept from the backend so that tasks and artifacts are owned by workspace only; all project-related APIs, storage, types, and request/response fields are removed.

## Modules

| Module | Responsibility | Owns |
|--------|----------------|------|
| **internal/model** | Domain types shared across layers | User, Workspace, Task, Artifact, ArtifactWithTask, etc. |
| **internal/storage/entity** | MySQL persistence and store interfaces | Store, ProjectStore (to remove), TaskStore, ArtifactStore, entity types |
| **internal/server** | HTTP API and handlers | Config, routes, handlers, OpenAPI |
| **internal/workerapi** | Worker HTTP contract types | GetTaskRunResponse, TaskRunTask, etc. |
| **internal/servercmd** | Server bootstrap | RunServer, wiring Store into server Config |

## Structure

**internal/model**
- `models.go` — Domain structs. Remove `Project` and `Project.TableName()`; remove `ProjectID` from `Task` and `ArtifactWithTask`.

**internal/storage/entity**
- `project.go` — **Delete file** (GetProject, ListProjectsByWorkspace, CreateProject).
- `types.go` — Remove `type Project = model.Project`.
- `interfaces.go` — Remove `ProjectStore` interface; update `TaskStore` and `ArtifactStore` method signatures (drop `projectID *string`).
- `store.go` — Remove `&Project{}` from AutoMigrate; remove `BackfillTaskWorkspaceID` (it joins `project`; after project removal there is no source for workspace_id backfill; tasks created via API already have workspace_id).
- `task.go` — ListTasksByWorkspace, ListTasksByWorkspacePaginated: drop `projectID *string` param and all `Where("project_id = ?", ...)`; CreateTask: drop `projectID *string`, stop setting `Task.ProjectID`.
- `artifact.go` — ListArtifactsByWorkspace: signature `(ctx, workspaceID, taskID *string)`; remove `projectID` param and `AND t.project_id = ?` from raw SQL; SELECT no longer needs `t.project_id` (remove from ArtifactWithTask too via model).
- `store_test.go` — Update ListArtifactsByWorkspace calls to 3 args `(ctx, workspaceID, taskID *string)`; remove the test that filters by `project_id` (listEmpty with "other-project").

**internal/server**
- `server.go` — Remove `ProjectStore` from Config; remove routes `GET/POST /api/workspaces/{workspace_id}/projects`.
- `projects.go` — **Delete file**.
- `tasks.go` — Remove `resolveProjectForWorkspace`; list handler: no `project_id` query param, call store with no projectID; create handler: request body without `project_id`, call CreateTask(workspaceID, nil, ...); response types without `project_id`.
- `artifacts.go` — List handler: no `project_id` query param; call ListArtifactsByWorkspace(ctx, workspaceID, taskIDPtr) only; response type without `project_id`.
- `worker_handlers.go` — Build TaskRunTask without ProjectID field.
- `helpers_test.go` — Remove `mockProjectStore`; update `mockTaskStore` (ListTasksByWorkspace, ListTasksByWorkspacePaginated, CreateTask without projectID); update `mockArtifactStore.ListArtifactsByWorkspace(ctx, workspaceID, taskID *string)`.
- `projects_test.go` — **Delete file**.
- `tasks_test.go` — Remove projectStore from test cases; remove project_id query/body and all project-related expectations (404 project not found, 400 project wrong workspace, filtered list by project).
- `artifacts_test.go` — No project_id in requests; mockArtifactStore signature updated via helpers_test.
- `static/openapi.json` — Remove path `"/api/workspaces/{workspace_id}/projects"` (entire key); in tasks GET remove `project_id` parameter and response `project_id`, and 400/404 project descriptions; in tasks POST remove requestBody `project_id` and response `project_id`, and 400/404 project descriptions; in artifacts GET remove `project_id` parameter and response `project_id`.

**internal/workerapi**
- `types.go` — Remove `ProjectID` from `TaskRunTask`.

**internal/servercmd**
- `run.go` — Remove `ProjectStore: st` from server Config (or remove the field from Config and this assignment).

## Method design

| Receiver | Method | New/updated signature | Responsibility |
|----------|--------|------------------------|----------------|
| **Store** | (removed) | — | GetProject, ListProjectsByWorkspace, CreateProject deleted. |
| **Store** | (removed) | — | BackfillTaskWorkspaceID removed. |
| **TaskStore** | ListTasksByWorkspace | `(ctx, workspaceID, order string)` | List tasks in workspace; no project filter. |
| **TaskStore** | ListTasksByWorkspacePaginated | `(ctx, workspaceID, executedOnly bool, limit, offset int)` | Same; no projectID param. |
| **TaskStore** | CreateTask | `(ctx, workspaceID, input, title, createdBy string)` | Create task in workspace; no projectID. |
| **ArtifactStore** | ListArtifactsByWorkspace | `(ctx, workspaceID, taskID *string)` | List artifacts; optional task filter only. |
| **Server** | (removed) | — | listProjectsHandler, createProjectHandler, resolveProjectForWorkspace. |
| **Server** | listWorkspaceTasksHandler | (unchanged name) | No project_id query; call store without projectID. |
| **Server** | createWorkspaceTaskHandler | (unchanged name) | Body: `input` only; CreateTask(..., nil, input, ...). |
| **Server** | listWorkspaceArtifactsHandler | (unchanged name) | Only task_id query; ListArtifactsByWorkspace(ctx, workspaceID, taskIDPtr). |

## How they work together

**Data flow after change**

1. Client calls GET/POST `/api/workspaces/{id}/tasks` or GET `/api/workspaces/{id}/artifacts` with no project_id. Server validates workspace ownership, calls TaskStore/ArtifactStore with workspace (and optional task_id for artifacts) only.
2. Worker GET `/api/worker/task-runs/{run_id}` returns task with workspace_id, session_id, last_run_id only (no project_id).
3. No code reads or writes the `project` table or `task.project_id` column.

**Dependencies**

- Server depends on entity (Store, interfaces). Removing ProjectStore from Config and from handlers removes that dependency for project.
- Entity no longer implements ProjectStore; model no longer defines Project or Task.ProjectID / ArtifactWithTask.ProjectID.

**Database**

- **Option A (recommended for this task):** No DDL. Stop all code references to `project` and `task.project_id`. The `project` table and `task.project_id` column remain in the schema but are unused. Simplest and reversible.
- **Option B (optional follow-up):** Run one-time migration: `DROP TABLE project`, `ALTER TABLE task DROP COLUMN project_id`. Can be done in a separate migration script or startup hook if desired.

## Changes for review

- **Deleted**: `internal/storage/entity/project.go`
- **Deleted**: `internal/server/projects.go`
- **Deleted**: `internal/server/projects_test.go`
- **Modified**: `internal/model/models.go` — Remove Project struct and TableName; remove ProjectID from Task and ArtifactWithTask.
- **Modified**: `internal/storage/entity/types.go` — Remove Project alias.
- **Modified**: `internal/storage/entity/interfaces.go` — Remove ProjectStore; TaskStore and ArtifactStore method signatures without projectID.
- **Modified**: `internal/storage/entity/store.go` — AutoMigrate without Project; remove BackfillTaskWorkspaceID.
- **Modified**: `internal/storage/entity/task.go` — ListTasksByWorkspace, ListTasksByWorkspacePaginated, CreateTask without projectID.
- **Modified**: `internal/storage/entity/artifact.go` — ListArtifactsByWorkspace(ctx, workspaceID, taskID *string); raw SQL without project_id.
- **Modified**: `internal/storage/entity/store_test.go` — ListArtifactsByWorkspace calls and remove project_id filter test.
- **Modified**: `internal/server/server.go` — Config without ProjectStore; routes without projects.
- **Modified**: `internal/server/tasks.go` — No resolveProjectForWorkspace; list/create without project_id.
- **Modified**: `internal/server/artifacts.go` — List without project_id query.
- **Modified**: `internal/server/worker_handlers.go` — TaskRunTask without ProjectID.
- **Modified**: `internal/server/helpers_test.go` — Remove mockProjectStore; mock task/artifact stores with new signatures.
- **Modified**: `internal/server/tasks_test.go` — No project store or project_id cases.
- **Modified**: `internal/server/artifacts_test.go` — mockArtifactStore signature (via helpers_test).
- **Modified**: `internal/server/static/openapi.json` — Remove projects path; remove project_id from tasks and artifacts.
- **Modified**: `internal/workerapi/types.go` — TaskRunTask without ProjectID.
- **Modified**: `internal/servercmd/run.go` — Do not set ProjectStore on server Config.
