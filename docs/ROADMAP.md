# BuildMax Roadmap

> **简体中文：** [阅读中文镜像](zh-CN/ROADMAP.md)
>
> **Audience:** users, operators, and contributors · **Status:** current — Alpha
> **Last reviewed:** 2026-09-08

BuildMax is an open-source Agent runtime for local work and private Space
deployment. CLI/TUI, Desktop, and Server/Portal use the same Go Agent Core.
You can use the local tools without deploying a Server.

**The next milestone is a dependable private-deployment Beta:** an operator can
deploy, run work, understand failures, and recover using documented procedures.
BuildMax has not passed that gate. No Beta release date is committed here;
release readiness depends on evidence, not the number of features implemented.

## At A Glance

| Horizon | User outcome | Current position |
|---|---|---|
| Available in Alpha | Run Agents locally or in a private Space, with managed models, background work, shared results, and diagnostic traces. | Implemented capabilities have different limits; see the [current-state assessment](current-state.md) and [user manual](../manual/introduction.md). |
| Next: private-deployment Beta | Trust the worker boundary, supported Server topology, persistence, and recovery procedures. | Engineering gaps and candidate operating evidence remain open. |
| Later: evidence-led expansion | Richer Workflows, integrations, and local experiences that solve demonstrated user problems. | Candidate directions, not release commitments. |

This roadmap owns priority, sequencing, and release gates. Implementation
evidence belongs in [current state](current-state.md), design rationale in
[design records](design/README.md), and release proof in the
[Beta readiness record](deploy/beta-readiness.md). “Implemented” does not mean
qualified in a real deployment. Earlier P0–P4 phase names in design records are
historical capability groupings; the R0–R5 order below governs current work.

## Active Priority Order

R0–R2 come first because execution safety and state correctness underpin every
Server feature. R3–R4 complete the operating and qualification evidence. These
are priorities, not claims that someone is currently assigned to every item.

### R0. Contain Unattended Worker Execution

**Partly implemented.** Official worker images select the worker sandbox
baseline; Bash confinement, process limits, hook transport policy, and backend
self-tests are implemented and exercised by deployment smoke. MCP stdio child
processes and cluster-level network egress remain outside that boundary.
The worker API already has separate listeners, TLS support, and a shipped
Server-ingress NetworkPolicy; worker-wide egress is a separate gap.

**Next:** define and enforce the MCP child-process boundary; verify the
shipped worker API network boundary in the candidate environment; decide the
wider worker egress policy.
Make resolved sandbox policy understandable in the operator surfaces.

**Done when:** the supported worker profile enforces its documented process and
network boundaries, fails closed when required enforcement is unavailable,
and has deployment evidence for those claims. The optional gVisor profile
requires qualification with the actual worker and sandbox probe before it is
supported or recommended.

Design: [trust harness](design/trust-harness.md),
[worker API network boundary](design/worker-api-network-boundary.md), and
[gVisor worker runtime](design/gvisor-worker-runtime.md).

### R1. Qualify Shared Server Coordination

**Mechanism implemented; qualification open.** Local mode supports one Server;
Redis mode supplies shared streams, connection events, and Conversation turn
leases. Basic/kind and production manifests now use Redis with two replicas,
and architecture tests reject multiple replicas without coordination.

**Next:** enforce lease fencing tokens in message-history writes and exercise
worker updates, reconnects, concurrent turns, and Redis failures in a deployed
candidate. Do not count an in-process two-replica test as a cluster exercise.

**Done when:** the supported topology has candidate evidence for live delivery,
turn serialization, stale-writer protection, and recovery under failure.

Design: [Server coordination](design/server-coordination.md).

### R2. Widen Real-Database And Recovery Evidence

**Partly implemented.** A MySQL integration scope runs on pull requests and
covers critical authorization and TaskRun transitions, including contention.
The remaining work is case coverage, not introducing the CI gate.

**Next:** extend the existing retry, checkpoint, Artifact retention, and
Space-isolation tests with remaining Workflow revision advancement, restart
recovery, and cross-Space scenarios. Retry lineage and Artifact tombstoning
already have real-database tests. Retire test plans for removed mechanisms, including the old
result-delivery queue, rather than recreate them for a checklist.

**Done when:** the critical persistence and recovery paths have real-database
regression tests and deployment failure evidence. Schema upgrade and rollback
proof must exercise the existing migrations against an older schema and verify
the candidate’s rollback limits; the migration list is no longer empty.

Design: [verification program](design/verification-program.md) and
[end-to-end testing](design/end-to-end-testing.md).

### R3. Validate Account And Space Operator Journeys

**Core lifecycle implemented; operating evidence open.** Account bootstrap,
login-code recovery, Space invitations, role changes, ownership transfer, and
member-scoped recovery exist. Creating an account remains a system administrator
authority. Space approval workflows are intentionally out of scope.

