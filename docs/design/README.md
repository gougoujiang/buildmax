# Design Records

> **Audience:** contributors · **Status:** current

Why BuildMax is built the way it is. These are **rationale, not user
documentation** — when a design ships something configurable, the user-facing
half lives in [../guide/](../guide/) or [../reference/](../reference/), and the
design record keeps the trade-offs and the open gaps.

Documents use stable, semantic filenames. The index supplies their lifecycle
and reading order; filenames do not encode chronology or roadmap priority.

## Product Direction

Durable product decisions that guide more than one roadmap phase.

| Document | Scope |
|---|---|
| [Product vision](product-vision.md) | Long-range product model, ownership boundaries, and rules for future bets |
| [Surface positioning](surface-positioning.md) | How Agent Core, CLI, Desktop, and Portal relate |

## Active Roadmap Plans

Work tracked by [ROADMAP.md](../ROADMAP.md). Plans can be partly implemented;
their status must say what is shipped and what remains. When a plan is complete,
move durable decisions into a subsystem specification or the architecture
reference, then delete the plan.

| Document | Priority | Current state |
|---|---|---|
| [Agent Core trust harness](trust-harness.md) | P0.5 | Hooks shipped; sandbox worker hardening, trace follow-ups, and the §3.9 worker-boundary authority question open |
| [Context durability](context-durability.md) | P0.5 | Implemented: accumulating compaction, durable session notes, the pre-compaction checkpoint, and the additional system prompt. The §6 and §11 follow-ups are open |
| [Local Projects and Project Memory](local-project-memory.md) | P0.5 local follow-on | Phases 1 and 2 shipped: shared CLI/Desktop Project identity, bounded cross-session Project Memory, `MemoryRead` and `MemoryWrite` on enabled local primary runs, and the memory half of `buildmax info` and the TUI `/info` panel. Desktop lists and reads memories and nothing more; its editor, per-memory delete, and enable toggle are unbuilt, as is the §17 session review command. Phase 3 not started. Portal, team, and user memory remain outside this record |
| [Local session storage](local-session-storage.md) | unscheduled | Implemented: atomic session bundles, linked journal history, rewind, and physical-copy fork. Server synchronization is outside this record |
| [Session usage stats](session-usage-stats.md) | P1 follow-on | Per-session `buildmax info` and the session tab of the TUI `/info` panel shipped, with the subagent and compaction metering fixes they required. Cross-session aggregation and the Desktop surface are designed and not built |
| [Local background jobs](local-background-jobs.md) | P0.5 | Stages 1–3 shipped: background `Bash`/`Task` jobs, `Monitor`, typed delivery with parked wake-up on both surfaces, durable job logs. Durability beyond the process — spool, supervisor, scheduling — is decided against, not pending |
| [Evaluation and qualification](evaluation-system.md) | P0.6 | Local and worker slices shipped: the contract, both black-box adapters, deterministic/command/trace graders, preflight, repetition, paired comparison, and three tasks. The legacy harness is retired. The Terminal-Bench 2.1 adapter and importer are built and pinned, and have run the oracle smoke and a one-task canary; no wider run exists, so there is no external score yet. The canary subset is pinned and selectable but has not been run; that run, conversation and deployment adapters, model-grader calibration, the holdout, and the Inspect spike are open |
| [Issue model](issue-model.md) | P2 follow-on | Implemented: the backend and frontend plans in full. Typed links, threaded replies, mentions, and realtime push are out of scope, not pending |
| [Issue agent access](issue-agent-access.md) | unscheduled | Shipped on both planes: `GetIssue` and `ReportToIssue`, scoped by construction, behind run-token routes for a worker and `buildmax --issue` locally. Status, assignment, and hierarchy stay out of the model's hands, and a local report is stored as `local_agent` — a claim, never the `agent` a scheduled run writes. Artifact references in `GetIssue` are deferred on an ownership question |
| [Enterprise deployment](enterprise-deployment.md) | P3 | M1, M2, M4, and M5 shipped; M3 mostly done. Operating evidence and configuration checks remain. |
| [Graceful shutdown](graceful-shutdown.md) | P3 follow-on | Implemented: the shutdown ladder and its budget, the draining state, watcher-stream drain and its Portal half, turn quiescing, managed terminal callbacks, HTTP timeouts, the manifests, a worker that reports what it produced when asked to stop, a bounded scheduler stop, and the recorded liveness signal that closes a run whose worker was killed without warning. Re-dispatching an interrupted run belongs to run retry; durable workflow advance stays open |
| [Managed LLM gateway](llm-gateway.md) | P3 | Shipped for CLI/TUI/Desktop and task runs; strict quota open. Sections 1, 4.2, 7, 10, and 12 are revised by [client modes](client-modes.md), summarised in its status block |
| [LLM provider adapters](llm-provider-adapters.md) | P3 follow-on | Complete: three wire protocols, reasoning, prompt caching, and image input |
| [Prompt cache control](prompt-cache-control.md) | P3 follow-on | Phases 1–3 shipped and phase 4's mechanism with them: cache policy, the Anthropic and OpenAI native paths, usage and cost telemetry. The qualification suite has been run and both native paths qualified; compatible profiles stay empty by decision. The per-entry capability claim and the section 6 diagnostics are open |
| [Local Ollama provider](local-ollama-provider.md) | P1 follow-on | Complete: the adapter, local inventory, the CLI surface, and credential-free managed targets |
| [Worker run token](worker-run-token.md) | P3 | Complete: the only credential every worker route takes, and the shared worker token is removed |
| [Team governance](team-governance.md) | P4 | Roles, quota, workflow lifecycle, the audit trail, its retention and export, and quota alerting shipped; the second slice of actions and audit-to-run correlation open |
| [System administration](system-administration.md) | P4 | Implemented: grant model, operator command, admin API, Portal area, and model catalog surface |
| [Plugin distribution and private marketplace](plugin-marketplace.md) | post-Beta, P4 follow-on | Phases A–C shipped: plugins load, the Marketplace publishes and installs, Portal and Desktop manage it. Phase D is tracked by the next record; its instruction-only D1 slice is shipped |
| [Team and worker plugin distribution](plugin-team-distribution.md) | post-Beta, after the Marketplace | D1 works end to end: a team curates its plugin list or opens the whole catalog, an agent loads only what it names, and a worker materializes exactly the pinned releases. Portal manages activations; the agent's own plugin field is still API-only. Neither D2, executable content behind operator eligibility, nor D3, secret delivery, is started |
| [Entity identity and relational keys](entity-identity.md) | Beta gate | Implemented: opaque public handles, numeric relational keys, and the store boundary between them. §8 decided database foreign keys: none, until a real deletion feature adds them, and the §17 amendment moving storage off `BINARY(12)` to the canonical text form is in the schema |
| [Timestamp representation](timestamp-representation.md) | Beta gate | Implemented: every persisted instant is `time.Time`, `DATETIME(6)`, and RFC 3339, with a UTC-pinned connection and an architecture guard |
| [Workspace root and worktrees](workspace-root-and-worktrees.md) | step 5 product bet | Complete, phases 1-5: the root is session state every tool reads per call, an agent creates, enters, leaves, and removes its own worktrees under the containment, permission, and occupancy rules, derived configuration follows a move, three hook events announce it, and a delegate can have a tree of its own. Whether listing alone keeps worktrees from accumulating is the open question |
| [Tool permissions](tool-permissions.md) | unscheduled | Implemented; operator control over autonomous surfaces (§7) and the §10 open questions remain, one of which records a defect in shipped sandbox policy rather than in this design |
| [Parallel tool execution](parallel-tool-execution.md) | unscheduled | Implemented for read-only tools and read-only `Task` agent types; presentation and trace questions open (§11) |
| [Local end-to-end verification](end-to-end-testing.md) | unscheduled | Harness, CLI, Desktop bridge, CI policy, named suites, and the runbook done; some Portal paths, the deployment cancellation and failure-recovery paths, and the packaged-app smoke open |
| [Verification program](verification-program.md) | R0–R4 | Planned risk-weighted verification matrix, pull-request MySQL gate, deterministic critical journeys, controlled failure injection, scheduled deployment evidence, and external release rehearsal |
| [Unified artifacts](unified-artifacts.md) | P2 follow-on | Implemented: durable team artifacts with stable `ar_` references, upload/preview/download, tombstoned deletion, and `UploadArtifact` on every surface with a server. Registering a run's output directory and external sharing are decided against; the phase 4 follow-ons stay open |
| [Portal execution model](portal-execution-model.md) | P2 follow-on | Phases 0 and 3 shipped and phase 1 half shipped: outcome projection, durable result delivery, and run provenance. Phase 2 is reduced to what evidence supports; phases 4 and 5, including removing Conversation ownership, are deferred |

