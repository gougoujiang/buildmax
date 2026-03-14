# Design 094 — Webhook adapter (Phase 4)

## Goal

Add a Tier 1 webhook channel adapter and rule-based conversation path so that incoming webhook HTTP requests (no LLM) map to a single Tier 2 ChatRun, with optional callback on run complete.

## Modules


| Module (package)              | Responsibility                                                                                                                                              | Owns                                                                                       |
| ----------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| **internal/conversation**     | Tier 1 contract: turn/result types (existing), channel constants (existing), ChannelAdapter and ConversationEngine interfaces (new), webhook adapter (new). | types.go, interfaces.go (new), webhook_adapter.go (new).                                   |
| **internal/app/conversation** | Tier 1 app orchestration: existing Service (HandleTurn); new RuleBasedEngine implementing ConversationEngine for webhook turns.                             | service.go (existing), rulebased_engine.go (new).                                          |
| **internal/server**           | HTTP server and route registration. New webhook handler package; portal endpoints for webhook key CRUD.                                                     | server.go (modify: register webhook), webhook/ handler (new), portal webhook-keys handlers (new). |
| **portal** (frontend)         | Workspace settings UI: webhook API keys section — create (show key once), list, revoke.                                                                     | Workspace settings page or route; webhook keys component/section.                               |
| **internal/config**           | Env var names and docs.                                                                                                                                     | env_spec.go (add BUILDMAX_WEBHOOK_MESSAGE_PATH etc.), .env.example.                         |
| **internal/storage/entity**   | WorkspaceWebhookKey model and WorkspaceWebhookKeyStore; optional GetWorkspace for validation.                                                               | models, workspace_webhook_key.go, interfaces.go, store impl.                                |


## Structure

**Directory / files**

- `internal/conversation/` — Tier 1 contract and webhook adapter
  - `types.go` — existing (ConversationTurn, ConversationResult, channel constants)
  - `interfaces.go` — **new**: ChannelAdapter, ConversationEngine
  - `webhook_adapter.go` — **new**: WebhookAdapter type and config, Receive/Send
- `internal/app/conversation/` — Tier 1 app service and rule-based engine
  - `service.go` — existing (unchanged for this task)
  - `rulebased_engine.go` — **new**: RuleBasedEngine, Process for webhook only
- `internal/server/` — HTTP server
  - `server.go` — add webhook route registration and optional WebhookConfig
  - `webhook/` — **new** package (or inline in server): handler, config, auth
  - `webhook/handler.go` — POST handler, adapter Receive → engine Process → 202
- `internal/config/` — env
  - `env_spec.go` — add EnvKeyBuildmaxWebhookMessagePath (optional)
- `portal/` — frontend
  - Workspace settings page (or section) — **new** or extend existing: "Webhook API keys" with create (show key once), list (metadata only), revoke
- `design/` or `docs/` — **new** short doc: webhook configuration (per-workspace keys, payload shape, callback)

**Main types and interfaces**

- **ChannelAdapter** (conversation): Receive(ctx, raw) (ConversationTurn, error); Send(ctx, conversationID, output) error.
- **ConversationEngine** (conversation): Process(ctx, workspaceID, chatID string, turn ConversationTurn) (ConversationResult, error).
- **WebhookAdapter** (conversation): struct with MessagePath string (JSON path for message, e.g. "message" or "body.text"); implements ChannelAdapter; Receive accepts *WebhookRequest or *http.Request; Send POSTs to callback URL from turn.Raw if present.
- **WebhookRequest** (conversation or server/webhook): struct { Body []byte, Header http.Header, WorkspaceID string (from path or header), optional parsed body for path lookup }. Used so adapter can be tested without http.Request.
- **RuleBasedEngine** (app/conversation): struct { Chat *chat.Service }; implements ConversationEngine; Process only accepts ChannelWebhook, else error; creates one run via StartBackgroundChat or CreateRun.

## Method design


