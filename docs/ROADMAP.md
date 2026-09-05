# BuildMax Roadmap

> **Audience:** maintainers and contributors · **Status:** current

## Product Promise

BuildMax is an out-of-the-box, privately deployable enterprise Agent platform.

It is built around one shared Go Agent Core:

- local single-user execution through CLI/TUI and Desktop
- enterprise/team operation through Server and Portal
- background execution through worker task runs

This is not a choice between a local AI file assistant and a team AI workspace.
Users can use only the local surfaces, deploy the Portal for a company, or use
both together. The core rule is that important Agent capability belongs in the
shared runtime first, then each surface exposes it in the way that fits its job.

## Roadmap Principle

Plan by platform maturity, not by piling features onto one surface.

This is a status-bearing document, not a record of intended work. "Shipped"
means the behavior is present in the repository and covered by its relevant
automated tests. It does **not** mean that a real customer deployment, upgrade,
or recovery exercise has happened; those are called out separately as operating
evidence. When the code and this document disagree, update this document.

The [current-state assessment](current-state.md) records the code evidence,
maturity judgment, and gaps behind this roadmap. Keep detailed audit evidence
there. This document owns only priority, sequencing, and release gates.

The near-term goal is:

> A company can privately deploy BuildMax and immediately use the same Agent
> Core for local execution, team collaboration, background work, result
> delivery, and basic governance.

## Active Priority Order

The current milestone is operational trust, not another broad feature family.
Work proceeds in this order unless new deployment evidence justifies changing
it.

### R0. Contain Unattended Worker Execution

Mostly closed. A worker run selects the stricter surface, `bwrap` actually
confines its Bash commands on both the `k8s_job` and `local_process` paths, and
the deployment smoke's own probe proves it organically rather than by
inspection. What is left is a child process the boundary does not reach and a
network boundary that was never in it.

Required outcomes:

- every worker run records and enforces an explicit worker sandbox surface —
  **done**: `internal/agentapp/taskrun` passes `config.WorkerSandboxSurface()`,
  and the resolved tiers are pinned onto the run for audit;
- absence of the OS backend either fails closed or follows one documented,
  visible downgrade policy — **done**: the strict baseline is selected only
  where `BUILDMAX_SANDBOX_BACKEND_INSTALLED` marks the backend as installed,
  and a `Manager` probes its backend with a real confined command before
  trusting it, so one that cannot enforce anything fails closed;
- process and resource limits are enforced and tested — **done**;
- hook and MCP child processes have an explicit boundary of their own —
  **half**: the `command` and `http` hook transports go through the same
  wrapper and host policy `Bash` and `WebFetch` use; MCP does not.
  `internal/infra/mcp/transport.go` execs a stdio server directly, with no
  sandbox involvement anywhere in that package;
- a Beta candidate does not run unrestricted worker Bash merely because it is
  inside a Kubernetes pod — **done**.

Still open beyond that list: the cluster-level `NetworkPolicy` question
[`design/trust-harness.md`](design/trust-harness.md) §3.9 leaves open — a
worker pod reaches whatever the cluster's network allows, independent of the
in-process sandbox — `buildmax sandbox overrides`, and surfacing a run's
resolved tiers in Portal's task-run detail view rather than only in the API
response and audit trail.

The Server control channel is the first bounded network slice: separate the
public and worker listeners, keep worker routes off the public mux, encrypt the
Pod-to-Server path, and admit only worker Pods to its internal port. The
accepted direction and its limits are in
[`design/worker-api-network-boundary.md`](design/worker-api-network-boundary.md).
It does not close the wider domain-aware worker egress question above.

The Pod-to-host boundary has a separate qualified direction: support an
operator-selected, fail-closed gVisor RuntimeClass around the complete worker
while retaining `bwrap` for command-to-worker policy. The exact BuildMax worker
and sandbox probe must pass under `runsc` before this becomes a supported or
recommended production profile; see
[`design/gvisor-worker-runtime.md`](design/gvisor-worker-runtime.md).

