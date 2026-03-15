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
- **Portal (web)**: A separate web-based entry point under `portal/` — React 19 + Vite + TypeScript; builds and runs independently. Depends on the shared **gui** package for theme and other reusable widgets. See `portal/README.md` for install, build, and dev commands.
- **Desktop (Wails)**: Native desktop app under `desktop/` and `cmd/buildmax-desktop/` — Wails embeds a React 19 + Vite frontend in `desktop/frontend/`. Same shared **gui** package as portal so UI components are implemented once and used in both.
- **Shared GUI package (`gui/`)**: An npm package at repo root (`@buildmax/gui`) that exports presentational React components and CSS (theme, and later e.g. settings UI, chat history rendering, input box). Portal and desktop have different inner logic (data, auth, routing); they share **widgets** from gui via props/callbacks. Both use React 19. Build with `cd gui && npm install && npm run build`; `./make build` and `./make run portal` / `./make run desktop` build or use gui as needed. See `gui/README.md`.

### 2.3 Portal product vision (design reference)

High-level direction for the Portal / Nexus-style workspace (detailed design: **[design/001-about-portal.md](design/001-about-portal.md)** — read that doc on demand for full context):

- **Intent over tools**: User states goals; agent operates on a versioned text workspace (Markdown, CSV, JSON, YAML). Flow: Human → Agent → Workspace → Versioned state.
- **Agent loop**: Observe → Plan → Act → Observe. Agent reads/edits files, runs code, commits; user does not interact with files directly.
- **Workspace model**: Workspace (context) → Task (background task) → TaskRun (single run). No project entity; chats and artifacts are workspace-scoped. Git is the hidden version engine; user sees timeline + restore, not commits/branches.
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
- **TUI**: Bubble Tea via `internal/tui`; default when running `buildmax` with no flags. Layout: scrollable area (banner + version, chat history), input at bottom, footer (model, workspace, ctrl+c: quit). `--resume <id>`, `--continue`, `--session-id <uuid>` for session handling. Session persisted after each assistant reply.
- **Session**: In-memory session in `internal/session` (id, title, created_at, message history); multi-turn; save/load under `DataDir()/sessions/<id>.json`; list index in `sessions.json` via `session.LoadList`.

