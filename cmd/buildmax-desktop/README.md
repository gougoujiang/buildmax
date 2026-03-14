# BuildMax Desktop App

Native desktop application for BuildMax (Wails + Go). Provides a local, first-hand agent experience similar to the CLI, without requiring a backend server.

## Prerequisites

- **Go** (for building and for Wails)
- **Wails CLI** — Run **`./make setup`** from the repo root (installs Wails if missing), or install manually:

  ```bash
  go install github.com/wailsapp/wails/v2/cmd/wails@latest
  ```

  Ensure `$HOME/go/bin` (or `$GOPATH/bin`) is in your `PATH`.

- **macOS**: Xcode Command Line Tools (or full Xcode) for building the native app.

## Build

From the **repo root**:

```bash
cd cmd/buildmax-desktop && wails build
```

Or from this directory:

```bash
wails build
```

Output: `build/bin/buildmax-desktop` (or platform-specific path under `build/`).

## Run (dev mode)

From the **repo root**:

```bash
./make run desktop
```

Or from this directory:

```bash
wails dev
```

This opens a window with the BuildMax title and a placeholder for the future chat/agent UI.

## Configuration

The desktop app uses the same application data directory as the CLI: **`BUILDMAX_HOME`** (default `~/.buildmax`). Sessions and logs will use that path when agent features are added.

## Project layout

- `main.go` — Thin entrypoint; embeds frontend and calls `desktop.Run()`.
- `wails.json` — Wails project config.
- `frontend/` — Minimal HTML/CSS (title + placeholder).
- Implementation lives in **`internal/cmd/desktop`** (App, Run, Startup/Shutdown).