### R1. Make Multi-Instance Semantics Correct Or Declare One Replica

The production manifest requests two Server replicas, while stream fan-out,
WebSocket connection registration, and conversation turn queues are held in
process memory.

Required outcomes:

- immediately make one Server replica the supported production topology; or
- add shared pub/sub, delivery, and distributed conversation serialization
  before advertising horizontal Server scaling;
- test a worker update, browser session, reconnect, and concurrent turns across
  the supported topology.

### R2. Put Persistence In The Pull-Request Evidence Path

The scope exists and runs: `./make test mysql` requires a DSN rather than
skipping without one, runs on a database it creates and drops, fails if a test
in the scope skips for the DSN's absence, and a pinned `mysql:8.0` service
container runs it on every pull request. It found a defect on its first run
that had been on `main` for 387 commits.

Required outcomes:

- run a hermetic MySQL integration scope in CI for critical store and migration
  behavior — **done**;
- cover authorization-bearing and run-state transitions against the real
  database — **largely**: task-run transitions, claiming, system grants, plugin
  activation, and the team invitation and ownership-transfer lifecycle are
  covered, and the four conditional-UPDATE claims that decide whether one
  caller wins — task claiming, run transition, result-delivery claiming, and
  cancellation beside a report — are now tested under contention and checked by
  mutation. Retry attempts, workflow revision advancement, restart recovery,
  cross-team store lookups, and artifact tombstoning remain; see
  [`design/verification-program.md`](design/verification-program.md) §4.2,
  which also records why the N-1 fixture is blocked and the quota bullet
  withdrawn;
- retain broader Compose, kind, failure, restore, and upgrade drills as
  deployment evidence rather than forcing every one into every pull request —
  unchanged, and deliberately so.

### R3. Close Account And Team Operations

Account creation, credential issuance, team invitation, role promotion,
ownership transfer, access recovery, and team approvals must form complete,
audited operator journeys. Existing authentication, role checks, system
administration, quota, and audit code are the foundation, not the finished
operation.

### R4. Expand Qualification Breadth

The evaluation framework is implemented, but three BuildMax-owned tasks and a
one-task external canary do not qualify the platform. Expand representative
local, worker, conversation, trust, failure-recovery, and deployment suites;
add performance and soak evidence separately. Do not publish a Terminal-Bench
claim until the pinned protocol has actually run.

### R5. Deepen Product Capability From Evidence

After R0–R4, deepen the durable Workflow runtime selected in
[the design record](design/workflow-runtime.md), or choose real channel
adapters, executable team plugins, Portal performance, Desktop automation, or
throughput work from observed user and qualification evidence. Workflow work
starts with reconciliation and typed dataflow before graph breadth. Do not let
the existence of names, types, or partial adapters count as a shipped product
surface.

## Existing Capability Baseline

The phase labels below describe capability already built and the acceptance
criteria those surfaces remain subject to. They are retained as a compact
baseline for existing design records; they do not override the active order
above. "Complete" means the scoped capability landed, not that production
readiness is complete.

### P0. Agent Core Stability — complete

This was the highest priority because CLI, Desktop, worker execution, and Portal
all depend on it.

Focus:

- context-window and token-budget behavior
- reliable tool-calling error recovery
- consistent MCP, skills, and subagent behavior across CLI, Desktop, and worker
- safer file reading, editing, bash, grep, and glob behavior
- run statistics, logs, traces, and tool-call summaries

Acceptance:

- the same task has comparable capability in CLI, Desktop, and worker execution
- differences come from environment and permissions, not separate Agent implementations

### P1. Local Agent Experience — complete

CLI and Desktop are the direct expression of what one Agent can do for one user.
They are not secondary to Portal.

Focus:

- CLI/TUI slash commands, session handling, model visibility, and tool visibility
- Desktop project/workspace picker, session management, streaming polish
- local output and artifact viewing
- local file and diff awareness
- local model, MCP, skill, and tool settings

Acceptance:

