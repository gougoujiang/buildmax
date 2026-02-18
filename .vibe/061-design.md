# Design 061: Prevent scheduled multiple times (SCHEDULED phased flow)

## Goal

Introduce status **SCHEDULED** so the lifecycle is **PENDING → SCHEDULED → RUNNING → SUCCEEDED | FAILED**. The scheduler claims a task by moving it to SCHEDULED, spawns the worker, and reverts to PENDING if spawn fails. The worker only proceeds when the task is SCHEDULED and atomically moves it to RUNNING before execution.

## Status flow

| Status     | Meaning |
|-----------|---------|
| PENDING   | Not yet claimed by any scheduler. |
| SCHEDULED | Claimed by a scheduler; worker should be (or will be) spawned. |
| RUNNING   | Worker has started execution. |
| SUCCEEDED | Task completed successfully. |
| FAILED    | Task ended in failure. |

Transitions:

- **PENDING → SCHEDULED**: Scheduler (atomic via `UpdateTaskStatusIf`). On spawn failure, **SCHEDULED → PENDING**.
- **SCHEDULED → RUNNING**: Worker via PATCH (atomic via `UpdateTaskStatusIf`; 409 if not updated).
- **RUNNING → SUCCEEDED | FAILED**: Worker via PATCH (unconditional `UpdateTaskStatus`).

## Modules

| Module (package) | Change |
|------------------|--------|
| **internal/storage/entity** | Add `UpdateTaskStatusIf` to TaskStore and Store. Status column already string; no migration if it accepts any value. |
| **internal/executor** | Scheduler loop: after GetNextPendingTask, call UpdateTaskStatusIf(PENDING, SCHEDULED); on success spawn; on spawn failure call UpdateTaskStatus(taskID, "PENDING", ...). Define `ErrTaskAlreadyClaimed` for HTTP 409. WorkerHTTPUpdater returns it on 409. |
| **internal/server** | patchWorkerTaskHandler: for RUNNING use UpdateTaskStatusIf(SCHEDULED, RUNNING); if !updated return 409. |
| **internal/workercmd** | Require task.Status == "SCHEDULED"; on 409 from PATCH RUNNING return ErrAlreadyClaimed and exit without RunTask. |
| **cmd/buildmax-worker** | On workercmd.ErrAlreadyClaimed exit 2. |

## Structure

**Files to modify**

- `internal/storage/entity/interfaces.go` — Add `UpdateTaskStatusIf` to TaskStore.
- `internal/storage/entity/task.go` — Implement `UpdateTaskStatusIf` (WHERE task_id AND status = expectedStatus; Updates; return RowsAffected == 1).
- `internal/executor/scheduler.go` — In loop: get task; if task != nil, call store.UpdateTaskStatusIf(PENDING, SCHEDULED); if !updated continue; spawn worker; if spawn err, store.UpdateTaskStatus(taskID, "PENDING", ...).
- `internal/server/worker_handlers.go` — For req.Status == "RUNNING", call UpdateTaskStatusIf(SCHEDULED, RUNNING, ...); if !updated writeJSONError 409 and return.
- `internal/executor/worker_api.go` — On resp.StatusCode == 409 return ErrTaskAlreadyClaimed.
- `internal/workercmd/run.go` — Require status == "SCHEDULED"; after PATCH RUNNING, if errors.Is(err, executor.ErrTaskAlreadyClaimed) return ErrAlreadyClaimed (do not call RunTask).
- `cmd/buildmax-worker/main.go` — If RunWorker returns workercmd.ErrAlreadyClaimed, os.Exit(2).

**APIs / UI**

- Task list and activity already return `status`; SCHEDULED is a new value. Portal or any client that displays status can map it (e.g. "Scheduled" or "Queued"). No new endpoint; existing GET task and list tasks already return status.

## Main types and interfaces

**TaskStore (entity)**

- **UpdateTaskStatusIf** `(ctx, taskID, expectedStatus, newStatus string, startedAt, endedAt *int64, output, errorMessage, sessionID *string) (updated bool, err error)`  
  Updates the task only when current status equals `expectedStatus`; sets status and optional fields to `newStatus` and provided pointers. Returns updated = (RowsAffected == 1).

**Executor**

- **ErrTaskAlreadyClaimed** — Sentinel error returned by WorkerHTTPUpdater when server responds 409 (e.g. task not SCHEDULED or already RUNNING).

**Workercmd**

- **ErrAlreadyClaimed** — Sentinel returned by RunWorker when PATCH RUNNING returns 409; main uses it for exit code 2.

## Method design

| Receiver / Scope | Method | Responsibility |
|------------------|--------|----------------|
| entity.Store | **UpdateTaskStatusIf** | Build updates map for newStatus and non-nil optional fields. `Model(&Task{}).Where("task_id = ? AND status = ?", taskID, expectedStatus).Updates(updates)`. Return (result.RowsAffected == 1, nil) or (false, err). |
| entity.TaskStore | **UpdateTaskStatusIf** | Interface contract. |
| executor.Scheduler | **loop** | GetNextPendingTask. If task != nil: UpdateTaskStatusIf(ctx, task.TaskID, "PENDING", "SCHEDULED", nil, nil, nil, nil, nil). If !updated: continue. Spawn worker (exec). If spawn err: TaskStore.UpdateTaskStatus(ctx, task.TaskID, "PENDING", nil, nil, nil, nil, nil); log. |
| server | **patchWorkerTaskHandler** | If req.Status == "RUNNING": updated, err := UpdateTaskStatusIf(ctx, taskID, "SCHEDULED", "RUNNING", req.StartedAt, nil, nil, nil, req.SessionID). If err != nil: 500. If !updated: writeJSONError(409, "task not scheduled or already running"); return. Else: same as today (artifact handling if present, 200). For other statuses: UpdateTaskStatus (unconditional) and 200. |
| executor.WorkerHTTPUpdater | **UpdateTaskStatus** | On resp.StatusCode == 409 return ErrTaskAlreadyClaimed. Else non-2xx return existing error. |
| workercmd | **RunWorker** | GET task. If task.Status != "SCHEDULED" return error ("task not scheduled (status=...)"). PATCH RUNNING. If errors.Is(err, executor.ErrTaskAlreadyClaimed): log, return ErrAlreadyClaimed. Else if err != nil return err. Build storage and paths; executor.RunTask(...). |
| cmd/buildmax-worker main | (existing) | If err := workercmd.RunWorker(...); err != nil { if errors.Is(err, workercmd.ErrAlreadyClaimed) { os.Exit(2) }; os.Exit(1) }. Else os.Exit(0). |

