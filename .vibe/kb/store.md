# Store

## Purpose

The `internal/store` package provides persistence for the BuildMax backend: users, workspaces, projects, and tasks. It uses GORM with MySQL. Entities use ULIDs for public IDs (`user_id`, `workspace_id`, `project_id`, `task_id`); internal auto-increment IDs are for the database only and not exposed in the API. Table names are singular per project convention.

## Key Types and Interfaces

| Name | Kind | Role |
|------|------|------|
| **User** | struct | Email, name, user_id (ULID), created_at; table `user` |
| **Workspace** | struct | workspace_id, owner_user_id, name, created_at; table `workspace` |
| **Project** | struct | project_id, workspace_id, name, description, created_at; table `project` |
| **Task** | struct | task_id, project_id, status, input, output, created_by, created_at, started_at, ended_at, error_message; table `task` |
| **UserStore** | interface | `UserByEmail(ctx, email) (*User, error)` |
| **WorkspaceStore** | interface | `EnsureDefaultWorkspaceForUser`, `ListWorkspacesByOwner` |
| **ProjectStore** | interface | `GetProject`, `ListProjectsByWorkspace`, `CreateProject` |
| **TaskStore** | interface | `ListTasksByProject`, `CreateTask` |
| **Store** | struct | Implements all four store interfaces with a MySQL backend |

## How It Works

### Creation

```go
st, err := store.New(ctx, dsn)  // DSN is MySQL connection string
```

`New` opens the DB, runs `AutoMigrate` for User, Workspace, Project, and Task, and returns a `*Store` that implements `UserStore`, `WorkspaceStore`, `ProjectStore`, and `TaskStore`.

### ID Generation

Public IDs are ULIDs generated with `ulid.MustNew(ulid.Now(), rand.Reader).String()` (via internal `newULID()`). This keeps IDs URL-safe and sortable.

### Conventions

- JSON and API fields use **snake_case** (e.g. `created_at`, `owner_user_id`).
- **Table names are singular**: `user`, `workspace`, `project`, `task`.
- GORM struct tags define column types; `json:"-"` on internal ID fields keeps them out of API responses.

## Dependencies

- **Uses**: `gorm.io/gorm`, `gorm.io/driver/mysql`, `github.com/oklog/ulid/v2`.
- **Used by**: `internal/server` (injected as store interfaces), `internal/cmd` (server command builds Store from env and passes it to server.Config).

## Notes

- The server accepts store interfaces so tests can inject in-memory or mock implementations.
- `Store.Close()` closes the underlying DB connection; optional for process lifetime.
- See also: [Server](server.md), [CLI](cli.md).