| Receiver                           | Method              | Signature                                                                                                                                                                                                                                                                                                                                                     | Responsibility                                                                                                                                                                                                                                                                                     |
| ---------------------------------- | ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **ChannelAdapter** (interface)     | Receive             | `(ctx context.Context, raw any) (ConversationTurn, error)`                                                                                                                                                                                                                                                                                                    | Normalize channel input to ConversationTurn.                                                                                                                                                                                                                                                       |
| **ChannelAdapter** (interface)     | Send                | `(ctx context.Context, conversationID string, output string) error`                                                                                                                                                                                                                                                                                           | Deliver output (e.g. POST to callback URL).                                                                                                                                                                                                                                                        |
| **ConversationEngine** (interface) | Process             | `(ctx context.Context, workspaceID string, chatID string, turn ConversationTurn) (ConversationResult, error)`                                                                                                                                                                                                                                                 | Process one turn; may create run(s), return reply and/or TaskIDs.                                                                                                                                                                                                                                  |
| **WebhookAdapter**                 | Receive             | `(ctx context.Context, raw any) (ConversationTurn, error)`                                                                                                                                                                                                                                                                                                    | Type-assert raw to *WebhookRequest; read workspace from request; extract message via MessagePath (e.g. gjson or simple path); build ConversationTurn (Channel=ChannelWebhook); put callback_url in turn.Raw if present in body.                                                                    |
| **WebhookAdapter**                 | Send                | `(ctx context.Context, conversationID string, output string) error`                                                                                                                                                                                                                                                                                           | If conversationID is a callback URL (or lookup from context/stored state), POST output JSON to it; else no-op.                                                                                                                                                                                     |
| **RuleBasedEngine**                | Process             | `(ctx context.Context, workspaceID string, chatID string, turn ConversationTurn) (ConversationResult, error)`                                                                                                                                                                                                                                                 | If turn.Channel != ChannelWebhook return error; if chatID == "" call ChatService.StartBackgroundChat(workspaceID, turn.UserID, turn.Message, nil, nil) and return result with TaskIDs=[runID]; else call ChatService.CreateRun(ctx, chatID, turn.Message, turn.UserID) and return TaskIDs=[runID]. |
| **WebhookHandler** (server)        | ServeHTTP or Handle | Parse body; validate API key (WorkspaceWebhookKeyStore.GetWorkspaceIDByKey), require path workspace_id match; build WebhookRequest; call adapter.Receive; validate workspace exists; call RuleBasedEngine.Process; write 202 with {"task_id"}; optionally store callback_url for run-complete. |


**Webhook route**

- Option A (workspace in path): `POST /api/workspaces/{workspace_id}/webhook` — workspace from path, body JSON (message path, optional callback_url).
- Option B (workspace in header): `POST /api/webhook` — header `X-Workspace-ID` (or similar), body same.
- Design choice: support both or pick one. Recommend **workspace in path** for clarity: `POST /api/workspaces/{workspace_id}/webhook`. Body: `{"message": "..."}` or configurable path; optional `"callback_url": "https://..."`.

**Auth: per-workspace API keys (recommended)**

- Use **per-workspace webhook API keys** instead of a single static env secret. Each workspace can have one or more API keys; the caller sends a key (e.g. `Authorization: Bearer <key>` or header `X-Webhook-Key`); the server resolves the key to a workspace and requires the path `workspace_id` to match. Benefits: per-user/workspace isolation, revocable keys, no shared secret.
- **Entity**: `WorkspaceWebhookKey` — workspace_id, key_hash (SHA256 of the secret key), name (optional label), created_at. The plaintext key is generated at creation (e.g. `whsec_` + 32 bytes hex) and returned once; only the hash is stored.
- **Store**: `WorkspaceWebhookKeyStore` (or extend an existing store): `CreateKey(ctx, workspaceID, name) (plaintextKey string, err error)`, `GetWorkspaceIDByKey(ctx, key string) (workspaceID string, err error)` (hash key, lookup by key_hash), `ListKeys(ctx, workspaceID) ([]KeyMeta, error)`, `RevokeKey(ctx, workspaceID, keyID) error`. KeyMeta: id, name, created_at (no plaintext).
- **Handler**: Require `Authorization: Bearer <key>` or `X-Webhook-Key: <key>`. Lookup workspace via GetWorkspaceIDByKey(key); if not found → 401. Validate path `workspace_id` == resolved workspace_id; else 403. Proceed with that workspace.
- **Key creation**: Portal API (authenticated): `POST /api/workspaces/{id}/webhook-keys` (body: optional name) → 201 + `{"key": "whsec_...", "id": "..."}` (key shown once); `GET /api/workspaces/{id}/webhook-keys` → list key metadata (no plaintext); `DELETE /api/workspaces/{id}/webhook-keys/{key_id}` → revoke. Only workspace owner can manage keys.
- **Portal UI — workspace settings (webhook keys):** Users need a place to create and manage keys. Provide a **workspace settings page** (or a "Webhook" / "API keys" section inside existing workspace settings) with: (1) **Create key** — button that calls POST webhook-keys, then shows the new key **once** (copy button + warning "This key won't be shown again"); (2) **List keys** — table or cards with name/label, created date (optional: last used); no plaintext; (3) **Revoke** — delete control per row calling DELETE webhook-keys/{id}. Same access control as other workspace management (workspace owner). If the portal has no workspace settings page yet, add one with at least this webhook keys section.
- **Fallback**: Optionally keep `BUILDMAX_WEBHOOK_SECRET` as a single shared secret for backward compatibility or simple setups; if set, accept either (a) Bearer matching that secret and treat as “any workspace” (then path workspace_id is required and we only validate workspace exists) or (b) prefer API key when present. Recommendation: **no fallback** in 094; require per-workspace keys. If needed later, add env fallback in a small follow-up.

