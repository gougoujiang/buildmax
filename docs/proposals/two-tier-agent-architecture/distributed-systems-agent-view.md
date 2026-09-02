# Distributed-Systems Agent View: Durable Orchestration Is The Authority

> **Audience:** participants in the two-tier Agent architecture roundtable · **Status:** proposal — under discussion · **Author role:** independent distributed-systems review Agent

Opened: 2026-08-30

Related: [roundtable index](README.md),
[current Portal execution model](../../design/portal-execution-model.md),
[product vision](../../design/product-vision.md),
[current-state assessment](../../current-state.md), and
[roadmap](../../ROADMAP.md).

## Contents

- [1. Scope, Goals, And Non-Goals](#1-scope-goals-and-non-goals)
- [2. Position](#2-position)
- [3. First-Principles Invariants](#3-first-principles-invariants)
- [4. The Correct Responsibility Boundaries](#4-the-correct-responsibility-boundaries)
- [5. State, Idempotency, Fan-Out, Recovery, And Permissions](#5-state-idempotency-fan-out-recovery-and-permissions)
- [6. What BuildMax Already Gets Right](#6-what-buildmax-already-gets-right)
- [7. Where The Current Design Conflates Responsibilities](#7-where-the-current-design-conflates-responsibilities)
- [8. Concrete Current-Code Observations](#8-concrete-current-code-observations)
- [9. Agreements, Disagreements, And Qualifications](#9-agreements-disagreements-and-qualifications)
- [10. Options](#10-options)
- [11. Recommended Target Architecture](#11-recommended-target-architecture)
- [12. Suggested Evolution Sequence](#12-suggested-evolution-sequence)
- [13. Evidence Needed](#13-evidence-needed)
- [14. Open Questions](#14-open-questions)

## 1. Scope, Goals, And Non-Goals

This is an independent review of BuildMax's two-tier Portal Agent design from
the perspective of distributed systems and durable orchestration. It treats
the current accepted design as implementation context, not as a conclusion the
roundtable is required to preserve.

The goals are to:

- define the authority boundary among an orchestrator service, a Planner Agent,
  and Worker Agents;
- assign state machines, idempotency, fan-out/fan-in, recovery, and permissions
  to their correct owners;
- distinguish the parts of current BuildMax that already form a sound durable
  execution substrate from the parts that remain process-local or model-owned;
- identify concrete failure windows in the current implementation; and
- propose a target that works for a single background run before requiring a
  general multi-Agent graph engine.

The non-goals are to:

- choose user-facing names for these roles;
- claim that dynamic multi-Agent decomposition is already needed;
- prescribe a specific queue, workflow engine, or database product;
- redesign every existing Task, Workflow, and Issue entity in this paper; or
- document any proposed behavior as shipped.

## 2. Position

BuildMax should preserve the foreground/background execution split, but the
authoritative orchestrator must be a deterministic durable service rather than
a Conversation Agent.

The most important distinction is not between a Tier 1 Agent and Tier 2 Agents.
It is among:

1. semantic judgement, where a model can understand ambiguity and propose a
   plan;
2. authoritative orchestration, where deterministic code validates, records,
   and advances that plan;
3. execution, where a Worker Agent completes one bounded objective under a
   fixed grant; and
4. presentation, where durable outcomes are projected or explained to a user.

The short rule is:

> A model may propose what should happen. The orchestration kernel owns what is
> allowed, what has happened, and which transition may happen next.

BuildMax already has a credible durable worker substrate in Task, TaskRun, the
scheduler, workers, Artifacts, traces, and result-delivery records. It does not
yet have a complete durable multi-step orchestration kernel. In particular, a
Workflow's reaction to a terminal TaskRun still crosses a process-local
callback window rather than a durable event or obligation.

The current physical topology therefore resembles enterprise
orchestrator/worker systems, while the current semantic behavior is closer to
an Agentic conversation dispatcher in front of a durable job runtime.

## 3. First-Principles Invariants

### 3.1 Durable state is truth; model context is a view

A system must be able to answer these questions without asking a model to
reconstruct them from text:

- Which goal was accepted?
- Which execution specification was authorized?
- Which nodes exist and what do they depend on?
- Which attempt currently owns execution?
- Which effects and Artifacts were reported?
- Which terminal events still require handling?
- Which completion condition has or has not been met?

Conversation history, summaries, and Planner reasoning can help explain these
facts. They cannot be their only representation.

### 3.2 One owner defines every transition

Every authoritative state transition needs one implementation with explicit
preconditions. A Handler, Planner, Worker, scheduler, recovery sweep, and
Workflow callback must not each invent their own meaning of `complete`,
`retry`, or `ready`.

The orchestration service owns logical readiness and completion. The scheduler
owns placement of already-ready attempts. A Worker owns execution inside one
attempt and reports observations. This separation makes competing actors safe:
only the actor that satisfies the persisted transition precondition wins.

### 3.3 Processing is at-least-once; committed effects are deduplicated

An Agent runtime cannot promise end-to-end exactly-once execution. A Worker may
change an external system, lose connectivity, and fail before recording the
effect. A Server can commit a terminal run and crash before invoking a listener.

The useful guarantees are narrower:

- commands and events may be delivered at least once;
- state transitions use compare-and-swap or an equivalent version check;
- consumers deduplicate by a stable command or event identifier;
- a logical retry creates a new immutable attempt;
- a terminal attempt cannot be rewritten; and
- external side effects use idempotency keys or effect receipts where the
  target supports them.

When an external effect cannot be proven absent or present, the honest state is
`uncertain` or `waiting_for_review`, not an automatic retry presented as safe.

### 3.4 Authority only narrows as it moves toward execution

A Planner cannot grant a capability it was not given. A Worker cannot expand
its own tools, plugins, secrets, model, filesystem scope, network access, or
budget. Authorization begins with the actor and Team policy, is validated at
admission, is frozen into an execution grant, and is enforced again at each
resource boundary.

### 3.5 Recovery is reconciliation, not memory

Recovery compares durable desired state with observed state and performs the
next idempotent action. It does not ask a model to remember whether something
probably happened.

This is the difference between a conversation that can discuss old work and an
orchestration system that can safely resume it.

## 4. The Correct Responsibility Boundaries

| Responsibility | Orchestrator service | Planner Agent | Worker Agent |
|---|---|---|---|
| Interpret an ambiguous goal | Supplies authorized context | Proposes an interpretation or asks a question | Does not reinterpret the parent goal |
| Decompose work | Validates and persists an accepted graph | Proposes nodes, dependencies, and completion policy | May request delegation; does not create durable children directly |
| Select an executor | Validates capability, policy, availability, and Team scope; freezes the choice | Recommends a capability or Agent | Cannot choose itself into more authority |
| Own state machines | Sole authority | No | Reports attempt-local observations |
| Decide readiness | Computes dependency and policy predicates | May recommend sequencing | No |
| Dispatch | Creates or releases a runnable attempt; scheduler places it | No direct dispatch authority | Claims one authorized attempt |
| Retry and cancel | Records policy and creates a new attempt or cancellation request | May recommend | Cooperatively stops its own attempt |
| Fan-out and fan-in | Persists children, edges, join policy, and join outcome | Proposes dynamic expansion | Produces one node result |
| Validate output | Enforces schema and deterministic predicates | May perform semantic review as a bounded verifier | Produces result and evidence |
| Determine composite completion | Applies persisted completion policy | A `done` statement is advisory | Cannot declare the parent complete |
| Permissions | Authorizes, attenuates, snapshots, and signs grants | Uses only visible authorized capabilities | Enforces and consumes the fixed grant |
| Recovery | Reconciles durable records, leases, and obligations | Can be called again for an explicit replan | Restores only its own checkpoint if supported |
| User presentation | Projects durable state or invokes a bounded presenter | May act as presenter under a separate no-control profile | May produce a candidate deliverable, not the authoritative user reply |

The orchestrator service and scheduler are also distinct. The orchestrator says
which logical node is ready. The scheduler says where and when a ready attempt
runs. Polling a `PENDING` TaskRun and launching a process is scheduling; it is
not by itself multi-step orchestration.

## 5. State, Idempotency, Fan-Out, Recovery, And Permissions

### 5.1 State machine ownership

For a single background objective, BuildMax's existing Task and TaskRun split
already captures a useful distinction: stable work versus an execution attempt.
A composite goal needs one additional logical layer, regardless of the eventual
entity names:

```text
Composite execution / PlanRun
  └── logical NodeRun
        └── immutable Attempt, possibly represented by TaskRun
```

A representative lifecycle is:

```text
PlanRun
  proposed -> validated -> running
                           |-> waiting_input
                           |-> waiting_approval
                           |-> succeeded
                           |-> failed
                           `-> canceled

NodeRun
  waiting_dependencies -> ready -> active -> terminal

Attempt
  pending -> scheduled -> running -> succeeded / failed / canceled
```

The names are less important than three rules:

1. a logical node and one attempt are not the same object;
2. terminal attempt records are immutable; and
3. the parent completion condition is explicit rather than inferred from the
   latest natural-language answer.

Task status may remain a user-facing projection of the relevant run. Dependency
advancement must read authoritative node and attempt state, not a duplicated UI
projection.

### 5.2 Idempotency and fencing

Every command that can be repeated across a timeout or restart needs a stable
key. Examples include:

- accept this Plan proposal;
- create this child node;
- materialize an attempt for this node generation;
- consume this terminal event;
- publish this Artifact reference; and
- deliver this outcome to this destination.

Uniqueness constraints should express domain facts such as one consumption per
`(event_id, consumer)` and one attempt creation per `(node_run_id, generation)`.
State changes then use a version or expected status.

A worker lease should identify both the attempt and an ownership generation or
fencing token. Heartbeats demonstrate recent contact; they do not prove an old
Worker stopped executing after a partition. If a replacement attempt starts,
the old generation must be unable to commit new orchestration state. It may
still have caused an external side effect, which is why tool-level idempotency
and explicit uncertainty remain necessary.

### 5.3 Fan-out and fan-in

A Planner may propose a dynamic graph such as:

```text
                 +-> investigate A --+
accepted goal ---+-> investigate B --+-> synthesize -> verify
                 +-> investigate C --+
```

The orchestration kernel must:

- reject cycles;
- bound depth, node count, concurrency, cost, and time;
- record whether children are required or optional;
- record an explicit join policy such as `all`, `any`, `quorum`, or
  `best_effort`;
- compute the ready set from durable state;
- record failure and cancellation propagation policy; and
- make dynamic graph expansion append-only or versioned.

A synthesis Agent is simply another node released by a join. It does not need
authority over the graph to combine results.

### 5.4 Recovery

A recovery loop should be able to derive actions such as:

- a ready node has no attempt, so create one idempotently;
- a scheduled attempt has no live owner, so record a dispatch failure or an
  uncertain loss according to policy;
- a running attempt's lease expired, so fence its state writes and escalate or
  create a new attempt only when retry policy permits;
- a terminal event has not been consumed by Workflow advancement, so retry the
  consumer;
- a join predicate became true but the successor is absent, so create it using
  the same idempotency key; and
- an outcome delivery is still owed, so retry or record bounded abandonment.

The recovery loop should operate from durable records even when no Conversation
exists and no model is available.

### 5.5 Permissions

The expected grant flow is:

```text
actor and trigger authority
  -> Team policy and admission
  -> immutable ExecutionSpec
  -> run-scoped capability token
  -> Worker sandbox and service-gateway enforcement
```

The service decides which context the Planner may see. The Planner returns
references rather than arbitrary credentials. The service validates every
reference and freezes the effective grant. The Worker enforces local tool and
filesystem policy, while the Server or gateway independently enforces access
to Team resources and managed inference.

This is privilege attenuation, not trust in the Planner's prompt compliance.

## 6. What BuildMax Already Gets Right

The current design contains several strong distributed-system decisions that
should survive any change in Agent terminology.

### 6.1 Foreground and detached execution are separated by lifecycle

The [Portal execution model](../../design/portal-execution-model.md) correctly
uses latency, durability, cancellation, isolation, and parallelism rather than
difficulty or model strength to separate foreground and background execution.
This derives two execution lifecycles, even though it does not derive two
mandatory Agent identities.

### 6.2 Task and TaskRun separate work from attempts

The current [Task domain](../../../internal/core/task/task.go) gives TaskRun an
explicit lifecycle and makes terminal statuses immutable. Retry creates a new
attempt rather than overwriting the record that explains an earlier failure.
That is the right basis for provenance, accounting, and recovery.

### 6.3 Transitions use compare-and-swap and commit projections together

[`TransitionTaskRun`](../../../internal/infra/db/task_run.go) checks the
expected status, updates the TaskRun, records terminal Artifact paths, and
updates the Task projection in one transaction. A worker and a recovery sweep
therefore cannot both overwrite the accepted outcome.

This is a stronger correctness boundary than any natural-language instruction
to an Agent could provide.

### 6.4 Unknown side effects are not retried automatically

The [stale-run reaper](../../../internal/server/scheduler/stale_runs.go) records
a lost Worker as failed or canceled but does not automatically repeat it. The
code explicitly recognizes that arbitrary side effects may already have
happened. That caution is correct and should remain when retry policy becomes
more expressive.

### 6.5 Outcome projection does not depend on a live announcement

Task cards and Issue results read durable run state. A WebSocket event is an
invalidation rather than the only copy of an outcome. A dropped connection or
a failed summary does not erase the result.

### 6.6 Result delivery persists the obligation

`task_result_delivery` records that a terminal run still owes a Conversation a
report. Claiming uses a lease, retries are bounded, and one row per TaskRun
makes enqueue idempotent. The important abstraction is that the obligation is
durable even though the generated sentence is not.

This is the pattern that orchestration advancement should generalize: persist
the obligation or event in the same durable boundary as the state change, then
process it at least once with deduplication.

### 6.7 Provenance is moving in the right direction

`source_message_id`, Agent revision, plugin pins, trigger source, actor, trace,
and usage make execution inspectable. Run-scoped credentials also prevent a
Worker from choosing the Team or Task identity it presents to the Server.

These are necessary foundations, although the snapshot timing and effective
runtime permission boundary still need work.

## 7. Where The Current Design Conflates Responsibilities

### 7.1 `Orchestrator` refers to both a model and a control plane

The current Portal design says Tier 1 owns interaction, intent, decomposition,
executor selection, dispatch, and result synthesis. Those are not one trust
domain.

Understanding and decomposition may require a model. Dispatch, authorization,
state advancement, dependency satisfaction, retry, and completion require a
deterministic authority. Result synthesis requires access to untrusted results
but normally no control capability.

Using one word for all three hides where correctness has to live.

### 7.2 Conversation is treated as an execution parent

The [product vision](../../design/product-vision.md) names Issue as the primary
user-facing work object, but also describes Conversation as Tier 1 and Task plus
TaskRun as Tier 2 reporting to it. The current required
`task.conversation_id` turns an interaction origin into an ownership parent.

Team should own shared execution. Conversation, source message, Issue,
Workflow step, webhook, or another trigger should be optional origins and
delivery destinations. Durable work must remain complete without a synthetic
Conversation.

### 7.3 Planner output is too close to execution authority

When a Conversation Agent rewrites a user request, selects an Agent, and calls
StartTask, one probabilistic output simultaneously becomes instruction text,
executor selection, and a dispatch command. Server validation can prevent an
invalid Team reference, but it does not by itself prove instruction fidelity,
appropriate permissions, or a valid completion contract.

A typed PlanProposal followed by deterministic validation separates semantic
judgement from authority and leaves a record of what was accepted.

### 7.4 Presentation is coupled to orchestration context

Raw Worker output is untrusted data. Sending it through a Conversation Agent
that later regains task tools creates a path from data to control influence.
A Presenter may use the same model implementation, but it should run with a
different input envelope and no orchestration authority.

### 7.5 A two-tier name hides two orthogonal axes

BuildMax has at least two independent distinctions:

| Axis | Side one | Side two |
|---|---|---|
| Lifecycle | Foreground interaction | Durable background execution |
| Authority | Deterministic control plane | Agentic execution/data plane |

A Planner and Presenter are optional semantic roles around these axes, not
necessarily permanent tiers. Treating the whole system as a supervisor Agent
above subordinate Agents makes an implementation topology look like a domain
invariant.

## 8. Concrete Current-Code Observations

### 8.1 Workflow terminal advancement is not durable yet

The strongest current correctness gap is the path from a terminal TaskRun to
the next Workflow step.

[`runterminal.Announcer`](../../../internal/server/handlers/runterminal/runterminal.go)
loads the terminal run and starts an in-process callback. Server assembly wires
that callback through
[`buildOnTaskRunTerminal`](../../../internal/server/server.go) to
[`workflow.Service.HandleTaskRunTerminal`](../../../internal/service/workflow/service.go).
Failures are logged and are not propagated to a durable retry mechanism.

The TaskRun terminal transition is already committed before this callback. A
Server crash in the gap can therefore leave:

```text
TaskRun = SUCCEEDED
WorkflowStepRun = RUNNING
next Workflow step = never dispatched
```

The current durable result-delivery sweep repairs an analogous Conversation
reporting gap, but no equivalent Workflow-advancement obligation or terminal
event consumer exists.

The fix should not be merely to retry the callback. The terminal transition
should atomically enqueue a durable event or advancement obligation. A consumer
then claims it, applies an idempotent Workflow transition, and records
consumption. A reconciliation sweep handles missed or expired claims.

### 8.2 Creating a Workflow step Task and binding it are not atomic

`workflow.Service.dispatchNextStep` first creates the Task and TaskRun through
`createStepTask`, then updates WorkflowStepRun with their identifiers and its
`running` status. A failure between those operations can create an orphaned
active Task while the step remains `pending`. Replaying the operation without a
domain idempotency key can create another Task.

The orchestration boundary needs an invariant such as one active logical
execution per `(workflow_step_run_id, generation)`. It may be implemented by a
transaction where package boundaries permit it, or by an idempotent command
and unique key with reconciliation. The invariant matters more than whether
the mechanism is a single database transaction.

### 8.3 Current Workflow records are durable, but the engine is not fully so

WorkflowRun and WorkflowStepRun persist a linear plan, step status, target
Agent snapshot, Task, and TaskRun references. That is useful orchestration
state. However, durable records alone do not make an engine durable when the
transition trigger and dispatch side effects can be lost between records.

This distinction matters when describing current capability: BuildMax has a
durable linear Workflow model and a working happy path, not yet crash-safe
multi-step orchestration.

### 8.4 Scheduler claiming is safe but does not yet provide a full lease model

The [scheduler](../../../internal/server/scheduler/scheduler.go) reads the
oldest `PENDING` run and uses an expected-status transition to claim it as
`SCHEDULED`. Competing schedulers cannot both win that state transition, which
is correct.

There is no persisted scheduler owner, dispatch generation, or fencing token.
A process that dies after claim can leave a run `SCHEDULED` until the broad
stale timeout records it as failed. Heartbeats detect a Worker that had reached
`RUNNING`, but a heartbeat is evidence of liveness, not proof that an old Worker
stopped after a partition.

This may be acceptable for the current Alpha scheduler. It is not sufficient
evidence for automatic failover or safe automatic retry of side-effecting work.

### 8.5 Process-local coordination conflicts with multiple Server replicas

The [current-state assessment](../../current-state.md) correctly identifies
that the production manifest declares multiple Server replicas while the
WebSocket hub and Conversation turn queue remain process-local. If a
Conversation Agent is also treated as the authoritative orchestrator, split
turn serialization becomes a control-plane correctness problem rather than
only a live-UI issue.

Until shared coordination exists, the supported topology should be one Server
replica. A multi-replica control plane requires durable commands or a shared
queue, distributed serialization for the relevant aggregate, shared event
delivery, and idempotent consumers.

### 8.6 ExecutionSpec freezes too late and inconsistently

For ordinary TaskRuns, Agent revision and plugin pins are resolved when the
Worker asks for its run, and the first write records what that Worker received.
This is valuable actual-execution provenance, but queued work can change
meaning before claim if an Agent definition or Team activation changes.

WorkflowStepRun, by contrast, snapshots the target Agent definition when the
Workflow run starts. The two paths therefore answer `which definition should
queued work use?` differently.

The preferred enterprise default is early binding at admission or PlanRun
activation:

```text
request accepted
  -> policy and capability resolution
  -> immutable ExecutionSpec stored
  -> scheduler and Worker consume that exact spec
```

Late binding can remain an explicit policy when the product wants `use the
latest approved definition at execution time`, but it should not be an
accidental consequence of claim timing. Provenance should record both the
binding policy and the exact effective snapshot.

An ExecutionSpec should eventually cover at least:

- normalized instructions plus the original source reference;
- Agent and revision;
- model profile;
- tool and plugin pins;
- permission and sandbox profile;
- workspace and context references;
- time, token, cost, and resource budgets;
- output schema or acceptance contract;
- actor, origin, and delivery destinations; and
- retry, cancellation, and binding policy.

### 8.7 The effective Worker permission boundary remains a P0 gap

The current-state assessment records that the Worker runtime does not select
the stricter Worker sandbox surface and uses an allow-all tool policy. A
run-scoped Server token is useful but does not contain local filesystem, child
process, hook, MCP, or network behavior by itself.

The design should therefore distinguish `grant recorded` from `grant enforced`.
Enterprise orchestration is not safe until both the Worker runtime and every
service boundary enforce the effective grant and record any downgrade.

### 8.8 Result delivery has lease idempotency but not a universal event model

`task_result_delivery` is intentionally narrow, which is a virtue for its
current job. Its success does not mean every terminal consequence is durable.
Workflow advancement, Issue reporting, live invalidation, and optional
presentation each have different current delivery semantics.

The next abstraction need not be one polymorphic event table for every Portal
object. It does need a consistent rule: every consequence that must survive a
crash is either committed atomically with its source transition or is
recoverable by a deterministic reconciler.

## 9. Agreements, Disagreements, And Qualifications

### 9.1 Agreements

This view agrees with the following current or emerging positions:

- foreground and background work require different execution lifecycles;
- TaskRun, Artifact, trace, and durable outcome projection should remain;
- simple deterministic commands should bypass a Planner model;
- a model suggests actions while the system validates and records them;
- raw Worker output is not a user instruction;
- a Presenter should not inherit orchestration tools;
- a Worker cannot unilaterally create durable children or enlarge permissions;
- Team, not Conversation, is the ownership and authorization boundary; and
- dynamic fan-out/fan-in should be added only when evidence shows recurring
  demand.

### 9.2 Disagreements

This view rejects the following as stable architecture claims:

- `Conversation Agent owns orchestration`;
- `every durable execution must have a Conversation parent`;
- `every Worker result must pass through another foreground model call`;
- `two execution lifecycles imply two Agent identities`;
- `a persisted Workflow graph alone proves durable orchestration`; and
- `a model's synthesis or verifier answer is itself authoritative completion`.

### 9.3 Qualifications

One logical BuildMax Assistant is a good product model but is insufficient as
an internal correctness model. Once the product supports dynamic decomposition,
parallel children, joins, replanning, or approval, it needs explicit durable
plan and node state outside Conversation history.

The rule against Worker-created children also needs precision. A Worker may use
ephemeral subagents inside the same attempt, sandbox, budget, and failure
boundary. A child that needs independent durability, cancellation, quota,
provenance, or recovery must be created by the orchestration kernel from a
validated delegation request.

## 10. Options

| Option | Strength | Failure or limitation | Position |
|---|---|---|---|
| Keep the current Tier 1 Agent as orchestrator | Small conceptual change; natural-language routing already works | Mixes model judgement with control authority; no durable composite plan; repeats trust and recovery problems | Reject as target |
| One synchronous Agent for all work | Simple identity and context | Browser lifetime, latency, isolation, cancellation, and recovery do not fit long work | Reject |
| One logical Assistant with direct and detached modes | Clear product model; preserves workers | Does not alone define durable multi-step orchestration | Accept as product framing, not complete internals |
| Deterministic kernel plus optional Planner and bounded Workers | Separates authority, semantics, and execution; supports simple and complex work | Requires explicit specs, events, and state models | Recommend |
| Adopt a full general workflow engine immediately | Mature timers, retries, and graph execution may be available | Adds operational and semantic weight before demand is measured; does not solve Agent trust by itself | Evaluate later, not a prerequisite |
| Let each Worker recursively create durable Workers | Flexible emergent decomposition | Unbounded authority, cost, fan-out, recovery ambiguity, and graph ownership | Reject; accept only validated delegation requests |

## 11. Recommended Target Architecture

```text
User / Issue / Workflow / Webhook
                 |
                 v
Interaction API / Conversation Service
                 |
        +--------+---------+
        |                  |
deterministic command   optional Planner Agent
                       read-only structured context
                       typed PlanProposal
        |                  |
        +--------+---------+
                 v
Durable Orchestration Kernel
- admission and Team authorization
- immutable ExecutionSpec
- PlanRun / NodeRun / Attempt state
- dependency and join evaluation
- cancellation, retry, waiting, and approval
- event obligations, idempotency, and recovery
                 |
                 v
Scheduler / placement
        +--------+--------+
        |        |        |
        v        v        v
      Worker   Worker   Worker
        |        |        |
        +--- typed outcomes and evidence ---+
                                            |
                                            v
                              Durable state and Artifacts
                                            |
                         +------------------+------------------+
                         |                                     |
                         v                                     v
                deterministic projection             optional Presenter
```

The defining data flow is:

> Immutable, attenuated capabilities flow downward. Untrusted claims and
> evidence flow upward. Only the kernel converts accepted claims into durable
> orchestration state.

A direct background request is a one-node plan and should not pay the complexity
cost of a graph. An explicit Workflow is a predeclared plan. A Planner-created
dynamic graph is the same orchestration substrate with a different proposal
source. This lets one kernel serve deterministic and Agentic use cases without
making a model mandatory.

## 12. Suggested Evolution Sequence

### 12.1 Make current transitions crash-safe

- Persist a terminal TaskRun event or Workflow-advancement obligation.
- Make Workflow terminal consumption idempotent.
- Make step Task creation and StepRun binding atomic or reconcilable under a
  unique domain key.
- Add recovery tests for a crash at every commit/callback boundary.
- Support one Server replica honestly until shared coordination exists.

### 12.2 Freeze and enforce the execution envelope

- Define the minimum ExecutionSpec.
- Choose and record early versus explicit late binding.
- Wire the Worker sandbox surface and non-allow-all permission policy.
- Enforce the same Team and run grant at Server resource boundaries.
- Record effective downgrades and refuse ones policy marks fail-closed.

### 12.3 Separate model roles

- Keep deterministic task commands outside Planner calls.
- Make Planner output typed and advisory.
- Remove raw Worker output from ordinary Conversation history.
- Give Presenter and verifier calls separate no-control profiles.

### 12.4 Add composite orchestration only with evidence

- Introduce PlanRun and NodeRun semantics or generalize the existing Workflow
  model.
- Persist dependencies, joins, budgets, and completion policy.
- Allow dynamic expansion through validated, bounded proposals.
- Add waiting-for-input and approval states before pretending a failed model
  call represents every interruption.

The entity migration should be decided in a separate accepted design. This
paper recommends the invariants, not a premature table layout.

## 13. Evidence Needed

The recommendation should be tested against deterministic failure injection
and product evaluation.

### 13.1 Distributed correctness evidence

- Crash after TaskRun terminal commit but before Workflow advancement.
- Crash after child Task creation but before StepRun binding.
- Duplicate terminal-event delivery to two Server replicas.
- Expired claim followed by a late Worker report.
- Network partition where an old Worker continues external work.
- Cancellation followed by Worker loss.
- Retry of an idempotent tool call and of a non-idempotent tool call.
- Server restart with ready nodes, outstanding joins, and owed deliveries.
- Policy or Agent edits while a run waits in the queue.

Each case should assert the durable state, permitted next action, deduplication
key, and user-visible projection.

### 13.2 Agent and product evidence

- Frequency of requests that genuinely need dynamic decomposition.
- Planner instruction-preservation accuracy against the source message.
- Agent and capability selection accuracy.
- Cost and latency added by Planner, verifier, and Presenter calls.
- User value from automatic summaries versus durable cards and on-demand
  explanation.
- Adversarial Worker output influencing later control decisions.
- Human success in understanding one Assistant while internal roles remain
  separate.

### 13.3 Falsification criteria

This recommendation should be reconsidered if evidence shows all of the
following:

- dynamic multi-node work is negligible;
- deterministic direct commands cover nearly all Portal use;
- no accepted use case requires recoverable joins, replanning, or approval;
  and
- the operational cost of an orchestration abstraction exceeds the failure
  risk of the remaining single-run system.

That evidence would support keeping only the current Task/TaskRun substrate and
removing `orchestrator` language, not restoring model ownership of state.

## 14. Open Questions

- Should Task remain the logical single-node work object while a new PlanRun
  groups Tasks, or should Workflow become the general orchestration aggregate?
- Which execution fields must freeze at request admission, Plan activation,
  scheduler claim, and Worker start?
- What is the smallest event/outbox shape that makes Workflow advancement
  durable without creating a universal Portal event store?
- Which external tools can support BuildMax-provided idempotency keys or effect
  receipts, and how should unknown effects be represented?
- Does cancellation mean only `the accepted result is canceled`, or must the
  platform prove the underlying process and all child effects stopped?
- Which join policies are needed by observed workloads rather than by imagined
  generality?
- When should a semantic verifier influence completion, and when is human
  approval required?
- Can Conversation turn serialization remain a presentation concern once all
  durable commands are idempotent, or does the product need aggregate-level
  distributed ordering?
- Should automatic result presentation exist at all, and if so, which durable
  object owns its idempotency and provenance?
