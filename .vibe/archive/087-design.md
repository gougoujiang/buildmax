# Design 087: Scheduler optimization

## Goal

Make the chat-run scheduler use a one-way state machine: **PENDING → SCHEDULED → RUNNING → SUCCEEDED or FAILED**. When worker spawn fails (`runner.Run()` returns an error), set the run to **FAILED** with `error_message` and `ended_at`, sync the chat, and do **not** revert to PENDING.

## Modules


| Module                      | Responsibility                     | Changes                                                                                        |
| --------------------------- | ---------------------------------- | ---------------------------------------------------------------------------------------------- |
| **internal/executor**       | Scheduler loop, spawn-failure path | In `scheduler.loop()`: on Run() error → FAILED + SyncChatFromRun; no PENDING revert. Add test. |
| **internal/storage/entity** | No interface change                | `ChatRunStore` already has `UpdateChatRunStatus`, `SyncChatFromRun`. Use as-is.                |


## Structure

### Scheduler loop (internal/executor/scheduler.go)

- **loop()** — main poll loop. Current behavior on `runner.Run()` error: revert run to PENDING and continue. New behavior:
  1. When `s.runner.Run(ctx, *run)` returns `err != nil`:
    - Build `error_message`: use `err.Error()` (or a short prefix if very long; e.g. truncate to 500 chars to avoid huge DB field).
    - Build `ended_at`: `time.Now().Unix()`.
    - Call `s.chatRuns.UpdateChatRunStatus(ctx, run.ChatRunID, "FAILED", nil, &endedAt, nil, &errorMessage, nil, nil, nil)`.
    - If that succeeds, call `s.chatRuns.SyncChatFromRun(ctx, run.ChatRunID)`. Log if SyncChatFromRun fails but do not revert.
    - Log: `slog.Warn("scheduler: worker spawn failed, marking run as FAILED", "chat_run_id", run.ChatRunID, "err", err)`.
    - `continue` (do not call UpdateChatRunStatus with PENDING).
  2. Remove the existing block that reverts to PENDING and the log "reverting run to PENDING".
- **Comment** (optional): At top of `loop` or package doc, add one line: state machine is PENDING → SCHEDULED → RUNNING → SUCCEEDED/FAILED; spawn failure → FAILED (no revert to PENDING).

### Store usage

- **UpdateChatRunStatus(ctx, chatRunID, status string, startedAt, endedAt *int64, output, errorMessage, sessionID *string, promptTokens, completionTokens int)*  
Used with `status = "FAILED"`, `startedAt = nil`, `endedAt = now`, `errorMessage = err string`, others nil. Note: `UpdateChatRunStatus` for FAILED does not sync chat (it only syncs for PENDING/SCHEDULED/RUNNING per existing code), so we must call SyncChatFromRun after.
- **SyncChatFromRun(ctx, chatRunID)**  
Updates the chat row for the run’s `chat_id` from the run’s current row (last_run_id, status, output, started_at, ended_at, error_message, session_id). Called after setting run to FAILED so the chat shows the failed run.

## How they work together

1. Poll tick: `GetNextPendingChatRun` → one PENDING run (or nil).
2. Claim: `UpdateChatRunStatusIf(PENDING, SCHEDULED)` → run is SCHEDULED.
3. Spawn: `runner.Run(ctx, run)`.
4. **If spawn fails**:
  - `UpdateChatRunStatus(chatRunID, "FAILED", nil, &endedAt, nil, &errorMessage, nil, nil, nil)`.  
  - `SyncChatFromRun(ctx, chatRunID)`.  
  - Log and continue. Next poll will not see this run (it is FAILED).
5. **If spawn succeeds**:
  - `UpdateChatRunWorkerInfo(...)`. Worker later PATCHes SCHEDULED→RUNNING and then SUCCEEDED/FAILED; existing handler syncs chat on FAILED. No change.

## Tests

- **internal/executor/scheduler_test.go** (new file) or add to **executor_test.go**:
  - **TestScheduler_Loop_SpawnFailure_MarksRunFailed** (or similar name):
    - Use a **spy store** that implements `ChatRunStore` and records:
      - Last `UpdateChatRunStatus` call: (chatRunID, status, endedAt, errorMessage).
      - Whether `SyncChatFromRun` was called and with which chatRunID.
    - Use a **failing runner** that implements `WorkerRunner` and returns an error from `Run`.
    - Start the scheduler (e.g. with a short poll interval), ensure one tick runs (e.g. inject a PENDING run via the spy or use a store that returns one run then nil), then stop.
    - Assert: `UpdateChatRunStatus` was called with `status == "FAILED"`, `endedAt != nil`, `errorMessage != nil` and non-empty string.
    - Assert: `UpdateChatRunStatus` was **not** called with `status == "PENDING"`.
    - Assert: `SyncChatFromRun` was called with the same `chatRunID` as the run returned by GetNextPendingChatRun.
  - Implementation note: the scheduler runs in a goroutine and uses a ticker; the test can use a store that returns one run on first GetNextPendingChatRun and nil thereafter, and a runner that always fails, then wait for at least one tick (e.g. poll interval 10ms and sleep 20ms) before stopping. Then assert on the spy’s recorded calls.

## Changes for review


| Area                                                          | Change                                                                                                                                                     |
| ------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **internal/executor/scheduler.go**                            | In `loop()`: replace “revert to PENDING” block with “set FAILED + SyncChatFromRun”; update log message; optional one-line state-machine comment.           |
| **internal/executor/scheduler_test.go** (or executor_test.go) | New test: spawn failure → UpdateChatRunStatus(FAILED, endedAt, errorMessage), SyncChatFromRun called, no PENDING update. Use spy store and failing runner. |
| **internal/storage/entity**                                   | No code change.                                                                                                                                            |


