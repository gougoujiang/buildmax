# Portal Interaction And Execution Model

> **Audience:** contributors and product reviewers · **Status:** proposal — under discussion
>
> **Opened:** 2026-08-22

Related: [ROADMAP](../ROADMAP.md) P2 and P0.5, the
[product vision](../design/product-vision.md),
[surface positioning](../design/surface-positioning.md), the
[issue model](../design/issue-model.md),
[context durability](../design/context-durability.md), and the
[data model](../contribute/architecture/data-model.md).

## Proposal Summary

Portal should keep its Tier 1 / Tier 2 model, but redefine the capability
boundary between the tiers. Tier 1 should not remain a thin router limited to
managing Tasks. It should be a complete foreground orchestration Agent. Tier 2
should not be understood as a passive job queue either: it should be able to
evolve from today's general-purpose Agent into a plane of selectable,
purpose-specific execution Agents.

The Agent tiers and the infrastructure that carries them should be described
separately:

1. **Tier 1 — Foreground Orchestrator Agent.** It remains in contact with the
   user and owns interaction, intent recognition, context interpretation,
   bounded foreground work, decomposition, executor selection, dispatch,
   coordination, and result synthesis.
2. **Tier 2 — Execution Agent Plane.** It performs focused work in the
   background. Today it is primarily one general-purpose Agent; over time it
   may include declaratively defined research, coding, data-analysis, or other
   specialized Agents.
3. **Persistent Execution Substrate.** Task, TaskRun, the scheduler, and
   workers carry Tier 2 Agents and persist status, output, artifacts, traces,
   usage, and failures. They are not Agents themselves.
4. **Outcome Projection.** Conversation task cards, Issue results, and
   notifications are derived from durable Task/TaskRun state. Their existence
   does not depend on an active browser connection or a successful additional
   Tier 1 model call.

The candidate decision is:

> Expand Tier 1's Agent capability while narrowing its responsibility for
> persistence, authoritative run state, and reliable delivery. Let Tier 2
> evolve from one general executor into a selectable and composable execution
> Agent plane. Tier 1 keeps the single voice and coordination authority, but
> does not monopolize facts or delivery.

This is not an accepted architecture decision. The purpose of this paper is to
make the direction concrete enough to accept, change, or reject.

## Why Re-Evaluate This Now

The current two-tier design solves a real problem: a Portal conversation must
not remain blocked by an Agent run lasting several minutes. Background work
also needs scheduling, cancellation, retry, quota, artifacts, traces, and
worker isolation. None of those belong inside the lifecycle of one synchronous
HTTP or WebSocket turn.

The problem is not the separation. The problem is that current Tier 1 is
restricted to four Task-management tools while also being made responsible for
delivery work that an Agent cannot reliably guarantee. Its current duties are
mostly:

- decide whether the user is chatting or asking for execution;
- rewrite the user's intent into a Task input;
- acknowledge that a background Task started;
- call a model again when a TaskRun finishes to turn the output into a reply.

Tier 1 should grow into a fuller foreground Agent: it should understand
context, perform bounded lightweight tool work, decompose goals, choose
execution Agents, coordinate multiple executions, and synthesize their
results. At the same time, durable Task state and result visibility must not be
side effects of Tier 1 model behavior or a live socket.

## Current Baseline

### What Already Works

The following parts have independent value and should remain:

- `internal/service/conversation` uses the shared Agent loop, but currently
  exposes only `StartTask`, `ListTasks`, `GetTask`, and `ContinueTask` as Portal
  orchestration tools.
- Portal Agent definitions can already be selected by a Task and appended to a
  worker run's system prompt. This is an early foundation for a Tier 2 Agent
  catalog, although it has no capability, input/output-contract, or execution
  profile metadata yet.
- A Task is a continuable unit of background work, while TaskRun is one attempt
  to execute it.
- A later TaskRun for the same Task restores the previously persisted Agent
  session.
- TaskRun independently records status, output, error, tokens, trace, cancel
  request, and retry lineage.
- The scheduler and worker separate long work from the lifecycle of a Portal
  request.
- A server-scoped queue serializes turns for one Conversation across WebSocket,
  HTTP, multiple tabs, and system turns.
