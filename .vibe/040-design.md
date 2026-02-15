# Design 040: Entity id refactor

## Goal

Standardize entity identity: internal integer primary keys for DB use only, public ULID columns for API and references, and singular table names. AGENTS.md already updated with the table-naming rule (§6.2); this design covers store, server, migration, and tests.

## Modules

| Module (package) | Responsibility | Changes |
|------------------|----------------|---------|
| **internal/store** | User, Workspace, Project, Task persistence | New model fields (id + user_id/workspace_id/project_id); TableName singular; ULID generation; GetProject by project_id column; all create methods use ULID |
| **internal/server** | HTTP API and auth | Use store’s public id fields (UserID, WorkspaceID, ProjectID, TaskID) for JWT, responses, and ownership; no API path/param renames |
| **internal/server (\*_test.go)** | Handler and integration tests | Mocks and test data use new struct fields (UserID, WorkspaceID, ProjectID); GetProject mock matches by ProjectID |
| **AGENTS.md** | Project rules | Already done: §6.2 Database table naming (singular tables) |

## Dependencies

- Add **github.com/oklog/ulid/v2** for ULID generation. Store ULIDs as `string` (e.g. varchar(26) in MySQL). No change to API contract: ids remain strings; only format changes from UUID to ULID.

## Structure

### internal/store/store.go

**ULID helper**

- Add a package-level or inline generator using `ulid.MustNew(ulid.Now(), rand)` (or equivalent) when creating new entities. Use a single place (e.g. `newULID() string`) so all create paths call it.

**User**

- **id** (int / uint): primary key, auto-increment, internal only. GORM tag: `gorm:"primaryKey;autoIncrement"`, json: `"-"`.
- **user_id** (string): ULID, unique, not null; used in API and JWT. GORM: `type:varchar(26);uniqueIndex;not null`; json: `"user_id"`. For login response the API still exposes `id` (see server) — value is `user_id`.
- Keep: **email**, **name**, **created_at** (unchanged).
- **TableName()** return `"user"` (singular).
- **UserByEmail**: unchanged signature; returned `*User` now has `UserID` populated. Server will use `user.UserID` for JWT `sub` and login response `user.id`.

**Workspace**

- **id** (int / uint): primary key, auto-increment, internal only; json: `"-"`.
- **workspace_id** (string): ULID, unique, not null; GORM varchar(26); json: `"workspace_id"`. API response `id` = workspace_id.
- **owner_user_id** (string): unchanged name; references `user.user_id` (varchar(26)).
- Keep: **name**, **created_at**.
- **TableName()** return `"workspace"`.
- **EnsureDefaultWorkspaceForUser(ctx, userID)**: `userID` is user’s `user_id` (ULID). Create workspace with `WorkspaceID: newULID()`, `OwnerUserID: userID`.
- **ListWorkspacesByOwner(ctx, userID)**: unchanged; still filter by `owner_user_id = ?` (user_id).

**Project**

- **id** (int / uint): primary key, auto-increment, internal only; json: `"-"`.
- **project_id** (string): ULID, unique, not null; varchar(26); json: `"project_id"`. API response `id` = project_id.
- **workspace_id** (string): references workspace.workspace_id; varchar(26).
- Keep: **name**, **description**, **created_at**.
- **TableName()** return `"project"`.
- **GetProject(ctx, projectID)**: query by `project_id = ?` (not `id = ?`). Signature unchanged.
- **ListProjectsByWorkspace**, **CreateProject**: use `workspace_id` and set `ProjectID: newULID()` on create.

**Task**

- **id** (uint): keep; primary key, auto-increment; json: `"-"`.
- **task_id** (string): change from UUID to ULID; varchar(26); unique, not null. API response `id` = task_id.
- **project_id**, **created_by**: store ULID strings (varchar(26)); semantics unchanged.
- **TableName()** return `"task"`.
- **CreateTask**: set `TaskID: newULID()` (replace uuid.New().String()).

**Interfaces**

