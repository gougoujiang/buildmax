# Design 109 - User only communicate with Tier 1

## Goal

When a Tier 2 task run reaches terminal status (SUCCEEDED/FAILED), the server automatically triggers a Tier 1 conversation turn that summarizes the result and streams it to the user via the conversation WebSocket — removing all direct user↔Tier 2 interaction from the portal.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/server/portal** | WebSocket handler, connection registry, portal API | `ConnRegistry`, `wsConn`, `wsSink`, `Handler` |
| **internal/server/worker** | Worker API handler; fires completion callback | `Config.OnTaskRunTerminal`, `Handler` |
| **internal/server/server.go** | Wires registry, callback, and services together | `Server`, `New()` |
| **internal/app/conversation** | Tier 1 orchestration; existing `HandleTurn` reused | `Service`, `HandleTurnCmd` |
| **internal/conversation** | Low-level LLM loop; system prompt | `systemPrompt`, `Run` |
| **internal/wsconn** | WebSocket protocol types | Event type constants, payload types |
| **portal/src/** | Frontend; remove TaskDetail, task subscriptions | Pages, router, ws.ts, AppRouter |

## Structure

**Directory / files**

- `internal/server/portal/`
  - `conn_registry.go` — **new**; `ConnRegistry` type: thread-safe map of userID → `[]*wsConn`
  - `ws_handler.go` — **modified**; register/unregister on connect/disconnect; add `runSystemConversationTurn`
  - `config.go` — **modified**; add `ConnRegistry` field
  - `register.go` — unchanged
- `internal/server/worker/`
  - `config.go` — **modified**; add `OnTaskRunTerminal` callback field
  - `handlers.go` — **modified**; fire callback after terminal status update
- `internal/server/`
  - `server.go` — **modified**; create `ConnRegistry`, wire callback
- `internal/conversation/`
  - `agent.go` — **modified**; update system prompt
- `internal/wsconn/`
  - `protocol.go` — **modified** (minor); remove task subscription types or mark unused
- `portal/src/`
  - `pages/TaskDetail.tsx` — **deleted**
  - `features/tasks/` — **deleted** or gutted (remove direct task API usage)
  - `components/AppRouter.tsx` — **modified**; remove task route
  - `router.ts` — **modified**; remove task route/segment
  - `lib/api/ws.ts` — **modified**; remove `subscribeTask`/`unsubscribeTask`
  - `hooks/useTasks.ts` — **deleted** or unused
  - `lib/taskUtils.ts` — **deleted** or unused

**Main types and interfaces**

- **ConnRegistry** (portal): Thread-safe registry of active WebSocket connections per user. Methods: `Register`, `Unregister`, `ForUser`.
- **OnTaskRunTerminal** (worker.Config): Callback function `func(ctx, taskRunID, taskID, conversationID, userID, status, output, errorMsg)` fired when a run reaches terminal status.
- **wsSink** (portal): Existing type; unchanged. Used by system-triggered turns to stream deltas to the user's WebSocket.

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|-----------------|
| **ConnRegistry** | Register | `(userID string, c *wsConn)` | Add connection to user's set |
| **ConnRegistry** | Unregister | `(userID string, c *wsConn)` | Remove connection from user's set |
| **ConnRegistry** | ForUser | `(userID string) []*wsConn` | Return snapshot of active connections for user |
| **wsConn** | runSystemConversationTurn | `(ctx, conversationID, message string)` | Acquire turnMu (blocking), run Tier 1 turn with channel="system", stream result |
| **Handler** (worker) | handlePatchTerminalStatus | (existing, modified) | After DB update + Hub.Done, fire `OnTaskRunTerminal` callback in goroutine |
| **Server** | New | (existing, modified) | Create ConnRegistry, wire OnTaskRunTerminal callback that looks up user connections and triggers system turn |

## How they work together

**Data/control flow — task completion → Tier 1 response**

1. **Worker finishes** task run → calls `PATCH /api/worker/task-runs/{id}` with `status=succeeded`, `output=<text>` (or `status=failed`, `error_message=<msg>`).
2. **Worker handler** `patchTaskRun` → `handlePatchTerminalStatus`:
   - Updates run in DB (`UpdateRun`)
   - Calls `OnRunComplete` (for succeeded) or `SyncTaskFromRun` (for failed)
   - Calls `Hub.Done(taskID)` to clean up stream buffers
   - **New**: Fires `cfg.OnTaskRunTerminal(ctx, taskRunID, taskID, conversationID, userID, status, output, errorMsg)` in a goroutine
3. **OnTaskRunTerminal callback** (wired in `server.New`):
   - Calls `connRegistry.ForUser(userID)` to find the user's active WebSocket connections
   - If no connections: log and return (user offline; out of scope)
   - Builds the task result notification message (see format below)
   - For the **first** active connection: calls `wc.runSystemConversationTurn(ctx, conversationID, message)`
4. **wsConn.runSystemConversationTurn**:
   - `wc.turnMu.Lock()` — blocks until any in-progress user turn finishes
   - Creates `wsSink{c: wc, conversationID: conversationID}`
   - Calls `convapp.Service.HandleTurn(ctx, HandleTurnCmd{UserID, Channel: "system", Message, ConversationID, StreamSink})`
   - HandleTurn → `handleConversationTurn` → `coreconv.Run` → LLM loop with system prompt + conversation history + task result message
   - LLM generates summary response, streamed via `wsSink.OnDelta` → `conversation.message.delta` events
   - On completion: sends `conversation.message.completed` event
   - `wc.turnMu.Unlock()`

**Task result notification message format**

For succeeded:
```
[Task Result] task_id: <taskID> | status: succeeded

<output, truncated to 4000 chars>
```

For failed:
```
[Task Result] task_id: <taskID> | status: failed

Error: <error message>
```

The LLM sees this as a "user" message in the conversation (persisted with `channel="system"`) and uses the system prompt instructions to generate a natural summary.

**Dependencies**

- `internal/server/portal` depends on `internal/app/conversation` for `HandleTurn` (existing)
- `internal/server/worker` depends on nothing new; the callback is a plain function injected via Config
- `internal/server/server.go` depends on `portal.ConnRegistry` and wires everything

**Key data structures**

- **ConnRegistry.conns** (`map[string][]*wsConn`): created in `server.New`, shared between portal handler (register/unregister) and the task completion callback (lookup). Protected by `sync.RWMutex`.
- **HandleTurnCmd** (convapp): existing struct; reused with `Channel: "system"` for system-triggered turns. No new fields.
- **worker.Config.OnTaskRunTerminal**: callback function, called by the worker handler, implemented as a closure in `server.New` that captures `connRegistry` and `convapp.Service` dependencies.

**Connection lifecycle**

```
wsUpgradeHandler  →  create wsConn  →  connRegistry.Register(userID, wc)
                                            ↓
                              readLoop / writeLoop (active)
                                            ↓
                              cleanup()  →  connRegistry.Unregister(userID, wc)
```

**Turn serialization**

Both user-initiated and system-triggered turns use `wsConn.turnMu`:
- User turn: `TryLock()` — fails immediately if busy (returns error to user)
- System turn: `Lock()` — blocks until the current turn finishes, then proceeds

This guarantees at most one turn runs per connection at any time, and system turns are never dropped — they queue behind any in-progress turn.

## Portal frontend changes

**Removed**
- `portal/src/pages/TaskDetail.tsx` — deleted
- `portal/src/features/tasks/` — remove `useTaskDetail`, `TaskDetailView`, and direct task API calls (`getTask`, `createTaskRun`, `getTaskConversation`, `subscribeTaskStream`). Keep `getTasks`/`getTasksPaginated`/`createTask` if they are used by NewConversation or AgentList for task creation (Tier 1 flow).
- `portal/src/hooks/useTasks.ts` — delete or reduce to what's needed
- `portal/src/lib/taskUtils.ts` — delete
- `portal/src/components/ArtifactContentModal.tsx` — delete (artifact viewing is out of scope)

**Modified**
- `portal/src/router.ts` — remove `task` segment from `SEGMENT`, `parseHash`, `buildHash`; remove `task` from `Route` union type
- `portal/src/components/AppRouter.tsx` — remove `task` route branch, remove TaskDetail import, remove `pendingTask`/`setPendingTask` usage, remove `useTasks` if no longer needed
- `portal/src/lib/api/ws.ts` — remove `subscribeTask()`, `unsubscribeTask()`, `_pendingTaskSubs`, `restoreSubscriptions()`
- `portal/src/App.tsx` — remove `ArtifactContentModal` and `viewArtifact` state
- `portal/src/lib/types.ts` — remove `Task` type and task-related route variants if no longer used

**Unchanged**
- `portal/src/pages/ConversationDetail.tsx` — already works; receives `conversation.message.delta` and `conversation.message.completed` events for both user-initiated and system-triggered turns
- `portal/src/features/conversations/` — already handles streaming conversation messages via WebSocket

## System prompt update

Current prompt in `internal/conversation/agent.go`:
```
...
- StartTask: create and schedule a new background task. Always tell the user the task_id and run_id and where to check progress (Activity or task detail). Do not claim the work is done immediately—the task runs in the background.
...
```

Updated:
```
...
- StartTask: create and schedule a new background task. Tell the user you have started a task and will report back when it completes. Do not provide internal task or run IDs to the user. Do not tell the user to check a task detail page — you will deliver the result directly.
...

When you receive a message starting with "[Task Result]", it means a background task you started has completed. Read the status and output, then:
- If succeeded: summarize the result clearly and concisely for the user. Present key findings naturally.
- If failed: explain what went wrong and suggest next steps (e.g. retry, provide more info).
Do not mention task IDs, run IDs, or internal system details. Speak to the user as their assistant.
```

## Changes for review

- **New**: `internal/server/portal/conn_registry.go` — `ConnRegistry` type (Register, Unregister, ForUser)
- **Modified**: `internal/server/portal/config.go` — add `ConnRegistry *ConnRegistry` field
- **Modified**: `internal/server/portal/ws_handler.go` — register/unregister wsConn; add `runSystemConversationTurn` method
- **Modified**: `internal/server/worker/config.go` — add `OnTaskRunTerminal` callback field
- **Modified**: `internal/server/worker/handlers.go` — fire callback after `handlePatchTerminalStatus`
- **Modified**: `internal/server/server.go` — create ConnRegistry, pass to portal config, wire OnTaskRunTerminal closure
- **Modified**: `internal/conversation/agent.go` — update `systemPrompt` for task result handling
- **Deleted**: `portal/src/pages/TaskDetail.tsx`
- **Deleted**: `portal/src/components/ArtifactContentModal.tsx`
- **Modified**: `portal/src/components/AppRouter.tsx` — remove task route
- **Modified**: `portal/src/router.ts` — remove task segment/route
- **Modified**: `portal/src/lib/api/ws.ts` — remove task subscription methods
- **Modified**: `portal/src/App.tsx` — remove artifact modal
- **Deleted/reduced**: `portal/src/features/tasks/`, `portal/src/hooks/useTasks.ts`, `portal/src/lib/taskUtils.ts`