## Subsystem Specifications

Durable records for implemented or partly implemented subsystems. Keep these
aligned with code and link user-facing behavior to `guide/` or `reference/`.

| Document | Current state | User docs |
|---|---|---|
| [Client modes: local and managed](client-modes.md) | Implemented: the mode is whether a login is stored, models come from one source, the alias layer and per-team model policy are gone, and the call ledger is user-scoped | [guide/models-and-modes.md](../guide/models-and-modes.md) |
| [Hook system](hook-system.md) | 16 events and 4 transports implemented; the optional inspector and frontmatter integrations stay deferred | [guide/hooks.md](../guide/hooks.md) |
| [Sandbox boundaries](sandbox-boundaries.md) | Local phases A–E implemented; phase F — worker hardening and its documentation — open, with it the §13 gaps: no process rlimits, hook transports that do not consult `SandboxView`, and no `sandbox overrides` command | [guide/sandbox.md](../guide/sandbox.md) |
| [Durable run trace](durable-run-trace.md) | Phase 1 implemented; richer events and retention open | [guide/sessions-and-traces.md](../guide/sessions-and-traces.md) |
| [Queued messages](queued-messages.md) | Queueing on all three surfaces, mid-run injection on CLI/TUI and Desktop; persistence and Portal injection decided against | [reference/cli.md](../reference/cli.md) |

## Where The Designs Land

| Area | Package |
|---|---|
| Shared agent runtime assembly | `internal/agentapp` |
| Task-run execution runtime | `internal/agentapp/taskrun` |
| Tier 1 conversation orchestration | `internal/service/conversation` |
| Issue, task, workflow, and quota services | `internal/service/*` |
| HTTP API handlers | `internal/server/handlers` |
| Scheduler | `internal/server/scheduler` |
| Worker API client and updater | `internal/infra/workerclient` |
| Runtime agent tools | `internal/tool` |

Full tree: [contribute/repo-layout.md](../contribute/repo-layout.md).

## Adding One

1. Choose a short semantic filename such as `execution-policy.md`.
2. State the problem, options considered, chosen approach, status, and phases.
3. Add it to exactly one section above.
4. If it ships something a user configures, write the user-facing half in
   `guide/` or `reference/` and link it from the table.

When it stops describing the current direction, **delete it** and remove its
row. Git history keeps it; see
[contribute/documentation.md](../contribute/documentation.md).
