# Design 090: Refactor cmd entrypoint

## Goal

Consolidate `internal/cmd`, `internal/servercmd`, and `internal/workercmd` under a single directory `internal/cmd` with subpackages `cli`, `server`, and `worker`. No behavior change; only moves, package renames, and import updates.

## Target layout

```
internal/cmd/
├── cli/
│   ├── root.go
│   ├── version.go
│   ├── print.go
│   ├── tui.go
│   ├── setup.go
│   └── root_test.go
├── server/
│   ├── run.go
│   └── run_test.go
└── worker/
    └── run.go
```

- **Removed**: top-level `internal/cmd` package (replaced by `internal/cmd/cli`), `internal/servercmd`, `internal/workercmd`.

## Modules

### 1. `internal/cmd/cli` (package `cli`)

**Responsibility**: Cobra root command, flags, version subcommand, print-mode and TUI runners, agent/session setup for CLI.

**Files moved from** `internal/cmd/`:

| File        | Contents |
|------------|----------|
| root.go    | `Version`, `NewRootCommand()`, `runRoot`, `resolveResumeID` |
| version.go | `newVersionCommand()` (unexported) |
| print.go   | `runPrintMode`, `stdoutStreamSink`, `formatDuration` |
| tui.go     | `runTUI` |
| setup.go   | `setupResult`, `toolBuilder`, `buildBaseTools`, `buildAgentTypes`, `setupAgentAndSession` |
| root_test.go | `TestRootCommand_InvalidSessionIDReturnsError` |

**Package rename**: `package cmd` → `package cli` in every file.

**Exported API** (unchanged semantics):

- `var Version string`
- `func NewRootCommand() *cobra.Command`

**Internal references**: All current `internal/cmd` files reference each other by same-package symbols (e.g. `runPrintMode`, `setupAgentAndSession`, `Version`). After the move they remain in the same package `cli`, so no cross-file changes except the package declaration.

**External imports**: `cmd/buildmax/main.go` will import `buildmax/internal/cmd/cli` and call `cli.NewRootCommand()`.

---

### 2. `internal/cmd/server` (package `server`)

**Responsibility**: Server startup — load env, open DB, blob storage, executor, run HTTP server.

**Files moved from** `internal/servercmd/`:

| File       | Contents |
|-----------|----------|
| run.go    | `chatTitleGenAdapter`, `RunServer(ctx, port)` |
| run_test.go | `TestResolveServerPort_UsedByRunServer` |

**Package rename**: `package servercmd` → `package server` in both files.

**Exported API**:

- `func RunServer(ctx context.Context, port int) error`

**External imports**: `cmd/buildmax-server/main.go` will import `buildmax/internal/cmd/server` and call `server.RunServer(ctx, port)`.

---

### 3. `internal/cmd/worker` (package `worker`)

**Responsibility**: Worker run — validate env, fetch chat run via API, materialize, run buildmax, update status.

**Files moved from** `internal/workercmd/`:

| File   | Contents |
|--------|----------|
| run.go | `ErrAlreadyClaimed`, `RunWorker(ctx, chatRunID)` |

**Package rename**: `package workercmd` → `package worker` in `run.go`.

**Exported API**:

- `var ErrAlreadyClaimed error`
- `func RunWorker(ctx context.Context, chatRunID string) error`

**External imports**: `cmd/buildmax-worker/main.go` will import `buildmax/internal/cmd/worker`, call `worker.RunWorker(ctx, *chatRunID)`, and use `worker.ErrAlreadyClaimed` for exit-code handling.

---

## Method / API design (contract)

No signature or behavior changes. Only import paths and package names change.

| Location              | Symbol            | Signature / type (unchanged) |
|-----------------------|-------------------|------------------------------|
| internal/cmd/cli      | Version           | `var Version string`         |
| internal/cmd/cli      | NewRootCommand    | `func NewRootCommand() *cobra.Command` |
| internal/cmd/server   | RunServer         | `func RunServer(ctx context.Context, port int) error` |
| internal/cmd/worker   | ErrAlreadyClaimed | `var ErrAlreadyClaimed error` |
| internal/cmd/worker   | RunWorker         | `func RunWorker(ctx context.Context, chatRunID string) error` |

---

## How they work together

- **cmd/buildmax/main.go**: `log.Init(...)` → `cli.NewRootCommand()` → `root.Execute()`.
- **cmd/buildmax-server/main.go**: `log.Init(...)` → `flag` for port → `server.RunServer(ctx, port)`.
- **cmd/buildmax-worker/main.go**: `log.Init(...)` → `flag` for `--chat-run-id` → `worker.RunWorker(ctx, chatRunID)`; on `errors.Is(err, worker.ErrAlreadyClaimed)` exit 2, else exit 1 on error.

No new call paths; no package depends on another of the three entry packages (cli, server, worker). Dependencies remain: cli → agent, config, session, llm, tools, app, tui, util, executor; server → config, executor, llm, server, session, storage; worker → config, executor, storage, workerapi.

---

## Implementation order

1. **Create directories**: `internal/cmd/cli`, `internal/cmd/server`, `internal/cmd/worker`.
2. **CLI**: Move `internal/cmd/*.go` and `internal/cmd/*_test.go` into `internal/cmd/cli/`; in each file change `package cmd` to `package cli`.
3. **Server**: Move `internal/servercmd/run.go` and `run_test.go` into `internal/cmd/server/`; change `package servercmd` to `package server`.
4. **Worker**: Move `internal/workercmd/run.go` into `internal/cmd/worker/`; change `package workercmd` to `package worker`.
5. **Entrypoints**: Update `cmd/buildmax/main.go`, `cmd/buildmax-server/main.go`, `cmd/buildmax-worker/main.go` imports and calls as specified in the task.
6. **Delete**: Remove directories `internal/servercmd` and `internal/workercmd`. Remove all files from the old `internal/cmd` (they now live under `internal/cmd/cli`); do not leave a top-level `internal/cmd` package (no `internal/cmd/something.go` at the same level as `internal/cmd/cli/`).
7. **Verify**: `go build ./...`, `go test ./...`.

---

## Changes for review

| Change type   | Item |
|---------------|------|
| **New**       | `internal/cmd/cli/` — root.go, version.go, print.go, tui.go, setup.go, root_test.go (from `internal/cmd`), package `cli`. |
| **New**       | `internal/cmd/server/` — run.go, run_test.go (from `internal/servercmd`), package `server`. |
| **New**       | `internal/cmd/worker/` — run.go (from `internal/workercmd`), package `worker`. |
| **Edit**      | `cmd/buildmax/main.go` — import `buildmax/internal/cmd/cli`, use `cli.NewRootCommand()`. |
| **Edit**      | `cmd/buildmax-server/main.go` — import `buildmax/internal/cmd/server`, use `server.RunServer(ctx, port)`. |
| **Edit**      | `cmd/buildmax-worker/main.go` — import `buildmax/internal/cmd/worker`, use `worker.RunWorker`, `worker.ErrAlreadyClaimed`. |
| **Delete**    | `internal/cmd/*.go` and `internal/cmd/*_test.go` (moved to `internal/cmd/cli/`). |
| **Delete**    | `internal/servercmd/` (moved to `internal/cmd/server/`). |
| **Delete**    | `internal/workercmd/` (moved to `internal/cmd/worker/`). |

No new dependencies. No changes to AGENTS.md or other docs in this task.
