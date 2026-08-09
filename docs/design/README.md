# Design Documents

Current design documents for BuildMax. Each one records a decision or a planned
piece of work: the problem, the chosen approach, and the implementation phases.

Priority and sequencing decisions live in [../../ROADMAP.md](../../ROADMAP.md).
This directory holds the supporting product and technical designs that explain
or implement that roadmap. For how the system works today rather than why, see
[../architecture/](../architecture/). Superseded documents move to
[../archive/](../archive/).

Documents keep their number for life so code comments and other documents can
cite them stably. Numbers are never reused, including after archival — gaps in
the sequence are expected.

## Product Direction

| # | Document | Scope |
|---|---|---|
| 001 | [About Portal](001-about-portal.md) | Long-range product vision for the AI-native workspace direction |
| 023 | [Desktop, CLI, and Portal positioning](023-desktop-cli-portal-positioning.md) | How Agent Core, CLI, Desktop, and Portal relate as product surfaces |

## Roadmap Designs

Ordered by roadmap priority.

| # | Document | Priority | Status |
|---|---|---|---|
| 024 | [Agent Core stability](024-agent-core-stability.md) | P0 | Active plan |
| 030 | [Agent Core P0.5 trust harness](030-agent-core-p0-5-trust-harness.md) | P0.5 | Partially shipped — see detail designs below |
| 025 | [Local agent experience](025-local-agent-experience.md) | P1 | Active plan |
| 026 | [Portal outcome surface](026-portal-outcome-surface.md) | P2 | Active plan |
| 027 | [Enterprise deployment loop](027-enterprise-deployment-loop.md) | P3 | Active plan |
| 028 | [Team governance foundation](028-team-governance-foundation.md) | P4 | Active plan |
| 029 | [Versioned workspace design](029-versioned-workspace-design.md) | P5 | Active plan |

### P0.5 Detail Designs

These implement sections of 030 and carry their own phase tracking.

| # | Document | Implements | Status |
|---|---|---|---|
| 031 | [Hook system v2](031-hook-system-v2.md) | 030 hooks | Shipped — 13 events, 4 transports |
| 032 | [Sandbox and execution boundaries](032-sandbox-and-execution-boundaries.md) | 030 §3.2 | Phases A–E shipped; phase F (worker hardening) open — see its §13.1 |
| 034 | [Durable run trace](034-durable-run-trace.md) | 030 §3.3 | Phase 1 shipped |

## Foundations

Still-active designs that are not tied to a single roadmap priority.

| # | Document | Scope |
|---|---|---|
| 002 | [Environment config maintainability](002-env-config-maintainability.md) | Env-only configuration model and the single source of truth for env keys |

## Code Anchors

Where the designs above land in the tree:

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

## Adding A Document

1. Take the next unused number and name the file `NNN-kebab-case-title.md`.
2. State the problem, the options considered, the chosen approach, and the
   implementation phases.
3. Add a row to the appropriate table above.
4. When the design is completed, superseded, or no longer describes the current
   direction, move it to [../archive/](../archive/) and add it to that index
   rather than editing it to match new behavior.