**UserID for webhook-created runs**

- ChatService.CreateChat/CreateRun require CreatedBy (userID). Use a fixed system user ID for webhook (e.g. constant `"webhook"` or env `BUILDMAX_WEBHOOK_USER_ID`). RuleBasedEngine passes turn.UserID; WebhookAdapter sets turn.UserID to that system user (from config or constant).

## How they work together

**Data/control flow**

1. Incoming `POST /api/workspaces/{id}/webhook` with JSON body and `Authorization: Bearer <api_key>`. Handler parses body, validates API key via WorkspaceWebhookKeyStore.GetWorkspaceIDByKey(key); if key invalid → 401; if resolved workspace_id != path id → 403. Build WebhookRequest{ WorkspaceID: id, Body, Header }.
2. Handler calls WebhookAdapter.Receive(ctx, &WebhookRequest) → ConversationTurn (WorkspaceID, Channel=webhook, Message from body path, UserID=system user, Raw with callback_url if present).
3. Handler optionally validates workspace exists (e.g. WorkspaceStore.GetWorkspace(ctx, id)); if not, return 404.
4. Handler calls RuleBasedEngine.Process(ctx, workspaceID, "", turn). Engine calls ChatService.StartBackgroundChat(workspaceID, turn.UserID, turn.Message, nil, nil) → chatID, runID; returns ConversationResult{ TaskIDs: [runID] }.
5. Handler responds 202 with `{"task_id": "<runID>"}`. Optionally: persist callback_url (e.g. in run metadata or a small store) for run-complete callback.
6. When run completes (worker PATCH): if callback_url was stored for that run, call WebhookAdapter.Send(ctx, callback_url, jsonSummary) or a small HTTP POST helper to send `{"chat_run_id","status","reply_summary"}`.

**Dependencies**

- internal/conversation: no dependency on server or app; only stdlib and maybe a small JSON path helper.
- internal/app/conversation: depends on internal/conversation (types, interfaces), internal/app/chat (Service).
- internal/server/webhook: depends on internal/conversation (adapter, types), internal/app/conversation (RuleBasedEngine), entity (WorkspaceStore, WorkspaceWebhookKeyStore for auth and validation), config (env).

**Key data structures**

- **WebhookRequest**: Body, Header, WorkspaceID; built by handler from *http.Request; passed to adapter.Receive.
- **ConversationTurn.Raw["callback_url"]**: Optional; set by adapter from body; used later by Send or by run-complete logic.
- **Callback payload**: Documented shape e.g. `{"chat_run_id": "...", "status": "SUCCEEDED"|"FAILED", "reply_summary": "..."}`.

## Run-complete callback (minimal)

- **Option 1**: Store callback_url per run (e.g. in a new table or in run metadata). When worker PATCHes run to SUCCEEDED/FAILED, server (or worker) calls a small helper that POSTs JSON to the stored URL. Requires: where to store (run has no callback column today — could add or use a side table).
- **Option 2**: Document only; implement POST in a follow-up. For this task: document payload shape; implement logging when a webhook-origin run completes (e.g. log run_id and status).
- **Recommendation**: Document payload; implement a small `internal/server/webhook/callback.go` (or internal/conversation) that POSTs to a given URL with JSON. Call it from the handler that processes worker PATCH run complete — only if we have a way to associate run with callback_url. Simplest: add optional `callback_url` to run context only in memory for the request (we don’t persist it); then we can’t callback on complete unless we persist it. So: either add persistence for callback_url (e.g. column or table) or leave callback as “document only” and do log on complete. Design: **persist callback_url** — e.g. ChatRun has no such column; add a small store (e.g. run_id → callback_url map in DB or in-memory with TTL). Out of scope for minimal: use RunOutputLister / existing run completion flow to trigger callback. For **minimal** implementation: document callback payload; implement **log** when run created from webhook completes (slog); defer POST callback to a follow-up unless we add a simple persistence (e.g. in-memory map runID→callbackURL cleared after use). So: **Changes**: Document callback payload; add optional in-memory store (runID → callbackURL) set in webhook handler, read in run-complete handler when worker PATCHes; on complete POST to URL and remove from store. If in-memory is unacceptable (multi-instance server), document only and log.

