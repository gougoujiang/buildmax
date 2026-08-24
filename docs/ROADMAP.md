# BuildMax Roadmap

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

The near-term goal is:

> A company can privately deploy BuildMax and immediately use the same Agent
> Core for local execution, team collaboration, background work, result
> delivery, and basic governance.

## Near-Term Priorities

P0, P1, and P2 are **complete**. Active work starts at
P0.5, P0.6, and P3. The completed sections are kept because their focus and
acceptance criteria are the standard the surfaces are held to, not because the
work is outstanding.

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

Delivery of the summary is durable too: the report a finished run owes its
conversation is recorded in `task_result_delivery` and retried by a sweep, so a
failed model call, a full queue, and a restart between the run finishing and the
turn starting are all recoverable rather than silent. A report is given up on
after a bounded number of attempts, with the reason recorded; the result is not
given up with it, because the card reads it from `task_run`.

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
- still absent: a worker selecting `SandboxSurfaceWorker`, process rlimits,
  sandboxing of command/HTTP hook transports, trace retention, and typed
  command-level boundary, file-change, hook, approval, retry, and failure-cause
  records;
- deliberately not covered by the shipped context work: general cross-session
  user/workspace/team memory and a UI for editing every memory source.

Acceptance:

- users can inspect and explain Agent runs without leaving the local surfaces
- local and worker sandbox boundaries are explicit and visible
- worker runs produce enough trace data for Portal diagnostics
- memory sources are visible, scoped, and user-controllable
- local and worker runtime differences are explicit, not hidden in surface-specific code

