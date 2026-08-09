# User-Scoped Ownership Refactor

## Goal

Remove `workspace` as the primary product/domain container and replace the current ownership model with a simpler hierarchy:

- `user` is the top-level concept
- `agent` belongs to `user`
- `conversation` belongs to `user`
- `task` belongs to `conversation`
- `task_run` belongs to `task`
- webhook keys belong to `user`
- uploaded files belong to `user`

This refactor also includes:

- portal API route changes
- server-worker contract changes
- local filesystem layout changes
- MinIO/S3 object key changes

Assumption for this refactor:

- the project is still in development
- dropping and recreating DB tables is acceptable
- we do **not** need a compatibility DB migration path

We should still keep the storage-path refactor explicit, because MinIO/local storage and worker/runtime code are coupled to the old `workspace_id` layout.

---

## Why

Today `workspace` is overloaded:

- ownership boundary
- authorization boundary
- route prefix
- blob storage partition key
- executor/runtime directory partition key
- webhook key scope

That makes the model feel unnatural and forces unrelated concepts to hang under the same container.

The new model makes ownership more direct:

- users own their own resources
- conversations are the parent for background work
- tasks are no longer globally grouped under a workspace
- storage and execution align with the actual resource graph

---

## Target domain model

### Ownership graph

```text
user
├── agents
├── conversations
│   ├── messages
│   └── tasks
│       └── task_runs
├── webhook_keys
└── files
```

### Entity shape

#### User

Unchanged as the top-level owner.

#### Agent

- keep `agent_id`
- replace `workspace_id` with `user_id`

#### Conversation

- keep `conversation_id`
- replace `workspace_id` with `user_id`
- keep `channel`, `title`, `created_by`, `created_at`

`created_by` may remain for audit clarity even though ownership is via `user_id`.

#### Task

- keep `task_id`
- remove `workspace_id`
- make `conversation_id` required
- keep `created_by`, `agent_id`, `status`, `input`, `output`, `last_run_id`, `session_id`

Tasks belong to exactly one conversation.

#### TaskRun

- unchanged parent relationship: `task_run.task_id -> task.task_id`

#### WebhookKey

- replace `workspace_id` with `user_id`

#### Files

Files remain blob-stored, not DB-backed, but they become user-owned rather than workspace-owned.

---

## Non-goals

- preserve backward-compatible workspace APIs
- preserve old DB tables or columns
- preserve old worker response shapes
- preserve old MinIO/local-storage object layout

If we want temporary dual-read compatibility for old blob keys during local testing, that can be added separately, but it is not required by this design.

---

## API design

## Auth rule

All user-facing API routes remain Bearer-token authenticated.

Ownership checks must follow the new graph:

- agent belongs to authenticated user
- conversation belongs to authenticated user
- task belongs to a conversation that belongs to authenticated user
- task run belongs to a task that belongs to a conversation that belongs to authenticated user
- webhook key belongs to authenticated user
- files are read/written under authenticated user

There should be no `user owns workspace` checks left in the portal API.

## Route shape

### User-scoped resources

```text
GET    /api/agents
POST   /api/agents
GET    /api/agents/{agent_id}
PATCH  /api/agents/{agent_id}
DELETE /api/agents/{agent_id}

GET    /api/files
GET    /api/files/{path...}
POST   /api/upload

GET    /api/webhook-keys
POST   /api/webhook-keys
DELETE /api/webhook-keys/{key_id}

GET    /api/conversations
POST   /api/conversations
GET    /api/conversations/{conversation_id}/messages
POST   /api/conversations/{conversation_id}/messages
```

### Conversation-nested task routes

```text
GET  /api/conversations/{conversation_id}/tasks
POST /api/conversations/{conversation_id}/tasks
```

### Task-scoped routes

```text
GET  /api/tasks/{task_id}
POST /api/tasks/{task_id}/runs
GET  /api/tasks/{task_id}/conversation
GET  /api/tasks/{task_id}/stream
```

### Run/artifact routes

Recommended shape:

```text
GET /api/tasks/{task_id}/artifacts
GET /api/task-runs/{task_run_id}/artifacts/items
GET /api/task-runs/{task_run_id}/artifacts/content?path=
```

Alternative acceptable shape:

```text
GET /api/conversations/{conversation_id}/artifacts
```

But the first option is simpler because artifacts are run-scoped.

## Webhook route

Workspace should disappear from the webhook route.

Recommended route:

```text
POST /api/webhook
```

Authentication:

- `Authorization: Bearer <webhook key>` or `X-Webhook-Key`
- key resolves to `user_id`

Behavior:

- create or select a conversation for that user
- create task(s) through the normal conversation/task path

This repo should avoid a route like `/api/workspaces/{workspace_id}/webhook` after the refactor.

---

## Portal handler changes

### Remove

- `withWorkspaceAuth`
- `withWorkspaceAndStore`
- `WorkspaceStore.WorkspaceBelongsToUser`

### Replace with

Resource-based ownership checks:

- `getAgentForUser`
- `getConversationForUser`
- `getTaskForUser`
- `getTaskRunForUser`
- `getWebhookKeyForUser`

Handlers should never trust a route param alone. They should always load the entity and verify ownership through the new graph.

---

## Store interface changes

## AgentStore

Replace workspace-scoped methods with user-scoped methods:

```go
ListAgentsByUser(ctx context.Context, userID string) ([]Agent, error)
CreateAgent(ctx context.Context, userID, name, description, instructions string) (*Agent, error)
UpdateAgent(ctx context.Context, agentID, userID, name, description, instructions string) (*Agent, error)
DeleteAgent(ctx context.Context, agentID, userID string) error
```

## ConversationStore

Replace workspace-scoped methods with user-scoped methods:

```go
CreateConversation(ctx context.Context, userID, channel, createdBy string) (*Conversation, error)
ListConversationsByUser(ctx context.Context, userID string, limit, offset int) ([]Conversation, int, error)
```

## TaskStore

Replace workspace list methods with conversation-based and user-checked methods:

```go
ListChatsByConversation(ctx context.Context, conversationID string, order string) ([]Chat, error)
ListChatsByConversationPaginated(ctx context.Context, conversationID string, executedOnly bool, limit, offset int) ([]Chat, int, error)
CreateChat(ctx context.Context, in *CreateChatInput) (*Chat, error)
GetChat(ctx context.Context, chatID string) (*Chat, error)
```

`CreateChatInput` should require `ConversationID` and should not include `WorkspaceID`.

## Artifact / run listing

Replace workspace-based artifact listing with task- or conversation-based listing:

```go
ListRunOutputsByTask(ctx context.Context, chatID string) ([]ArtifactWithChat, error)
```

If the UI needs a conversation-level artifact feed, add:

```go
ListRunOutputsByConversation(ctx context.Context, conversationID string) ([]ArtifactWithChat, error)
```

## WebhookKeyStore

Replace workspace-scoped methods with user-scoped methods:

```go
CreateKey(ctx context.Context, userID, name string) (plaintextKey, keyID string, err error)
GetUserIDByKey(ctx context.Context, plaintextKey string) (userID string, err error)
ListKeys(ctx context.Context, userID string) ([]WebhookKeyMeta, error)
RevokeKey(ctx context.Context, userID, keyID string) error
```

---

## Application service changes

## Chat service

`CreateChatCmd` should change from workspace-scoped to conversation-scoped:

```go
type CreateChatCmd struct {
    ConversationID string
    UserID         string
    Input          string
    AgentID        *string
}
```

Notes:

- `ConversationID` should be required
- `resolveInput` should validate that `agent.user_id == user_id`
- `CreateChat` should no longer accept `WorkspaceID`

## Conversation service

`HandleTurnCmd` should drop `WorkspaceID`:

```go
type HandleTurnCmd struct {
    UserID         string
    Channel        string
    Message        string
    ChatID         string
    ConversationID string
    StreamSink     llm.StreamSink
}
```

Conversation tools should stop speaking in terms of "current workspace".

Examples:

- "List recent tasks in this conversation"
- "Get details for a task in this conversation"
- "Continue a task in this conversation"

