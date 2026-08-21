# Agent Loop

> **Audience:** contributors · **Status:** current
>
> User-facing view of the same loop: [start/concepts.md](../../start/concepts.md)

## Purpose

`internal/core/agent` implements the shared tool-calling loop: build messages,
call the LLM, execute requested tools, append results, repeat until the model
produces a final text reply. It is pure — it imports `internal/core/llm` and
nothing else from the project.

Every surface runs this loop. CLI, Desktop, and worker runs reach it through
`internal/agentapp`; Portal conversation turns reach it through
`internal/service/conversation/runtime`.

## Key Types

| Name | Kind | Role |
|---|---|---|
| **RunLoopOpts** | struct | Everything one run needs — see below |
| **RunStats** | struct | `ToolCalls`, `PromptTokens`, `CompletionTokens` for one run |
| **MessageHistory** | interface | `HistoryMessages()` and `Append(m)` — the loop works against in-memory sessions and DB-backed conversations alike |
| **CompactionHistory** | interface | Optional extension: `AddCompaction(summary, n)` so a persistent history keeps the compaction boundary across turns |
| **ContextCompactor** | interface | `Compact(ctx, msgs)` — summarizes old messages into replacement text |
| **ToolPolicy** / **ApprovalHandler** | interfaces | Allow, deny, or ask before a tool executes |
| **HookRunner** | interface | Dispatches lifecycle hooks; may block a tool call or a compaction |
| **Event** | struct | Structured runtime event delivered to `EventSink` |

## RunLoopOpts

```go
type RunLoopOpts struct {
    LLMClient    llm.LLMClient      // required
    SystemPrompt string
    ToolRegistry llm.ToolRegistry
    MaxIter      int                // DefaultMaxIterations = 200
    History      MessageHistory     // required
    StreamSink   llm.StreamSink     // non-nil selects the streaming call

    Policy    ToolPolicy            // nil = AllowAllPolicy
    Approval  ApprovalHandler       // nil collapses ToolActionAsk to Deny, and marks
                                    //   the surface as having nobody to prompt
    Grants    *SessionGrants        // caller-owned; nil grants nothing
    Compactor ContextCompactor      // nil disables compaction; TrimHistory is the fallback
    EventSink func(Event)           // nil disables event emission entirely
    Hooks     HookRunner            // nil or NoopHookRunner disables hooks

    SessionID string                // forwarded to hook payloads
    Workspace string                // forwarded to hook payloads
    IsSubagent bool                 // flips Stop to SubagentStop; stamped on every event
    AgentType  string               // subagent definition name when IsSubagent
}
```

The optional fields are what make one loop serve every surface: the CLI supplies
an approval handler and a stream sink, the worker supplies neither, and the
conversation runtime supplies a DB-backed `CompactionHistory`.

## How It Works

```text
history + system prompt ──▶ LLMClient
                                 │
                         ┌───────┴────────┐
                         │  tool_calls?   │
                         └───┬────────┬───┘
                          No │        │ Yes
                             ▼        ▼
                      return reply   for each call:
                                       policy → approval → hook
                                       execute, append result
                                       ↺ next iteration
```

1. **Build messages** — system prompt prepended to `HistoryMessages()`. The
   system prompt is never stored in history.
2. **Compact if needed** — when the context window is filling up, `Compactor`
   summarizes older messages and the summary is injected into the system prompt.
   `PreCompact` hooks can block this; without a compactor, `TrimHistory` drops
   messages instead.
3. **Call the LLM** — `ChatCompletionStreaming` when `StreamSink` is set,
   otherwise `ChatCompletionBlocking`.
4. **No tool calls** → append the assistant reply and return.
5. **Tool calls** → append the assistant message, then run each call through the
   gates below.
6. **Repeat**, up to `MaxIter`.

## Tool Execution

The calls in one assistant message run through four stages:
`parseCalls` → `groupCalls` → `gateCall` → `runGroup` → commit.

| Stage | Goroutine | Order | Does |
|---|---|---|---|
| parse | loop | batch | unmarshal arguments, resolve tools — no side effects, which is what lets it run ahead |
| group | loop | batch | cut the batch into units that execute together |
| gate | loop | call order | `EventToolStart`, the checks below, `PreToolUse` |
| run | worker | overlapped | `tool.Execute`, then `EventToolEnd` |
| commit | loop | call order | post hooks, then `History.Append` |

**Only `Execute` overlaps.** Everything that decides — the loop guard,
permission resolution, the approval prompt, `PreToolUse` — stays on the loop
goroutine in call order. That is what keeps concurrency cheap: the guard needs
no lock, approval prompts stay one at a time so neither UI handler has to become
re-entrant, and hooks still observe calls in the order the model made them.

