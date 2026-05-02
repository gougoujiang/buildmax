# Log

## Purpose

The `internal/infra/log` package configures the application's default `log/slog` logger. Log level is read from `BUILDMAX_LOG_LEVEL` (debug, info, warn, error, off). Output goes only to a rotating file under `config.DataDir()/logs/buildmax.log` (Lumberjack); nothing is written to stdout/stderr so the TUI and prompt-mode output stay clean.

## Key Functions

| Name | Role |
|------|------|
| **Init()** | Sets slog level from env, creates logs dir, configures file-only handler with Lumberjack |
| **DisableConsole()** | Re-applies file-only output (e.g. after something changed the default logger) |
| **SetOutput(w)** | Replaces default logger with one writing to `w`; used by tests to capture logs |

## How It Works

- Level: `parseLevel(BUILDMAX_LOG_LEVEL)` — case-insensitive; invalid or empty defaults to Info; `"off"` disables logging.
- File: `config.LogsDir()`/`buildmax.log`, max 10MB, 3 backups, 7 days, compressed.
- If the logs directory cannot be created, the default logger is set to discard.

## Dependencies

- **Uses**: `internal/config` (DataDir/LogsDir), `gopkg.in/natefinch/lumberjack.v2`
- **Used by**: `cmd/buildmax/main.go` (calls `log.Init()` at startup)

## Notes

- See [Configuration](config.md) for `BUILDMAX_HOME` and data directory layout.
