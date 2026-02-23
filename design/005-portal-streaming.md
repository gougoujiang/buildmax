# Portal Streaming: Design Options

**Design reference for adding streaming assistant text to the Portal.**  
*Context: Task 080 added streaming in the CLI (print + TUI); the Portal still loads the full conversation only after a run completes.*

**Implementation status:** Phases 1–3 are implemented. Worker streams stdout to server (POST stream); server hub + GET SSE endpoint; Portal subscribes after createChatRun and shows streaming reply until done, then refetches conversation.

---

## 0. User vs internal model (chat vs chat-run)

**Principle:** Chat run is an internal concept (server ↔ worker). The user is not exposed to “runs”; they only see “chat”: they start a chat on the New Chat page, then continue the conversation on the chat detail page.

| Layer | User-facing (Portal ↔ Server) | Internal (Server ↔ Worker) |
|-------|-------------------------------|----------------------------|
| **Concepts** | **Chat** only: create chat, get conversation, get stream for “this chat” | **Chat run**: worker gets run by id, reports deltas/status per run |
| **Session** | Allocated when the user submits a new chat; if “no session” (e.g. conversation 404), treat as “chat just started” and show initial user input | Session ID is created at chat creation and passed through to the worker so the same ID is used from the start |
| **Streaming** | Portal gets stream **by chat**: e.g. `GET .../chats/{chat_id}/stream` (no run_id in URL) | Worker still reports per run: `POST /api/worker/chat-runs/{run_id}/stream`; server maps run → chat and keys hub by chat_id |
| **Swagger / APIs** | User-facing APIs use **chat** (and workspace); no chat_run_id in Portal-facing paths | Worker APIs keep **chat_run** in the path; these are internal and not exposed to the user |

**Concrete changes (to implement):**

1. **Session ID at chat creation**  
   When a new chat is submitted (create chat), allocate a **session_id** and persist it on the chat immediately. Pass this session_id through to the worker (e.g. in the run payload or via chat) so the agent/CLI uses it from the very beginning. This ensures a session is “allocated” at the start and avoids races.

2. **No session / conversation not found**  
   If the conversation API returns “not found” (e.g. no session file in blob yet), treat this as “chat just started.” The chat detail page can show the user’s initial input first (already supported in the Portal) and optionally poll or subscribe to the stream until the first run produces a conversation.

3. **Stream by chat (user-facing)**  
   The Portal should get the stream **by chat**, not by run:
   - **User API:** e.g. `GET /api/workspaces/{workspace_id}/chats/{chat_id}/stream` (no run_id). The client subscribes to “this chat’s” stream; at most one run is active per chat, so this is well-defined.
   - **Existing run-scoped URL** (e.g. `GET .../chats/{chat_id}/runs/{run_id}/stream`) can remain for backward compatibility or be deprecated in favor of the chat-scoped endpoint.

4. **Stream hub keyed by chat_id**  
   The worker (each chat-run) still reports to the server using run-scoped endpoints, but the **stream hub** should be keyed by **chat_id**, not chat_run_id:
   - Worker: `POST /api/worker/chat-runs/{run_id}/stream` with `{ "delta": "..." }` (unchanged).
   - Server: resolve run_id → chat_id (e.g. via ChatRunStore); then `hub.Append(chat_id, delta)` and `hub.Done(chat_id)` when the run completes. So all run-level traffic is translated to chat-level keys in the hub.
   - User: `GET .../chats/{chat_id}/stream` subscribes to `hub.Subscribe(chat_id)`, receiving deltas for whichever run is currently active for that chat.

This keeps the user model simple (one chat, one stream) while preserving the internal run-based model for scheduling and worker communication.

**Current implementation (Phases 1–3)** uses run-scoped hub keys and run-scoped user stream URL (`GET .../chats/{chat_id}/runs/{run_id}/stream`). The Portal currently receives `chat_run_id` from `createChatRun` and subscribes using that run_id. **Next steps** are to: (1) allocate session_id at chat creation and pass it to the worker; (2) add chat-scoped stream endpoint and key the hub by chat_id; (3) have the Portal subscribe by chat_id only (no run_id in the URL).

### 0.1 Implementation checklist