- No interface signature changes: `UserByEmail(ctx, email)`, `EnsureDefaultWorkspaceForUser(ctx, userID)`, `ListWorkspacesByOwner(ctx, userID)`, `GetProject(ctx, projectID)`, `ListProjectsByWorkspace(ctx, workspaceID)`, `CreateProject(ctx, workspaceID, name, description)`, `ListTasksByProject(ctx, projectID)`, `CreateTask(ctx, projectID, input, createdBy)`. All id arguments and return fields are the public ids (user_id, workspace_id, project_id, task_id).

### internal/server

**login.go**

- JWT claims `Sub`: set to `user.UserID` (not `user.ID`).
- Login response `User.ID`: set to `user.UserID` so API still exposes `"id"` as the user’s public id.

**workspaces.go**

- When creating dirs: `filepath.Join(root, w.WorkspaceID)` (not `w.ID`).
- **WorkspaceResponse**: `ID` field value = `list[i].WorkspaceID` (so JSON `"id"` remains the public workspace id).

**projects.go**

- **userOwnsWorkspace**: compare `w.WorkspaceID == workspaceID` (not `w.ID`). Callers pass path param `workspace_id` and `project.WorkspaceID`; both are now ULIDs.
- **ProjectResponse**: `ID` = `p.ProjectID` (not `p.ID`).

**tasks.go**

- **TaskResponse**: `ID` = `t.TaskID` (already the case). Task model’s TaskID becomes ULID; no handler change beyond store returning ULID.

No changes to path or query parameter names; only the values are ULIDs.

## Method design

| Receiver / Package | Method | Signature | Change |
|--------------------|--------|-----------|--------|
| **store** | (internal) | `newULID() string` or inline | Return new ULID string (e.g. ulid.MustNew(ulid.Now(), rand)). |
| **store.Store** | UserByEmail | `(ctx, email) (*User, error)` | Unchanged. Returned User has UserID set; server uses UserID. |
| **store.Store** | EnsureDefaultWorkspaceForUser | `(ctx, userID string) error` | userID is user_id (ULID). Create with WorkspaceID = newULID(). |
| **store.Store** | ListWorkspacesByOwner | `(ctx, userID string) ([]Workspace, error)` | Unchanged; filter by owner_user_id. |
| **store.Store** | GetProject | `(ctx, projectID string) (*Project, error)` | Where `project_id = ?` (not id). |
| **store.Store** | ListProjectsByWorkspace | `(ctx, workspaceID string) ([]Project, error)` | Unchanged. |
| **store.Store** | CreateProject | `(ctx, workspaceID, name, description string) (*Project, error)` | Set ProjectID = newULID(); WorkspaceID = workspaceID. |
| **store.Store** | ListTasksByProject | `(ctx, projectID string) ([]Task, error)` | Unchanged. |
| **store.Store** | CreateTask | `(ctx, projectID, input, createdBy string) (*Task, error)` | Set TaskID = newULID() (replace uuid). |
| **server** | loginHandler | — | JWT Sub = user.UserID; response user.id = user.UserID. |
| **server** | workspacesHandler | — | Dir = w.WorkspaceID; response id = list[i].WorkspaceID. |
| **server** | userOwnsWorkspace | — | Compare w.WorkspaceID == workspaceID. |
| **server** | listProjectsHandler / createProjectHandler | — | Response id = p.ProjectID. |
| **server** | listTasksHandler / createTaskHandler | — | No logic change; task.TaskID is now ULID. |

## How they work together

**Login and JWT**

1. User logs in with email; UserByEmail returns User (with UserID).
2. JWT `sub` = user.UserID; login response `user.id` = user.UserID. Clients and subsequent requests use this ULID.

**Workspaces**

1. EnsureDefaultWorkspaceForUser(userID) creates workspace with WorkspaceID = newULID(), OwnerUserID = userID.
2. ListWorkspacesByOwner(userID) returns workspaces; server uses w.WorkspaceID for response `id` and for workspace dir path.

**Projects**

