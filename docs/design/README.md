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
| [Product vision](product-vision.md) | Long-range direction for the AI-native workspace |
| [Surface positioning](surface-positioning.md) | How Agent Core, CLI, Desktop, and Portal relate |

## Active Roadmap Plans

Work tracked by [ROADMAP.md](../../ROADMAP.md). Plans can be partly implemented;
their status must say what is shipped and what remains. When a plan is complete,
move durable decisions into a subsystem specification or the architecture
reference, then delete the plan.

| Document | Priority | Current state |
|---|---|---|
| [Agent Core trust harness](trust-harness.md) | P0.5 | Hooks shipped; sandbox worker hardening and trace follow-ups open |
| [Enterprise deployment](enterprise-deployment.md) | P3 | M1 shipped; M2–M5 open |
| [Team governance](team-governance.md) | P4 | Roles, quota, and workflow lifecycle shipped; audit/event visibility open |
| [Versioned workspace](versioned-workspace.md) | P5 | Design ready for review; implementation not started |

## Subsystem Specifications

Durable records for implemented or partly implemented subsystems. Keep these
aligned with code and link user-facing behavior to `guide/` or `reference/`.

| Document | Current state | User docs |
|---|---|---|
| [Hook system](hook-system.md) | 13 events and 4 transports implemented | [guide/hooks.md](../guide/hooks.md) |
| [Sandbox boundaries](sandbox-boundaries.md) | Local phases A–E implemented; worker hardening open | [guide/sandbox.md](../guide/sandbox.md) |
| [Durable run trace](durable-run-trace.md) | Phase 1 implemented; richer events and retention open | [guide/sessions-and-traces.md](../guide/sessions-and-traces.md) |

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
