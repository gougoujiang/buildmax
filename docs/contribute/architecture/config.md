# Configuration

> **Audience:** contributors · **Status:** current
>
> User-facing field reference:
> [reference/configuration.md](../../reference/configuration.md)

## Purpose

`internal/config` loads configuration and resolves paths. It reads YAML files
via Viper plus a small set of bootstrap environment variables, and it does
**not** import infrastructure implementations — startup wiring belongs in
`internal/bootstrap`.

## The Two Files

| Type | File | Loader | Used by |
|---|---|---|---|
| `Settings` | `<BUILDMAX_HOME>/settings.yaml` | `LoadSettings()` | CLI, Desktop |
| `ServerConfig` | `<BUILDMAX_HOME>/server.yaml` | `LoadServerConfig()` | Server, Worker |

`Settings` carries `log_level`, `server_url`, `models[]`, `hooks`, and
`sandbox`. `ServerConfig` carries the port, `jwt_secret`, `dev_login_otp`,
`cors_origin`, `workspaces_dir`, `default_quota_tier`, and the nested
`conversation`, `database`, `webhook`, `worker`, and `storage` blocks. Both use
`mapstructure` tags in `snake_case` to match the on-disk form.

## Models

`ModelEntry` is one LLM entry: `model`, `name`, `api_url`, `api_key`,
`context_window`, `call_timeout`. The **first entry in `models[]` is the
default**; `--model` selects by id or name.

Zero values fall back to constants in the package — `DefaultContextWindow`,
`DefaultCallTimeoutSecs`, `DefaultOpenRouterBaseURL`, `DefaultModel`.

With no models configured, the loader returns
`no models configured; add models to settings.yaml`, and the CLI prints the
resolved `SettingsPath()` so the user knows which file to create. There is no
implicit API-key environment variable.

## Environment Variables

`env_spec.go` is the single source of truth: the `EnvVars` slice lists every
variable BuildMax reads, and a test keeps it consistent. Only bootstrap-level
values live here — anything that can be in a file is in a file.

Per-subsystem env constants sit next to the resolver that reads them
(`EnvKeyBuildmaxSandboxEnabled` in `sandbox.go`, `EnvKeyBuildmaxTraceDisabled`
in `trace.go`) and are registered into `EnvVars`.

## Paths

`DataDir()` returns `$BUILDMAX_HOME`, or `~/.buildmax`. Everything else derives
from it: `SettingsPath()`, `ServerConfigPath()`, `SessionsDir()`, `LogsDir()`,
`TracesDir()`, `PolicyPath()`. Path helpers **do not create directories** —
callers `os.MkdirAll` as needed.

Run-scoped paths for the server and worker — `WorkspacesDir()`,
`PersistentWorkspaceDir()`, `RuntimeWorkspaceDir()`, `ArtifactDir()` — resolve
against the configured `workspaces_dir`.

## Precedence

```text
env var  >  policy.yaml  >  settings.yaml / server.yaml  >  built-in default
```

`policy.yaml` applies to the sandbox block only, and is how an operator
overrides a user's `settings.yaml`.

## Dependencies

- **Uses**: standard library plus `github.com/spf13/viper`
- **Used by**: `internal/bootstrap`, `internal/agentapp`, `internal/interface/cli`,
  `internal/infra/log`

## Notes

- `./make test` sets `BUILDMAX_HOME=./testing-sandbox`, so tests never touch a
  real data directory.
- See also: [LLM Client](llm-client.md), [CLI](cli.md), [Overview](overview.md).
