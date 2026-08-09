# Log

> **Audience:** contributors · **Status:** current

## Purpose

The `internal/infra/log` package configures the application's default `log/slog` logger. Log level comes from `log_level` in `settings.yaml` (CLI and desktop) or `server.yaml` (server and worker) — `debug`, `info`, `warn`, `error`, `off`, defaulting to `info` via `config.LogLevel()`. Output goes only to a rotating file under `config.DataDir()/logs/buildmax.log` (Lumberjack); nothing is written to stdout/stderr so the TUI and prompt-mode output stay clean.

## Key Functions

| Name | Role |
|------|------|
| **Init()** | Sets slog level from env, creates logs dir, configures file-only handler with Lumberjack |
| **DisableConsole()** | Re-applies file-only output (e.g. after something changed the default logger) |
| **SetOutput(w)** | Replaces default logger with one writing to `w`; used by tests to capture logs |

## How It Works

- `Init(LogConfig{LogsDir, Level, Filename, AlsoStdout})` — the caller passes the
  resolved level in, so this package never reads configuration itself.
- Level: `parseLevel` is case-insensitive; invalid or empty defaults to Info;
  `"off"` disables logging.
- File: `LogsDir`/`buildmax.log` by default, max 10MB, 3 backups, 7 days,
  compressed. `AlsoStdout` additionally tees to stdout; the CLI leaves it false.
- If the logs directory cannot be created, the default logger discards output
  rather than falling back to stderr and corrupting the TUI.

## Dependencies

- **Uses**: `gopkg.in/natefinch/lumberjack.v2`
- **Used by**: process bootstrap in `internal/bootstrap` and `cmd/*`, which
  resolve `config.LogsDir()` and `config.LogLevel(...)` and pass them in

## Notes

- See [Configuration](config.md) for where `log_level` comes from and for the
  data directory layout.
