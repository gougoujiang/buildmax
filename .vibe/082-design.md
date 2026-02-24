# Design 082: API tidy up

## Goal

Identify unused server HTTP API endpoints (used by neither Portal nor Worker), document the audit, and safely remove those endpoints while **keeping all infrastructure endpoints** (e.g. `/healthz`, `/openapi.json`, `/swagger/`) unchanged.

## Route audit

Infrastructure endpoints are **kept** and not considered for removal.

| Method | Path | Used by | Unused? | Recommendation |
|--------|------|---------|--------|----------------|
| GET | /healthz | Ops / tooling | No | **Keep** (infrastructure) |
| GET | /openapi.json | Ops / Swagger UI | No | **Keep** (infrastructure) |
| GET | /swagger/, /swagger/index.html, /swagger | Swagger UI | No | **Keep** (infrastructure) |
| POST | /api/otp/request | Portal (login flow) | No | Keep |
| POST | /api/login | Portal | No | Keep |
| GET | /api/workspaces | Portal | No | Keep |
| POST | /api/workspaces | Portal | No | Keep |
| GET | /api/workspaces/{workspace_id}/agents | Portal (AgentList, getAgents) | No | Keep |
| POST | /api/workspaces/{workspace_id}/agents | Portal (createAgent) | No | Keep |
| GET | /api/workspaces/{workspace_id}/agents/{agent_id} | — | Yes (unused) | **Keep** — retained for future agent-detail use |
| GET | /api/workspaces/{workspace_id}/chats | Portal (getChats, getChatsPaginated) | No | Keep |
| POST | /api/workspaces/{workspace_id}/chats | Portal (createChat) | No | Keep |
| POST | /api/workspaces/{workspace_id}/chats/{chat_id}/runs | Portal (createChatRun) | No | Keep |
| GET | /api/workspaces/{workspace_id}/artifacts | Portal (getArtifacts) | No | Keep |
| GET | /api/workspaces/{workspace_id}/artifacts/{chat_run_id}/items | Portal (getArtifactItems) | No | Keep |
| GET | /api/workspaces/{workspace_id}/artifacts/{chat_run_id}/content | Portal (getArtifactContent) | No | Keep |
| POST | /api/workspaces/{workspace_id}/upload | Portal (uploadFiles) | No | Keep |
| GET | /api/workspaces/{workspace_id}/files | Portal (getFileTree) | No | Keep |
| GET | /api/workspaces/{workspace_id}/files/{path...} | Portal (getFileContent) | No | Keep |
| GET | /api/workspaces/{workspace_id}/chats/{chat_id}/conversation | Portal (getChatConversation) | No | Keep |
| GET | /api/workspaces/{workspace_id}/chats/{chat_id}/stream | Portal (subscribeChatStream, ChatDetail) | No | Keep |
| GET | /api/workspaces/{workspace_id}/chats/{chat_id}/runs/{run_id}/stream | — | **Yes** | **Remove** — run-scoped stream not used; chat-level stream is used; design 005 prefers chat stream |
| GET | /api/worker/chat-runs/{chat_run_id} | Worker (GetWorkerChatRun) | No | Keep |
| PATCH | /api/worker/chat-runs/{chat_run_id} | Worker (UpdateRunStatus) | No | Keep |
| POST | /api/worker/chat-runs/{chat_run_id}/stream | Worker (SendDelta) | No | Keep |

**Summary:** Remove **1** endpoint (run-scoped stream); keep all others including all infrastructure endpoints and the single-agent GET.

## Modules

| Module | Responsibility | Relevant change |
|--------|----------------|------------------|
| **internal/server** | HTTP routes and handlers | Remove one route registration and one handler (run stream); keep healthz, openapi, swagger and agents/{agent_id} unchanged |
| **internal/server/static** | OpenAPI spec | Remove one path entry (run stream) |
| **portal/src/lib/api** | Portal API client | Remove one unused function that called the removed run stream endpoint |

## Structure and removal targets

**internal/server/server.go**

- Remove the single line that registers `GET /api/workspaces/{workspace_id}/chats/{chat_id}/runs/{run_id}/stream` → `s.getRunStreamHandler`. Do **not** remove the `agents/{agent_id}` route.

**internal/server/stream_handlers.go**

- Remove `getRunStreamHandler` (the method and its doc comment). Keep `getChatStreamHandler`, `writeSSE`, and shared helpers unchanged.

**internal/server/static/openapi.json**

- Remove the path entry `"/api/workspaces/{workspace_id}/chats/{chat_id}/runs/{run_id}/stream"` (entire key and value). Do **not** remove the agents/{agent_id} path.

**portal/src/lib/api/index.ts**

- Remove the exported function `subscribeRunStream` and its JSDoc (it is not imported anywhere; it targeted the removed run stream endpoint). No other API functions or exports are changed.

## Method / code design (removals only)

| Location | Action | Detail |
|----------|--------|--------|
| **Server** `New()` | Delete one `mux.HandleFunc` | For `GET .../chats/.../runs/{run_id}/stream` and handler `getRunStreamHandler` only |
| **stream_handlers.go** | Delete func `getRunStreamHandler` | Full method body and comment |
| **openapi.json** | Delete one path key | `"/api/workspaces/{workspace_id}/chats/{chat_id}/runs/{run_id}/stream"` only |
| **portal api** | Delete function | `subscribeRunStream` (and its comment block) |

No new types or methods. No changes to handler signatures of remaining handlers. No changes to Config or Server struct.

## How they work together

- After removal, Portal and Worker call only the remaining endpoints; no code path will reference the removed routes.
- Infrastructure endpoints (`/healthz`, `/openapi.json`, `/swagger/`) are untouched and remain the way ops or tooling check liveness and discover the API.

## Tests

- **internal/server**: Existing tests that hit other routes remain. If any test explicitly calls `getRunStreamHandler` or the removed run stream path, remove or adjust those tests so they do not reference the removed handler/path.
- Run `go build ./...` and `go test ./...` after changes; fix any broken references (e.g. in server tests or OpenAPI consumers).

## Changes for review

- **Modified**: `internal/server/server.go` — remove one `mux.HandleFunc` line (runs/{run_id}/stream only).
- **Modified**: `internal/server/stream_handlers.go` — remove `getRunStreamHandler` method.
- **Modified**: `internal/server/static/openapi.json` — remove one path entry (chats/.../runs/{run_id}/stream only).
- **Modified**: `portal/src/lib/api/index.ts` — remove `subscribeRunStream` function and its JSDoc.
- **Optional**: If any test file references the removed run stream handler or path, **modified** those test files to remove or update the cases.

No new files. No new packages. Infrastructure endpoints (healthz, openapi, swagger) unchanged.
