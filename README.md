# buildmax

Build Everything with AI.

BuildMax is a multi-entry AI agent system with a shared Go runtime and three main product surfaces:

- CLI/TUI for local terminal use
- Portal web app backed by the Go server
- Wails desktop app for local native chat

The backend also includes a scheduler/worker execution pipeline for background task runs, plus a shared React GUI package used by Portal and Desktop.

## Current Model

The active ownership and execution model is:

- `user`
- `conversation`
- `task`
- `task_run`

Agents, uploaded files, and webhook keys are user-scoped. Portal and backend have largely moved away from the old workspace-centric product model.

## Main Components

### CLI / TUI

- Binary: `cmd/buildmax`
- Default experience: Bubble Tea TUI
- Also supports print/prompt mode
- Uses the shared runtime in `internal/app/agentrun`
- Persists local sessions under `BUILDMAX_HOME` / `~/.buildmax`

### Server

- Binary: `cmd/buildmax-server`
- HTTP API for auth, agents, conversations, tasks, files, artifacts, webhook keys, and worker callbacks
- Starts the scheduler that claims pending task runs and launches workers

### Worker

- Binary: `cmd/buildmax-worker`
- Executes one task run at a time
- Materializes user files, prepares run-scoped `AGENTS.md`, runs the shared runtime, writes artifacts, and reports status back to the server

### Portal

- Directory: `portal/`
- Stack: React 19 + Vite + TypeScript
- Uses the Go backend over HTTP
- Supports conversations, task detail, artifacts, files, agents, login/signup, and related user flows

### Desktop

- Entrypoint: `cmd/buildmax-desktop`
- Frontend: `desktop/frontend/`
- Stack: Wails + React 19 + Vite
- Local-only chat experience using the shared runtime and local session storage

### Shared GUI

- Directory: `gui/`
- Package: `@buildmax/gui`
- Shared presentational React components and CSS for Portal and Desktop
- Current exports include theme, modal, avatar, chat thread/composer, form modal, and recent-list components

## Architecture Snapshot

### Shared runtime

`internal/app/agentrun` is the reusable runtime used by:

- CLI/TUI
- worker task execution
- desktop app chat

### Two-tier flow

BuildMax is moving on a clear two-tier architecture:

- Tier 1: user-facing conversation orchestration in `internal/app/conversation`
- Tier 2: background execution through `task` and `task_run`, executed by worker processes

The low-level Tier 1 loop lives in `internal/conversation`, while the app-layer orchestration boundary lives in `internal/app/conversation`.

## Repository Layout

```text
buildmax/
├── cmd/
│   ├── buildmax/              # CLI/TUI binary
│   ├── buildmax-server/       # HTTP server + scheduler
│   ├── buildmax-worker/       # Worker binary
│   └── buildmax-desktop/      # Wails desktop app
├── internal/
│   ├── app/
│   │   ├── agentrun/          # Shared runtime for CLI, worker, desktop
│   │   ├── conversation/      # Tier 1 conversation orchestration
│   │   └── task/              # Task and task_run workflows
│   ├── agent/                 # Core agent loop and tool-calling logic
│   ├── conversation/          # Low-level conversation loop
│   ├── executor/              # Scheduler + worker execution helpers
│   ├── server/                # HTTP API handlers and wiring
│   ├── storage/               # Entity and blob storage
│   ├── tools/                 # Agent tools
│   ├── tui/                   # Bubble Tea TUI
│   └── session/               # Local session persistence
├── portal/                    # Portal web app
├── desktop/frontend/          # Desktop frontend
├── gui/                       # Shared React GUI package
├── design/                    # Design docs
└── .vibe/                     # Task and working docs
```

## Build and Run

Primary local workflow uses the root `./make` script.

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

- `.env.example`
- `internal/config/env_spec.go`
- `design/002-env-config-maintainability.md`

## Important Design Docs

- `design/001-about-portal.md`
- `design/003-store-workspacestorage-reorg.md`
- `design/007-two-tier-agent.md`
- `design/008-backend-architecture-refactor.md`
- `design/009-user-scoped-ownership-refactor.md`

## Current Status

As of the current main branch state:

- server, scheduler, and worker are wired together
- Portal uses conversation/task terminology and user-scoped ownership
- Desktop already supports local chat backed by the shared runtime
- shared GUI is used by both Portal and Desktop
- Go tests pass on the current branch

## Notes

- If a workspace `AGENTS.md` exists, the agent runtime appends it to the system prompt.
- For remote task runs, the worker prepares a run-scoped `AGENTS.md` so the same convention applies in worker execution.
