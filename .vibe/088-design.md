# Design 088: New chat from an agent

## Goal

Allow users to create a new chat from a workspace agent so that the chat’s `input` is composed from the agent’s name, description, and instructions (LLM receives task context via the first message). Expose “start new chat from agent” in the Portal. No changes to worker/executor or AGENTS.md.

## Modules and responsibilities

| Module | Responsibility |
|--------|----------------|
| **internal/model** | Add optional `AgentID` to `Chat`. |
| **internal/storage/entity** | Persist `agent_id` on chat; extend `CreateChat` to accept optional `agentID`; `ChatStore` interface updated. |
| **internal/server** | `POST .../chats`: accept optional `agent_id`; when present, validate agent in workspace, compose `input` from agent (name, description, instructions) + optional user `input`, then create chat with that `input` and `agent_id`. Response and list include `agent_id`. |
| **portal** | API client and types: optional `agent_id` on create and on `ApiChat`; “New chat” action from agent list (and optionally agent detail) that calls create with `agent_id`, then navigates to chat. |
| **OpenAPI** | Create-chat body: optional `agent_id`; chat response schema: optional `agent_id`. |

## Structure and data flow

### Composed input format

When `agent_id` is provided, the stored chat `input` is built as:

```
<name>: {agent.Name}
<description>: {agent.Description}
<instructions>: {agent.Instructions}
```

If the client also sends a non-empty `input`, append it after a blank line:

```
<name>: ...
<description>: ...
<instructions>: ...

{user input}
```

Implementation: a single helper (e.g. in `server` or a small internal package) that takes `*Agent` and optional `userInput string` and returns the composed string. No new package required; server is fine.

### Create-chat flow (backend)

1. Handler receives `POST /api/workspaces/{workspace_id}/chats` with body `{ "input"?: string, "agent_id"?: string }`.
2. If `agent_id` is present and non-empty:
   - Require `AgentStore`; call `GetAgent(ctx, agent_id)`.
   - If agent is nil or `agent.WorkspaceID != workspaceID`, respond 400 (e.g. "agent not found or not in workspace").
   - Compose `input := buildChatInputFromAgent(agent, req.Input)` (agent block + optional `req.Input` appended).
   - Use composed `input` for title truncation and for `CreateChat`. Pass `agent_id` into store so the new chat row gets `agent_id` set.
3. If `agent_id` is absent or empty:
   - Require `req.Input != ""`; otherwise 400 "input required".
   - Current behavior: title from truncate or title generator, `CreateChat(..., req.Input, ...)` with `agentID == nil`.
4. Title generation: use the same logic as today (truncate composed or plain input, or LLM title if configured). Quota check unchanged.
5. Call `ChatStore.CreateChat(ctx, workspaceID, input, title, createdBy, titlePromptTokens, titleCompletionTokens, agentID)` where `agentID` is `*string` (nil when no agent).

### Persistence

- **model.Chat**: add `AgentID *string` with `gorm:"type:varchar(64);index" json:"agent_id,omitempty"`. Table name remains `chat`; GORM AutoMigrate will add the column.
- **entity.Chat**: alias of `model.Chat`, so it gains `AgentID` automatically.
- **ChatStore.CreateChat**: add final parameter `agentID *string`. In `entity/chat.go`, when building the `Chat` struct, set `c.AgentID = agentID`. The first `ChatRun` still gets `run.Input = input` (the composed or user input).

### API contract

- **Request** `POST /api/workspaces/{workspace_id}/chats`:
  - `input`: optional when `agent_id` is present; required when `agent_id` is absent.
  - `agent_id`: optional; when present, must be a valid agent in the same workspace.
- **Response** (create and list): chat object includes `agent_id` (string or null) when set.
- **Errors**: 400 if `agent_id` is provided but agent not found or wrong workspace; 400 if neither `agent_id` nor `input` provided (or `input` required when no `agent_id`).

### Portal

- **Types**: `ApiChat` gains optional `agent_id?: string`. Create-chat body type: `{ input?: string; agent_id?: string }`.
- **API**: `createChat(workspaceId, { input?: string, agent_id?: string }, token)`. Backend enforces rules (input optional only when agent_id present).
- **Agent list**: On each agent card, add a “New chat” (or “Start chat”) control. On click: call `createChat(workspaceId, { agent_id: a.id }, token)` (no user input for minimal scope), then on success navigate to `{ name: "chat", workspaceId, chatId: chat.id }`. Optionally allow a short prompt (e.g. modal or inline field) to pass as `input`; for this task at least the no-input path is required.
- **New-chat page**: Optionally allow selecting an agent (e.g. dropdown) and sending `agent_id` with or without `input`; at least one path (“from agent list”) must work.