- Issues aggregate TaskRun output, and workflow terminal callbacks advance
  later steps.

### Current Chat Path

```text
user message
  |
  v
Tier 1 conversation LLM
  |-- direct reply
  `-- StartTask(input, agent_id?)
         |
         v
      Task + TaskRun(PENDING)
         |
         v
      scheduler -> worker -> shared Agent runtime
         |
         v
      TaskRun terminal callback
         |
         v
  broadcast the invalidation; submit a Tier 1 system turn
         |
         v
      Tier 1 summarizes Task output
         |
         v
      assistant message
```

### What The Current Model Gets Right

Keeping long execution from blocking a conversation is the right goal. The
following alternatives would be regressions:

- execute every Agent request synchronously inside a Tier 1 turn;
- let a browser connection own the lifetime of a TaskRun;
- remove Task and append worker output directly to a Conversation;
- reimplement retry, cancel, trace, artifacts, and quota as chat-message state.

This proposal therefore does not remove Task/TaskRun and does not merge workers
back into server requests.

## Confirmed Problems

### Result Delivery Is Not Durable

Phase 0 took the terminal callback off the socket. It no longer looks for a
connection belonging to `created_by`, no longer skips when there is none, and no
longer picks the first of several; the invalidation is broadcast to the team and
the Tier 1 turn is submitted to the server's turn queue. What remains is the
durability half:

- the system turn uses an in-memory queue and is dropped when the queue is full;
- a server restart loses turns that have not started;
- no outbox, delivery row, or reconciler repairs a missed result reply.

The documented promise that a result always returns to the originating
Conversation is therefore still not true for the natural-language reply. The
result itself is no longer at risk: TaskRun is durable and the Conversation now
reads its card from the database.

### The Original Intent Is Not Traceably Connected To Execution

On the Chat path, `StartTask.input` is a tool argument generated by the Tier 1
model. Neither Task nor TaskRun records the `conversation_message_id` that
caused it, or the exact messages, attachments, and context references used to
construct the execution request.

That creates three problems:

1. Tier 1 may omit a constraint, scope boundary, or acceptance condition while
   rewriting the request.
2. Diagnostics can inspect the input sent to the worker, but cannot compare it
   with the user's original wording.
3. There is no stable reference point for future attachment, Issue-context, or
   workspace-snapshot support.

Tier 1 may organize an execution plan, but that plan must not become the only
copy of the original intent.

### Task Results Masquerade As User Messages

The system turn appends `[Task Result] ...` to `conversation_message` with
`role=user`. Three separate questions are answered by that one column:

- who produced the content;
- whether it belongs in the visible transcript;
- whether it is trusted instruction that should be replayed to a model.

Phase 0 separated the second: the messages route excludes the system channel, so
a task result is no longer drawn as the user's own message, and the run's card
reports the outcome instead. The first and third are unchanged. The row still
says `user`, so provenance is carried by a channel rather than stated, and the
content is still replayed to the model as user input. The system channel
disables Task tools, which keeps an injected result from starting more work, but
it is not the prompt-injection boundary this needs.

### Successful Work Requires An Unnecessary Additional Model Call

A typical background request includes at least:

1. Tier 1 dispatch;
2. Tier 2 execution;
3. Tier 1 completion presentation.

Title generation may add another call. The third call can improve wording, but
must not determine whether the result is visible. A timeout, exhausted quota,
or missing Conversation model must not hide a completed TaskRun.

### Conversation Does Not Expose Durable Execution State

The current Conversation UI renders user/assistant text, a streaming assistant
reply, and queued user messages. It does not read or render the Conversation's
Tasks, TaskRuns, artifacts, cancellation, retry, or trace state.

This disagrees with the P2 statement in `ROADMAP.md` that
"conversation-visible result cards and artifact links" are complete. Current
code is the fact; until the UI exists, that roadmap statement is a documentation
gap.

### Conversation Is The Mandatory Parent Of Every Execution

Task currently requires a non-null `conversation_id`. An Issue Agent run and a
Workflow run must create a synthetic Conversation before they can create a
Task, even when no chat initiated the work.

As a result, `conversation_id` simultaneously means:

- a user interaction thread;
- Task ownership;
- part of an artifact/object-key path;
- worker-session location;
- a result delivery target.

Those are not one concept. Team is the authorization and ownership boundary;
Conversation is at most one origin or delivery target.

### Conversation And Issue Do Not Yet Form A Coherent Product Loop

Current documents say all of the following:

- Conversation is the front door through which a user talks to the system;
- Issue is the primary user-facing work object;
- Task/TaskRun are low-level execution records users rarely inspect directly.

These can all be true, but their transitions must be explicit. Chat can create
a Task without projecting durable work state back into Chat. Issue can display
results without establishing a clear relationship to the Conversation the
user is currently using.

## Goals And Non-Goals

### Goals

- Make Tier 1 a complete foreground orchestration Agent rather than a Task
  manager limited to `StartTask` and `ContinueTask`.
- Let Tier 1 answer directly, use lightweight tools within a foreground budget,
  and decompose, dispatch, monitor, and synthesize multiple execution goals.
- Let Tier 2 become an extensible execution Agent plane in which today's
  general Agent and future specialized Agents share one Agent Core.
- Show a durable task card immediately after the user starts executable work,
  instead of relying on a transient acknowledgement.
- Preserve final TaskRun status and results across logout, refresh, tab changes,
  disconnect, and server restart.
- Make successful results visible without another LLM call.
- Keep the original message, Tier 1's normalized instructions, and the actual
  worker input separately traceable.
- Let Conversation, Issue, Workflow, and webhook use the same execution plane
  without manufacturing meaningless objects.
- Continue using the shared Agent Core rather than introducing a Portal-only
  execution runtime.
- Migrate without invalidating existing Task, TaskRun, artifact, trace, cancel,
  and retry data.
- Distinguish user content, orchestration decisions, worker output, and
  presentation text by trust level.

### Non-Goals

- Remove background workers or make all work synchronous.
- Automatically create an Issue for every Chat request.
- Replace Issue, Workflow, or Task with Conversation.
- Solve versioned workspace, approvals, worker sandbox hardening, or a complete
  notification system in this proposal.
- Introduce a general event-sourcing platform.
- Change the local session model used by CLI/TUI or Desktop.
- Require users to understand Task IDs, TaskRun IDs, workers, or the scheduler.
- Hard-code `ResearchAgent`, `CodingAgent`, or similar Go types before usage
  evidence shows that those categories are stable.

## Proposed Conceptual Model

### Tier 1: Foreground Orchestrator Agent

Tier 1 is a complete Agent that remains in contact with the user. It should
have enough context and tools to:

- interact naturally, recognize intent, and clarify requirements;
- understand Conversation, Issue, team-file, and existing-result context;
- complete lightweight operations within explicit foreground time and
  permission budgets;
- decompose a complex goal into one or more executable goals;
- choose general or specialized Tier 2 Agents;
- decide ordering, parallelism, dependencies, and follow-up work;
- synthesize several Tier 2 results while preserving one user-facing voice.

Tier 1 and Tier 2 are not distinguished by model strength or by whether they do
"real work." They differ by user relationship and execution lifetime. Tier 1
preserves interactive responsiveness; Tier 2 can execute durably, retryably,
and independently of the connection.

### Conversation

Conversation is an interaction thread between users and the system. It owns
replayable LLM message history, but does not own background execution resources.

A Conversation may:

- answer directly;
- request clarification;
- create a Task;
- continue an existing Task;
- display related Task/TaskRun projections;
- reference or create an Issue without forcing every conversation to become an
  Issue.

### Issue

Issue is a durable, collaborative, decomposable unit of team work. It can
receive execution results originating from Chat, an Agent, a Workflow, or a
direct action.

Issue does not require Conversation in order to execute. One Conversation may
discuss several Issues, and one Issue may be referenced from several
interaction entry points.

### Tier 2: Execution Agent

A Tier 2 Agent receives a bounded ExecutionSpec and performs focused work on
the persistent execution substrate. It does not own user dialogue directly,
but may return structured status, results, artifacts, and next-step suggestions
for Tier 1 to use.

The shared Agent runtime running in today's worker is the default general Tier
2 Agent. Purpose-specific Agents can be added through declarative definitions
rather than copied runtimes or hard-coded Agent classes.

A Tier 2 Agent may be selected by several entry points:

- Tier 1 selects dynamically from goal and capability descriptors;
- a Workflow step names one explicitly;
- an Issue assignee names one explicitly;
- an API or webhook uses a preconfigured executor;
- server policy validates the final Team, permissions, model, and execution
  profile.

### Task

Task is continuable Agent work with one continuous session and several
TaskRuns.

Its authoritative owner is `team_id`. Other relationships are optional origins
or delivery targets:

- `conversation_id`: where it originated or where it is projected;
- `source_message_id`: the user message that directly caused it;
- `issue_id`: the Issue it advances;
- workflow-step relation: owned by the existing workflow step run;
- trigger provenance: owned by the existing `trigger_source`.

### TaskRun

TaskRun is one immutable execution attempt. Existing status, input, output,
error, usage, trace, cancel, and retry-lineage fields remain the facts of the
run.

TaskRun terminal state is authoritative for result visibility. Chat cards,
Issue outputs, and notifications are projections of it, not independent copies
of run state.

### Execution Agent Definition

The current Portal Agent definition contains name, description, and
instructions and can remain the center of an execution Agent definition. Usage
evidence should determine whether it grows the following descriptors:

- suitability description: which goals fit this Agent;
- capabilities: research, coding, data processing, or other tags Tier 1 can
  match;
- allowed tools: the tool set this Agent may use;
- input/output contract: the context it accepts and result shape it promises;
- model policy: model or model-alias constraints;
- execution profile: resource, network, sandbox, and timeout requirements;
- workspace/memory policy: required workspace and memory scope.

These fields help Tier 1 select and the server validate. They should not become
a list of mutually exclusive, hard-coded Agent classes. One Agent may have
several capabilities, and a Workflow may bypass dynamic selection by naming an
Agent directly.

### ExecutionSpec

ExecutionSpec is the structured request sent from the interaction tier to the
execution tier. It need not become its own table in the first phase, but it
needs stable persisted fields and trace representation.

A candidate shape is:

```go
type ExecutionSpec struct {
    SourceMessageID string       `json:"source_message_id"`
    OriginalText    string       `json:"original_text"`
    Instructions    string       `json:"instructions"`
    ContextRefs     []ContextRef `json:"context_refs,omitempty"`
    TargetAgentID   *string      `json:"target_agent_id,omitempty"`
    Capabilities    []string     `json:"capabilities,omitempty"`
    IssueID         *string      `json:"issue_id,omitempty"`
    DeliveryTargets []Target     `json:"delivery_targets,omitempty"`
}
```

Constraints:

- at least one of `OriginalText` and `SourceMessageID` is present;
- Tier 1 may normalize `Instructions`, but may not overwrite the original
  message;
- Tier 1 may choose `TargetAgentID` directly or request capabilities from a
  constrained resolver;
- the server validates that Agent, Issue, and context references belong to the
  Team and satisfy execution policy;
- the worker receives an explicit version of the ExecutionSpec rather than
  rereading a Chat history that may have changed;
- the trace records source, size, and references without bypassing existing
  redaction to create a secret-bearing duplicate.

### Result Presentation

Results have two layers:

1. **Authoritative result card:** derived directly from Task/TaskRun and
   artifact data.
2. **Optional natural-language presentation:** produced during the worker run
   or by an asynchronous Presenter model.

Presenter success cannot change TaskRun status or prevent the authoritative
card from appearing. Presenter output records its provenance and treats worker
output as untrusted data rather than system instruction.

## Proposed Flow

```text
user message
  |
  |-- 1. persist conversation_message
  |
  v
