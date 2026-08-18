# Trusted Private Execution Loop

> **Audience:** contributors, operators, and early adopters · **Status:**
> proposal — under discussion

Related: [roadmap](../ROADMAP.md) P0.5, P3, and P4; [trust harness
design](../design/trust-harness.md); [enterprise deployment
design](../design/enterprise-deployment.md); [managed LLM gateway
design](../design/llm-gateway.md); [team governance
design](../design/team-governance.md); and [versioned workspace
design](../design/versioned-workspace.md).

## Problem And Current Context

BuildMax is now public and has the main pieces of its product promise: one
shared Agent runtime, local CLI/TUI and Desktop execution, Portal results,
background task runs, a private-deployment reference, and basic team
governance. The next risk is not that an Agent cannot perform more work. It is
that a private team cannot yet prove one ordinary task is safe enough,
explainable enough, and operable enough to trust in production.

Several recent changes narrow that gap without closing it:

- Worker Jobs are non-root, drop capabilities, use a read-only root filesystem,
  have no mounted service-account token, and receive only the documented
  `BUILDMAX_*` variables.
- A task-run trace records the boundary actually resolved, and Portal can show
  a run's trace, artifacts, tools, usage, and failure cause on any storage
  backend.
- The managed LLM gateway supplies an operator catalog, aliases, a call ledger,
  streaming, and managed clients for CLI, TUI, Desktop, and task runs. Under
  `worker.llm.transport: buildmax` a worker holds no upstream provider
  credential, and every run is dispatched with a run token naming its user,
  team, and task — see [worker run token](../design/worker-run-token.md).
- The production reference documents Kubernetes, external MySQL, S3-compatible
  storage, rolling binary rollback, and its limits. It has not been proven
  against a real managed dependency pair, and workers do not yet have a bounded
  egress policy.
- Roles, authorization-matrix tests, and the first audit-trail slice exist, but
  task execution and several shared-automation changes are not yet represented
  by audit events.

The long-range versioned workspace is the intended product center, but its
first implementation would add state, restore, and concurrency risks before
the execution substrate that changes that state has earned operational trust.

## Goals

- Decide whether the next cross-cutting product milestone is a **trusted
  private execution loop**: one team can deploy, authenticate, submit work,
  execute it in a constrained Worker, inspect its outcome, and diagnose or
  recover common failure modes.
- Define the smallest evidence-backed path from the current partial pieces to
  that loop, without treating the Beta gate as a list of unrelated features.
- Make managed Worker inference the default candidate for the first deployment
  that wants centrally controlled models, while retaining direct local mode.
- Reduce the Worker network and credential surface enough that an operator can
  state what a task run could reach.
- Close the minimum operational and audit evidence needed to investigate one
  real task run.

## Non-Goals

- Replacing direct CLI or Desktop model configuration, or requiring a Server
  for local use.
- Designing a general policy language, approval platform, or a per-team
  execution-profile hierarchy before there is evidence it is required.
- Claiming that the OS sandbox, a strict cost ceiling, full SIEM export, SSO,
  or complete compliance retention is part of the first loop.
- Starting broad versioned-workspace implementation, a Git user interface, or
  a Desktop clone of Portal.
- Adding provider-specific adapters merely to increase the provider list.

## Options And Trade-Offs

| Option | Strength | Main concern |
|---|---|---|
| Continue independent P0.5, P3, and P4 tasks | Small, locally optimizable pull requests | The product can remain unable to prove an end-to-end team run; gaps between packages are easy to miss |
| Prioritize versioned workspace now | Advances the long-range differentiator | Builds durable state and restore semantics on an execution path that is not yet sufficiently constrained or operable |
| Prioritize Desktop, workflow, or provider breadth | Visible demos and more surface capability | Does not remove the adoption blocker for a private team evaluating deployment |
| Form a trusted private execution loop across P0.5, P3, and P4 | Directly tests the product promise and gives every security claim a concrete proof point | Requires sequencing work across runtime, worker, deployment, Portal, and governance |

The likely direction is the final option. It is a sequencing decision, not a
new product surface: work continues to land in the existing subsystem plans
and packages.

## Proposed Minimum Loop

The order below was dependency-driven. Step 1 has shipped; steps 2 and 3 are
deferred by an explicit decision recorded here, which leaves step 4 as the
remaining work.

1. **Adopt managed inference in the Worker.** *Done.* A task run reaches
   operator-approved models through the gateway and holds no provider
   credential. The server states the transport and alias in the run's
   worker-API response, so a process running model-chosen code never picks its
   own model. Every managed call is attributable to its task run, user, and
   team in the call ledger. Direct mode is unchanged and remains the default.
   Proven end to end on Compose and Kubernetes against a deterministic mock
   upstream.