- a user can get a complete useful Agent experience without deploying Portal

### P2. Portal Outcome Surface — complete

Portal already has issues, workflows, tasks, runs, and artifacts. The next step
is to make results the first-class user surface.

Focus:

- issue-level Results / Outputs section
- conversation-visible result cards and artifact links
- lightweight Markdown/text previews
- stable `latest_result` / `outputs[]` aggregation shape
- task/run/step pages become drill-down views, not the main result surface

Acceptance:

- opening an issue makes it obvious what was produced, without reading raw run or step internals

Both surfaces are built. `issue_outputs.go` serves the aggregation, the API
returns `latest_result`, and `IssueDetail.tsx` renders it. A Conversation now
carries a card per task — status, output, files, run details, stop and run again
— ordered against the messages by creation time and read from the database, so
the cards survive a refresh, a dropped socket, and a summary that never arrives.
The transcript excludes the system channel, so a `[Task Result]` message is no
longer drawn as the user's own.

The forced Tier 1 summary delivery is gone. A finished run no longer enqueues a
presentation attempt (`task_result_delivery` and its retry sweep were removed
along with the code that replayed a `[Task Result]` message into the
conversation); a terminal run now only broadcasts an invalidation
(`task.status.changed`) to connected clients, and the Conversation task card
reads `task_run` directly. `task_run` was always authoritative for the
result — this removed the obligation that a foreground model call had to
succeed, or even run, before that result was durable or visible. Direct Agent
Tasks require no Conversation at all. See
[Agent execution and Task threads](design/agent-execution-and-task-threads.md).

What was deliberately not done, and why, is in the [Portal execution
design](design/portal-execution-model.md).

### P0.5. Agent Core Trust Harness — partly shipped

After Portal outcomes are visible, return to the shared Agent Core and close the
trust gaps that separate a working agent from a serious execution harness.

Focus:

- sandbox and execution boundaries for filesystem, network, env, and process behavior
- runtime hooks for approvals, tools, file changes, compaction, and run outcome
- durable run traces with redaction, bounded tool output, usage, and latency
- scoped memory and instruction loading across user, workspace, team, agent, and session
- TUI/Desktop activity views and local diagnostics
- local background jobs and monitors shared by TUI and Desktop
  (see [design/local-background-jobs.md](design/local-background-jobs.md))
- subagent trace linkage and optional isolation groundwork
- safer non-interactive worker execution

Code state:

- shipped: hook configuration and transports, tool permissions, local OS
  sandboxing, bounded redacted traces, session notes/todos and compaction
  checkpoints, local background jobs, subagent trace parents, and the Portal
  run-trace view;
- shipped: one shared CLI/TUI/Desktop Project identity and bounded
  cross-session Project Memory
  ([design/local-project-memory.md](design/local-project-memory.md)). A Project
  is one Git repository including its worktrees, or one directory; the session
  pickers and session clearing select by it rather than by folder path,
  `--continue` selects within the Workspace with `--project` to widen, and
  Desktop's private `projects.json` is gone. Memory is a set of small Markdown
  files, one per memory, with a generated index; only the index is resident,
  bodies are read on demand, a replacement requires having read it, and
  `--no-project-memory` withdraws index and tools together. `buildmax project`
  lists and relinks. The `context_sources` trace record replaces `prompt_layers`
  and names every source a run was assembled from by its own kind; `buildmax
  doctor` reports the Project, the memory count and index size, skipped memory
  files, and detached sessions;
- still absent: a worker selecting `SandboxSurfaceWorker`, process rlimits,
  sandboxing of command/HTTP hook transports, trace retention, typed
  command-level boundary, file-change, hook, approval, retry, and failure-cause
  records, and the Project Memory surface work — a Desktop memory list and
  editor, a CLI inspection command beyond `doctor`, the user-invoked
  session-review command of the design's phase 2, and the usage evidence that
  would justify raising the memory count, ranking the index, or promoting
  memories automatically;
