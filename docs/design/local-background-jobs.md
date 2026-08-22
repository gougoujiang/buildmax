# Local Background Jobs

> **Audience:** contributors · **Status:** stages 1–3 implemented — background
> `Bash` and `Task` jobs, the `Monitor` tool with line events and
> backpressure, typed non-user provenance on delivered events, and serialized
> wake-up (`deliver_result`, per-monitor `react`) on the TUI. Desktop is
> notify-only so far: its drawer shows jobs and lines, but wake-up turns are
> not wired. Stage 4 (durability, spool, supervisor) open

Related: [durable run trace](durable-run-trace.md); [queued
messages](queued-messages.md); [tool permissions](tool-permissions.md); [hook
system](hook-system.md); [sandbox boundaries](sandbox-boundaries.md); [agent
loop architecture](../contribute/architecture/agent-loop.md).

## Problem

BuildMax can perform long local work, but it cannot put that work aside and
keep the conversation responsive:

- `Bash.Execute` blocks on `cmd.Run`, with a 120-second default and a
  ten-minute maximum timeout.
- `Task.Execute` blocks on the subagent's final reply.
- Parallel tool execution overlaps only adjacent read-only calls; `Bash` and
  `Task` are write barriers.
- The TUI and Desktop each allow one top-level run per session or project.
  Additional messages queue behind it ([queued messages](queued-messages.md));
  they do not create independent work.

The missing capability is broader than a `run_in_background` boolean. Once the
launching tool call returns, nothing in the process owns the operation: no
identity, status, bounded output, cancellation, completion delivery, or trace
linkage. A monitor adds one more requirement — it emits events while it stays
alive and lets the Agent react to them.

## Scope

This design covers the **local surfaces only**: CLI/TUI and Desktop. Portal's
Tier 1 conversation orchestrator and Tier 2 `Task`/`TaskRun` execution are out
of scope and unchanged. A local job is a process-lifetime object; `TaskRun`
remains the durable team execution object. They may later share state names
and `@buildmax/gui` presentation, but never storage or service ownership.

Explicitly out of scope for the first version:

- Requiring a BuildMax Server for local background work.
- Jobs that survive the BuildMax process. Stage 4 below names what that would
  take; accepting stages 1–3 does not imply it.
- Background start from `buildmax -p`, eval, or worker execution. Print mode
  has no host process to own a job; eval and workers get background tools only
  with an explicit unattended lifecycle and policy.
- Cron, recurring prompts, agent teams, peer-to-peer messaging, or nested
  delegation.
- Automatic merging of concurrent edits to one workspace.
- Treating a monitor as a security boundary; it observes whatever its command
  can reach under normal tool and sandbox policy.

## Decision

TUI and Desktop share **one local job manager owned by `AgentApp`**. The
rejected alternatives:

| Option | Why not |
|---|---|
| Shell `&` / `nohup` | No identity, output capture, process-tree stop, trace, ownership, or cleanup |
| Independent TUI and Desktop implementations | Duplicates process and safety semantics against the shared-runtime rule |
| Reuse Portal `TaskRun` locally | Pulls server and database concepts into direct local mode; still no line-driven monitor events |

### Alternatives by axis

Below the product-level choice, the implementation space decomposes into four
independent axes. The design is one point on each; the serious alternatives
and their verdicts:

| Axis | Chosen | Strongest alternative | Verdict |
|---|---|---|---|
| Manager topology | In-process manager owned by `AgentApp` | Local daemon behind a unix socket (the docker / tmux-server shape), which gives cross-UI job survival for free | Deferred, not rejected — it is the stage 4 supervisor. The manager contract stays transport-agnostic so the move needs no rewrite above it |
| Event delivery | Session inbox with serialized wake-up, layered over queryable job tools | A run loop that never exits, blocking on an event channel (actor model) | Rejected: it rewrites the `RunLoop` contract, run-scoped traces, and compaction timing for elegance the inbox already buys. Hook-based delivery is an observation point, never the primary channel |
| Process adapter | `exec` with process groups / Job Objects | PTY hosting (ConPTY on Windows), so line-buffered programs emit output in real time | Deferred as a stated limitation of the first version; add a PTY adapter when a real case forces it |
| Monitor form | One generic command whose stdout lines are events | Typed native watchers (file watch, HTTP poll), bounded by construction | Later sugar, not a rival: if most monitors watch one file, add a typed kind on the same job and event pipeline. Snapshot-and-diff sampling survives as one coalescing strategy |

Also considered on the topology axis and rejected: orphan processes with
pidfiles (PID reuse, no live event stream — though its disk format is exactly
the stage 4 spool), and delegating to tmux or OS service managers (an external
dependency against the single-binary rule, and three platforms with three
semantics).