## Config / env

- **Webhook auth**: Per-workspace API keys (no global env secret required). Keys are created via portal API and stored hashed; see Auth section above.
- **BUILDMAX_WEBHOOK_MESSAGE_PATH** (optional): JSON path for message in body (default `"message"`). Add EnvKeyBuildmaxWebhookMessagePath; default "message".
- **BUILDMAX_WEBHOOK_USER_ID** (optional): User ID used as CreatedBy for webhook-created chats (default constant e.g. `"webhook"`). Add if we want it configurable.

## Tests

- **conversation/webhook_adapter_test.go**: Receive with body `{"message":"hello"}` and workspace in request → ConversationTurn with Message="hello", Channel=ChannelWebhook; Receive with missing message path → error; Send with URL in conversationID or Raw → POST (mock HTTP client).
- **app/conversation/rulebased_engine_test.go**: Process with webhook turn and chatID empty → StartBackgroundChat called, TaskIDs length 1; Process with webhook turn and chatID set → CreateRun called, TaskIDs length 1; Process with channel=portal → error.
- **server/webhook**: Optional integration test: POST to /api/workspaces/{id}/webhook with mock stores → 202 and one PENDING run in ChatRunStore.

## Changes for review

- **New**: `internal/conversation/interfaces.go` — ChannelAdapter (Receive, Send), ConversationEngine (Process).
- **New**: `internal/conversation/webhook_adapter.go` — WebhookAdapter struct (MessagePath, optional UserID), WebhookRequest struct; Receive(raw) builds ConversationTurn from WebhookRequest; Send POSTs to URL if present.
- **New**: `internal/app/conversation/rulebased_engine.go` — RuleBasedEngine struct { Chat *chat.Service }; Process(workspaceID, chatID, turn) webhook-only, one run created.
- **New**: `internal/storage/entity` — WorkspaceWebhookKey model (workspace_id, key_hash, name, created_at; table `workspace_webhook_key`); WorkspaceWebhookKeyStore interface (CreateKey, GetWorkspaceIDByKey, ListKeys, RevokeKey); implementation in Store.
- **New**: `internal/server/webhook/` package — Handler with Config (Adapter, Engine, WorkspaceStore, WorkspaceWebhookKeyStore, MessagePath); Register(mux); POST /api/workspaces/{id}/webhook (auth via API key, resolve workspace, then Process).
- **New**: Portal endpoints for key management: POST/GET/DELETE /api/workspaces/{id}/webhook-keys (create returns plaintext key once; list returns key metadata only; delete revokes). Require workspace auth (user is owner).
- **New**: Portal UI — workspace settings page (or section): Webhook API keys — create key (show once with copy + warning), list keys (name, created_at), revoke key. Same access as workspace owner.
- **Modified**: `internal/server/server.go` — Build webhook handler config (adapter, engine, stores including WorkspaceWebhookKeyStore); register webhook route; wire WorkspaceWebhookKeyStore from entity.Store.
- **Modified**: `internal/config/env_spec.go` — Add EnvKeyBuildmaxWebhookMessagePath (and optional BUILDMAX_WEBHOOK_USER_ID); add to EnvVars. No BUILDMAX_WEBHOOK_SECRET (auth is per-workspace API keys).
- **Modified**: `.env.example` — Add BUILDMAX_WEBHOOK_MESSAGE_PATH (and optional BUILDMAX_WEBHOOK_USER_ID).
- **New**: `design/004-webhook.md` or section in README — Webhook configuration (per-workspace API keys, payload shape, callback payload).
- **Optional**: WorkspaceStore.GetWorkspace(ctx, workspaceID) (*Workspace, error) for handler to validate workspace exists.
- **Optional**: Run-complete callback: persist callback_url; invoke POST from run-complete flow; document payload.

