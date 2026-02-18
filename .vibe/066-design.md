# Design 066: Separate log file for worker and server

## Goal

Implement per-binary log file names (buildmax.log, buildmax-server.log, buildmax-worker.log), optional stdout for server/worker only, and upload of the worker log file when uploading task buildmax to persist storage.

## Modules

| Module | Responsibility |
|--------|-----------------|
| **internal/log** | Init(filename, alsoStdout); file name and file+stdout vs file-only. |
| **cmd/buildmax** | Init("", false) — default file, file-only. |
| **cmd/buildmax-server** | Init("buildmax-server.log", true). |
| **cmd/buildmax-worker** | Init("buildmax-worker.log", true). |
| **internal/executor** | uploadTaskBuildmax: also upload worker log from config.LogsDir()/buildmax-worker.log to relPath logs/buildmax-worker.log. |

## Structure

### internal/log

- **Constants**: Keep `logFilename = "buildmax.log"` as the default when `filename` is empty. Lumberjack settings unchanged.
- **Variables**: `currentLevel`, `fileWriter` (io.Writer, the Lumberjack only) — same as today. `fileWriter` is always the file writer so DisableConsole can restore file-only.
- **Init(filename string, alsoStdout bool)**:
  - If `filename == ""`, use `logFilename` ("buildmax.log").
  - Otherwise use `filename` under `config.LogsDir()`.
  - Create logs dir; on MkdirAll failure set fileWriter = nil, default logger to discard, return.
  - Build Lumberjack with `filepath.Join(logsDir, chosenName)`.
  - Set `fileWriter = lj`.
  - Build output for slog: if `alsoStdout` then `io.MultiWriter(lj, os.Stdout)`, else `lj`.
  - Set slog.Default to TextHandler with that output and parsed level.
- **DisableConsole**: Unchanged — reapply handler with `fileWriter` (or discard) and currentLevel. Still file-only.
- **SetOutput**: Unchanged — replace default with handler to `w` and currentLevel.
- **parseLevel**: Unchanged.

Package comment: update to describe that Init accepts filename and alsoStdout; when alsoStdout is true logs go to file and stdout (for server/worker); when false file-only (CLI).

### internal/executor

- **uploadTaskBuildmax(ctx, buildmaxDir, workspaceID, taskID, persist)** (signature unchanged):
  - Keep existing logic: relPaths from buildmaxDir (logs/buildmax.log, settings.json, sessions/*.json); for each existing file open and PutTaskBuildmax; log and continue on error.
  - After that loop, add: upload worker log from worker’s log dir. Path = `filepath.Join(config.LogsDir(), "buildmax-worker.log")`. If os.Stat succeeds and is regular, open file, call `persist.PutTaskBuildmax(ctx, workspaceID, taskID, "logs/buildmax-worker.log", f)`, close; on open or put error log and continue (best-effort). Use literal `"buildmax-worker.log"` in executor (no dependency on log package for the name).

### Entry points

- **cmd/buildmax/main.go**: Replace `log.Init()` with `log.Init("", false)`.
- **cmd/buildmax-server/main.go**: Replace `log.Init()` with `log.Init("buildmax-server.log", true)`.
- **cmd/buildmax-worker/main.go**: Replace `log.Init()` with `log.Init("buildmax-worker.log", true)`.

### Tests

- **internal/log/log_test.go**: All four tests call `Init()` today. Change each to `Init("", false)` so behavior is unchanged (default file name, file-only; tests then SetOutput to capture in buffer).

## Method design

### log.Init(filename string, alsoStdout bool)

- **Receiver**: none (package-level).
- **Parameters**: `filename` — log file name under LogsDir(); empty means "buildmax.log". `alsoStdout` — if true, logs also go to os.Stdout; if false, file only.
- **Behavior**: Parse level from config.LogLevel(). Create LogsDir(); on failure set default to discard and return. Create Lumberjack for `LogsDir()/chosenName` (chosenName = filename or "buildmax.log"). Set fileWriter = lj. Build out = lj or MultiWriter(lj, os.Stdout). Set slog.Default to TextHandler(out, level).
- **No return value.**

### uploadTaskBuildmax (internal/executor)

- **Signature**: unchanged. `func uploadTaskBuildmax(ctx context.Context, buildmaxDir, workspaceID, taskID string, persist blob.PersistStorage)`.
- **New behavior**: After uploading files from buildmaxDir, open `filepath.Join(config.LogsDir(), "buildmax-worker.log")`. If open succeeds, call `persist.PutTaskBuildmax(ctx, workspaceID, taskID, "logs/buildmax-worker.log", f)`, then close; on error log and continue. If file does not exist or is not regular, skip silently (same as other missing files).

## How they work together

1. **Startup**: Each binary calls `log.Init` with its chosen filename and alsoStdout. CLI gets file-only buildmax.log; server gets file+stdout buildmax-server.log; worker gets file+stdout buildmax-worker.log.
2. **Task run**: Worker runs with BUILDMAX_HOME set to task buildmax dir for the child `buildmax -p`; the worker process itself uses the worker’s DataDir (and thus LogsDir() is the worker’s log dir). So the worker’s buildmax-worker.log lives at config.LogsDir()/buildmax-worker.log in the worker process.
3. **Upload**: After buildmax -p exits, RunTask calls uploadTaskBuildmax(buildmaxDir, ...). uploadTaskBuildmax uploads buildmaxDir contents (agent log, sessions, settings) then uploads the worker’s own log from config.LogsDir()/buildmax-worker.log to the same task buildmax prefix as logs/buildmax-worker.log. Persist implementation (local_fs vs minio) unchanged; local_fs no-ops PutTaskBuildmax, minio writes.

## Changes for review

| Package / File | Change |
|----------------|--------|
| **internal/log/log.go** | Replace `Init()` with `Init(filename string, alsoStdout bool)`. Use chosenName (filename or default); when alsoStdout use MultiWriter(lj, os.Stdout). Update package comment. |
| **internal/log/log_test.go** | Replace every `Init()` with `Init("", false)`. |
| **cmd/buildmax/main.go** | `log.Init("", false)`. |
| **cmd/buildmax-server/main.go** | `log.Init("buildmax-server.log", true)`. |
| **cmd/buildmax-worker/main.go** | `log.Init("buildmax-worker.log", true)`. |
| **internal/executor/executor.go** | In uploadTaskBuildmax, after the existing relPaths loop, add: open config.LogsDir()/buildmax-worker.log; if ok, PutTaskBuildmax(..., "logs/buildmax-worker.log", f); close; log and continue on error. |

No new packages. No new env vars. No changes to blob.PersistStorage interface or implementations.