Terminology: this design uses **job** for the local runtime object so it does
not collide with Portal's `Task` entity or the `Task` subagent tool.

| Term | Meaning |
|---|---|
| Background job | A process-scoped unit owned by one `AgentApp`, workspace, and local session |
| Command job | Runs one shell command and publishes a terminal outcome |
| Subagent job | Runs one isolated subagent context and publishes its final reply |
| Monitor job | Runs a command whose stdout lines become events until exit, timeout, stop, or shutdown |
| Job event | A bounded lifecycle, output, completion, or monitor record with explicit provenance |
| Agent wake-up | A new serialized top-level turn caused by queued job events, never a concurrent write to session history |

Jobs are **process-scoped but session-owned**: a job may continue when the
user switches sessions inside the same process, but its events and result
belong to the session that started it. Closing the owning `AgentApp` or the
application stops it.

## Runtime Model

### Manager and ownership

`AgentApp` owns one job manager for its workspace — Desktop already caches one
`AgentApp` per project, and the TUI owns one per process — giving the manager
a lifecycle that outlives any tool call without a new global process. The
manager's operations are start, list, get, cursor-based output read, stop,
per-session subscribe, and close.

That contract is deliberately transport-agnostic: no request contexts, shared
objects, or callback closures cross it, and subscriptions are streams. The
constraint costs nothing now and keeps the stage 4 option open of moving the
manager behind a local socket without rewriting `internal/tool` or either
surface.

Package ownership follows the existing boundaries: `internal/agentapp`
coordinates jobs and sessions; process creation and process-tree termination
are platform code in `internal/infra`; LLM-facing argument validation and tool
output stay in `internal/tool`; `internal/core/agent` imports none of them.

Each job captures immutable provenance at start: job ID and kind; workspace
and session ID; parent trace ID and parent tool-call ID when launched by a
tool; model and subagent type for subagent jobs; the resolved sandbox and
permission facts; timestamps; terminal status and a bounded error summary.
The sandbox boundary is captured at launch so a settings reload cannot
silently change a running job's boundary.

### State machine

```text
starting → running → succeeded
                   → failed
                   → canceled
```

`canceled` means BuildMax accepted a stop request and the operation ended, and
carries a `stop_reason` (`user_stop`, `shutdown`, or `timeout`). A stopped
persistent monitor is `canceled` with its reason, not a fourth terminal state:
the state machine stays minimal and the motive lives in a field. Returning
from the launching tool call is successful detachment, not cancellation.

## Command Jobs

The LLM-facing shape is an optional `run_in_background` field on the existing
`Bash` tool. `Bash` remains the single authority for argument validation, risk
classification, permission, sandbox wrapping, and environment scrubbing;
backgrounding must not construct a second, weaker shell path. The permission
gate runs synchronously before detachment; once allowed, the tool returns the
job identity and initial state instead of command output.

The process adapter must capture stdout and stderr without blocking the child,
retain bounded recent output behind a cursor, distinguish exit code, timeout,
stop request, and spawn failure, terminate the **process tree**, run under the
sandbox and scrubbed environment resolved for the approved call, and stop
every owned child at manager shutdown.

Process-tree termination is new work, not a reuse: today `exec.CommandContext`
signals only the direct child, so killing `bash` while its spawned server
survives is the current behavior. Unix process groups and the Windows
equivalent need explicit tests; a stop that leaves a grandchild running is a
failed stop even if the registry says `canceled`.

The first version runs children on pipes, not a PTY. A program that switches
to block buffering away from a TTY — many dev servers — may emit output late
or only at exit; the monitor example's `--line-buffered` exists for the same
reason. This is a stated limitation, and a PTY adapter (ConPTY on Windows) is
the known fix when a real case forces it.

## Subagent Jobs

The same optional field on the existing `Task` tool. A background subagent
receives a fresh context and returns immediately with a job identity. The
existing recursion restriction (no `Task` inside a subagent) is retained.

Approval needs no new mechanism: subagents already run with no approval
handler, and the loop resolves an `Ask` under a nil handler as a denial. A
background subagent inherits exactly that — an operation that would ask is
denied with a useful job result the user can inspect before retrying in the
foreground. What is new is only the framing: the denial reaches the user as a
job outcome instead of a foreground tool result.

A background subagent shares the workspace, as foreground subagents do now.
That is a correctness risk, not a detail: two writers can race, and the
parent's view can go stale. The first version **states the shared-workspace
boundary in tool output and UI and recommends rather than requires
isolation**; requiring worktree isolation would gate the capability on
infrastructure it does not need for read-mostly delegation. Worktree-isolated
writers are the stage 4 follow-up.

