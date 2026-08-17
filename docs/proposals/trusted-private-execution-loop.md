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
  a run's trace, artifacts, tools, usage, and failure cause.
- The managed LLM gateway supplies an operator catalog, aliases, a call ledger,
  streaming, and managed clients for CLI, TUI, and Desktop. Its worker endpoint
  exists, but `agentapp/taskrun` still receives a direct model entry, so the
  worker still needs an upstream provider credential.
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

The following order is intentionally dependency-driven.

1. **Adopt managed inference in the Worker.** Decide how a task run resolves a
   permitted alias, use the task-run-scoped gateway endpoint, and remove the
   upstream model credential from a managed Worker. Preserve direct mode as an
   explicit alternative. A managed call must be attributable to its task run
   in the call ledger.
2. **Constrain the Worker around that new shape.** Ship and test a
   deployment-level default-deny egress policy that permits only the Server and
   the object store needed by a normal managed run. Then decide how static
   object-store credentials are replaced or narrowed; a long-lived bucket key
   in a model-directed process remains a material exception.
3. **Prove operability, rather than only configuration parsing.** Exercise the
   recommended deployment with external MySQL and object storage, a task result
   visible in Portal, dependency-failure diagnostics, and a binary rollback or
   backup-restore procedure across a schema change. The existing absence of
   down migrations must remain explicit.
4. **Close the minimum evidence trail.** Record task execution and the next
   sensitive shared-automation changes, and link safe identifiers among task
   run, managed call, artifact/result, trace, and audit event. Retention and
   export remain separate decisions.

The OS-level worker sandbox, subagent isolation, richer local activity views,
and execution profiles remain valuable defense in depth. They should follow
the basic pod, credential, egress, and visibility boundary rather than be used
to defer it.

## Questions To Resolve

- Should every managed Worker use the team's default alias, or should a
  workflow/task record an approved alias at creation time for reproducibility?
- Can the first default-deny NetworkPolicy be deployment-wide, or do operators
  already need team-specific profiles? The latter should require evidence, not
  assumption.
- What is the smallest server-issued or workload-identity design that removes
  long-lived object-store credentials from Worker Jobs without moving arbitrary
  file access into the Server?
- Which task state transitions, configuration changes, and credential changes
  must become audit events for the first investigation workflow?
- What recovery claim can BuildMax make before it has completed a live
  backup/restore and upgrade exercise?
- Does a trusted design-partner team validate this loop before versioned
  workspace work begins, and what evidence would change the sequence?

## Evidence Needed For A Decision

- An end-to-end test in which a managed Worker has no upstream provider key,
  completes a task, and leaves a call-ledger record, trace, artifact, result,
  and audit evidence that Portal can retrieve through team authorization.
- A kind smoke test for default-deny Worker egress that still completes that
  task, followed by an exercise on external MySQL and object storage.
- A threat model covering a malicious prompt and model-chosen shell command,
  including the credentials, network destinations, filesystem, and Kubernetes
  identity available to the Worker.
- A documented upgrade/rollback or backup/restore exercise across a migration.
- Feedback from at least one prospective private operator on whether a
  deployment-wide boundary is sufficient and which identity path they require.

## Likely Destination If Accepted

Acceptance would update the sequencing in [ROADMAP.md](../ROADMAP.md) and the
P0.5, P3, and P4 design plans. It should produce bounded implementation Issues
for Worker managed inference, egress and object-storage credential scope,
deployment verification, and audit correlation. It should not create a new
parallel roadmap or a generic "Beta platform" package.
