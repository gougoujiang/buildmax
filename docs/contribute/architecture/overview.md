# Project Overview

## Purpose

BuildMax is an out-of-the-box, privately deployable enterprise Agent platform
with one shared Go runtime and three product surfaces:

- CLI/TUI for fast local terminal execution
- Desktop for local project/session workbench usage
- Portal for team-scoped conversations, issues, workflows, agents, files, and results

The backend includes a Go HTTP server, a scheduler, and worker task-run
execution. Portal and Desktop share React presentation components from `gui/`.

The main promise is not local-vs-cloud. Users can use local CLI/Desktop only,
deploy Portal for a company, or use both. All paths should share the same Agent
core.

## Current Product Model

The active collaboration model is:

- `team`
- `conversation`
- `issue`
- `agent`
- `workflow`
- `task`
- `task_run`

Team is the ownership boundary for shared Portal work. CLI and Desktop operate
against local folders and local sessions, with optional server login for identity.

## Architecture

```text
CLI/TUI  ─┐
Desktop  ├─> internal/agentapp ─> internal/core/agent ─> tools / LLM
Worker   ┘              ▲
                        │
Portal -> server/handlers -> service/conversation -> service/task -> scheduler -> worker
```

The main layers are:

1. **Entry points**: `cmd/buildmax`, `cmd/buildmax-server`, `cmd/buildmax-worker`, `cmd/buildmax-desktop`.
2. **Local interfaces**: `internal/interface/cli`, `internal/interface/desktop`, `internal/interface/auth`, `internal/interface/client`.
3. **Server**: `internal/server` and `internal/server/handlers`.
4. **Application services**: `internal/service/conversation`, `issue`, `task`, `workflow`, and `quota`.
5. **Shared runtime**: `internal/agentapp` and `internal/agentapp/taskrun`.
6. **Pure core**: `internal/core/agent`, `internal/core/llm`, `internal/core/model`, `internal/core/session`.
7. **Infrastructure**: `internal/infra/db`, `llm`, `objectstore`, `mcp`, `workerclient`, `k8s`, and `log`.

## Directory Layout

```text
cmd/
  buildmax/              CLI/TUI binary
  buildmax-server/       HTTP server + scheduler binary
  buildmax-worker/       Worker binary for one task run
  buildmax-desktop/      Wails desktop binary
internal/
  agentapp/              Shared agent runtime assembly
    taskrun/             Worker task-run execution runtime
  bootstrap/             Process startup and dependency wiring
  config/                Env/config/path loading
  core/                  Pure domain contracts and agent loop primitives
  service/               Application orchestration
  server/                HTTP API, handlers, websocket, scheduler
  infra/                 DB, LLM, object storage, MCP, worker client, logging
  interface/             CLI, desktop, auth, server client
  tool/                  Runtime agent tools
portal/                  React Portal app
desktop/frontend/        React Desktop frontend
gui/                     Shared React component package
```

## Notes

- The full repository tree lives in [../repo-layout.md](../repo-layout.md); the
  abbreviated version above is for orientation only.
- Current product planning starts in [../../design/README.md](../../design/README.md)
  and [ROADMAP.md](../../ROADMAP.md).
- The product-level picture — what BuildMax is and who each surface is for —
  is in [../../start/concepts.md](../../start/concepts.md).
