# Design 017 - Todo write tool

## Goal

Define the structure and APIs for an agent tool **TodoWrite** that lets the agent create and update a session-scoped structured task list (pending, in progress, completed), so the user and agent can track multi-step progress. Session identity is carried in context; the tool stores one list per session in memory and replaces the list on each call.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/session** | Session identity and context. | `session.go` (existing); **add** context helpers for session ID in ctx. |
| **internal/agent** | Agent loop, Tool interface, tool invocation. Injects session ID into context before tool calls. | `agent.go` (**edit** processLoop to set ctx with session ID) |
| **internal/tools** | Concrete agent tools (Read, Write, WebFetch, **TodoWrite**). TodoWrite: session-scoped in-memory store, replace semantics. | `readfile.go`, `writefile.go`, `webfetch.go`; **new** `todowrite.go`, `todowrite_test.go` |
| **cmd/buildmax** | CLI entry, agent/session setup, tool construction. | `root.go` (wiring: add TodoWrite to tool list) |

## Structure

**Directory / files**

- `internal/session/` — session and context
  - `session.go` — **Edit**: add context key type, `CtxWithSessionID`, `SessionIDFromContext` (and `"context"` import)

- `internal/agent/` — agent loop
  - `agent.go` — **Edit**: in `processLoop`, set `ctx = session.CtxWithSessionID(ctx, sess.ID())` at start of loop; pass this ctx to `ChatWithTools` and `processOneToolCall`

- `internal/tools/` — agent tools
  - **`todowrite.go`** — TodoWrite struct (store map + mutex), NewTodoWrite, Name/Description/Parameters/Execute implementing `agent.Tool`
  - **`todowrite_test.go`** — Unit tests: missing session ID, valid replace, invalid todos, invalid status, store isolation, Name/Parameters

- `cmd/buildmax/` — CLI
  - `root.go` — **Edit**: create `todoWriteTool` via `tools.NewTodoWrite()`, pass it in the tool slice to `NewAgent`

**Main types and interfaces**

- **context key** (internal/session): Private type (e.g. `type ctxKey struct{}`) and package-level `var sessionIDKey = &ctxKey{}` so context values do not collide. Not exported.
- **TodoWrite** (internal/tools): Tool that holds `store map[string][]todoItem` (session ID → list) and `sync.RWMutex`. Implements `agent.Tool`: Name → `"TodoWrite"`, Description, Parameters (todos array), Execute(ctx, args). Reads session ID from ctx; replaces store[sessionID] with parsed todos; returns short confirmation.
- **todoItem** (internal/tools, unexported or local): Represents one todo: `content string`, `status string`, `active_form string`. Used when parsing args and storing; JSON/tool args use snake_case keys `content`, `status`, `active_form`.
- **Tool** (internal/agent): Unchanged. `Name()`, `Description()`, `Parameters() any`, `Execute(ctx, args) (string, error)`.

## Method design

| Receiver   | Method       | Signature | Responsibility |
|-----------|--------------|-----------|-----------------|
| (package) | **CtxWithSessionID** | `(ctx context.Context, id string) context.Context` | Return `context.WithValue(ctx, sessionIDKey, id)`. Session package. |
| (package) | **SessionIDFromContext** | `(ctx context.Context) (string, bool)` | Value from ctx for sessionIDKey; if missing return `("", false)`. Session package. |
| (package) | **NewTodoWrite** | `() (*TodoWrite, error)` | Allocate TodoWrite with `store: make(map[string][]todoItem)` and zero mutex. Return it. Tools package. |
| **TodoWrite** | **Name** | `() string` | Return `"TodoWrite"`. |
| **TodoWrite** | **Description** | `() string` | One short sentence for the LLM: create/update a structured task list for the current session; items have content, status (pending/in_progress/completed), active_form; use to track multi-step progress. |
| **TodoWrite** | **Parameters** | `() any` | JSON schema: `type: "object"`, `properties`: `todos` — array of objects with `content` (string), `status` (string), `active_form` (string, optional). `required`: `["todos"]`. Snake_case. |
| **TodoWrite** | **Execute** | `(ctx context.Context, args map[string]any) (string, error)` | Call `session.SessionIDFromContext(ctx)`; if !ok return error "session ID not in context". Get `todos` from args; if missing or not `[]any` return error. For each element, assert map; extract content, status, active_form (optional); validate status in `pending`, `in_progress`, `completed`; validate content non-empty. Replace `store[sessionID]` with parsed slice. Return e.g. `"Updated N todos."` or brief summary. Hold write lock for the replace. |

