# Tier 2 Visibility In The Portal Conversation

> **Audience:** contributors, Portal maintainers, and operators · **Status:** proposal — under discussion
>
> **Opened:** 2026-08-30

Related: [Portal execution model](../design/portal-execution-model.md) §1–§4;
[durable run trace](../design/durable-run-trace.md); [graceful
shutdown](../design/graceful-shutdown.md) §5.1;
[../current-state.md](../current-state.md) P0 "The Reference Replica Count
Exceeds Coordination Semantics"; and [../ROADMAP.md](../ROADMAP.md).

## Contents

- [Problem](#problem)
- [Decision Sought](#decision-sought)
- [The Boundary, Stated Precisely](#the-boundary-stated-precisely)
- [What The Code Does Today](#what-the-code-does-today)
- [Why Live Tier 2 Output Is The Wrong Thing To Fix](#why-live-tier-2-output-is-the-wrong-thing-to-fix)
- [Proposed Direction](#proposed-direction)
- [What This Does And Does Not Do To The Replica P0](#what-this-does-and-does-not-do-to-the-replica-p0)
- [Open Questions](#open-questions)
- [Likely Destination If Accepted](#likely-destination-if-accepted)

## Problem

The Server carries a path that attaches a browser to a Tier 2 agent's live
output: the worker POSTs model deltas, an in-memory hub buffers them, and an SSE
endpoint streams them to the Portal. It has never had a consumer.

That absence was first read as an unfinished feature, and the question asked of
it was how to make the hub work across Server replicas. The better question is
whether a browser should be attached to a Tier 2 agent at all. Under the tier
boundary this repository already committed to, it should not — which makes the
missing consumer a symptom rather than the defect.

## Decision Sought

1. **Is Tier 2 a user-facing concept in a Portal conversation, or an execution
   detail of Tier 1?** This paper argues the second.
2. **Does the live Tier 2 output path get finished, distributed, or deleted?**
   This paper argues deleted, as a consequence of the first answer.

## The Boundary, Stated Precisely

"Portal talks only to Tier 1" is the right instinct and the wrong rule: taken
literally it forbids the task card, which reads the tasks route and shipped
deliberately as [§4.1](../design/portal-execution-model.md).

The distinction is not which tier the Portal calls. It is whether the Portal is
**reading a derived projection of durable state** or **attached to a live
agent**.

[§1](../design/portal-execution-model.md) point 3 settles the first half: Task,
TaskRun, the scheduler, and workers are "persistent execution substrate … They
are not agents." Reading substrate rows to render a projection is not
conversing with Tier 2. [§1](../design/portal-execution-model.md) point 4 then
requires that such a projection "must not depend on a live browser connection or
a successful extra model call."

Subscribing to a running agent's output stream fails both halves: it is an
attachment to the agent rather than a read of a record, and it exists only while
a browser is connected. It is also what [AGENTS.md](../../AGENTS.md) forbids when
it says Tier 2 "reports results to Tier 1; it does not speak directly to the
user."

## What The Code Does Today

The Tier 2 output path is alive on the write side and dead on the read side.

- **Producing is real.** A worker streams model output through
  [`internal/agentapp/taskrun/runtime.go`](../../internal/agentapp/taskrun/runtime.go)
  to the debounced sender wired in
  [`internal/bootstrap/worker.go`](../../internal/bootstrap/worker.go); the
  handler in
  [`internal/server/handlers/worker/worker.go`](../../internal/server/handlers/worker/worker.go)
  calls `Hub.Append`. Debouncing is 80 ms or 512 bytes, so a running run posts up
  to roughly twelve times a second.
- **Buffering is in memory.** `memStreamHub` in
  [`internal/server/websocket/hub.go`](../../internal/server/websocket/hub.go)
  holds up to 2 MiB per task and is cleared on `Done`.
- **Consuming is nothing.** The hub's only reader is the task SSE endpoint in
  [`internal/server/handlers/work/stream.go`](../../internal/server/handlers/work/stream.go),
  whose only client implementation — `subscribeTaskStream` in
  [`portal/src/features/tasks/api.ts`](../../portal/src/features/tasks/api.ts) —
  is called by no Portal component. The WebSocket alternative
  (`subscribe.task`, `task.stream.delta`, `task.stream.done` in
  [`internal/server/websocket/protocol.go`](../../internal/server/websocket/protocol.go))
  has constants and payload structs but no producer and no handler on either
  side.

[Graceful shutdown](../design/graceful-shutdown.md) §5.1 already recorded the
missing consumer without drawing a consequence from it.

What the Portal actually renders is
[`TaskCard.tsx`](../../portal/src/features/conversations/components/TaskCard.tsx):
a status label, an output preview, Stop, Run again, and a files list, reloaded
from the tasks route on the `task.status.changed` invalidation. Tier 1
conversation streaming is a separate path that never touches the hub.

## Why Live Tier 2 Output Is The Wrong Thing To Fix

- **The tier is defined by the user not being there.**
  [§2](../design/portal-execution-model.md) gives Tier 2 the user relationship
  "Continues with nobody connected." Streaming tokens to someone who has by
  definition moved on optimizes a case the tier was defined not to have.
- **The repository already rejected persisting deltas.** [Durable run
  trace](../design/durable-run-trace.md) decided not to record streaming deltas
  because they are redundant with the final content and would bloat the file.
  Making the stream durable would contradict that decision rather than extend it.
- **It costs something today.** Roughly twelve HTTP posts a second per running
  run, for output nothing reads.
- **The foreground case is unaffected.** Tier 1 turns stream to the user over
  `?stream=1` SSE and the socket sink. That is where streaming earns its keep —
  the user is waiting on that exact text — and this paper does not touch it.

## Proposed Direction

**1. Tier 2 stops being user vocabulary.** A conversation user should never need
to learn "task", "run", or "worker" to understand that the assistant is working
on something. Tier 2 remains fully visible to operators through run details,
traces, and artifacts; the claim is about the conversation surface only.

**2. Delete the live Tier 2 output path.** `StreamHub`, the task SSE endpoint
and its OpenAPI entry, the three unused WebSocket constants and their payload
structs, `subscribeTaskStream`, and the worker-side `SendDelta` and
`DebouncedStreamSender`. Under Alpha rules this is a deletion, not a
deprecation. It restores a boundary rather than merely removing dead code.

**3. Add no progress fields.** An earlier draft of this paper proposed
structured progress on `task_run` — current tool, step counter — carried on the
existing five-second cancel poll. That is the same assumption at a coarser
grain: "which tool is it running" is precisely the Tier 2 detail this boundary
hides. What a conversation user needs is that work is in flight, roughly how
long it has been, and how to stop it. `status` and `started_at` already carry
the first two and the card already carries the third, so the direction requires
no new state.

**4. The card stays, reframed.** It remains the projection — derived from
durable rows, surviving a refresh, a dropped socket, and a summary that never
arrives. What changes is what it claims to be: an indication that the assistant
is working, not a console attached to a run.

The net shape is close to pure deletion, which is the point. The cheapest
version of this design is the one that stops doing something.

## What This Does And Does Not Do To The Replica P0

It removes one of the three process-local mechanisms
[current-state.md](../current-state.md) names, and it removes the one that never
cost anything, because nothing read it.

It leaves both mechanisms that do:

| Mechanism | Live consumer | Cost of two Server replicas |
|---|---|---|
| `ConnRegistry` | Yes | A user connected to one replica misses invalidations broadcast by another; the card goes stale until reloaded. The durable report still lands. |
| `turnqueue.Registry` | Yes | Two turns for one conversation can run concurrently on different replicas — a broken invariant, not a missed notification. |

The turn queue is a correctness problem and is untouched by this paper. The
supported production topology remains an open question, and accepting this
direction must not be read as answering it.

## Open Questions

- **Stop and Run again are Tier 2 verbs.** "Run again" is inherently a run
  concept; re-asking is more naturally a conversation act that dispatches new
  work through Tier 1 than a retry button on a card. Does the card keep
  lifecycle controls, or does it keep only "stop what you are doing"?
- **The card previews raw worker output.**
  [§3](../design/portal-execution-model.md) classifies worker output as
  untrusted data for the presenter. Showing it in a projection is sanctioned,
  but it is worth deciding whether the card should show the worker's own text or
  Tier 1's synthesis of it.
- **Several parallel tasks.** [§2](../design/portal-execution-model.md) lists
  parallelism as a Tier 2 property. With Tier 2 out of the vocabulary, a
  conversation running three of them needs a way to say so that is not three
  run handles.
- **Does any deployment need live background output at all?** Operator-facing
  run inspection is a different surface from a conversation, and incremental
  trace upload — traces record a `trace_path` only at terminal today — would
  serve it without putting an agent stream in front of a conversation user.

## Likely Destination If Accepted

[Portal execution model](../design/portal-execution-model.md), whose tier split
already owns what Tier 2 reports and how; a section there would record that the
conversation surface holds no live Tier 2 attachment and why. The P0 in
[current-state.md](../current-state.md) is then restated to name `ConnRegistry`
and the turn queue separately, without the hub.
