# Server Refactor Opportunities

Scanned `internal/server` (and `internal/storage/entity` where server calls have long arg lists). Focus: **long argument lists**, **long method implementations**, **repetitive code patterns**.

---

## 1. Long argument lists

### 1.1 Entity store methods (called from server)

| Location | Method | Issue |
|----------|--------|--------|
| `entity.ChatStore` | `CreateChat(ctx, workspaceID, input, title, createdBy, titlePromptTokens, titleCompletionTokens, agentID, conversationID)` | **10 arguments**. Call sites: `portal/chats.go` (createWorkspaceChatHandler), `portal/conversation_tools.go` (doStartChat). |
| `entity.ChatStore` | `UpdateChatStatus(ctx, chatID, status, startedAt, endedAt, output, errorMessage, sessionID)` | **8 args**; many optional `*int64`/`*string`. |
| `entity.ChatStore` | `UpdateChatStatusIf(ctx, chatID, expectedStatus, newStatus, startedAt, endedAt, output, errorMessage, sessionID)` | **9 args**. |
| `entity.TaskRunStore` | `UpdateTaskRunStatus(ctx, chatRunID, status, startedAt, endedAt, output, errorMessage, sessionID, promptTokens, completionTokens)` | **10 args**. Called from `worker/handlers.go` patchTaskRun with a long line. |
| `entity.TaskRunStore` | `UpdateTaskRunStatusIf(ctx, chatRunID, expectedStatus, newStatus, startedAt, endedAt, output, errorMessage, sessionID)` | **9 args**. |

**Refactor idea (entity layer):** Introduce small structs for “update” and “create” operations, e.g. `CreateChatInput`, `UpdateTaskRunStatusInput`, so that call sites in server pass a single struct and entity signatures stay stable. Optional: do the same for status-if variants.

**Refactor idea (server-only):** In portal/chats and portal/conversation_tools, build a local struct and pass its fields to `CreateChat` to avoid 10-argument call sites and improve readability.

---

### 1.2 Server wiring: `portal.Config` from `server.Config`

**Location:** `internal/server/server.go` — `New()` builds `portal.Config` with **~15 field assignments**.

**Refactor idea:** Add a function that builds `portal.Config` from `server.Config` and `*Server` (for `s.hub`), e.g. `server.BuildPortalConfig(cfg, s)`. This shortens `New` and keeps a single place for the mapping.

---

### 1.3 Conversation engine call sites (long parameter lists)

**Location:** `internal/server/portal/conversation_handlers.go` — `conversation.RunLoop` and `conversation.RunLoopStream` are called with many arguments (stores, caller, conversationID, message, channel, workspaceID, userID, startTaskRunner, titleGen, and for stream: sink).

**Refactor idea:** If the conversation package allows, introduce an options struct or a “context” struct that holds stores, caller, workspaceID, userID, startTaskRunner, titleGen so that handler code passes fewer arguments. Alternatively, add a small helper in portal that captures these and exposes `Run(ctx, conversationID, message, channel, stream bool, w ...)` to avoid repeating the long list in both createConversationHandler and addConversationMessageHandler.

---

## 2. Long method implementations

### 2.1 `portal.createWorkspaceChatHandler` (~65 lines)

**Flow:** withWorkspaceAuth → requireStore(ChatStore) → decode body → resolve input (optional agent) → truncate title / optional title generation → quota check → CreateChat → write response.

**Refactor ideas:**

- Extract **resolve chat input and agent**: e.g. `resolveCreateChatInput(r, h.cfg, workspaceID, &req) (input string, agentID *string, err error)` returning a clear error for “agent not found” and “input required”.
- Extract **title and usage**: e.g. `resolveTitleAndUsage(ctx, h.cfg.ChatTitleGenerator, input) (title string, promptTokens, completionTokens int)`.
- Handler then: auth → store check → decode → resolveCreateChatInput → resolveTitleAndUsage → quota → CreateChat → write. Each step stays short and testable.

---

### 2.2 `portal.createTaskRunHandler` (~65 lines)

**Flow:** withWorkspaceAuth → requireStore(TaskRunStore) → task_id → getChatForWorkspace → decode → quota → **branch**: conversation path (PortalAdapter + ConversationEngine) vs legacy CreateTaskRun.

**Refactor ideas:**

- Extract **conversation path**: e.g. `(h *Handler) createTaskRunViaConversation(w, r, userID, workspaceID, chatID, input) (handled bool)` that returns true if it wrote a response (so caller can skip legacy path).
- Extract **legacy path**: e.g. `(h *Handler) createTaskRunLegacy(w, r, userID, workspaceID, chatID, input)`.
- createTaskRunHandler becomes: auth → store → chatID → getChat → decode → quota → createTaskRunViaConversation; if !handled then createTaskRunLegacy. This keeps the method short and the two behaviors clearly separated.

