# Repository Layout

> **Audience:** contributors · **Status:** current
>
> **This file is the single source of truth for the repository tree.** README,
> `AGENTS.md`, and the architecture reference link here rather than repeating
> it, so there is one place to update when a package moves.

## Top Level

```text
buildmax/
├── cmd/                  Binary entry points (main.go only)
├── internal/             All Go implementation
├── portal/               Portal web app (React 19 + Vite + TypeScript)
├── desktop/frontend/     Desktop frontend (React 19 + Vite)
├── gui/                  Shared React package @buildmax/gui, used by both
├── docs/                 Documentation
├── config-examples/      settings.yaml / server.yaml / hooks.yaml examples
├── deployment/           Kubernetes manifests and migrations
├── setup/                Local kind cluster scripts and manifests
├── eval/                 Agent benchmark task catalog
├── example/              Sample data used in demos and manual testing
└── scripts/              Repository scripts (third-party notice generation)
```

## Binaries

| Path | Binary | Role |
|---|---|---|
| `cmd/buildmax` | `buildmax` | CLI/TUI |
| `cmd/buildmax-server` | `buildmax-server` | HTTP API + in-process scheduler |
| `cmd/buildmax-worker` | `buildmax-worker` | Executes one task run, then exits |
| `cmd/buildmax-desktop` | — | Wails desktop app; embeds `desktop/frontend` |
| `cmd/buildmax-eval` | `buildmax-eval` | Agent benchmark runner over `eval/` |
| `cmd/local-test-mcp-server` | — | Small MCP server for testing MCP integration |
| `cmd/mk` | — | Task runner behind `./make` and `make.bat`; dev only, not released |

Every `cmd/*` package is a thin `main.go` that delegates to `internal/`.

## `internal/`

```text
internal/
├── bootstrap/          Process startup and dependency wiring (server, worker, objectstore)
├── config/             YAML + env config loading and path resolution
│
├── core/               Pure domain layer — no infra imports
│   ├── model/          Shared entities, repository contracts, LLM contracts, Tool contract
│   ├── agent/          The tool-calling loop, prompts, options, sandbox contract
│   ├── llm/            LLM contract helpers
│   └── session/        Local session model and persistence helpers
│
├── agentapp/           Agent runtime assembly: LLM client cache, tool registry,
│   │                   MCP, hooks, sandbox, traces, skills, sessions, workspace
│   └── taskrun/        One task run inside its run-scoped directory (worker)
│
├── service/            Application services: coordinate stores, enforce rules
│   ├── conversation/   Tier 1 orchestrator — the single voice to the user
│   │   ├── channel/    Normalized turn types and channel adapters (webhook)
│   │   ├── runtime/    Turn-loop mechanics: replay, tool assembly, streaming
│   │   └── tool/       Tier 1 tools and the task-runner bridge
│   ├── issue/          Issue service
│   ├── task/           Task and task_run service
│   ├── workflow/       Workflow and workflow-run orchestration
│   └── quota/          Team quota enforcement
│
├── tool/               Runtime agent tools: Read, Write, Edit, Bash, Glob, Grep,
│                       WebFetch, TodoWrite, Skill, Task, and the MCP gateway.
│                       names.go is the single source of truth for tool names.
│
├── infra/              External-system implementations
│   ├── db/             MySQL/GORM implementation of the core repositories
│   ├── objectstore/    Local FS and S3/MinIO persist + artifact storage
│   ├── llm/            OpenAI-compatible LLM client
│   ├── mcp/            MCP protocol, client transport, registry
│   ├── hook/           Hook transports: command, http, mcp_tool, prompt
│   ├── sandbox/        Seatbelt/bwrap backends, egress proxy, violations
│   ├── trace/          Durable run-trace recorder (bounded, redacted JSONL)
│   ├── k8s/            Kubernetes worker job launcher
│   ├── workerclient/   Worker-side HTTP client for the server worker API
│   ├── git/            Branch and diff helpers
│   └── log/            slog + lumberjack logging
│
├── interface/          Local user-facing entry points
│   ├── cli/            Cobra CLI, Bubble Tea TUI, print mode
│   ├── desktop/        Wails app bridge
│   ├── auth/           Login client and credential persistence
│   └── client/         HTTP client for the BuildMax server API
│
├── server/             HTTP API for Portal and worker callbacks
│   ├── handlers/       Route handlers
│   ├── httputil/       Shared request/response helpers
│   ├── scheduler/      Claims pending task runs and spawns workers
│   ├── websocket/      Team websocket hub and stream fan-out
│   └── static/         Embedded OpenAPI and Swagger assets
│
├── agenteval/          Evaluation harness: task catalog and runner
├── architecture/       Architectural constraint tests (import boundaries)
├── mock/               Test-only mocks and helpers
└── util/               ID generation, workspace helpers, git, argparse
```

## Dependency Direction

```text
bootstrap ──▶ interface / server / service / agentapp / infra ──▶ core
```

- `core` imports nothing from `config`, `infra`, `service`, `server`,
  `agentapp`, or `interface`. It is pure domain.
- `config` does env and file loading only; it does not import infra
  implementations.
- `internal/tool` is not pure — it imports infra (MCP, git) as needed.

These rules are enforced by tests in `internal/architecture`. If a change trips
one, the import is the problem, not the test.

## Frontends

| Directory | Package | Notes |
|---|---|---|
| `gui/` | `@buildmax/gui` | Shared presentational React components and theme. Build output in `gui/dist/`. |
| `portal/` | — | Depends on gui via `"@buildmax/gui": "file:../gui"` |
| `desktop/frontend/` | — | Depends on gui via `file:../../gui` |

Portal and Desktop share **widgets**, not logic — data, auth, and routing are
each app's own. Both run React 19.

## Related

- [architecture/](architecture/README.md) — what each subsystem does
- [CONTRIBUTING.md](../../CONTRIBUTING.md) — build, test, and run
