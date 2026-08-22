# Local Background Work And Monitors

> **Audience:** contributors and early adopters · **Status:** proposal — under discussion

Related: [roadmap](../ROADMAP.md) P0.5 and P1; [agent loop
architecture](../contribute/architecture/agent-loop.md); [CLI
architecture](../contribute/architecture/cli.md); [Desktop
architecture](../contribute/architecture/desktop.md); [session
architecture](../contribute/architecture/session.md); [queued messages
design](../design/queued-messages.md); [parallel tool execution
design](../design/parallel-tool-execution.md); [tool permission
design](../design/tool-permissions.md); and [durable run trace
design](../design/durable-run-trace.md).

## Problem And Current Context

BuildMax can perform long local work, but it cannot put that work aside and
keep the local conversation responsive.

- `Bash.Execute` waits for `cmd.Run` and caps the call at ten minutes.
- `Task.Execute` waits for the subagent's final reply.
- Parallel tool execution overlaps only adjacent read-only calls inside one
  assistant tool-call batch. `Bash` and `Task` are write barriers, and the
  parent run still waits for every call in the batch before its next model
  turn.
- The TUI runs the top-level Agent on a goroutine, but still treats it as the
  one unit of work occupying the conversation.
- Desktop allows at most one top-level run per project. Additional user
  messages join or queue behind that run; they do not create independent work.
- Portal has durable `Task` / `TaskRun`, a scheduler, and Workers. That is a
  team/server execution model, not a local-session primitive: requiring it
  would make the Server a dependency of the local product and would put local
  process lifecycle behind database and worker concepts it does not need.

The missing capability is broader than a `run_in_background` boolean. A useful
local background operation needs an identity, ownership, bounded output,
status, cancellation, completion delivery, permission semantics, trace
correlation, and shutdown behavior. A monitor adds one more requirement: it
does not merely finish later; it emits events while it remains alive and lets
the Agent react to them.

The local surfaces already have useful seams:

- `agentapp.AgentApp` is the shared assembly boundary for CLI, TUI, Desktop,
  eval, and Worker execution.
- `RunLoopOpts.EventSink` carries live structured events to the surfaces and
  the durable trace.
- TUI and Desktop already serialize top-level work and can queue user prompts.
- Subagent traces already link a child run to its immediate parent run.

None of those seams owns an operation after the tool call that launched it has
returned. Background work needs that new owner.

## User Outcomes

A local user should be able to:

1. Start a build, test suite, download, development server, or other command in
   the background and continue the conversation.
2. Delegate independent investigation or implementation to a background
   subagent and receive its final result later.
3. Watch logs, files, CI, deployments, or another long-lived source and have
   meaningful changes delivered to the active session.
4. List running and finished work, inspect recent output, and stop one item.
5. Understand which session and workspace own the work and what permission and
   sandbox boundary it runs under.
6. Quit without leaving an unowned local process behind.

The monitor outcome is specifically event-driven:

```text
background command / subagent       monitor
             │                         │
       terminal outcome          stdout line/event
             │                         │
             └──────────┬──────────────┘
                        ▼
             session-owned event inbox
                        │
              UI notification first
                        │
           optional serialized Agent wake-up
```

## Goals

- Define one local background-work model shared by TUI and Desktop.
- Keep the CLI binary self-contained and preserve local mode with no Server or
  Node runtime.
- Support three initial kinds of work: command, subagent, and monitor.
- Give every job stable process-lifetime identity, status, ownership, output,
  cancellation, and trace provenance.
- Deliver job events without misrepresenting them as user-authored prompts.
- Preserve one writer to a session history and one interactive approval at a
  time.
- Make noisy or malicious process output bounded and visibly untrusted.
- Establish a foundation that later scheduling, worktree isolation, Desktop
  notifications, and restart durability can build on.

## Non-Goals

- Replacing Portal `Task` / `TaskRun`, its scheduler, or Worker execution.
- Requiring a BuildMax Server for local background work.
- Keeping a first version alive after the BuildMax process exits.
- Supporting detached work from `buildmax -p`; print mode has no host process
  in which a local job can remain owned.
- Shipping cron, recurring prompts, or an unattended local daemon in the first
  version.
