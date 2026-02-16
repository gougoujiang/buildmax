# Design 048: Use generated session id and save to task

## Goal

Link every task execution to its agent conversation by generating a session UUID in the executor, passing it to the CLI via `--session-id`, and persisting it on the task record.

## Modules

| Module | Role |
|--------|------|
| `internal/store` | Task model, TaskStore interface, Store implementation |
| `internal/executor` | Task runner — generates session id, spawns CLI, updates task |
| `internal/server` | API response type for tasks |

## Structure

### store.Task — new field

```go
SessionID *string `gorm:"type:varchar(36)" json:"session_id,omitempty"`
```

Added after `ErrorMessage`. GORM AutoMigrate adds the column automatically.

### store.TaskStore interface — updated signature

```go
UpdateTaskStatus(ctx context.Context, taskID, status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) error
```

New `sessionID *string` parameter appended at the end.

### store.Store.UpdateTaskStatus — updated implementation

Add `sessionID` handling to the updates map, same pattern as other optional fields.

### executor.TaskStore — updated signature

Mirror the change from `store.TaskStore`.

### executor.executeTask — changes

1. Generate `sessionID := uuid.New().String()` before marking RUNNING.
2. Pass `&sessionID` to `UpdateTaskStatus` when marking RUNNING.
3. Pass `--session-id`, `sessionID` to the `exec.Command` args.
4. Pass `nil` for sessionID in subsequent `UpdateTaskStatus` calls (already set).

### server.TaskResponse — new field

```go
SessionID *string `json:"session_id,omitempty"`
```

### server.taskToResponse — map field

```go
SessionID: t.SessionID,
```

## How they work together

1. API creates task (PENDING) — `session_id` is nil.
2. Executor picks up pending task, generates UUID, marks RUNNING with `session_id` set.
3. Executor spawns `buildmax -p "<input>" --session-id <uuid>`.
4. CLI creates/resumes session with that UUID; session file saved under `DataDir()/sessions/`.
5. Task completes — `session_id` already persisted from step 2.
6. API returns task with `session_id` populated for any non-PENDING task.

## Changes for review

| File | Change |
|------|--------|
| `internal/store/store.go` | Add `SessionID` to `Task`; update `TaskStore` interface and `UpdateTaskStatus` impl |
| `internal/executor/executor.go` | Import `uuid`; generate session id; pass `--session-id`; update call sites |
| `internal/server/tasks.go` | Add `SessionID` to `TaskResponse` and `taskToResponse` |
| `internal/server/helpers_test.go` | Update `mockTaskStore.UpdateTaskStatus` signature |