Tier 1 Foreground Orchestrator Agent
  |-- direct answer / clarification / lightweight tool work
  |
  |-- decompose and coordinate one or more execution goals
  |
  `-- choose Tier 2 Agent(s) and produce validated ExecutionSpec(s)
        |
        |-- 2. persist Task + TaskRun
        |-- 3. render a durable Conversation task card immediately
        v
Persistent Execution Substrate
  scheduler -> worker -> Tier 2 Execution Agent(s)
                       |-- general Agent
                       `-- purpose-specific Agent definitions
        |
        |-- output
        |-- artifacts
        |-- trace and usage
        v
TaskRun terminal state
        |
        |-- 4. Conversation card becomes succeeded/failed/canceled
        |-- 5. project Issue result/comment when issue_id exists
        |-- 6. emit real-time invalidation/notification
        `-- 7. optionally generate a Presenter summary
```

Steps 3 and 4 recover from database state. WebSocket events only make the UI
reread sooner; they are not the data.

## New Tier 1 Responsibility Boundary

### Tier 1 Should

- handle natural-language interaction, intent recognition, clarification, and
  short answers;
- perform lightweight tool work within foreground latency, permission, and
  resource budgets;
- combine Conversation, Issue, Team, and existing-result context into a plan;
- decompose complex goals into independently executable goals;
- choose Tier 2 executors from Agent definitions, capabilities, and policy;
- coordinate ordering, parallelism, dependencies, cancellation, and follow-up
  across Tasks;
