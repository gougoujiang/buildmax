# Parallel Tool Execution

## Status

- roadmap_priority: `unscheduled` — performance work, not yet placed in
  [../ROADMAP.md](../ROADMAP.md)
- status: `implemented` (§8 phases 1-4 landed; §6 items and §11 open)
- depends on: [tool-permissions.md](./tool-permissions.md), which defines the
  `Access` classification this design schedules on. That record ships first —
  see §11.
- follows: [hook-system.md](./hook-system.md),
  [durable-run-trace.md](./durable-run-trace.md),
  [trust-harness.md](./trust-harness.md)
- touches: `internal/core/llm`, `internal/core/agent`, `internal/tool`,
  `internal/interface/cli`, `internal/interface/desktop`, `internal/config`
- created_at: `2026-08-20`

## 1. Purpose

The model already batches its tool calls. The runtime does not run them in
parallel.

`DefaultSystemPrompt` tells the model to "call multiple tools in a single
message so they can run in parallel" (`internal/agentapp/prompt.go:51`), and
`Glob`'s description repeats the advice. The wire path honours that: one
assistant message carries N calls, and both the blocking and the streaming
client rebuild the full list (`internal/infra/llm/client.go:115`, `:247`).
Then `executeToolCalls` (`internal/core/agent/agent.go:347`) runs them in a
plain `for` loop, one after another.

So batching today buys exactly one thing — fewer LLM round trips. It buys no
wall-clock time. A turn that reads five files pays five file reads end to end;
a turn that fetches three URLs pays three network round trips in series, and
network-bound tools are where the loss is largest.

This document defines how adjacent tool calls run concurrently **without
changing what the run produces**: the same message history, the same hook
sequence, the same approval prompts, in the same order, whatever the
scheduler does.

## 2. Current Baseline

What already works, and which seams assume exactly one tool is in flight.

**The per-call pipeline is one function.** For each call,
`executeToolCalls` unmarshals arguments, looks the tool up, emits
`EventToolStart`, checks the loop guard, and hands off to
`applyPolicyAndExecute`, which resolves the policy, prompts for approval,
runs the `PreToolUse` hook, calls `tool.Execute`, emits `EventToolEnd`, and
runs `PostToolUse`. The result is appended to history and the loop moves on.

**Events already carry a call id.** `Event.ToolCallID` is set on
`EventToolStart`, `EventToolEnd`, and `EventToolDenied`
(`internal/core/agent/event.go:69`), and the trace recorder and Desktop both
key on it. The event stream is therefore already shaped for concurrency; the
CLI TUI is the one consumer that throws the id away.

**State the pipeline touches, and whether it survives concurrency:**

| Component | Shared state | Concurrency-safe today |
|---|---|---|
| `trace.Recorder.Record` | file, buffer, counter | yes — `r.mu` guards every write |
| `HookManager.Run` | matcher cache | yes — documented and mutex-guarded |
| `Read`, `Glob`, `Grep`, `Skill` | none | yes — stateless over a workspace root |
| `WebFetch` | response cache | yes — `cacheMu sync.RWMutex` |
| `Bash` | none (`WithSandbox` returns a copy) | yes for the struct, no for the effects |
| `loopGuard.counts` | `map[string]int` | **no** — unsynchronised map |
| `Session.Append`, `SetNotes`, `SetTodos` | slices on the session | **no** — no mutex |
| `Model.currentToolArgs` (TUI) | one slot for the live call | **no** — single-call assumption |
| `DesktopApprovalHandler.pending` | one response channel | **no** — a second request orphans the first |

The last three are the real work. The first five mean the payload — reads,
searches, and fetches — is already safe to run concurrently.

## 3. Gaps

### 3.1 The loop has no concurrency at all

`executeToolCalls` is a sequential range over `toolCalls`. Nothing in
`internal/core/agent` or `internal/tool` starts a goroutine.

### 3.2 Nothing in the runtime knows which tools write

`llm.Tool` has four methods and no notion of effect. Nor can the classification
be recovered from the policy layer: `ReadFile.CheckArgs` and
`WriteFile.CheckArgs` are byte-for-byte identical — both return `Ask` for a
sensitive path and `Allow` otherwise (`internal/tool/read_file.go:57`,
`internal/tool/write_file.go:51`). That axis is *sensitivity*, not *effect*.