**Agent & tools**
- **LLM integration**: OpenAI-compatible client in `internal/llm` (OpenRouter default); env-based config (`BUILDMAX_API_KEY`, `BUILDMAX_BASE_URL`, `BUILDMAX_MODEL`).
- **Agent loop**: Single-turn flow with tool calling (LLM → tool_calls → execute tools → re-call LLM → reply) in `internal/agent`; default system prompt prepended in `Process`.
- **Optional workspace AGENTS.md**: When running the CLI, if a file named `AGENTS.md` exists at the workspace root (current working directory), its contents are appended to the default system prompt so the agent receives project-specific instructions. See the [agents.md](https://agents.md/) convention. For remote runs, the worker prepares an `AGENTS.md` in the run directory (run directory layout plus optional workspace `AGENTS.md` from materialized home) so the same convention applies when the shared agent runtime runs with cwd = run dir.
- **Tools** (`internal/tools`): `read_file`, `writefile`, `editfile`, `bash`, `glob`, `grep` (with format), `webfetch`, `todowrite`, `skill`, `agentdef`, `task`; path under configurable root (e.g. CWD). Tool output is LLM-oriented (meaningful on success and failure).

**Config & infra**
- **Application data**: `config.DataDir()` — default `~/.buildmax`, override via `BUILDMAX_HOME`; `make test` uses `testing-sandbox`.
- **Config**: Env-only; `internal/config` provides `LoadLLM()`, `DataDir()`, `SessionsDir()`, `LogsDir()`, `SettingsPath()`, `LoadSettings()`, `MySQLDSN()`, `LoadServerEnv()`, `ResolveServerPort()`, `WorkspacesDir()`, `PersistentWorkspaceDir()`, `RuntimeWorkspaceDir()`, `ArtifactDir()`, `WorkerBinaryPath()`, `WorkerServerURL()`, `WorkerToken()`, `LoadWorkspaceStorageConfig()`, `BuildS3Client()`, `BuildPersistStorage()`, `BuildArtifactStorage()`. Single source of truth for env keys in `env_spec.go`; see `.env.example` and `design/002-env-config-maintainability.md`.
- **Logging**: `log/slog` via `internal/log`; level from `BUILDMAX_LOG_LEVEL`; file-only (rotated under `DataDir()/logs`, Lumberjack); TUI/prompt output stays clean.

**HTTP server & Portal backend**
- **Server** (`buildmax-server` binary): HTTP API in `internal/server`; started via `./make run server` or by running the `buildmax-server` binary. Listen address from `--port` or `BUILDMAX_SERVER_PORT` (default 5678). Requires `BUILDMAX_JWT_SECRET`; optional `BUILDMAX_CORS_ORIGIN` (default `http://localhost:5173`). Optional `BUILDMAX_WORKER_TOKEN` for worker-to-server auth (`/api/worker/*`).
- **Routes**: `GET /healthz`, `GET /openapi.json`, `GET /swagger/`; `POST /api/otp/request`, `POST /api/login`; `GET/POST /api/workspaces`, `GET/POST /api/workspaces/{id}/tasks`, `GET /api/workspaces/{id}/artifacts`, `GET .../artifacts/{task_run_id}/items`, `GET .../artifacts/{task_run_id}/content`; `POST /api/workspaces/{id}/upload`, `GET /api/workspaces/{id}/files`, `GET /api/workspaces/{id}/files/{path...}`; `GET /api/sessions/{session_id}`; **worker API** (chat-run-id only, worker token): `GET /api/worker/task-runs/{task_run_id}`, `PATCH /api/worker/task-runs/{task_run_id}`. JWT auth for user API; workspace-scoped access.
- **Storage**: Entity persistence (MySQL via GORM) in `internal/storage/entity` — the canonical backend model layer for User, Workspace, Agent, Conversation, ConversationMessage, Chat, TaskRun, Artifact, ArtifactItem, quota entities, and related repository interfaces. Blob storage in `internal/storage/blob` — PersistStorage (workspace uploads under **home**), ArtifactStorage (artifact result files); backends: local FS or S3/MinIO; config via `BUILDMAX_PERSIST_STORAGE`, `BUILDMAX_ARTIFACT_STORAGE`, `BUILDMAX_MINIO_*`. See `design/003-store-workspacestorage-reorg.md`. **Workspace on-disk layout**: workspace root has `home/` (uploads); each task run uses `tasks/<taskID>/<taskRunID>/` with `home/` (materialized workspace home), `artifacts/` (run output, e.g. result.md), and `global/` (BUILDMAX_HOME for that run). Blob keys use segment `home` for workspace uploads and `tasks/.../global/...` for run state. The **worker** accesses storage directly for materialize and artifacts; no proxy through server.
- **Executor** (`internal/executor`): **Scheduler** (in server process): polls for PENDING task runs, claims them with the typed run lifecycle API, and spawns the **buildmax-worker** binary with `--chat-run-id` only. **Worker** (`buildmax-worker` binary): gets task run via `GET /api/worker/task-runs/{task_run_id}`, updates status/results via `PATCH`, materializes workspace `home` to run `home`, prepares run-directory `AGENTS.md` (layout + optional workspace AGENTS.md), runs the shared agent runtime in-process with BUILDMAX_HOME = run `global` and cwd = run dir, writes result to run `artifacts/result.md`, uploads run `global` to blob, and streams assistant reply deltas when enabled. Requires `BUILDMAX_SERVER_URL`, `BUILDMAX_WORKER_TOKEN`, and storage env when running the worker.

**Shared GUI and frontends**
- **GUI package** (`gui/`): Shared React 19 package at repo root; exports theme (ThemeProvider, useTheme, ThemeToggle, theme.css) and is the place for other shared widgets (e.g. settings, chat history rendering, input box). Consumed by portal and desktop via `"@buildmax/gui": "file:../gui"` (or `file:../../gui` from desktop/frontend). Build output in `gui/dist/`; `./make build` builds gui before desktop; `./make clean` removes `gui/node_modules` and `gui/dist`.
- **Portal** (`portal/`): React 19 + Vite + TypeScript; depends on `@buildmax/gui` for theme and shared components. Builds independently (`cd portal && npm install && npm run dev` / `npm run build`). Pages: Login, SignUp, Explore, WorkspaceHome, Activity, TaskDetail; API client, AuthContext; AppShell, Sidebar, TopBar, modals. Connects to Go backend for auth and workspace/chat/artifact/file APIs (no project; chats are workspace-scoped).
- **Desktop app**: Wails app in `cmd/buildmax-desktop/` with frontend in `desktop/frontend/` (React 19 + Vite, JSX). Same `@buildmax/gui` dependency as portal; theme and future shared widgets come from gui. Desktop has its own data/runtime (Wails bindings, local session); only the presentational layer is shared with portal.

### 4.2 Tier 1 and Tier 2 architecture

- **Tier 1 = Conversation application service (orchestrator).** The application-layer orchestrator lives in `internal/app/conversation`. It is the single entry point for portal turns: it receives the normalized request, decides whether to run a direct conversation turn or create a background task run, and owns what the user sees. Tier 1 is the single voice to the user.
- **Low-level Tier 1 loop = `internal/conversation`.** The `internal/conversation` package is now the low-level LLM loop used by the app-layer service. It owns message persistence, tool execution, and optional streaming for one turn, but it is not the server composition or transport boundary anymore.
- **Tier 2 = Task + TaskRun (execution in the back).** A Chat with one or more TaskRuns is Tier 2: the worker materializes a run directory, executes the shared agent runtime there, produces artifacts, and can take a long time. Tier 2 is “tools in the back” - it does not send messages directly to the user; it always reports back to Tier 1 (run status, result, artifacts). Tier 1 turns that into what the user sees.
- **Tier 1 tools for Tier 2 (implemented via app services):** Tier 1 can (1) **create a Tier 2 task** through `internal/app/chat` and the `StartChat` tool, which creates a Chat and TaskRun for the worker; and (2) **rerun / follow-up on an existing chat** by creating a new run for that chat. In both cases Tier 2 reports back to Tier 1; Tier 1 orchestrates the reply to the user.

### 4.3 Planned / Not yet implemented

- Session list/delete from CLI; TUI session picker (session list type exists in `internal/session`).
- Config subcommand, Viper, or config-file binding (env-only today).
- Shell completion (e.g. `buildmax completion bash`).

## 5. Project Directory Structure

Following common Golang project conventions, the current structure is:

```
buildmax/
├── gui/                       # Shared GUI package (React 19): theme, future widgets; consumed by portal & desktop
│   ├── package.json           # @buildmax/gui; peerDependencies react ^19
│   ├── src/                   # ThemeContext, ThemeToggle, theme.css, index.ts
│   └── dist/                  # Build output (ESM, .d.ts, theme.css); not committed
├── cmd/
│   ├── buildmax/              # CLI binary (TUI, -p, version)
│   │   └── main.go
│   ├── buildmax-server/       # Server binary (HTTP API + chat-run scheduler)
│   │   └── main.go
│   ├── buildmax-worker/       # Worker binary (runs one task run via API + direct storage)
│   │   └── main.go
│   └── buildmax-desktop/       # Desktop app (Wails); embeds desktop/frontend
│       └── main.go
├── desktop/                   # Desktop frontend (React 19 + Vite); depends on gui
│   └── frontend/              # Wails UI; package.json has "file:../../gui"
├── internal/                  # Private packages (this project only)
│   ├── app/                   # Application-layer orchestration services shared by transports/runtime
│   │   ├── agentrun/          # Shared agent runtime used by CLI and worker
│   │   ├── chat/              # Chat application service (create task, create run, background task)
│   │   └── conversation/      # Tier 1 orchestration entry point for portal turns
│   ├── cmd/                   # Cobra root, flags, version; prompt/TUI runners, setup (CLI only; no server)
│   ├── tui/                   # Bubble Tea TUI (models, views, program entry: banner, input, viewport, format)
│   ├── agent/                 # Core Agent logic (Process, tools, system prompt, subagent runner)
│   ├── conversation/          # Low-level Tier 1 LLM loop (message persistence, tool execution, streaming)
│   ├── llm/                   # LLM client (OpenAI-compatible), types, ChatWithTools
│   ├── config/                # Config: LoadLLM(), DataDir(), MySQLDSN(), server env, workspace paths, workspace storage (env_spec.go, workspace_storage.go)
│   ├── log/                   # slog init, BUILDMAX_LOG_LEVEL, rotated file
│   ├── session/               # Session (id, title, history), SaveToDir, LoadFromDir, list index (sessions.json)
│   ├── tools/                 # Tool implementations (readfile, writefile, editfile, bash, glob, grep, webfetch, todowrite, skill, agentdef, task)
│   ├── util/                  # ID generation (prefixed IDs: u_, w_, c_, r_, ar_, f_), workspace helpers, git, argparse
│   ├── storage/               # Persistence under one namespace
│   │   ├── entity/            # MySQL (GORM): canonical backend models, typed lifecycle commands, interfaces and Store
│   │   └── blob/              # Blob/file storage: PersistStorage (workspace home), ArtifactStorage; local FS and S3; keys use home, chats/.../global
│   ├── server/                # HTTP server: routes, auth (JWT, OTP), workspaces, chats, artifacts, upload, files, sessions; static (openapi, swagger)
│   ├── servercmd/             # Server startup: RunServer (config, DB, blob, executor.NewScheduler, server.Run); used by cmd/buildmax-server
│   ├── workercmd/             # Worker startup: RunWorker (env, get task run via API, blob storage, executor.RunTask); used by cmd/buildmax-worker
│   └── executor/              # Scheduler: polls and spawns buildmax-worker; RunTask: run-scoped dirs (home, artifacts, global), materialize, shared runtime execution, TaskRunUpdater (API)
├── portal/                    # Web UI (React 19 + Vite + TypeScript); depends on gui
│   ├── package.json           # Scripts: dev, build; dependency "@buildmax/gui": "file:../gui"
│   ├── vite.config.ts         # Vite config (build out: dist/)
│   ├── index.html             # Vite entry HTML
│   ├── README.md              # Install, build, dev instructions
│   └── src/                   # App, router; pages; components; lib; contexts (Auth); theme from @buildmax/gui
├── design/                    # Design docs (001-about-portal, 002-env-config, 003-store-workspacestorage-reorg)
├── configs/                   # Config file examples
├── example/                   # Example files for tools (e.g. shakespeare.txt)
├── setup/                     # Setup scripts (e.g. setup.sh, port-forward)
├── deployment/                # Application manifests (e.g. buildmax-deploy.yaml for kind)
├── .vibe/                     # Task documents and design docs (vibe lifecycle)
├── make.bat                   # Windows: build, test, run (build uses cmd/buildmax)
├── go.mod
├── go.sum
├── .env.example               # Env var template (sync with config/env_spec.go)
└── README.md
```

- **cmd/buildmax**: CLI entry point; `main.go` only. Build with `go build -o buildmax ./cmd/buildmax` or `make.bat build`. Provides TUI, `-p` print mode, and `version`.
- **cmd/buildmax-server**: Server entry point; `main.go` only. Build with `go build -o buildmax-server ./cmd/buildmax-server`. Runs the HTTP API and chat-run scheduler (spawns `buildmax-worker` per pending task run); use `./make run server` to build and run it.
- **cmd/buildmax-worker**: Worker entry point; `main.go` only. Accepts `--chat-run-id`; calls `workercmd.RunWorker` (get task run via API, blob storage, executor.RunTask). Requires `BUILDMAX_SERVER_URL`, `BUILDMAX_WORKER_TOKEN`, `BUILDMAX_WORKSPACES_DIR`, and storage env when running the worker.
- **internal/cmd**: Cobra root command, version subcommand, CLI flags, prompt mode and TUI runners, internal setup for agent/session. No server subcommand.
- **internal/app**: Application services that own orchestration boundaries: shared agent runtime, task workflows, and Tier 1 conversation handling.
- **internal/storage**: Entity persistence (DB) in `entity/`; blob/file storage in `blob/`. See `design/003-store-workspacestorage-reorg.md`.
- **internal/conversation**: Low-level conversation loop primitives used by `internal/app/conversation`; not the transport-layer orchestration entry point.
- **internal/server**: HTTP API for the Portal; depends on `app/chat`, `app/conversation`, `storage/entity`, `storage/blob`, `config`; executor is started by the server binary via `internal/servercmd`.
- **internal/**: Packages not exposed externally; can be split or partially moved to **pkg/** later.
- **gui/**: Shared React package; build with `cd gui && npm install && npm run build` (output in `gui/dist/`). Portal and desktop depend on it via npm `file:`; `./make build` builds gui first when building desktop. Implement shared widgets (theme, settings, chat history, input box) here so portal and desktop do not duplicate UI.
- **portal/**: Frontend app; depends on `@buildmax/gui`. Run with `cd portal && npm install && npm run dev`; build with `npm run build` (output in `portal/dist/`). No change to `go.mod` or Go build/test.
- **desktop/frontend/**: Desktop UI; depends on `@buildmax/gui`. Same React 19 and shared components as portal; app logic (Wails bindings, session) is desktop-specific.
- **design/**: Product and technical design docs; see section 6.

## 6. Documentation and Repository

- **Task docs**: `.vibe/NNN.md` (e.g. `.vibe/001.md`); design docs `.vibe/NNN-design.md`. TOC: `.vibe/000-TOC.md`.
- **Design docs** (in `design/`): [design/001-about-portal.md](design/001-about-portal.md) — Portal/Nexus product vision, concepts, principles, wireframe; [design/002-env-config-maintainability.md](design/002-env-config-maintainability.md) — env-based config; [design/003-store-workspacestorage-reorg.md](design/003-store-workspacestorage-reorg.md) — storage layout (entity/blob). **Workspace on-disk layout** (home, run dirs home/artifacts/global): `.vibe/075-design.md`.
- **Env reference**: `.env.example` and `internal/config/env_spec.go` (single source of truth for env keys).
- Code and scripts: repository root, managed with Go modules.

### 6.1 Persistence naming style

- **Use the same naming style for all persisted data** (e.g. session files, config, any JSON on disk).
- **Convention: snake_case** for JSON object keys (e.g. `created_at`, `tool_call_id`, `tool_calls`).
- Ensure structs that are serialized to disk have explicit `json:"snake_case"` tags so the on-disk format is consistent; do not rely on Go’s default (PascalCase) for persisted fields.

### 6.2 Database table naming

- **Use singular table names.** One table per entity type, named in the singular (e.g. `user`, `workspace`, `chat`, `task_run`, `artifact`). Do not use plural names (e.g. `users`, `workspaces`). This applies to all database tables created or migrated by the project.

### 6.3 Entity ID format

- **Entity IDs use a prefixed format** `<prefix>_<body>`: prefix is a short type abbreviation (e.g. `u_` user, `w_` workspace, `a_` agent, `c_` chat, `r_` task run, `ar_` artifact, `f_` artifact item), body is 20 characters from `[a-z0-9]` (lowercase base36). Generated via `internal/util.NewPrefixedID(prefix)`; ordering uses `created_at`, not ID. See `.vibe/074-design.md` for full semantics.

### 6.4 Tool output for LLM

- **Tools are built for the LLM.** The agent passes tool results back to the model as tool-role messages.
- **Output meaningful results on both success and failure** so the LLM can understand what happened and decide on next steps (e.g. retry, inform the user, or continue).
- Success: return a clear, concise message (e.g. what was done or what was returned).
- Failure: return a clear error message (e.g. "path outside allowed root", "file not found"); the agent prefixes tool errors with `error: ` when sending to the LLM.

## 7. Build & Test

**Primary (macOS/Unix):** Use the `./make` script (bash) at the repo root. It sources `./loadenv` if present.

- **Build**: `./make build` — builds the CLI (`./buildmax`), server (`./buildmax-server`), worker (`./buildmax-worker`), then the **gui** package (if `gui/` exists), then the desktop app (Wails). Portal is not built by default; run `cd portal && npm run build` for that. To build only one: `go build -o buildmax ./cmd/buildmax`, etc.
- **Clean**: `./make clean` — removes binaries, desktop build dir, **gui** (`gui/node_modules`, `gui/dist`), portal, and desktop frontend (`node_modules`, `dist`).
- **Test**: `./make test` — sets `BUILDMAX_HOME=./testing-sandbox` and runs `go test ./...`. Use this after code changes.
- **Smoke**: `./make smoke` — builds the CLI, then runs `./buildmax -p "/smoke 0"` with `BUILDMAX_HOME=testing-sandbox` (manual sanity check).
- **Run server**: `./make run server` — builds and runs `buildmax-server` (default port 5678). The server spawns `buildmax-worker` for each task; ensure `buildmax-worker` is on PATH or in the same directory, and set `BUILDMAX_SERVER_URL` and `BUILDMAX_WORKER_TOKEN` (and storage env) when running the worker.
- **Run portal**: `./make run portal` — builds gui if missing, then starts the Portal dev server (Vite; installs npm deps if needed).
- **Run desktop**: `./make run desktop` — builds gui if missing, then starts the desktop app in Wails dev mode (installs frontend deps if needed).
- **Bump version**: `./make bump [patch|minor|major]` — updates `Version` in `internal/cmd/root.go` (default: patch).
- **Setup / Unsetup**: `./make setup` runs `setup/setup.sh` (one-click local dev: kind cluster, MinIO, MySQL, port-forwards, test job; idempotent). `./make unsetup` runs `setup/unsetup.sh` to tear down. Requires Homebrew (kind, helm, kubectl, awscli). Do not use `./make run server` or `./make smoke` in automated CI; they are for local manual use.

**Windows:** `make.bat` in the repo root provides `build` and `test` for Windows; prefer PowerShell over batch when running commands. Build output: `buildmax.exe`.

---

*This document is updated as the project evolves.*
