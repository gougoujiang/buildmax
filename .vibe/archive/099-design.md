# Design 099 — Add desktop app (Wails basics)

## Goal

Add a Wails-based desktop application under **`cmd/buildmax-desktop/`** (entrypoint only) with real implementation in **`internal/cmd/desktop`**, matching the pattern of CLI/server/worker. The app builds and runs, shows a minimal window (title “BuildMax”, placeholder for future chat), and uses `internal/config` for application data directory. Same naming pattern: `cmd/buildmax` → `internal/cmd/cli`, `cmd/buildmax-server` → `internal/cmd/server`, `cmd/buildmax-worker` → `internal/cmd/worker`, **`cmd/buildmax-desktop`** → **`internal/cmd/desktop`**.

## Modules

| Module (package / dir)            | Responsibility | Owns |
|-----------------------------------|----------------|------|
| **cmd/buildmax-desktop**          | Entrypoint only. | `main.go` (thin: log init, call `desktop.Run()`), `wails.json`, `frontend/`. No app logic. |
| **internal/cmd/desktop**          | Desktop app implementation. | `App` struct, `NewApp()`, `Run()`, `Startup`/`Shutdown`; builds Wails options and calls `wails.Run()`; calls `config.DataDir()` in Startup. |
| **cmd/buildmax-desktop/frontend** | Minimal UI. | `index.html`, minimal JS/CSS to show “BuildMax” and a chat placeholder. |
| **internal/config**               | (existing) Application data path. | `DataDir()`, `LogsDir()`, etc. No changes. |

## Structure

**Directory layout**

- **`cmd/buildmax-desktop/`** — Wails project root; **entrypoint and assets only** (same idea as other cmd: thin main).
  - `main.go` — `package main`; init log (using `config.LogsDir()`, `config.LogLevel()`), then call `desktop.Run()`. No App struct or Wails logic here.
  - `wails.json` — Wails project config: `name`, `outputfilename`, `frontend:dir`, `frontend:build`, `frontend:dev`. Point frontend to `frontend` subdir.
  - `frontend/` — Minimal frontend assets.
    - `index.html` — Single page with app title “BuildMax” and a visible placeholder for “Chat / agent UI (coming soon)” or similar.
    - Optional: `app.js` / `style.css` for minimal styling; or vanilla HTML only.
- **`internal/cmd/desktop/`** — **Real implementation** (like `internal/cmd/cli`, `internal/cmd/server`, `internal/cmd/worker`).
  - `app.go` — `App` struct with `Startup(ctx)` and `Shutdown(ctx)`; in `Startup`, call `config.DataDir()` so BUILDMAX_HOME is respected.
  - `run.go` — `Run()` function: create `App` via `NewApp()`, build Wails options (bindings, lifecycle, window title “BuildMax”, frontend dir), call `wails.Run(...)`, return error. This is the single entry used by `cmd/buildmax-desktop/main.go`.

**Why cmd vs internal**

- `cmd/buildmax-desktop`: only the binary entrypoint and Wails project layout (main.go, wails.json, frontend/). Matches `cmd/buildmax` (thin) → `internal/cmd/cli`, `cmd/buildmax-server` (thin) → `internal/cmd/server`, `cmd/buildmax-worker` (thin) → `internal/cmd/worker`.
- `internal/cmd/desktop`: all desktop app logic, testable without building the Wails binary; can be unit-tested (e.g. App.Startup sets up config).

## Method design

**internal/cmd/desktop**

| Receiver / Type    | Method   | Signature               | Responsibility |
|--------------------|----------|-------------------------|----------------|
| **desktop**        | Run      | `() error`              | Create `App` via `NewApp()`, build Wails options (app as bindings, OnStartup/OnShutdown, window title “BuildMax”, frontend dir from project), call `wails.Run(...)`, return its error. |
| **desktop**        | NewApp   | `() *App`               | Return a new `App` instance (zero or minimal fields for this task). |
| **App**            | Startup  | `(ctx context.Context)` | Call `config.DataDir()` so application data path is set for the process. Return nil. |
| **App**            | Shutdown | `(ctx context.Context)` | No-op for this task; return nil. |

Optional: `App.GetDataDir() string` returning `config.DataDir()` for frontend bindings; not required for minimal UI.

**cmd/buildmax-desktop/main.go**

- Init log: `log.Init(log.LogConfig{LogsDir: config.LogsDir(), Level: config.LogLevel(), ...})` (same pattern as server/worker).
- Call `desktop.Run()`; if non-nil, print error and `os.Exit(1)`.
- No App struct or Wails types in main.

## How they work together