- deliberately not covered by the local Project plan: global user memory,
  team memory, Portal/worker memory, semantic retrieval, and automatic memory
  extraction.

Acceptance:

- users can inspect and explain Agent runs without leaving the local surfaces
- local and worker sandbox boundaries are explicit and visible
- worker runs produce enough trace data for Portal diagnostics
- memory sources are visible, scoped, and user-controllable
- local and worker runtime differences are explicit, not hidden in surface-specific code

Worker execution containment is now a Beta gate. A `k8s_job` worker runs in a
constrained Kubernetes pod and reports that it is unsandboxed; it does **not**
receive the stricter in-process sandbox baseline. `local_process` remains one
trust domain with the Server. The candidate must wire and prove the worker
boundary, or disable unrestricted Bash on that path; recording an unavailable
boundary is evidence of the gap, not containment.

### P0.6. Evaluation And Qualification System

BuildMax needs evidence for the capability, reliability, trust, and product
outcome claims made across the shared runtime and its surfaces. This replaces
the early coding benchmark rather than extending its formats.

Focus:

- a BuildMax-owned, versioned contract for tasks, subjects, trial bundles,
  grader results, experiments, and qualification reports
- black-box evaluation of built binaries and deployment artifacts across local,
  worker, conversation, and deployment execution
- product-owned capability, reliability, trust/control, and product-outcome
  suites, reported separately rather than collapsed into a global score
- repeated and paired trials, explicit uncertainty, and separate Agent,
  grader, and infrastructure failure classes
- private-by-default trial data, an access-controlled or rotating holdout, and
  explicit bounded export
- maintainer regression workflows and operator model/config/deployment
  qualification
- replaceable framework adapters: Inspect or a thin controller for experiments,
  Harbor for container/public-benchmark execution, Terminal-Bench 2.1 as the
  first external capability coordinate, and optional viewers

Acceptance:

- one representative black-box slice runs local, worker, conversation, and
  trust-boundary scenarios against built artifacts
- a failed trial yields a subject manifest, trace, final-state evidence,
  classification, and bounded reproduction path
- maintainers can compare a baseline and candidate with repetitions and
  uncertainty; operators can qualify a model, configuration, or deployment in
  their own environment
- no private prompt, trace, workspace snapshot, or grader body must leave the
  owning environment
- Harbor can run the built BuildMax Agent against a pinned Terminal-Bench 2.1
  release, preserve one BuildMax trial bundle per attempt, and compare harnesses
  under the same model, effort, resources, and attempt count — **partly met**:
  the oracle smoke and a one-task canary have run end to end and imported, so
  the path works for one task. The canary subset is pinned in
  `evaluation/harbor/pins.json` and selectable with `--canary`; the criterion
  needs that subset run, and then the full protocol
- the legacy `eval/` catalog and `internal/agenteval` are retired rather than
  preserved behind compatibility code — **done**: both are deleted, and
  `./make eval` now measures the built CLI against the CLI tasks in
  `evaluation/suite/`; worker tasks are selected explicitly

The black-box vertical slice is enabling work before substantial new Agent
capability. Framework selection is deliberately downstream of that slice; see
[design/evaluation-system.md](design/evaluation-system.md).

Code state: **partly shipped**. `evaluation/contract`, the black-box CLI and
worker adapters, deterministic/command/trace graders, preflight, repeated and
paired experiments, and three representative tasks are implemented.
`tools/eval` is the entry point for that contract; the old `eval/`
catalog and `internal/agenteval` are deleted.

`evaluation/harbor` adds the external Terminal-Bench 2.1 target: pinned harness,
dataset ref, and adapter versions; the Python custom-Agent that uploads the
built CLI into a task container; the importer that files a finished job as trial
bundles; `./make doctor harbor` and `./make eval harbor`. The oracle smoke
passed 5/5 and a one-task canary ran through the adapter and imported cleanly,
so the path is verified for one task and no further. There is **no
Terminal-Bench score**, and running it found one product bug — a Bash command
that left a background process behind hung the agent indefinitely — which is
fixed. Expect the first wider run to find more.

