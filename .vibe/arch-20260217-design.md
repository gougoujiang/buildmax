# Design — Architecture Refactor (from arch-20260217)

## Goal

Implement the five architecture refactors from `.vibe/arch-20260217.md` in order: (1) unify executor store dependency, (2) inject config/path functions for testability, (3) extract shared domain types to `internal/model`, (4) invert tools–agent coupling via SubAgentRunner, (5) optionally split server package by domain.

---

## Refactor 1: Unify executor store dependency

### Goal

Executor depends only on `store` interfaces and `store.Task`; remove the duplicate `executor.TaskStore` interface.

### Modules

| Module | Responsibility | Owns |
|--------|----------------|------|
| **internal/executor** | Poll for pending tasks; run buildmax CLI; update task status and create artifacts. | Runner, executeTask, copy helpers. |
| **internal/store** | Persistence contracts and implementation. | TaskStore, ArtifactStore, Task, Store. |

### Structure

- `internal/executor/executor.go` — Runner takes two store interfaces; no local TaskStore type.
- `internal/store/` — No new files; interfaces and types unchanged (Task already in store).

### Method / type design

| Location | Change | Signature / type | Responsibility |
|----------|--------|------------------|----------------|
| **executor** | Remove type | (delete `executor.TaskStore` interface) | — |
| **executor.Runner** | New field shape | `taskStore store.TaskStore` and `artifactStore store.ArtifactStore` (or single interface, see below) | Runner holds both stores. |
| **executor** | New | `New(taskStore store.TaskStore, artifactStore store.ArtifactStore) *Runner` | Construct Runner with both interfaces. |
| **executor** | Unchanged | `executeTask(ctx, task store.Task)` | Still uses `store.Task`; calls `taskStore.*` and `artifactStore.CreateArtifactWithItem`. |

**Option**: Instead of two parameters, `store` can expose a composed interface used only by executor, e.g. `TaskRunnerStore` embedding `TaskStore` and adding `CreateArtifactWithItem`. Then `New(store.TaskRunnerStore)` single param. Prefer two params to avoid extra store types unless the team prefers a single dependency.

### How they work together

1. cmd/server passes `st` (concrete `*store.Store`) as both `TaskStore` and `ArtifactStore` to `executor.New(st, st)`.
2. Executor loop: `taskStore.GetNextPendingTask` → `taskStore.UpdateTaskStatus` (RUNNING/SUCCEEDED/FAILED) and `artifactStore.CreateArtifactWithItem` when creating artifact.
3. No executor code references a custom interface; all methods come from `store`.

### Changes for review (Refactor 1)

- **Modified**: `internal/executor/executor.go` — Remove `TaskStore` interface; add `New(taskStore store.TaskStore, artifactStore store.ArtifactStore) *Runner`; Runner struct holds both; `executeTask` and helpers call the appropriate store.
- **Modified**: `internal/cmd/server.go` — Call `executor.New(st, st)` instead of `executor.New(st)`.

---

## Refactor 2: Inject config / path functions for testability

### Goal

Server and executor receive path (and optionally other) configuration via structs/functions so tests can override without global `config.*` calls.

### Modules

| Module | Responsibility | Owns |
|--------|----------------|------|
| **internal/config** | Default values from env/files; no new interfaces. | LoadLLM, DataDir, PersistentWorkspaceDir, etc. |
| **internal/server** | HTTP API; uses only injected config. | Config struct extended with path fields. |
| **internal/executor** | Task runner; uses only injected paths. | Runner accepts a paths/config struct. |

### Structure

- **Server**: Extend `server.Config` with fields used by handlers that currently call `config.*`:
  - `WorkspacesDir string` — used for workspace root (upload, files, artifacts). Handlers already use `config.PersistentWorkspaceDir(workspaceID)` etc.; they will use a helper that takes `WorkspacesDir` and `workspaceID`, or we add to Config: `PersistentWorkspaceDir(workspaceID)`, `RuntimeWorkspaceDir(workspaceID, taskID)`, `ArtifactDir(workspaceID, taskID, artifactID)` as functions or as a single struct holding `WorkspacesDir` and helpers that take IDs.
- **Executor**: Runner receives a struct for paths (e.g. `WorkspacePaths`) with at least:
  - `PersistentWorkspaceDir(workspaceID string) string`
  - `RuntimeWorkspaceDir(workspaceID, taskID string) string`
  - `ArtifactDir(workspaceID, taskID, artifactID string) string`
  Implemented by a type in config (or a simple struct with closures) so cmd can pass `config.DefaultWorkspacePaths()` and tests can pass a stub.

### Method / type design