---

### 2.3 `portal.artifactContentHandler` (~65 lines)

**Flow:** withWorkspaceAuth → requireStore x3 (RunOutputLister, TaskRunStore, ArtifactStorage) → task_run_id → GetTaskRunWithChat → nil/workspace check → path param + clean → allow result.md or file in output list → GetResult or GetArtifactFile → write body.

**Refactor ideas:**

- Extract **get run+chat and validate workspace**: e.g. `(h *Handler) getArtifactRunAndChat(w, r, workspaceID, chatRunID) (run *entity.TaskRun, chat *entity.Chat, ok bool)` — same pattern is repeated in listArtifactItemsHandler and artifactContentHandler (get run/chat, nil check, workspace check). Reuse in both handlers.
- Extract **path validation**: e.g. `resolveArtifactPath(w, r, h, workspaceID, chatRunID, run, chat) (pathParam string, ok bool)` that handles default "result.md", CleanRelPath, and “file in artifact list” check.
- Handler then: auth → require stores → getArtifactRunAndChat → resolveArtifactPath → read content (result vs file) → write. Shorter and easier to follow.

---

### 2.4 `portal.createConversationHandler` and `portal.addConversationMessageHandler` (~75 and ~70 lines)

**Pattern:** Both branches (stream vs non-stream) set up SSE headers, flusher, sseSink, titleGen from ChatTitleGenerator, then call `RunLoopStream` or `RunLoop`. The block for “stream response” is long and duplicated between the two handlers.

**Refactor ideas:**

- Extract **stream vs non-stream execution** into a shared helper, e.g. `runConversationTurn(ctx, h, convID, message, channel, workspaceID, userID, stream bool, w http.ResponseWriter) (reply string, err error)` that:
  - If stream: sets SSE headers, builds sink and titleGen, calls RunLoopStream, writes "done".
  - If !stream: builds titleGen, calls RunLoop.
  - Returns reply and error so the handler only does: create/get conv → decode → runConversationTurn → write JSON or handle stream write.
- This reduces duplication and keeps each handler focused on “get conversation, get message, run turn, respond”.

---

### 2.5 `worker.patchTaskRun` (~60 lines)

**Flow:** path + body validation → if status RUNNING: UpdateTaskRunStatusIf (with many nil args) → else: UpdateTaskRunStatus (long arg list) + OnRunComplete or SyncChatFromRun + Hub.Done.

**Refactor ideas:**

- Extract **RUNNING** branch: e.g. `(h *Handler) handlePatchRunning(w, r, chatRunID, req) bool` that returns true if it wrote a response (so caller can skip the rest).
- Extract **terminal status** (SUCCEEDED/FAILED etc.): e.g. `(h *Handler) handlePatchTerminalStatus(ctx, chatRunID, req)` that does UpdateTaskRunStatus, OnRunComplete/SyncChatFromRun, and Hub.Done. Then patchTaskRun becomes: validate → handlePatchRunning; if !written then handlePatchTerminalStatus → write 200.
- Optionally, pass a small struct for the PATCH body (status, startedAt, endedAt, output, errorMessage, sessionID, artifact, tokens) so the method signatures inside worker are shorter.

---

## 3. Repetitive code patterns

### 3.1 Portal: workspace auth + require store

**Pattern:** Many handlers start with:

```go
_, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
if !ok {
    return
}
if !h.requireStore(w, h.cfg.XStore, "x not configured") {
    return
}
```

**Refactor idea:** Optional helper, e.g. `withWorkspaceAndStore(w, r, pathKey string, store interface{}, unavailableMsg string) (userID, workspaceID string, ok bool)` that runs both and returns only when both succeed. Not critical since the current pattern is only two blocks; useful if you add more “required store” checks in the same handler.

---

### 3.2 Path parameter required

**Pattern:** Repeated in portal and worker:

```go
id := r.PathValue("x_id")
if id == "" {
    writeJSONError(w, http.StatusBadRequest, "x_id required")
    return
}
```

**Refactor idea:** In portal (and optionally worker), add a small helper: `pathValueRequired(w, r, key string) (value string, ok bool)`. Use it for `workspace_id`, `task_id`, `agent_id`, `task_run_id`, `conversation_id` where “missing = 400”. Reduces duplication and keeps the “required param” rule in one place.

---

### 3.3 Artifact: get run+chat and validate workspace

**Pattern:** In `listArtifactItemsHandler` and `artifactContentHandler` the same block appears:

- Get chatRunID from path.
- `GetTaskRunWithChat(ctx, chatRunID)`.
- if run == nil || chat == nil → 404.
- if chat.WorkspaceID != workspaceID → 404.

