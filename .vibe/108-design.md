# Design 108 - Portal WebSocket Communication

## Goal

Replace SSE-based Portal↔server communication with a persistent WebSocket connection carrying typed JSON events, so the server can push messages proactively and the portal can send user input without HTTP round-trips.

## Modules


| Module (package)                                   | Responsibility                                                               | Owns                                                                       |
| -------------------------------------------------- | ---------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| `**internal/wsconn**` (new)                        | Event protocol: typed envelope, all payloads, marshal/unmarshal helpers      | `protocol.go`, `protocol_test.go`                                          |
| `**internal/server/portal**` (modified)            | WebSocket upgrade handler, per-connection read/write loops, dispatch, wsSink | `ws_handler.go`, `ws_handler_test.go`; modified `register.go`, `config.go` |
| `**portal/src/lib/api**` (modified)                | WebSocket client class with reconnect and typed event dispatch               | `ws.ts` (new); `sse.ts` kept but unused by conversations                   |
| `**portal/src/contexts**` (modified)               | WebSocket React context providing the client instance to the tree            | `WebSocketContext.tsx` (new); `App.tsx` updated                            |
| `**portal/src/features/conversations**` (modified) | Conversation hooks/api switch from fetch+SSE to WebSocket events             | `api.ts`, `hooks/useConversationDetail.ts`                                 |
| `**portal/src/features/tasks**` (modified)         | Task streaming switches from fetch+SSE to WebSocket events                   | `api.ts`, `hooks/useTaskDetail.ts`                                         |


## Structure

**Directory / files**

- `internal/wsconn/` — event protocol types (shared by handler and tests)
  - `protocol.go` — envelope struct, event type constants, typed payload structs, `Encode`/`Decode` helpers
  - `protocol_test.go` — round-trip serialization tests
- `internal/server/portal/` — handler additions
  - `ws_handler.go` — `wsHandler` (HTTP upgrade + per-connection management), `wsConn` (connection state), `wsSink` (StreamSink adapter)
  - `ws_handler_test.go` — integration tests with httptest + gorilla/websocket client
  - `register.go` — add `GET /api/ws` route
  - `config.go` — add `ConversationService *convapp.Service` field (pre-built, avoids re-creating per event)
- `portal/src/lib/api/`
  - `ws.ts` — `BuildMaxWebSocket` class (connect, send, on/off, reconnect, close)
- `portal/src/contexts/`
  - `WebSocketContext.tsx` — `WebSocketProvider`, `useWebSocket()` hook
- `portal/src/features/conversations/`
  - `api.ts` — add `createConversationWS`, `addMessageWS` helpers (thin wrappers around ws.send)
  - `hooks/useConversationDetail.ts` — use WebSocket instead of fetch+SSE
- `portal/src/features/tasks/`
  - `api.ts` — add `subscribeTaskWS`, `unsubscribeTaskWS` helpers
  - `hooks/useTaskDetail.ts` — use WebSocket instead of fetch+SSE

**Main types and interfaces**

- `**wsconn.Envelope`**: `{ Type string; Payload json.RawMessage }` — the wire format for every WebSocket message.
- `**wsconn.ClientEvent*` structs**: Typed payloads for each client→server event (ConversationCreate, ConversationMessage, SubscribeTask, UnsubscribeTask).
- `**wsconn.ServerEvent*` structs**: Typed payloads for each server→client event (ConversationCreated, MessageDelta, MessageCompleted, ConversationError, TaskStatusChanged, TaskStreamDelta, TaskStreamDone, SystemError).
- `**portal.wsConn`**: Per-connection state: gorilla `*websocket.Conn`, userID, write channel, active task subscriptions, active conversation turn mutex.
- `**portal.wsSink**`: Implements `llm.StreamSink`; captures conversation_id and sends `MessageDelta` events to the connection's write channel.
- `**BuildMaxWebSocket**` (TS): Client class; manages native `WebSocket`, event listeners map, reconnect timer, pending subscriptions for reconnect.

## Method design

### `internal/wsconn` — Protocol


| Function           | Signature                                         | Responsibility                                               |
| ------------------ | ------------------------------------------------- | ------------------------------------------------------------ |
| `Encode`           | `(eventType string, payload any) ([]byte, error)` | Marshal an envelope to JSON bytes for writing to WebSocket.  |
| `Decode`           | `(data []byte) (Envelope, error)`                 | Unmarshal raw bytes into an Envelope (type + raw payload).   |
| `DecodePayload[T]` | `(env Envelope) (T, error)`                       | Generic helper: unmarshal `env.Payload` into typed struct T. |


