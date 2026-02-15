# Design 003 - Application Data Folder Configuration

## Goal

Define how the BuildMax application data folder path is resolved (default under user home, optional `HOME_DIR` override) and how tests use a workspace `testing-sandbox` via a Makefile `test` target.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/config** | Configuration loading and defaults (LLM + data dir). | `config.go`, `config_test.go`; `LoadLLM()`, `DataDir()`. |
| **Repository root** | Build and test entry points; sandbox dir and ignore rules. | `Makefile`, `.gitignore`; `testing-sandbox/` (created by Makefile). |

## Structure

**Directory / files**

- `internal/config/` — configuration (unchanged package boundary)
  - `config.go` — existing LLM types and `LoadLLM()`; **add** `DataDir()`.
  - `config_test.go` — **new**; unit tests for `DataDir()`.
- Repository root
  - `Makefile` — **new**; target `test` creates `testing-sandbox`, sets `HOME_DIR`, runs `go test ./...`.
  - `.gitignore` — **modified**; add `testing-sandbox/`.

**Main types and interfaces**

- No new types. Only a new **function** `DataDir() string` in package `config`.

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|-----------------|
| (package) | **DataDir** | `() string` | Return the application data folder path: if `HOME_DIR` env is set (non-empty), return `filepath.Clean(os.Getenv("HOME_DIR"))`; else return `filepath.Join(os.UserHomeDir(), ".buildmax")`. No side effects; does not create the dir. |

**Test helpers / test cases**

- **TestDataDir_Default**: With `HOME_DIR` unset (e.g. `t.Setenv("HOME_DIR", "")` or ensure it is unset), call `DataDir()`. Assert result contains `os.UserHomeDir()` and has suffix `.buildmax` (or path ends with `.buildmax`).
- **TestDataDir_Override**: Set `HOME_DIR` via `t.Setenv("HOME_DIR", tmpDir)` where `tmpDir` is from `t.TempDir()`. Call `DataDir()`. Assert result equals `filepath.Clean(tmpDir)` (or equivalent cleaned path).

## How they work together

**Data/control flow**

1. **Runtime (future)**: Any component (TUI, agent, persistence) that needs the data dir will call `config.DataDir()` and use the returned path (e.g. to create subdirs or read config). No such callers in this task.
2. **Tests**: Unit tests in `config_test.go` set or unset `HOME_DIR` and assert on `DataDir()` return value. No other packages call `DataDir()` in this task.
3. **Makefile**: When the user runs `make test`, the Makefile (1) ensures `testing-sandbox` exists, (2) sets `HOME_DIR` to the absolute path of `./testing-sandbox`, (3) runs `go test ./...`. All tests (including `config_test.go` and any future tests that rely on `HOME_DIR`) then see the sandbox as the data dir.

**Dependencies**

- `internal/config` uses only stdlib: `os`, `path/filepath`. No new package dependencies.
- No other internal package depends on `config` for data dir in this task (they will in follow-ups).

**Key data structures**

- None. Only a string path is returned by `DataDir()`.

**Makefile behavior**

- **test** target:
  - Create `testing-sandbox` if it does not exist (e.g. `mkdir -p testing-sandbox` on Unix; on Windows with GNU make, same or equivalent).
  - Export `HOME_DIR` as the absolute path of `./testing-sandbox`. Use a variable for the repo root (e.g. `CURDIR` in GNU make) and set `HOME_DIR := $(CURDIR)/testing-sandbox` (or Windows-safe equivalent).
  - Run `go test ./...`. Ensure `HOME_DIR` is exported so the Go test process sees it (e.g. `HOME_DIR=... go test ./...` on Unix; on Windows with make, `set HOME_DIR=... && go test ./...` or export then run).

## Changes for review

- **New**: `internal/config` — function **`DataDir() string`** in `config.go`; resolution logic as above (HOME_DIR cleaned, else UserHomeDir/.buildmax).
- **New**: `internal/config/config_test.go` — tests **TestDataDir_Default** and **TestDataDir_Override** using `t.Setenv` / `t.TempDir()` where appropriate; no leftover env for other tests.
- **New**: Repository root **`Makefile`** — target **`test`**: create `testing-sandbox`, set `HOME_DIR` to its absolute path, run `go test ./...`.
- **Modified**: **`.gitignore`** — add one line: `testing-sandbox/`.
- **Unchanged**: `internal/config/config.go` — existing types and `LoadLLM()` remain as-is; only `DataDir()` and `path/filepath` import added.