- Implementing agent teams, peer-to-peer agent messaging, or nested delegation.
- Automatically merging concurrent edits or promising that two writers to one
  workspace cannot conflict.
- Turning every line of arbitrary command output into a model call.
- Treating a monitor as a security boundary. It observes whatever its command
  can reach under the normal tool and sandbox policy.

## Terms And Semantics

This proposal uses **job** for the local runtime object so it does not collide
with Portal's durable Task domain entity or the existing `Task` subagent tool.

| Term | Meaning |
|---|---|
| Background job | A process-scoped unit owned by one `AgentApp`, workspace, and local session |
| Command job | Runs one shell command and publishes a terminal outcome |
| Subagent job | Runs one isolated subagent context and publishes its final reply |
| Monitor job | Runs a command whose stdout lines are event notifications until exit, timeout, stop, or shutdown |
| Job event | A bounded lifecycle, output, completion, or monitor record with explicit provenance |
| Agent wake-up | A new serialized top-level turn caused by queued background events, never a concurrent write to session history |

The minimum state machine is:

```text
starting → running → succeeded
                   → failed
                   → canceled
```

`canceled` means BuildMax accepted a stop request and the operation ended.
Losing the parent tool-call context is not itself cancellation: returning from
the tool call is the definition of successful detachment.

The first version is **process-scoped but session-owned**. A job may continue
when the user switches sessions inside the same process, but its events and
result belong to the session that started it. Closing the owning `AgentApp` or
the application stops it. Restart recovery is a later, separate contract.

## Options And Trade-Offs

| Option | Strength | Main concern |
|---|---|---|
| Tell the model to use shell `&`, `nohup`, or platform equivalents | Almost no implementation work | No reliable identity, output, process-tree stop, trace, ownership, cleanup, or cross-platform behavior |
| Add independent background implementations to TUI and Desktop | Each UI can move quickly | Duplicates process and safety semantics and violates the shared-runtime direction |
| Reuse Portal TaskRun locally | Already has durable states, cancellation, artifacts, and Workers | Introduces server/database concepts into direct local mode and still does not provide line-driven monitor events |
| Add a shared local job manager beneath both surfaces | One lifecycle and safety contract; no Server dependency; extensible to monitors and subagents | Requires a typed event inbox and careful coordination with session persistence |

The likely direction is the final option. Portal TaskRun remains the durable
team execution object; a local job is a smaller process-lifetime object. They
may share state names and presentation components later, but not storage or
service ownership.

## Proposed Runtime Model

### One manager per local `AgentApp`

`AgentApp` should own one local job manager for its workspace. Desktop already
caches one `AgentApp` per project; the TUI owns one for its workspace. This
gives the manager a lifecycle that outlives a tool call without creating a new
global process or server.

The manager needs operations equivalent to:

```go
Start(ctx, Spec, Provenance) (Job, error)
List(Filter) []Job
Get(jobID string) (Job, bool)
Output(jobID string, cursor Cursor) OutputPage
Stop(ctx, jobID string) error
Subscribe(sessionID string) Subscription
Close(ctx context.Context) error
```

This is an illustrative contract, not a committed API. The important boundary
is ownership: `internal/agentapp` coordinates jobs and sessions;
platform-specific process creation and process-tree termination belong in
`internal/infra`; LLM-facing argument validation and tool output remain in
`internal/tool`. `internal/core/agent` must not import any of them.

Each job captures immutable provenance at start:

- local job ID and kind;
- workspace and session ID;
- parent trace ID and parent tool-call ID, when launched by a tool;
- model and subagent type for a subagent job;
- resolved sandbox and permission facts relevant to the launched operation;
- creation, start, and end timestamps;
- terminal status and bounded error summary.

The exact ID format is deliberately undecided. If this becomes a persisted
entity, it needs a documented prefix in the project conventions; a transient
implementation may use an internal UUID like sessions do.

### Commands

The existing `Bash` tool is the authority for command argument validation,
risk classification, permission, workspace, sandbox wrapping, and environment
scrubbing. Background execution must not construct a second, weaker shell path
around it.