If the product wants user-wide task discovery later, that should be explicit and not implicit through a removed workspace concept.

---

## Conversation/tool runtime changes

The low-level conversation package currently carries `WorkspaceID` through:

- turn types
- tool builders
- webhook adapter requests
- tool-runner interfaces

That should be replaced with `UserID` and `ConversationID` as needed.

Recommended rules:

- tools that create or list tasks should use `conversation_id`
- tools that validate agents should use `user_id`
- webhook adapter input should resolve to `user_id`, not `workspace_id`

---

## Worker/server communication

This refactor must update the worker API contract at the same time as the DB and storage changes.

Today the worker response includes `workspace_id`, and executor code uses it to:

- materialize uploaded files
- build run directories
- build artifact/blob keys

That contract should move to the actual ownership and execution identifiers.

## Target worker GET response

`GET /api/worker/task-runs/{task_run_id}`

Recommended task payload:

```json
{
  "run": {
    "task_run_id": "r_xxx",
    "task_id": "t_xxx",
    "input": "..."
  },
  "task": {
    "task_id": "t_xxx",
    "conversation_id": "c_xxx",
    "user_id": "u_xxx",
    "session_id": "uuid-or-null",
    "last_run_id": "r_prev-or-null"
  }
}
```

Required fields for worker execution:

- `user_id`
- `conversation_id`
- `task_id`
- `task_run_id`
- `session_id`
- `last_run_id`

`workspace_id` should not appear in the worker contract after the refactor.

## PATCH/update flow

The existing PATCH flow can stay structurally similar, but any server-side artifact registration and task syncing logic must stop relying on workspace lineage.

It should derive ownership via:

- task run -> task -> conversation -> user

---

## Executor and runtime path layout

## Current layout

Today the runtime layout is effectively:

```text
<root>/<workspace_id>/home
<root>/<workspace_id>/tasks/<task_id>/<task_run_id>/home
<root>/<workspace_id>/tasks/<task_id>/<task_run_id>/artifacts
<root>/<workspace_id>/tasks/<task_id>/<task_run_id>/global
<root>/<workspace_id>/artifacts/<task_id>/<task_run_id>
```

## Target layout

Recommended new local layout:

```text
<root>/<user_id>/home
<root>/<user_id>/conversations/<conversation_id>/tasks/<task_id>/<task_run_id>/home
<root>/<user_id>/conversations/<conversation_id>/tasks/<task_id>/<task_run_id>/artifacts
<root>/<user_id>/conversations/<conversation_id>/tasks/<task_id>/<task_run_id>/global
<root>/<user_id>/artifacts/<conversation_id>/<task_id>/<task_run_id>
```

Rationale:

- user-owned uploads live under `user_id/home`
- run directories include `conversation_id`, which matches the new domain model
- task artifacts remain easy to locate

`internal/executor/paths.go` should be updated to take:

- `userID`
- `conversationID`
- `taskID`
- `taskRunID`

The old `WorkspacePaths` interface should be renamed to reflect the new scope.

Suggested replacement:

```go
type RuntimePaths interface {
    PersistentUserDir(userID string) string
    RuntimeTaskRunDir(userID, conversationID, taskID, taskRunID string) string
    RuntimeTaskRunHomeDir(userID, conversationID, taskID, taskRunID string) string
    RuntimeTaskRunArtifactsDir(userID, conversationID, taskID, taskRunID string) string
    RuntimeTaskRunGlobalDir(userID, conversationID, taskID, taskRunID string) string
    RunOutputDir(userID, conversationID, taskID, taskRunID string) string
}
```

---

## Blob / MinIO / S3 object layout

This is a required part of the refactor.

## Persisted user files

### Current

```text
<prefix>/<workspace_id>/home/<rel_path>
```

### Target

```text
<prefix>/<user_id>/home/<rel_path>
```

## Run global state

### Current

```text
<prefix>/<workspace_id>/tasks/<task_id>/<task_run_id>/global/<rel_path>
```

### Target