**Next:** have an operator exercise these journeys through the documented UI
and CLI, identify friction or missing audit evidence, and fix demonstrated gaps. Track
transactional authority audit, admin CLI Session parity, quota-tier assignment,
and runtime diagnosis metadata in the
[administration operations proposal](proposals/system-administration-operations.md).

**Done when:** an operator can onboard people, manage membership, transfer
ownership, and recover access without reading code or bypassing authorization.
Reopen account policy or approval workflow decisions only for a concrete need.

Design: [Space membership lifecycle](design/space-membership-lifecycle.md) and
[Space governance](design/space-governance.md).

### R4. Expand Qualification Breadth

**Framework implemented; coverage limited.** Three BuildMax-owned tasks and a
one-task external canary establish the evaluation path, not platform-wide
reliability or a Terminal-Bench score.

**Next:** expand representative local, worker, Conversation, trust-boundary,
failure-recovery, and deployment scenarios. Collect performance and soak
evidence separately. Run the pinned Harbor canary before the full benchmark
protocol; publish a score only with the completed protocol and its conditions.

**Done when:** a release candidate has representative, reproducible results
across the supported surfaces, with failures and limits reported explicitly.

Design: [evaluation system](design/evaluation-system.md).

### R5. Deepen Product Capability From Evidence

**Later; scope depends on demand and qualification results.** Candidate work
includes durable Workflow reconciliation and typed dataflow, real channel
adapters, executable Space plugins, Portal performance, Desktop automation,
and throughput. Local CLI/TUI and Desktop improvements remain welcome when they
address concrete problems; the Beta focus does not make Portal the only product.

Workflow expansion starts with reconciliation and typed dataflow before graph
breadth. A provider-neutral structured-output contract in the shared runtime is
a prerequisite for typed routes, planners, evaluators, and richer Task results.
Channel names or partial adapters do not count as delivered integrations.

Design: [Workflow runtime](design/workflow-runtime.md) and
[orchestration and continuity decisions](design/orchestration-and-continuity-decisions.md).

## Beta Gate

The first Beta targets **one trusted Space on a private network**. It is not a
claim of public multi-tenant readiness. Qualification uses the same immutable
Server, worker, and Portal artifacts proposed for release.

| Required proof | Acceptance outcome |
|---|---|
| Candidate deployment | Deploy pinned image digests with external MySQL, S3, and TLS; record versions, configuration, operator, and date. |
| Execution boundary and topology | Prove the supported sandbox, resource limits, hook/MCP treatment, and Server topology. Unrestricted Bash with a recorded `none` boundary does not pass. Record residual worker egress and storage-credential limits explicitly. |
| Persistence and failure behavior | Attach passing critical MySQL tests; exercise cancellation, worker loss, database outage, and storage denial. Runs reach documented terminal states and retain available results and diagnostic evidence. |
| Recovery and maintenance | Restore the database and bucket together; exercise a schema upgrade and binary rollback, plus credential rotation. Record recovery time, data checks, and accepted loss. |
| Operator journey | An operator who did not implement the feature can sign in, execute and retry work with a managed model, and diagnose results from TaskRun, Artifacts, traces, usage, and audit history. |
| Release verification | Attach current CI, direct and managed Compose/kind smoke, Portal browser E2E, archive verification, image scans, SBOMs, and provenance. |

Engineering closes the execution and topology gaps first, then widens
persistence and negative deployment tests. External candidate qualification
follows, ending with a signed readiness record. Account journeys and evaluation
coverage can progress alongside this work.

The [Beta readiness record](deploy/beta-readiness.md) holds the detailed
procedure and evidence. Passing unit tests or local smoke does not replace
candidate restore, failure, and upgrade exercises. Desktop polish, SSO,
executable Space plugin content, additional providers, and general durable
Session sync are outside the first Beta gate.

## How To Help

Start with [CONTRIBUTING.md](../CONTRIBUTING.md) and the
[testing guide](contribute/testing.md). You do not need to tackle an entire
priority to make a useful contribution.

| If you want to… | A useful contribution |
|---|---|
| Make a first contribution | Follow a local setup or operator journey and improve unclear documentation; browse [good first issues](https://github.com/gougoujiang/buildmax/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22). |
| Improve reliability | Reproduce a failure and add a focused regression test, especially for the R1–R2 state and recovery paths. |
| Help qualify private deployment | Run a documented deployment journey and report versions, topology, expected/actual behavior, and redacted evidence. |
| Shape a feature | Describe the user problem, a concrete example, and why existing behavior is insufficient in [Discussions](https://github.com/gougoujiang/buildmax/discussions). |

Search [existing issues](https://github.com/gougoujiang/buildmax/issues) before
opening a bug or implementation proposal. For substantial work, link the
relevant R priority and design record and discuss scope before implementing it.
An entry here does not imply an assigned owner or an open implementation issue.

Maintainers should update this page when a priority, completion criterion, or
release gate changes, and keep the Chinese mirror in sync. Routine implementation
details belong in the linked evidence and issue, rather than growing this page
into another implementation inventory.