Conversation and deployment adapters, model-grader calibration, a private or
rotating holdout, and the Inspect spike remain open.

### P3. Enterprise Deployment Loop — implementation mostly shipped; operating evidence open

The product promise depends on private deployment being boring and repeatable.

Focus:

- recommended private deployment path for server, worker, Portal, MySQL, and MinIO/S3
- synchronized server config, storage config, and deployment docs
- clear startup errors and health checks
- Docker/kind/k8s path that runs end to end
- default admin/user/team/quota/model initialization story
- optional managed LLM connection mode, so a deployment can supply approved
  models without distributing provider credentials to users and workers —
  shipped for CLI, TUI, Desktop, and task runs, none of which hold a provider
  key. A task run reaches it with a per-run credential; an interactive client
  reaches it with the session its user signed in with
- an operator model catalog behind the shared LLM contract, with per-call usage
  recorded before any spending limit is claimed — the catalog and call ledger
  exist; catalog names and availability are deployment-wide, and the withdrawn
  per-team alias layer must not be described as current
  (see [design/client-modes.md](design/client-modes.md))
- an orderly stop: a restart or a rolling upgrade drains connections, stops
  claiming runs, and lets an interrupted run report what happened instead of
  sitting in `RUNNING` until the stale-run reaper closes it
  (see [design/graceful-shutdown.md](design/graceful-shutdown.md))

Code state:

- shipped: the production reference manifest, the local Compose and kind
  deployment paths, `/healthz` plus dependency-aware `/readyz`, database schema
  migrations, operator `user` and `admin` commands, System Administration UI,
  managed inference for local clients and workers, per-run worker tokens, an
  ordered shutdown across server, scheduler, and worker, and
  post-merge/scheduled Compose and kind smoke workflows;
- the smoke paths exercise account bootstrap, login, team authorization,
  worker execution, artifacts, retry, managed inference, the call ledger, and
  Portal browser views;
- still unproven or incomplete: a deployment against real external MySQL/S3 and
  TLS, backup/restore and schema-upgrade exercises, deployment-level
  cancellation and worker-failure recovery, worker-launch and LLM-config
  readiness checks, credential rotation, and a supported dependency-version
  matrix.

Acceptance:

- a new environment can reach login, create work, run a worker task, and view the result without reading code
- a deployment can serve approved models to CLI, Desktop, and worker runs without distributing provider keys, while direct mode still runs with no server

### P4. Team Governance Foundation — first slice shipped

Keep this practical. The near-term need is basic enterprise confidence, not a
full policy platform.

Focus:

- team-scoped quota UI and documentation
- role/permission boundary tests
- clear workflow lifecycle UI and copy for draft/published/archived
- design the smallest audit/event model
- make sensitive assets traceable over time: webhook keys, agent definitions, workflows
- a deployment-scoped System Administrator, separate from every Team role, so
  account lifecycle, access recovery, system status, and cross-team audit stop
  requiring database or cluster credentials
  (see [design/system-administration.md](design/system-administration.md))

Acceptance:

- admins understand who can do what, what resources are used, and what state shared automation is in
- an operator runs routine account and deployment work through an audited surface rather than through the database

Code state: role-route matrix tests, quota visibility/enforcement, workflow
lifecycle, audit retention/export, audit UI, System Administrator grants and
administration routes are implemented. Audit-to-run correlation and a broader
set of audited actions remain follow-ups; neither should be presented as a
missing Beta prerequisite.

## Beta Gate

Alpha to Beta is not more Agent capability. It is an **operating proof** for one
trusted team, performed with the immutable artifacts proposed for release. Code
and automated tests establish that a proof is worth attempting; they do not
substitute for a restore, failure drill, or upgrade in the target environment.

Beta is reached only after all entry checks below pass and their exact evidence
is recorded in [deploy/beta-readiness.md](deploy/beta-readiness.md):

