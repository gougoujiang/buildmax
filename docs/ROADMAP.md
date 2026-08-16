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

The near-term goal is:

> A company can privately deploy BuildMax and immediately use the same Agent
> Core for local execution, team collaboration, background work, result
> delivery, and basic governance.

## Near-Term Priorities

P0, P1, and P2 are **complete**. Active work starts at P0.5 and P3. The
completed sections are kept because their focus and acceptance criteria are the
standard the surfaces are held to, not because the work is outstanding.

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

### P0.5. Agent Core Trust Harness

After Portal outcomes are visible, return to the shared Agent Core and close the
trust gaps that separate a working agent from a serious execution harness.

Focus:

- sandbox and execution boundaries for filesystem, network, env, and process behavior
- runtime hooks for approvals, tools, file changes, compaction, and run outcome
- durable run traces with redaction, bounded tool output, usage, and latency
- scoped memory and instruction loading across user, workspace, team, agent, and session
- TUI/Desktop activity views and local diagnostics
- subagent trace linkage and optional isolation groundwork
- safer non-interactive worker execution

Acceptance:

- users can inspect and explain Agent runs without leaving the local surfaces
- local and worker sandbox boundaries are explicit and visible
- worker runs produce enough trace data for Portal diagnostics
- memory sources are visible, scoped, and user-controllable
- local and worker runtime differences are explicit, not hidden in surface-specific code