| Location | Type / method | Signature | Responsibility |
|----------|---------------|-----------|----------------|
| **config** | (optional) | `type WorkspacePaths struct { BaseDir string }` + methods | `PersistentWorkspaceDir(id)`, `RuntimeWorkspaceDir(ws, task)`, `ArtifactDir(ws, task, art)` using `BaseDir` (default `WorkspacesDir()`). |
| **server.Config** | New fields | `WorkspacesDir string` | If set, handlers build paths from it; else keep calling `config.WorkspacesDir()` for backward compat, or require it (then cmd always sets from config). |
| **executor** | New param | `Runner struct { ..., paths WorkspacePaths }` | `New(taskStore, artifactStore, paths WorkspacePaths)`. `executeTask` uses `r.paths.PersistentWorkspaceDir(task.WorkspaceID)` etc. |

**Minimal approach**: Add only what’s needed. (1) server.Config: add `WorkspacesDir string`; in handlers that today call `config.PersistentWorkspaceDir` / `config.RuntimeWorkspaceDir` / `config.ArtifactDir`, use `filepath.Join(cfg.WorkspacesDir, workspaceID, "persist")` etc. when `WorkspacesDir != ""`, else fall back to `config.WorkspacesDir()`. (2) executor: introduce `WorkspacePaths` interface or struct with the three path methods; `New(..., paths)`; in cmd pass a default implementation that delegates to config.

### How they work together

1. cmd/server builds `server.Config` and sets `WorkspacesDir: config.WorkspacesDir()` (and keeps SessionsDir etc.).
2. cmd/server builds executor with `executor.New(st, st, config.DefaultWorkspacePaths())` (or equivalent).
3. Tests inject custom WorkspacesDir or WorkspacePaths to isolate filesystem.

### Changes for review (Refactor 2)

- **New (optional)**: `internal/config/workspace_paths.go` — `WorkspacePaths` struct and `DefaultWorkspacePaths()` returning implementation using `WorkspacesDir()`.
- **Modified**: `internal/server/server.go` — Config add `WorkspacesDir string`; handlers in artifacts.go, upload.go, files.go use Config’s value or fallback to config.
- **Modified**: `internal/executor/executor.go` — Add `WorkspacePaths` dependency to Runner; `New(store.TaskStore, store.ArtifactStore, WorkspacePaths)`; use it in `executeTask` instead of `config.PersistentWorkspaceDir` etc.
- **Modified**: `internal/cmd/server.go` — Pass WorkspacesDir and executor paths from config.

---

## Refactor 3: Extract shared domain types (internal/model)

### Goal

Domain entities live in `internal/model` with JSON tags only; store implements persistence and returns `*model.*`; server and executor use model types and store interfaces.

### Modules

| Module | Responsibility | Owns |
|--------|----------------|------|
| **internal/model** | Domain structs (User, Workspace, Project, Task, Artifact, ArtifactItem, ArtifactWithTask). JSON tags only; no GORM. | user.go, workspace.go, project.go, task.go, artifact.go (or single models.go). |
| **internal/store** | Implements store interfaces; depends on model; uses GORM. Own types for DB (embed or convert to/from model). | Store, interfaces returning *model.X, GORM mapping. |

### Structure

- `internal/model/` — New package.
  - `user.go`, `workspace.go`, `project.go`, `task.go`, `artifact.go` (or one file) — structs with `json:"snake_case"` only. No `TableName`; no GORM.
- `internal/store/models.go` — Becomes adapters: either store defines local structs with GORM tags that map to/from model, or store embeds model and adds GORM on a wrapper. Recommendation: store keeps its own GORM-tagged structs (e.g. `userRow`) and converts to `*model.User` on read and from `*model.User` on write; store interfaces then use `*model.User`, `*model.Task`, etc.

### Method / type design

| Type | Package | Fields | Notes |
|------|---------|--------|--------|
| **User** | model | UserID, Email, Name, CreatedAt | json snake_case; no ID (internal DB id). |
| **Workspace** | model | WorkspaceID, OwnerUserID, Name, CreatedAt | |
| **Project** | model | ProjectID, WorkspaceID, Name, Description, CreatedAt | |
| **Task** | model | TaskID, WorkspaceID, ProjectID, Status, Input, Output, CreatedBy, CreatedAt, StartedAt, EndedAt, ErrorMessage, SessionID, ArtifactSeq, LastArtifactID | |
| **Artifact** | model | TaskID, ArtifactID, CreatedAt, Seq | |
| **ArtifactItem** | model | ArtifactID, RelativePath | |
| **ArtifactWithTask** | model | ArtifactID, TaskID, WorkspaceID, ProjectID, CreatedAt, Seq, TaskInputSnippet | DTO for list. |

Store interfaces (in store) change to return/accept `*model.*`:

- `UserStore.UserByEmail(ctx, email) (*model.User, error)`; `CreateUser(ctx, email) (*model.User, error)`.
- `WorkspaceStore` — same idea: list and create return `[]model.Workspace` / `*model.Workspace`.
- `ProjectStore`, `TaskStore`, `ArtifactStore` — same: use `*model.Project`, `*model.Task`, `*model.Artifact`, `[]model.ArtifactItem`, `[]model.ArtifactWithTask`.

Store implementation: keep internal GORM structs (with GORM tags and TableName) in store; convert to model in each get/list/create method.

### How they work together

1. store interfaces are defined in store but use model types in signatures.
2. Server and executor import `model` for types and `store` for interfaces; they no longer need store’s struct definitions for response or task handling.
3. Store remains the only package that knows about GORM and DB schema.

### Changes for review (Refactor 3)

- **New**: `internal/model/` — All domain structs with json tags (and ErrEmailExists in model or store; if used only by store, keep in store).
- **Modified**: `internal/store/interfaces.go` — All method signatures use `*model.User`, `*model.Task`, etc.
- **Modified**: `internal/store/models.go` — Rename or replace with internal GORM-only structs (e.g. userRow) and conversion to/from model.
- **Modified**: `internal/store/*.go` — Implementations convert between GORM structs and model; return *model.X.
- **Modified**: `internal/server/**` — Use `model.*` in response types and handler logic; keep using store interfaces.
- **Modified**: `internal/executor/executor.go` — Use `model.Task` (or keep `store.Task` if store re-exports type as model.Task; prefer executor importing model and store returning *model.Task).

---

## Refactor 4: Invert tools–agent coupling (SubAgentRunner)

### Goal

The Task tool does not construct `agent.Agent` or use `agent.SystemPrompt`; it receives a `SubAgentRunner` interface and calls it. Agent package defines the interface and provides the default implementation; cmd wires it.

### Modules

| Module | Responsibility | Owns |
|--------|----------------|------|
| **internal/agent** | Agent loop; defines SubAgentRunner; provides default implementation. | Agent, SubAgentRunner, NewAgent, Process. |
| **internal/tools** | Task tool takes SubAgentRunner; no direct use of agent.NewAgent or agent.SystemPrompt. | TaskTool, AgentTypeConfig (still holds []agent.Tool), NewTask(caller, runner, agentTypes). |
| **internal/cmd** | Builds SubAgentRunner implementation and passes it to tools.NewTask. | setup.go wires runner. |

### Structure

- `internal/agent/agent.go` — Add `SubAgentRunner` interface and optional default impl type.
- `internal/tools/task.go` — TaskTool holds `runner agent.SubAgentRunner`; `Execute` calls `runner.RunSubAgent(...)` instead of `agent.NewAgent(...).Process(...)`.
- `internal/cmd/setup.go` — Build runner (e.g. `agent.NewSubAgentRunner(client)` or inline struct that calls `agent.NewAgent(...).Process(...)`); pass to `tools.NewTask(client, runner, agentTypes)`.

### Method / type design

| Location | Type / method | Signature | Responsibility |
|----------|---------------|-----------|----------------|
| **agent** | SubAgentRunner | `RunSubAgent(ctx context.Context, caller LLMCaller, tools []Tool, systemPrompt string, userMessage string) (reply string, err error)` | Run one sub-agent turn: build ephemeral session with user message, run loop, return assistant reply. |
| **agent** | (optional) | `func (a *Agent) RunSubAgent(ctx, tools, systemPrompt, userMessage) (string, error)` | Default implementation: create ephemeral session, build temp agent with given tools and prompt, call Process. Or a standalone function `RunSubAgent(ctx, caller, tools, systemPrompt, userMessage)` that creates agent and process. |
| **agent** | New type (optional) | `type subAgentRunner struct { caller LLMCaller }`; `func (r *subAgentRunner) RunSubAgent(ctx, tools, systemPrompt, userMessage) (string, error)` | Implements SubAgentRunner by calling `agent.NewAgent(r.caller, tools, agent.SystemPrompt(systemPrompt)).Process(ctx, sess, userMessage)`. |
| **tools** | TaskTool | `caller agent.LLMCaller`, `runner agent.SubAgentRunner`, `agentTypes map[string]AgentTypeConfig` | Execute calls `t.runner.RunSubAgent(ctx, t.caller, subTools, config.SystemPrompt, prompt)` instead of constructing Agent. |
| **tools** | NewTask | `NewTask(caller agent.LLMCaller, runner agent.SubAgentRunner, agentTypes map[string]AgentTypeConfig) (*TaskTool, error)` | Add runner param; validate runner != nil. |

**Interface design (agent package)**:

```go
// SubAgentRunner runs a single sub-agent invocation: given caller, tools, system prompt, and user message, returns the assistant reply.
type SubAgentRunner interface {
    RunSubAgent(ctx context.Context, caller LLMCaller, tools []Tool, systemPrompt string, userMessage string) (reply string, err error)
}
```

