# Configuration Reference

> **Audience:** users and operators · **Status:** current

BuildMax is configured by **YAML files inside the data directory**, not by a
long list of environment variables. Only a handful of bootstrap values stay in
the environment, because they must be known before any file can be read.

| File | Read by | Purpose |
|---|---|---|
| `<BUILDMAX_HOME>/settings.yaml` | CLI, Desktop | Models, hooks, sandbox, log level |
| `<BUILDMAX_HOME>/server.yaml` | Server, Worker | Port, auth, database, storage, worker, Tier 1 model |
| `<BUILDMAX_HOME>/policy.yaml` | CLI, Desktop, Worker | Operator sandbox policy that overrides `settings.yaml` |
| `<workspace>/.buildmax/hooks.yaml` | CLI, Desktop | Per-workspace hook overlay, additive to global hooks |
| `<BUILDMAX_HOME>/mcp.json` | CLI, Desktop, Worker | MCP servers, merged with the workspace file |
| `<workspace>/.buildmax/mcp.json` | CLI, Desktop | Per-workspace MCP servers; wins on a duplicate server id |

`BUILDMAX_HOME` defaults to `~/.buildmax`. Copy the starting points from
[`config-examples/`](../../config-examples/):

```bash
mkdir -p ~/.buildmax
cp config-examples/settings.example.yaml ~/.buildmax/settings.yaml
cp config-examples/server.example.yaml   ~/.buildmax/server.yaml   # server/worker only
cp config-examples/policy.example.yaml   ~/.buildmax/policy.yaml   # operator policy, optional
cp config-examples/mcp.example.json      ~/.buildmax/mcp.json      # MCP servers, optional
```

`mcp.example.json` carries a `_comment` key holding its own documentation; drop
that key before use.

## Environment Variables

This is the complete list. `internal/config/env_spec.go` is the source of truth;
anything not listed here is not read by BuildMax.

| Variable | Default | Purpose |
|---|---|---|
| `BUILDMAX_HOME` | `~/.buildmax` | Data directory; locates `settings.yaml` and `server.yaml`. Must be an env var — nothing else can be found until it is known. |
| `BUILDMAX_JWT_SECRET` | — | Overrides `jwt_secret` in `server.yaml`. Inject this at deploy time rather than committing the secret to a file. |
| `BUILDMAX_DEV_LOGIN_OTP` | — | Overrides `dev_login_otp`. **Development only** — see [deploy/authentication.md](../deploy/authentication.md). |
| `BUILDMAX_SANDBOX_ENABLED` | — | Overrides `sandbox.enabled`. Accepts `1/true/yes/on` or `0/false/no/off`. |
| `BUILDMAX_TRACE_DISABLED` | — | Disables durable run traces when truthy. Traces are on by default. |
| `BUILDMAX_TEST_DSN` | — | MySQL DSN for store integration tests. Unset skips those tests. |

### Credential Overrides

Every field below overrides the matching `server.yaml` entry. They exist so a
deployment can inject credentials from a Kubernetes Secret, a Docker secret, or
a CI variable instead of writing them to disk. An unset variable leaves the file
value alone.

| Variable | Overrides |
|---|---|
| `BUILDMAX_DATABASE_PASSWORD` | `database.password` |
| `BUILDMAX_STORAGE_MINIO_ACCESS_KEY` | `storage.minio.access_key` |
| `BUILDMAX_STORAGE_MINIO_SECRET_KEY` | `storage.minio.secret_key` |
| `BUILDMAX_WORKER_TOKEN` | `worker.token` |
| `BUILDMAX_CONVERSATION_MODEL_API_KEY` | `conversation.model.api_key` |

The split to aim for: **`server.yaml` carries shape and non-secret values; the
environment carries credentials.** That is exactly how
`deployment/buildmax-deploy.yaml` is arranged — a ConfigMap for the file, a
Secret for these variables.

### Local development `.env`

`./make` and `make.bat` load a `.env` file from the repository root before
running anything, so a local `BUILDMAX_*` value applies to every task without
exporting it in your shell. This is a **development convenience only** — a
released binary never reads `.env`; it reads the environment it is given.

The file is gitignored. There is deliberately no `.env.example`: the supported
configuration surface is `settings.yaml` and `server.yaml`, and a second
committed template would invite the two to disagree. Put in `.env` only what
genuinely belongs to your machine:

