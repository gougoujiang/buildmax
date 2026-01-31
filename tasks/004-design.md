# Design 004 - Log Framework (slog)

## Goal

Configure Go's standard **`log/slog`** as the application logger with level from **`BUILDMAX_LOG_LEVEL`**, dual output (stderr + rotating file under **`config.DataDir()/logs`**), and a test-only **`SetOutput(w)`** so the rest of the app can log via slog and tests can assert on log content.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/log** | Configure slog default logger at startup; parse BUILDMAX_LOG_LEVEL; create logs dir and Lumberjack file; expose SetOutput for tests. | `log.go`, `log_test.go`; `Init()`, `SetOutput(w)`, level parsing. |
| **cmd/buildmax** | CLI entry; call log init at startup before any logging. | `main.go` (add one call to log init). |

## Structure

**Directory / files**

- `internal/log/` — log bootstrap (new package)
  - `log.go` — `Init()`, `SetOutput(w io.Writer)`, `parseLevel(s string) slog.Level` (package-internal), package-level state for current level and optional Lumberjack ref.
  - `log_test.go` — unit tests: level filtering, env default, output content; use `SetOutput(bytes.Buffer)` and `t.Setenv` for BUILDMAX_LOG_LEVEL.
- `cmd/buildmax/`
  - `main.go` — add import alias `buildmaxlog "buildmax/internal/log"` and call **`buildmaxlog.Init()`** at the very start of `main()` (before flag parse or other logic that might log).
- Repository root
  - `go.mod` — add dependency **`gopkg.in/natefinch/lumberjack.v2`** (or v3). No other new deps.

**Main types and interfaces**

- No new public types. Package **internal/log** keeps package-level state:
  - **currentLevel** (`slog.Level`): set in `Init()` from env; used by `SetOutput(w)` to build a handler with the same level writing only to `w`.
  - Optionally: a reference to the Lumberjack writer (for future flush/close if needed); not required for this task.

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|-----------------|
| (package) | **Init** | `()` | Read `BUILDMAX_LOG_LEVEL` via `parseLevel(os.Getenv("BUILDMAX_LOG_LEVEL"))`; default to `slog.LevelInfo` if unset/invalid. Create `config.DataDir()/logs` with `os.MkdirAll(..., 0750)`. Create Lumberjack writer for `filepath.Join(config.DataDir(), "logs", "buildmax.log")` with sensible defaults (MaxSize e.g. 10 MB, MaxBackups e.g. 3, MaxAge e.g. 7 days, Compress e.g. true). Set `slog.Default()` to `slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, lumberjack), &slog.HandlerOptions{Level: level}))`. Store `level` in package state for `SetOutput`. |
| (package) | **SetOutput** | `(w io.Writer)` | Replace `slog.Default()` with a logger whose handler is `slog.NewTextHandler(w, &slog.HandlerOptions{Level: currentLevel})`. Used by tests to capture output in a buffer. Idempotent: calling again replaces with a new handler for the new `w`. |
| (package) | **parseLevel** | `(s string) slog.Level` | Map string (case-insensitive) to slog level: "debug"→Debug, "info"→Info, "warn"→Warn, "error"→Error, "off"→max (e.g. slog.LevelError+1 or use a level that drops all). Invalid/empty → `slog.LevelInfo`. Unexported. |

**Lumberjack defaults (in code or comment)**

- Filename: `filepath.Join(config.DataDir(), "logs", "buildmax.log")`.
- MaxSize: e.g. 10 (MB).
- MaxBackups: e.g. 3.
- MaxAge: e.g. 7 (days).
- Compress: e.g. true for rotated files.

**Test cases**

- **Level filtering**: `SetOutput(buf)`; set level to Info (e.g. by calling `Init()` after `t.Setenv("BUILDMAX_LOG_LEVEL","info")`); call `slog.Debug("x")` → buf empty or no "x"; call `slog.Info("y")` → buf contains "y" and level INFO.
- **Env default**: `t.Setenv("BUILDMAX_LOG_LEVEL","debug")`, `Init()`, `SetOutput(buf)`, `slog.Debug("z")` → buf contains "z". Restore env after test (defer or explicit) so other tests are not affected.
- **Output content**: After `SetOutput(buf)` and Init (or level set), log one message; assert buffer contains expected level string and message text (slog TextHandler format).

## How they work together

**Data/control flow**

1. **Startup**: `main()` calls **`buildmaxlog.Init()`** first. `Init()` reads `BUILDMAX_LOG_LEVEL`, creates `DataDir()/logs`, creates Lumberjack + MultiWriter(stderr, file), sets `slog.Default()` to a logger with that handler and level. All subsequent `slog.Info`, `slog.Debug`, etc. from any package go to both stderr and the rotating file.
2. **Tests**: Tests that need to assert on log output call **`SetOutput(buf)`** (and optionally **`Init()`** with env set). Then they call `slog.Info(...)` etc. and assert on `buf`. No real file or stderr is written when using `SetOutput`.
3. **Log file path**: Resolved at init via **`config.DataDir()`**; logs dir created there; Lumberjack writes to `{DataDir()}/logs/buildmax.log` and rotates by size/backups/age.

**Dependencies**

- **internal/log** depends on **internal/config** (for `config.DataDir()`), **log/slog**, **io**, **os**, **path/filepath**, and **gopkg.in/natefinch/lumberjack.v2**.
- **cmd/buildmax** depends on **internal/log** only for the single `Init()` call.
- **internal/config** is unchanged; no new dependencies there.

**Key data structures**

- **Package-level level**: Stored in `internal/log` after `Init()` so `SetOutput(w)` can build a handler with the same minimum level. Enables tests to use `SetOutput` without re-parsing env.

## Changes for review

- **New**: **`internal/log/log.go`** — `Init()`, `SetOutput(w io.Writer)`, unexported `parseLevel(s string) slog.Level`; package-level current level; create logs dir and Lumberjack in Init; set `slog.Default()` with TextHandler to MultiWriter(stderr, lumberjack).
- **New**: **`internal/log/log_test.go`** — tests for level filtering, env-based level (BUILDMAX_LOG_LEVEL=debug), and output content using `SetOutput(bytes.Buffer)` and `t.Setenv`.
- **Modified**: **`cmd/buildmax/main.go`** — add `buildmaxlog "buildmax/internal/log"` and call **`buildmaxlog.Init()`** at the start of `main()`.
- **Modified**: **`go.mod`** — add **`gopkg.in/natefinch/lumberjack.v2`** (or v3).
- **Unchanged**: `internal/config` — no changes; log package calls `config.DataDir()` only.