> An operator can deploy pinned BuildMax server, worker, and Portal artifacts to
> a private Kubernetes environment backed by external MySQL, S3, and TLS; sign
> in; execute and retry work with an approved managed model; diagnose the result
> from the TaskRun, artifacts, trace, managed-call ledger, and audit history;
> and recover predictably from cancellation, worker loss, dependency outage,
> restore, credential rotation, and an upgrade rollback.

| Entry proof | What the repository provides | Evidence required before Beta |
|---|---|---|
| Candidate deployment | Production manifest, migration ledger, `/readyz`, account bootstrap, managed worker inference, and deterministic Compose/kind smoke are implemented. | Deploy immutable candidate image digests against real external MySQL and S3 over TLS. Record the cluster and dependency versions, image digests, configuration, operator, and date. |
| Constrained execution | Per-run JWT, minimized Job environment, read-only/capability-dropped pod (root, not non-root — see `docs/reference/configuration.md`), no service-account token, required CPU/memory bounds, an explicit trace boundary, and the worker's own `SandboxSurfaceWorker` selection with `bwrap`-confined Bash calls are implemented and organically verified by the deployment smoke's own probe. | Prove process/resource limits and hook/MCP child-process treatment against the deployed candidate, not only the smoke probe. Unrestricted Bash with a recorded `none` boundary does not pass. |
| Server topology | Durable state is shared through MySQL, but live stream fan-out, WebSocket connections, and conversation turn queues are process-local while the reference manifest requests two Server replicas. | Run exactly one Server replica, or implement and prove shared delivery plus distributed conversation serialization. Exercise cross-instance worker updates, reconnects, and concurrent turns if multiple replicas are claimed. |
| Persistence gate | `./make test mysql` runs the store scope against a pinned MySQL service container on every pull request, refusing to skip for an absent DSN. Its case list is still narrower than [`design/verification-program.md`](design/verification-program.md) §4.2 asks for. | Attach a passing hermetic MySQL CI scope for critical schema, query, authorization, and state-transition behavior, then repeat the candidate deployment proof against its external database. |
| Failure behavior | Cancellation, interrupted-run reporting, liveness heartbeats, lost-worker reaping, partial artifact retention, and explicit retry exist with focused tests. | In the deployed candidate, cancel a running run, kill a worker without a graceful report, interrupt database access, and deny object-storage access. Prove each run reaches the documented terminal state, retains the available evidence, and can recover or be retried without an ambiguous or dangling result. |
| Recovery and maintenance | Forward migrations, an N-1 binary compatibility rule, environment-injected credentials, and operator-visible readiness and status surfaces exist. | Restore the database and bucket as a pair, exercise an upgrade containing a schema change followed by binary rollback, and perform the documented drain/restart credential-rotation procedure. Record recovery time, data checks, and any accepted loss. |
| Operator diagnosis and governance | Portal exposes the run result, stored trace, artifacts, managed-call usage, quota, audit history, and System Administration; authorization and retention/export paths are tested. | Have an operator who did not implement the feature perform the full journey and failure drills using documented surfaces. Record whether logs, `/readyz`, System Status, TaskRun/artifact state, trace, managed-call ledger, and audit are enough to explain every outcome. |

Every candidate must also attach current results for `./make check ci`, the
direct and managed Compose/kind deployment smoke, Portal browser E2E, release
archive verification, image vulnerability scans, SBOMs, and provenance
attestations. These are **per-release evidence**, not one-time substitutes for
the entry proof above.

The first Beta still accepts explicit limits. It is for a trusted team on a
private network, not direct public exposure. Worker egress and storage
credentials remain operator-owned threat-model decisions until an enforced
network and credential boundary ships. The readiness record must repeat these
limits. Unlike Alpha, however, Beta does not accept an accidentally inherited
CLI sandbox baseline or multi-replica semantics that the Server cannot enforce.

Deliberately outside the Beta gate: Desktop polish, SSO, executable team plugin
content, additional model providers, and general durable Session sync. The
instruction half of team plugin distribution — a team activating skill and
subagent releases, and a worker materializing exactly what it pinned — is
implemented; releases contributing hooks or MCP servers cannot be activated.