```bash
# Point the local server and worker at a scratch data directory.
BUILDMAX_HOME=./testing-sandbox

# Run the MySQL-backed store tests instead of skipping them.
BUILDMAX_TEST_DSN=root:pass@tcp(127.0.0.1:3306)/buildmax_test?parseTime=true

# Anything from the tables above, for `./make run server`.
BUILDMAX_JWT_SECRET=dev-only-secret
BUILDMAX_DATABASE_PASSWORD=...
```

Two variables are read by the task runner itself rather than by BuildMax:

| Variable | Default | Purpose |
|---|---|---|
| `BUILDMAX_KIND_CLUSTER` | `buildmaxdev` | Which kind cluster `./make kind …` creates and addresses. Every `kubectl` call uses that cluster's explicit context. |
| `BUILDMAX_IMAGE_PLATFORM` | host platform | Target platform for `./make kind images` — for example `linux/amd64` on Apple Silicon. |

The Compose stack is separate and does not use the root `.env`. It reads
`deployment/compose/.env`, which `deployment/compose/generate-env.sh` creates
with generated secrets and the host ports `BUILDMAX_SERVER_PORT` and
`BUILDMAX_PORTAL_PORT`; `./make compose up` generates it on first run. See
[deploy/compose.md](../deploy/compose.md).

## `settings.yaml` — CLI and Desktop

```yaml
log_level: info                      # debug | info | warn | error | off
server_url: http://localhost:5678    # default offered by `buildmax login`

models:                              # first entry is the default model
  - model: openai/gpt-3.5-turbo
    name: GPT-3.5-turbo
    api_url: https://openrouter.ai/api/v1
    api_key: your-api-key
    context_window: 16385            # 0 = built-in default (32000)
    call_timeout: 300                # seconds; 0 = default (300)

hooks: {}                            # see guide/hooks.md
sandbox: {}                          # see guide/sandbox.md
```

| Key | Default | Notes |
|---|---|---|
| `log_level` | `info` | Logs go to `<BUILDMAX_HOME>/logs/buildmax.log` only, never to the terminal, so the TUI stays clean. |
| `server_url` | — | Only used as the prompt default for `buildmax login`. |
| `models[]` | — | Any OpenAI-compatible endpoint. Select one per run with `--model <id or name>`. |
| `hooks` | empty | Lifecycle hooks. Reference: [guide/hooks.md](../guide/hooks.md). |
| `sandbox` | disabled | Bash sandboxing. Reference: [guide/sandbox.md](../guide/sandbox.md). |

## `server.yaml` — Server and Worker

```yaml
log_level: info
port: 5678
jwt_secret: ""                       # inject via BUILDMAX_JWT_SECRET in production
# allow_signup: true                 # default false; accounts are created with `buildmax-server user create`
# dev_login_otp: "123456"            # development only; see deploy/authentication.md
cors_origin: http://localhost:5173
workspaces_dir: /data/buildmax/workspaces
default_quota_tier: free_trial

conversation:                        # Tier 1 model used by the Portal agent loop
  model:
    model: openai/gpt-4o
    api_url: https://openrouter.ai/api/v1
    api_key: your-api-key
    context_window: 128000
  # model_target: fast               # use an llm.targets id for Tier 1 instead

# llm:                               # optional; see the table below
#   aliases:
#     default: fast
#   default_alias: default
#   targets:
#     - id: fast
#       name: Fast
#       provider: openai_compatible
#       model: openai/gpt-4o-mini
#       api_url: https://openrouter.ai/api/v1
#       api_key: your-api-key
#       context_window: 128000
#       call_timeout: 300
#       capabilities: [text_chat, tool_calls, streaming_text, usage_reporting]
#       disabled: false

database:                            # MySQL
  host: localhost
  port: 3306
  user: buildmax
  password: buildmax
  name: buildmax

webhook:
  message_path: message              # JSON path to the prompt in the request body
  user_id: webhook                   # fallback identity for webhook-created runs

worker:
  binary: buildmax-worker
  run_mode: local_process            # or k8s_job
  server_url: http://localhost:5678  # how the worker reaches the server
  token: your-worker-token           # shared secret for /api/worker/*
  k8s:
    namespace: buildmax
    image: buildmax:local
    config_map: buildmax-config      # ConfigMap holding server.yaml for worker pods
    home_dir: /buildmax              # BUILDMAX_HOME inside a worker pod

storage:
  persist_backend: local_fs          # or minio — team uploads
  artifact_backend: local_fs         # or minio — run outputs
  minio:
    endpoint: http://localhost:9000
    region: us-east-1
    access_key: minio
    secret_key: minio123
    bucket: bmstore
    prefix: workspaces
```