Status as of doc update: **Done** = implemented; **Partial** = partly there; **No** = not done.

| # | Area | Task | Status | Notes |
|---|------|------|--------|--------|
| 1 | **Storage / entity** | Allocate `session_id` when creating a chat (e.g. in `CreateChat` or handler). Persist on the chat row. | **Done** | `entity/chat.go` `CreateChat` allocates a UUID (`uuid.New().String()`) for session_id; session is not exposed to user, buildmax CLI expects UUID. |
| 2 | **Server (create chat)** | Ensure create-chat response and DB state include `session_id`. | **Done** | Chat returned from store now has session_id from creation. |
| 3 | **Worker API (get run)** | Include `session_id` in the run/chat payload returned to the worker (e.g. `GET /api/worker/chat-runs/{id}`). | **Done** | `GetChatRunResponse.Chat.SessionID` is already returned (`worker_handlers.go`, `workerapi/types.go`). Value is nil until 1 is done. |
| 4 | **Worker (run task)** | Pass `session_id` from API response into the run environment or CLI args so the agent uses it. | **Done** | `workercmd/run.go`: uses `chat.SessionID` when present, else generates UUID. `executor.RunTask` receives `sessionID` and passes it to `runBuildmaxCmd` (e.g. `--session-id`). Once 1 is done, worker will use server-allocated id. |
| 5 | **Stream hub** | Change hub to be keyed by **chat_id** instead of chat_run_id. | **Done** | `stream_hub.go`: interface and impl use `chatID` (Append/Buffer/Done/Subscribe all keyed by chat_id). |
| 6 | **Server (worker stream)** | In `POST /api/worker/chat-runs/{run_id}/stream`: resolve run_id → chat_id (via ChatRunStore), then `hub.Append(chat_id, delta)`. | **Done** | `postWorkerStreamHandler` calls `GetChatRunWithChat` then `hub.Append(run.ChatID, req.Delta)`. |
| 7 | **Server (worker PATCH)** | When worker PATCHes run SUCCEEDED/FAILED: call `hub.Done(chat_id)` (resolve run_id → chat_id). | **Done** | `patchWorkerChatRunHandler` resolves run → chat_id and calls `hub.Done(run.ChatID)`. |
| 8 | **Server (user stream)** | Add `GET /api/workspaces/{workspace_id}/chats/{chat_id}/stream` (no run_id). Auth: workspace + chat ownership. Subscribe to `hub.Subscribe(chat_id)`; send buffer then live deltas then "done". | **Done** | `getChatStreamHandler` in `stream_handlers.go`; route in `server.go`. |
| 9 | **OpenAPI / Swagger** | Document the new chat-scoped stream endpoint for user API. Keep worker stream endpoints as internal (or clearly marked). | **Done** | `openapi.json`: path `GET .../chats/{chat_id}/stream` added. |
| 10 | **Portal** | After create chat or create run: subscribe to `GET .../chats/{chat_id}/stream` (no run_id). No need to pass or store chat_run_id for streaming. | **Done** | `subscribeChatStream(workspaceId, chatId, token, callbacks)` in `api/index.ts`; ChatDetail subscribes on mount and does not use run_id for streaming; follow-up only calls `createChatRun`. |
| 11 | **Conversation 404** | When conversation API returns 404 (no session file yet): Portal already shows initial user input; ensure session_id on chat doesn’t change this (404 when file missing is still valid). | **Done** | Chat detail shows `initialInput` when session is null or empty; conversation API can still 404 until session file exists in blob. |

**Summary:** All items implemented. **1, 2**: session_id at chat creation. **3, 4**: worker API returns and uses session_id. **5–7**: hub keyed by chat_id; worker POST/PATCH map run → chat_id. **8–9**: chat-scoped GET stream + OpenAPI. **10**: Portal subscribes by chat on mount; no run_id for streaming. **11**: initial input on 404 (unchanged).

**Ordering:** 1–2 (session at creation) first. Then 5–7 (hub keyed by chat_id, worker POST/PATCH map to chat_id), then 8–9 (user stream endpoint + docs) and 10 (Portal).

---

## 1. Current State

### 1.1 Where streaming exists today

