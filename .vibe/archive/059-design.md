# Design 059: Entry cmd binary split (part 1)

## Goal

Introduce a dedicated server binary (`buildmax-server`) and a callable server-startup unit so the CLI binary no longer links server/executor/storage code, achieving a clear split between agent CLI and backend server.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **cmd/buildmax** | CLI entry: log init, Cobra root execute. | main.go (unchanged). |
| **cmd/buildmax-server** | Server entry: log init, port parsing, call RunServer. | main.go (new). |
| **internal/cmd** | CLI only: root, version, TUI, print mode. No server. | root.go, version.go, tui.go, print.go, setup.go. |
| **internal/servercmd** | Server startup: config load, DB, blob storage, executor, HTTP server run. | RunServer; workspace paths impl for executor. |
| **internal/executor** | Unchanged: WorkspacePaths interface, Runner, spawns `buildmax -p`. | executor.go, paths.go. |
| **internal/server** | Unchanged: HTTP server, Config, routes. | server.go, routes. |
| **internal/config** | Unchanged: ResolveServerPort, LoadServerEnv, WorkspacesDir, storage builders. | config.go, env_spec.go, workspace_storage.go. |

## Structure

**Directory / files**

- `cmd/buildmax/` — CLI binary
  - `main.go` — unchanged: log.Init(), NewRootCommand().Execute()
- `cmd/buildmax-server/` — Server binary (new)
  - `main.go` — log.Init(); parse --port (default 0); port = config.ResolveServerPort(portFlag); servercmd.RunServer(ctx, port)
- `internal/cmd/` — CLI commands only
  - `root.go` — remove AddCommand(newServerCommand()); no server imports
  - `version.go`, `tui.go`, `print.go`, `setup.go` — unchanged
  - `server.go` — **deleted** (moved to internal/servercmd)
  - `server_test.go` — **deleted** (replaced by servercmd test)
- `internal/servercmd/` — Server startup (new package)
  - `run.go` — RunServer(ctx context.Context, port int) error; contains all current runServer logic (workspaces dir, entity.New, blob builders, executor.New+Start+Stop, server.New+Run)
  - `paths.go` — serverWorkspacePaths struct implementing executor.WorkspacePaths (moved from internal/cmd/server.go)
  - `run_test.go` — test that ResolveServerPort is used correctly (port resolution); optional: test RunServer fails fast on missing env (e.g. workspaces dir) without starting HTTP

**Main types and interfaces**

- **RunServer** (internal/servercmd): `func RunServer(ctx context.Context, port int) error`. Loads config, opens DB, builds blob storage, creates executor and HTTP server, runs server until shutdown. Single entry for the server binary.
- **serverWorkspacePaths** (internal/servercmd): struct with `root string`; implements executor.WorkspacePaths (PersistentWorkspaceDir, RuntimeWorkspaceDir, RuntimeTaskBuildmaxDir, RuntimeTaskWSDir, ArtifactDir). Built from normalized workspacesDir at startup.
- **WorkspacePaths** (internal/executor): unchanged interface; servercmd provides one implementation.

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| (package) | **RunServer** | `(ctx context.Context, port int) error` | Load server env and workspaces dir; open DB; build persist and artifact storage; create executor and start it; create and run HTTP server; return when server exits. |
| **serverWorkspacePaths** | **PersistentWorkspaceDir** | `(workspaceID string) string` | Return filepath.Join(p.root, workspaceID, "persist"). |
| **serverWorkspacePaths** | **RuntimeWorkspaceDir** | `(workspaceID, taskID string) string` | Return filepath.Join(p.root, workspaceID, "tasks", taskID). |
| **serverWorkspacePaths** | **RuntimeTaskBuildmaxDir** | `(workspaceID, taskID string) string` | Return .../tasks/taskID/buildmax. |
| **serverWorkspacePaths** | **RuntimeTaskWSDir** | `(workspaceID, taskID string) string` | Return .../tasks/taskID/ws. |
| **serverWorkspacePaths** | **ArtifactDir** | `(workspaceID, taskID, artifactID string) string` | Return .../artifacts/taskID/artifactID. |

Port and env: RunServer uses `config.ResolveServerPort(port)` and `config.LoadServerEnv()`, `config.WorkspacesDir()` (required), same as current runServer. No Cobra; the server main passes the resolved port into RunServer.

## How they work together

