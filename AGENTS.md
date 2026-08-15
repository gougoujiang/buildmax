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
- **Ownership model**: Team → Conversation → Task → TaskRun. Personal usage is represented by a default personal team (`My Space`). Issues, agents, workflows, conversations, tasks, and uploaded files are team-scoped. Webhook keys remain user-scoped for now. No project entity in the server/Portal model (the desktop app has its own local-only `Project` concept for recent folders, which is not a domain entity). Git is the hidden version engine; user sees timeline + restore, not commits/branches.
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
- **CLI**: Cobra in `internal/interface/cli` — root command (TUI, or `-p` print mode) plus `init`, `version`, `login`/`logout`/`whoami`, `sandbox status|deps|mode|enable|disable`, and cobra's built-in `completion`; `cmd/buildmax/main.go` is the thin entry point. Exit codes are a stable contract (`exit_code.go`). The server and worker run as separate binaries.
- **TUI**: Bubble Tea via `internal/interface/cli` (`tui.go`); default when running `buildmax` with no flags. Layout: scrollable area (banner, chat history, streaming text, in-flight tool activity), input at bottom, two-line footer (model, workspace + git branch, sandbox tag, login; then run status and key hints). Slash panels: `/model`, `/sessions`, `/tools`, `/skills`, `/mcp`, `/diff`. Tool approval prompts run through `TUIApprovalHandler`. `--resume <id>`, `--continue`, `--session-id <uuid>` for session handling. Session persisted after each assistant reply.
- **Local auth**: CLI/desktop-side login client and credential persistence live in `internal/interface/auth`; server-side auth handlers live in `internal/server/handlers/auth.go`.
- **Session**: In-memory session in `internal/core/session` (id, title, created_at, message history); multi-turn; save/load under `DataDir()/sessions/<id>.json`; list index in `sessions.json`. Persistence and lifecycle (create, load, save, list, rename, delete, pin, title generation) live in `internal/agentapp` (`SessionManager`, `LoadSessionList`), not in `core/session`.

