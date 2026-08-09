# BuildMax - Project Design Document

## 1. Overview

BuildMax is an **AI Agent project** aimed at building a **general-purpose Agent** so that users can:

- **Run quickly**: Out of the box with minimal configuration and dependencies
- **Get AI Agent capabilities**: Typical Agent features such as LLM interaction, task planning, and tool calling

The project targets users who want to deploy AI Agents locally or privately, providing a unified, extensible Agent runtime.

**Active roadmap**: `ROADMAP.md` at the repository root is the current
near-term product roadmap and priority order. Design docs under `docs/design/`
support that roadmap with product/technical detail and historical context.
When roadmap language and older design docs conflict, prefer `ROADMAP.md` plus
the current codebase.

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
- **Shared GUI package (`gui/`)**: An npm package at repo root (`@buildmax/gui`) that exports presentational React components and CSS. It already includes shared theme, modal, avatar, chat thread/composer, recent-list, and form-modal building blocks. Portal and desktop have different inner logic (data, auth, routing); they share **widgets** from gui via props/callbacks. Both use React 19. Build with `cd gui && npm install && npm run build`; `./make build` and `./make run portal` / `./make run desktop` build or use gui as needed. See `gui/README.md`.

### 2.3 Portal product vision (design reference)

High-level direction for the Portal / Nexus-style workspace (detailed design: **[docs/design/001-about-portal.md](docs/design/001-about-portal.md)** — read that doc on demand for full context):

- **Intent over tools**: User states goals; agent operates on a versioned text workspace (Markdown, CSV, JSON, YAML). Flow: Human → Agent → Workspace → Versioned state.
- **Agent loop**: Observe → Plan → Act → Observe. Agent reads/edits files, runs code, commits; user does not interact with files directly.
- **Ownership model**: Team → Conversation → Task → TaskRun. Personal usage is represented by a default personal team (`My Space`). Issues, agents, workflows, conversations, tasks, and uploaded files are team-scoped. Webhook keys remain user-scoped for now. No project entity. Git is the hidden version engine; user sees timeline + restore, not commits/branches.
- **Principles**: Intent first; text as primary representation; state versioned and reversible; mechanisms hidden, meaning visible; the team-owned file space is the agent’s body.
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
- **CLI**: Cobra in `internal/interface/cli` — root command (TUI or `-p`/`--resume` prompt mode), `buildmax version` subcommand; `cmd/buildmax/main.go` is the thin entry point. The server and worker run as separate binaries.
- **TUI**: Bubble Tea via `internal/interface/cli` (`tui.go`); default when running `buildmax` with no flags. Layout: scrollable area (banner + version, chat history), input at bottom, footer (model, workspace, ctrl+c: quit). `--resume <id>`, `--continue`, `--session-id <uuid>` for session handling. Session persisted after each assistant reply.
- **Local auth**: CLI/desktop-side login client and credential persistence live in `internal/interface/auth`; server-side auth handlers live in `internal/server/handlers/auth.go`.
- **Session**: In-memory session in `internal/core/session` (id, title, created_at, message history); multi-turn; save/load under `DataDir()/sessions/<id>.json`; list index in `sessions.json` via `session.LoadList`.