**Grouping merges only adjacent read-only calls**, and never reorders. A write,
a shell command, an unknown tool, or a call that failed to parse is a barrier.
`agent.max_parallel_tools` bounds a group; at 1 every call is its own group,
which is the sequential behaviour exactly. The message history a run produces is
identical at any limit — `TestHistoryIsSchedulerIndependent` pins it.

Two asymmetries are deliberate. `EventToolEnd` is emitted from the worker that
ran the call, because the event stream is live and holding a completion until
the slowest sibling returns would misreport what is still running; a consumer
pairs it with `EventToolStart` by `ToolCallID`, never by arrival. Post hooks
fire at the join in call order, because they are an audit surface and one that
reorders under load is worse than one that arrives late.

Design: [design/parallel-tool-execution.md](../../design/parallel-tool-execution.md).

### Gates

Each requested call passes four checks before it runs. Every rejection is
appended to history as a tool-role message beginning with `error:`, so the model
sees the refusal and can choose a different approach rather than stalling:

| Gate | Denial message |
|---|---|
| Tool exists, arguments parse as JSON | lookup or parse error |
| **Loop guard** — the same call repeated too many times | `blocked — repeated identical call detected (loop guard)` |
| **Permission** — the layered resolution below, then `ApprovalHandler` on `ToolActionAsk` | `denied by policy` / `denied by user` |
| **PreToolUse hook** | `denied by hook: <reason>` |

The loop guard exists because a model that gets an unhelpful tool result will
often retry the identical call forever; the counter turns that into a message it
must react to.

### Permission resolution

`resolveAction` walks five layers, first decision wins. Rationale and the
per-tool table: [design/tool-permissions.md](../../design/tool-permissions.md).

| # | Layer | Source |
|---|---|---|
| 1 | configured `deny` — a prohibition | `tools.permissions` |
| 2 | `ArgChecker.CheckArgs` — argument-level risk | the tool |
| 3 | configured `allow`/`ask` — category preference | `tools.permissions` |
| 4 | `PolicyProvider.DefaultAction` — explicit tool default | the tool |
| 5 | derived from `AccessDeclarer.Access` — writes ask | the tool |

Two properties of that order are load-bearing:

- **The configured layers straddle the risk check.** `Read: allow` quiets the
  category prompt without consenting to open a sensitive path; only a
  configured `deny` outranks layer 2.
- **Layer 5 runs only when `Approval != nil`.** A category prompt is a question
  for a person, so a surface with nobody attached does not raise it rather than
  answering it with a default. Without that gate, giving `Write` a default of
  `Ask` would deny every file write on a worker. Layers 1–4 are unaffected, so a
  risky shell command is still refused there.

`ToolPolicy.Check` returns `(action, bool)` because `ToolActionAllow` means
*abstain* everywhere else — a policy returning it could never say "allow this,
stop asking".

After resolution, a session grant (`SessionGrants`) can turn an `Ask` into an
`Allow`. It is applied after resolution, never before, so it cannot soften a
denial.

## Events

When `EventSink` is set the loop emits structured events — `EventIterStart`,
`EventLLMStart`, `EventLLMDelta`, `EventLLMEnd`, `EventToolStart`,
`EventToolEnd`, `EventToolDenied`, `EventContextCompacted`, `EventRunEnd`. The sink is called **synchronously from the RunLoop
goroutine and must not block**. Nil sink means zero overhead.

This is the single seam the TUI, `--output jsonl`, and the durable run trace all
hang off. See the [durable run trace design](../../design/durable-run-trace.md).

## Cancellation

When `ctx` is cancelled mid-run, `RunLoop` returns the last assistant content
produced so far with a **nil error**. Callers get a partial result rather than an
empty failure, which is what makes interrupting a long run in the TUI useful.

Exceeding `MaxIter` is different — that returns an error.

## Dependencies

- **Uses**: `internal/core/llm` (`Message`, `ToolDef`, `ToolCall`, `LLMClient`,
  `ToolRegistry`, `StreamSink`) and the standard library. Nothing else.
- **Used by**: `internal/agentapp` and `internal/service/conversation/runtime`

## Related

- [tools.md](tools.md) — the registry and the tool implementations
- [llm-client.md](llm-client.md) — the client behind `llm.LLMClient`
- [session.md](session.md) — the local `MessageHistory` implementation
- [guide/hooks.md](../../guide/hooks.md) — the hook contract from the outside