### `internal/server/portal` — WebSocket handler


| Receiver  | Method                      | Signature                                                   | Responsibility                                                                                                                                       |
| --------- | --------------------------- | ----------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Handler` | `wsUpgradeHandler`          | `(w http.ResponseWriter, r *http.Request)`                  | Validate JWT from `?token=` query param, upgrade to WebSocket, create `wsConn`, start read/write loops.                                              |
| `wsConn`  | `readLoop`                  | `(ctx context.Context)`                                     | Read messages from WebSocket, decode envelope, dispatch to `handleClientEvent`. Closes on error or context cancel.                                   |
| `wsConn`  | `writeLoop`                 | `(ctx context.Context)`                                     | Read from write channel, encode, write to WebSocket. Handles ping/pong keepalive.                                                                    |
| `wsConn`  | `handleClientEvent`         | `(ctx context.Context, env wsconn.Envelope)`                | Switch on `env.Type`; call `handleConversationCreate`, `handleConversationMessage`, `handleSubscribeTask`, `handleUnsubscribeTask`.                  |
| `wsConn`  | `handleConversationCreate`  | `(ctx context.Context, payload wsconn.ConversationCreate)`  | Create conversation via store, send `conversation.created`, then run turn in goroutine with wsSink, send `conversation.message.completed` when done. |
| `wsConn`  | `handleConversationMessage` | `(ctx context.Context, payload wsconn.ConversationMessage)` | Validate ownership, run turn in goroutine with wsSink, send `conversation.message.completed` when done.                                              |
| `wsConn`  | `handleSubscribeTask`       | `(ctx context.Context, payload wsconn.SubscribeTask)`       | Validate task ownership, subscribe to `StreamHub`, start forwarding goroutine that sends `task.stream.delta` / `task.stream.done`.                   |
| `wsConn`  | `handleUnsubscribeTask`     | `(payload wsconn.UnsubscribeTask)`                          | Call unsub function for that task, remove from active map.                                                                                           |
| `wsConn`  | `sendEvent`                 | `(eventType string, payload any)`                           | Encode and push to write channel (non-blocking; drop if full to avoid blocking agent goroutine).                                                     |
| `wsConn`  | `cleanup`                   | `()`                                                        | Unsubscribe all active task streams, cancel any running conversation turns, close write channel.                                                     |
| `wsSink`  | `OnDelta`                   | `(delta string)`                                            | Call `wsConn.sendEvent("conversation.message.delta", MessageDelta{ConversationID: id, Delta: delta})`.                                               |


### Portal — `BuildMaxWebSocket` (TypeScript)


| Method            | Signature                                     | Responsibility                                                                                                        |
| ----------------- | --------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `connect`         | `(token: string): void`                       | Open `ws(s)://<host>/api/ws?token=<token>`. Set up `onopen`, `onmessage`, `onclose`, `onerror`.                       |
| `send`            | `(type: string, payload: unknown): void`      | JSON.stringify envelope and call `ws.send()`. Queue if not yet open.                                                  |
| `on`              | `(type: string, cb: (payload) => void): void` | Register event handler for a server event type.                                                                       |
| `off`             | `(type: string, cb: (payload) => void): void` | Unregister event handler.                                                                                             |
| `close`           | `(): void`                                    | Set intentional-close flag, close WebSocket, clear reconnect timer.                                                   |
| `_reconnect`      | `(): void`                                    | Exponential backoff (1s→2s→4s→…→30s cap). On open: re-send all active subscriptions from `_pendingSubscriptions` set. |
| `subscribeTask`   | `(taskId: string): void`                      | Send `subscribe.task`, add to `_pendingSubscriptions`.                                                                |
| `unsubscribeTask` | `(taskId: string): void`                      | Send `unsubscribe.task`, remove from `_pendingSubscriptions`.                                                         |


## How they work together

### Flow 1: New conversation with streaming