A likely LLM-facing shape is an optional `run_in_background` field on `Bash`.
The permission gate runs synchronously before detachment. Once allowed, the
tool returns the job identity and initial state rather than command output.

The process adapter must:

- capture stdout and stderr without blocking the child;
- retain bounded recent output with a cursor for incremental reads;
- distinguish an exit code, timeout, stop request, and spawn failure;
- terminate the process tree, not only the immediate shell;
- use the resolved sandbox and scrubbed environment from the approved call;
- stop every owned child during manager shutdown.

Appending `&` to a command is not an implementation of this contract.

### Subagents

A likely shape is the same optional field on the existing `Task` tool. A
background subagent receives a fresh context and returns immediately to the
parent with a job identity.

The current `Task` tool already prevents recursive `Task` access. The first
background version should retain that restriction. It should also run with no
interactive approval handler: an operation that would ask is denied with a
useful job result, because a prompt arriving later from an invisible subagent
has no unambiguous foreground owner. The user can inspect the refusal and retry
the subagent in the foreground.

A background subagent shares the workspace by default, as foreground
subagents do now. This is a correctness risk, not an implementation detail:
two writers can race or make the parent's view stale. Initial UI and tool
output must state the shared-workspace boundary. Worktree isolation is the
likely follow-up for jobs expected to edit concurrently.

### Monitors

A Monitor is a specialized command job, not a second scheduler. Its command is
expected to remain quiet until an event worth delivering occurs. Each stdout
line becomes one bounded `monitor_event`; process exit becomes a normal job
lifecycle event. Stderr is diagnostic job output and does not automatically
wake the Agent.

A likely Monitor input includes:

```json
{
  "command": "tail -F server.log | grep --line-buffered ERROR",
  "description": "watch server errors",
  "timeout_ms": 3600000,
  "persistent": true
}
```

Monitor uses the same command risk checks, permission policy, sandbox, and
environment rules as Bash. `persistent` means application-session lifetime,
not survival after process exit.

The manager must apply backpressure before events reach the model:

- a per-line byte cap and UTF-8-safe truncation;
- a bounded per-job queue;
- an event-rate limit;
- coalescing or a dropped-event summary when the consumer falls behind;
- no automatic delivery of stderr;
- redaction before durable trace bytes are written.

Without those bounds, `tail -F` on a busy log is a context-exhaustion tool.

## Event Delivery And Agent Wake-Up

### Do not overload queued user input

`RunLoopOpts.PendingInput` is specifically user-authored input. It runs the
`UserPromptSubmit` hook, appends a user message, and emits `EventUserInput`.
Sending monitor output through that seam would make the trace claim the user
said something they did not say and would apply the wrong hook contract.

Background delivery needs a typed source, whether that becomes a sibling
runtime-inbox interface in `core/agent` or an `agentapp` turn coordinator above
the loop. The record must distinguish at least:

```text
user_prompt | command_result | subagent_result | monitor_event
```

The exact wire role remains an implementation question because model APIs do
not share a portable mid-history event role. If an event is represented as a
user-role message for provider compatibility, the persisted message and trace
still need explicit non-user provenance, and the system prompt must frame the
payload as untrusted observation rather than instruction.

### One session writer

A background event must never call `RunPrompt` concurrently against the same
session. TUI and Desktop should use a session-owned inbox and a coordinator:

1. Record the event and notify the UI immediately.
2. If a top-level run is active, queue or coalesce the event.
3. Deliver queued events at a valid iteration boundary when supported, or
   start a new internal turn after the active run finishes.
4. If the session is idle, optionally start one internal turn immediately.
5. Persist the resulting history through the existing `SessionManager` path.

This preserves the assistant-tool-result pairing and the one-writer assumption
of `Session` while still allowing many background producers.

### Notification is not always reasoning

Every lifecycle event should reach the UI. Not every event should wake the
model.

| Event | Default behavior candidate |
|---|---|
| Command succeeded or failed | Notify UI; wake the owning session when the Agent launched it for a result |
| Subagent completed | Notify and deliver the final reply to the owning session |
| Monitor stdout line | Queue a bounded event and wake according to the monitor's policy |
| Progress output | Update activity UI only |
| Job stopped during shutdown | Record and display on the next in-process view; do not wake |

