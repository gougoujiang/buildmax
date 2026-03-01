# Two-Tier Agent Architecture

**Design Reference**  
*Unified entrypoint (Tier 1) and task execution (Tier 2).*

---

## 1. Overview

BuildMax introduces a **two-tier agent architecture** so that all external interactions flow through a single conceptual entrypoint before any task execution happens.

| Tier | Role | Scope |
|------|------|--------|
| **Tier 1** | Conversation / gateway | External conversations: portal chat, cron, webhook, instant messaging (e.g. Telegram) |
| **Tier 2** | Task execution | Current chat/chat_run model; invoked by Tier 1 when work is needed |

**Goals:**

- One unified entrypoint for all channels (portal, cron, webhook, IM).
- Tier 1 handles understanding, clarification, and routing; Tier 2 handles execution (tools, workspace, artifacts).
- Clear separation so different channels can share the same execution model.

---

## 2. Tier 1: Conversation / Gateway

### 2.1 Responsibilities

- **Channel normalization**: Accept input from portal, IM, cron, webhook → produce a uniform *conversation turn*.
- **Conversation state**: Multi-turn memory across task runs (today a Chat has no real conversation history between runs).
- **Intent resolution**: Decide whether to:
  - Respond conversationally (answer, ask for clarification), or
  - Spawn one or more Tier 2 task(s).
- **Task decomposition** (optional/future): Break a complex request into multiple Tier 2 tasks.
- **Response routing**: Send Tier 2 output (or direct reply) back to the originating channel (portal SSE, Telegram, webhook callback).

### 2.2 Tier 1 Is Not Always an LLM

Different channels imply different Tier 1 behavior:

| Channel | Tier 1 behavior | Needs LLM? |
|---------|------------------|------------|
| Portal chat | Multi-turn conversation, intent understanding, can ask clarifying questions, then spawn task | Yes |
| Telegram / IM | Same as portal but via message adapter | Yes |
| Cron | Structured trigger → predetermined task | No — rule-based adapter |
| Webhook | Structured payload → task mapping | No — rule-based adapter (optional template) |

So **Tier 1 is an interface**, not a single LLM agent. Implementations:

- **LLM-based conversation engine** for portal and IM.
- **Rule-based dispatcher** for cron and webhook (map trigger/payload to task input).

Making Tier 1 an LLM call for cron/webhook would add cost and latency with little benefit.

---

## 3. Tier 2: Task Execution

Tier 2 is the **current chat/chat_run model**, used as the task execution level:

- **Chat** (evolved) = conversation container; can hold Tier 1 conversation state.
- **ChatRun** = one task execution: PENDING → SCHEDULED → RUNNING → SUCCEEDED | FAILED.
- Scheduler picks PENDING runs → spawns worker → worker runs `buildmax -p` (agent loop with tools, workspace, artifacts).

No change to the execution pipeline; Tier 1 *invokes* Tier 2 by creating/updating chats and runs.

### 3.1 Relationship to `internal/agent`

The **`internal/agent`** package was originally developed for the BuildMax CLI/TUI and is also used as the **agent engine** when the worker handles chat runs (Tier 2). We can reuse its key mechanism (LLM loop, tool calling) when implementing a Tier 1 **conversation manager**, but the two usages must be kept distinct:

| Aspect | Tier 2 (current agent) | Tier 1 (conversation manager) |
|--------|------------------------|------------------------------|
| **Tools** | Workspace-oriented: read_file, writefile, editfile, bash, glob, grep, etc. | Different set: no direct workspace file access; e.g. delegate to Tier 2 (create chat run), maybe lightweight tools (reply, ask_clarification). |
| **System prompt** | Task execution: “help with software engineering tasks”, use tools, follow conventions, etc. | Conversation: understand intent, clarify, decide when to spawn a task; do not perform workspace work. |
| **Workspace** | Heavy use: CWD = run dir, tools operate on workspace files, AGENTS.md, artifacts. | **No heavy reliance.** Tier 1 does not operate on the workspace directly. All workspace-related work is achieved by **creating chat/chat runs** and letting Tier 2 execute. |

**Implication:** When adding an LLM-based Tier 1 engine (Phase 3), either use a separate agent instance with a different system prompt and tool list, or factor `internal/agent` so that system prompt, tools, and workspace binding are configurable — and Tier 1 is configured with conversation-oriented prompt and tools (e.g. “spawn_task” instead of read_file/writefile). Tier 1 must not be given the same workspace-scoped tools as Tier 2.

---

## 4. Architecture Sketch

```
                    ┌─────────────────────────────┐
                    │         Tier 1               │
                    │   "Conversation Manager"      │
                    │                              │
  Portal ──────────▶│  ┌─────────────────────┐    │
  Telegram ────────▶│  │ Channel Adapter      │    │
  Cron ────────────▶│  │ (normalize input)   │    │
  Webhook ─────────▶│  └────────┬────────────┘    │
                    │           ▼                  │
                    │  ┌─────────────────────┐    │
                    │  │ Conversation Engine  │    │
                    │  │ (LLM or rule-based) │    │
                    │  └────────┬────────────┘    │
                    │           │ spawn task      │
                    └───────────┼─────────────────┘
                                ▼
                    ┌─────────────────────────────┐
                    │         Tier 2              │
                    │   "Task Execution"           │
                    │                              │
                    │  ChatRun → Scheduler → Worker│
                    │  → Agent loop (tools, LLM)  │
                    │  → Artifacts                 │
                    └─────────────────────────────┘
```