Default implementation: a function or a small struct in agent that uses `NewAgent(caller, tools, SystemPrompt(systemPrompt))` and `Process(ctx, session, userMessage)`. So tools still depend on agent (for Tool, LLMCaller, SubAgentRunner), but no longer on `NewAgent` or `SystemPrompt` in task.go code path—only the runner implementation does.

### How they work together

1. cmd/setup creates a SubAgentRunner (e.g. `agent.DefaultSubAgentRunner()` or `&agent.SubAgentRunnerImpl{}` that wraps the Process call).
2. cmd/setup calls `tools.NewTask(client, runner, agentTypes)`.
3. When the Task tool Execute runs, it calls `t.runner.RunSubAgent(ctx, t.caller, subTools, config.SystemPrompt, prompt)` and returns the reply.
4. Tests can pass a mock SubAgentRunner that returns a fixed string or error.

### Changes for review (Refactor 4)

- **New (in agent)**: `SubAgentRunner` interface and default implementation (struct or func).
- **Modified**: `internal/tools/task.go` — TaskTool has `runner agent.SubAgentRunner`; in Execute, replace `agent.NewAgent(...).Process(...)` with `t.runner.RunSubAgent(ctx, t.caller, subTools, config.SystemPrompt, prompt)`.
- **Modified**: `internal/tools/task.go` — `NewTask(caller, runner, agentTypes)`; add nil check for runner.
- **Modified**: `internal/cmd/setup.go` — Build SubAgentRunner; pass to `tools.NewTask(client, runner, agentTypes)`.

---

## Refactor 5: Split server package by domain (optional)

### Goal

Improve discoverability of server handlers by grouping routes and handlers by domain; keep `internal/server` as the single HTTP entry (Config, Server, routing, middleware).

### Modules

| Module | Responsibility | Owns |
|--------|----------------|------|
| **internal/server** | Routing, Config, Server, middleware (auth, CORS, logging). | server.go, auth.go, response.go, cors.go. |
| **internal/server** (same package) | Handlers grouped by file or (optional) subpackages. | workspaces.go, projects.go, tasks.go, artifacts.go, upload.go, files.go, login.go, otp.go, session.go. |

### Structure (minimal — no subpackages)

- Keep all code in `internal/server`.
- Add `internal/server/README.md` or a short comment block in server.go listing route groups and the file that implements them, e.g.:
  - `/api/workspaces` → workspaces.go
  - `/api/workspaces/{id}/projects` → projects.go
  - `/api/workspaces/{id}/tasks` → tasks.go
  - `/api/workspaces/{id}/artifacts`, artifact content/items → artifacts.go
  - Upload, files tree/content → upload.go, files.go
  - Auth, login, OTP → login.go, otp.go, auth.go
  - Sessions → session.go

No code signature changes; only documentation and possibly file renames for consistency.

### Optional: subpackages

- If subpackages are introduced: e.g. `server/workspace`, `server/artifact`, `server/auth`, each exposes `Register(mux *http.ServeMux, cfg *Config)` and server.go calls them. Config (or a handler-specific opts struct) is passed so handlers can access stores and paths. This is a larger change; the design prefers the minimal “documentation + same package” approach unless the team wants full subpackages.

### Changes for review (Refactor 5)

- **New (optional)**: `internal/server/README.md` — Route-to-file mapping.
- **Modified (optional)**: `internal/server/server.go` — Short comment listing which file handles which route group.
- **No** change to handler signatures or call flow unless subpackages are added.

---

## Implementation order and dependencies

1. **Refactor 1** (executor store) — No dependency on others; do first.
2. **Refactor 2** (config injection) — Independent; can run in parallel with 1 or right after.
3. **Refactor 3** (model) — Do after 1 so executor already uses store interfaces; then switch store and consumers to model types.
4. **Refactor 4** (SubAgentRunner) — Independent of 1–3; can be done after or in parallel with 2.
5. **Refactor 5** (server split) — Purely organizational; any time after 2/3 when server is stable.

---

## Changes for review (full list)

- **Refactor 1**: executor/executor.go (remove custom TaskStore; use store.TaskStore + store.ArtifactStore); cmd/server.go (executor.New(st, st)).
- **Refactor 2**: config (optional WorkspacePaths); server.Config (WorkspacesDir); executor (WorkspacePaths param); cmd (wire paths).
- **Refactor 3**: new internal/model; store interfaces and impls use model types; server and executor use model.
- **Refactor 4**: agent (SubAgentRunner + default impl); tools/task.go (take runner, call RunSubAgent); cmd/setup (wire runner).
- **Refactor 5**: server README or comments (route-to-file map); optional subpackages.