## Monitor Jobs

A monitor is a specialized command job, not a second scheduler. Its command is
expected to stay quiet until something worth delivering happens; each stdout
line becomes one bounded `monitor_event`, process exit becomes a normal
lifecycle event, and stderr is diagnostic job output that never wakes the
Agent. Monitors pass the same risk checks, permission policy, sandbox, and
environment rules as `Bash`. A `persistent` monitor means application-session
lifetime, not survival past process exit.

Backpressure is applied before anything reaches the model: a per-line byte cap
with UTF-8-safe truncation, a bounded per-job queue, an event-rate limit,
coalescing with a dropped-event summary when the consumer falls behind, and
redaction before durable trace bytes. Without these, `tail -F` on a busy log
is a context-exhaustion tool.

**Reaction policy is decided per monitor at launch.** The default is
`notify`: events reach the UI and the session inbox but do not call the model.
The launching Agent may declare `react`, in which case queued events wake the
owning session under the serialization rules below. A hidden automatic model
call is a cost and a side effect, so it is never the silent default; a monitor
that never causes reasoning is still useful as a watch.

## Event Delivery And Wake-Up

Delivery is layered, pull first. The job list/get/output tools are the whole
of stage 1 delivery — the model reads a result when it cares — and they remain
permanently as the fallback and debugging channel. The inbox wake-up below is
the stage 3 increment on top of them, never a replacement.

### Typed provenance, not queued user input

`RunLoopOpts.PendingInput` is specifically user-authored input: it runs the
`UserPromptSubmit` hook, appends a user message, and emits `EventUserInput`.
Job events must not travel through it — the trace would claim the user said
something they did not, and the wrong hook contract would apply. Delivery uses
a typed source distinguishing at least:

```text
user_prompt | command_result | subagent_result | monitor_event
```

Model providers share no portable mid-history event role, so an event may be
represented on the wire as a user-role message — but the persisted message and
the trace record carry the typed non-user provenance, the envelope frames the
payload as untrusted observation rather than instruction, and **compaction
must preserve the provenance**, or one compaction turns a monitor line back
into something the user said.

### One session writer, enforced

A job event must never run a turn concurrently against the same session.
Today that guarantee is only a surface convention: `Session.Append` and the
session manager have no locking, and serialization exists because each surface
allows one run. A job manager multiplies producers, so the first version adds
a **session turn coordinator** that makes the single-writer rule a refused or
queued call rather than a data race. The delivery sequence:

1. Record the event and notify the UI immediately.
2. If a top-level run is active, queue or coalesce the event.
3. Deliver queued events at a valid iteration boundary when supported, or
   start a new internal turn after the active run finishes.
4. If the session is idle, optionally start one internal turn immediately.
5. Persist the resulting history through the existing session manager path.

### When completion wakes the model

Every lifecycle event reaches the UI; not every event wakes the model.
**Completion wakes the owning session only when the launching call requested
result delivery.** A monitor wakes according to its declared mode. Progress
output updates the activity UI only. A job stopped during shutdown is recorded
and shown on the next in-process view without waking anything.

## Permission, Trust, And Safety

Backgrounding changes when a call finishes, not whether it is allowed.

- Command and monitor launches pass the normal argument-risk, permission,
  hook, sandbox, and environment resolution before the manager starts them; a
  pre-tool hook can refuse the launch.
- `PostToolUse` observes **job accepted**, because that is the tool result
  that actually returned. Job completion is a new lifecycle notification, not
  a delayed `PostToolUse` for a call already answered. This extends the hook
  contract, so [hook-system.md](hook-system.md) and the hooks guide change in
  the same slice that ships the events — hook authors must not be left
  inferring the semantics.
- Job output, monitor lines, and subagent replies are untrusted input; the
  event envelope tells the model to analyze, not obey.
- A monitor launch does not pre-authorize its reactions. Each resulting tool
  call passes normal policy, with interactive approval when a foreground
  surface is attached; with nobody attached, a reaction may analyze and notify
  but never reinterprets `Ask` as `Allow`.

## Output, Retention, And Traces

Raw job output does not belong in conversation history. Each job keeps a
bounded ring of recent output with cursor-based incremental reads; whether the
ring is memory-only or spooled beneath `BUILDMAX_HOME` is open, and either way
the contract states what survives shutdown (in the first version: nothing).

A parent run trace cannot stay open after the launching call returns, so
background work gets its own job-event stream linked by owning session ID,
parent trace ID, parent tool-call ID, and job ID and kind. It records at
minimum start, boundary, stop request, exit, error, output truncation, dropped
monitor events, and any wake-up caused. Subagent jobs keep their existing
child-run trace. Trace failure stays fail-open and output is bounded and
redacted before persistence, per the [durable trace
design](durable-run-trace.md).