Whether monitors default to `notify`, `react`, or an explicit per-monitor mode
is an open product decision. A hidden automatic model call is a cost and side
effect; a monitor that never causes reasoning is only a log viewer.

## Tool And Surface Shape

The following names are illustrative. Tool names are a public contract in hook
matchers and subagent definitions, so implementation must choose them once and
update the tool-name source of truth and user documentation together.

| Capability | Candidate surface |
|---|---|
| Start background command | `Bash(run_in_background: true)` |
| Start background subagent | `Task(run_in_background: true)` |
| Start event watcher | New `Monitor` tool |
| List jobs | New job-list tool plus TUI/Desktop activity view |
| Inspect output/status | New job-get/output tool plus UI detail |
| Stop | New job-stop tool plus UI action |

TUI should gain one activity panel, likely reached by `/tasks`, showing state,
kind, owner session, age, and a bounded output preview. Desktop should expose
the same information in a project activity surface. Shared presentation may
live in `@buildmax/gui`; lifecycle and process behavior must not.

Print mode should reject background start explicitly. Silently starting a job
and then exiting would either kill it before useful work or orphan it. Eval and
Worker execution should also leave local background tools disabled until they
have an explicit unattended lifecycle and policy.

## Permission, Trust, And Safety

Backgrounding changes when a call finishes, not whether it is allowed.

- Command and Monitor calls pass the normal Bash argument-risk, permission,
  hook, sandbox, and environment resolution before the manager starts them.
- A pre-tool hook can refuse the launch. Post-tool semantics must distinguish
  "job accepted" from "job eventually finished"; the latter needs a new
  lifecycle/notification event rather than a delayed `PostToolUse` for a tool
  call whose result already returned.
- Background subagents receive no asynchronous approval channel in the first
  version. `Ask` resolves as it does on other unattended surfaces.
- Monitor output, command output, file contents, network responses, and
  subagent replies are untrusted inputs. An event envelope must tell the model
  to analyze the payload, not obey instructions found inside it.
- Automatic monitor reactions can cause writes. The original monitor launch
  does not pre-authorize those later actions; each resulting tool call passes
  normal policy and interactive approval when the owning foreground surface
  is attached.
- If no user is attached to answer, a reaction may analyze and notify but must
  not reinterpret `Ask` as `Allow`.

The OS boundary matters more when several jobs share one process and
workspace. The effective sandbox must be captured at launch so a settings
reload cannot silently change a running job's boundary.

## Output, Retention, And Traces

Job output and Agent history answer different questions. Raw build logs do not
belong in the conversation, and a one-line monitor event should not require
retaining an unbounded stream in memory.

The first version should keep a bounded ring of recent output per job and
support cursor-based incremental reads. Whether that ring is memory-only or
spooled beneath `BUILDMAX_HOME` is open; either way the contract must state
what survives shutdown.

A parent run trace cannot remain open indefinitely after the launching tool
returns. Background work therefore needs its own trace or job-event stream,
linked by:

- owning session ID;
- parent trace ID;
- parent tool-call ID;
- job ID and kind.

At minimum it records start, boundary, stop request, exit, error, output
truncation, dropped monitor events, and any Agent wake-up it caused. Subagent
jobs also retain their existing child-run trace. Trace failure remains
fail-open and output is bounded and redacted before persistence, following the
durable trace design.

## Lifecycle And Failure Semantics

The manager owns a root context independent of any one launching tool-call
context. It must not blindly use `context.WithoutCancel` on that context,
because doing so also retains arbitrary values and obscures which identity was
captured. Launch should copy explicit provenance into a manager-owned context.

Shutdown is bounded:

1. Stop accepting jobs.
2. Request cancellation for every running job.
3. Wait for a short grace period.
4. Terminate remaining process trees.
5. Record which jobs did not exit cleanly.

Platform behavior needs explicit tests on macOS, Linux, and Windows. Killing
only `bash`, `sh`, or `cmd.exe` while its child server survives is a failed
stop, even if the registry says `canceled`.