**Agent & tools**
- **LLM integration**: OpenAI-compatible implementation in `internal/infra/llm` (OpenRouter default); shared LLM contracts such as `Message`, `ToolCall`, `Usage`, and `LLMClient` live in `internal/core/llm` (not `core/model`, which holds domain entities and repository contracts); model config is loaded by `internal/config` from `<BUILDMAX_HOME>/settings.yaml` (`models:` list, first entry is the default).
- **Agent loop**: Shared tool-calling loop (LLM -> tool_calls -> execute tools -> re-call LLM -> reply) in `internal/core/agent`; default system prompt prepended by the core agent run.
- **Agent runtime assembly**: Runtime wiring for CLI, worker, and desktop lives in `internal/agentapp`; process bootstrapping lives in `internal/bootstrap/*`. `AgentApp` owns the LLM client cache, tool registry construction, MCP manager, skill/subagent discovery, session persistence, and workspace resolution.
- **Optional workspace AGENTS.md**: When running the CLI, if a file named `AGENTS.md` exists at the workspace root (current working directory), its contents are appended to the default system prompt so the agent receives project-specific instructions. See the [agents.md](https://agents.md/) convention. For remote runs, the worker prepares an `AGENTS.md` in the run directory (run directory layout plus optional workspace `AGENTS.md` from materialized home) so the same convention applies when the shared agent runtime runs with cwd = run dir.
- **Runtime tools** (`internal/tool`): LLM-facing names come from `names.go` — `Read`, `Write`, `Edit`, `Glob`, `Grep`, `Bash`, `WebFetch`, `TodoWrite`, `Skill`, `Task` — plus `LoadMcpTools`/`CallMcpTool` from `mcp_gateway.go`. Hook matchers and subagent `tools:` fields match these exact strings. Paths resolve under a configurable root (e.g. CWD). MCP gateway tools (`mcp_gateway.go`) also live here — no separate mcptool package. Tool output is LLM-oriented (meaningful on success and failure). Conversation-specific tools such as `StartTask`, `ContinueTask`, `ListTasks`, and `GetTask` live in the Tier 1 conversation subsystem under `internal/service/conversation/tool`.
- **MCP tools**: MCP protocol/client transport lives in `internal/infra/mcp`; agent-facing MCP gateway tools live in `internal/tool` (`mcp_gateway.go`).
- **Runtime hooks**: lifecycle hooks (P0.5 trust harness). `HookManager` (`internal/agentapp/hook_manager.go`) merges global `hooks:` from `settings.yaml` with workspace `<workspace>/.buildmax/hooks.yaml` and dispatches to per-transport drivers in `internal/infra/hook` (`command`, `http`, `mcp_tool`, `prompt`). 13 events shipped — `SessionStart/End`, `UserPromptSubmit` (gating), `PreToolUse` (gating), `PostToolUse`/`PostToolUseFailure`, `Notification`, `PreCompact` (gating)/`PostCompact`, `SubagentStart/Stop`, `Stop`/`StopFailure`. Those are the Go constant names; the YAML keys are snake_case (`session_start`, `pre_tool_use`, …), and a `matcher` is a regex against the tool name. Block contract: command exit 2, HTTP 422, or response JSON `{"decision":"block","reason":"..."}`. Failures fail open. Subagents inherit the parent HookManager and stamp every event with `is_subagent`/`agent_type`. Design: `docs/design/031-hook-system-v2.md`. Examples: `config-examples/settings.example.yaml` and `config-examples/hooks.workspace.example.yaml`.
- **Bash sandbox** (P0.5 trust harness §3.2): bash subprocesses can run under an OS sandbox — Seatbelt on macOS, `bwrap` on Linux/WSL2, unavailable elsewhere. Non-bash tools keep their existing permission boundary (`util.ResolvePath`, `tool/safety.go`, `agentapp/policy.go`). Contract in `internal/core/agent/sandbox.go` (`SandboxView`), backends and proxy in `internal/infra/sandbox`, assembly in `internal/agentapp/sandbox.go`. Config is a `sandbox:` block in `settings.yaml`, overridable by `<BUILDMAX_HOME>/policy.yaml` and `BUILDMAX_SANDBOX_ENABLED`; boundaries cover filesystem allow/deny paths, domain allow/deny via a Go-side HTTP/SOCKS proxy, and scrubbing of secret-shaped env vars. Modes: `auto_allow` (sandboxed bash skips the prompt) and `regular`. Per-call escape hatch `dangerously_disable_sandbox`, honored only when `allow_unsandboxed_commands` is true. Inspect with `buildmax sandbox status|deps|mode|enable|disable`; the TUI footer shows the active mode. **Default off on all surfaces today** — the stricter worker default, process rlimits, and hook-transport enforcement are not wired yet (see `docs/design/032-sandbox-and-execution-boundaries.md` §13.1). Design: `docs/design/032-sandbox-and-execution-boundaries.md`.
- **Durable run traces** (P0.5 trust harness §3.3): every run persists the agent event stream as a bounded, redacted JSONL trace. The recorder lives in `internal/infra/trace` and is attached at the single `agentapp.RunPrompt` chokepoint (tees the `EventSink`), so CLI/TUI, Desktop, eval, and worker runs all produce traces. Layout: `<DataDir>/traces/<session_id>/<run_id>.jsonl` (run id prefix `rt_`) with a `run_start` record, per-iteration `llm_*`/`tool_*`/`context_compacted` records, and a terminal `run_end`; `RunResult.TraceID` points at the file. Free-text fields are truncated and common secret shapes redacted (`internal/infra/trace/redact.go`). On by default; disable with `BUILDMAX_TRACE_DISABLED`. Fail-open: a trace failure never breaks or slows a run. Design: `docs/design/034-durable-run-trace.md`.

**Config & infra**
- **Application data**: `config.DataDir()` — default `~/.buildmax`, override via `BUILDMAX_HOME`; `make test` uses `testing-sandbox`.
- **Config**: YAML files plus a handful of bootstrap env vars. `internal/config` loads `<BUILDMAX_HOME>/settings.yaml` (`LoadSettings()`, CLI/desktop: models, hooks, sandbox, log level) and `<BUILDMAX_HOME>/server.yaml` (`LoadServerConfig()`, server/worker: port, jwt, database, webhook, worker, storage), and resolves paths — `DataDir()`, `SessionsDir()`, `LogsDir()`, `TracesDir()`, `SettingsPath()`, `ServerConfigPath()`, `PolicyPath()`, `AuthPath()`, plus run-scoped helpers that take `workspacesDir` explicitly (`PersistentWorkspaceDir`, `RuntimeTaskRunDir`, `RuntimeTaskRunHomeDir`, `RuntimeTaskRunArtifactsDir`, `RuntimeTaskRunGlobalDir`). Startup wiring lives under `internal/bootstrap/*`; config does not import infra implementations. `env_spec.go` is the single source of truth for the few env vars that remain (`BUILDMAX_HOME`, `BUILDMAX_JWT_SECRET`, `BUILDMAX_DEV_LOGIN_OTP`, `BUILDMAX_SANDBOX_ENABLED`, `BUILDMAX_TRACE_DISABLED`, `BUILDMAX_TEST_DSN`). There is no `.env.example`. See `config-examples/*.example.yaml` and `docs/reference/configuration.md`.
- **Logging**: `log/slog` via `internal/infra/log`; level from `log_level` in settings.yaml/server.yaml; file-only (rotated under `DataDir()/logs`, Lumberjack); TUI/prompt output stays clean.

**HTTP server & Portal backend**
- **Server** (`buildmax-server` binary): HTTP API in `internal/server`, bootstrapped by `internal/bootstrap`; started via `./make run server` or by running the `buildmax-server` binary. Listen address from `--port` or `port` in server.yaml (default 5678). Requires `jwt_secret` (or `BUILDMAX_JWT_SECRET`); `cors_origin` defaults to `http://localhost:5173`. `worker.token` authenticates worker-to-server calls (`/api/worker/*`).
- **Login / OTP**: there is no mail channel, so `POST /api/login` verifies **single-use login codes** an operator issues out of band with `buildmax-server user login-code <email>` (`internal/bootstrap/user_admin.go`). Codes are per-account, expiring, SHA-256 hashed at rest, and redeemed by a conditional UPDATE so concurrent attempts produce one winner (`internal/infra/db/login_code.go`, contract in `internal/core/model/login_code.go`). Accounts come from `buildmax-server user create`; **`allow_signup` defaults to false**, so `POST /api/otp/request` refuses `intent: signup` with 403. The legacy `dev_login_otp` fixed code still works when set — it authenticates *any* registered email and `internal/bootstrap/server.go` warns at startup. Verification order lives in `internal/server/handlers/auth.go` (`verifyLoginCode`).
- **Routes**: `GET /healthz`, `GET /openapi.json`, `GET /swagger/`; `POST /api/otp/request`, `POST /api/login`; `GET /api/teams`, `POST /api/teams`, `GET/POST/DELETE /api/teams/{team_id}/members`; `GET/POST/PATCH/DELETE /api/teams/{team_id}/agents`; `GET/POST/PATCH /api/teams/{team_id}/issues`, `GET /api/teams/{team_id}/issues/{issue_id}`, `GET /api/teams/{team_id}/issues/{issue_id}/flow`, `POST /api/teams/{team_id}/issues/{issue_id}/agent-runs`, `POST /api/teams/{team_id}/issues/{issue_id}/workflow-runs`; `GET/POST/PATCH /api/teams/{team_id}/workflows`, `GET /api/teams/{team_id}/workflows/{workflow_id}`, `GET/POST /api/teams/{team_id}/workflows/{workflow_id}/runs`, `GET /api/teams/{team_id}/workflow-runs/{workflow_run_id}`; `POST /api/teams/{team_id}/upload`, `GET /api/teams/{team_id}/files`, `GET /api/teams/{team_id}/files/{path...}`; `GET/POST/DELETE /api/webhook-keys`; `GET/POST /api/teams/{team_id}/conversations`, `GET/POST /api/teams/{team_id}/conversations/{conversation_id}/messages`, `GET/POST /api/teams/{team_id}/conversations/{conversation_id}/tasks`; `GET /api/teams/{team_id}/tasks/{task_id}`, `POST /api/teams/{team_id}/tasks/{task_id}/runs`, `GET /api/teams/{team_id}/tasks/{task_id}/conversation`, `GET /api/teams/{team_id}/tasks/{task_id}/stream`, `GET /api/teams/{team_id}/tasks/{task_id}/artifacts`; `GET /api/teams/{team_id}/task-runs/{task_run_id}/artifacts/items`, `GET /api/teams/{team_id}/task-runs/{task_run_id}/artifacts/content`; `GET /api/teams/{team_id}/ws`; `GET /api/usage`, `GET /api/teams/{team_id}/usage`; `POST /api/webhook`; **worker API** (worker token, task-run-id only): `GET`/`PATCH /api/worker/task-runs/{task_run_id}`, `POST /api/worker/task-runs/{task_run_id}/stream`. `internal/server/handlers/routes.go` is the source of truth; the server also serves its own `GET /openapi.json`. JWT auth for user API; team membership is the main ownership/authz boundary for working resources.
- **Storage**: Shared core models and repository contracts live in `internal/core/model`. MySQL/GORM schema rows plus repository implementation live in `internal/infra/db`. Blob storage implementations live in `internal/infra/objectstore` — PersistStorage (team uploads under **home**) and ArtifactStorage (artifact result files); backends: local FS or S3/MinIO; config via the `storage:` block in server.yaml. Startup builders live in `internal/bootstrap/objectstore.go`. **Run/storage layout**: each team has a persistent `home/`; each task run uses `conversations/<conversationID>/tasks/<taskID>/<taskRunID>/` with `home/` (materialized team home), `artifacts/` (run output, e.g. result.md), and `global/` (BUILDMAX_HOME for that run). Blob keys use team-scoped `home` for uploads and run-scoped `conversations/.../global/...` for run state. The **worker** accesses storage directly for materialize and artifacts; no proxy through server.
- **Execution**: Scheduler lives in `internal/server/scheduler` and runs in the server process: it polls for PENDING task runs, claims them with the typed run lifecycle API, and spawns the **buildmax-worker** binary. The worker-side API client and HTTP updater live in `internal/infra/workerclient`. Run execution lives in `internal/agentapp/taskrun`: the worker materializes workspace `home` to run `home`, prepares run-directory `AGENTS.md`, runs the shared agent runtime in-process with BUILDMAX_HOME = run `global` and cwd = run dir, writes result to run `artifacts/result.md`, uploads run `global` to blob, and streams assistant reply deltas when enabled. The `buildmax-worker` binary is bootstrapped by `internal/bootstrap` and reads the same `server.yaml` — it needs `worker.server_url`, `worker.token`, `workspaces_dir`, and the `storage:` block. With `worker.run_mode: k8s_job`, `internal/infra/k8s` mounts that file into each worker pod from the ConfigMap named by `worker.k8s.config_map` and sets `BUILDMAX_HOME` to `worker.k8s.home_dir`; credentials reach the pod through the inherited `BUILDMAX_*` environment. Secret-bearing `server.yaml` fields have env overrides (`BUILDMAX_DATABASE_PASSWORD`, `BUILDMAX_STORAGE_MINIO_ACCESS_KEY`/`_SECRET_KEY`, `BUILDMAX_WORKER_TOKEN`, `BUILDMAX_CONVERSATION_MODEL_API_KEY`) so deployments inject them from a Secret.

**Shared GUI and frontends**
- **GUI package** (`gui/`): Shared React 19 package at repo root; exports theme plus shared presentational components such as `BaseModal`, `FormModal`, `Avatar`, `ChatComposer`, `ChatThread`, and `RecentList`. Consumed by portal and desktop via `"@buildmax/gui": "file:../gui"` (or `file:../../gui` from desktop/frontend). Build output in `gui/dist/`; `./make build` builds gui before desktop; `./make clean` removes `gui/node_modules` and `gui/dist`.
- **Portal** (`portal/`): React 19 + Vite + TypeScript; depends on `@buildmax/gui` for theme and shared components. Builds independently (`cd portal && npm install && npm run dev` / `npm run build`). Pages now include Login, SignUp, Home, Explore, Conversations, ConversationDetail, Issues, IssueDetail, Workflows, WorkflowDetail, WorkflowRunDetail, Agents, and Team Settings; API client, AuthContext, TeamContext; AppShell, Sidebar, TopBar, modals. Connects to Go backend for auth and the team-scoped conversation/issue/workflow/agent/task/file APIs.
- **Desktop app**: Wails app in `cmd/buildmax-desktop/` with frontend in `desktop/frontend/` (React 19 + Vite, JSX). Same `@buildmax/gui` dependency as portal. Desktop already supports local session listing/loading and local chat using the shared Go agent runtime; only the presentational layer is shared with portal.

### 4.1.1 Roadmap Outcome Snapshot

The Team / Issue / Workflow program is complete for the currently planned scope. The repo should now be understood with these rules:

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
  - Team-scoped quota is landed: `internal/service/quota` checks run and token budgets at task creation, with tiers in the `quota_tier` table and `default_quota_tier` in server.yaml.
  - Approvals and audit log remain deferred.
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

- Approvals and audit log for team governance (roles and workflow lifecycle are landed).
- Versioned workspace / timeline restore (P5; see `docs/design/029-versioned-workspace-design.md`).
- Worker sandbox hardening: stricter worker default, process rlimits, hook-transport
  enforcement (`docs/design/032-sandbox-and-execution-boundaries.md` §13.1).
- End-to-end CI for the Kubernetes deployment path and for native Windows.

## 5. Project Directory Structure

**The repository tree has one source of truth:
[docs/contribute/repo-layout.md](docs/contribute/repo-layout.md).** It carries the
top level, every `internal/` package, the nested Go modules, the binaries, and
the dependency direction. Read it there and update it there — do not restate the
tree in this file or anywhere else.

Orientation, so the rest of this document reads without a detour:

| Path | What lives there |
|---|---|
| `cmd/` | Binary entry points, `main.go` only |
| `internal/core/` | Pure domain: entities, contracts, the tool-calling loop |
| `internal/agentapp/` | Agent runtime assembly shared by CLI, desktop, and worker |
| `internal/service/` | Application services; `service/conversation` is Tier 1 |
| `internal/tool/` | Runtime agent tools (Read, Write, Bash, …) |
| `internal/infra/` | External systems: DB, object store, LLM, MCP, hooks, sandbox |
| `internal/interface/` | CLI/TUI, desktop bridge, API client |
| `internal/server/` | HTTP API, scheduler, websocket hub |
| `gui/`, `portal/`, `desktop/` | Shared React package and the two frontends |
| `docs/`, `config-examples/`, `deployment/` | Documentation, config examples, deployment |
| `eval/`, `sample-data/`, `scripts/` | Benchmark fixtures, workspace seed datasets, repo tooling |
| `.buildmax/` | This repository's own workspace agent config — see `.buildmax/README.md` |

Dependency direction, enforced by tests in `internal/architecture`:

```text
bootstrap ──▶ interface / server / service / agentapp / infra ──▶ core
```

`internal/core` imports nothing from `config`, `infra`, `service`, `server`,
`agentapp`, or `interface`.

## 6. Documentation and Repository

- **Active roadmap**: [ROADMAP.md](ROADMAP.md) — current near-term product
  roadmap for the shared Agent Core, local surfaces, Portal outcome surface,
  enterprise deployment loop, governance foundation, and versioned workspace
  design.
- **Documentation index**: [docs/README.md](docs/README.md) routes by reader — `start/`, `guide/`, `deploy/`, `reference/`, `contribute/`, `design/`.
- **Architecture reference** (in `docs/contribute/architecture/`): how the system works today, one document per package or subsystem. Start with [docs/contribute/architecture/overview.md](docs/contribute/architecture/overview.md). The repository tree has a single source of truth in [docs/contribute/repo-layout.md](docs/contribute/repo-layout.md) — update that file and no other when a package moves.
- **Design records** (in `docs/design/`): [docs/design/README.md](docs/design/README.md) splits them into durable specifications (031 hooks, 032 sandbox, 034 traces) and expiring roadmap plans (024–030). [docs/design/001-about-portal.md](docs/design/001-about-portal.md) is the Portal product vision; [docs/design/023-desktop-cli-portal-positioning.md](docs/design/023-desktop-cli-portal-positioning.md) records surface positioning. A design record is rationale, not user documentation — the user-facing half belongs in `docs/guide/` or `docs/reference/`. Conventions: [docs/contribute/documentation.md](docs/contribute/documentation.md).
- **Contributor documentation**: [CONTRIBUTING.md](CONTRIBUTING.md) is the process; [docs/contribute/conventions.md](docs/contribute/conventions.md) is the naming, ID, tool-output, and commit rules; [docs/contribute/first-pr.md](docs/contribute/first-pr.md) is the clone-to-pull-request path.
- **This repository's own agent config**: `.buildmax/` holds the workspace skills, subagents, and MCP servers the CLI loads when the workspace is this repository — see [.buildmax/README.md](.buildmax/README.md).
- **Local task notes**: the `/vibe` workflow keeps working notes in `.vibe/` at the repository root. That directory is gitignored — it is a local scratch area, not published documentation. Anything worth keeping belongs in `docs/`.
- **Config reference**: [docs/reference/configuration.md](docs/reference/configuration.md), backed by `config-examples/*.example.yaml` and `internal/config/env_spec.go`.
- Code and scripts: repository root, managed with Go modules.

### 6.1 Code conventions

**Single source of truth:
[docs/contribute/conventions.md](docs/contribute/conventions.md).** It carries
the rules that used to live here, in full and with examples:

- persisted JSON uses `snake_case`, with explicit `json:` tags
- database tables are singular (`task`, not `tasks`)
- entity IDs are `<prefix>_<20 chars of base36>`, generated by `NewPrefixedID`
  in `internal/util`
- tool output is written for the LLM, and is meaningful on both success and failure
- commit subjects are a single imperative line; no tooling trailers or
  "Generated with ..." footers in commits or pull request descriptions
- user-visible changes get a `CHANGELOG.md` entry under `## [Unreleased]`

Follow that document; do not restate its rules here.

## 7. Build & Test

`./make <command>` at the repo root — on Windows, `make.bat <command>`. Both are
one-line shims that forward to the task runner in **`cmd/mk`** (`go run
./cmd/mk`), so every platform runs the same task code. `cmd/mk` depends only on
the standard library, loads `.env` from the repo root itself, and appends `.exe`
to binary names on Windows. Add or change commands in `cmd/mk`, never in the
shims.

```bash
./make build          # CLI, server, worker, gui, desktop → bin/
./make test           # go test ./... with BUILDMAX_HOME=./testing-sandbox
./make lint           # golangci-lint and govulncheck, CI's pinned versions
./make help           # every command
```

Run `./make test` after code changes. Build, test, lint, and the deployment
smokes need no model API key.

The full command list, what each deployment smoke covers, the `desktop` build
tag, and the Windows caveats are in
[CONTRIBUTING.md](CONTRIBUTING.md); `./make help` prints the commands.

## 8. Commit Messages And Pull Requests

See [docs/contribute/conventions.md](docs/contribute/conventions.md#commit-messages-and-pull-requests).
In short: single imperative commit subject, no `Co-Authored-By` or
`Claude-Session` trailers, no "Generated with ..." footer or assistant session
link in a pull request description, and a `CHANGELOG.md` entry under
`## [Unreleased]` for user-visible changes.

---

*This document is updated as the project evolves.*