```text
<prefix>/<user_id>/conversations/<conversation_id>/tasks/<task_id>/<task_run_id>/global/<rel_path>
```

## Run artifacts staged in persist storage

### Current

```text
<prefix>/<workspace_id>/tasks/<task_id>/<task_run_id>/artifacts/<rel_path>
```

### Target

```text
<prefix>/<user_id>/conversations/<conversation_id>/tasks/<task_id>/<task_run_id>/artifacts/<rel_path>
```

## Final artifact storage

### Current

```text
<prefix>/<workspace_id>/artifacts/<task_id>/<task_run_id>/<rel_path>
```

### Target

```text
<prefix>/<user_id>/artifacts/<conversation_id>/<task_id>/<task_run_id>/<rel_path>
```

## Blob interface changes

Current blob types use `WorkspaceID`.

They should be changed to carry the identifiers actually needed for key construction:

```go
type RunObjectRef struct {
    UserID         string
    ConversationID string
    ChatID         string
    TaskRunID      string
    RelPath        string
}

type RunRef struct {
    UserID         string
    ConversationID string
    ChatID         string
    TaskRunID      string
}
```

`PersistStorage` should become user-scoped:

```go
Put(ctx context.Context, userID string, relPath string, r io.Reader) error
Get(ctx context.Context, userID string, relPath string) ([]byte, error)
ListFiles(ctx context.Context, userID string) ([]string, error)
MaterializeToDir(ctx context.Context, userID string, dstDir string) error
```

All MinIO/local-FS key builders and tests should be updated accordingly.

---

## Webhook behavior

Webhook keys become user-scoped, not conversation-scoped.

When a webhook event arrives, the server needs a deterministic rule for where the work goes.

Recommended behavior for now:

- one webhook request creates a new conversation for that user when no conversation target is specified
- that conversation receives the webhook message
- any resulting tasks belong to that new conversation

This keeps webhook behavior simple and aligned with the new ownership graph.

If later we need stable webhook-to-conversation mapping, add an explicit target field or webhook endpoint configuration.

---

## Clean rebuild strategy

Because DB migration compatibility is not required:

- update GORM models directly
- update `AutoMigrate` to the new shapes
- remove obsolete workspace tables and columns from code
- drop and recreate the local/dev database

However, code changes still need to be sequenced so the repo remains buildable.

---

## Suggested implementation order

1. Update design docs and route contract docs.
2. Refactor entity models and store interfaces to the new ownership model.
3. Refactor portal auth helpers from workspace checks to resource ownership checks.
4. Refactor agent, conversation, task, artifact, file, and webhook handlers to new routes.
5. Refactor application services to use `user_id` and `conversation_id`.
6. Refactor low-level conversation tool runners and prompt text.
7. Refactor worker API response structs and handler implementations.
8. Refactor executor path builders and worker execution scope.
9. Refactor blob interfaces, MinIO key builders, and local-FS layout.
10. Update tests across entity, server, executor, blob, and conversation packages.
11. Drop and rebuild local/dev tables and storage as needed.

---

## Acceptance criteria

- no portal route requires `workspace_id`
- no ownership check depends on `workspace`
- no DB model uses workspace as the parent of agent/conversation/task/webhook key
- tasks are always created under a conversation
- agents are always validated against `user_id`
- uploaded files are stored and read under `user_id`
- worker API payloads do not include `workspace_id`
- executor local directories do not use `workspace_id`
- MinIO/S3 object keys do not use `workspace_id`
- user-facing and tool-facing text no longer says "current workspace" unless referring to a filesystem working directory only

---

## Open questions

### Artifact listing scope

Do we want artifact feeds at:

- task level
- conversation level
- both

Task-level is enough for the refactor and is simpler to authorize.

### Webhook conversation behavior

Should each webhook request create:

- a new conversation every time
- or reuse one stable conversation per webhook integration

Default recommendation: create a new conversation every time until a stronger use case appears.

### Route cleanup timing

Do we remove all workspace routes immediately, or keep stubs returning errors during portal transition?

Given the repo is still under active development, immediate removal is acceptable if portal is updated in the same change set.