**Startup flow**

1. User runs `wails dev` (or built app) from `cmd/buildmax-desktop` (or via `make run desktop`).
2. `main.main()` inits log, then calls `desktop.Run()`. `desktop.Run()` creates `App`, builds Wails options (with `App` as bindings and lifecycle), and calls `wails.Run(...)`.
3. Wails starts the Go backend and loads the frontend (`frontend/index.html`). Before the window is shown, Wails calls `App.Startup(ctx)`; `App` calls `config.DataDir()` so any later use of sessions/logs uses BUILDMAX_HOME.
4. Window opens with frontend content: “BuildMax” and a placeholder for chat.

**Data path**

- `config.DataDir()` uses `BUILDMAX_HOME` env (default `~/.buildmax`). Desktop app does not override it; same as CLI. No new config surface in this task.

**Build and test**

- Desktop: `cd cmd/buildmax-desktop && wails build` (or `wails dev`). Does not affect `go build ./cmd/...` for other binaries.
- Existing: `go build ./...` builds `buildmax`, `buildmax-server`, `buildmax-worker`; `go test ./...` runs tests. Wails app is not part of `go build ./...` (Wails toolchain produces the desktop binary). So no regression to CLI/server/worker.

## wails.json (contract)

- **name**: e.g. `"BuildMax"` or `"buildmax-desktop"`.
- **outputfilename**: e.g. `"buildmax-desktop"`.
- **frontend**: `dir` = `"frontend"`; `build` and `dev` can be empty or a no-op command if frontend is static HTML (e.g. `echo` or `true`) so Wails does not require Node for this minimal UI.
- **author**, **info**: optional.

If the frontend is vanilla HTML/CSS/JS with no bundler, `frontend.build` can be omitted or a no-op; `frontend.dev` can be the same. That keeps the first step dependency-free beyond Wails CLI.

## Documentation and setup

- **One-click setup**: **`./make setup`** (which initializes everything for a new developer) must install the Wails CLI if missing, so desktop app development is ready after setup. Add an **ensure_wails** step in `setup/setup.sh`: ensure Go is available, then if `wails` is not in PATH run `go install github.com/wailsapp/wails/v2/cmd/wails@latest`; if after install `wails` is still not in PATH, log a hint to add `$GOPATH/bin` or `$HOME/go/bin` to PATH. Idempotent: if `wails` is already in PATH, skip.
- **cmd/buildmax-desktop/README.md** (or a section in repo README): (1) Prerequisite: run **`./make setup`** (installs Wails) or install Wails CLI manually (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`; ensure `$HOME/go/bin` or `$GOPATH/bin` is in PATH). (2) Build: `cd cmd/buildmax-desktop && wails build`. (3) Dev: `cd cmd/buildmax-desktop && wails dev` or **`./make run desktop`** from repo root.

## Changes for review

- **New**: `cmd/buildmax-desktop/main.go` — Thin entrypoint: `package main`, init log, call `desktop.Run()`, exit on error. No App or Wails types.
- **New**: `cmd/buildmax-desktop/wails.json` — Wails project configuration (name, output, frontend dir, optional no-op build/dev).
- **New**: `cmd/buildmax-desktop/frontend/index.html` — Minimal page: title “BuildMax”, placeholder for chat.
- **New** (optional): `cmd/buildmax-desktop/frontend/app.js`, `cmd/buildmax-desktop/frontend/style.css` if needed for a clear placeholder.
- **New**: `internal/cmd/desktop/app.go` — `App` struct, `NewApp()`, `Startup(ctx)` calling `config.DataDir()`, `Shutdown(ctx)`.
- **New**: `internal/cmd/desktop/run.go` — `Run()`: create App, build Wails options, call `wails.Run(...)`, return error.
- **New**: `cmd/buildmax-desktop/README.md` — How to install Wails, build, and run.
- **Modified**: repo root `make` script — optional `run desktop` target that runs `wails dev` from `cmd/buildmax-desktop`.
- **Modified**: **`setup/setup.sh`** — add **ensure_wails**: require Go, then install Wails CLI via `go install .../wails@latest` if not in PATH; log PATH hint if needed. Call ensure_wails from main (e.g. after ensure_brew) so `./make setup` gives new developers everything including desktop tooling.
- **Modified**: repo root `README.md` or docs — add one-line pointer to desktop app (e.g. “Desktop app: see `cmd/buildmax-desktop/README.md`”).
- **Dependency**: `go.mod` — add Wails v2 dependency; `internal/cmd/desktop` will import Wails and `internal/config`. No change to other cmd or internal packages beyond normal `go mod tidy`.