- query structured Task, Run, Issue, and result state;
- distinguish continuation of an existing Task from creation of a new Task;
- produce candidate ExecutionSpecs;
- synthesize several execution results and decide whether the user's goal is
  satisfied;
- explain progress and next steps without inventing run state;
- preserve the single user-facing voice.

### Tier 1 Should Not

- guarantee durable TaskRun delivery;
- use model memory as the source of Task state;
- guess whether work is complete without structured state;
- relabel worker output as a user message;
- become a mandatory path for Task creation from Issue, Workflow, or webhook;
- determine whether a result card exists by whether an LLM summary succeeds.

This boundary narrows system responsibility, not Agent capability. Tier 1 can
be stronger and more proactive while authoritative state remains in durable
infrastructure.

### When Work Stays In Tier 1 Or Moves To Tier 2

The boundary should not be "easy versus hard" or "weak model versus strong
model." It should be based on execution properties:

| Dimension | Tier 1 foreground work | Tier 2 background work |
|---|---|---|
| User relationship | Responds within the current interaction | Continues without a user connection |
| Time | Predictable short latency budget | May take an extended period |
| Durability | No independent retry/cancel needed | Must be durable and retryable/cancelable |
| Isolation and resources | Lightweight shared foreground policy | Independent worker, workspace, or resource policy |
| Specialization | General interaction and coordination | Specific tools, model, memory, or execution Agent |
| Parallelism | A few operations inside one turn | May become several independent parallel Tasks |