2. **Constrain the Worker around that new shape.** *Deferred.* A
   default-deny egress policy that permits only the Server and the object store
   would stop a task run reaching the internet at all, and a coding agent needs
   that: `git` is not on the risky-prefix deny list, and `WebFetch` is
   unrestricted while the sandbox is off. Cutting both is a capability
   decision, not only a security one, and it needs a default allow-list nobody
   has evidence for yet. Narrowing the object-store credential is deferred with
   it, since both belong to the same boundary.

3. **Prove operability, rather than only configuration parsing.** *Partly
   deferred.* Exercising the recommended deployment against a real managed
   MySQL and object store, and rehearsing rollback or backup-restore across a
   schema change, both need infrastructure this project does not have. The part
   that does not — making a dependency failure diagnosable in the run that hit
   it — stays in scope and is described in step 4.

4. **Make the existing evidence reachable.** *Remaining work.* The records
   already exist; what is missing is the ability to reach and join them: no
   route reads the call ledger. Recording task execution as audit events was
   dropped from this step — see below. Retention and export remain separate
   decisions.

The OS-level worker sandbox, subagent isolation, richer local activity views,
and execution profiles remain valuable defense in depth. They should follow
the basic pod, credential, egress, and visibility boundary rather than be used
to defer it.

## What Step 4 Actually Covers

Stated concretely, because "evidence trail" is otherwise open to reading as
anything.

An operator holding one task run should be able to answer what it did, what it
spent, and who authorized it — reading only identifiers and enumerations, never
prompt text, tool arguments, or generated content. The ledger already excludes
bodies and an audit `Detail` is explicitly a short, non-sensitive note; that
constraint is what "safe identifiers" means.

The records mostly exist. What is uneven is reaching them:

| From | To | Link | State |
|---|---|---|---|
| task run | actor, trigger, timing, outcome | `task_run` columns | Present: `created_by`, `created_by_type`, `trigger_source`, `status`, timestamps |
| task run | managed call | `llm_call.task_run_id` | Recorded, with user, team, and task — but no route reads it |
| task run | artifact | `task_run_artifact` rows | Present and served |
| task run | trace | `task_run.trace_path` | Present and served on every storage backend |
| trace | managed call | — | Missing, and deferred: see below |

So the work is one route's worth, not a new record: read a run's managed calls
through team authorization rather than a database query.

### Why Task Execution Is Not An Audit Event

The earlier draft called for recording the task-run lifecycle in
`audit_event`. That was wrong, and the reason is worth keeping.

`task_run` already stores who triggered a run (`created_by`,
`created_by_type`), what triggered it (`trigger_source`, an eight-value
enumeration), when it started and ended, and how it finished. Writing the same
facts into `audit_event` adds no information. Volume is not the objection —
three rows per run against the ledger's dozens is noise — duplication is.

It also contradicts a rule this project already wrote down, in
[llm-gateway.md](../design/llm-gateway.md) §10: a governance audit log is for
configuration and authorization actions, and operational records belong
elsewhere. Task execution is an operational record.

The one thing an append-only log offers that a mutable `task_run` row does not
is proof that something happened at all, since `UpdateRun` overwrites. The
natural candidate — who cancelled someone else's run — does not exist: task runs
have no cancelled state, only `PENDING → SCHEDULED → RUNNING →
SUCCEEDED/FAILED`. If cancellation is added later, the actor that caused it is
an audit event; until then there is nothing here to record.

### Deferred From This Step

- **Turn-level correlation between a trace and the ledger.** A trace records
  each LLM turn but carries no call ID, and the gateway's response ID is
  discarded by the remote client because `core/llm.LLMClient` has nowhere to
  return it. Threading it through changes a domain contract for a correlation
  that run-level linking already approximates.
- **Making a dependency failure distinguishable in a run's record.** Readiness
  already reports which dependency is down; what is missing is that a run
  failing *because of* one is indistinguishable from a run whose model or task
  failed. One case is worse than indistinguishable: an artifact upload that
  fails is logged and skipped, so a run can report SUCCEEDED with its result
  missing from storage. That is a correctness bug rather than an evidence gap,
  and it belongs to whoever fixes it rather than to this loop.

