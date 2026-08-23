# Portal Execution Model: Tier 1, Tier 2, And What Carries Them

## Status

- roadmap_priority: `P2 follow-on`
- status: `partly implemented` — phases 0 and 3 shipped, phase 1 half shipped,
  phase 2 reduced to what evidence supports, phases 4 and 5 deferred
- follows: [product-vision.md](./product-vision.md),
  [surface-positioning.md](./surface-positioning.md),
  [issue-model.md](./issue-model.md)
- roadmap: [../ROADMAP.md](../ROADMAP.md)
- created_at: `2026-08-23`

Opened as a proposal on 2026-08-22 and accepted; this record replaces it. The
direction was never in doubt — keep Tier 1 and Tier 2, keep background workers,
and fix the seams between them. What the proposal was actually deciding was
which seams, and in what order.

Current schema is in
[../contribute/architecture/data-model.md](../contribute/architecture/data-model.md)
and the request path is in
[../contribute/architecture/server.md](../contribute/architecture/server.md);
this record keeps the reasoning.

## 1. Decision

Four things, described separately because conflating them is what produced the
defects this record closes:

1. **Tier 1 — foreground orchestrator agent.** Stays in contact with the user
   and owns interaction, intent, bounded foreground work, decomposition,
   executor selection, dispatch, and result synthesis.
2. **Tier 2 — execution agent plane.** Focused background work. Today one
   general-purpose agent; the shape allows specialized ones later without a
   Portal-only runtime.
3. **Persistent execution substrate.** Task, TaskRun, the scheduler, and workers
   carry Tier 2 and persist status, output, artifacts, traces, usage, and
   failure. They are not agents.
4. **Outcome projection.** Conversation task cards, issue results, and
   notifications are *derived from* durable state. Their existence must not
   depend on a live browser connection or a successful extra model call.

Point 4 is the load-bearing one. Most of what was wrong came from a projection
that was manufactured at announcement time instead of derived from a record.

## 2. What Divides Tier 1 From Tier 2

Not difficulty, and not model strength. Execution properties:

| Dimension | Tier 1 foreground | Tier 2 background |
|---|---|---|
| User relationship | Answers inside the current interaction | Continues with nobody connected |
| Time | Predictable short latency budget | May take an extended period |
| Durability | Needs no independent retry or cancel | Must be durable, retryable, cancelable |
| Isolation | Lightweight shared foreground policy | Its own worker, workspace, resource policy |
| Specialization | General interaction and coordination | Specific tools, model, memory, or agent |
| Parallelism | A few operations inside one turn | Several independent parallel tasks |

Dispatching to Tier 2 does not mean Tier 1 could not do the work. It means the
work's execution properties do not fit a foreground turn.

Tier 1 must never: guarantee durable delivery, use model memory as the source of
task state, guess completion without structured state, relabel worker output as
a user message, become the mandatory path for task creation from Issue,
Workflow, or webhook, or decide whether a result card exists by whether its own
summary succeeded. Each of those was true at some point and each produced a bug.

## 3. Transcript And Execution Events Stay Distinct

User messages and assistant replies are the transcript. A task card is a
structured execution projection. A card does not masquerade as a message, and a
message is not synthesized to carry execution state.

They are merged for display by `created_at`, with a stable type priority for the
same second — a task after a message, because the turn read the message and then
started the task. If a strict total order is ever needed, a per-conversation
sequence can be designed then; the first task card does not justify an event
store.

Content types stay distinct by trust level:

| Content | Producer | User instruction | In LLM history by default |
|---|---|---:|---:|
| User message | User | Yes | Yes |
| Normalized instructions | Tier 1, server-validated | Derived | Only in the target worker run |
| Worker output | Tier 2 agent and tools | No | No; untrusted data for the presenter |
| Task state, usage, trace refs | Runtime | No | Through structured tools |
| Presenter summary | Presenter model | No | May persist as an assistant reply |

Worker output is never replayed as `role=user`. A presenter cannot start or
continue a task without a separately authorized orchestration turn.

## 4. What Shipped

### 4.1 Outcome Projection Is Derived, Not Announced

A conversation carries one card per task it started, read from the tasks route
and reloaded on every invalidation the socket reports. Status, output, files,
run details, stop, and run again. The cards survive a refresh, a dropped socket,
and a summary that never arrives.

A terminal run broadcasts `task.status.changed` to every connection on the
team — an invalidation, not the outcome. It used to pick the creator's first
socket, which announced nothing when they had none and told one tab when they
had three.

The transcript excludes the system channel, so a `[Task Result]` message is no
longer drawn as the user's own.

### 4.2 Delivery Is Durable

The report a finished run owes its conversation is a row
(`task_result_delivery`), not a queued closure. One per run, claimed so two
servers cannot report one run twice, retried by a sweep, abandoned after a
bounded number of attempts with the reason kept. What the report says is derived
from the run on each attempt rather than stored.

The boundary is worth stating plainly: **what is durable is the obligation, not
the sentence.** An abandoned report is a lost sentence about the result, never a
lost result — the outcome is on `task_run` and the card reads it directly.

### 4.3 Intent Is Traceably Connected To Execution