## Beta Execution Order

The active priorities define engineering order. The Beta proof then closes in
this sequence:

1. **Contain worker execution and make topology honest.** Wire and test the
   worker boundary. Change the supported manifest to one Server replica unless
   shared coordination lands first.
2. **Add real persistence evidence to CI.** The gate runs and the contention
   cases are written; what remains is retry, workflow revision, restart
   recovery, and artifact tombstoning per
   [`design/verification-program.md`](design/verification-program.md) §4.2.
3. **Complete negative deployment smoke.** Cancellation is covered. Add hard
   worker loss, database unavailability, and object-storage denial, asserting
   terminal state and retained evidence rather than only an error response; see
   [design/end-to-end-testing.md](design/end-to-end-testing.md) §6.2.
4. **Qualify immutable candidate images externally.** Use external MySQL, S3,
   and TLS for the operator journey, paired restore, schema upgrade and binary
   rollback, and credential rotation. Record exact artifacts and evidence in
   [deploy/beta-readiness.md](deploy/beta-readiness.md).
5. **Close the candidate record.** Attach current CI, direct and managed
   Compose/kind smoke, Portal E2E, release archive verification, image scan,
   SBOM, provenance, and every required readiness artifact.
6. **Widen qualification in parallel after the safety gates are explicit.** Run
   the pinned Harbor canary and then the full protocol, and expand BuildMax's
   product-owned conversation, deployment, trust, and recovery tasks. A
   one-task canary proves the adapter path, not a product score.

Account/team closure can proceed alongside steps 2–4 when it does not distract
from the execution boundary. New workflow, channel, plugin, or local-session
features wait for evidence from these steps or a concrete deployment partner.

## Avoid For Now

- a large workflow engine rewrite before results and runtime stability improve
- a generic policy platform before the concrete team approval journey is clear
- Desktop duplicating Portal issue/workflow/team administration
- a full Git restore UI before the outcome and change model is clear
- any Portal-only Agent capability that bypasses the shared runtime

## Related Documents

- [../README.md](../README.md) — current system overview
- [current-state.md](current-state.md) — code-based implementation and readiness assessment
- [design/README.md](design/README.md) — design document index
- [design/product-vision.md](design/product-vision.md) — long-range AI-native workspace vision
- [design/surface-positioning.md](design/surface-positioning.md) — product surface positioning
- [design/trust-harness.md](design/trust-harness.md) — P0.5 Agent Core trust harness design
- [design/evaluation-system.md](design/evaluation-system.md) — P0.6 evaluation and qualification design
- [design/verification-program.md](design/verification-program.md) — R0–R4 verification matrix, persistence gate, failure evidence, and release rehearsal
- [design/context-durability.md](design/context-durability.md) — P0.5 instructions and session notes that survive compaction
- [design/local-project-memory.md](design/local-project-memory.md) — shared CLI/Desktop Project identity and bounded cross-session Project Memory
- [design/local-background-jobs.md](design/local-background-jobs.md) — P0.5 local background jobs and monitors for TUI and Desktop
- [design/workspace-root-and-worktrees.md](design/workspace-root-and-worktrees.md) — a session that moves its own workspace root into a worktree
- [design/enterprise-deployment.md](design/enterprise-deployment.md) — P3 Enterprise deployment design
- [design/llm-gateway.md](design/llm-gateway.md) — P3 Managed LLM gateway design
- [design/graceful-shutdown.md](design/graceful-shutdown.md) — P3 shutdown ladder for server, scheduler, and worker
- [design/team-governance.md](design/team-governance.md) — P4 Team governance design
- [design/system-administration.md](design/system-administration.md) — P4 Deployment-scoped system administration design
- [design/team-membership-lifecycle.md](design/team-membership-lifecycle.md) — R3 team invitation, role change, ownership transfer, and member-scoped access recovery