1. **Portal** (on user submit): `ws.send("conversation.create", { channel: "portal", message: "analyze my sales data" })`.
2. **Server** `wsConn.readLoop` receives the message, decodes envelope, calls `handleConversationCreate`.
3. `handleConversationCreate`:
  a. Creates conversation via `ConversationStore.CreateConversation`.
   b. Sends `conversation.created` event with `conversation_id` to client.
   c. Spawns a goroutine that calls `conversationService.HandleTurn(ctx, HandleTurnCmd{ ..., StreamSink: wsSink })`.
4. **Agent loop** (in goroutine): LLM calls `ChatWithToolsStream` → each delta calls `wsSink.OnDelta(delta)`.
5. `wsSink.OnDelta` → `wsConn.sendEvent("conversation.message.delta", { conversation_id, delta })` → pushed to write channel.
6. `**wsConn.writeLoop`** picks up from channel, encodes JSON, writes to WebSocket.
7. **Portal** `BuildMaxWebSocket.onmessage` fires, dispatches to registered `conversation.message.delta` handler → hook accumulates `streamingContent`.
8. When `HandleTurn` returns: goroutine sends `conversation.message.completed` event.
9. **Portal** handler for `completed` → `refetchMessages()`, clear streaming state.

### Flow 2: Follow-up message

1. **Portal**: `ws.send("conversation.message", { conversation_id, content: "add region breakdown" })`.
2. **Server** `handleConversationMessage`: validates conversation ownership, spawns goroutine with `HandleTurn` + `wsSink`. Same delta/completed flow as above.

### Flow 3: Task stream subscription

1. **Portal** (on task detail mount): `ws.subscribeTask(taskId)`.
2. **Server** `handleSubscribeTask`: validates task ownership, calls `hub.Subscribe(taskId)`, starts forwarding goroutine.
3. **Forwarding goroutine**: reads from hub channel.
  - On delta: sends `task.stream.delta` to write channel.
  - On `StreamEventDone`: sends `task.stream.done`, exits.
4. **Also**: sends buffered content (`hub.Buffer(taskId)`) as initial delta if non-empty.
5. **Portal** on unmount: `ws.unsubscribeTask(taskId)`.
6. **Server** `handleUnsubscribeTask`: calls unsub, forwarding goroutine exits.

### Flow 4: Reconnect

1. WebSocket closes unexpectedly.
2. **Portal** `BuildMaxWebSocket._reconnect`: waits backoff, re-opens connection with stored token.
3. On successful open: iterates `_pendingSubscriptions`, re-sends `subscribe.task` for each active task.
4. For conversations: no re-subscribe needed — the client refetches messages on reconnect (deltas during disconnect are lost but messages are persisted in DB).

### Dependencies

- `internal/wsconn` has **no dependencies** on other internal packages (pure types + JSON).
- `internal/server/portal` depends on `wsconn` (protocol), `streamhub` (task streaming), `app/conversation` (Tier 1 service), `storage/entity` (stores), `llm` (StreamSink), `gorilla/websocket`.
- Portal `ws.ts` depends on nothing (pure browser WebSocket API).
- Portal hooks depend on `WebSocketContext` (React context).

### Key data structures

- `**wsconn.Envelope`**: Wire format `{ type, payload }`. Created by `Encode`, consumed by `Decode`. Both sides produce and consume.
- `**wsConn.writeCh**` (`chan []byte`, buffered 256): Serializes all WebSocket writes. Producer: any goroutine (conversation turn, task forward, event handlers). Consumer: single `writeLoop` goroutine.
- `**wsConn.taskSubs**` (`map[string]func()` — taskID → unsub): Tracks active StreamHub subscriptions for cleanup on disconnect.
- `**wsConn.turnMu**` (`sync.Mutex`): Prevents concurrent conversation turns on the same connection. A second `conversation.message` while one is running gets a `conversation.error` response.
- `**wsSink**`: Stateless adapter: holds `wsConn` pointer and `conversationID`. Multiple sinks can exist (different conversations) but `turnMu` serializes actual execution.
- `**BuildMaxWebSocket._pendingSubscriptions**` (`Set<string>`): Task IDs with active subscriptions; used to re-subscribe on reconnect.

## Concurrency model

### Server side

Each WebSocket connection has exactly **three goroutine roles**:

