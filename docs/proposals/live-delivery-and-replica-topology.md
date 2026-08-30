# Live Delivery And Server Replica Topology

> **Audience:** contributors, operators, and Portal maintainers · **Status:** proposal — under discussion
>
> **Opened:** 2026-08-30

Related: [../current-state.md](../current-state.md) P0 "The Reference Replica
Count Exceeds Coordination Semantics"; [../ROADMAP.md](../ROADMAP.md); [graceful
shutdown](../design/graceful-shutdown.md) §5.1; [Portal execution
model](../design/portal-execution-model.md); [enterprise
deployment](../design/enterprise-deployment.md); and
[`deployment/production/buildmax.yaml`](../../deployment/production/buildmax.yaml).

## Contents

- [Problem](#problem)
- [Decision Sought](#decision-sought)
- [What The Code Actually Does](#what-the-code-actually-does)
- [The Three Mechanisms Have Different Stakes](#the-three-mechanisms-have-different-stakes)
- [Goals](#goals)
- [Non-Goals](#non-goals)
- [Options And Trade-Offs](#options-and-trade-offs)
- [Open Questions](#open-questions)
- [Likely Destination If Accepted](#likely-destination-if-accepted)

## Problem

The production manifest configures two Server replicas, while every live
coordination mechanism in the Server is process-local. `current-state.md` records
this as a P0 and names three mechanisms: the stream hub, the WebSocket
connection registry, and the per-conversation turn queue.

That framing is correct but treats the three as one problem. Reading the code
shows they are not comparable: one of them has no consumer at all, one breaks a
correctness invariant, and one breaks user-visible delivery. A single "make it
distributed" answer would build shared infrastructure for a mechanism that is
currently dead, and a single "drop to one replica" answer would leave the reason
each mechanism exists unexamined.

The question that prompted this paper was narrower — *is the stream hub still
meaningful?* — and the answer turned out to reframe the P0.

## Decision Sought

Two decisions, in this order:

1. **What is the supported production topology today?** One Server replica until
   coordination exists, or two replicas with the losses documented as accepted.
2. **What happens to Tier 2 live task output?** It is fully wired from the worker
   into the Server and read by nobody. Finish it, distribute it, or delete it.

## What The Code Actually Does

The Tier 2 task output path is alive on the write side and dead on the read
side.

**Producing.** A worker run streams model output through a sink adapter in
[`internal/agentapp/taskrun/runtime.go`](../../internal/agentapp/taskrun/runtime.go),
which POSTs deltas to the worker stream endpoint via the debounced sender wired
in [`internal/bootstrap/worker.go`](../../internal/bootstrap/worker.go). The
handler in
[`internal/server/handlers/worker/worker.go`](../../internal/server/handlers/worker/worker.go)
calls `Hub.Append`. This is real production wiring, not a test path.

**Buffering.** `memStreamHub` in
[`internal/server/websocket/hub.go`](../../internal/server/websocket/hub.go)
holds up to 2 MiB per task in memory and fans out to subscribers.
`runterminal.Announcer` clears it on `Done`.

**Consuming — nothing does.** The hub's only reader is the task SSE endpoint in
[`internal/server/handlers/work/stream.go`](../../internal/server/handlers/work/stream.go).
Its only client implementation, `subscribeTaskStream` in
[`portal/src/features/tasks/api.ts`](../../portal/src/features/tasks/api.ts), is
referenced by its own unit test and by a design record, and by no Portal
component. The WebSocket alternative — `subscribe.task`, `task.stream.delta`,
`task.stream.done` in
[`internal/server/websocket/protocol.go`](../../internal/server/websocket/protocol.go)
— has constants and payload structs but no producer and no handler on either
side.

So the hub is a write-only buffer: workers push deltas into it every run, and
they are discarded at `Done` without ever being read.
[Graceful shutdown](../design/graceful-shutdown.md) §5.1 already recorded the
observation ("The task SSE endpoint turned out to have no live consumer in the
Portal") without drawing the consequence for the replica question.

What the Portal actually watches is the WebSocket: `task.status.changed` in
[`portal/src/features/conversations/hooks/useConversationTasks.ts`](../../portal/src/features/conversations/hooks/useConversationTasks.ts),
and Tier 1 conversation deltas, which stream from the request goroutine over
`?stream=1` SSE or over the socket sink — neither of which touches the hub.

Durability is unaffected by any of this. `task_run.Output` is written on the
terminal transition, and the report owed to the conversation is enqueued in
`task_result_delivery` with a DB-backed claim and retry. Those are the record;
the hub never was.

## The Three Mechanisms Have Different Stakes

| Mechanism | Has a live consumer | What two replicas cost |
|---|---|---|
| `StreamHub` | No | Nothing observable. Dead on the read side. |
| `ConnRegistry` | Yes | A user connected to replica A misses `task.status.changed` and message events announced on replica B. The card stays stale until a manual reload; the durable report still lands. |
| `turnqueue.Registry` | Yes | Two turns for one conversation can run concurrently on different replicas. This is a broken invariant, not a missed notification. |

Only the turn queue is a correctness problem. The connection registry is a
user-visible staleness problem that the code already treats as best-effort — the
comment on `reportTaskRunTerminal` says a card nobody is connected to see does
not stop the report from being made. The hub costs nothing because nothing reads
it.

## Goals

- State one supported production topology and make the manifest agree with it.
- Decide whether Tier 2 live output is a product commitment or removable surface.
- Keep the durable path — `task_run.Output` and `task_result_delivery` — as the
  single record, whatever is decided about live delivery.

## Non-Goals

- Horizontal scaling of the worker plane. Runs are already claimed by atomic
  status transition and are not affected.
- Tier 1 conversation streaming, which does not use the hub.
- Any change to what is persisted. This paper is about live transport only.

## Options And Trade-Offs

**A. Honest topology now, decide delivery later.** Set the Server Deployment to
one replica, correct the manifest comment that claims replicas are safe because
the Server keeps no local state that matters, and record the availability cost
(a rollout or node drain is a brief outage; the PodDisruptionBudget becomes
meaningless). Cheapest, immediately true, and it does not spend design effort on
a mechanism whose fate is undecided. It withdraws an availability property the
manifest currently advertises.

**B. Distribute what has consumers.** Keep one replica as the supported topology
until a shared conversation lock replaces `turnqueue.Registry` and a pub/sub
fan-out replaces `ConnRegistry` broadcast. Adds a Redis-class dependency to a
deployment that currently needs only MySQL and object storage. Correct, and the
largest amount of work — and note it does not require the hub at all.

**C. Delete the Tier 2 live stream.** Remove `StreamHub`, the task SSE endpoint,
its OpenAPI entry, the unused protocol constants, `subscribeTaskStream`, and the
worker-side delta sender. The worker stops making an HTTP call per debounce
window for output nobody reads. Loses the option of live run output without
rebuilding it, and removes published API surface — cheap under Alpha rules, but
it is a product decision, not a cleanup.

**D. Finish the Tier 2 live stream.** Wire a Portal consumer, then it acquires a
real consumer and joins B's scope. Only worth doing if watching a background run
token-by-token is a product commitment; the durable report already delivers the
result.

A and C are compatible and independent of each other. B is the eventual answer
if horizontal scaling is wanted. D decides whether B's scope includes the hub.

## Open Questions

- Is live token-by-token output of a *background* run a product commitment, or
  is a status card plus the delivered result the intended Tier 2 experience? The
  answer decides C versus D, and nothing in the code answers it.
- Does the private deployment target actually require more than one Server
  replica, or was `replicas: 2` a default rather than a requirement? If no
  deployment needs it, A is not a withdrawal of anything real.
- How long has the Tier 2 stream had no consumer, and did it ever? Git history
  would say whether this is an unfinished feature or a regressed one.
- If B is taken, is a Redis dependency acceptable in the private-deployment
  contract, or should the conversation lock use the database that is already
  required?

## Likely Destination If Accepted

The topology decision belongs in
[enterprise deployment](../design/enterprise-deployment.md) and in the manifest.
The Tier 2 delivery decision belongs in
[Portal execution model](../design/portal-execution-model.md), whose two-tier
split already owns what Tier 2 reports and how. `current-state.md` P0 and its
rebased priority order are then restated to name the mechanisms separately
rather than as one coordination gap.