- **CLI (task 080)**: `internal/llm.ChatWithToolsStream` and `internal/agent.StreamSink` stream content deltas. Print mode writes deltas to stdout; TUI appends to a streaming buffer and refreshes the viewport. Session is still updated only with the full message when the turn completes.
- **StreamSink** (agent): `OnDelta(delta string)`; implementations in `internal/cmd` (stdout) and `internal/tui` (channel → `streamDeltaMsg`).

### 1.2 Where streaming does not exist

- **Worker**: `internal/executor.runBuildmaxCmd` uses `cmd.CombinedOutput()`, so it only receives the full stdout when the process exits. CLI streams to stdout, but the worker never sees incremental output.
- **Server**: No endpoint that pushes deltas. `GET .../chats/{chat_id}/conversation` returns the full session JSON in one response (session is read from blob/local after the run’s `global/` is uploaded).
- **Portal**: After `createChatRun`, it polls `getChats` every 2s until status is SUCCEEDED/FAILED, then `refetchSession()` → `getChatConversation` to get the whole conversation. No incremental display.

### 1.3 Data flow today

```
User (Portal) → createChatRun → Worker runs buildmax -p (CombinedOutput)
                                     → exit → upload global, PATCH SUCCEEDED
Portal polls getChats → terminal status → getChatConversation (full session from blob)
```

So: **streaming is confined to the CLI**. To show streaming in the Portal we need (1) worker → server deltas, and (2) server → browser delivery.

---

## 2. Options for Portal Streaming

### 2.1 Option A: SSE + worker pushes deltas (true streaming)

**Idea**

- **Worker**: Replace `CombinedOutput()` with streaming stdout (e.g. `cmd.StdoutPipe()` + goroutine). For each chunk/delta, POST to server, e.g. `POST /api/worker/chat-runs/{id}/stream` with body `{"delta": "..."}`.
- **Server**:  
  - New endpoint: `GET /api/workspaces/{id}/chats/{id}/runs/{run_id}/stream` returning `text/event-stream`. For each delta received from the worker for that run, write an SSE event to subscribers; on worker PATCH SUCCEEDED/FAILED, send a “done” event and close.  
  - In-memory (or Redis) store: run_id → subscribers and optional buffer.
- **Portal**: After `createChatRun`, open `EventSource(streamUrl)` (auth via query param if needed). Append each event to the “current assistant” message; on “done”, refetch session once and stop the stream.

**Pros**: Real streaming; best UX.  
**Cons**: Worker and server changes; SSE auth (e.g. token in query); lifecycle (timeouts, cleanup when no subscribers).

---

### 2.2 Option B: Polling with partial output (“live tail”)

**Idea**

- **Worker**: Read stdout incrementally; periodically (e.g. every 500 ms or every N bytes) PATCH with a new field such as `output_so_far` (or append-only buffer). Final PATCH still sends full output and SUCCEEDED.
- **Server**: Store “partial output” per run (in-memory or short-lived store). Extend conversation or run API so that while status is RUNNING, the response includes the current partial assistant text (e.g. last message content = buffer).
- **Portal**: After `createChatRun`, poll conversation (or run status) more frequently (e.g. every 500 ms). Render the last message as “assistant typing…” with partial content; when status becomes SUCCEEDED, one final refetch.

**Pros**: No SSE/WebSocket; reuses REST + polling.  
**Cons**: Not true streaming; more requests; possible duplication if not designed carefully.

---

### 2.3 Option C: WebSocket

**Idea**: Same as Option A, but the Portal opens a WebSocket to the server; the server subscribes to “run X” and when the worker pushes deltas (e.g. via HTTP to an internal endpoint), the server forwards them over the WebSocket.

**Pros**: Full duplex; auth once per connection.  
**Cons**: More infrastructure (WebSocket on server, connection handling); worker→server delta channel still required as in A.

---

### 2.4 Option D: Agent runs in server process

**Idea**: For “streaming” runs, do not use the worker: the server runs the agent in-process with a `StreamSink` that writes to a channel; the HTTP handler uses SSE and reads from that channel.

**Pros**: Reuses task 080 streaming path; no worker→server streaming.  
**Cons**: Server must run the agent (LLM + tools); different scaling and deployment; likely out of scope for current architecture.

---

## 3. Recommendation