Dependency diagnosis is narrower than it sounds, because readiness already
exists: `/readyz` probes the database and the object store, names the check
that failed, and keeps the reason — which carries DSNs and bucket names — in
the log rather than an unauthenticated response. What readiness does not answer
is why *one run* failed. The concrete gap is that a dependency failure is
currently indistinguishable from a model or task failure in the run's record,
and in one case is not recorded at all: an artifact upload that fails is logged
and skipped, so a run can report SUCCEEDED with its result missing from
storage.

## What Deferring Steps 2 And 3 Costs

Stating this plainly, because the loop was proposed as a whole and is no longer
being delivered as one.

A task run now holds a credential scoped to itself and, in managed mode, no
provider key. What it still has is unrestricted egress and a long-lived
object-store key. So the honest claim after step 4 is **an explainable private
execution loop**, not a trusted one: an operator can say what a run did and
what it spent, and cannot yet say what it could have reached.

That is a smaller claim than this proposal set out to make, and documentation
must not round it up. In particular, the Beta gate wording — "a task run holds
only the credentials that run needs, executes in a hardened pod with a bounded
network egress" — is not met by the deferred plan, and the gate should be
restated rather than quietly reinterpreted.

## Questions To Resolve

Open, and in scope for the remaining work:

- Which task state transitions, configuration changes, and credential changes
  must become audit events for the first investigation workflow?
- Should every managed Worker use the team's default alias, or should a
  workflow/task record an approved alias at creation time for reproducibility?
  A run may currently name any alias its team is granted.
- Does a trusted design-partner team validate this loop before versioned
  workspace work begins, and what evidence would change the sequence?

Parked with the deferred steps, and unanswered:

- What set of destinations does a task run legitimately need? A default-deny
  egress policy is only an improvement over unrestricted egress if the
  allow-list is grounded in what real runs do — package registries, Git hosts,
  and whatever a team configures — rather than assumed. Whether the boundary
  should be a NetworkPolicy alone, or a proxy enforcing the host allow-list the
  sandbox contract already models with `HostAllowed` and `ProxyAddress`, is part
  of the same question.
- Can the first default-deny policy be deployment-wide, or do operators already
  need team-specific profiles? The latter should require evidence, not
  assumption.
- What is the smallest server-issued or workload-identity design that removes
  long-lived object-store credentials from Worker Jobs without moving arbitrary
  file access into the Server?
- What recovery claim can BuildMax make before it has completed a live
  backup/restore and upgrade exercise? Until one is run, the answer is none.

## Evidence Needed For A Decision

Obtained:

- An end-to-end test in which a managed Worker has no upstream provider key,
  completes a task, and leaves a call-ledger record, artifact, and result.
  `./make compose smoke managed` and `./make kind smoke managed` run it in CI
  against a deterministic mock upstream, reading the ledger through the same
  team-authorized route an operator would. Audit evidence is **not** covered,
  and deliberately so: see why task execution is not an audit event.

Still needed for the remaining work:

- A pivot from one task run to its managed calls, artifacts, trace, and audit
  events that an operator can perform through team authorization rather than a
  database query.

Needed before the deferred steps could be reconsidered, and not obtainable
today:

- A threat model covering a malicious prompt and model-chosen shell command,
  including the credentials, network destinations, filesystem, and Kubernetes
  identity available to the Worker. This is the cheapest of the deferred items
  and the one that would make the egress allow-list an evidenced decision.
- A kind smoke test for default-deny Worker egress that still completes a task.
- An exercise on external MySQL and object storage, and a documented
  upgrade/rollback or backup/restore across a migration. Both need
  infrastructure this project does not have.
- Feedback from at least one prospective private operator on whether a
  deployment-wide boundary is sufficient and which identity path they require.

## Where This Lands

Step 1 already landed in the existing subsystem plans and packages, as intended:
[llm-gateway.md](../design/llm-gateway.md) and
[worker-run-token.md](../design/worker-run-token.md) hold the durable
rationale, and no parallel roadmap or "Beta platform" package was created.

What remains for this document is step 4, and then its own deletion: once audit
correlation ships, the sequencing decision belongs in
[ROADMAP.md](../ROADMAP.md) and the rationale in a design record, leaving
nothing here that is still an open question. The deferred boundary work does not
keep this proposal alive — it is a P0.5 and P3 concern with its own plans, and
the questions parked above should move there rather than sit in a paper whose
other decisions are made.

The Beta gate in [ROADMAP.md](../ROADMAP.md) claims a task run "executes in a
hardened pod with a bounded network egress". Deferring step 2 means that
sentence is not on a path to being true, and restating the gate is part of
accepting this rescope rather than a separate cleanup.
