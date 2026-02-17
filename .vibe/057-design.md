# Design 057: Decouple workspace dir from buildmax home

## Goal

Make the workspace directory independent of BuildMax's data directory. Workspace dir is only used when running the server. There is no default; `BUILDMAX_WORKSPACES_DIR` is a required environment variable for server mode. When unset, `WorkspacesDir()` returns empty string; server startup must validate and fail with a clear error.

## Modules and structure

| Package / file | Responsibility |
|----------------|----------------|
| `internal/config/config.go` | `WorkspacesDir()`: return `""` when env unset, else `filepath.Clean(dir)`. No reference to DataDir(). |
| `internal/config/env_spec.go` | EnvVar for BUILDMAX_WORKSPACES_DIR: description = required for server, no default. |
| `internal/cmd/server.go` | In `runServer`, after existing validations (port, LoadServerEnv) and before building storage/config: require non-empty `config.WorkspacesDir()`, return error if empty. |
| `internal/config/config_test.go` | Update WorkspacesDir tests: default `""`, override returns cleaned path; remove TestWorkspacesDir_WithBuildmaxHome. |
| `.env.example` | Comment for BUILDMAX_WORKSPACES_DIR: required for server mode, no default. |

No new packages or types. Existing callers of `WorkspacesDir()`, `PersistentWorkspaceDir`, `RuntimeWorkspaceDir`, `ArtifactDir` are unchanged in signature; only the contract of `WorkspacesDir()` changes (empty when unset). Server is the only code path that must have a non-empty value, and we enforce that at startup.

## Method / function design

### config.WorkspacesDir()

- **Signature**: `func WorkspacesDir() string`
- **Behaviour**:
  - If `os.Getenv(EnvKeyBuildmaxWorkspacesDir)` is non-empty (after trimming if desired; current code does not trim): return `filepath.Clean(dir)`.
  - Otherwise return `""`.
- **Comment**: Replace current doc with: "WorkspacesDir returns the parent directory of all workspace roots. It is only used in server mode. If BUILDMAX_WORKSPACES_DIR is set, returns that path (cleaned). If unset, returns empty string (no default). Server startup must check and fail if empty."

No change to `PersistentWorkspaceDir`, `RuntimeWorkspaceDir`, or `ArtifactDir` implementations: they continue to call `WorkspacesDir()` and join paths. When `WorkspacesDir()` is `""`, those functions return paths like `ws-id/persist` (relative); that is only reachable from code paths that do not validate (e.g. tests that don't start the server). Server will never see empty because we fail before building config.

### internal/cmd/runServer

- **Insert** after `LoadServerEnv()` (or after port resolution) and before building entity/storage:
  1. `workspacesDir := config.WorkspacesDir()`
  2. If `workspacesDir == ""`, return `fmt.Errorf("%s is required for server mode", config.EnvKeyBuildmaxWorkspacesDir)` (or equivalent message naming the env var).
- **Order**: Validate workspace dir early so we fail fast before opening DB or building S3 clients. Logical place: immediately after `LoadServerEnv()` so all "required for server" env checks are together.

### config tests

- **TestWorkspacesDir_Default**: Clear `EnvKeyBuildmaxWorkspacesDir` (and optionally `EnvKeyBuildmaxHome` to avoid confusion). Assert `WorkspacesDir() == ""`.
- **TestWorkspacesDir_Override**: Keep as is: set env to a temp dir, assert `WorkspacesDir()` equals `filepath.Clean(tmp)`.
- **TestWorkspacesDir_WithBuildmaxHome**: Remove. This test asserted that when BUILDMAX_WORKSPACES_DIR is unset, result is DataDir()/workspaces; that default no longer exists.
- **TestPersistentWorkspaceDir, TestRuntimeWorkspaceDir, TestArtifactDir**: No change. They already set `EnvKeyBuildmaxWorkspacesDir` to a temp dir; they continue to pass.

### env_spec.go

- **EnvVars entry** for `EnvKeyBuildmaxWorkspacesDir`: set `Default` to `""` (already is). Set `Description` to something like: "Parent of workspace roots (required for server mode; no default)".

### .env.example

- **Line(s) for BUILDMAX_WORKSPACES_DIR**: Change comment from "Parent directory of all workspace roots; default DataDir()/workspaces." to state that it is required for server mode and has no default, e.g. "Parent of workspace roots (required for server mode; no default)."

## How they work together

1. **CLI/TUI**: Do not call workspace dir for normal operation; no change. If any code path called `WorkspacesDir()` and relied on a non-empty default, it would now get `""`; the only such path is server startup, which we fix by validating.
2. **Server startup** (`runServer`): After loading server env (JWT, etc.), check `config.WorkspacesDir()`. If empty, return error and exit. Otherwise pass the value into `server.Config.WorkspacesDir`; executor and server paths use it as today.
3. **Executor and server handlers**: Unchanged; they receive non-empty `WorkspacesDir` from config because server never starts with empty.
4. **Tests**: Config tests set the env var when they need a non-empty workspace dir; default test asserts `""`.

## Changes for review

| Change | File | Detail |
|--------|------|--------|
| Edit | `internal/config/config.go` | `WorkspacesDir()`: when env unset return `""`; remove `return filepath.Join(DataDir(), "workspaces")`. Update doc comment. |
| Edit | `internal/config/env_spec.go` | EnvVars description for BUILDMAX_WORKSPACES_DIR: "required for server mode; no default". |
| Edit | `internal/cmd/server.go` | In `runServer`, after `LoadServerEnv()`, add check: if `config.WorkspacesDir() == ""` return error with env key name. |
| Edit | `internal/config/config_test.go` | TestWorkspacesDir_Default: assert `WorkspacesDir() == ""`. Remove TestWorkspacesDir_WithBuildmaxHome. TestWorkspacesDir_Override unchanged. |
| Edit | `.env.example` | Comment for BUILDMAX_WORKSPACES_DIR: required for server mode, no default. |