- **Short term / minimal change**: **Option B (polling + partial output)**  
  - Worker: stream stdout and periodically PATCH “output so far” (new field or endpoint).  
  - Server: store partial output per run; expose in existing or extended conversation/run API.  
  - Portal: poll more frequently while RUNNING and show partial last message; stop and refetch on SUCCEEDED/FAILED.  
  Improves UX without adding SSE/WebSocket or new auth patterns.

- **Medium term / best UX**: **Option A (SSE + worker pushes deltas)**  
  - Add worker→server delta channel (HTTP POST chunks), server→Portal SSE, and run-scoped subscriber map (and optional buffer).  
  Keeps “run in worker” model and gives real-time streaming in the Portal.

---

## 4. Conversation persistence: session JSON vs DB

### 4.1 Current state

- **DB**: Holds only **pointers and denormalized summary** for conversation:
  - `chat`: `session_id`, `last_run_id`, and denormalized fields (e.g. `output`, `title`, `status`).
  - `chat_run`: `output` (full assistant text for that run), `session_id`, `status`, etc.
- **Blob (run global/)**: Holds the **full conversation**: session JSON file `sessions/<session_id>.json` with `id`, `title`, `messages[]` (each with `role`, `content`, `tool_calls`, etc.). Written by the CLI and uploaded by the worker when the run completes.
- **Conversation API**: Reads the session file from blob (or local workspace dir) and returns it; no message data is read from the DB.

So today the **canonical conversation** (full message list) lives only in the session JSON file in blob, not in the DB.

### 4.2 Do we need to persist conversation in the DB?

**For true streaming (Option A), no.** Streaming works with the current model:

- **Live deltas**: Pushed over SSE from an in-memory (or short-lived) buffer; no need to persist each delta to DB.
- **Final state**: When the run completes, the worker uploads `global/` (including the session file) and PATCHes SUCCEEDED. The conversation API keeps reading the full conversation from that session file in blob. So we do **not** need to duplicate the message list into the DB just to enable streaming.

**Reasons you might add DB persistence for messages anyway:**

| Reason | Benefit |
|--------|--------|
| **Single source of truth** | Chats, runs, and artifacts are in DB; putting messages there simplifies backups, replication, and “everything in one store.” |
| **Consistency** | If blob upload fails after run succeeds, DB has status but conversation is missing. Storing messages in DB (e.g. on run complete) gives a fallback or primary source. |
| **Queryability** | Search message content, filter by role, paginate messages, or do analytics without loading full session JSON from blob. |
| **Reconnection / polling fallback** | If we ever want “show partial reply after reconnect,” storing partial or final assistant message in DB (or a dedicated column) can support that without relying only on in-memory stream buffer. |

**Reasons to keep conversation in blob only:**

- **Simplicity**: No new tables or sync logic; session format is already rich (tool_calls, etc.) and matches the CLI/agent.
- **Blob as run output**: The run produces a session file in `global/`; that is the run’s artifact. Design 003 keeps bulk content in blob and metadata in DB.

### 4.3 Recommendation

- **For true streaming (Option A)**: **No requirement to persist conversation in the DB.** Keep the current model: final conversation stays in session JSON in blob; SSE delivers live deltas from an in-memory (or ephemeral) buffer; when the stream ends, the Portal can refetch conversation from the existing API (blob-backed).
- **Optional later**: If you want a single source of truth, search, or robustness against blob failures, add a `conversation` or `chat_message` store in the DB. The worker (or a post-completion step) would write the final session into DB when the run completes, in addition to uploading blob. The conversation API could then read from DB when present and fall back to blob, or read from DB only.

---

## 5. Task size evaluation (Option A: true streaming)

### 5.1 Scope summary

