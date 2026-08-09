# buildmax

[![CI](https://github.com/gougoujiang/buildmax/actions/workflows/ci.yml/badge.svg)](https://github.com/gougoujiang/buildmax/actions/workflows/ci.yml)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

Build Everything with AI.

> **Status: Alpha.** BuildMax is under active development. Interfaces,
> deployment guidance, and runtime behavior may change before a stable release.

BuildMax is an out-of-the-box, privately deployable enterprise Agent platform.
It provides one shared Go Agent Runtime that can be used directly on a local
machine or operationalized through a team Portal.

The main product promise is:

> A company can deploy BuildMax privately and immediately get practical Agent
> capability: local single-agent execution through CLI/Desktop, and team-scale
> orchestration, governance, files, workflows, and results through Portal, all
> powered by the same Agent core.

BuildMax has three main product surfaces over that same runtime:

- CLI/TUI for local terminal use
- Wails Desktop app for local native workbench use
- Portal web app backed by the Go server for team and enterprise use

The backend also includes a scheduler/worker execution pipeline for background task runs, plus a shared React GUI package used by Portal and Desktop.

## Product Shape

BuildMax is not choosing between "local AI file assistant" and "team AI
workspace." The local surfaces and the Portal are two ways to expose the same
Agent capability.

- **Agent Core** is the foundation: LLM loop, tool calling, local file actions,
  MCP, skills, subagents, sessions, and runtime assembly.
- **CLI/Desktop** show what one Agent can do for one user in a local environment.
- **Portal/Server** turns that same Agent capability into an enterprise
  platform: team ownership, conversations, issues, workflows, agent
  definitions, permissions, quota, shared files, background execution, and
  results.

Users can use only the local tools, deploy only the server/Portal path for a
team, or use both together.

## Current Model

The active product model is:

- `team`
- `conversation`
- `issue`
- `agent`
- `workflow`
- `task`
- `task_run`

Recommended mental model:

- work happens inside a `team`
- users talk to the system through a `conversation`
- user-facing work is tracked as an `issue`
- reusable execution plans live in `workflow`
- low-level background execution still runs through `task` and `task_run`

This means BuildMax is no longer primarily a simple CLI agent or a user-scoped
workspace app. It is a shared Agent core plus local and enterprise deployment
surfaces.

## Main Components

### CLI / TUI

- Binary: `cmd/buildmax`
- Default experience: Bubble Tea TUI
- Also supports print/prompt mode
- Uses the shared runtime assembly in `internal/agentapp`
- Persists local sessions under `BUILDMAX_HOME` / `~/.buildmax`
- Represents the direct single-user Agent experience: what the Agent can do
  against a local workspace with local tools

### Server

- Binary: `cmd/buildmax-server`
- HTTP API for auth, teams, members, agents, issues, workflows, conversations, tasks, files, artifacts, webhook keys, and worker callbacks
- Starts the scheduler that claims pending task runs and launches workers
- Makes the shared Agent runtime deployable as an enterprise/team system

### Worker

- Binary: `cmd/buildmax-worker`
- Executes one task run at a time
- Materializes team files, prepares run-scoped `AGENTS.md`, runs the shared runtime, writes artifacts, and reports status back to the server

### Portal

- Directory: `portal/`
- Stack: React 19 + Vite + TypeScript
- Uses the Go backend over HTTP
- Supports conversations, issues, workflows, agents, team settings, files, login/signup, and related team-scoped work flows
- Provides the enterprise operation layer on top of the same Agent core

### Desktop

- Entrypoint: `cmd/buildmax-desktop`
- Frontend: `desktop/frontend/`
- Stack: Wails + React 19 + Vite
- Local workbench experience using the shared runtime and local session storage
- Represents the same single-user Agent capability as CLI with a richer UI

### Shared GUI

- Directory: `gui/`
- Package: `@buildmax/gui`
- Shared presentational React components and CSS for Portal and Desktop
- Current exports include theme, modal, avatar, chat thread/composer, form modal, and recent-list components

## Architecture Snapshot

### Shared runtime

`internal/agentapp` is the reusable runtime assembly used by:

- CLI/TUI
- worker task execution
- desktop app chat

### Two-tier flow

BuildMax now uses a clear two-tier architecture:

- Tier 1: user-facing conversation orchestration in `internal/service/conversation`
- Tier 2: background execution through `task` and `task_run`, executed by worker processes

The core agent tool-calling loop lives in `internal/core/agent`; task-run runtime execution lives in `internal/agentapp/taskrun`.

`internal/service/conversation` should be read as the Tier 1 application subsystem rather than a thin package. Its current internal shape is:

- root package: service entrypoints, exported contracts, webhook turn policy, and runtime facade
- `channel/`: channel-facing normalized turn types and adapters such as the webhook adapter
- `runtime/`: Tier 1 turn-loop runtime mechanics such as message replay, tool assembly, prompt assembly, and streaming integration
- `tool/`: Tier 1 conversation tools plus the task-service/store runner bridge used to expose task operations to the model

## Repository Layout

```text
buildmax/
├── cmd/
│   ├── buildmax/              # CLI/TUI binary
│   ├── buildmax-server/       # HTTP server + scheduler
│   ├── buildmax-worker/       # Worker binary
│   └── buildmax-desktop/      # Wails desktop app
├── internal/
│   ├── bootstrap/             # Process startup and dependency wiring
│   │   ├── server.go          # Server bootstrap
│   │   ├── worker.go          # Worker bootstrap
│   │   └── objectstore.go     # Object storage builders
│   ├── config/                # Env loading and path/config resolution
│   ├── agentapp/              # Shared agent runtime assembly for CLI, worker, and desktop
│   │   └── taskrun/           # Task-run execution in run-scoped directories
│   ├── core/                  # Pure domain contracts and algorithms
│   │   ├── model/             # Shared entities, repository contracts, LLM contracts, Tool contract
│   │   ├── agent/             # Core agent loop and tool-calling logic
│   │   ├── llm/               # Shared LLM contracts
│   │   └── session/           # Local session model/persistence helpers
│   ├── service/               # Application services and orchestration
│   │   ├── conversation/      # Tier 1 conversation subsystem
│   │   │   ├── channel/       # Channel-facing types and adapters
│   │   │   ├── runtime/       # Tier 1 turn-loop runtime internals
│   │   │   └── tool/          # Tier 1 conversation tools and task runner bridge
│   │   ├── issue/             # Issue workflows
│   │   ├── task/              # Task and task_run workflows
│   │   ├── workflow/          # Workflow orchestration
│   │   └── quota/             # Team quota checks
│   ├── infra/                 # External-system implementations
│   │   ├── db/                # MySQL/GORM persistence
│   │   ├── objectstore/       # Local FS and S3/MinIO blob storage
│   │   ├── llm/               # OpenAI-compatible LLM client
│   │   ├── k8s/               # Kubernetes job support
│   │   ├── mcp/               # MCP transport/registry/probe
│   │   └── log/               # slog/lumberjack logging
│   ├── interface/             # Local user-facing entry points
│   │   ├── auth/              # CLI/desktop auth client and credential persistence
│   │   ├── cli/               # Cobra CLI and prompt/TUI entry
│   │   ├── tui/               # Bubble Tea TUI
│   │   └── desktop/           # Wails app bridge
│   ├── server/                # HTTP API, handlers, auth, worker API, webhook, websocket, scheduler
│   │   ├── handlers/          # Route handlers for Portal and worker APIs
│   │   └── scheduler/         # Pending-run scheduler and worker launcher
│   ├── session/               # Local session persistence
│   ├── tool/                  # Runtime agent tools (file, bash, grep, MCP gateway, skill, subagent)
│   ├── mock/                  # Test-only mocks/helpers
│   └── util/                  # IDs, workspace helpers, git, argparse
├── portal/                    # Portal web app
├── desktop/frontend/          # Desktop frontend
├── gui/                       # Shared React GUI package
└── docs/                      # Documentation: architecture reference, design docs, archive
```

The main dependency direction is `bootstrap -> interface/server/service/agentapp/infra -> core`, with `config` kept as env/path loading only. Core packages should not import `config`, `infra`, `service`, `server`, `agentapp`, or `interface`.

## Install

Download an archive from [Releases](https://github.com/gougoujiang/buildmax/releases).
Each one contains `buildmax`, `buildmax-server`, and `buildmax-worker` for
Linux, macOS, and Windows, plus `config-examples/`. Verify it against
`checksums.txt`, then put the binaries on your `PATH`.

With a Go toolchain, for the CLI alone:

```bash
go install github.com/gougoujiang/buildmax/cmd/buildmax@latest
```

As a container, carrying all three binaries:

```bash
docker pull ghcr.io/gougoujiang/buildmax:latest
```

Set `BUILDMAX_API_KEY` (and optionally `BUILDMAX_BASE_URL` / `BUILDMAX_MODEL`),
then run `buildmax` for the TUI or `buildmax -p "your prompt"` for print mode.

The desktop app is not published as a binary: it needs code signing and
notarization to be launchable on macOS. Build it locally with `./make build`.

## Build and Run

For working on the project, the primary local workflow uses the root `./make`
script.

- `./make build`
  Builds CLI, server, worker, shared GUI, and desktop app.
- `./make test`
  Runs `go test ./...` with `BUILDMAX_HOME=./testing-sandbox`.
- `./make run server`
  Builds and runs the Go server.
- `./make run portal`
  Starts the Portal dev server and builds `gui` if needed.
- `./make run desktop`
  Starts the Wails desktop app and builds `gui` if needed.
- `./make clean`
  Removes binaries and frontend build artifacts.

Portal can also be run directly:

```bash
cd portal
npm install
npm run dev
```

GUI package can be built directly:

```bash
cd gui
npm install
npm run build
```

Desktop app details are in [cmd/buildmax-desktop/README.md](cmd/buildmax-desktop/README.md). Portal-specific instructions are in [portal/README.md](portal/README.md).

## Security And Operating Boundaries

BuildMax can invoke model-selected tools and shell commands. Treat every
runtime configuration as an execution boundary: use dedicated credentials,
least-privilege workspace access, and an explicit network policy. The bash
sandbox and runtime hooks are available for tighter controls, but are not a
substitute for reviewing the permissions granted to a runtime or worker.

Never commit credentials or production secrets. See [SECURITY.md](SECURITY.md)
for responsible disclosure and operator guidance.

### Server Authentication Is Not Production-Ready

The server has no one-time-password delivery channel yet. `POST /api/login` is
therefore **disabled by default** and returns 503.

For local development you can enable a single fixed code with `dev_login_otp`
in `server.yaml` (or `BUILDMAX_DEV_LOGIN_OTP`). Understand what that does:
**one code signs in every registered account**, so anyone who knows a
registered email address can authenticate. Use it only on a local or otherwise
trusted deployment, never on a host reachable by untrusted users. Putting the
Portal on the public internet requires wiring a real identity provider first.

## Configuration

Configuration is env-based.

Common examples:

- `BUILDMAX_API_KEY`
- `BUILDMAX_BASE_URL`
- `BUILDMAX_MODEL`
- `BUILDMAX_HOME`
- `BUILDMAX_JWT_SECRET`
- `BUILDMAX_SERVER_PORT`
- `BUILDMAX_WORKER_TOKEN`

See:

- `config-examples/.env.example`
- `internal/config/env_spec.go`
- `docs/design/002-env-config-maintainability.md`

## Contributing

BuildMax welcomes focused contributions. Read [CONTRIBUTING.md](CONTRIBUTING.md)
for development checks, architectural boundaries, and pull request guidance.

## License And Name

BuildMax is licensed under the [Apache License 2.0](LICENSE). The BuildMax name
and logo are not granted by that license; see [TRADEMARKS.md](TRADEMARKS.md).

## Documentation

Full documentation lives in [docs/](docs/) — see [docs/README.md](docs/README.md)
for the index.

- [ROADMAP.md](ROADMAP.md) — active near-term roadmap and priority order
- [docs/architecture/](docs/architecture/) — how the system works today, per subsystem
- [docs/design/](docs/design/) — current design documents behind the roadmap
- [docs/archive/](docs/archive/) — superseded designs, kept for history

## Current Status

As of the current main branch state:

- server, scheduler, and worker are wired together
- team is the ownership boundary for issues, workflows, conversations, tasks, and uploaded files
- Portal supports team-scoped conversations, issues, workflows, agents, and file browsing/upload
- issue detail already exposes issue-centric execution visibility across workflow runs and agent-backed tasks
- governance foundation is landed: team roles support `owner/admin/member`, and workflows use `draft/published/archived`
- Desktop already supports local chat backed by the shared runtime
- shared GUI is used by both Portal and Desktop
- `internal/service/conversation` is internally split into `channel`, `runtime`, and `tool` sub-packages as the Tier 1 application subsystem
- the previous Team / Issue / Workflow roadmap is effectively complete for its intended scope
- the active near-term product roadmap is `ROADMAP.md`
- older phase plans and completed refactor roadmaps live in `docs/archive/`

## Notes

- If a workspace `AGENTS.md` exists, the agent runtime appends it to the system prompt.
- For remote task runs, the worker prepares a run-scoped `AGENTS.md` so the same convention applies in worker execution.