## Method and interface design

### internal/model

- **Chat**: add field  
  `AgentID *string \`gorm:"type:varchar(64);index" json:"agent_id,omitempty"\``

### internal/storage/entity

- **ChatStore.CreateChat** (interface and implementation):  
  - Signature: `CreateChat(ctx context.Context, workspaceID, input, title, createdBy string, titlePromptTokens, titleCompletionTokens int, agentID *string) (*Chat, error)`  
  - In implementation: set `c.AgentID = agentID` on the new `Chat` before create.

### internal/server

- **createChatRequest**: add `AgentID *string \`json:"agent_id,omitempty"\``.
- **ChatResponse**: add `AgentID *string \`json:"agent_id,omitempty"\``.
- **chatToResponse**: set `AgentID: c.AgentID`.
- **createWorkspaceChatHandler** (logic):
  - Decode body; if `req.AgentID != nil && *req.AgentID != ""`: require `s.cfg.AgentStore`, get agent, validate workspace, compose input via `buildChatInputFromAgent(agent, req.Input)`, set `agentID = req.AgentID`; else require `req.Input != ""`, use `req.Input` and `agentID = nil`.
  - Title and quota as today; call `CreateChat(..., agentID)`.
- **buildChatInputFromAgent(agent *model.Agent, userInput string) string**: build the three-line block from agent name/description/instructions; if `userInput != ""` append `"\n\n" + userInput`; return.

### portal

- **lib/api/types.ts**: `ApiChat` add `agent_id?: string`. Create body: extend to `{ input?: string; agent_id?: string }` (input required only when agent_id not sent; server enforces).
- **lib/api/index.ts**: `createChat(workspaceId, body: { input?: string; agent_id?: string }, token)`.
- **AgentList.tsx**: per card, add “New chat” button; on click call `createChat(workspaceId, { agent_id: a.id }, token)`, then `navigate({ name: "chat", workspaceId, chatId: chat.id })`; handle loading and errors.

## How they work together

1. User clicks “New chat” on an agent card in the Portal. Frontend calls `createChat(workspaceId, { agent_id: id }, token)`.
2. Server receives request; loads agent, validates workspace, composes input from agent fields, calls `ChatStore.CreateChat(..., &id)` with composed input.
3. Store creates chat and first run with that input and `agent_id` set; returns chat.
4. Server responds with chat (including `agent_id`). Portal navigates to the chat view.
5. Scheduler picks up the PENDING run; worker runs `buildmax -p` with existing run directory; the first user message in the run is the composed input (no AGENTS.md or worker changes).

## OpenAPI changes

- **POST /api/workspaces/{workspace_id}/chats** request body: remove `required: ["input"]`; add `agent_id` to properties (type string, optional). Document: “When agent_id is provided, input is optional and the stored input is composed from the agent; when agent_id is absent, input is required.”
- **Chat response** (create and list): add `agent_id` (string, nullable) to the schema for chat objects.

## Changes for review

| Area | Change |
|------|--------|
| **internal/model** | Add `AgentID *string` to `Chat` with gorm and json tags. |
| **internal/storage/entity** | `ChatStore.CreateChat`: add parameter `agentID *string`; in `chat.go` set `c.AgentID = agentID` when creating chat. |
| **internal/server** | `createChatRequest`: add `AgentID *string`. `ChatResponse`: add `AgentID *string`. `chatToResponse`: map `AgentID`. New helper `buildChatInputFromAgent(agent, userInput string) string`. `createWorkspaceChatHandler`: when `agent_id` present, require AgentStore, get agent, validate workspace, compose input, pass agentID to CreateChat; when absent, require input; call CreateChat with optional agentID. |
| **internal/server/static/openapi.json** | Create-chat body: optional `input`, optional `agent_id`; adjust required array and description. Chat response schema: add `agent_id` (nullable). |
| **portal/src/lib/api/types.ts** | `ApiChat`: add `agent_id?: string`. Create-chat body type: `input?` and `agent_id?`. |
| **portal/src/lib/api/index.ts** | `createChat(workspaceId, body, token)` accept body with optional `input` and `agent_id`. |
| **portal/src/pages/AgentList.tsx** | Add “New chat” button per agent card; on click create chat with `agent_id`, then navigate to chat; loading/error state. |

No changes to executor, worker, or AGENTS.md.