| Area | Work items | Est. size |
|------|------------|-----------|
| **Worker** | Replace `CombinedOutput()` with streaming stdout; send each chunk to server via new HTTP endpoint; still accumulate full output for persist/PATCH. | Medium |
| **Server (worker API)** | New `POST /api/worker/chat-runs/{id}/stream` (worker auth); accept `{"delta":"..."}`; append to run buffer and broadcast to SSE subscribers. | Small–medium |
| **Server (user API + hub)** | In-memory stream hub (run_id → subscribers + buffer); new `GET .../runs/{run_id}/stream` returning SSE; user auth (workspace/chat/run ownership); on worker PATCH SUCCEEDED/FAILED, emit "done" and close subscribers. | Medium–large |
| **Portal** | After createChatRun, open SSE (or fetch+ReadableStream) to stream URL; render "current assistant" message that grows with each event; on "done", refetch conversation and close stream; reduce or replace polling. | Medium |
| **Auth for SSE** | EventSource does not send `Authorization` in some browsers; either token in query (simpler but sensitive) or fetch()+ReadableStream for SSE (no token in URL). | Small |
| **Tests** | Executor: mock stream receiver, assert deltas and final output. Server: stream hub unit tests; handler tests for POST stream + GET stream + auth. Portal: optional E2E or manual. | Medium |

**Rough total**: on the order of **3–5 days** for one developer (assuming no major surprises). The work spans three codebases (executor, server, portal), new concurrency (hub), and a new protocol (SSE). Doing it in one shot is possible but carries risk: a mistake in the hub or auth can block everything, and debugging across worker → server → Portal is harder.

### 5.2 Recommended: three sub-phases

Breaking into phases gives shippable milestones, easier review, and a fallback at each step (e.g. keep polling if SSE is delayed).

**Phase 1 — Worker streams stdout to server (backend-only)**  
- **Goal**: Worker reads stdout incrementally and POSTs deltas to the server; server stores them in a run-scoped buffer (in-memory hub). No SSE yet; no Portal changes.  
- **Deliverables**:  
  - Executor: `runBuildmaxCmd` streams stdout; new interface or callback to send deltas (e.g. `StreamSender` or extend updater); worker calls `POST /api/worker/chat-runs/{id}/stream` for each chunk (or batched/flushed).  
  - Server: `POST /api/worker/chat-runs/{id}/stream` (worker auth); hub that maps run_id → buffer, appends delta.  
  - Optional: GET endpoint that returns current buffer (for testing or Phase 2 polling fallback).  
- **Exit condition**: Worker run causes server to receive and store deltas; full output still used for persist and PATCH. Tests for executor and server.  
- **Size**: ~1–1.5 days.

**Phase 2 — Server exposes SSE for a run**  
- **Goal**: User-facing SSE endpoint; when the Portal (or curl) subscribes to a run, it receives stored buffer (if any) plus live deltas, then a "done" event when the run completes.  
- **Deliverables**:  
  - Stream hub: subscribe(run_id) → channel; broadcast deltas and "done"; cleanup on run end or TTL.  
  - `GET /api/workspaces/{id}/chats/{id}/runs/{run_id}/stream` (or equivalent) with user auth (workspace/chat/run); return `text/event-stream`; auth via query param or fetch with Authorization.  
  - When worker PATCHes SUCCEEDED/FAILED, hub gets notified (e.g. from PATCH handler or a shared hook), sends "done", closes subscribers.  
- **Exit condition**: Subscribing to a run stream returns buffered + live deltas and "done". Manual or automated test with a running worker.  
- **Size**: ~1–1.5 days.

**Phase 3 — Portal consumes SSE and shows streaming reply**  
- **Goal**: Chat detail view opens the stream after createChatRun, shows assistant message growing in real time, then finalizes on "done".  
- **Deliverables**:  
  - Portal: build stream URL (with auth); open via EventSource (token in query) or fetch + ReadableStream; append events to a "streaming" message; on "done", refetch conversation and close stream.  
  - Simplify or remove the 2s polling used only for refetching session (keep or adjust sidebar refresh as needed).  
  - Error handling: stream error, run failed, navigate away (close stream).  
- **Exit condition**: User sees streaming reply in the UI; final state matches refetched conversation.  
- **Size**: ~1 day.

**Dependencies**: 1 → 2 → 3. Each phase can be merged and deployed independently (Phase 1 and 2 are backward compatible; Phase 3 can fall back to polling if stream fails).

### 5.3 One-shot vs phased

| Approach | Pros | Cons |
|----------|------|------|
| **One shot** | Single PR; no temporary endpoints. | Large change; hard to review; if one part fails, entire feature blocked; debugging across worker/server/portal is heavier. |
| **Phased (recommended)** | Smaller PRs; test worker→server then server→client; can ship Phase 1+2 and use stream from curl/scripts before Portal; rollback or pause at any phase. | Slightly more total time (coordination, hub built in two steps); may need a small "buffer GET" or internal hook for Phase 1 testing. |