Dispatch to Tier 2 does not mean Tier 1 is incapable of the work. It means the
execution properties do not fit a foreground turn.

### Routing Policy

Chat still has to distinguish direct interaction from execution:

| Option | Strength | Main concern |
|---|---|---|
| Tier 1 decides automatically | Least user operation | A wrong dispatch is hard to explain and may answer instead of execute |
| User always selects Chat or Run | Deterministic | Makes users understand an internal mode |
| Hybrid | Natural default with an explicit override | Needs clear UI state and observable routing decisions |

The likely direction is hybrid: Tier 1 may produce ExecutionSpecs by default,
while the UI provides explicit execution feedback and a force-run or answer-only
control where useful. In every option, the server validates the spec and a
created task card becomes authoritative.

## Durable Outcomes And Real-Time Notifications

### Minimum: Project Existing Facts

The first phase does not need to copy full output into a `RunOutcome` table.
Existing records can construct the card:

- Task provides title, status, Issue, Agent, and latest run;
- TaskRun provides attempt status, output, error, usage, and trace;
- task_run_artifact provides artifacts;
- ConversationID scopes the projection during migration.

Conversation loads visible messages and related Tasks together. A TaskRun
terminal callback only broadcasts that a Task or Conversation changed. A
reconnected client queries the same database state and recovers.

### When An Outbox Is Needed

The following side effects should use a durable outbox or equivalent delivery
row if they become product requirements:

- automatic Presenter summaries;
- email, mobile, or external webhook notifications;
- third-party actions that must happen exactly once;
- actions needing independent retry and observable delivery state.

A database-backed TaskRun card does not need an outbox because it is a query
projection, not a side effect.