---

## 5. Key Interfaces (Go)

Tier 1 is expressed as interfaces so we can plug in LLM or rule-based implementations and different channel adapters.

### 5.1 Channel Adapter

One implementation per channel type (portal HTTP, Telegram, cron job, webhook).

```go
// ConversationTurn is the normalized input from any channel.
type ConversationTurn struct {
    WorkspaceID string
    Channel     string   // "portal", "telegram", "cron", "webhook"
    ConversationID string   // channel-specific conversation/thread id
    UserID      string
    Message     string
    Raw         map[string]any // optional channel-specific payload
}

// ChannelAdapter normalizes channel-specific input and sends output back.
type ChannelAdapter interface {
    // Receive normalizes raw input into a ConversationTurn.
    Receive(ctx context.Context, raw any) (ConversationTurn, error)
    // Send delivers output (reply or task result) to the channel.
    Send(ctx context.Context, conversationID string, output string) error
}
```

### 5.2 Conversation Engine

Decides what to do with a turn: respond only, or spawn Tier 2 task(s).

```go
// ConversationResult is what Tier 1 returns after processing a turn.
type ConversationResult struct {
    Reply   string   // direct reply to user (if any)
    TaskIDs []string // chat_run_ids spawned (if any)
}

// ConversationEngine processes one turn; may respond and/or spawn Tier 2 tasks.
type ConversationEngine interface {
    Process(ctx context.Context, workspaceID, chatID string, turn ConversationTurn) (ConversationResult, error)
}
```

### 5.3 Usage

- **Portal**: HTTP handler receives message → adapter builds `ConversationTurn` (channel=portal) → engine `Process` → if `TaskIDs` non-empty, create chat run(s); stream or return `Reply` and run status.
- **Cron**: Cron trigger → adapter builds `ConversationTurn` (channel=cron, message from config) → rule-based engine maps to one task → create chat run; optionally `Send` summary when run completes.
- **Webhook**: Incoming request → adapter parses payload → same as cron.
- **Telegram**: Bot receives message → adapter builds `ConversationTurn` (channel=telegram) → LLM engine `Process` → `Send` reply and/or run result to chat.

---

## 6. Entity Mapping

Two options; **Option A** is recommended for less disruption.

### 6.1 Option A: Evolve Existing Entities (recommended)

- **Chat** = Tier 1 conversation container. Already has workspace, agent, status, last run. Add:
  - `channel` (portal, telegram, cron, webhook).
  - Tier 1 conversation history (new store or column): multi-turn messages for the conversation engine, separate from the Tier 2 agent session.
- **ChatRun** = Tier 2 task execution. No change.
- **Agent** (workspace persona) = Tier 1 configuration. Its `instructions` (and name/description) define Tier 1 behavior for that conversation.

### 6.2 Option B: New Entity Layer

- New **Conversation** entity: channel, state, history.
- **Chat** + **ChatRun** remain purely Tier 2 (task-scoped).
- One Conversation can spawn multiple Chats/Runs.

Option B is cleaner conceptually but adds migration and more concepts; Option A reuses Chat as the conversation handle and keeps the current API shape.

---

## 7. Concerns and Mitigations

| Concern | Mitigation |
|---------|------------|
| **Latency (portal)** | Tier 1 LLM adds 1–3s before task. Allow short-circuit: for obvious “run a task” single-turn requests, Tier 1 can skip LLM and spawn Tier 2 directly. |
| **Cost** | Two tiers = two LLM sessions per interaction when Tier 1 is LLM. Use a smaller/cheaper model for Tier 1, or rule-based for cron/webhook. |
| **Complexity** | Introduce incrementally: (1) Define interfaces + channel abstraction, (2) Portal goes through Tier 1 as pass-through, (3) Add Tier 1 LLM for portal/IM, (4) Add cron/webhook adapters. |

---

## 8. Implementation Phases (suggested)

1. **Phase 1**: Define Tier 1 interfaces (`ConversationTurn`, `ChannelAdapter`, `ConversationEngine`, `ConversationResult`) and channel enum in a new package (e.g. `internal/conversation` or under `internal/server`).
2. **Phase 2**: Implement portal as first `ChannelAdapter`; conversation engine is pass-through (every message spawns one Tier 2 run). No new LLM call yet.
3. **Phase 3**: Add Tier 1 LLM conversation for portal (and later IM): maintain conversation history, allow clarification and deferred task spawn.
4. **Phase 4**: Add cron and webhook adapters with rule-based engine (no LLM).
5. **Phase 5** (optional): Task decomposition — Tier 1 breaks one request into multiple ChatRuns.

---

## 9. References

- Current agent loop: `internal/agent/agent.go`
- Chat/ChatRun model: `internal/model/models.go`, `internal/storage/entity`
- Scheduler/worker: `internal/executor/scheduler.go`, worker binary and RunTask
- Portal product vision: [design/001-about-portal.md](001-about-portal.md)
