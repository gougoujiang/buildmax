# Design Records

> **Audience:** contributors · **Status:** current

Why BuildMax is built the way it is. These are **rationale, not user
documentation** — when a design ships something configurable, the user-facing
half lives in [../guide/](../guide/) or [../reference/](../reference/), and the
design record keeps the trade-offs and the open gaps.

Documents keep their number for life so code comments can cite them stably.
Numbers are never reused; gaps are documents that were retired.

## Specifications

Durable records of how a subsystem is designed. These stay current.

| # | Document | Subsystem | User docs |
|---|---|---|---|
| 031 | [Hook system v2](031-hook-system-v2.md) | Lifecycle hooks — 13 events, 4 transports | [guide/hooks.md](../guide/hooks.md) |
| 032 | [Sandbox and execution boundaries](032-sandbox-and-execution-boundaries.md) | Bash sandbox, egress proxy, policy layering | [guide/sandbox.md](../guide/sandbox.md) |
| 034 | [Durable run trace](034-durable-run-trace.md) | Bounded, redacted JSONL trace per run | — |

Shipped status, honestly: hooks are complete; the sandbox has phases A–E
shipped with worker hardening open (see its §13.1); traces are at phase 1.

## Product Direction

| # | Document | Scope |
|---|---|---|
| 001 | [About Portal](001-about-portal.md) | Long-range vision for the AI-native workspace direction |
| 023 | [Desktop, CLI, and Portal positioning](023-desktop-cli-portal-positioning.md) | How Agent Core, CLI, Desktop, and Portal relate as surfaces |

## Roadmap Plans

Work planned under a [ROADMAP.md](../../ROADMAP.md) priority. **These expire** —
when the work lands, its durable half moves to a specification or to the
architecture reference, and the plan is retired. Every document below is
therefore work that is designed but **not yet built**.

| # | Document | Priority | Status |
|---|---|---|---|
| 030 | [Agent Core P0.5 trust harness](030-agent-core-p0-5-trust-harness.md) | P0.5 | Partly shipped — 031/032/034 implement sections of it |
| 027 | [Enterprise deployment loop](027-enterprise-deployment-loop.md) | P3 | Designed, not implemented |
| 028 | [Team governance foundation](028-team-governance-foundation.md) | P4 | Designed, not implemented |
| 029 | [Versioned workspace design](029-versioned-workspace-design.md) | P5 | Designed, not implemented |

P0 (Agent Core stability), P1 (Local agent experience), and P2 (Portal outcome
surface) are complete; their plans were retired. Numbers 024, 025, and 026 stay
retired and are not reused — recover the documents from git history if you need
the reasoning behind a decision made then.

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

1. Take the next unused number; name it `NNN-kebab-case-title.md`.
2. State the problem, the options considered, the chosen approach, and the
   phases.
3. Add a row above — Specification or Roadmap Plan.
4. If it ships something a user configures, write the user-facing half in
   `guide/` or `reference/` and link it from the table.

When it stops describing the current direction, **delete it** and remove its
row. Git history keeps it; see
[contribute/documentation.md](../contribute/documentation.md).