### Real-Time Scope

Notifications should broadcast to Conversation viewers or a Team scope rather
than only the Task creator's first connection. Authorization remains Team
membership. Events carry IDs and a change type, and clients retrieve details
through the normal team-scoped API.

## Conversation UI

### Task Card

When Tier 1 creates a Task, Conversation immediately shows a task card:

```text
+---------------------------------------------+
| Analyzing this month's sales data           |
| Running - started 10:42                     |
|                                             |
| Current progress or latest output           |
|                                             |
| [Stop] [Open details]                       |
+---------------------------------------------+
```

A terminal card shows:

- succeeded, failed, or canceled;
- a short output preview;
- artifact links;
- retry;
- a trace/details entry point;
- its Issue relationship, when present.

The UI may hide internal IDs but may not hide state and recovery controls.

### Transcript And Execution Events Stay Distinct

User messages and natural-language assistant replies form the transcript. A
task card is a structured execution projection and does not masquerade as a
user or assistant message.

The first phase can merge messages and Task projections by `created_at`, with a
stable type priority for events in the same second. If product or audit needs
later require a strict total order, a per-Conversation sequence can be designed
then. The first task card does not justify a general event store.

### Result Summary

The TaskRun card updates first. A natural-language summary may appear later or
come from a summary produced during the worker run. If summary generation
fails, the card and original preview remain visible; the UI does not claim the
result is still pending.

## Data Model Direction

### Minimal First Addition

Add traceability before removing every old constraint:

- add nullable `source_message_id` to Task or the initial TaskRun;
- record the routing decision and normalized instructions in the trace;
- return the latest Run and artifacts with Conversation-related Tasks;
- temporarily keep `conversation_id` required to avoid migrating the object
  storage key at the same time.

Placement depends on semantics:

- the original message that creates a Task belongs to Task;
- each ContinueTask message belongs to the corresponding TaskRun;
- in the long term, both may need a source message, or every execution origin
  can be normalized onto TaskRun.

### Later Remove Mandatory Conversation Ownership

The target direction is:

- `task.team_id` is authoritative for ownership and authorization;
- `task.conversation_id` becomes a nullable origin/delivery relation;
- an Issue Agent run creates an Issue-scoped Task directly;
- a Workflow step creates a Workflow-scoped Task directly;
- a webhook creates a Conversation only when its delivery channel needs one,
  not because Task schema requires one.

This is not only a nullable-column change. The migration must first address:

- ConversationID in worker directories and object-storage keys;
- artifact handlers that currently authorize through Conversation;
- session restoration paths;
- API paths and responses that assume Conversation;
- compatibility between one release of workers and servers.

New run-storage paths should be authoritative by Team, Task, and TaskRun rather
than requiring a synthetic ConversationID.

### Models Not Recommended Yet

The current phase should not:

- copy complete output into a new `run_outcome` row for every terminal run;
- create Conversation Task status separate from TaskRun state;
- encode a task card as a Markdown assistant message;
- introduce one general polymorphic event table for every Portal object;
- automatically promote every Task into an Issue.

## Trust And Security Boundaries

The following content types must remain distinct:

| Content | Producer | User instruction | In LLM history by default |
|---|---|---:|---:|
| User message | User | Yes | Yes |
| ExecutionSpec instructions | Tier 1, server-validated | Derived | Only in the target worker run |
| Worker output | Tier 2 Agent and tools | No | No; untrusted data for Presenter |
| Task state, usage, and trace references | Runtime | No | Queried through structured tools |
| Presenter summary | Presenter model | No | May persist as an assistant reply |

In particular:

- worker output is never replayed as `role=user`;
- a Presenter cannot start or continue a Task without a separately authorized
  orchestration turn;
- Task/result reads remain Team-scoped;
- context references are authorized before Task creation;
- summaries do not copy or spread content hidden by trace redaction.

## Options