The manager also needs limits: maximum concurrent command jobs, subagent jobs,
and monitors; bounded queued starts; and a clear refusal rather than silently
exceeding the limit. Defaults should be conservative until race and resource
evidence supports larger values.

## Delivery Options

The capability can land in stages without calling an incomplete stage
Monitor.

### Stage 1 — Local job foundation

- Shared manager, job identity, state machine, bounded output, list/get/stop,
  shutdown, and process-tree tests.
- Background Bash in TUI and Desktop.
- UI lifecycle notifications and activity views.
- No model wake-up yet; completion is visible and queryable.

### Stage 2 — Background subagents

- Detach the existing `Task` execution behind the manager.
- Preserve parent trace and tool-call linkage.
- Deliver final replies through the serialized session inbox.
- State the shared-workspace conflict explicitly; optionally prototype
  worktree isolation.

### Stage 3 — Monitor and reactive delivery

- Monitor command kind, line framing, rate limits, coalescing, and stop.
- Typed background-event provenance in history and traces.
- Serialized Agent wake-up and an explicit notify/react policy.
- TUI/Desktop controls for noisy or paused monitors.

### Stage 4 — Durability and automation, only with evidence

- Output spool and job metadata recovery.
- A local supervisor if jobs must survive UI or CLI exit.
- Desktop system notifications.
- Worktree-isolated writers.
- One-shot or recurring scheduling built on the same inbox and job model.

Stage 4 is not implied by accepting stages 1-3. Process-surviving execution is
a product and security boundary of its own.

## Questions To Resolve

- Is the first high-value slice background Bash, background subagents, or a
  deliberately paired command-plus-subagent release?
- Should completion automatically wake the Agent, or only when the launching
  call requested a result delivery mode?
- Should Monitor expose `notify` and `react` modes, or should reaction always
  be decided by the Agent that starts it?
- What message representation preserves non-user provenance across every
  supported model provider and session compaction?
- Should switching away from the owning session pause reactions while leaving
  the producer running?
- What are safe default concurrency and event-rate limits on a laptop?
- Does a stopped persistent monitor become `canceled` or a distinct normal
  `stopped` terminal state?
- Is recent output valuable enough to spool in the first version, even though
  jobs themselves are not restored?
- Should a background writer require worktree isolation, merely recommend it,
  or remain unavailable until isolation exists?
- Which hooks should observe job lifecycle and background-event delivery?
- Are plugin-declared monitors worth supporting after the interactive Monitor
  tool, and what trust level should they have?

## Evidence Needed For A Decision

- A prototype showing that a long test command can run while the TUI and
  Desktop both complete another prompt, then show and stop the same job through
  equivalent surface controls.
- Race tests proving that job completion cannot append concurrently to a
  session or overlap approval prompts.
- Cross-platform process-tree tests, including a shell that launches a child
  server and a forced application shutdown.
- A noisy-monitor test proving bounded memory, bounded context injection,
  dropped-event accounting, and UI responsiveness.
- Trace inspection proving a launch can be followed from parent tool call to
  job, subagent run, monitor event, and resulting Agent turn.
- Permission tests proving backgrounding never widens an allowed command and a
  detached subagent cannot wait forever for an approval nobody can answer.
- User evidence that people continue useful foreground work while jobs run,
  rather than treating the activity view as a second terminal.
- Comparable-product evidence is encouraging but not decisive: Claude Code's
  [Monitor tool](https://code.claude.com/docs/en/tools-reference) demonstrates
  line-driven background events, while its [parallel-agent
  model](https://code.claude.com/docs/en/agents) demonstrates demand for
  observable detached work.

## Likely Destination If Accepted

The accepted priority belongs in [the roadmap](../ROADMAP.md), most likely
across P0.5 local activity/trace work and a reopened local-experience slice.
Durable decisions about lifecycle, event provenance, permissions, and tracing
belong in a focused record under `docs/design/`. User-facing tools and controls
belong in `docs/guide/` and `docs/reference/` only when they ship.

Implementation should be split into independently reviewable issues for the
manager/process boundary, command tools, subagents, typed event delivery,
traces, TUI, Desktop, and cross-platform verification. This proposal should
then be deleted rather than retained as a second roadmap.