Required for a working server: `jwt_secret` (or `BUILDMAX_JWT_SECRET`),
`database`, and `worker.token` if you run workers. Everything else has a usable
default for local development.

`allow_signup` defaults to **false**, so nobody can register themselves. Create
accounts and issue login codes from the server instead — see
[deploy/authentication.md](../deploy/authentication.md):

```bash
buildmax-server user create alice@example.com
buildmax-server user login-code alice@example.com
```

The worker reads the same `server.yaml` and needs at minimum `worker.server_url`,
`worker.token`, `workspaces_dir`, and the `storage` block — it talks to blob
storage directly rather than proxying through the server.

### `llm` — model catalog

The `llm` block is the operator-owned model catalog for the managed LLM gateway
designed in [design/llm-gateway.md](../design/llm-gateway.md). It is optional and
**partly implemented**. What works today:

- `GET /api/teams/{team_id}/llm/models` lists the aliases a team may use.
- `POST /api/teams/{team_id}/llm/completions` runs one call, blocking or
  streamed, and records it in the call ledger. Abandoning a streamed call
  cancels the provider request.
- `conversation.model_target` picks the server's own Tier 1 model.

What does not exist yet: a managed client wired into CLI, Desktop, or the
worker, and quota enforcement beyond refusing a team that is *already* over its
limit. So those surfaces still call providers directly with their own
credentials, and the ledger is accounting data, not a spending ceiling.

A streaming reverse proxy in front of the server must have response buffering
off and an idle timeout longer than the target's `call_timeout`.

| Key | Meaning |
|---|---|
| `llm.targets[].id` | Catalog ID referenced by `llm.aliases` and `conversation.model_target`. `conversation` is reserved. |
| `llm.targets[].name` | Operator-facing display name. Defaults to the ID. |
| `llm.targets[].provider` | Client implementation. `openai_compatible` is the only one implemented, and the default. |
| `llm.targets[].model` | The provider's own model identifier. |
| `llm.targets[].api_url` | Upstream base URL. Required. |
| `llm.targets[].api_key` | Upstream credential. Required. |
| `llm.targets[].context_window` | Usable context size; `0` uses the client default. |
| `llm.targets[].call_timeout` | Per-call timeout in seconds; `0` uses the client default. |
| `llm.targets[].capabilities` | `text_chat`, `tool_calls`, `streaming_text`, `usage_reporting`. Omit to accept everything the provider's client contract guarantees. |
| `llm.targets[].disabled` | Retires a target without deleting it. |
| `llm.aliases` | Maps a team-facing alias to a target ID. **Leaving it empty means no team may use the gateway** — a catalog says which models exist, not who may call them. |
| `llm.default_alias` | Alias used when a caller names none. Required when more than one alias exists. |
| `conversation.model_target` | Runs Tier 1 on a catalog target instead of `conversation.model`. A catalog ID, not an alias: the server picks its own model rather than being granted one. |

A server with no `llm` block behaves exactly as before: `conversation.model`
serves Tier 1, and no team has managed access. A catalog that does not validate —
an alias pointing at no target, an unknown capability, a target with no
credential — fails startup rather than starting a server with no Tier 1 model.

Managed calls need a database, because every one of them is recorded in the
`llm_call` ledger. Without a store the routes answer `503` rather than serving
inference nobody can account for.

## Data Directory Layout

```text
<BUILDMAX_HOME>/
├── settings.yaml       CLI and Desktop configuration
├── server.yaml         Server and worker configuration
├── policy.yaml         Optional operator sandbox policy (overrides settings.yaml)
├── sessions/           Local session JSON files plus sessions.json index
├── traces/<session>/   Durable run traces, one JSONL file per run
└── logs/               Rotating buildmax.log
```

`./make test` sets `BUILDMAX_HOME=./testing-sandbox`, so tests never touch a real
data directory.

## Precedence

For any value that appears in more than one place:

```text
environment variable  >  policy.yaml  >  settings.yaml / server.yaml  >  built-in default
```

The sandbox block is the only one with a workspace-level layer; hooks are the
only block that merges additively (global hooks and workspace hooks both run)
rather than overriding.