**Data/control flow**

1. User runs `buildmax-server` (or `./make run server` which builds and runs it). `cmd/buildmax-server/main.go` runs: parse --port (e.g. flag.Int("port", 0, ...)), resolve port via config.ResolveServerPort(portFlag), then servercmd.RunServer(context.Background(), port).
2. RunServer: load server env; require workspaces dir and normalize to absolute; create serverWorkspacePaths{root: workspacesDir}; open entity store (MySQL); build persist and artifact storage (config); create executor.Runner with those deps, Start() it, defer Stop(); build server.Config and server.New(cfg); s.Run() (blocks until shutdown).
3. Executor (unchanged) continues to run `exec.CommandContext(ctx, "buildmax", "-p", ...)`. So the CLI binary `buildmax` must remain on PATH when the server runs; no change to executor code.
4. CLI: `buildmax` from cmd/buildmax. Root has no server subcommand; only version, TUI, -p. internal/cmd does not import internal/server, internal/executor, or storage in the default build path.

**Dependencies**

- cmd/buildmax → internal/cmd, internal/log. internal/cmd (CLI) → config, session, cobra, uuid; no server/executor/entity/blob.
- cmd/buildmax-server → internal/log, internal/servercmd. internal/servercmd → internal/config, internal/executor, internal/server, internal/storage/entity, internal/storage/blob.
- internal/executor → config, storage/entity, storage/blob, util; defines WorkspacePaths interface. internal/servercmd implements WorkspacePaths and uses executor.New.

**Key data structures**

- **server.Config** (internal/server): built in RunServer from entity store, blob storage, workspaces dir, JWT/CORS from LoadServerEnv. Passed to server.New(cfg).
- **serverWorkspacePaths**: created in RunServer with normalized workspacesDir root; passed to executor.New so the executor knows where to run tasks and write artifacts.

## Make and AGENTS.md

- **make build**: Keep building only the CLI: `go build -o buildmax ./cmd/buildmax`. No change to default build output.
- **make run server**: Change to build the server binary and run it: `go build -o buildmax-server ./cmd/buildmax-server` then `./buildmax-server` (with optional --port via flag; server main will pass it to RunServer). Optionally support `./make build server` to only build buildmax-server, and document that two binaries exist (buildmax = CLI, buildmax-server = backend).
- **AGENTS.md**: In "Project Directory Structure", add `cmd/buildmax-server/` with main.go. In "Build & Test", state that `./make build` builds the CLI; running the server is done via the separate binary (e.g. `./make run server` runs buildmax-server). Remove or rephrase "buildmax server" subcommand as "replaced by running the buildmax-server binary".

## Tests

- **internal/servercmd/run_test.go**: At least one test that port resolution works (e.g. ResolveServerPort(0) returns default when env unset). Can mirror current internal/cmd/server_test.go. No need to start the real HTTP server in tests.
- **internal/cmd**: Remove server_test.go. root_test.go and other cmd tests remain; ensure no reference to server command.
- **go build ./cmd/buildmax** and **go build ./cmd/buildmax-server** both succeed; **go test ./...** passes.

## Changes for review

- **New**: `cmd/buildmax-server/main.go` — main that inits log, parses --port, calls servercmd.RunServer(ctx, port).
- **New**: `internal/servercmd/run.go` — RunServer(ctx, port) with full server startup logic (moved from internal/cmd/server.go runServer).
- **New**: `internal/servercmd/paths.go` — serverWorkspacePaths implementing executor.WorkspacePaths (moved from internal/cmd/server.go).
- **New**: `internal/servercmd/run_test.go` — test(s) for port resolution / RunServer contract (e.g. missing env returns error).
- **Modified**: `internal/cmd/root.go` — remove `root.AddCommand(newServerCommand())` and any server-related comment.
- **Deleted**: `internal/cmd/server.go` — logic moved to internal/servercmd.
- **Deleted**: `internal/cmd/server_test.go` — replaced by internal/servercmd tests.
- **Modified**: `make` — `cmd_run_server` builds `buildmax-server` from `./cmd/buildmax-server` and runs `./buildmax-server` (with --port if needed); usage/help text updated to mention two binaries.
- **Modified**: AGENTS.md — directory structure (add cmd/buildmax-server), Build & Test (build = CLI; run server = buildmax-server binary), and internal/cmd description (no server subcommand).