**Recommendation**: Use **three sub-phases** as above. If the team is very small and prefers one PR, at least implement and test in the same order (worker → server hub → SSE endpoint → Portal) and consider a feature flag so streaming can be turned off until stable.

---

## 6. Multi-instance scaling impact

The current design uses an **in-memory stream hub** per server process: run_id → buffer + SSE subscribers. That works for a **single instance** only.

### 6.1 What breaks with multiple instances

- **Worker** POSTs deltas to `POST /api/worker/chat-runs/{id}/stream`. The load balancer may send that request to **instance A**.
- **User** opens `GET .../runs/{run_id}/stream` for SSE. The LB may send that to **instance B**.
- Instance A’s hub receives all deltas and has no SSE clients for that run. Instance B has the SSE client but its hub never receives deltas. **Result: the user sees no streamed content.**

So with a per-process in-memory hub, streaming only works if **worker stream POSTs and user SSE GETs for the same run_id hit the same instance**. That is not guaranteed with a normal stateless LB.

### 6.2 Options when scaling out

| Option | Idea | Pros | Cons |
|--------|------|------|------|
| **Sticky sessions (affinity)** | LB routes all requests that include a given run_id (e.g. path or header) to the same instance. | No new infra; in-memory hub still works. | LB must support affinity by run_id; scaling is less flexible; instance that “owns” the run must stay up for the duration of the stream. |
| **Shared pub/sub (e.g. Redis)** | Hub does not store state in process. Worker POST → any instance → **publish** to Redis channel `run:{run_id}`. Every instance **subscribes** to Redis for runs it cares about (e.g. runs for which it has an open SSE connection). When a delta is published, the instance that has the SSE client forwards it. Buffer can live in Redis (e.g. Redis Streams) so late-join or replay is consistent. | Works with any number of instances; no affinity required; worker and user can hit different instances. | Requires Redis (or similar); hub implementation becomes “publish/subscribe + optional Redis buffer” instead of “in-memory only”; ops dependency. |
| **Dedicated stream gateway** | One (or a small fixed set of) instance(s) handle only stream endpoints (worker POST + user GET). Rest of API scales horizontally. | Clear separation; rest of server stays stateless. | Gateway is a bottleneck and single point of failure unless it too is clustered with shared state (back to Redis). |

### 6.3 Recommendation

- **Single instance (today)**: In-memory hub is fine; no change.
- **When you scale to multiple instances**: Prefer **shared pub/sub (Redis)** so that:
  - Worker can POST to any instance; that instance publishes deltas (and optionally “done”) to a run-scoped channel.
  - User GET stream can land on any instance; that instance subscribes to the run’s channel and forwards events to the SSE response. If the buffer is in Redis, the same instance can send “catch-up” buffer then live events.
  - Design the hub behind an **interface** (e.g. “append delta”, “subscribe”, “notify done”) so the in-memory implementation can be swapped for a Redis-backed one without changing the HTTP handlers.

No need to implement Redis for Phase 1–3; just keep the hub behind an interface and document that multi-instance will require a Redis (or equivalent) backend for that interface.

---

## 7. References

- Task 080: streaming in CLI (`.vibe/080.md`, `.vibe/080-design.md`).
- `internal/agent.StreamSink` and `internal/llm.ChatWithToolsStream`.
- `internal/executor.runBuildmaxCmd` (currently `CombinedOutput()`).
- Portal: `portal/src/pages/ChatDetail.tsx` (polling + `getChatConversation`).
- Server: `internal/server/chats.go` (`getChatConversationHandler`, `loadChatConversationData`).
- **Section 0 refactor:** `internal/storage/entity/chat.go` (CreateChat, session_id); `internal/server/stream_hub.go` (key by chat_id); `internal/server/worker_handlers.go` (POST stream, PATCH run → resolve to chat_id); `internal/server/stream_handlers.go` (user GET stream by chat_id); `internal/executor/worker_api.go`, `internal/workercmd/run.go` (worker receives and uses session_id).
