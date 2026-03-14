# Design 092 — Portal as ChannelAdapter and pass-through engine

## Goal

Wire the portal create-run flow through Tier 1: add a portal adapter and a pass-through conversation engine in the server, make the handler use them when `Config.ConversationEngine` is set, and set the engine in the server entrypoint so the default run uses Tier 1. API contract (201, body, 409, 400) unchanged.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/server** | HTTP API; add portal adapter, pass-through engine, optional ConversationEngine in Config; createChatRunHandler branches on engine. | PortalTurnInput, PortalAdapter, PassThroughEngine; Config.ConversationEngine; updated createChatRunHandler. |
| **internal/conversation** | Tier 1 contract (existing). | ConversationTurn, ConversationResult, ChannelAdapter, ConversationEngine. No changes. |
| **internal/cmd/server** | Server bootstrap. | Build PassThroughEngine with ChatRunStore, set cfg.ConversationEngine. |

## Structure

**Directory / files**

- `internal/server/` — HTTP server and Tier 1 portal/pass-through implementations
  - `conversation_tier1.go` (new) — PortalTurnInput, PortalAdapter, PassThroughEngine. PortalAdapter implements conversation.ChannelAdapter; PassThroughEngine implements conversation.ConversationEngine.
  - `chats.go` — existing; add import of conversation; createChatRunHandler updated to use engine when cfg.ConversationEngine != nil.
  - `server.go` — add ConversationEngine to Config (type conversation.ConversationEngine).
  - `conversation_tier1_test.go` (new) — unit tests for PortalAdapter.Receive and PassThroughEngine.Process.
- `internal/cmd/server/run.go` — after building cfg, set cfg.ConversationEngine = NewPassThroughEngine(cfg.ChatRunStore) (or equivalent constructor).

**Main types**

- **PortalTurnInput** (server): Request-shaped input for the portal adapter. Fields: WorkspaceID, ChatID, UserID, Message (string). Passed to PortalAdapter.Receive as `raw`.
- **PortalAdapter** (server): Implements conversation.ChannelAdapter. No fields or a zero-size struct. Receive(ctx, raw): type-assert raw to *PortalTurnInput (or struct with same shape); build and return conversation.ConversationTurn (WorkspaceID, Channel=conversation.ChannelPortal, ConversationID=ChatID, UserID, Message, Raw=nil). Send(ctx, conversationID, output): no-op for Phase 2 (return nil).
- **PassThroughEngine** (server): Implements conversation.ConversationEngine. Fields: chatRuns entity.ChatRunStore. Process(ctx, workspaceID, chatID, turn): call chatRuns.CreateChatRun(ctx, chatID, turn.Message, turn.UserID); on err return (zero result, err); on success return conversation.ConversationResult{ TaskIDs: []string{ run.ChatRunID } }, nil.
- **Config** (server): New field ConversationEngine conversation.ConversationEngine (optional, may be nil).

## Method design

| Receiver / Type | Method | Signature | Responsibility |
|-----------------|--------|-----------|----------------|
| **PortalAdapter** | Receive | `(ctx context.Context, raw any) (conversation.ConversationTurn, error)` | Type-assert raw to *PortalTurnInput; return ConversationTurn with ChannelPortal, ConversationID=ChatID, UserID, Message. Return error if raw is nil or assertion fails. |
| **PortalAdapter** | Send | `(ctx context.Context, conversationID string, output string) error` | No-op; return nil. |
| **PassThroughEngine** | Process | `(ctx context.Context, workspaceID, chatID string, turn conversation.ConversationTurn) (conversation.ConversationResult, error)` | Call ChatRunStore.CreateChatRun(ctx, chatID, turn.Message, turn.UserID). On error return (zero, err). On success return Result{ TaskIDs: []string{ run.ChatRunID } }, nil. |
| **server** | (constructor) | `NewPassThroughEngine(chatRuns entity.ChatRunStore) *PassThroughEngine` | Return engine with store field set. |

**createChatRunHandler (updated logic)**

- Existing: auth, require ChatRunStore, path chat_id, getChatForWorkspace, decode body, validate input, quota. Keep all of that.
- **If** `s.cfg.ConversationEngine != nil`: build PortalTurnInput{ WorkspaceID: workspaceID, ChatID: chatID, UserID: userID, Message: req.Input }; call `s.cfg.PortalAdapter.Receive(r.Context(), &input)` to get turn (or build ConversationTurn directly in handler to avoid adapter in Config — task allows either). Call `s.cfg.ConversationEngine.Process(r.Context(), workspaceID, chatID, turn)`. On error: if errors.Is(err, entity.ErrRunInProgress) write 409; else write 500. On success: if len(result.TaskIDs) == 0 write 500 (unexpected); else write 201 with map["chat_run_id"]=result.TaskIDs[0], map["chat_id"]=chatID.
- **Else**: current behavior — s.cfg.ChatRunStore.CreateChatRun(...), then 201 with run.ChatRunID and chatID.

**Config**

