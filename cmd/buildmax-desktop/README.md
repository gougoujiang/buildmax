# BuildMax Desktop App

Native desktop application for BuildMax (Wails + Go). Provides a local, first-hand agent experience similar to the CLI, without requiring a backend server.

## Prerequisites

- **Go** (for building and for Wails)
- **Node.js** and **npm** (for the React frontend in `desktop/frontend/`)
- **Wails CLI** — Run **`./make setup`** from the repo root (installs Wails if missing), or install manually:

  ```bash
  go install github.com/wailsapp/wails/v2/cmd/wails@latest
  ```

  Ensure `$HOME/go/bin` (or `$GOPATH/bin`) is in your `PATH`.

- **macOS**: Xcode Command Line Tools (or full Xcode) for building the native app.

## Build

**Production build** (Wails runs `npm install` and `npm run build` in `desktop/frontend/` first, then builds the Go app):

From the **repo root**:

```bash
cd cmd/buildmax-desktop && wails build
```

Or from this directory:

```bash
wails build -tags desktop
```

Output: `build/bin/buildmax-desktop` (or platform-specific path under `build/`).

To build the React frontend only (e.g. for testing): `cd desktop/frontend && npm install && npm run build`.

**Note:** `-tags desktop` is required. The frontend bundle in
`desktop/frontend/dist/` is embedded only under that tag, so that plain
`go build ./...` keeps working on a checkout where the bundle has not been built
yet. `wails build -tags desktop` produces the bundle first, and `./make build`
does the same. A binary built without the tag refuses to start and prints how to
rebuild it — except during Wails' binding generation, which strips the `desktop`
tag by design; see [bindings_on.go](bindings_on.go).

## Run (dev mode)

From the **repo root**:

```bash
./make run desktop
```

Or from this directory:

```bash
wails dev
```

This starts the Vite dev server for the React frontend and opens the app window with hot reload.

## Configuration

The desktop app uses the same application data directory as the CLI: **`BUILDMAX_HOME`** (default `~/.buildmax`). Sessions and logs will use that path when agent features are added.

## Project layout

- **`cmd/buildmax-desktop/`** — Entrypoint only: `main.go`, `wails.json`.
- **`desktop/`** (repo root) — React + Vite app in `desktop/frontend/` (src/, index.html, package.json). `desktop/assets_embed.go` embeds `frontend/dist/` under the `desktop` build tag; `desktop/assets_stub.go` is compiled without it and embeds nothing. Wails runs `npm run build` to produce `dist/`, then the Go binary serves it.
- **`internal/interface/desktop`** — App logic (App, Run, Startup/Shutdown).