**Agent & tools**
- **LLM integration**: OpenAI-compatible implementation in `internal/infra/llm` (OpenRouter default); shared LLM contracts such as `Message`, `ToolCall`, `Usage`, and `LLMClient` live in `internal/core/model`; env-based config (`BUILDMAX_API_KEY`, `BUILDMAX_BASE_URL`, `BUILDMAX_MODEL`) is loaded by `internal/config`.
- **Agent loop**: Shared tool-calling loop (LLM -> tool_calls -> execute tools -> re-call LLM -> reply) in `internal/core/agent`; default system prompt prepended by the core agent run.
- **Agent runtime assembly**: Runtime wiring for CLI, worker, and desktop lives in `internal/agentapp`; process bootstrapping lives in `internal/bootstrap/*`. `AgentApp` owns the LLM client cache, tool registry construction, MCP manager, skill/subagent discovery, session persistence, and workspace resolution.
- **Optional workspace AGENTS.md**: When running the CLI, if a file named `AGENTS.md` exists at the workspace root (current working directory), its contents are appended to the default system prompt so the agent receives project-specific instructions. See the [agents.md](https://agents.md/) convention. For remote runs, the worker prepares an `AGENTS.md` in the run directory (run directory layout plus optional workspace `AGENTS.md` from materialized home) so the same convention applies when the shared agent runtime runs with cwd = run dir.
- **Runtime tools** (`internal/tool`): `read_file`, `writefile`, `editfile`, `bash`, `glob`, `grep` (with format), `webfetch`, `todowrite`, `skill`, `agentdef`, `task`; path under configurable root (e.g. CWD). MCP gateway tools (`mcp_gateway.go`) also live here — no separate mcptool package. Tool output is LLM-oriented (meaningful on success and failure). Conversation-specific tools such as `StartTask`, `ContinueTask`, `ListTasks`, and `GetTask` live in the Tier 1 conversation subsystem under `internal/service/conversation/tool`.
- **MCP tools**: MCP protocol/client transport lives in `internal/infra/mcp`; agent-facing MCP gateway tools live in `internal/tool` (`mcp_gateway.go`).
- **Runtime hooks**: lifecycle hooks (P0.5 trust harness). `HookManager` (`internal/agentapp/hook_manager.go`) merges global `hooks:` from `settings.yaml` with workspace `<workspace>/.buildmax/hooks.yaml` and dispatches to per-transport drivers in `internal/infra/hook` (`command`, `http`, `mcp_tool`, `prompt`). 13 events shipped — `SessionStart/End`, `UserPromptSubmit` (gating), `PreToolUse` (gating), `PostToolUse`/`PostToolUseFailure`, `Notification`, `PreCompact` (gating)/`PostCompact`, `SubagentStart/Stop`, `Stop`/`StopFailure`. Block contract: command exit 2, HTTP 422, or response JSON `{"decision":"block","reason":"..."}`. Failures fail open. Subagents inherit the parent HookManager and stamp every event with `is_subagent`/`agent_type`. Design: `docs/design/031-hook-system-v2.md`. Examples: `config-examples/settings.example.yaml` and `config-examples/hooks.workspace.example.yaml`.
- **Bash sandbox** (P0.5 trust harness §3.2): bash subprocesses can run under an OS sandbox — Seatbelt on macOS, `bwrap` on Linux/WSL2, unavailable elsewhere. Non-bash tools keep their existing permission boundary (`util.ResolvePath`, `tool/safety.go`, `agentapp/policy.go`). Contract in `internal/core/agent/sandbox.go` (`SandboxView`), backends and proxy in `internal/infra/sandbox`, assembly in `internal/agentapp/sandbox.go`. Config is a `sandbox:` block in `settings.yaml`, overridable by `<BUILDMAX_HOME>/policy.yaml` and `BUILDMAX_SANDBOX_ENABLED`; boundaries cover filesystem allow/deny paths, domain allow/deny via a Go-side HTTP/SOCKS proxy, and scrubbing of secret-shaped env vars. Modes: `auto_allow` (sandboxed bash skips the prompt) and `regular`. Per-call escape hatch `dangerously_disable_sandbox`, honored only when `allow_unsandboxed_commands` is true. Inspect with `buildmax sandbox status|deps|mode|enable|disable`; the TUI footer shows the active mode. **Default off on all surfaces today** — the stricter worker default, process rlimits, and hook-transport enforcement are not wired yet (see `docs/design/032-sandbox-and-execution-boundaries.md` §13.1). Design: `docs/design/032-sandbox-and-execution-boundaries.md`.
- **Durable run traces** (P0.5 trust harness §3.3): every run persists the agent event stream as a bounded, redacted JSONL trace. The recorder lives in `internal/infra/trace` and is attached at the single `agentapp.RunPrompt` chokepoint (tees the `EventSink`), so CLI/TUI, Desktop, eval, and worker runs all produce traces. Layout: `<DataDir>/traces/<session_id>/<run_id>.jsonl` (run id prefix `rt_`) with a `run_start` record, per-iteration `llm_*`/`tool_*`/`context_compacted` records, and a terminal `run_end`; `RunResult.TraceID` points at the file. Free-text fields are truncated and common secret shapes redacted (`internal/infra/trace/redact.go`). On by default; disable with `BUILDMAX_TRACE_DISABLED`. Fail-open: a trace failure never breaks or slows a run. Design: `docs/design/034-durable-run-trace.md`.

**Config & infra**
- **Application data**: `config.DataDir()` — default `~/.buildmax`, override via `BUILDMAX_HOME`; `make test` uses `testing-sandbox`.
- **Config**: Env-only; `internal/config` provides env loading and path resolution such as `LoadLLM()`, `DataDir()`, `SessionsDir()`, `LogsDir()`, `SettingsPath()`, `LoadSettings()`, `MySQLDSN()`, `LoadServerEnv()`, `ResolveServerPort()`, `WorkspacesDir()`, `PersistentWorkspaceDir()`, `RuntimeWorkspaceDir()`, `ArtifactDir()`, `WorkerBinaryPath()`, `WorkerServerURL()`, `WorkerToken()`, and `LoadWorkspaceStorageConfig()`. Startup-time wiring helpers live under `internal/bootstrap/*`; config does not import infra implementations. Single source of truth for env keys in `env_spec.go`; see `config-examples/.env.example` and `docs/design/002-env-config-maintainability.md`.
- **Logging**: `log/slog` via `internal/infra/log`; level from `BUILDMAX_LOG_LEVEL`; file-only (rotated under `DataDir()/logs`, Lumberjack); TUI/prompt output stays clean.

**HTTP server & Portal backend**
- **Server** (`buildmax-server` binary): HTTP API in `internal/server`, bootstrapped by `internal/bootstrap`; started via `./make run server` or by running the `buildmax-server` binary. Listen address from `--port` or `BUILDMAX_SERVER_PORT` (default 5678). Requires `BUILDMAX_JWT_SECRET`; optional `BUILDMAX_CORS_ORIGIN` (default `http://localhost:5173`). Optional `BUILDMAX_WORKER_TOKEN` for worker-to-server auth (`/api/worker/*`).
- **Login / OTP**: there is no OTP delivery channel yet, so `POST /api/login` is **disabled by default** and returns 503. The only supported verifier is the development fixed code in `server.yaml` `dev_login_otp` (env override `BUILDMAX_DEV_LOGIN_OTP`), which authenticates *any* registered email address and is for trusted local deployments only. Wiring lives in `internal/server/handlers/auth.go` (`Config.DevLoginOTP`, constant-time compare) and `internal/bootstrap/server.go` logs a warning whenever it is enabled.
- **Routes**: `GET /healthz`, `GET /openapi.json`, `GET /swagger/`; `POST /api/otp/request`, `POST /api/login`; `GET /api/teams`, `POST /api/teams`, `GET/POST/DELETE /api/teams/{team_id}/members`; `GET/POST/PATCH/DELETE /api/teams/{team_id}/agents`; `GET/POST/PATCH /api/teams/{team_id}/issues`, `GET /api/teams/{team_id}/issues/{issue_id}`, `GET /api/teams/{team_id}/issues/{issue_id}/flow`, `POST /api/teams/{team_id}/issues/{issue_id}/agent-runs`, `POST /api/teams/{team_id}/issues/{issue_id}/workflow-runs`; `GET/POST/PATCH /api/teams/{team_id}/workflows`, `GET /api/teams/{team_id}/workflows/{workflow_id}`, `GET/POST /api/teams/{team_id}/workflows/{workflow_id}/runs`, `GET /api/teams/{team_id}/workflow-runs/{workflow_run_id}`; `POST /api/teams/{team_id}/upload`, `GET /api/teams/{team_id}/files`, `GET /api/teams/{team_id}/files/{path...}`; `GET/POST/DELETE /api/webhook-keys`; `GET/POST /api/teams/{team_id}/conversations`, `GET/POST /api/teams/{team_id}/conversations/{conversation_id}/messages`, `GET/POST /api/teams/{team_id}/conversations/{conversation_id}/tasks`; `GET /api/teams/{team_id}/tasks/{task_id}`, `POST /api/teams/{team_id}/tasks/{task_id}/runs`, `GET /api/teams/{team_id}/tasks/{task_id}/conversation`, `GET /api/teams/{team_id}/tasks/{task_id}/stream`, `GET /api/teams/{team_id}/tasks/{task_id}/artifacts`; `GET /api/teams/{team_id}/task-runs/{task_run_id}/artifacts/items`, `GET /api/teams/{team_id}/task-runs/{task_run_id}/artifacts/content`; `GET /api/teams/{team_id}/ws`; `POST /api/webhook`; `GET /api/sessions/{session_id}`; **worker API** (task-run-id only, worker token): `GET /api/worker/task-runs/{task_run_id}`, `PATCH /api/worker/task-runs/{task_run_id}`. JWT auth for user API; team membership is the main ownership/authz boundary for working resources.
- **Storage**: Shared core models and repository contracts live in `internal/core/model`. MySQL/GORM schema rows plus repository implementation live in `internal/infra/db`. Blob storage implementations live in `internal/infra/objectstore` — PersistStorage (team uploads under **home**) and ArtifactStorage (artifact result files); backends: local FS or S3/MinIO; config via `BUILDMAX_PERSIST_STORAGE`, `BUILDMAX_ARTIFACT_STORAGE`, `BUILDMAX_MINIO_*`. Startup builders live in `internal/bootstrap/objectstore.go`. Historical storage design background lives in `docs/archive/003-store-workspacestorage-reorg.md` and `docs/archive/017-team-scoped-files-upload-alignment.md`. **Run/storage layout**: each team has a persistent `home/`; each task run uses `conversations/<conversationID>/tasks/<taskID>/<taskRunID>/` with `home/` (materialized team home), `artifacts/` (run output, e.g. result.md), and `global/` (BUILDMAX_HOME for that run). Blob keys use team-scoped `home` for uploads and run-scoped `conversations/.../global/...` for run state. The **worker** accesses storage directly for materialize and artifacts; no proxy through server.
- **Execution**: Scheduler lives in `internal/server/scheduler` and runs in the server process: it polls for PENDING task runs, claims them with the typed run lifecycle API, and spawns the **buildmax-worker** binary. The worker-side API client and HTTP updater live in `internal/infra/workerclient`. Run execution lives in `internal/agentapp/taskrun`: the worker materializes workspace `home` to run `home`, prepares run-directory `AGENTS.md`, runs the shared agent runtime in-process with BUILDMAX_HOME = run `global` and cwd = run dir, writes result to run `artifacts/result.md`, uploads run `global` to blob, and streams assistant reply deltas when enabled. The `buildmax-worker` binary is bootstrapped by `internal/bootstrap`. Requires `BUILDMAX_SERVER_URL`, `BUILDMAX_WORKER_TOKEN`, and storage env when running the worker.

**Shared GUI and frontends**
- **GUI package** (`gui/`): Shared React 19 package at repo root; exports theme plus shared presentational components such as `BaseModal`, `FormModal`, `Avatar`, `ChatComposer`, `ChatThread`, and `RecentList`. Consumed by portal and desktop via `"@buildmax/gui": "file:../gui"` (or `file:../../gui` from desktop/frontend). Build output in `gui/dist/`; `./make build` builds gui before desktop; `./make clean` removes `gui/node_modules` and `gui/dist`.
- **Portal** (`portal/`): React 19 + Vite + TypeScript; depends on `@buildmax/gui` for theme and shared components. Builds independently (`cd portal && npm install && npm run dev` / `npm run build`). Pages now include Login, SignUp, Home, Explore, Conversations, ConversationDetail, Issues, IssueDetail, Workflows, WorkflowDetail, WorkflowRunDetail, Agents, and Team Settings; API client, AuthContext, TeamContext; AppShell, Sidebar, TopBar, modals. Connects to Go backend for auth and the team-scoped conversation/issue/workflow/agent/task/file APIs.
- **Desktop app**: Wails app in `cmd/buildmax-desktop/` with frontend in `desktop/frontend/` (React 19 + Vite, JSX). Same `@buildmax/gui` dependency as portal. Desktop already supports local session listing/loading and local chat using the shared Go agent runtime; only the presentational layer is shared with portal.

### 4.1.1 Roadmap Outcome Snapshot

The archived `docs/archive/010-team-task-workflow-roadmap.md` program is effectively complete for the currently planned scope. The repo should now be understood with these rules:

- **Issue is the main user-facing work object.**
  - Issues are separate from low-level `task` / `task_run`.
  - Portal has a top-level Issues area with list, detail, assignment, and execution visibility.
- **Team is the ownership boundary.**
  - Personal usage is an implicit single-member team (`My Space`).
  - Issues, agents, workflows, conversations, tasks, and uploaded files belong to a team.
  - Most Portal APIs use explicit `/api/teams/{team_id}/...` routes.
- **Workflow is a team-scoped reusable execution plan.**
  - Workflow v1 is a lightweight linear-step model.
  - Issues can be assigned to a workflow.
  - Workflow runs are durable and inspectable.
- **Issue flow visibility is landed.**
  - Issue detail shows workflow runs, step state, agent task sequence, execution summary, and timeline-style progress.
- **Governance foundation is landed.**
  - Team roles are `owner`, `admin`, `member`.
  - Sensitive actions use centralized team authz checks.
  - Workflows have lifecycle states `draft`, `published`, `archived`.
  - Approvals, audit log, and team-scoped quota remain deferred.
- **Files/upload were aligned after the roadmap phases.**
  - Upload and file browsing are team-scoped.
  - Portal file APIs use team routes.
  - Worker runtime materializes `task.TeamID` files instead of `task.CreatedBy` files.

### 4.2 Tier 1 and Tier 2 architecture

- **Tier 1 = Conversation application service (orchestrator).** The orchestrator lives in `internal/service/conversation`. It is the single entry point for portal turns: it receives the normalized request, decides whether to run a direct conversation turn or create/continue a background task run, and owns what the user sees. Tier 1 is the single voice to the user.
- **Internal shape of Tier 1.** `internal/service/conversation` contains: root package for exported contracts, `ConversationService`, webhook turn policy, and runtime façade; `channel/` for normalized turn/channel adapter types and webhook adapter; `runtime/` for turn-loop mechanics (message replay, tool assembly, prompt assembly, streaming); and `tool/` for Tier 1 tools plus the task-service/store runner bridge.
- **Low-level Tier 1 loop = `internal/service/conversation/runtime` + `internal/core/agent`.** `internal/service/conversation/runtime` owns message persistence, tool selection, system-channel handoff, and optional streaming for one turn. `internal/core/agent` owns the shared tool-calling loop. System task-result turns do not expose task-creation tools, preventing task-run feedback loops.
- **Tier 2 = Task + TaskRun (execution in the back).** A Task with one or more TaskRuns is Tier 2: the worker materializes a run directory, executes the shared agent runtime there, produces artifacts, and can take a long time. Tier 2 is “tools in the back” - it does not send messages directly to the user; it always reports back to Tier 1 (run status, result, artifacts). Tier 1 turns that into what the user sees.
- **Tier 1 tools for Tier 2 (implemented via application services):** Tier 1 can (1) **create a Tier 2 task** through `internal/service/task` and the `StartTask` tool, which creates a Task and TaskRun for the worker; and (2) **rerun / follow-up on an existing task** by creating a new run for that task. In both cases Tier 2 reports back to Tier 1; Tier 1 orchestrates the reply to the user.

### 4.3 Planned / Not yet implemented

- Session list/delete from CLI; TUI session picker (session list type exists in `internal/core/session`).
- Config subcommand, Viper, or config-file binding (env-only today).
- Shell completion (e.g. `buildmax completion bash`).

## 5. Project Directory Structure

Following common Golang project conventions, the current structure is:

```
buildmax/
├── gui/                       # Shared GUI package (React 19): theme + shared presentational widgets; consumed by portal & desktop
├── cmd/
│   ├── buildmax/              # CLI/TUI binary
│   ├── buildmax-server/       # Server binary (HTTP API + scheduler)
│   ├── buildmax-worker/       # Worker binary (runs one task run via API + direct storage)
│   └── buildmax-desktop/      # Desktop app (Wails); embeds desktop/frontend
├── desktop/                   # Desktop frontend (React 19 + Vite); depends on gui
├── internal/                  # Private Go packages (this project only)
│   ├── agentapp/              # Agent runtime assembly: LLM client cache, tool registry, MCP, hooks, sandbox, skills, sessions, workspace
│   │   └── taskrun/           # Task-run execution in run-scoped directories (used by the worker)
│   ├── agenteval/             # Agent evaluation harness: task catalog and runner
│   ├── architecture/          # Architectural constraint tests
│   ├── bootstrap/             # Process startup and dependency wiring
│   │   ├── server.go          # Server bootstrap; used by cmd/buildmax-server
│   │   ├── worker.go          # Worker bootstrap; used by cmd/buildmax-worker
│   │   └── objectstore.go     # Startup builders for object storage implementations
│   ├── config/                # Env loading, paths, server/worker/storage config, env_spec.go
│   ├── core/                  # Pure domain layer: entities, contracts, algorithms (no application logic)
│   │   ├── model/             # Shared entities, repository contracts, LLM contracts, Tool contract
│   │   ├── agent/             # Core tool-calling loop and prompt/options (no infra imports)
│   │   ├── session/           # Local session model and persistence helpers
│   │   └── llm/               # LLM contract helpers
│   ├── service/               # Application services: coordinate stores, enforce rules, create runs
│   │   ├── conversation/      # Tier 1 conversation service (contracts, webhook policy, runtime façade)
│   │   │   ├── channel/       # Channel-facing turn types and adapters
│   │   │   ├── runtime/       # Tier 1 turn-loop runtime internals
│   │   │   └── tool/          # Tier 1 tools and task runner bridge
│   │   ├── issue/             # Issue application service
│   │   ├── task/              # Task and task_run application service
│   │   ├── workflow/          # Workflow and workflow-run orchestration service
│   │   └── quota/             # Team quota enforcement service
│   ├── tool/                  # Runtime agent tool implementations (read, write, bash, grep, task, skill, mcp_gateway, etc.)
│   ├── infra/                 # External-system implementations
│   │   ├── db/                # MySQL/GORM implementation of core model repositories
│   │   ├── objectstore/       # Local FS and S3/MinIO persist/artifact storage
│   │   ├── llm/               # OpenAI-compatible LLM implementation
│   │   ├── k8s/               # Kubernetes worker job launcher
│   │   ├── mcp/               # MCP protocol/client transport and registry
│   │   ├── hook/              # Hook transport drivers (command, http, mcp_tool, prompt)
│   │   ├── sandbox/           # Bash sandbox: Seatbelt/bwrap backends, egress proxy, violations
│   │   ├── trace/             # Durable run-trace recorder (JSONL, bounded + redacted)
│   │   ├── git/               # Git branch/diff helpers
│   │   ├── workerclient/      # Worker-side HTTP client for the server worker API
│   │   └── log/               # slog/lumberjack logging implementation
│   ├── interface/             # Local user-facing entry points
│   │   ├── auth/              # CLI/desktop login client and credential persistence
│   │   ├── cli/               # Cobra CLI, Bubble Tea TUI, prompt mode, version command
│   │   ├── client/            # HTTP client for BuildMax server API
│   │   └── desktop/           # Wails app bridge
│   ├── server/                # HTTP API, auth handlers, portal API, webhook, worker API
│   │   ├── handlers/          # Route handlers
│   │   ├── httputil/          # Shared request/response helpers
│   │   ├── scheduler/         # Polls pending task runs and spawns workers (runs in-process)
│   │   ├── websocket/         # Team websocket hub and stream fan-out
│   │   └── static/            # Embedded static assets (OpenAPI, Swagger)
│   ├── mock/                  # Test-only mocks and helpers
│   └── util/                  # ID generation, workspace helpers, git, argparse
├── portal/                    # Web UI (React 19 + Vite + TypeScript); depends on gui
├── docs/                      # Documentation
│   ├── architecture/          # How the system works today (per subsystem)
│   ├── design/                # Current design docs (numbered, cited from code)
│   └── archive/               # Superseded design docs, kept for history
├── config-examples/           # Config file examples (.env.example, settings/server/hooks yaml)
├── setup/                     # Setup scripts
├── deployment/                # Application manifests
├── go.mod
├── go.sum
├── ROADMAP.md                 # Active near-term product roadmap
└── README.md
```

- **cmd/buildmax**: CLI entry point; `main.go` only. Build with `go build -o buildmax ./cmd/buildmax` or `make.bat build`. Provides TUI, `-p` print mode, and `version`; delegates to `internal/interface/cli`.
- **cmd/buildmax-server**: Server entry point; `main.go` only. Build with `go build -o buildmax-server ./cmd/buildmax-server`. Delegates to `internal/bootstrap`, which wires DB, storage, LLM, HTTP server, and scheduler.
- **cmd/buildmax-worker**: Worker entry point; `main.go` only. Accepts `--task-run-id`; delegates to `internal/bootstrap` startup logic (get task run via API, blob storage, `agentapp/taskrun`). Requires `BUILDMAX_SERVER_URL`, `BUILDMAX_WORKER_TOKEN`, `BUILDMAX_WORKSPACES_DIR`, and storage env when running the worker.
- **internal/agentapp**: Agent runtime assembly used by CLI, worker, and desktop. Owns `AgentApp` — LLM client cache, tool registry construction, MCP manager, hook manager, sandbox manager, run-trace wiring, skill/subagent discovery, session persistence, and workspace resolution. `agentapp/taskrun` runs one task run in its run-scoped directory.
- **internal/architecture**: Architectural constraint tests (import boundary enforcement).
- **internal/bootstrap**: Process startup and dependency wiring for server, worker, and startup storage builders.
- **internal/interface**: Local user-facing entry points: CLI + TUI (`cli/`), HTTP client for server API (`client/`), desktop Wails bridge (`desktop/`), and local auth credential/client code (`auth/`).
- **internal/core**: Pure domain layer. `core/model` owns shared entities, repository contracts, LLM contracts, and the Tool contract. `core/agent` is the pure tool-calling loop with no infra imports. No application services live here.
- **internal/service**: Application services that coordinate stores, enforce business rules, and manage run lifecycles. `service/conversation` is Tier 1 (orchestrator); `service/task`, `service/issue`, `service/workflow`, and `service/quota` are the remaining application services.
- **internal/tool**: Runtime agent tool implementations — file I/O, bash, grep, glob, web fetch, MCP gateway, skill, subagent runner, task bridge. Imports `internal/infra/mcp` and other infra as needed; not a pure domain package.
- **internal/infra**: External-system implementations such as DB, object storage, LLM, Kubernetes, MCP transport, hook transports, the bash sandbox, the run-trace recorder, git helpers, the worker API client, and logging.
- **internal/server**: HTTP API for the Portal and worker callbacks, plus the in-process scheduler (`server/scheduler`) that polls pending task runs and spawns workers. It depends on core services and the worker client contracts; it is bootstrapped by `internal/bootstrap`.
- **gui/**: Shared React package; build with `cd gui && npm install && npm run build` (output in `gui/dist/`). Portal and desktop depend on it via npm `file:`; `./make build` builds gui first when building desktop.
- **portal/**: Frontend app; depends on `@buildmax/gui`. Run with `cd portal && npm install && npm run dev`; build with `npm run build` (output in `portal/dist/`).
- **desktop/frontend/**: Desktop UI; depends on `@buildmax/gui`. Same React 19 and shared components as portal; app logic (Wails bindings, session) is desktop-specific.
- **docs/**: All project documentation — `docs/architecture/` describes how the system works today, `docs/design/` holds current numbered design docs, `docs/archive/` keeps superseded ones. See section 6 and [docs/README.md](docs/README.md).

## 6. Documentation and Repository

- **Active roadmap**: [ROADMAP.md](ROADMAP.md) — current near-term product
  roadmap for the shared Agent Core, local surfaces, Portal outcome surface,
  enterprise deployment loop, governance foundation, and versioned workspace
  design.
- **Documentation index**: [docs/README.md](docs/README.md) explains the three documentation types and when to update each.
- **Architecture reference** (in `docs/architecture/`): how the system works today, one document per package or subsystem. Start with [docs/architecture/overview.md](docs/architecture/overview.md) and [docs/architecture/packages.md](docs/architecture/packages.md). Update these in the same change that moves a package boundary or alters a runtime contract.
- **Design docs** (in `docs/design/`): [docs/design/README.md](docs/design/README.md) is the design index; [docs/design/001-about-portal.md](docs/design/001-about-portal.md) is the Portal/Nexus product vision; [docs/design/002-env-config-maintainability.md](docs/design/002-env-config-maintainability.md) covers env-based config; [docs/design/023-desktop-cli-portal-positioning.md](docs/design/023-desktop-cli-portal-positioning.md) records the Agent Core / CLI / Desktop / Portal positioning. Historical and superseded design docs live under `docs/archive/`.
- **Local task notes**: the `/vibe` workflow keeps working notes in `.vibe/` at the repository root. That directory is gitignored — it is a local scratch area, not published documentation. Anything worth keeping belongs in `docs/`.
- **Env reference**: `config-examples/.env.example` and `internal/config/env_spec.go` (single source of truth for env keys).
- Code and scripts: repository root, managed with Go modules.

### 6.1 Persistence naming style

- **Use the same naming style for all persisted data** (e.g. session files, config, any JSON on disk).
- **Convention: snake_case** for JSON object keys (e.g. `created_at`, `tool_call_id`, `tool_calls`).
- Ensure structs that are serialized to disk have explicit `json:"snake_case"` tags so the on-disk format is consistent; do not rely on Go’s default (PascalCase) for persisted fields.

### 6.2 Database table naming

- **Use singular table names.** One table per entity type, named in the singular (e.g. `user`, `agent`, `conversation`, `task`, `task_run`). Do not use plural names (e.g. `users`, `tasks`). This applies to all database tables created or migrated by the project.

### 6.3 Entity ID format

- **Entity IDs use a prefixed format** `<prefix>_<body>`: prefix is a short type abbreviation (e.g. `u_` user, `a_` agent, `c_` conversation, `t_` task, `r_` task run, `ar_` artifact, `f_` artifact item, `cm_` conversation message, `whk_` webhook key), body is 20 characters from `[a-z0-9]` (lowercase base36). Generated via `internal/util.NewPrefixedID(prefix)`; ordering uses `created_at`, not ID. The generator and its tests in `internal/util/id.go` are the reference for the format.

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
- **Bump version**: `./make bump [patch|minor|major]` — updates `Version` in `internal/interface/cli/root.go` (default: patch).
- **Setup / Unsetup**: `./make setup` runs `setup/setup.sh` (one-click local dev: kind cluster, MinIO, MySQL, port-forwards, test job; idempotent). `./make unsetup` runs `setup/unsetup.sh` to tear down. Requires Homebrew (kind, helm, kubectl, awscli). Do not use `./make run server` or `./make smoke` in automated CI; they are for local manual use.

**Windows:** `make.bat` in the repo root provides `build` and `test` for Windows; prefer PowerShell over batch when running commands. Build output: `buildmax.exe`.

## 8. Commit Message
Do NOT include: Co-Authored-By and Claude-Session in commit message

---

*This document is updated as the project evolves.*
