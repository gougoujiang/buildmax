# Design 110 — New task from agent should also use conversation based

## Goal

Inject the user's available agents into the `StartTask` tool description so Tier 1 knows which agents exist, include agent ID in the portal's initial message when starting from an agent card, and rename the UI from "New Task" to "New Conversation".

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/tools** | Tool definitions for agent loop | `AgentSummary`, `startTaskTool`, `NewStartTaskTool` |
| **internal/conversation** | Low-level Tier 1 LLM loop; builds tools from runners | `ConversationToolRunners`, `buildConversationTools` |
| **internal/app/conversation** | Tier 1 orchestration; fetches agents, wires into tool runners | `Service`, `handleConversationTurn` |
| **internal/server/portal** | Portal handler; constructs `Service` with dependencies | `conversationService()` |
| **portal/src/pages** | Agent list page | `AgentList.tsx` |
| **portal/src/components** | Agent-to-conversation modal | `NewTaskFromAgentModal.tsx` |

## Structure

**Directory / files**

- `internal/tools/`
  - `start_chat.go` — **modified**; add `AgentSummary` type, `agents` field on `startTaskTool`, dynamic `Description()`, update `NewStartTaskTool` signature, update `Execute` return message
- `internal/conversation/`
  - `agent.go` — **modified**; add `AgentSummaries` field to `ConversationToolRunners`, pass to `NewStartTaskTool` in `buildConversationTools`
- `internal/app/conversation/`
  - `service.go` — **modified**; add `AgentStore` field to `Service`, fetch agents in `handleConversationTurn`, convert to `AgentSummary` slice, set on runners
- `internal/server/portal/`
  - `conversation_service.go` — **modified**; pass `cfg.AgentStore` to `convapp.Service`
- `portal/src/pages/`
  - `AgentList.tsx` — **modified**; rename button text "New task" → "New Conversation", rename aria-label
- `portal/src/components/`
  - `NewTaskFromAgentModal.tsx` — **modified**; update `buildAgentPreview` to include agent ID and a prompt line; rename modal title and submit button text

## Main types

### `tools.AgentSummary` (new)

```go
type AgentSummary struct {
    ID          string
    Name        string
    Description string
}
```

Simple value type used to inject available agent info into the `StartTask` tool description. No methods.

### `startTaskTool` (modified)

```go
type startTaskTool struct {
    scopeID string
    userID  string
    runner  StartTaskRunner
    agents  []AgentSummary  // new
}
```

`Description()` returns the base description plus, when `agents` is non-empty, an appended "Available agents" section listing each agent.

## Method design

| Receiver | Method | Signature | Change | Responsibility |
|----------|--------|-----------|--------|----------------|
| `startTaskTool` | Description | `() string` | **modified** | Returns base description; when `t.agents` is non-empty, appends `\n\nAvailable agents:\n- name (id: xxx) - desc` for each agent |
| `startTaskTool` | Execute | `(ctx, args) (string, error)` | **modified** | Update return message: remove task_id/run_id exposure, say "Background task started." |
| — | NewStartTaskTool | `(scopeID, userID string, runner StartTaskRunner, agents []AgentSummary) core.Tool` | **modified** | Accepts agents slice; passes to `startTaskTool` |
| — | buildConversationTools | `(scopeID, userID string, runners *ConversationToolRunners) []core.Tool` | **modified** | Passes `runners.AgentSummaries` to `NewStartTaskTool` |
| `Service` | handleConversationTurn | `(ctx, cmd) (HandleTurnResult, error)` | **modified** | Fetches agents from `AgentStore`, converts to `[]tools.AgentSummary`, sets on runners before calling `coreconv.Run` |
| `Handler` | conversationService | `() *convapp.Service` | **modified** | Includes `AgentStore: h.cfg.AgentStore` |

## How they work together

**Data flow — agent summaries to tool description**

1. **Portal handler** constructs `convapp.Service` with `AgentStore: h.cfg.AgentStore` (already available on `Config`).
2. **User sends message** via WebSocket → `wsConn.executeConversationTurn` → `svc.HandleTurn(cmd)`.
3. **`handleConversationTurn`**: if `s.AgentStore != nil`, calls `s.AgentStore.ListAgentsByUser(ctx, cmd.UserID)`. Converts result to `[]tools.AgentSummary`.
4. **`conversationToolRunners`** receives the summaries and sets `AgentSummaries` on the returned `*ConversationToolRunners`.
5. **`coreconv.Run`** → `prepareRun` → `buildConversationTools(scopeID, userID, runners)` → calls `tools.NewStartTaskTool(scopeID, userID, runners.StartTask, runners.AgentSummaries)`.
6. **`startTaskTool.Description()`** renders the base text plus the "Available agents" list.
7. **LLM sees** the tool description with agent names and IDs, and can call `StartTask(input=..., agent_id=<id>)` correctly.

**Data flow — portal "New Conversation" from agent card**

1. User clicks **"New Conversation"** on agent card → opens modal.
2. Modal shows prefilled text including agent ID: `Agent: name (id: a_xxx)\nDescription: ...\nInstructions: ...\n\nPlease start a background task with this agent.`
3. User optionally edits, clicks **"Start"** → `handleStartTaskFromAgent(input)`.
4. Portal creates conversation via REST, sets `pendingConversation`, navigates to `ConversationDetail`.
5. `ConversationDetail` sends initial message via WebSocket → Tier 1 sees message with agent context and ID → LLM calls `StartTask` with correct `agent_id`.

**Dependencies**

- `internal/tools` — no new imports
- `internal/conversation` — imports `internal/tools` (existing)
- `internal/app/conversation` — imports `internal/storage/entity` (existing), `internal/tools` (new import for `AgentSummary`)
- `internal/server/portal` — no new imports (already has `AgentStore` on Config)

## Changes for review

- **Modified**: `internal/tools/start_chat.go` — add `AgentSummary` type, `agents` field on `startTaskTool`, dynamic `Description()`, update `NewStartTaskTool` signature (add `agents []AgentSummary`), update `Execute` return message
- **Modified**: `internal/conversation/agent.go` — add `AgentSummaries []tools.AgentSummary` to `ConversationToolRunners`, pass to `NewStartTaskTool` in `buildConversationTools`
- **Modified**: `internal/app/conversation/service.go` — add `AgentStore entity.AgentStore` to `Service`, fetch agents in `handleConversationTurn`, convert to summaries, pass to `conversationToolRunners`
- **Modified**: `internal/server/portal/conversation_service.go` — add `AgentStore: h.cfg.AgentStore` in `conversationService()`
- **Modified**: `portal/src/pages/AgentList.tsx` — button text "New task" → "New Conversation", aria-label update
- **Modified**: `portal/src/components/NewTaskFromAgentModal.tsx` — `buildAgentPreview` includes agent ID + prompt line, modal title and submit button text updated