The tool-call linkage is a prerequisite, not an existing seam: subagent traces
carry the parent **run** ID today, but the launching tool-call ID reaches
neither the trace-run context nor a tool's `Execute`. That plumbing lands
before stage 2.

## Lifecycle And Limits

The manager owns a root context independent of any launching call. It does not
use `context.WithoutCancel` on a request context — that drags arbitrary values
along and obscures identity; launch copies explicit provenance into a
manager-owned context.

Shutdown is bounded: stop accepting jobs, request cancellation everywhere,
wait a short grace period, terminate remaining process trees, and record which
jobs did not exit cleanly. The manager enforces conservative limits — maximum
concurrent command jobs, subagent jobs, and monitors, and bounded queued
starts — refusing clearly rather than silently exceeding them, until race and
resource evidence supports larger defaults.

## Delivery Stages

Stage 1 is the committed first slice: command jobs alone are useful with no
model wake-up at all, and they force the heaviest foundations — the manager,
state machine, process trees, and shutdown — that later kinds reuse cheaply.

1. **Local job foundation.** Shared manager, job identity, state machine,
   bounded output, list/get/stop, shutdown, process-tree tests; background
   `Bash` on TUI and Desktop; UI lifecycle notifications and an activity view
   (TUI `/tasks` or similar, Desktop project activity surface); completion
   visible and queryable, no wake-up.
2. **Background subagents.** Detach `Task` behind the manager; plumb the
   parent tool-call ID into trace linkage; make the final reply queryable
   through the job tools; state the shared-workspace conflict in tool output
   and UI. Push delivery of the reply is stage 3 work: it needs the typed
   inbox, and shipping it here would mean inventing that machinery twice.
3. **Monitors and reactive delivery.** Monitor command kind, line framing,
   rate limits, coalescing, stop; typed event provenance in history and
   traces; serialized wake-up with per-monitor `notify`/`react`; TUI/Desktop
   controls for noisy or paused monitors.
4. **Durability and automation, only with evidence.** Output spool and job
   metadata recovery; a local supervisor if jobs must survive UI exit; Desktop
   system notifications; worktree-isolated writers; scheduling on the same
   inbox and job model. Within this stage the spool precedes the supervisor:
   durable output and metadata are useful alone after a crash, and the
   supervisor — the deferred local-daemon topology from [Alternatives by
   axis](#alternatives-by-axis) — builds on them. Process-surviving execution
   is a product and security boundary of its own, not implied by stages 1–3.

Shared presentation may live in `@buildmax/gui`; lifecycle and process
behavior must not.

## Prerequisites

Independently reviewable work that stages above depend on:

1. **Session write serialization** — the turn coordinator that makes the
   one-writer rule enforced (stage 1, before any event delivery).
2. **Process-tree termination in `internal/infra`** — with macOS, Linux, and
   Windows tests including a shell that spawns a child server (stage 1).
3. **Parent tool-call ID plumbing** into trace-run context and subagent run
   options (stage 2).
4. **Hook contract extension** for job lifecycle events, updated in
   [hook-system.md](hook-system.md) alongside the implementation (stages 1–3).

## Open Questions

- Exact tool and event names. Tool names are public contract in hook matchers
  and subagent `tools:` fields; choose once, update
  `internal/tool/names.go` and the docs the enforcement tests check.
- Job ID format: a documented prefix if jobs become persisted entities, or an
  internal UUID like sessions while they remain transient.
- Memory-only output ring versus a `BUILDMAX_HOME` spool in the first version.
- Safe default concurrency and event-rate limits on a laptop.
- Whether switching away from the owning session parks `react` deliveries
  until the owner returns. Today the producer keeps running and a delivery
  for a session not on screen degrades to a notification with the result
  still queryable; parking and replaying reactions is the refinement.
- Which hooks observe job lifecycle beyond the launch gate.
- Plugin-declared monitors, and their trust level, after the interactive tool
  exists.

## Evidence Before Widening

- A prototype showing a long command running while TUI and Desktop each
  complete another prompt, then show and stop the same job.
- Race tests proving job completion cannot append concurrently to a session or
  overlap approval prompts.
- A noisy-monitor test proving bounded memory, bounded context injection,
  dropped-event accounting, and UI responsiveness.
- Trace inspection following a launch from parent tool call to job, subagent
  run, monitor event, and resulting turn.
- Permission tests proving backgrounding never widens an allowed command and a
  detached subagent cannot wait forever on an approval nobody can answer.
- User evidence that people keep doing foreground work while jobs run, rather
  than treating the activity view as a second terminal.