**Validation in Execute**

- `todos` must be present and type `[]interface{}` (or `[]any`). If not, return error e.g. "todos must be an array".
- Each element must be a map. Extract `content` (string, required), `status` (string, required), `active_form` (string, optional). If content is empty or status not in the three allowed values, return a clear error (e.g. "invalid todo item: content required, status must be pending, in_progress, or completed").

## How they work together

**Data/control flow**

1. **Setup**: `setupAgentAndSession` creates `todoWriteTool` via `NewTodoWrite()`, passes it with the other tools to `NewAgent(client, tools)`. Agent builds `toolDefs` and `toolsByName` including `"TodoWrite"`.
2. **Agent loop**: User message → `Process` / `ProcessAfterUserAppended` → `processLoop`. At the start of the loop, `ctx = session.CtxWithSessionID(ctx, sess.ID())`. Same ctx is used for `ChatWithTools(ctx, ...)` and for each `processOneToolCall(ctx, a, sess, tc)`. So when the LLM returns a tool_call with name `"TodoWrite"` and arguments `{"todos": [...]}`, `processOneToolCall` calls `tool.Execute(ctx, args)` with ctx carrying the session ID.
3. **Tool execution**: TodoWrite.Execute reads session ID from ctx; if missing, returns error (agent appends "error: ..." to session). Otherwise parses and validates `todos`, replaces store[sessionID], returns confirmation. Result is appended as tool-role message; loop continues or returns final reply.

**Dependencies**

- **internal/agent** depends on **internal/session** (already does); uses `session.CtxWithSessionID` in processLoop.
- **internal/tools** (todowrite.go) depends on **internal/agent** (Tool interface) and **internal/session** (SessionIDFromContext). No new dependency from session or agent to tools except at construction in cmd.
- **cmd/buildmax** imports tools and agent; constructs TodoWrite and passes it to NewAgent.

**Key data structures**

- **args** for TodoWrite: `map[string]any` with key `todos` (array of maps). Each map: `content` (string), `status` (string), `active_form` (string, optional). Produced by LLM JSON, consumed by TodoWrite.Execute.
- **store** in TodoWrite: `map[string][]todoItem` keyed by session ID string. Replaced in full on each successful Execute for that session.

## Changes for review

- **New**: `internal/session` context helpers — private context key type and variable; **CtxWithSessionID(ctx, id)** and **SessionIDFromContext(ctx)**. Add `"context"` import to session.go.
- **Modified**: `internal/agent/agent.go` — In **processLoop**, at the top of the for-loop (first line inside the loop), set `ctx = session.CtxWithSessionID(ctx, sess.ID())`. Use this ctx for the rest of the iteration (ChatWithTools and processOneToolCall). No other signature changes.
- **New**: `internal/tools/todowrite.go` — **TodoWrite** struct (store map[string][]todoItem, mutex); **NewTodoWrite() (*TodoWrite, error)**; **Name**, **Description**, **Parameters**, **Execute** implementing agent.Tool. Execute: read session ID from ctx; parse and validate todos; replace store[sessionID]; return confirmation. Snake_case for parameters.
- **New**: `internal/tools/todowrite_test.go` — Tests: Execute without session ID in ctx returns error; Execute with ctx from CtxWithSessionID and valid todos returns success and updates store; missing or invalid todos return error; invalid status returns error; two session IDs get isolated lists; Name/Description/Parameters and agent.Tool compile check.
- **Modified**: `cmd/buildmax/root.go` — In `setupAgentAndSession`, after webFetchTool: `todoWriteTool, err := tools.NewTodoWrite()`; on error log and return; pass `readFileTool`, `writeFileTool`, `webFetchTool`, `todoWriteTool` to `NewAgent`.

**Out of scope (this design)**

- Persisting todo lists to disk or session files
- TUI display of the todo list
- Partial updates (add one, remove one); replace-only semantics in this task