**Refactor idea:** Extract `(h *Handler) getArtifactRunAndChat(w, r, workspaceID, chatRunID string) (run *entity.TaskRun, chat *entity.Chat, ok bool)` that performs the store call and these checks and writes the appropriate error. Both artifact handlers then use this and proceed with their specific logic (list items vs content). See also §2.3.

---

### 3.4 Conversation: SSE setup + RunLoopStream

**Pattern:** In createConversationHandler and addConversationMessageHandler, the “stream” branch repeats:

- Set Content-Type, Cache-Control, Connection.
- WriteHeader(201 or 200).
- Get flusher, build sseSink.
- Build titleGen from ChatTitleGenerator.
- Call conversation.RunLoopStream(..., titleGen, sink).
- Write SSE error if any, then write "done", flush.

**Refactor idea:** Extract a helper that takes (w, r, h.cfg, conversationID, message, channel, workspaceID, userID, startTaskRunner) and a callback or struct that knows how to call RunLoopStream (to avoid pulling conversation package into a generic “SSE runner”). Alternatively, a `conversationStreamRunner` struct in portal that holds stores, caller, titleGen and has `Run(ctx, convID, message, ...) (reply, err)` and `RunStream(ctx, convID, message, ..., w)` so the handlers only set headers and call Run/RunStream. This overlaps with §2.4.

---

### 3.5 Shared response helpers (writeJSON, writeJSONError, writeInternalError)

**Pattern:** `server/response.go`, `portal/response.go`, `auth/response.go`, and `worker/response.go` each define almost the same:

- `writeJSON(w, status, data)`
- `writeJSONError(w, status, message)`
- `writeInternalError(w, err, attrs...)` (with package-specific slog message: "handler internal error", "portal handler error", "auth handler error", "worker handler error")

**Refactor idea:** Move the common logic to a shared package, e.g. `internal/server/httputil` or `internal/httputil`:

- `WriteJSON(w, status, data)`, `WriteJSONError(w, status, message)`.
- `WriteInternalError(w, err, logMsg string, attrs ...any)` so each caller passes its log message ("auth handler error", etc.) and the rest is shared. Then auth, portal, worker, and server can call into that package and avoid maintaining four copies. Optional: add `WriteQuotaExceeded(w, reason)` there if used in more than one place.

---

## 4. Summary table

| Category              | Location / pattern                                      | Refactor idea (short) |
|-----------------------|---------------------------------------------------------|------------------------|
| Long arg list         | entity CreateChat / UpdateTaskRunStatus / Update*If     | Structs for create/update params (entity or server-side). |
| Long arg list         | server.New portal.Config build                          | BuildPortalConfig(cfg, s) helper. |
| Long arg list         | conversation.RunLoop / RunLoopStream in portal          | Options/context struct or portal helper to shorten call sites. |
| Long method           | portal createWorkspaceChatHandler                       | Extract resolveCreateChatInput, resolveTitleAndUsage. |
| Long method           | portal createTaskRunHandler                             | Extract createTaskRunViaConversation, createTaskRunLegacy. |
| Long method           | portal artifactContentHandler                           | Extract getArtifactRunAndChat, resolveArtifactPath. |
| Long method           | portal createConversationHandler, addConversationMessageHandler | Shared runConversationTurn(stream bool) helper. |
| Long method           | worker patchTaskRun                                     | Extract handlePatchRunning, handlePatchTerminalStatus. |
| Repetition            | Portal workspace auth + requireStore                    | Optional withWorkspaceAndStore helper. |
| Repetition            | Path param required                                     | pathValueRequired(w, r, key). |
| Repetition            | Artifact get run+chat + workspace check                 | getArtifactRunAndChat. |
| Repetition            | Conversation SSE + RunLoopStream                        | Shared stream runner or runConversationTurn. |
| Repetition            | writeJSON / writeJSONError / writeInternalError in 4 pkgs| Shared httputil (or server/httputil) with parameterized log message. |

---

*Document generated from a scan of `internal/server` and related entity call sites. Implement refactors incrementally and run tests after each change.*

---

## Implementation log

- **2025-03-14**: Implemented shared `internal/server/httputil` (WriteJSON, WriteJSONError, WriteQuotaExceeded, WriteInternalError); portal pathValueRequired, withWorkspaceAndStore, getArtifactRunAndChat, resolveArtifactPath; resolveCreateChatInput, resolveTitleAndUsage; createTaskRunViaConversation, createTaskRunLegacy; runConversationTurn; worker handlePatchRunning, handlePatchTerminalStatus; server buildPortalConfig; entity CreateChatInput and CreateChat(ctx, in *CreateChatInput). UpdateTaskRunStatusInput / UpdateTaskRunStatusIfInput left as optional (same pattern can be applied later).