## How they work together

**Scheduler (server process)**

1. Each tick: GetNextPendingTask(ctx). If nil, continue.
2. UpdateTaskStatusIf(ctx, task.TaskID, "PENDING", "SCHEDULED", nil, nil, nil, nil, nil). If !updated, another scheduler claimed it; continue.
3. Spawn worker: exec.Command(workerPath, "--task-id", task.TaskID).Run().
4. If Run() returns error (spawn failed or worker exited with failure): UpdateTaskStatus(ctx, task.TaskID, "PENDING", ...) so next poll retries. Log the spawn error.

**Worker process**

1. GET /api/worker/tasks/{task_id}. If task == nil or task.Status != "SCHEDULED", exit with error (e.g. "task not scheduled").
2. PATCH RUNNING with session_id, started_at. Server calls UpdateTaskStatusIf(SCHEDULED, RUNNING, ...). If !updated → 409.
3. If 409: WorkerHTTPUpdater returns ErrTaskAlreadyClaimed. RunWorker returns ErrAlreadyClaimed; main exits 2.
4. If 200: Build storage, paths; RunTask(...); PATCH SUCCEEDED or FAILED (unconditional).

**Concurrency**

- Two schedulers: both may GetNextPendingTask and get the same task. Both call UpdateTaskStatusIf(PENDING, SCHEDULED). One wins (updated=true), one gets false. Only the winner spawns. Loser continues to next tick.
- Two workers for same task_id (should not happen if only one scheduler spawns per task): both GET and see SCHEDULED. Both PATCH RUNNING. Server uses UpdateTaskStatusIf(SCHEDULED, RUNNING). One wins (200), one gets 409 and exits.

## Revert on spawn failure

Scheduler must have access to TaskStore to call UpdateTaskStatus. It already has TaskStore for GetNextPendingTask. After spawn (cmd.Run()), if err != nil we call tasks.UpdateTaskStatus(ctx, task.TaskID, "PENDING", nil, nil, nil, nil, nil). Optional: clear started_at/session_id if we ever set them at SCHEDULED time (in this design we don’t; only RUNNING sets started_at/session_id). So revert is just status PENDING.

## Dependencies

- Scheduler already has entity.TaskStore; now calls UpdateTaskStatusIf and UpdateTaskStatus.
- Server already has TaskStore; patchWorkerTaskHandler calls UpdateTaskStatusIf for RUNNING.
- Worker uses executor.GetWorkerTask and executor.WorkerHTTPUpdater; workercmd checks executor.ErrTaskAlreadyClaimed and returns workercmd.ErrAlreadyClaimed.

## Tests

- **entity**: UpdateTaskStatusIf — task PENDING, expected PENDING, new SCHEDULED → updated true, row SCHEDULED. Task SCHEDULED, expected PENDING → updated false. Task RUNNING, expected SCHEDULED, new RUNNING → updated false.
- **executor/scheduler**: With mock TaskStore that returns one PENDING task and records UpdateTaskStatusIf/UpdateTaskStatus calls: verify UpdateTaskStatusIf(PENDING, SCHEDULED) and, when spawn fails, UpdateTaskStatus(..., "PENDING", ...).
- **server**: PATCH RUNNING when task SCHEDULED → 200 and task becomes RUNNING. PATCH RUNNING when task PENDING or RUNNING → 409.
- **workercmd**: When task status != SCHEDULED, RunWorker returns error without calling RunTask. When updater returns ErrTaskAlreadyClaimed for RUNNING, RunWorker returns ErrAlreadyClaimed (optional mock).

## Changes for review

- **Modified**: `internal/storage/entity/interfaces.go` — Add UpdateTaskStatusIf to TaskStore.
- **Modified**: `internal/storage/entity/task.go` — Implement UpdateTaskStatusIf.
- **Modified**: `internal/executor/scheduler.go` — Claim with UpdateTaskStatusIf(PENDING, SCHEDULED); on spawn failure revert with UpdateTaskStatus(..., "PENDING", ...).
- **Modified**: `internal/server/worker_handlers.go` — RUNNING path uses UpdateTaskStatusIf(SCHEDULED, RUNNING); 409 when !updated.
- **Modified**: `internal/executor/worker_api.go` — ErrTaskAlreadyClaimed; return it when resp.StatusCode == 409.
- **Modified**: `internal/workercmd/run.go` — Require status == "SCHEDULED"; on ErrTaskAlreadyClaimed return ErrAlreadyClaimed without RunTask.
- **Modified**: `cmd/buildmax-worker/main.go` — Exit 2 when RunWorker returns ErrAlreadyClaimed.
- **Tests**: entity (UpdateTaskStatusIf); scheduler (claim + revert); server (PATCH RUNNING 409/200); workercmd (optional).
- **Docs / UI**: Any place that displays task status should handle SCHEDULED (no API change; status string already exposed).
