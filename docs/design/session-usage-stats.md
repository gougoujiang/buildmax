# Session Usage Stats

> **Audience:** contributors · **Status:** partly implemented — per-session
> statistics on the CLI and in the TUI, and the metering fixes they required,
> are shipped; cross-session aggregation is designed and not built
>
> Related: [durable run trace](durable-run-trace.md),
> [prompt cache control](prompt-cache-control.md) §6,
> [local session storage](local-session-storage.md),
> [managed LLM gateway](llm-gateway.md),
> [context durability](context-durability.md),
> [Session architecture](../contribute/architecture/session.md),
> [Agent Loop](../contribute/architecture/agent-loop.md),
> [sessions and traces](../guide/sessions-and-traces.md), and
> [reference/cli.md](../reference/cli.md).

## 1. Problem

BuildMax measured almost everything worth knowing about what a run cost and
what it did, in records that never met. A `-p` footer showed a fraction of it;
Portal showed a per-task-run trace summary and call ledger. Nothing answered
the questions a user asks after a week of use: where the money went, which tool
is eating the context, whether prompt caching is paying for itself, how much of
the wall clock was the model and how much was local tooling.

Two of those records were also wrong, in the same direction. See §3.

## 2. Decision

A **stats** view: a read-only fold over what sessions and traces already
record, per session, on the local surfaces — `buildmax stats` and a TUI
`/stats` overlay.

Stats is a reader. It introduces no durable state that is not a rebuildable
projection and does not change how a run executes. Where it needed a number
that was not being recorded, the fix went into the recording — not into a
heuristic in the reader.

Three sources, with different contracts, kept apart rather than blended:

- The **session file** is authoritative for tokens and cost. They accumulated
  turn by turn at the rates in force for each; nothing recomputes them on read,
  because a total derived later from whatever is configured now would restate
  turns already paid for at a different price. It is also the only source for
  per-tool result bytes, derived by joining `tool_call_id` back to the
  assistant message that issued the call.
- The **run traces** are authoritative for everything time-shaped, and for the
  per-run detail the session file never kept: durations, denials, failure
  kinds, which model ran, how much a delegation did.
- The **managed call ledger** (`llm_call`) is the only source that sees every
  HTTP call a surface made. It stays Portal's, and team-level analytics over it
  is out of scope here — a different question, a different authorization
  boundary, and §6 of [prompt cache control](prompt-cache-control.md) already
  sketches the spend view it belongs to.

They are not merged into one number because they can legitimately disagree: a
run killed before it wrote `run_end` is in the session's totals and missing
from the trace fold, and a reader shown one blended figure would have no way to
notice.

## 3. Metering Fixes This Required

Building the view surfaced two holes in the session's own totals. Both were
fixed before the view shipped, because publishing a number the project's own
honesty rules say is wrong would have been worse than publishing nothing.

1. **Subagent runs were not counted.** The subagent runner called
   `agent.RunLoop` and discarded the returned `RunStats`, so a parent session's
   tokens and cost excluded every `Task` delegation. A child now reports its
   totals to a `DelegatedUsage` accumulator the parent's loop installs on the
   context; the loop drains it after each iteration, so every exit path —
   reply, cancellation, error, iteration cap — reports the same totals. It
   travels on the context rather than through the tool interface because a
   delegation reaches the loop as an ordinary tool call, and widening
   `llm.Tool.Execute` so one tool could report spend would put accounting into
   every tool in the registry.

   Run totals became **inclusive**, and `RunStats.Delegated` is the breakdown —
   the same relationship the cache counts have to the prompt count. Tool calls
   are the exception and are additional: a delegation is one call of the
   parent, and the child's calls are counted nowhere else.

2. **Compaction calls were not counted anywhere.** `LLMCompactor.Compact`
   called the client directly, outside the loop's event stream, so there was no
   `RunStats` contribution and no trace record — only a `context_compacted`
   saying it happened. `ContextCompactor.Compact` now returns `llm.Usage`, the
   loop prices it like any other call, and the cost rides on the existing
   `context_compacted` record rather than a synthetic `llm_end`: compaction is
   a call the run paid for, but it is not a turn, and reporting it as one would
   put a reply in the trace the conversation never contained.

Two related defects were fixed alongside them:

