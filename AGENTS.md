# BuildMax - Project Design Document

## 1. Overview

BuildMax is an **AI Agent project** aimed at building a **general-purpose Agent** so that users can:

- **Run quickly**: Out of the box with minimal configuration and dependencies
- **Get AI Agent capabilities**: Typical Agent features such as LLM interaction, task planning, and tool calling

The project targets users who want to deploy AI Agents locally or privately, providing a unified, extensible Agent runtime.

## 2. Technology Choices

### 2.1 Language and Ecosystem

- **Primary language: Golang (Go)**
- **Principle: Implement all components in Golang where possible**, including:
  - Core Agent logic
  - CLI and TUI interface
  - LLM communication and plugin/tool wrappers
  - Infrastructure such as config, logging, and persistence

Rationale: A single language reduces maintenance cost, enables cross-compilation and single-binary distribution, and facilitates collaboration and contribution.

### 2.2 User Interface

- **CLI/TUI (primary)**: Command-line program with TUI (Text User Interface).
  - **Implementation**: Based on [Bubble Tea](https://github.com/charmbracelet/bubbletea)
  - Pure Go TUI framework, aligned with the project’s “all-Go stack”
  - Supports multiple components, message-driven flow, and keyboard/mouse interaction.
  - Users get a full Agent TUI experience in the terminal by running a single binary; no Node dependency for normal CLI use.
- **Portal (web)**: A separate web-based entry point under `portal/` — a minimal React (Vite + TypeScript) app that builds and runs independently. It provides a "BuildMax Portal" landing as a frontend foundation; chat, sessions, and API integration are planned for later. See `portal/README.md` for install, build, and dev commands.

### 2.3 Portal product vision (design reference)

High-level direction for the Portal / Nexus-style workspace (detailed design: **[design/001-about-portal.md](design/001-about-portal.md)** — read that doc on demand for full context):

- **Intent over tools**: User states goals; agent operates on a versioned text workspace (Markdown, CSV, JSON, YAML). Flow: Human → Agent → Workspace → Versioned state.
- **Agent loop**: Observe → Plan → Act → Observe. Agent reads/edits files, runs code, commits; user does not interact with files directly.
- **Workspace model**: Workspace (context) → Project (work unit) → Task (single run). Git is the hidden version engine; user sees timeline + restore, not commits/branches.
- **Principles**: Intent first; text as primary representation; state versioned and reversible; mechanisms hidden, meaning visible; workspace as the agent’s body.
- **Mental model**: User feels “I describe what I want” and “I can always go back,” not “I am operating software” or “I am managing versions.”

## 3. Goals and Principles

| Goal | Description |
|------|-------------|
| Generality | Agent can be configured to use different LLMs and tools, not tied to a single service |
| Ease of use | Runs with default configuration; advanced users can extend models and tools |
| Portability | Single binary or few files, easy to deploy on servers, local machines, or containers |
| All-Go implementation | Core and surrounding code in Go; call external APIs from Go when needed; no Python/Node runtime dependencies for CLI/TUI. The portal is an optional, separate frontend (React/Node tooling for dev and build). |

## 4. Core Capabilities

### 4.1 Implemented

**CLI & TUI**
- **CLI**: Cobra in `internal/cmd` — root command (TUI or `-p`/`--resume` prompt mode), `buildmax version` subcommand; `cmd/buildmax/main.go` is the thin entry point. The server runs as a separate binary (`buildmax-server`).
- **TUI**: Bubble Tea via `internal/app` + `internal/tui`; default when running `buildmax` with no flags. Layout: scrollable area (banner + version, chat history), input at bottom, footer (model, workspace, ctrl+c: quit). `--resume <id>`, `--continue`, `--session-id <uuid>` for session handling. Session persisted after each assistant reply.
- **Session**: In-memory session in `internal/session` (id, title, created_at, message history); multi-turn; save/load under `DataDir()/sessions/<id>.json`; list index in `sessions.json` via `session.LoadList`.

**Agent & tools**
- **LLM integration**: OpenAI-compatible client in `internal/llm` (OpenRouter default); env-based config (`BUILDMAX_API_KEY`, `BUILDMAX_BASE_URL`, `BUILDMAX_MODEL`).
- **Agent loop**: Single-turn flow with tool calling (LLM → tool_calls → execute tools → re-call LLM → reply) in `internal/agent`; default system prompt prepended in `Process`.
- **Tools** (`internal/tools`): `read_file`, `writefile`, `editfile`, `bash`, `glob`, `grep` (with format), `webfetch`, `todowrite`, `skill`, `agentdef`, `task`; path under configurable root (e.g. CWD). Tool output is LLM-oriented (meaningful on success and failure).

**Config & infra**
- **Application data**: `config.DataDir()` — default `~/.buildmax`, override via `BUILDMAX_HOME`; `make test` uses `testing-sandbox`.
- **Config**: Env-only; `internal/config` provides `LoadLLM()`, `DataDir()`, `SessionsDir()`, `LogsDir()`, `SettingsPath()`, `LoadSettings()`, `MySQLDSN()`, `LoadServerEnv()`, `ResolveServerPort()`, `WorkspacesDir()`, `PersistentWorkspaceDir()`, `RuntimeWorkspaceDir()`, `ArtifactDir()`, `WorkerBinaryPath()`, `WorkerServerURL()`, `WorkerToken()`, `LoadWorkspaceStorageConfig()`, `BuildS3Client()`, `BuildPersistStorage()`, `BuildArtifactStorage()`. Single source of truth for env keys in `env_spec.go`; see `.env.example` and `design/002-env-config-maintainability.md`.
- **Logging**: `log/slog` via `internal/log`; level from `BUILDMAX_LOG_LEVEL`; file-only (rotated under `DataDir()/logs`, Lumberjack); TUI/prompt output stays clean.

**HTTP server & Portal backend**
- **Server** (`buildmax-server` binary): HTTP API in `internal/server`; started via `./make run server` or by running the `buildmax-server` binary. Listen address from `--port` or `BUILDMAX_SERVER_PORT` (default 5678). Requires `BUILDMAX_JWT_SECRET`; optional `BUILDMAX_CORS_ORIGIN` (default `http://localhost:5173`). Optional `BUILDMAX_WORKER_TOKEN` for worker-to-server auth (`/api/worker/*`).
- **Routes**: `GET /healthz`, `GET /openapi.json`, `GET /swagger/`; `POST /api/otp/request`, `POST /api/login`; `GET/POST /api/workspaces`, `GET/POST /api/workspaces/{id}/projects`, `GET/POST /api/workspaces/{id}/tasks`, `GET /api/workspaces/{id}/artifacts`, `GET .../artifacts/{id}/items`, `GET .../artifacts/{id}/content`; `POST /api/workspaces/{id}/upload`, `GET /api/workspaces/{id}/files`, `GET /api/workspaces/{id}/files/{path...}`; `GET /api/sessions/{session_id}`; **worker API** (task-id-only, worker token): `GET /api/worker/tasks/{task_id}`, `PATCH /api/worker/tasks/{task_id}`. JWT auth for user API; workspace-scoped access.
- **Storage**: Entity persistence (MySQL via GORM) in `internal/storage/entity` — User, Workspace, Project, Task, Artifact, ArtifactItem. Blob storage in `internal/storage/blob` — PersistStorage (uploads, Explore files) and ArtifactStorage (artifact result files); backends: local FS or S3/MinIO; config via `BUILDMAX_PERSIST_STORAGE`, `BUILDMAX_ARTIFACT_STORAGE`, `BUILDMAX_MINIO_*`. See `design/003-store-workspacestorage-reorg.md`. The **worker** accesses MinIO (or local storage) directly for materialize and artifact blobs; no proxy through server.
- **Executor** (`internal/executor`): **Scheduler** (in server process): polls for PENDING tasks, spawns the **buildmax-worker** binary with `--task-id` only. **Worker** (`buildmax-worker` binary): gets task via `GET /api/worker/tasks/{task_id}`, updates status/results via `PATCH`, uses direct storage for materialize and artifacts, runs `buildmax -p` to execute the task. Requires `BUILDMAX_SERVER_URL`, `BUILDMAX_WORKER_TOKEN`, and storage env when running the worker.

**Portal (web)**
- **Portal** (`portal/`): React + Vite + TypeScript; builds independently (`cd portal && npm install && npm run dev` / `npm run build`). Pages: Login, SignUp, Explore (files), Projects, Project, Activity, TaskDetail; API client in `lib/api.ts`; AuthContext; AppShell, LeftSidebar, TopBar, modals (CreateWorkspace, CreateProject, ArtifactContent). Connects to Go backend for auth and workspace/project/task/artifact/file APIs.

### 4.2 Planned / Not yet implemented

- Session list/delete from CLI; TUI session picker (session list type exists in `internal/session`).
- Config subcommand, Viper, or config-file binding (env-only today).
- Shell completion (e.g. `buildmax completion bash`).

## 5. Project Directory Structure

Following common Golang project conventions, the current structure is:

```
buildmax/
├── cmd/
│   ├── buildmax/              # CLI binary (TUI, -p, version)
│   │   └── main.go
│   ├── buildmax-server/       # Server binary (HTTP API + task scheduler)
│   │   └── main.go
│   └── buildmax-worker/       # Worker binary (runs one task via API + direct storage)
│       └── main.go
├── internal/                  # Private packages (this project only)
│   ├── cmd/                   # Cobra root, flags, version; prompt/TUI runners, setup (CLI only; no server)
│   ├── app/                   # App bootstrap and TUI program entry
│   ├── tui/                   # Bubble Tea models and views (banner, input, viewport, format)
│   ├── agent/                 # Core Agent logic (Process, tools, system prompt, subagent runner)
│   ├── llm/                   # LLM client (OpenAI-compatible), types, ChatWithTools
│   ├── config/                # Config: LoadLLM(), DataDir(), MySQLDSN(), server env, workspace paths, workspace storage (env_spec.go, workspace_storage.go)
│   ├── log/                   # slog init, BUILDMAX_LOG_LEVEL, rotated file
│   ├── session/               # Session (id, title, history), SaveToDir, LoadFromDir, list index (sessions.json)
│   ├── tools/                 # Tool implementations (readfile, writefile, editfile, bash, glob, grep, webfetch, todowrite, skill, agentdef, task)
│   ├── util/                  # ID generation, workspace helpers, git, argparse
│   ├── model/                 # Shared domain types (User, Workspace, Project, Task, Artifact, ArtifactItem, ArtifactWithTask)
│   ├── storage/               # Persistence under one namespace
│   │   ├── entity/            # MySQL (GORM): User, Workspace, Project, Task, Artifact; interfaces and Store
│   │   └── blob/              # Blob/file storage: PersistStorage, ArtifactStorage; local FS and S3 adapters; keys, relpath
│   ├── server/                # HTTP server: routes, auth (JWT, OTP), workspaces, projects, tasks, artifacts, upload, files, sessions; static (openapi, swagger)
│   ├── servercmd/             # Server startup: RunServer (config, DB, blob, executor.NewRunner, server.Run); used by cmd/buildmax-server
│   └── executor/              # Scheduler: Runner polls and spawns buildmax-worker; RunTask (worker): materialize, buildmax -p, TaskUpdater (API); WorkerHTTPUpdater, GetWorkerTask
├── portal/                    # Web UI (React + Vite + TypeScript); independent of Go binary
│   ├── package.json           # Scripts: dev, build, preview
│   ├── vite.config.ts         # Vite config (build out: dist/)
│   ├── index.html             # Vite entry HTML
│   ├── README.md              # Install, build, dev instructions
│   └── src/                   # App, router; pages (Login, SignUp, Explore, Projects, Project, Activity, TaskDetail); components; lib (api, types); contexts (Auth)
├── design/                    # Design docs (001-about-portal, 002-env-config, 003-store-workspacestorage-reorg)
├── configs/                   # Config file examples
├── example/                   # Example files for tools (e.g. shakespeare.txt)
├── setup/                     # Setup scripts (e.g. setup.sh, port-forward)
├── .vibe/                     # Task documents and design docs (vibe lifecycle)
├── make.bat                   # Windows: build, test, run (build uses cmd/buildmax)
├── go.mod
├── go.sum
├── .env.example               # Env var template (sync with config/env_spec.go)
└── README.md
```

- **cmd/buildmax**: CLI entry point; `main.go` only. Build with `go build -o buildmax ./cmd/buildmax` or `make.bat build`. Provides TUI, `-p` print mode, and `version`.
- **cmd/buildmax-server**: Server entry point; `main.go` only. Build with `go build -o buildmax-server ./cmd/buildmax-server`. Runs the HTTP API and task scheduler (spawns `buildmax-worker` per task); use `./make run server` to build and run it.
- **cmd/buildmax-worker**: Worker entry point; `main.go` only. Accepts `--task-id`; gets task via `GET /api/worker/tasks/{task_id}`, updates status via `PATCH`, uses direct storage, runs `buildmax -p`. Requires `BUILDMAX_SERVER_URL`, `BUILDMAX_WORKER_TOKEN`, `BUILDMAX_WORKSPACES_DIR`, and storage env.
- **internal/cmd**: Cobra root command, version subcommand, CLI flags, prompt mode and TUI runners, internal setup for agent/session. No server subcommand.
- **internal/storage**: Entity persistence (DB) in `entity/`; blob/file storage in `blob/`. See `design/003-store-workspacestorage-reorg.md`.
- **internal/server**: HTTP API for the Portal; depends on `storage/entity`, `storage/blob`, `config`; executor is started by the server binary via `internal/servercmd`.
- **internal/**: Packages not exposed externally; can be split or partially moved to **pkg/** later.
- **portal/**: Frontend app; run with `cd portal && npm install && npm run dev`; build with `npm run build` (output in `portal/dist/`). No change to `go.mod` or Go build/test.
- **design/**: Product and technical design docs; see section 6.

## 6. Documentation and Repository

- **Task docs**: `.vibe/NNN.md` (e.g. `.vibe/001.md`); design docs `.vibe/NNN-design.md`. TOC: `.vibe/000-TOC.md`.
- **Design docs** (in `design/`): [design/001-about-portal.md](design/001-about-portal.md) — Portal/Nexus product vision, concepts, principles, wireframe; [design/002-env-config-maintainability.md](design/002-env-config-maintainability.md) — env-based config; [design/003-store-workspacestorage-reorg.md](design/003-store-workspacestorage-reorg.md) — storage layout (entity/blob).
- **Env reference**: `.env.example` and `internal/config/env_spec.go` (single source of truth for env keys).
- Code and scripts: repository root, managed with Go modules.

### 6.1 Persistence naming style

- **Use the same naming style for all persisted data** (e.g. session files, config, any JSON on disk).
- **Convention: snake_case** for JSON object keys (e.g. `created_at`, `tool_call_id`, `tool_calls`).
- Ensure structs that are serialized to disk have explicit `json:"snake_case"` tags so the on-disk format is consistent; do not rely on Go’s default (PascalCase) for persisted fields.

### 6.2 Database table naming

- **Use singular table names.** One table per entity type, named in the singular (e.g. `user`, `workspace`, `project`, `task`). Do not use plural names (e.g. `users`, `workspaces`). This applies to all database tables created or migrated by the project.

### 6.3 Tool output for LLM

- **Tools are built for the LLM.** The agent passes tool results back to the model as tool-role messages.
- **Output meaningful results on both success and failure** so the LLM can understand what happened and decide on next steps (e.g. retry, inform the user, or continue).
- Success: return a clear, concise message (e.g. what was done or what was returned).
- Failure: return a clear error message (e.g. "path outside allowed root", "file not found"); the agent prefixes tool errors with `error: ` when sending to the LLM.

## 7. Build & Test

**Primary (macOS/Unix):** Use the `./make` script (bash) at the repo root. It sources `./loadenv` if present.

- **Build**: `./make build` — builds the CLI (`./buildmax`), server (`./buildmax-server`), and worker (`./buildmax-worker`) from `cmd/buildmax`, `cmd/buildmax-server`, and `cmd/buildmax-worker`. To build only one: `go build -o buildmax ./cmd/buildmax`, etc.
- **Test**: `./make test` — sets `BUILDMAX_HOME=./testing-sandbox` and runs `go test ./...`. Use this after code changes.
- **Smoke**: `./make smoke` — builds the CLI, then runs `./buildmax -p "/smoke 0"` with `BUILDMAX_HOME=testing-sandbox` (manual sanity check).
- **Run server**: `./make run server` — builds and runs `buildmax-server` (default port 5678). The server spawns `buildmax-worker` for each task; ensure `buildmax-worker` is on PATH or in the same directory, and set `BUILDMAX_SERVER_URL` and `BUILDMAX_WORKER_TOKEN` (and storage env) when running the worker.
- **Run portal**: `./make run portal` — starts the Portal dev server (Vite; installs npm deps if needed).
- **Bump version**: `./make bump [patch|minor|major]` — updates `Version` in `internal/cmd/root.go` (default: patch).
- **Setup / Unsetup**: `./make setup` runs `setup/setup.sh` (one-click local dev: kind cluster, MinIO, MySQL, port-forwards, test job; idempotent). `./make unsetup` runs `setup/unsetup.sh` to tear down. Requires Homebrew (kind, helm, kubectl, awscli). Do not use `./make run server` or `./make smoke` in automated CI; they are for local manual use.

**Windows:** `make.bat` in the repo root provides `build` and `test` for Windows; prefer PowerShell over batch when running commands. Build output: `buildmax.exe`.

---

*This document is updated as the project evolves.*
