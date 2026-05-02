# Internal Package Refactor

## Goal

Reorganize `internal/` around stable architectural responsibilities while keeping each step small enough to compile and test independently.

Target top-level packages:

- `bootstrap`: startup composition for binaries and server/worker bootstrap.
- `config`: environment and startup configuration.
- `core`: business concepts, use cases, repository contracts, and shared model contracts.
- `execution`: agent/runtime execution, scheduler, worker coordination, artifacts, and tool adapters.
- `infra`: implementations for external systems such as DB, object storage, LLM providers, Kubernetes, MCP transport, and logging.
- `server`: HTTP/WebSocket transport handlers and server lifecycle.
- `interface`: local user entry points such as CLI, TUI, and desktop.
- `session`, `util`, `mock`: supporting packages with controlled scope.

## Target Shape

```text
internal/
  bootstrap/
    server.go
    worker.go
    objectstore.go
  config/
  core/
    model/
    agent/
    conversation/
    issue/
    task/
    workflow/
    team/
    workspace/
    quota/
  execution/
    runtime/
    agenttool/
    mcptool/
    scheduler/
    worker/
    artifact/
  infra/
    db/
    objectstore/
    llm/
    k8s/
    mcp/
    log/
  server/
    auth/
    portalapi/
    workerapi/
    webhook/
    websocket/
    httputil/
  interface/
    auth/
    cli/
    tui/
    desktop/
  session/
  util/
  mock/
```

## Dependency Rules

Allowed high-level direction:

```text
bootstrap -> config, interface, server, execution, infra
interface -> config, core, execution, infra, session, util
server -> core, execution/worker, server/websocket, session, util
execution -> core, infra/objectstore, infra/llm, infra/mcp, session, util
infra -> core, util
core -> util only where unavoidable, otherwise stdlib
```

Hard constraints:

- `core` must not depend on `config`, `infra`, `execution`, `server`, or `interface`.
- `infra` must not depend on `server` or `interface`.
- `execution` must not depend on `server` or `interface`.
- `server` must not depend on `interface`.
- `config` must not depend on `core`, `infra`, `execution`, or `server`.

## Migration Plan

Move in small compiling steps:

1. [x] Establish `core/model` for shared contracts such as `Tool`.
2. [x] Move low-risk application services from `app/task` and `app/issue` into `core/task` and `core/issue`.
3. [x] Move conversation and workflow orchestration into `core/conversation` and `core/workflow`.
4. [x] Split `executor` into `execution/scheduler`, `execution/runtime`, `execution/worker`, and infra-specific packages.
5. [x] Move runtime tools into `execution/agenttool`.
6. [x] Split MCP into `execution/mcptool` for agent tool integration and `infra/mcp` for protocol/client transport.
7. [x] Move storage, object storage, LLM, Kubernetes, and logging implementations into `infra/*`.
8. [x] Rename server API packages to `server/portalapi` and `server/workerapi`; collect WebSocket pieces under `server/websocket`.
9. [x] Move local entry points into `interface/*`.
10. [x] Remove compatibility shims after imports have been migrated.
11. [x] Update active documentation to describe the new package layout.

Each step should keep `go test ./...` and the three main Go binaries buildable.

## Current Status

The package refactor is complete for the planned scope. Source imports and active documentation now use the target package layout directly, and the old compatibility packages have been removed.

Remaining architecture cleanup:

- Consider whether to split `core/model` into smaller domain-specific model/contract packages if it becomes too broad.
- Consider introducing `core/team`, `core/workspace`, or `execution/artifact` when those concepts need dedicated orchestration logic rather than shared models or infrastructure adapters.