Worker OS sandboxing remains defense in depth rather than a Beta gate. A
`k8s_job` worker runs in a constrained Kubernetes pod and reports that it is
unsandboxed; it does **not** receive the stricter in-process sandbox baseline.
`local_process` remains one trust domain with the Server. See
[Beta Gate](#beta-gate) for the narrower boundary Beta requires and for the
cost of deferring OS sandboxing.

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
  Harbor for container/public-benchmark execution, Terminal-Bench as the first
  external capability coordinate, and optional viewers

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
- the legacy `eval/` catalog and `internal/agenteval` are retired rather than
  preserved behind compatibility code — **done**: both are deleted, and
  `./make eval` now measures a built binary against `evaluation/suite/`

The black-box vertical slice is enabling work before substantial new Agent
capability. Framework selection is deliberately downstream of that slice; see
[design/evaluation-system.md](design/evaluation-system.md).

Code state: **not started**. `cmd/buildmax-eval` and `internal/agenteval` are
the legacy evaluator that this priority is intended to replace, not evidence
that the new evaluation contract or black-box slice exists.

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
- operator model catalog and team model aliases behind the shared LLM contract,
  with per-call usage recorded before any spending limit is claimed — the
  catalog and call ledger exist; aliases are deployment-wide, not per-team
  (see [design/llm-gateway.md](design/llm-gateway.md) for what is and is not
  implemented)
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
trusted team. The former gate mixed code already present with evidence the
repository cannot provide (a real restore or a customer cluster). The gate now
names both, so a green unit suite is never misrepresented as production proof.

Beta is reached only after this is demonstrated and the resulting evidence is
recorded in the release or deployment runbook:

> An operator can bootstrap a private Kubernetes deployment, sign in, use an
> approved managed model, execute and retry work in a constrained worker, read
> its result, trace, model usage, and audit history, and recover from the
> documented dependency, cancellation, worker-loss, and upgrade cases.

| Gate | Code and automated-test state | Still needed for Beta |
|---|---|---|
| Worker boundary | Per-run JWT, minimized Job environment, non-root/read-only/capability-dropped pod, resource limits, no service-account token, and an explicit trace boundary are implemented. | Demonstrate the boundary in a clean cluster and state the policy for unsandboxed execution. Worker OS sandbox and egress restriction are not claimed. |
| Private deployment | Production manifest, migration ledger, `/readyz`, Compose/kind smoke, account bootstrap, and managed worker inference are implemented. | Apply the reference against real external MySQL/S3/TLS; exercise backup/restore and an N-1-compatible upgrade/rollback. |
| Explainable runs | Portal can read a stored run trace and managed-call ledger; traces contain model/tool timing and usage, artifacts, and the resolved sandbox boundary. Retry is deployment-smoke tested. | Deployment tests for cancellation, worker loss, and a dependency failure; typed failure classification and trace retention are follow-up hardening. |
| Minimum governance | Route authorization matrix, audit writes/export/retention, quota controls, and System Administration are implemented and tested. | Run the workflow with a real operator; audit-to-run correlation is useful follow-up work, not a duplicate event requirement. |
| Continuous verification | Post-merge/scheduled Compose and kind workflows plus Portal browser E2E are configured; support policy is published. | Treat the latest CI result as evidence at release time. Repository configuration alone cannot prove an external run passed. |

The first gate deliberately does not require the OS sandbox. Without it, the
worker's Bash path remains protected only by the in-process risky-command gate,
and a trace/Portal must say that no OS sandbox applied. Network egress is also
currently unbounded: no `NetworkPolicy` or evidenced allow-list is shipped. A
Beta release may state those limits; it may not imply containment it does not
enforce.

Deliberately outside the Beta gate: Desktop polish, SSO, executable team plugin
content, additional model providers, and general durable Session sync. The
instruction half of team plugin distribution — a team activating skill and
subagent releases, and a worker materializing exactly what it pinned — is
implemented; releases contributing hooks or MCP servers cannot be activated.

## Suggested Order

The previous sequence treated worker OS sandboxing as the next universal
dependency. That is not consistent with the Beta gate above: it is important
defense in depth, but it does not prove that the existing deployment works or
recovers. The immediate work is therefore evidence-first.

1. **Make the existing deployment proof complete.** Extend the deterministic
   Compose/kind smoke so it verifies cancellation, a worker that dies mid-run,
   and a dependency failure, in addition to the existing login, worker,
   artifact, retry, managed-model, authorization, and Portal checks. Keep these
   as deployment-level tests; unit tests already cover parts of the state
   machine but cannot prove the launched worker actually stops or reports.
2. **Run and record the operating exercises.** Against a real private-cluster
   dependency set, verify bucket permissions, backup/restore, and a migration
   upgrade followed by binary rollback. Add only the smallest product diagnostics
   exposed by those exercises; do not turn readiness into a destructive storage
   probe or invent metrics before deciding what an operator needs.
3. **Start P0.6 with a small black-box contract and slice.** Define the trial
   bundle/failure taxonomy first, then run built local, worker, conversation,
   and boundary subjects repeatedly. This can run alongside step 2, and it
   replaces the legacy evaluator only after it produces a useful failure bundle.
4. **Finish worker hardening as a parallel security track.** First decide and
   document fail-closed versus recorded downgrade when `bwrap` is unavailable;
   then add the backend to the image, prove the pod supports it, select
   `SandboxSurfaceWorker`, add rlimits, sandbox hook transports, and test the
   result. Treat egress as a separate threat-model and operator-policy decision;
   do not guess an allow-list. Sandboxing hook and MCP transports is the piece
   that bounds an activated plugin's own processes — the Bash sandbox never
   covered them, on any surface — so it is what makes executable team plugins
   contained rather than what permits them; see
   [design/plugin-team-distribution.md](design/plugin-team-distribution.md) §9.
   None of it hides steps 1–3 behind it.
5. **Choose one product bet from evidence.** The lowest-risk candidate is the
   local Issue work bridge. Durable Agent Sessions needed a decision on local
   session storage, privacy, retention, and synchronization first;
   [design/local-session-storage.md](design/local-session-storage.md) settles
   local storage and the privacy of what it holds, and its phase 0 has landed.
   Retention and synchronization are still open — that record leaves subagent
   bundle retention as a question and puts Server synchronization outside its
   scope. Session trees/mailboxes additionally require a workspace/change-set
   ownership design. SSO and the executable half of team plugin distribution
   stay behind the steps above unless a deployment partner supplies the
   evidence to reprioritize them.

## Avoid For Now

- a large workflow engine rewrite before results and runtime stability improve
- a complex approval/audit platform before basic governance lands
- Desktop duplicating Portal issue/workflow/team administration
- a full Git restore UI before the outcome and change model is clear
- any Portal-only Agent capability that bypasses the shared runtime

## Related Documents

- [../README.md](../README.md) — current system overview
- [design/README.md](design/README.md) — design document index
- [design/product-vision.md](design/product-vision.md) — long-range AI-native workspace vision
- [design/surface-positioning.md](design/surface-positioning.md) — product surface positioning
- [design/trust-harness.md](design/trust-harness.md) — P0.5 Agent Core trust harness design
- [design/evaluation-system.md](design/evaluation-system.md) — P0.6 evaluation and qualification design
- [design/context-durability.md](design/context-durability.md) — P0.5 instructions and session notes that survive compaction
- [design/local-background-jobs.md](design/local-background-jobs.md) — P0.5 local background jobs and monitors for TUI and Desktop
- [design/enterprise-deployment.md](design/enterprise-deployment.md) — P3 Enterprise deployment design
- [design/llm-gateway.md](design/llm-gateway.md) — P3 Managed LLM gateway design
- [design/graceful-shutdown.md](design/graceful-shutdown.md) — P3 shutdown ladder for server, scheduler, and worker
- [design/team-governance.md](design/team-governance.md) — P4 Team governance design
- [design/system-administration.md](design/system-administration.md) — P4 Deployment-scoped system administration design
