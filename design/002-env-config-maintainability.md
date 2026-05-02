# Env & Config Maintainability

## Status

- **Implemented:** 1 (single source of truth), 2 (centralize loading per domain).
- **Future:** 3 (optional config file), 4 (trim .env.example by audience), 5 (document “who needs what”).

## Problem

Many environment variables are used across `internal/config`, `internal/interface/cli`, `internal/infra/log`, and `internal/workspacestorage`. Keeping `.env` and `.env.example` in sync with code is error-prone.

## Recommendations

### 1. Single source of truth in code ✅

- **Define all env vars in one place** (`internal/config/env_spec.go`): `EnvVars` slice with name, default, and description.
- Use that list only for **documentation and consistency**: developers (and optional tooling) know where to look; `.env.example` can be updated manually from it or generated later.
- No new dependency; just a single slice or table that every loader respects when adding a new variable.

### 2. Centralize loading per domain ✅

- **All env reads live in the config package** (or `internal/config/workspace_storage.go`):
  - LLM: `LoadLLM()`
  - Server: `LoadServerEnv()` (JWT, CORS), `ResolveServerPort(portFromFlag)` (port from flag or BUILDMAX_SERVER_PORT)
  - MySQL: `MySQLDSN()`
  - Workspace storage: `LoadWorkspaceStorageConfig()`
  - Log level: `LogLevel()` (used by `internal/infra/log.Init()`)
  - App paths: `DataDir()`, `WorkspacesDir()`, etc.
- `cmd/server` and `log.Init()` call config only; no `os.Getenv` in cmd or log for these vars.

### 3. Optional config file (future)

- **Layer**: defaults in code → optional file (e.g. `~/.buildmax/config.yaml` or `config.yaml` in repo) → env overrides.
- Env vars remain the override mechanism (good for CI and secrets). Most non-secret options can live in the file to reduce env clutter.
- Implement when needed (e.g. with Viper or a small YAML loader) so that the same structs from 2 are filled from file first, then env.

### 4. Trim .env.example by audience (future)

- **Minimal “quick start”**: only vars that are required or commonly changed (e.g. `BUILDMAX_API_KEY`, `BUILDMAX_JWT_SECRET` for server, optional `BUILDMAX_HOME`).
- **Full reference**: either the same file with clear sections (as now) or a second file (e.g. `.env.example.full`) / a doc section that lists every variable with default and description, derived from the source of truth in 1.

### 5. Document “who needs what” (future)

- In README or AGENTS.md: short table “CLI/TUI” vs “Server” listing which env vars each mode needs. Reduces confusion and keeps .env smaller for developers who only run the CLI.

## Summary

| Action | Effort | Impact |
|--------|--------|--------|
| Single source of truth (env list in code) | Low | No drift; one place to add new vars |
| Centralize server env into `config` | Low | Fewer `os.Getenv` in cmd; clearer contract |
| Optional config file | Medium | Fewer env vars for most users |
| .env.example by audience / generated | Low–Medium | Easier onboarding and reference |

Implementing **1** and **2** gives the biggest maintainability gain with minimal change.