Enabling the OS sandbox on workers is not a Beta gate. It continues after Beta
as defense in depth; see [Beta Gate](#beta-gate) for the boundary Beta does
require and for what deferring the sandbox costs. The visibility half of this
priority is still a gate: a run must report the boundary it actually ran under.

### P3. Enterprise Deployment Loop

The product promise depends on private deployment being boring and repeatable.

Focus:

- recommended private deployment path for server, worker, Portal, MySQL, and MinIO/S3
- synchronized server config, storage config, and deployment docs
- clear startup errors and health checks
- Docker/kind/k8s path that runs end to end
- default admin/user/team/quota/model initialization story
- optional managed LLM connection mode, so a deployment can supply approved
  models without distributing provider credentials to users and workers —
  shipped for CLI, TUI, and Desktop; the worker path is still open
- operator model catalog and team model aliases behind the shared LLM contract,
  with per-call usage recorded before any spending limit is claimed — the
  catalog and call ledger exist; aliases are deployment-wide, not per-team
  (see [design/llm-gateway.md](design/llm-gateway.md) for what is and is not
  implemented)

Acceptance:

- a new environment can reach login, create work, run a worker task, and view the result without reading code
- a deployment can serve approved models to CLI, Desktop, and worker runs without distributing provider keys, while direct mode still runs with no server

### P4. Team Governance Foundation

Keep this practical. The near-term need is basic enterprise confidence, not a
full policy platform.

Focus:

- team-scoped quota UI and documentation
- role/permission boundary tests
- clear workflow lifecycle UI and copy for draft/published/archived
- design the smallest audit/event model
- make sensitive assets traceable over time: webhook keys, agent definitions, workflows

Acceptance:

- admins understand who can do what, what resources are used, and what state shared automation is in

### P5. Versioned Workspace Design

Versioned workspace is the long-term product center, but it should follow
outcome visibility and runtime stability.

Focus:

- define the minimum workspace state / snapshot / change / restore model
- derive from existing worker `home/`, `artifacts/`, and `global/` layout
- keep Git hidden as an implementation engine
- define how users see what changed and how they restore

Acceptance:

- there is an executable design for agent-produced state changes and restore before broad implementation begins

## Beta Gate

Alpha to Beta is not more agent capability. It is one trusted team using the
capability that already exists, stably, explainably, and recoverably.

Beta is reached when this sentence is true and tested:

> A new team deploys BuildMax to a private Kubernetes cluster by following the
> documentation, then signs in safely, uses operator-approved models, submits
> work, has it executed inside a constrained worker, and reads the result and
> the audit record. Common dependency failures, timeouts, and upgrades can be
> diagnosed, recovered, or rolled back.

Five gates, each with the surface that proves it:

| Gate | Proven by |
|---|---|
| Worker execution boundary is real, visible, and operator-controlled | A task run holds only the credentials that run needs, executes in a hardened pod with a bounded network egress, and records which boundary was actually in effect — including when it was not sandboxed |
| The recommended private deployment is operable | Kubernetes reference with external MySQL, S3, TLS, secrets, resource limits, and a versioned schema migration with a rollback path |
| A run explains itself | Portal answers model, tools, files, duration, tokens, and failure cause for any task run, with stable cancel/retry/timeout semantics |
| The minimum governance loop closes | Role and team authorization covered end to end by tests; audit events for sign-in, configuration, model use, task execution, and credential change |
| Beta claims are continuously verified | CI, Compose, and kind checks running again; Portal browser E2E; a published support and compatibility matrix (see [start/support.md](start/support.md)) |

The first gate deliberately does not require the OS sandbox. Enabling it on
workers is P0.5 work that continues past Beta; the Beta boundary is the pod and
the credential scope. That trade is only defensible once the credential scope is
actually small, because the fallback boundary is weak: without the sandbox, a
worker's `Bash` is gated by the first-token deny list in `internal/tool/safety.go`,
which stops an unintended `curl` but not a deliberate `bash -c` or `python -c`.
Two consequences follow, and neither may be left implicit in documentation:

- A run that is not sandboxed must say so — in its trace and in Portal. An
  unreported boundary is worse than a missing one.
- Workers stay unable to run `curl`, `npm`, `pip`, `rm`, and the rest of the
  risky-prefix list: those resolve to Ask, and Ask collapses to Deny where no
  approval handler exists. The sandbox is what would demote them to Allow, so
  deferring it is a capability decision as much as a security one.

Deliberately outside the Beta gate: Desktop polish, SSO, versioned workspace,
plugin distribution, and additional model providers. None of them block a
private server deployment reaching Beta.

## Suggested Order

Steps 1-3 of the original sequence — documentation and config cleanup, Agent
Core stability, and the Portal outcome surface — are done. What remains is
ordered so that each step can be verified when it lands, rather than verified
at the end:

0. Restore continuous verification: recover the GitHub Actions quota so `ci`,
   `deployment-smoke`, Compose, and kind gate merges again. Not a Beta gate of
   its own — a precondition, because every claim below is unverified without
   it.
1. Make the boundary observable before changing it: extend the run trace with
   the resolved execution boundary, and give Portal a run view that joins trace,
   artifacts, worker logs, and failure cause. A boundary nobody can see cannot
   be trusted or audited, and P0.5's own acceptance requires the view. The event
   must report an unsandboxed run as unsandboxed.
2. Shrink what a task run can reach. This is the Beta boundary, and it comes
   before any sandbox work because it is what makes deferring the sandbox
   defensible. Today `scheduler.LocalRunner` passes the server's entire
   environment to the worker, and `k8s.WorkerEnvFromEnviron` forwards every
   `BUILDMAX_*` variable into the Job pod — including the JWT secret, the
   database password, and the object-storage keys. A run should receive only
   what that run needs, the Job pod needs a security context, resource limits,
   no automounted service-account token, and bounded egress, and `LocalRunner`
   must be documented as a development path rather than a deployment topology.
3. Enterprise deployment loop: a production Kubernetes reference distinct from
   the kind smoke overlay, dependency-aware readiness in place of the fixed-200
   `/healthz`, the worker managed-LLM entry point, and versioned schema
   migration with a rollback path. Migration is the hardest item here, not the
   smallest: `AutoMigrate` has no down path today, so the rollback promise is
   currently unbacked.
4. Minimum team governance: role and team authorization tests, then the audit
   event model. Quota display and alerting follow; for a trusted team quota is
   cost control, not a security boundary.
5. Close the trust harness, as defense in depth rather than the only boundary:
   ship `bwrap` in the runtime image, confirm the pod permits unprivileged user
   namespaces, then pass `SandboxSurfaceWorker` from the task-run runtime, add
   process rlimits, ship `buildmax sandbox overrides`, and sandbox hook
   transports. Order matters here: the worker baseline sets
   `FailIfUnavailable: true`, so passing that surface before the image and pod
   can support a backend turns every worker run into a refusal. This step also
   restores `curl`, `npm`, and the rest of the risky-prefix list to workers.
6. Desktop local workbench polish: sessions, project selection, local results.
7. Versioned workspace design, ready for implementation planning.

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
- [design/enterprise-deployment.md](design/enterprise-deployment.md) — P3 Enterprise deployment design
- [design/llm-gateway.md](design/llm-gateway.md) — P3 Managed LLM gateway design
- [design/team-governance.md](design/team-governance.md) — P4 Team governance design
- [design/versioned-workspace.md](design/versioned-workspace.md) — P5 Versioned workspace design