So the runtime cannot tell a file read from a file write, and has no basis on
which to schedule anything but the worst case.

### 3.3 The loop guard and the session assume a single writer

`loopGuard.exceeded` increments a bare map. `Session.Append` and
`Session.SetNotes` mutate slices with no lock, and `NoteWrite`/`TodoWrite`
reach the session from inside `Execute` via `NoteStoreFromContext`.

### 3.4 The TUI pairs a result with the wrong arguments

`eventSinkToChannel` drops `ToolCallID` when building `toolStartMsg` and
`toolEndMsg` (`internal/interface/cli/tui_model.go:151`), and the model keeps
one `currentToolArgs` string that `handleToolEnd` reads back. With two calls
in flight the rendered line pairs the second call's arguments with the first
call's result.

### 3.5 An approval prompt outlives the run it belongs to — fixed ✅

This gap was originally written as a concurrency defect: `RequestApproval`
stores a single `pending` channel, so two concurrent prompts would orphan the
first. That diagnosis was wrong, and implementing phase 1 is what showed it.
Approval stays on the loop goroutine (D1), so the handler never sees concurrent
calls — the concurrency problem is prevented by the architecture, not present
in it.

The real defect was cancellation, and it needed no concurrency at all.
`ApprovalHandler.RequestApproval` took no context and blocked on a channel. A
user who cancelled a Desktop run while a prompt was up left the run goroutine
waiting for an answer nobody would give; its deferred cleanup never ran, so
`runCancels[projectID]` was never deleted and the project stayed permanently
"a run is already in progress".

Fixed by giving the handler the run's context and having both implementations
select on `ctx.Done()`. The single `pending` slot stays, because nothing
concurrent reaches it.

## 4. Direction

Six invariants. Everything in §5 follows from them.

- **D1 — Decide serially, execute concurrently.** Argument parsing, the loop
  guard, policy resolution, approval, and the `PreToolUse` gate all run on the
  loop goroutine in call order. Only `tool.Execute` runs on a worker.
- **D2 — Never reorder.** Calls run in the order the model emitted them. Only
  *adjacent* read-only calls form a group; every write is a barrier.
- **D3 — The history is scheduler-independent.** Results are appended in call
  order. A run's message list must be byte-identical whether the concurrency
  limit is 1 or 16.
- **D4 — One result per call, always.** Every `tool_call` in the assistant
  message gets exactly one `role: "tool"` message, including on denial,
  panic, and cancellation. A batch that half-executes still leaves a
  well-formed history for the next LLM call.
- **D5 — Only read-only calls overlap, and only when declared.** A tool
  classifies its own calls; anything undeclared counts as a write and runs
  sequentially, as it does today.
- **D6 — Sequential stays reachable.** A concurrency limit of 1 restores
  today's behaviour exactly, as a support escape hatch.

## 5. In Scope

### 5.1 What this schedules on

The classification is not defined here. `llm.Access` and `llm.AccessDeclarer`
are defined by [tool-permissions.md](./tool-permissions.md) §5.1, where a tool
declares whether a call changes anything:

```go
const (
    AccessWrite Access = iota // zero value: undeclared tools are writes
    AccessReadOnly
)
```

This design consumes that declaration and adds one obligation on top of it:

> **`AccessReadOnly` is necessary for a call to overlap its neighbours. It is
> not sufficient.** The call must also be safe to run from several goroutines
> at once, and that does not follow from being read-only.

`WebFetch` is the case that proves the gap. It is read-only with respect to the
workspace and still mutates an in-process response cache; it is eligible only
because `cacheMu` guards that cache. Remove the mutex and it stays read-only,
stays `Allow` for permission, and stops being schedulable.

So the scheduler's eligibility rule is:

```text
eligible = Access(args) == AccessReadOnly  AND  the tool is goroutine-safe
```

The second half is a contract a tool author accepts by declaring
`AccessReadOnly`, documented in `docs/contribute/architecture/tools.md` and
enforced by `./make test race` rather than by the type system. A tool that
declares nothing is `AccessWrite` and runs alone, which is D5 and which means
no existing tool changes for this feature to land.

