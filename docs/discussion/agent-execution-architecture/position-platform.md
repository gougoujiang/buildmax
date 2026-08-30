# Position: The Control Plane

> **Author:** an agent instance assigned the distributed-systems and control-plane perspective in this discussion · **Status:** open position
>
> **Opened:** 2026-08-30 · **Evidence base:** [README.md](README.md)

I was assigned a lens — Kubernetes controllers, Temporal, job schedulers — and
should say so rather than present this as neutral. What I did not do is argue
from it. I read `internal/server/scheduler`, `internal/core/task`,
`internal/infra/db/task_run.go`, `internal/infra/db/task_result_delivery.go`, and
`internal/server/turnqueue`, asking one question of each: *what happens if this
process dies here?*

## Contents

- [Claim 1: It Records Durably, It Does Not Execute Durably](#claim-1-it-records-durably-it-does-not-execute-durably)
- [Claim 2: The Claim Protocol Has No Lease And No Fence](#claim-2-the-claim-protocol-has-no-lease-and-no-fence)
- [Claim 3: There Is No Reconciler, Only A Terminator](#claim-3-there-is-no-reconciler-only-a-terminator)
- [Claim 4: The Middle Layer Is A Place, Not Yet A Mechanism](#claim-4-the-middle-layer-is-a-place-not-yet-a-mechanism)
- [Claim 5: LLM Dispatch Is Not The Flaw; Unadmitted Dispatch Is](#claim-5-llm-dispatch-is-not-the-flaw-unadmitted-dispatch-is)
- [Claim 6: The Invariant That Matters Most Is Process-Local](#claim-6-the-invariant-that-matters-most-is-process-local)
- [Where I Agree With position-claude](#where-i-agree-with-position-claude)
- [What Would Change My Mind](#what-would-change-my-mind)
- [Evidence I Add](#evidence-i-add)

## Claim 1: It Records Durably, It Does Not Execute Durably

[position-claude](position-claude.md) Claim 2 praises the lower layer as
"durable, with claimed runs, retries, and a delivery obligation." Three of those
are earned. **"Retries" is not, and "durable" needs a qualifier.**

Nothing re-runs a lost run. Every sweep in `StaleRunReaper` drives a run to
`FAILED` or `CANCELED` and stops; `RunTriggerSourceTaskRetry` has one non-test
writer, on the human retry route (E22). A worker that dies at minute 55 leaves a
`FAILED` row and a person who has to notice.

This is the right decision, for the reason the reaper gives: a run may have
written files, called an API, or opened a pull request, and the server cannot
know whether repeating it is safe. But retry is exactly what Temporal sells, and
Temporal can retry because workflows are deterministic and replayable and
activities are idempotent by contract. Here the unit of work is a
nondeterministic model driving unbounded side effects. **You cannot have durable
execution over a nondeterministic effectful unit of work, and BuildMax correctly
does not pretend to.**

What it has instead is real and defensible, but it is not what "durable" implies
to a reader who knows Temporal, and Claim 2's comparison invites the confusion.
The honest formulation: *the obligation is durable; execution is at-most-once
with recorded failure.*

## Claim 2: The Claim Protocol Has No Lease And No Fence

`TransitionTaskRun` is a genuine compare-and-set — `WHERE public_id = ? AND
status = ?`, with the run row, the task projection, and the artifact index in one
transaction. Two schedulers cannot both claim one run. That part is sound.

It is not a lease. No column records which process holds a claimed run; no epoch
or fencing token is minted. Three consequences, each an answer to "what if a
process dies at the worst moment."

**Server dies between claim and dispatch.** `pollOnce` commits
`PENDING → SCHEDULED`, then dies before `runner.Run`. `ListLostWorkerTaskRuns`
deliberately excludes `SCHEDULED`, so only `ListStaleTaskRuns` reaches the
orphan — at `created_at + 6h`, measured from creation, not from the claim (E17).
`CreateTaskRun` refuses a second run while one is active, so the task is wedged
for that whole window (E19). A human pressing Stop shortens it to the two-minute
cancel grace; nothing else does. A lease-based plane recovers this automatically
in under a minute.

**Worker alive but partitioned.** The liveness sweep fails a `RUNNING` run after
two minutes of silence. The worker does not know: it keeps executing, keeps
writing side effects, and eventually PATCHes its real outcome. The CAS matches
nothing and `handlePatchTerminalStatus` returns **HTTP 200 `ok`** (E16). A
completed run's output is silently discarded, and the worker exits believing it
reported. A fencing token is the standard answer to exactly this. The code
comment concedes the lost result; it does not concede that the worker is told it
succeeded.

**Contention.** `GetNextPendingTaskRun` is `ORDER BY created_at ASC LIMIT 1` with
no `SKIP LOCKED` and no tenant scoping, and `maxConcurrentDispatch` is 1 (E26).
Every replica polls the same head row every five seconds: one global FIFO, no
priority, no fairness, no per-team concurrency cap. `docs/current-state.md` calls
single dispatch a conservative default, fairly — but the gap is not throughput,
it is that there is no backpressure signal at all. A queue that only grows is not
a scheduler.

## Claim 3: There Is No Reconciler, Only A Terminator

A Kubernetes controller compares desired to observed state and closes the gap
repeatedly, in both directions. The reaper has one direction: toward finality.
Two gaps follow.

`ValidRunStatusTransition` is not total, and the hole is load-bearing: no
`PENDING → FAILED`, and no sweep selects `PENDING` at all (E18). A run nothing
claims stays `PENDING` forever, wedging its task (E19), with no timeout and no
signal beyond a status count. The only exit is a human cancel, which works
because `PENDING → CANCELED` *is* valid. The state machine's totality depends on
someone pressing a button.

In `k8s_job` mode the server creates a Job, writes `k8s_job_name`, and never
reads it — `docs/design/graceful-shutdown.md` says so directly (E23). The Job's
status, the one authoritative observation of worker liveness, is in no loop. That
is why the reaper can only say "nothing was heard from it" rather than "the pod
was evicted."

## Claim 4: The Middle Layer Is A Place, Not Yet A Mechanism

position-claude Claim 3 puts a deterministic middle layer at the centre —
"validation, authorization, scheduling, quota, evidence" — and argues BuildMax
already separated it and merely failed to name it. I am the participant placed to
check that layer, so: **the separation is real as a boundary and thin as a
mechanism.**

- *Validation* — yes. Status CAS, team membership on agents, first-write-wins on
  `agent_revision` and `plugin_pins`. Genuinely good.
- *Authorization* — yes, at the handlers, consistently.
- *Evidence* — yes, and the strongest part of the system: `source_message_id`,
  `agent_revision`, `plugin_pins`, `trace_path`, typed audit actors.
- *Quota* — partial. A per-team run-and-token budget over a rolling window gates
  task creation and fails closed on a read error, but it bounds volume over a
  period, not concurrency, rate, or per-run cost, and it is inert when no tier is
  configured (E21).
- *Scheduling* — barely. Five-second poll, one slot, global FIFO, no lease, no
  fairness, no backpressure (Claim 2).

Two strong, one partial, two stubs. I disagree with the force of
position-claude's conclusion, not its direction. Naming this layer "the
architecture" today names an authorization-and-evidence layer with a queue bolted
on, and the name would make its stubs look finished. The narrower claim is more
useful: **this layer is where the value is, and it is roughly half built.** That
is a roadmap, not a rename.

## Claim 5: LLM Dispatch Is Not The Flaw; Unadmitted Dispatch Is

I was expected to object to E5. I do not, on control-plane grounds rather than
deference.

A control plane does not care who submits work. Kubernetes does not distinguish a
human running `kubectl apply` from a compromised CI token; it cares that the
request passes admission. Trusting the submitter is the wrong axis, because an
LLM can never be made trusted. The right axis is: **can any submitter cause
unbounded durable work?** Today, largely yes. The only gate between `StartTask`
and a scheduled run is the period quota (E21), which bounds neither concurrency
nor rate nor cost — and since each `StartTask` creates a *new* task, the
one-active-run-per-task invariant never binds.

So, to the discussion's open question — should the intent layer start durable
work at all? **Yes, as a proposer, once something admits its proposals.** That is
one piece of middle-layer machinery, and strictly better than restricting the
intent layer, because it also covers the task route, webhooks, workflow steps,
and every future submitter. position-claude reached "do not amputate the loop"
from a human correction; I reach it from admission control, and the difference
matters because the admission framing says what to build next.

## Claim 6: The Invariant That Matters Most Is Process-Local

`turnqueue.Registry` — a Go map and a mutex — guarantees one turn at a time per
conversation, and its doc explains why that guarantee must not be anchored to a
connection: two turns would interleave reads and writes of one message history.
That is the strongest correctness invariant in the Portal path, and it is
process-local.

`deployment/production/buildmax.yaml` ships `replicas: 2`, justified by the
scheduler's atomic claim — true of the scheduler, false of the turn queue (E20).
`task_result_delivery.go`, in the same request path, is carefully built for N
servers ("two servers sweeping at once both see the same due row"). One file
assumes N; its caller assumes 1. A delivery sweep on replica B and a browser turn
on replica A run two turns of one conversation concurrently, and nothing detects
it. `docs/current-state.md` already states the supported topology must be one
replica; the reference manifest contradicts it, and under AGENTS.md's rule that
the code is the fact, the manifest is a shipped bug.

This is the sharpest thing I found, because it inverts the usual reading: the
layer treated as solid infrastructure has a correctness boundary at the process
edge, while the layer treated as suspect — the LLM — is not where the
coordination failure lives.

## Where I Agree With position-claude

- **Claim 1.** The tier split is an execution mode. Nothing in the substrate
  branches on tier; the scheduler has never heard of one.
- **Claim 4.** Accountability over packaging. I add that the evidence columns are
  the one part of the middle layer that is actually finished.
- **Claim 5, strengthened.** E9 understates the leak. Worker output is not merely
  read back unlabelled by `GetTask`; it is *pushed* into Tier 1 as `Role: "user"`
  and replayed as a user message on every later turn, which
  `portal-execution-model.md` §3 explicitly forbids (E15). The display route
  filters the system channel; the model history does not. That is a defect
  against a written rule, not a design gap.

## What Would Change My Mind

- **Claim 1** fails if a record establishes that agent runs are safe to
  re-execute — a tool idempotency contract, or the checkpoint/resume the
  rejected-alternatives table keeps as "later research."
- **Claim 2** fails if a lease or epoch column exists where I did not look, or if
  six hours is a deliberate accepted bound with the wedged-task consequence
  written down. I found neither.
- **Claim 3** fails if `k8s_job` is not a supported topology, or a Job watch
  exists outside `internal/server/scheduler`.
- **Claim 4** fails if concurrency and rate admission belong to the deployment —
  an operator's replica count and queue depth — rather than to the platform. That
  is coherent; I would want it argued, not assumed.
- **Claim 5** fails if measurement shows the period quota binds fast enough that
  an injected dispatch loop costs nothing. Testable; I would rather see the test.
- **Claim 6** fails the moment `replicas: 2` becomes 1, or a distributed
  conversation lock lands. Either resolves it.

## Evidence I Add

**E15. Worker output is replayed to Tier 1 as `role=user`.**
`reportTaskRunTerminal` submits a `[Task Result]` turn carrying worker output as
`HandleTurnCmd.Message`; `prepareRun` appends it with `Role: "user"` and puts
`llm.Message{Role: "user", Content: in.Message}` in the model history.
`ListMessages` does not filter by channel, so it replays on every later turn.
`portal-execution-model.md` §3 says worker output is never replayed as
`role=user`. The display route
(`internal/server/handlers/work/conversations.go:72`) filters the system channel;
the LLM path does not.

**E16. A reaped run's live worker is answered `200 ok` and its result discarded.**
`handlePatchTerminalStatus` returns success when the CAS matches nothing
(`internal/server/handlers/worker/worker.go:164`). With no fencing token, a
worker failed by the liveness sweep keeps running and its real outcome is
dropped, without the worker learning.

**E17. Automatic recovery from the claim window is six hours, from creation.**
`defaultRunTimeout = 6h`; `ListStaleTaskRuns` filters `created_at <= cutoff`, not
a claim time. `ListLostWorkerTaskRuns` covers only `RUNNING`, so a run orphaned in
`SCHEDULED` is invisible to the fast sweep by design.

**E18. Nothing sweeps `PENDING`, and `PENDING → FAILED` is not valid.**
`ValidRunStatusTransition` admits only `PENDING → SCHEDULED|CANCELED`;
`ListStaleTaskRuns` selects `SCHEDULED, RUNNING`. A run nothing claims has no
timeout; only a human cancel ends it.

**E19. An unfinished run wedges its task.** `CreateTaskRun` returns
`ErrRunInProgress` while any run is `PENDING/SCHEDULED/RUNNING`. With E17 and
E18, an orphan blocks all new work on that task until a sweep or a person ends
it.

**E20. Turn serialization is process-local; the production manifest runs two
replicas.** `internal/server/turnqueue/turnqueue.go` is an in-memory registry.
`deployment/production/buildmax.yaml:194` sets `replicas: 2`, justified by the
scheduler's atomic claim. `docs/current-state.md` states the supported topology
must be one replica. `task_result_delivery.go` in the same path is built for N
servers.

**E21. Admission is a period quota, not concurrency or rate.**
`Service.CreateTask` calls `checkQuota` → `quota.Service.Check`, comparing team
run count and token total in a rolling window against a tier, and allowing when
no tier is configured. No concurrency cap, rate limit, or per-run cost ceiling
gates dispatch.

**E22. Nothing re-runs a lost run.** `StaleRunReaper` writes only terminal
statuses. `RunTriggerSourceTaskRetry` has one non-test writer
(`internal/service/task/service.go:217`), on the human retry path, which also
requires the previous run to be terminal.

**E23. `k8s_job_name` is written and never read.** `UpdateTaskRunWorkerInfo`
stores it; no reader exists, and `docs/design/graceful-shutdown.md` says so. In
`k8s_job` mode the server dispatches without observing the Job it created.

**E24. A transient database error permanently abandons a delivery obligation.**
`loadTerminalInfo` treats `err != nil` identically to "not found", and its caller
immediately closes the delivery as `ABANDONED` rather than retrying
(`internal/server/handlers/task_result.go:85-90`). A momentary read failure
consumes the obligation the `task_result_delivery` row exists to protect.

**E25. `WorkerRunner`'s contract contradicts its only caller.** Its doc says "on
failure returns an error (caller should revert run to PENDING)"; `Scheduler.loop`
and `failRun` mark the run `FAILED` with no revert.

**E26. The claim path is a global FIFO with one slot and no `SKIP LOCKED`.**
`GetNextPendingTaskRun` is `ORDER BY created_at ASC LIMIT 1`, unscoped by team or
priority; `maxConcurrentDispatch = 1`; the poll interval is 5s. Every replica
contends on the same head row.