`task_run.source_message_id` names the conversation message a run was asked for
in. Each run records its own: a task's first run names the message that created
it, a continuation names the message that asked for it. It is bound per turn
rather than passed per tool call, so the model cannot choose or omit what its
work is attributed to.

`GET /api/teams/{team_id}/task-runs/{task_run_id}` quotes that message next to
the instruction the worker was given. They are different texts and that is the
point: a constraint missing from the instruction is either one the model dropped
or one the user never gave, and nothing else can tell those apart.

### 4.4 Which Definition Ran Is Recorded

`task_run.agent_revision` names the agent definition a run was served.
Instructions are resolved when a worker asks for its run, so editing an agent
changes what its next run does — intended, and the reason the number is needed:
two runs of one task can execute different text. The first write wins, so an
edit during a run cannot rewrite what that run was given.

**Selection source needed no column.** Every site that picks an agent pairs
one-to-one with a distinct `trigger_source`: Tier 1's `StartTask` is
`portal_conversation`, the task route is `portal_task_create`, an issue run is
`issue_agent_run`, a workflow step is `workflow_step`. A second record of the
same fact would only give it a way to drift. It stops being derivable the first
time one trigger admits two ways of choosing — a conversation run where the user
pins an agent and Tier 1 passes it through — and that is when to add it.

### 4.5 Machinery Is Out Of The Conversation List

A workflow step and an issue agent run each create a conversation because Task
requires one. `ListConversationsByTeam` excludes those channels, count and page
together. This is the visible half of §5.1, not a fix for it.

## 5. What Is Deferred, And Why

### 5.1 Conversation Is Still The Mandatory Parent Of Every Execution

`task.conversation_id` is required. The target is `task.team_id` authoritative
for ownership and `conversation_id` a nullable origin and delivery relation, so
an issue run, a workflow step, and a webhook each create a task directly.

Deferred because it is not a nullable-column change. It must first address
`ConversationID` in worker directories and object-storage keys, artifact
handlers that authorize through Conversation, session restoration, API paths
that assume Conversation, and compatibility between one release of workers and
servers. New run-storage paths should be authoritative by Team, Task, and
TaskRun.

The user-visible cost of waiting was synthetic conversations crowding the
conversation list; §4.5 removed it for a few lines rather than a migration.

### 5.2 The Tier 2 Catalog

Which capability, tool, input/output-contract, and execution-profile fields an
agent definition should carry is **evidence-gated**. Existing definitions and
usage should first establish which categories are stable. Writing the fields now
would be inventing demand.

Do not predefine `ResearchAgent`, `CodingAgent`, or similar Go types before that
evidence exists.

What already works: Tier 1 recommends an agent from the team's summaries via
`StartTask`, and Workflow, Issue, and the task route each pin one explicitly.
Team membership is validated; model and execution policy are not.

### 5.3 Tier 1's Foreground Budget And ExecutionSpec

Foreground time, tool, permission, and resource budgets, the context Tier 1 may
read, and a validated ExecutionSpec are all open.

Deferred safely because Tier 1 is already bounded in practice: ten iterations, a
non-interactive policy, and four task tools with no filesystem or shell access.
The budget becomes a **prerequisite** the moment Tier 1 is allowed to read team
files, issues, and results — not before.

Attachments and context references are also unrecorded. `source_message_id` is
the anchor they would hang from, and it exists now.

### 5.4 A Structured Summary During Tier 2 Execution

Generating the summary inside the worker run would save a model call. It stays
an evaluation rather than a decision: nobody has measured that call as a cost.

### 5.5 Product Language

Defining Tier 1 and Tier 2 as foreground and background tiers separated by
lifecycle, and describing Task, TaskRun, scheduler, and worker as infrastructure
rather than another agent type, is naming work with no behavior attached. It
belongs after §5.1, which changes what those objects are.

## 6. Models Not Recommended

- copying complete output into a `run_outcome` row for every terminal run;
- conversation-level task status separate from TaskRun state;
- encoding a task card as a Markdown assistant message;
- one general polymorphic event table for every Portal object;
- automatically promoting every task into an issue.

## 7. Rejected Alternatives

| Alternative | Why not |
|---|---|
| Keep the original Tier 1 → Tier 2 path unchanged | Offline reply loss, untraceable rewriting, a mandatory extra model call, synthetic conversations |
| Merge into one synchronous agent | Long work blocks the conversation; a browser's lifetime leaks into execution; durable retry, cancel, and isolation are lost |
| Suspend and resume a single agent run | Needs durable continuation, checkpoints, and cross-process streaming far beyond this problem. Kept as later research |

## 8. Questions Still Requiring Evidence

- **Which foreground capabilities Tier 1 should have.** Which requests users
  expect answered in the conversation rather than dispatched.
- **Whether stable specialized Tier 2 agents already exist.** Usage should name
  the categories, not this document.
- **Whether users want automatic summaries at all**, or prefer the card and an
  on-demand explanation. This decides how much §4.2 is worth.
- **The product relationship between Conversation and Issue** — when a chat
  request should become tracked work.
- **Whether a strict per-conversation timeline order is necessary**, or whether
  `created_at` with a type priority is enough. So far it is enough.