1. **Reader** (`readLoop`): Only goroutine that reads from `websocket.Conn`. Decodes messages and dispatches. Exits on read error or context cancel → triggers cleanup.
2. **Writer** (`writeLoop`): Only goroutine that writes to `websocket.Conn`. Reads from `writeCh`. Also sends periodic ping frames (every 30s) for keepalive. Exits when `writeCh` is closed.
3. **Worker goroutines** (0–N): One per active conversation turn (locked by `turnMu` so at most 1 at a time), one per active task subscription forwarding.

Write safety: all writes to the WebSocket go through `writeCh` → `writeLoop`. No goroutine writes directly to the conn. `sendEvent` is non-blocking: if `writeCh` is full (slow client), the event is dropped and logged.

### Client side

The browser's `WebSocket` API is inherently single-threaded. `BuildMaxWebSocket` dispatches incoming events synchronously to registered handlers. React state updates (from handlers) are batched by React 19.

## Auth and upgrade

1. Portal opens: `new WebSocket("ws(s)://<host>/api/ws?token=<jwt>")`.
2. Server `wsUpgradeHandler` extracts `token` from query, validates JWT using the same `userIDFromRequest`-style logic (parse JWT, extract `sub` claim). If invalid → HTTP 401 before upgrade.
3. On valid: `gorilla/websocket.Upgrader.Upgrade(w, r, nil)`. The upgrader's `CheckOrigin` is configured to allow the configured CORS origin (or `*` in dev).
4. From this point, `userID` is fixed for the connection lifetime; all events are scoped to this user.

## Keepalive and timeouts

- **Server**: Ping every 30s. Read deadline: 60s (reset on each pong or message). If no pong within deadline → close.
- **Client**: Browser responds to ping with pong automatically. If no message for 45s, send a client-side ping (or just rely on server ping/pong).
- **Reconnect**: Client detects `onclose` → exponential backoff reconnect. Backoff resets on successful connection that stays open for >10s.

## Changes for review

- **New**: `internal/wsconn/protocol.go` — Event envelope, all event type constants, typed payload structs, Encode/Decode helpers.
- **New**: `internal/wsconn/protocol_test.go` — Round-trip serialization tests.
- **New**: `internal/server/portal/ws_handler.go` — `wsUpgradeHandler`, `wsConn` (readLoop, writeLoop, dispatch, cleanup), `wsSink`, all `handle`* methods.
- **New**: `internal/server/portal/ws_handler_test.go` — Integration test: upgrade, send conversation.create, receive created+deltas+completed; subscribe.task flow.
- **Modified**: `internal/server/portal/register.go` — Add `GET /api/ws` route pointing to `wsUpgradeHandler`.
- **Modified**: `internal/server/portal/config.go` — Add `ConversationService *convapp.Service` field to `Config` (pre-built service instance for WebSocket handler).
- **Modified**: `internal/server/server.go` — Build and pass `convapp.Service` instance into portal config; pass CORS origin for WebSocket upgrader.
- **Modified**: `internal/server/middleware.go` — No change needed; CORS middleware already runs before handler (upgrade happens inside handler).
- **New**: `portal/src/lib/api/ws.ts` — `BuildMaxWebSocket` class.
- **New**: `portal/src/contexts/WebSocketContext.tsx` — Provider and `useWebSocket()` hook.
- **Modified**: `portal/src/App.tsx` — Wrap `AppContent` with `WebSocketProvider`.
- **Modified**: `portal/src/features/conversations/api.ts` — Add WS helper functions; deprecate `createConversationStream`, `addConversationMessageStream`.
- **Modified**: `portal/src/features/conversations/hooks/useConversationDetail.ts` — Use WebSocket for messaging instead of fetch+SSE.
- **Modified**: `portal/src/features/tasks/api.ts` — Add WS subscribe/unsubscribe helpers; deprecate `subscribeTaskStream`.
- **Modified**: `portal/src/features/tasks/hooks/useTaskDetail.ts` — Use WebSocket for task streaming instead of fetch+SSE.
- **Unchanged**: `internal/streamhub/hub.go` — StreamHub interface and memStreamHub unchanged; WebSocket handler subscribes the same way the SSE handler does.
- **Unchanged**: `internal/server/portal/stream_handlers.go` — SSE endpoint kept as deprecated fallback.
- **Unchanged**: `internal/server/portal/conversation_handlers.go` — POST+SSE endpoints kept as deprecated fallback.
- **Dependency**: `gorilla/websocket` promoted from indirect to direct in `go.mod`.