3. **Subagent traces were unreachable.** The runner then created an ephemeral
   session and discarded it on return, and the trace was filed under that id —
   producing directories no session could ever name. A subagent's trace is now
   filed under the parent's session, told apart by `is_subagent` and linked by
   `parent_run_id`. A subagent keeps its own bundle since
   [local session storage](local-session-storage.md), but it is hidden, so the
   trace still belongs with the parent.

4. **A failed tool call was invisible.** The loop already distinguished failure
   (it fires `PostToolUseFailure`), but flattened the error into `error: …`
   result text before anything durable saw it. `tool_end` now carries
   `error_kind`. It is a classification, not a flag, because the kinds mean
   different things: `invalid_args` is the model misusing a tool,
   `tool_error` is usually the environment, `panic` is a defect here. A call
   whose arguments would not parse now emits `tool_start`/`tool_end` like any
   other — `tool_start` already meant "the model issued this call" rather than
   "execution began", since a denied call emits one too.

   **`error_kind` reports a call that could not complete, never a task that
   completed badly.** A command exiting non-zero is a successful `Bash` call:
   the tool returns `nil` error with a readable message, deliberately. Reading
   this field as a failure rate would flatter exactly the runs going worst, so
   the CLI labels it "calls that could not complete" and says so inline.

Title generation was already folded in, which is why the inconsistency was easy
to miss.

## 4. Where The Code Lives

- `core/session.Stats` folds the stored history. Pure, takes a `*Session`,
  needs no run to be live, so it answers for a session whose traces are gone.
- `infra/trace.SummarizeSession` folds every run a session recorded. It owns
  the `<traces>/<session_id>/<run_id>.jsonl` layout because the recorder does.
- `agentapp.LoadSessionStats` combines them. `agentapp` owns both directories
  and is the only layer that may; `internal/service/*` is server-domain and is
  the wrong home. The architecture test enforces the direction — core may not
  import infra — so the combiner cannot drift downward.
- `interface/cli` renders twice: `buildmax stats [session-id]` with `--json`,
  and the `/stats` TUI overlay. The two layouts are separate on purpose — a
  boxed overlay a few dozen columns wide and a full terminal want different
  shapes — but the data and the caveat list are shared, because what a surface
  is allowed to claim is one answer and a warning that appeared on only one of
  them would be a warning nobody trusted. The panel folds the **live** session
  rather than the file, since a session is persisted after each assistant reply
  and reading it back mid-turn would answer about the previous turn.

## 5. Honesty Rules

Not new; the existing conventions applied to a view whose whole job is to state
numbers.

- An unmeasured quantity is absent, not zero. A provider reporting no cache
  usage has not reported a miss, so no cache line is printed at all.
- A session no model priced says so rather than showing `0.000000`.
- A partial total says it is partial: `cost_incomplete`, and a warning naming
  runs that never wrote an end record.
- A saving is reported only when `Baseline > Total`. A run that only ever wrote
  cache entries paid more than it would have uncached, and the view says that.
- Parent and delegated spend are summed correctly and the split is shown. Never
  one number that quietly means the smaller thing.
- Where summed tool time exceeds the wall clock — parallel tool execution makes
  this normal — the model/tools split is not printed, rather than printing a
  negative model time.

## 6. Open

- **Cross-session aggregation.** "What did this week cost", grouped by
  workspace, model, and day. The intended mechanism is a small projection on
  the row in `sessions/index.json`, written on the metadata write that already
  happens each turn — that file is explicitly rebuildable, so a wrong value is
  repairable rather than a migration. Deferred by choice, not blocked.
- **Per-turn recording.** It would remove the trace dependency and make a
  session self-describing. [Local session storage](local-session-storage.md)
  has since landed, so the place for it now exists: `meta.json` holds the
  running totals, and a per-turn breakdown belongs beside them there rather
  than being folded back out of the journal.
- **Trace retention.** Nothing prunes a session's `traces/` today. Stats is both an
  argument for retention and a victim of it; the view degrades visibly rather
  than silently, which is the most it can do until retention is decided.
- **Cache diagnostics.** When §6 of [prompt cache control](prompt-cache-control.md)
  lands its requested-mode / capability / strategy / outcome fields in the
  trace, the cache section should report the recorded outcome instead of
  inferring one from counts.
- **Desktop.** A binding needs nothing the aggregation does not already
  expose. Not built.
- **Worker runs contribute nothing**, by construction: a worker assembles
  inside a run-scoped `BUILDMAX_HOME`. That is correct — worker spend is the
  ledger's job — and is recorded here so it is stated rather than discovered.