- Add field: `ConversationEngine conversation.ConversationEngine` (optional).
- Add field: `PortalAdapter conversation.ChannelAdapter` (optional; used when ConversationEngine is set so handler can call Receive). Alternatively the handler can build ConversationTurn by hand (WorkspaceID, ChannelPortal, ConversationID=chatID, UserID, Message) and not store an adapter in Config; task says "adapter is used so that the handler (or a small helper) builds the input struct from the HTTP request and calls Receive". So we need a single portal adapter instance. Config can hold it, or the handler can instantiate PortalAdapter{} and call Receive with a struct. Design choice: hold optional PortalAdapter in Config; when ConversationEngine is set, set PortalAdapter to &server.PortalAdapter{} so handler uses it. Or for minimal Config surface, handler builds ConversationTurn directly and we don't add PortalAdapter to Config (still implement PortalAdapter type for tests and reuse). Task says "builds the turn (via portal adapter Receive or by constructing ConversationTurn directly)". I'll put both in design: Config.ConversationEngine and Config.PortalAdapter (optional). When engine is set, handler uses s.cfg.PortalAdapter.Receive if set, else builds turn inline. That way we can test the adapter and still keep handler simple. Actually task says "Adapter is used so that the handler ... builds the input struct ... and calls Receive". So we do use the adapter. Config: ConversationEngine + PortalAdapter (optional). When wiring in run.go we set both: PortalAdapter = &PortalAdapter{}, ConversationEngine = NewPassThroughEngine(st).
- So: Config gets ConversationEngine and PortalAdapter (both optional). createChatRunHandler: if engine != nil, build PortalTurnInput, call s.cfg.PortalAdapter.Receive(ctx, &input) — if PortalAdapter is nil we could build turn inline for backward compat, or require that when engine is set, adapter is set too. Simplest: when engine != nil, require adapter != nil and use it to get the turn.

## How they work together

**Data/control flow (create-run with Tier 1)**

1. Client POSTs to .../chats/{chat_id}/runs with body { "input": "..." }. Handler: auth, path, getChatForWorkspace, decode, validate, quota.
2. Handler builds PortalTurnInput{ workspaceID, chatID, userID, req.Input } and calls s.cfg.PortalAdapter.Receive(ctx, &input) → ConversationTurn.
3. Handler calls s.cfg.ConversationEngine.Process(ctx, workspaceID, chatID, turn) → ConversationResult or error.
4. Engine calls ChatRunStore.CreateChatRun(ctx, chatID, turn.Message, turn.UserID) → run or error (e.g. ErrRunInProgress).
5. On success, handler writes 201 with { "chat_run_id": result.TaskIDs[0], "chat_id": chatID }. On ErrRunInProgress, 409. On other error, 500.

**Dependencies**

- internal/server imports internal/conversation (for ConversationTurn, ConversationResult, ChannelAdapter, ConversationEngine, ChannelPortal) and internal/storage/entity (for ChatRunStore, ErrRunInProgress).
- internal/cmd/server imports internal/server and internal/storage/entity; builds PassThroughEngine(st) and optionally PortalAdapter; sets cfg.ConversationEngine and cfg.PortalAdapter.

**Key data structures**

- PortalTurnInput: built by handler from request; passed to PortalAdapter.Receive.
- ConversationTurn: produced by adapter; passed to engine.Process.
- ConversationResult: produced by engine; handler reads TaskIDs[0] for 201 body.

## Tests

- **conversation_tier1_test.go**
  - TestPortalAdapter_Receive: build PortalTurnInput; call Receive(ctx, &input); assert turn.Channel == conversation.ChannelPortal, turn.ConversationID == input.ChatID, turn.UserID == input.UserID, turn.Message == input.Message. Test Receive with nil raw returns error.
  - TestPassThroughEngine_Process: use mock ChatRunStore that records CreateChatRun args and returns a fake run with ChatRunID "r_abc"; call Process with a turn; assert one CreateChatRun with chatID and turn.Message and turn.UserID; assert result.TaskIDs has one element "r_abc". Test Process when store returns ErrRunInProgress: assert error and result is zero.
- **Handler test**: extend or add test that creates a run via the handler with ConversationEngine and PortalAdapter set (mock store); assert 201 and response body chat_run_id and chat_id; assert store received CreateChatRun with correct input. Can use existing helpers_test.go mocks.

## Changes for review

- **New**: `internal/server/conversation_tier1.go` — types PortalTurnInput, PortalAdapter (Receive, Send), PassThroughEngine (Process), constructor NewPassThroughEngine(chatRuns entity.ChatRunStore) *PassThroughEngine.
- **New**: `internal/server/conversation_tier1_test.go` — unit tests for PortalAdapter.Receive and PassThroughEngine.Process.
- **Modified**: `internal/server/server.go` — Config gains ConversationEngine conversation.ConversationEngine and PortalAdapter conversation.ChannelAdapter (optional).
- **Modified**: `internal/server/chats.go` — createChatRunHandler: when cfg.ConversationEngine != nil, build PortalTurnInput, call cfg.PortalAdapter.Receive, call engine.Process, return 201 from result.TaskIDs[0] or 409/500 on error; when nil, keep current CreateChatRun path. Add import of conversation and use entity.ErrRunInProgress for 409.
- **Modified**: `internal/cmd/server/run.go` — after building cfg, set cfg.ConversationEngine = server.NewPassThroughEngine(st), cfg.PortalAdapter = &server.PortalAdapter{} (or a single helper that returns both) so default server uses Tier 1.
