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

### P3. Enterprise Deployment Loop

The product promise depends on private deployment being boring and repeatable.

Focus:

- recommended private deployment path for server, worker, Portal, MySQL, and MinIO/S3
- synchronized server config, storage config, and deployment docs
- clear startup errors and health checks
- Docker/kind/k8s path that runs end to end
- default admin/user/team/quota/model initialization story

Acceptance:

- a new environment can reach login, create work, run a worker task, and view the result without reading code

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

## Suggested Order

Steps 1-3 of the original sequence — documentation and config cleanup, Agent
Core stability, and the Portal outcome surface — are done. What remains:

1. Agent Core P0.5 trust harness: finish the sandbox worker profile, rlimits,
   and hook-transport enforcement; extend traces beyond phase 1.
2. Enterprise deployment loop: verify the Kubernetes path end to end, add health
   and readiness diagnostics, write the production reference guide.
3. Desktop local workbench polish: sessions, project selection, local results.
4. Team governance: approvals and audit log on top of the existing roles and quota.
5. Versioned workspace design, ready for implementation planning.

## Avoid For Now

- a large workflow engine rewrite before results and runtime stability improve
- a complex approval/audit platform before basic governance lands
- Desktop duplicating Portal issue/workflow/team administration
- a full Git restore UI before the outcome and change model is clear
- any Portal-only Agent capability that bypasses the shared runtime

## Related Documents

- [README.md](README.md) — current system overview
- [docs/design/README.md](docs/design/README.md) — design document index
- [docs/design/product-vision.md](docs/design/product-vision.md) — long-range AI-native workspace vision
- [docs/design/surface-positioning.md](docs/design/surface-positioning.md) — product surface positioning
- [docs/design/trust-harness.md](docs/design/trust-harness.md) — P0.5 Agent Core trust harness design
- [docs/design/enterprise-deployment.md](docs/design/enterprise-deployment.md) — P3 Enterprise deployment design
- [docs/design/team-governance.md](docs/design/team-governance.md) — P4 Team governance design
- [docs/design/versioned-workspace.md](docs/design/versioned-workspace.md) — P5 Versioned workspace design
