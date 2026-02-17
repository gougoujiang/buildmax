# Design 047: Task Executor

## Goal

Background executor inside `buildmax server` that polls for PENDING tasks, runs `buildmax -p` on each one, and writes the result back to the DB and workspace filesystem.

---

## Modules

| Module | Location | Role |
|--------|----------|------|
| executor | `internal/executor/` (new) | Poll loop, spawn CLI, capture output, write result file |
| store | `internal/store/store.go` (edit) | New TaskStore methods + update interface |
| server wiring | `internal/cmd/server.go` (edit) | Start/stop executor alongside HTTP server |
| portal types | `portal/src/lib/types.ts` (edit) | Add `"pending"` to Task status union |
| portal api | `portal/src/lib/api.ts` (edit) | Map PENDING to `"pending"` status |
| portal UI | `portal/src/pages/ProjectDashboard.tsx` (edit) | Reverse task order, add pending icon |

---

## Structure

### 1. `internal/executor/executor.go` (new file)

```go
package executor

// TaskStore is the subset of store methods the executor needs.
type TaskStore interface {
    GetNextPendingTask(ctx context.Context) (*store.Task, error)
    UpdateTaskStatus(ctx context.Context, taskID, status string, startedAt, endedAt *int64, output, errorMessage *string) error
}

// Runner polls for pending tasks and executes them.
type Runner struct {
    store        TaskStore
    workspacesDir string      // config.WorkspacesDir()
    pollInterval time.Duration
    stopCh       chan struct{}
    doneCh       chan struct{}
}

func New(store TaskStore, workspacesDir string) *Runner
func (r *Runner) Start()           // launches the poll goroutine
func (r *Runner) Stop()            // signals stop and waits for in-flight task to finish
func (r *Runner) loop()            // poll loop (private)
func (r *Runner) executeTask(ctx context.Context, task store.Task) // run one task (private)
```

### 2. `internal/store/store.go` — interface extension

```go
// TaskStore — add two methods to the existing interface:
type TaskStore interface {
    ListTasksByWorkspace(ctx context.Context, workspaceID string, projectID *string) ([]Task, error)
    CreateTask(ctx context.Context, workspaceID string, projectID *string, input, createdBy string) (*Task, error)
    // NEW:
    GetNextPendingTask(ctx context.Context) (*Task, error)
    UpdateTaskStatus(ctx context.Context, taskID, status string, startedAt, endedAt *int64, output, errorMessage *string) error
}
```

---

## Method Design

### `executor.New(store TaskStore, workspacesDir string) *Runner`

Creates a Runner with a 5-second poll interval, initialises stop/done channels. Does not start polling.

### `(*Runner) Start()`

Launches `r.loop()` in a goroutine.

### `(*Runner) Stop()`

1. Close `r.stopCh` to signal the loop to exit.
2. Block on `r.doneCh` to wait for any in-flight task to finish.

### `(*Runner) loop()` (private)

```
defer close(r.doneCh)
ticker := time.NewTicker(r.pollInterval)
defer ticker.Stop()
for {
    select {
    case <-r.stopCh:
        return
    case <-ticker.C:
        task, err := r.store.GetNextPendingTask(ctx)
        if err != nil { log warning; continue }
        if task == nil { continue }
        r.executeTask(ctx, *task)
    }
}
```

### `(*Runner) executeTask(ctx context.Context, task store.Task)` (private)

1. **Mark RUNNING** — call `UpdateTaskStatus(taskID, "RUNNING", &now, nil, nil, nil)`.
2. **Resolve workspace dir** — `filepath.Join(r.workspacesDir, task.WorkspaceID)`. Ensure directory exists (`os.MkdirAll`).
3. **Build command** — `exec.CommandContext(ctx, "buildmax", "-p", task.Input)`. Set `cmd.Dir` to workspace dir. Inherit env from parent (`cmd.Env` = nil, which means inherit).
4. **Run and capture** — `cmd.CombinedOutput()` to capture both stdout and stderr.
5. **Write result file** — write output to `<workspace_dir>/result-<task_id>.md`.
6. **Update DB**:
   - On success (exit code 0): `UpdateTaskStatus(taskID, "SUCCEEDED", nil, &now, &output, nil)`.
   - On failure (non-zero exit or error): `UpdateTaskStatus(taskID, "FAILED", nil, &now, &output, &errMsg)`.

