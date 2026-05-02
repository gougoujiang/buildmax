# Application Boundaries

## Purpose

There is no longer an `internal/app` package. The old thin TUI wrapper and application-service packages were folded into the stable architecture:

- local user entry points live under `internal/interface/*`
- business use cases and orchestration live under `internal/core/*`
- runtime execution and worker/scheduler behavior live under `internal/execution/*`
- process startup and dependency wiring live under `internal/bootstrap/*`

## Current Mapping

| Old responsibility | Current package |
|--------------------|-----------------|
| TUI wrapper | `internal/interface/tui` called from `internal/interface/cli` |
| Tier 1 conversation orchestration | `internal/core/conversation` |
| Issue service | `internal/core/issue` |
| Task service | `internal/core/task` |
| Workflow service | `internal/core/workflow` |
| Shared agent runtime assembly | `internal/execution/agentrun` |

## Notes

Use `internal/bootstrap/*` only for process startup wiring. Do not introduce a new `internal/app` package unless the architecture is intentionally revised again.