| Option | Strength | Cost and risk | Judgment |
|---|---|---|---|
| Keep current Tier 1 -> Tier 2 | Least change; basic flow exists | Offline reply loss, untraceable rewriting, extra LLM dependency, synthetic Conversations | Do not keep unchanged |
| Merge into one synchronous Agent | Simple mental model; one continuous run | Long work blocks, browser lifetime leaks into execution, durable retry/cancel/isolation are lost | Reject |
| Suspend and resume one Agent run | May eventually unify interaction and execution context | Needs durable continuation, checkpoints, and cross-process streaming far beyond this problem | Keep as later research |
| Full Tier 1 orchestrator + Tier 2 Agent plane + persistent substrate | Preserves background execution, expands Tier 1, permits Tier 2 specialization, keeps outcomes reliable | Needs capability boundary, Agent descriptors, task cards, source linkage, and ownership migration | Recommended candidate |

## Phased Migration

### Phase 0: Repair The Current Contract — shipped

- Conversation loads its tasks and renders one card each, ordered against the
  messages by creation time;
- a terminal run broadcasts its invalidation to every connection on the team;
- the cards are read from the database on mount and on every invalidation,
  including a reconnect, so a refresh and a dropped socket both recover;
- the transcript excludes the system channel, so a Task Result is no longer
  displayed as the user's own message;
- the card is independent of the Presenter, so a summary that fails or never
  runs costs a sentence rather than the result;
- the roadmap's completion claim was corrected while this was open and updated
  when it shipped.

It changed no Task ownership and no object-storage path, and it did not make
delivery durable: the Tier 1 turn still runs from an in-process queue.

### Phase 1: Expand Tier 1 And Preserve Original Intent

- define foreground time, tool, permission, and resource budgets for Tier 1;
- let Tier 1 read the Team, Issue, file, and result context it needs;
- record the source message on TaskRun;
- define and validate ExecutionSpec;
- trace the relationship between original input and normalized instructions;
- record the source message for every continuation run;
- distinguish explicit run and automatic dispatch in provenance.

### Phase 2: Establish A Tier 2 Execution Agent Catalog

- build on existing Portal Agent definitions without copying the Agent runtime;
- determine which capability, tool, input/output-contract, and execution-profile
  fields have real demand;
- allow Tier 1 to recommend an Agent from descriptors;
- let Workflow, Issue, and API explicitly pin an Agent;
- have the server validate Team, permissions, model, and execution policy;
- record the selected Agent definition and selection source in the TaskRun
  trace.

This phase does not predefine `ResearchAgent`, `CodingAgent`, or other fixed
classes. Existing Agent definitions and usage evidence should first establish
which categories are stable.

### Phase 3: Make Presentation Optional

- remove natural-language result summary from the terminal callback's critical
  path;
- use durable delivery/outbox when automatic summary is required;
- retry summary independently while making TaskRun output immediately visible;
- evaluate generating structured summary during Tier 2 execution to avoid an
  extra call.

### Phase 4: Remove Conversation Ownership

- migrate run-storage paths;
- make Team the authoritative Task owner;
- make `conversation_id` an optional origin/delivery target;
- stop creating synthetic Conversations for Issue and Workflow;
- remove channels and empty conversations that existed only for the old schema.

### Phase 5: Update Product Language

- keep Tier 1/Tier 2, but define them as foreground orchestration and
  background execution Agent tiers separated by lifecycle;
- describe Task/TaskRun, scheduler, worker, and outcome projection as
  infrastructure rather than another Agent type;
- explain Conversation, Issue, Task state, and results in user documentation
  without exposing internal planes;
- record ownership and dependency boundaries in contributor architecture;
- move accepted rationale to `docs/design/` and delete this proposal.

## Acceptance Criteria

The implemented direction satisfies all of the following:

1. A user starts background work and closes the browser. After TaskRun
   completion, reopening the Conversation or Issue shows terminal state,
   preview, and artifacts.
2. A missing, timed-out, or quota-blocked Conversation model does not hide an
   already completed TaskRun.
3. A Task result is neither displayed as the user nor replayed as user
   instruction.
4. Every initial and continuation run created from Chat traces to its original
   source message.
5. Tier 1 normalized instructions and original user input can be inspected
   separately.
6. Tier 1 can complete bounded foreground work and can decompose, dispatch,
   and synthesize a goal across several Tier 2 runs.
7. Every Tier 2 run explains the selected Agent definition and whether the
   selection came from Tier 1, Workflow, Issue, or an explicit API request.
