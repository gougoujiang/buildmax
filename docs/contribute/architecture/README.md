# Architecture Reference

> **Audience:** contributors · **Status:** current

How BuildMax works today. Each document covers one package or subsystem and is
kept in sync with the code — when a package boundary moves or a runtime
contract changes, the corresponding document is updated in the same change.

For the reasoning behind these structures, see
[../../design/](../../design/README.md). For where each package sits in the
tree, see [../repo-layout.md](../repo-layout.md).

## Start Here

- [overview.md](overview.md) — system architecture, directory layout, and how
  the pieces fit together. Read this first.
- [packages.md](packages.md) — which direction dependencies may point, and the
  import rules the build enforces.

## Agent Runtime

| Document | Covers |
|---|---|
| [agent-loop.md](agent-loop.md) | Core agent logic: LLM calls, tool execution, conversation loop |
| [llm-client.md](llm-client.md) | OpenAI-compatible LLM client and message types |
| [tools.md](tools.md) | Tool contract, registry, and the runtime tool implementations |
| [session.md](session.md) | Chat session model, persistence, and the session list index |

## Local Surfaces

| Document | Covers |
|---|---|
| [cli.md](cli.md) | Cobra CLI entry point, flags, and command dispatch |
| [tui.md](tui.md) | Bubble Tea terminal UI: viewport, input, keyboard handling |

## Server And Portal

| Document | Covers |
|---|---|
| [server.md](server.md) | HTTP API, worker callbacks, WebSocket fan-out, and the scheduler |
| [store.md](store.md) | Persistence for team, conversation, issue, agent, workflow, task, task_run, usage |
| [portal.md](portal.md) | Web UI (React + Vite): auth, teams, conversations, issues, workflows, agents, files |

## Cross-Cutting

| Document | Covers |
|---|---|
| [config.md](config.md) | YAML settings/server config loading, env spec, and path resolution |
| [logging.md](logging.md) | slog initialization and the file-only rotating log |
| [util.md](util.md) | Workspace path resolution, git helpers, argument parsing |