The permission layer maps the same classification differently — read-only to
`Allow`, write to `Ask`, with explicit per-tool overrides. The two consumers
legitimately disagree: `TodoWrite` and `NoteWrite` are `Allow` for permission
(they write only the agent's own scratch state) and ineligible for scheduling
(that state has no mutex). One fact, two mappings; see tool-permissions.md
§5.2.

### 5.2 Grouping

Parsing a call — unmarshalling arguments and looking the tool up in the
registry — has no side effects, so it is hoisted ahead of execution for the
whole batch. Grouping then walks the parsed calls once, asking each its
`Access`, and cuts a new group at every call that is not read-only:

```text
[Read a, Read b, Grep c]              -> one group of 3
[Read a, Write b, Read c]             -> [Read a] [Write b] [Read c]
[Read a, Read b, Bash x, Read c]      -> [Read a, Read b] [Bash x] [Read c]
```

A call whose arguments failed to unmarshal, or whose tool is unknown, is its
own group and is never merged with a neighbour.

Groups are processed strictly in order. Within a group the gate runs in call
order, execution overlaps, and the commit runs in call order. A singleton
group is bit-for-bit today's code path — which is what makes D6 cheap.

### 5.3 The rewritten loop — shipped ✅

```go
func executeToolCalls(ctx context.Context, opts RunLoopOpts, toolCalls []llm.ToolCall, guard *loopGuard) (int, error) {
    count := 0
    for _, group := range groupCalls(opts.ToolRegistry, toolCalls, opts.MaxParallelTools) {
        // 1. Gate — loop goroutine, call order: guard, policy, approval, PreToolUse.
        for i := range group {
            gate(ctx, opts, guard, &group[i])
        }
        // 2. Run — workers, bounded, only for calls the gate let through.
        runGroup(ctx, opts, group)
        // 3. Commit — loop goroutine, call order: PostToolUse, then history.
        for i := range group {
            firePostHook(ctx, opts, &group[i])
            if err := opts.History.Append(llm.Message{
                Role: "tool", Content: group[i].result, ToolCallID: group[i].call.ID,
            }); err != nil {
                return count, err
            }
            count++
        }
    }
    return count, nil
}
```

`runGroup` bounds concurrency with a semaphore and recovers from a panicking
tool so one bad call cannot take the run down or strand its siblings:

```go
func runGroup(ctx context.Context, opts RunLoopOpts, group []pendingCall) {
    sem := make(chan struct{}, opts.MaxParallelTools)
    var wg sync.WaitGroup
    for i := range group {
        if group[i].decided { // denied, unknown, or bad arguments
            continue
        }
        wg.Add(1)
        go func(c *pendingCall) {
            defer wg.Done()
            defer func() {
                if r := recover(); r != nil {
                    c.result = fmt.Sprintf("error: tool %q panicked: %v", c.call.Name, r)
                }
            }()
            sem <- struct{}{}
            defer func() { <-sem }()
            c.result = execute(ctx, opts, c) // tool.Execute + EventToolEnd
        }(&group[i])
    }
    wg.Wait()
}
```

Each worker writes only to its own element of `group`, and `wg.Wait()` is the
happens-before edge before the commit loop reads them. No lock is needed on
the results themselves.

### 5.4 What runs where

| Step | Goroutine | Ordering |
|---|---|---|
| Parse arguments, registry lookup | loop | call order |
| `EventToolStart` | loop | call order |
| Loop guard | loop | call order — counts identical to today |
| Policy resolution, approval prompt | loop | call order — one prompt at a time |
| `PreToolUse` hook | loop | call order, before any sibling executes |
| `tool.Execute` | worker | overlapped |
| `EventToolEnd` | worker | completion order |
| `EventToolDenied` | loop | call order — every denial is decided in the gate |
| `PostToolUse` / `PostToolUseFailure` | loop | call order, after the group joins |
| `History.Append` | loop | call order |

Two asymmetries are deliberate.

**`EventToolEnd` fires from the worker, when the tool actually finishes.** The
event stream is a live feed and every event carries `ToolCallID`; holding a
completion until the slowest sibling returns would make the UI lie about what
is still running.

**`PostToolUse` fires at the join, in call order.** Post hooks are advisory —
their decision is discarded — but they are also an audit surface, and a script
that appends to a log is entitled to a deterministic sequence. The cost is
that an advisory hook waits for the group's slowest member. That is the right
trade: an audit hook that reorders under load is a worse defect than one that
fires a few hundred milliseconds late. If a future hook needs completion-time
delivery, that is a new event, not a reordering of this one.

`PreToolUse` running for the whole group before any member executes is a real
change for a hook that inspects the filesystem between calls. It is also
unavoidable — the calls overlap by construction — and it is confined to calls
a tool declared safe, which by §5.1 do not write.

### 5.5 The event contract

`RunLoopOpts.EventSink` is documented as "invoked synchronously from the
RunLoop goroutine". That becomes: *invoked from the loop goroutine or a tool
worker; the runtime serialises calls, so the sink sees one event at a time and
still must not block.*

`RunLoop` wraps the caller's sink once at entry:

```go
func serializedSink(sink func(Event)) func(Event) {
    if sink == nil {
        return nil // nil stays nil: no allocation, no lock, zero overhead
    }
    var mu sync.Mutex
    return func(e Event) {
        mu.Lock()
        defer mu.Unlock()
        sink(e)
    }
}
```

This keeps the guarantee inside the runtime rather than making every consumer
— TUI, Desktop, trace, Portal — solve it separately.

### 5.6 Approval and the two UIs

Approval stays on the loop goroutine (D1), so `ApprovalHandler` never sees
concurrent calls and neither handler needs to become re-entrant. The single
`pending` slot on the Desktop handler is therefore left alone: the problem it
looked like it had was cancellation, not concurrency, and that is fixed by
giving the handler the run's context (§3.5).

Denial does not cancel siblings. A denied call gets its denial string as its
result and the rest of the group runs — the same as today, where a denial does
not stop the following calls in the batch.

The TUI changes in two places: the tool messages carry `CallID`, and
`currentToolArgs string` becomes an ordered `[]activeTool`. Both shipped in
phase 1. How several concurrent tools should *render* is a UX question, not a
correctness one — see §11.

### 5.7 Which tools are eligible

`Access` values are the ones assigned in tool-permissions.md §6.

| Tool | Access | Eligible | Reason |
|---|---|---|---|
| `Read` | read-only | yes | resolves under the workspace root and opens for reading; no shared state |
| `Glob` | read-only | yes | filesystem walk |
| `Grep` | read-only | yes | filesystem walk |
| `Skill` | read-only | yes | map lookup plus `os.ReadFile` |
| `WebFetch` | read-only | yes | writes only its own cache, and `cacheMu` guards it; network-bound, so the largest single win |
| `Write` | write | no | creates or truncates a file |
| `Edit` | write | no | read-modify-write against file contents |
| `Bash` | write | no | arbitrary side effects, not knowable from the command string |
| `TodoWrite` | write | no | mutates `Session`, which has no mutex |
| `NoteWrite` | write | no | mutates `Session`, which has no mutex |
| `Task` | write | no | a nested `RunLoop` writes whatever its subagent writes — §6 |
| MCP gateway calls | write | no | a third-party server promises nothing on either axis |

The two interesting rows are the ones that show why the axis is *effect* and
not *touches the filesystem*:

- `TodoWrite` and `NoteWrite` never write a file, and are still ineligible.
  They mutate the session through `NoteStoreFromContext`, and `Session.SetNotes`
  has no lock (`internal/core/session/session.go:139`). Process state is state.
- `WebFetch` is the mirror image: it writes something on every call, and is
  eligible anyway, because the only thing it writes is a cache it owns and
  guards.

`Bash` is the honest loss. Most agent shell commands are reads — `git status`,
`ls`, `cat`, a test run's first half — and none of them can be classified from
the command string with the confidence this scheduler requires. The
argument-aware signature in §5.1 leaves the door open; §6 keeps it shut for
now.

### 5.8 Configuration

```yaml
agent:
  max_parallel_tools: 4   # 1 disables parallel execution entirely
```

A new `AgentConfig` under `config.Settings` with a `max_parallel_tools`
mapstructure tag, surfaced as `RunLoopOpts.MaxParallelTools`. Zero means
unset and resolves to the default of 4; values are clamped to `[1, 16]`.

Four, not "unbounded": the ceiling exists to protect file descriptors, memory
for large reads, and — for `WebFetch` and future MCP calls — remote rate
limits. Real batches from the model are two to five calls, so 4 captures
nearly all of the available win while keeping the failure modes boring.

### 5.9 Cancellation and failure

- **Cancelled context.** Workers share the run context. In-flight tools return
  their own context error, which becomes that call's result string. The commit
  loop still appends one result per call (D4), so the history stays
  well-formed and `RunLoop`'s existing cancellation path returns the partial
  reply as it does today.
- **A tool returning an error.** Unchanged: the error string is the result and
  `PostToolUseFailure` fires at the commit.
- **A tool panicking.** Recovered per worker into an error result. Today a
  panic in `Execute` takes down the process; this is a small improvement that
  falls out of needing the group to join.
- **`History.Append` failing.** Returns immediately with the count so far, as
  today. Calls already committed keep their results.

## 6. Out Of Scope

- **Parallel `Task` subagents.** The highest-value follow-up and the largest
  one: a nested `RunLoop` brings its own event stream, approval prompts, and
  session state, and needs an attribution model in the trace before several
  can run at once. Deliberately deferred to its own phase.
- **Parallel `Bash` or `Edit` behind file-level locking.** Predicting the file
  set of a shell command is not something the runtime can do honestly.
- **Argument-aware `Bash` classification** (returning `AccessReadOnly` for a
  `git status`-shaped command). The signature in §5.1 permits it; nothing
  implements it, because a command classifier that is wrong once is worse than
  one that never existed.
- **Overlapping the next LLM call with tool execution.** Speculative
  execution is a different design with a different risk profile.
- **Cross-iteration parallelism.** One assistant message is the unit.

## 7. Runtime Flow

```text
assistant message: [Read a] [Read b] [Write c] [Grep d]
                          |
                   parse + group
                          |
        +-----------------+--------------+-----------+
        |                                |           |
   group 1 (parallel)              group 2 (solo)  group 3 (solo)
   [Read a, Read b]                [Write c]       [Grep d]
        |                                |           |
   gate a, gate b       (loop, in order) gate c      gate d
        |                                |           |
   exec a || exec b     (workers)        exec c      exec d
        |                                |           |
   join                                  |           |
        |                                |           |
   post+append a, then b (loop, in order) post+append post+append
```

History after the batch: result a, result b, result c, result d — always, at
any concurrency limit.

## 8. Implementation Steps

### Phase 1 — make the seams safe (no behaviour change) — shipped ✅

- `RunLoop` wraps `EventSink` with `serializedSink`; the `RunLoopOpts` comment
  now states that a sink may be called from a worker and must pair tool events
  by `ToolCallID` rather than by arrival.
- TUI: `CallID` on `toolStartMsg`/`toolEndMsg`/`toolDeniedMsg`, and
  `currentToolArgs` replaced by an ordered `[]activeTool`. A slice rather than a
  map so the live view does not reorder between frames; matching falls back to
  the oldest call of the same name when an id is absent, so no surface can leak
  a spinner.
- Approval takes the run's context and returns on cancellation (§3.5). The
  Desktop `pending` slot is **not** keyed by call id — see §3.5 for why that
  was the wrong fix for the right bug.

### Phase 2 — split the loop (no behaviour change) — shipped ✅

- `executeToolCalls` is parse → group → gate → run → commit. `runGroup` is
  still sequential; the grouping is live but `MaxParallelTools` is 0 on every
  surface, which keeps each call in its own group.
- Every existing test in `internal/core/agent` passed unmodified. That was the
  point of the phase: if the refactor were wrong, it would be wrong here, with
  no concurrency in the picture.
- Groups are windows into the batch (`calls[start:end]`), not copies, so the
  stages mutate the real elements and the commit stage sees what the run stage
  wrote. A test pins that, because a `copy` slipped in later would break the
  results silently rather than loudly.
- `TestHistoryIsSchedulerIndependent` compares the full message list at limit 1
  and limit 8. It passes today because grouping alone does not reorder; it is
  written now so phase 3 cannot land without keeping it true.

### Phase 3 — the declaration and the workers — shipped ✅

- `runGroup` runs a group's calls on bounded workers, recovering from a
  panicking tool so it cannot take the run down or strand its siblings.
- The gate moved with it. `applyPolicyAndExecute` is gone: policy resolution,
  the approval prompt, and `PreToolUse` are now part of `gateCall` on the loop
  goroutine (D1), execution is `executeCall` on a worker, and the post hooks
  fire at the join. That was not framed as phase 3 work, but §5.4 requires it
  and adding goroutines without it would have put approval prompts on workers.
- `agent.max_parallel_tools` in `settings.yaml`, resolved through
  `config.ResolveMaxParallelTools`, default 4, clamped to `[1, 16]`. A load test
  guards the mapstructure tag: a wrong tag yields zero, which resolves to the
  default, so the setting would appear to work while doing nothing.
- `runGroup` still runs inline for a single-call group, which is every group at
  the default until a batch of reads arrives. No goroutine for the common case.

### Phase 4 — documentation — shipped ✅

- `contribute/architecture/agent-loop.md`: a Tool Execution section with the
  stage table, what overlaps and what does not, and the two deliberate
  asymmetries.
- `contribute/architecture/tools.md`: a Concurrency section stating the
  obligation `AccessReadOnly` carries, with `WebFetch` as the case that shows
  read-only and concurrency-safe are different properties.
- `reference/configuration.md` and `guide/tools.md`:
  `agent.max_parallel_tools`, and one paragraph for users on what overlaps.
- `design/hook-system.md`: `PreToolUse` fires for a whole group before any
  member executes; post hooks fire at the join, still in call order.
- `CHANGELOG.md` under `## [Unreleased]`.

## 9. Acceptance — met ✅

Every condition below has a test. Names are in `internal/core/agent`.

| Condition | Test |
|---|---|
| Byte-identical history at limits 1, 2, and 8 | `TestHistoryIsSchedulerIndependent` |
| `./make test race` clean with an 8-wide read-only group | the package suite under `-race` |
| Three 100ms calls finish in under 250ms; under limit 1 they do not | `TestParallel_ReadsOverlap`, `TestParallel_SequentialWhenLimitIsOne` |
| A write never overlaps a neighbour, by entry/exit timestamps | `TestParallel_WriteIsABarrier` |
| Loop-guard results identical at limit 1 and limit 8 | `TestParallel_LoopGuardIsSchedulerIndependent` |
| An `Ask` inside a group prompts once per call, in call order | `TestParallel_AskInsideAGroupPromptsOnceInOrder` |
| A denial leaves its siblings running | `TestParallel_DenialLeavesSiblingsRunning` |
| Cancelling mid-group still commits one result per call | `TestParallel_CancellationStillCommitsEveryCall` |
| A panicking tool neither kills the run nor strands siblings | `TestParallel_PanicIsContained` |
| Grouping never reorders, and groups are windows not copies | `TestGroupCalls_NeverReorders`, `TestGroupCalls_GroupsAreWindows` |

## 10. Sequencing

`tool-permissions.md` ships first, and not only because it owns `Access`.

Its phase 1 introduces the surface concept and touches the same five
`RunLoopOpts` call sites this design does. Landing them in the other order
would mean threading `Access` through the loop for a scheduler, then
re-deriving what it means for a user prompt — with a permission model chosen
after the fact to fit a classification built for goroutines. The user-visible
decision should shape the classification; the scheduler should take it as
given.

The practical prerequisite is narrow: tool-permissions.md phases 1 and 3 —
`Access` declared on every builtin, including the MCP path. Its phases 2, 4,
and 5 (session grants, configuration, documentation) are independent and can
run in parallel with this work.

Phase 1 of *this* design — the event-sink serialization, the TUI call-id fix,
and the Desktop approval keying — has no dependency at all and can land at any
time. It is worth landing early regardless: §3.5 is a live deadlock, and
tool-permissions.md phase 2 adds a third outcome to the same approval prompt.

## 11. Open Questions

- **TUI rendering.** With four tools live, does the TUI show a list of active
  lines, a count, or only the most recent? Correctness is settled in §5.6;
  the display is not. Worth a look at how the transcript reads afterwards,
  since a batch of four reads currently renders as four sequential lines.
- **Should the trace mark groups?** `tool_start`/`tool_end` with call ids and
  timestamps already let a reader reconstruct overlap. An explicit
  `tool_group` record would make it obvious at the cost of a schema bump.
- **MCP.** Concurrency could be declared per server rather than blanket-denied
  once the gateway grows a capability descriptor.
- **`HookManager.Run` reads `m.cfg` without holding `m.mu`,** which `Refresh`
  writes under it. Pre-existing and unrelated to this design, but this is the
  work that will point a race detector at that code.