8. Multiple tabs viewing one Conversation converge on the same database state.
9. Task-card cancel, retry, artifact, and trace links remain Team-authorized.
10. After ownership migration, Issue Agent runs and Workflow runs no longer
    create synthetic Conversations.
11. Existing Tasks and one explicitly supported worker/server compatibility
    window remain readable and executable.
12. Architecture tests, service tests, handler tests, and Portal browser E2E
    cover online, offline, reconnect, queue-full, Presenter failure, and
    duplicate terminal callbacks.

## Questions Requiring Evidence

### Which Foreground Capabilities Should Tier 1 Have

Measure:

- the share of Chat messages that are direct answers, new Tasks,
  continuations, and status queries;
- which user goals fit a predictable foreground budget with lightweight tools;
- which Team, Issue, file, and result context Tier 1 needs for correct planning;
- how often users correct or repeat a request after a routing error;
- common omissions between the original message and Tier 1-generated Task
  input;
- token, latency, and cost share of Tier 1 dispatch and completion presentation.

If nearly every Portal Chat request creates a Task, the default may need to
become "execute unless clearly conversational." If direct answers are common,
hybrid routing is more valuable.

### Whether Stable Specialized Tier 2 Agents Already Exist

Inspect existing Agent definitions, Workflow steps, and Task traces to learn:

- whether teams repeatedly create Agents with the same purpose;
- which descriptors Tier 1 needs when choosing an executor;
- whether specialization mainly comes from instructions, tools, model,
  execution profile, or workspace/memory;
- whether dynamic selection improves success compared with an explicit
  Workflow choice;
- whether capability labels are stable enough to become a persisted API
  contract.

Until that evidence exists, this proposal decides only that Tier 2 permits
specialization, not what the fixed categories are.

### Whether Users Need Automatic Summaries

Compare:

- worker summary plus result card;
- an additional assistant reply produced by a Presenter after terminal state;
- an on-demand "Summarize result" action.

Measure time to result, summary fidelity, incremental cost, and whether users
open the artifact.

### The Product Relationship Between Conversation And Issue

Decide:

- when a Chat-created Task should attach to an existing Issue;
- whether a task card can explicitly create an Issue;
- whether an Issue needs a canonical Conversation;
- whether Team members opening another person's Conversation have the same
  continuation authority;
- whether real-time notification goes to creator, current viewers, Issue
  followers, or Team activity.

This proposal decides only that Conversation cannot remain the mandatory owner
of every Task. It does not pre-decide those interactions.

### Whether A Strict Timeline Order Is Necessary

The first phase can merge messages and Task projections by time. If product or
audit requirements later prove that a strict cross-object order is necessary,
add a per-Conversation sequence then. Do not add a general event model without
that evidence.

## Evidence Needed For A Decision

Before accepting this direction, collect at least:

- an end-to-end prototype in which a Task completes while the user is offline
  and appears after reopening the Conversation;
- a browser E2E for a task card moving through pending, running, and each
  terminal state;
- a count of synthetic `issue_agent` and `workflow` Conversations in test or
  real deployment data;
- annotated real Chat prompts for routing and Tier 1 input-fidelity review;
- a capability-clustering review of existing Agent definitions, Workflow steps,
  and Task traces;
- an end-to-end prototype in which Tier 1 delegates to two Tier 2 Agents and
  synthesizes the results;
- a result-visibility test with Presenter disabled or intentionally failing;
- convergence tests across multiple tabs and a server restart;
- one-release compatibility validation for the Task ownership and object-key
  migration.

## Likely Destination If Accepted

If this direction is accepted:

- schedule the Portal outcome follow-up and ownership migration in
  `ROADMAP.md`;
- record responsibility boundaries in
  `docs/contribute/architecture/overview.md` and the relevant Conversation,
  Task, server, and store architecture documents;
- create a durable subsystem design record under `docs/design/` containing the
  accepted rationale;
- update `docs/start/concepts.md` to explain behavior through user-facing
  objects rather than internal planes;
- correct the statement that Conversation owns Task in the data-model
  reference;
- delete this proposal and let Git history preserve the discussion.
