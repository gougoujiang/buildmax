# Application Boundaries

## Purpose

There is no longer an `internal/app` package. The old thin TUI wrapper and application-service packages were folded into the stable architecture:

- local user entry points live under `internal/interface/*`
- pure domain contracts and agent loop primitives live under `internal/core/*`
- application services and orchestration live under `internal/service/*`
- reusable local/worker agent runtime assembly lives under `internal/agentapp`
- scheduler and worker launch/update coordination live under `internal/server/scheduler`, `internal/infra/workerclient`, and `internal/agentapp/taskrun`
- process startup and dependency wiring live under `internal/bootstrap/*`

## Current Mapping

| Old responsibility | Current package |
|--------------------|-----------------|
| TUI / prompt mode | `internal/interface/cli` |
| Desktop Wails bridge | `internal/interface/desktop` |
| Tier 1 conversation orchestration | `internal/service/conversation` |
| Issue service | `internal/service/issue` |
| Task service | `internal/service/task` |
| Workflow service | `internal/service/workflow` |
| Quota service | `internal/service/quota` |
| Shared agent runtime assembly | `internal/agentapp` |
| Task-run execution runtime | `internal/agentapp/taskrun` |
| HTTP API handlers | `internal/server/handlers` |
| Scheduler | `internal/server/scheduler` |
| Worker API client/updater | `internal/infra/workerclient` |

## Notes

Use `internal/bootstrap/*` only for process startup wiring. Do not introduce a new `internal/app` package unless the architecture is intentionally revised again.