### `(*Store) GetNextPendingTask(ctx) (*Task, error)`

```sql
SELECT * FROM task WHERE status = 'PENDING' ORDER BY created_at ASC LIMIT 1
```

Returns `(nil, nil)` when no rows found.

### `(*Store) UpdateTaskStatus(ctx, taskID, status, startedAt, endedAt, output, errorMessage) error`

Builds a GORM `Updates` map from the non-nil parameters:

```go
updates := map[string]interface{}{"status": status}
if startedAt != nil { updates["started_at"] = *startedAt }
if endedAt != nil   { updates["ended_at"] = *endedAt }
if output != nil    { updates["output"] = *output }
if errorMessage != nil { updates["error_message"] = *errorMessage }
db.Model(&Task{}).Where("task_id = ?", taskID).Updates(updates)
```

---

## How They Work Together

```
buildmax server starts
    │
    ├─ HTTP server starts (existing)
    │
    └─ executor.New(store, workspacesDir).Start()
           │
           └─ loop() goroutine
                  │
                  ├─ every 5s: store.GetNextPendingTask()
                  │       │
                  │       └─ if task found → executeTask()
                  │               │
                  │               ├─ store.UpdateTaskStatus → RUNNING
                  │               ├─ exec.Command("buildmax", "-p", input)
                  │               │   CWD = workspacesDir/<workspace_id>
                  │               ├─ capture output
                  │               ├─ write result-<task_id>.md
                  │               └─ store.UpdateTaskStatus → SUCCEEDED / FAILED
                  │
                  └─ on stopCh closed → finish in-flight task → close doneCh

Server shutdown (SIGINT/SIGTERM):
    ├─ runner.Stop()   ← blocks until doneCh closed
    └─ http.Server.Shutdown() (existing)
```

### Server wiring (`internal/cmd/server.go`)

After `store.New()` and before `s.Run()`:

```go
runner := executor.New(st, config.WorkspacesDir())
runner.Start()
defer runner.Stop()
```

The `defer runner.Stop()` ensures the executor stops before the function returns (i.e., after the HTTP server shuts down).

---

## Portal Changes

### `portal/src/lib/types.ts`

Add `"pending"` to the Task status union:

```typescript
status: "pending" | "running" | "success" | "failed" | "canceled"
```

### `portal/src/lib/api.ts`

Update `taskStatusToUI`:

```typescript
function taskStatusToUI(status: string): Task["status"] {
  switch (status) {
    case "SUCCEEDED": return "success"
    case "FAILED":    return "failed"
    case "CANCELED":  return "canceled"
    case "PENDING":   return "pending"
    case "RUNNING":
    default:          return "running"
  }
}
```

### `portal/src/pages/ProjectDashboard.tsx`

1. **Reverse task order** — show newest first: `[...tasks].reverse()` in the render.
2. **Add pending icon** — update `statusIcon()`:

```typescript
function statusIcon(status: Task["status"]): string {
  switch (status) {
    case "success":  return "✅"
    case "running":  return "⏳"
    case "failed":   return "❌"
    case "canceled": return "⛔"
    case "pending":  return "🕐"
  }
}
```

---

## Changes for Review

| Action | File | What |
|--------|------|------|
| **New** | `internal/executor/executor.go` | `Runner` struct, `New`, `Start`, `Stop`, `loop`, `executeTask` |
| **Edit** | `internal/store/store.go` | Add `GetNextPendingTask`, `UpdateTaskStatus` to interface + implementations |
| **Edit** | `internal/cmd/server.go` | Create and start executor, defer stop |
| **Edit** | `portal/src/lib/types.ts` | Add `"pending"` to Task status |
| **Edit** | `portal/src/lib/api.ts` | Map `PENDING` → `"pending"` in `taskStatusToUI` |
| **Edit** | `portal/src/pages/ProjectDashboard.tsx` | Reverse task order, add pending icon |