1. GetProject(projectID) looks up by column project_id. List/Create use workspace_id and project_id (ULIDs).
2. userOwnsWorkspace(userID, workspaceID) compares workspaceID to each w.WorkspaceID in the user’s list.
3. Project response `id` = p.ProjectID.

**Tasks**

1. ListTasksByProject / CreateTask use projectID (ULID); Task.TaskID is ULID; API response `id` = task_id unchanged.

## Migration and compatibility

**New installs**

- GORM AutoMigrate with the new model definitions will create tables with singular names and new columns. If the app has never run against a DB, no migration script is needed.

**Existing databases (optional path)**

- GORM’s AutoMigrate adds columns but does not rename tables or columns. For an existing DB with plural tables and UUID primary keys:
  1. **Option A (recommended for greenfield)**: Document that this refactor assumes a fresh schema; existing deployments with data must run a one-time migration (see below) or recreate DB.
  2. **Option B (one-time migration)**: Document steps: (1) Add new columns (id bigint AUTO_INCREMENT, user_id varchar(26) UNIQUE, etc.) and new tables if needed; (2) Backfill user_id from id (e.g. generate ULID per row or keep existing UUID and treat as legacy); (3) Add workspace_id, project_id, update task_id to ULID if desired; (4) Switch application to new schema (TableName singular, query by new columns); (5) Optionally rename/drop old columns in a follow-up. Implementation in this task can limit to “new schema only” and add a short migration note in code or .vibe/040.md.

For this task, **deliverable**: either implement AutoMigrate-only (new schema) and document that existing DBs need a manual migration, or add a small migration helper that creates new tables (singular names) and copies data. Design prefers minimal scope: **new schema + doc** (migration steps in a comment or .vibe/040.md).

## Testing

- **internal/store**: If there are store tests, update expectations to use UserID, WorkspaceID, ProjectID, TaskID and singular table names; creation tests should assert ULID format (length/prefix) if desired.
- **internal/server/login_test.go**: Mock user with `UserID` set; assert JWT sub and response `user.id` equal that UserID.
- **internal/server/workspaces_test.go**: Mock workspaces with `WorkspaceID`; assert response `id` and any dir logic use WorkspaceID.
- **internal/server/projects_test.go**: Mock workspaces and projects with `WorkspaceID`, `ProjectID`; GetProject mock should match by `ProjectID`; assert response `id` is ProjectID.
- **internal/server/tasks_test.go**: Tasks already use TaskID; ensure mock/store use ULID-shaped ids; existing assertions on `id`, `project_id` unchanged.

## Changes for review

- **Modified** `go.mod` / `go.sum` — Add `github.com/oklog/ulid/v2`.
- **Modified** `internal/store/store.go` — User: id (int), user_id (ULID), TableName "user". Workspace: id, workspace_id (ULID), owner_user_id, TableName "workspace". Project: id, project_id (ULID), workspace_id, TableName "project". Task: task_id as ULID, TableName "task". Add newULID(); replace uuid in create paths; GetProject by project_id.
- **Modified** `internal/server/login.go` — JWT Sub and LoginUser.ID use user.UserID.
- **Modified** `internal/server/workspaces.go` — Dir and response ID use WorkspaceID.
- **Modified** `internal/server/projects.go` — userOwnsWorkspace compare WorkspaceID; response ID use ProjectID.
- **Modified** `internal/server/tasks.go` — No logic change; TaskID from store is ULID.
- **Modified** `internal/server/login_test.go` — User mock and assertions use UserID.
- **Modified** `internal/server/workspaces_test.go` — Workspace mocks use WorkspaceID; assertions on response id.
- **Modified** `internal/server/projects_test.go` — Workspace/Project mocks use WorkspaceID/ProjectID; GetProject mock match by ProjectID; assertions on response id.
- **Modified** `internal/server/tasks_test.go` — Use ULID-shaped ids in mocks/stubs if needed; existing response checks unchanged.
- **AGENTS.md** — Already updated with §6.2 Database table naming (singular).
- **Optional** — Migration note in store or .vibe/040.md for existing DBs (new schema only + short migration steps).
