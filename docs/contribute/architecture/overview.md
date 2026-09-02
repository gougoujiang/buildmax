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
4. **Application services**: `internal/service/conversation`, `issue`, `task`, `workflow`, `quota`, `identity`, `llmcatalog`, `systemadmin`, and the rest. See [repo-layout.md](../repo-layout.md).
5. **Shared runtime**: `internal/agentapp` and `internal/agentapp/taskrun`.
6. **Pure core**: `internal/core/agent`, `internal/core/llm`, `internal/core/session`, and one package per domain — `task`, `team`, `issue`, `workflow`, `artifact`, `audit`, and the rest. See [repo-layout.md](../repo-layout.md).
7. **Infrastructure**: `internal/infra/db`, `llm`, `objectstore`, `mcp`, `workerclient`, `k8s`, and `log`.

`agentapp.NewAgentApp` has two explicit phases. `resolveAgentAppConfig` reads and
merges workspace, settings, plugin, hook, permission, and sandbox inputs into an
immutable assembly description; `buildAgentApp` opens MCP, sandbox, registry,
job, and trace resources from that description. `AgentApp.Close` owns those
resources, and a failed partial build closes what it already opened.

## Notes

- The repository tree lives in [../repo-layout.md](../repo-layout.md).
- Current product planning starts in [../../design/README.md](../../design/README.md)
  and [ROADMAP.md](../../ROADMAP.md).
- The product-level picture — what BuildMax is and who each surface is for —
  is in [../../start/concepts.md](../../start/concepts.md).
