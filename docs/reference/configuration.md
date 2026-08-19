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
| `BUILDMAX_SANDBOX_ENABLED` | — | Overrides `sandbox.enabled`. Accepts `1/true/yes/on` or `0/false/no/off`. |
| `BUILDMAX_TRACE_DISABLED` | — | Disables durable run traces when truthy. Traces are on by default. |
| `BUILDMAX_RUN_TOKEN` | — | One task run's credential for every `/api/worker/*` route. Minted per run by the scheduler and placed in the worker process or Job pod — not something an operator sets. |
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

### What A Worker Receives

A task-run worker is given only the variables it reads, whether it runs as a
local process or a Kubernetes Job:

| Variable | Why a worker needs it |
|---|---|
| `BUILDMAX_HOME` | Run-scoped data directory |
| `BUILDMAX_WORKER_TOKEN` | Deprecated fallback for its `/api/worker/*` calls; a run uses `BUILDMAX_RUN_TOKEN` |
| `BUILDMAX_STORAGE_MINIO_ACCESS_KEY` / `_SECRET_KEY` | Reads and writes run state and artifacts |
| `BUILDMAX_CONVERSATION_MODEL_API_KEY` | Calls a provider directly — **withheld** when `worker.llm.transport` is `buildmax` |
| `BUILDMAX_SANDBOX_ENABLED`, `BUILDMAX_TRACE_DISABLED` | Runtime toggles |

`BUILDMAX_RUN_TOKEN` reaches a worker by a different route. It is not inherited
from the server — the filter above strips it, so a stale value cannot be picked
up — and is added to the process or pod at dispatch, naming the one run it
authorizes. It is what a worker presents on every `/api/worker/*` route, so a run
can only read and write its own record. `worker.token` is still accepted there
for one release; see
[design/worker-run-token.md](../design/worker-run-token.md).

A worker clears `BUILDMAX_RUN_TOKEN` and `BUILDMAX_WORKER_TOKEN` from its own
environment once it has read them, keeping both in memory only. The sandbox
would strip secret-shaped variables from a child process, but it is off by
default, so a model-chosen `printenv` would otherwise print them.

`BUILDMAX_JWT_SECRET` and `BUILDMAX_DATABASE_PASSWORD` are deliberately
withheld. A worker never reads them — it reaches the server over HTTP with its
run token and never touches the database — and it executes model-chosen
shell commands, so holding the signing secret would let one mint a token for
any user, and holding the database password would give it every team's data.
An unrecognized `BUILDMAX_` variable is withheld too, so a variable added to
the server without a decision about workers stays on the server.

`WorkerNeeds` in `internal/config/env_spec.go` is the source of truth. Marking a
variable there is what sends it to workers.

### How A Worker Pod Is Confined

Every worker Job pod is created non-root, with no service-account token, no
added capabilities, `RuntimeDefault` seccomp, and a read-only root filesystem
plus a writable `/tmp`. None of that is configurable: a worker executes
model-chosen shell commands, so it is treated as running untrusted code even
when the team that submitted the task is trusted — the prompt, the repository
content, and the tool output steering those commands are not.

Two settings under `worker.k8s` remain an operator's:

| Setting | Default | Purpose |
|---|---|---|
| `run_as_user` | `65532` | The uid the pod runs as. Set it on a cluster that assigns its own uid range, OpenShift most commonly. The image needs no matching user — the worker writes only into mounted volumes, and `fsGroup` makes them writable. |
| `resources.cpu_request` / `cpu_limit` / `memory_request` / `memory_limit` | unset | Kubernetes quantity strings. An empty value leaves that request or limit unset, so a deployment that has not chosen numbers keeps running unbounded rather than inheriting a limit nobody picked. An unparseable value is logged and skipped rather than failing the run. |

This applies to `run_mode: k8s_job`. `local_process` runs the worker beside the
server with no such boundary; it is a development path, not a deployment
topology.

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

  # - model: default                 # a team alias, not a provider model id
  #   name: Team Default
  #   transport: buildmax
  #   server_url: https://buildmax.example.com
  #   team_id: tm_example

hooks: {}                            # see guide/hooks.md
sandbox: {}                          # see guide/sandbox.md
```

| Key | Default | Notes |
|---|---|---|
| `log_level` | `info` | Logs go to `<BUILDMAX_HOME>/logs/buildmax.log` only, never to the terminal, so the TUI stays clean. |
| `server_url` | — | Only used as the prompt default for `buildmax login`. |
| `models[]` | — | Any OpenAI-compatible endpoint. Select one per run with `--model <id or name>`. |
| `models[].transport` | `direct` | `direct` calls a provider from this machine. `buildmax` calls a server's managed gateway. |
| `models[].server_url` | — | Required for `buildmax` transport: which deployment serves the call. |
| `models[].team_id` | — | Required for `buildmax` transport: which team the call is billed and authorized against. |

### Managed models

A `transport: buildmax` entry calls a BuildMax deployment instead of a provider,
so the server holds the provider credential and decides which models the team
may use. Its `model` field is a **team alias**, not a provider model id. List a
team's aliases with:

```bash
buildmax models --team <team_id>
```

`buildmax models` also prints where every configured model sends prompts, and
`buildmax doctor` reports a managed entry that is missing `server_url` or
`team_id`, or whose login is absent, expired, or for a different server.

The credential is never written into `settings.yaml`. It comes from
`buildmax login` and is only used when the stored login belongs to that entry's
`server_url` — a mismatch fails rather than sending the token to whatever host
the file names.

Model selection is first-match by file order: `--model` and `/model` match a
name or a model id against `models[]` top to bottom, and the first entry is the
default. Two entries sharing a name are therefore not interchangeable — the
lower one is unreachable — which is why `buildmax models` and the model pickers
print each entry's destination.

Three limits worth knowing before you rely on this:

- **There is no fallback between the modes.** A managed entry does not quietly
  become a direct call when the server is down, because that would redirect
  governed traffic to a personal provider key. Configure both entries and pick.
- **The login renews itself, until it does not.** The access token is refreshed
  automatically before each managed call that would otherwise use an expired
  one, so a long-lived session keeps working without another login code. When
  the refresh token itself expires or its session is revoked, calls fail with a
  clear error and you re-run `buildmax login`. See
  [design/llm-gateway.md](../design/llm-gateway.md) section 11.
- **Workers and the evaluation harness never use managed mode.** A worker runs
  the server's own model with the server's own credential, and `eval/` stays
  direct so benchmark results do not move with team model policy or quota.

Prompts, tool schemas, and tool results pass through the server for a managed
call. That is the point of the mode, and it is a real change in where your data
goes — which is why the model picker and `buildmax models` always name the
destination.
| `hooks` | empty | Lifecycle hooks. Reference: [guide/hooks.md](../guide/hooks.md). |
| `sandbox` | disabled | Bash sandboxing. Reference: [guide/sandbox.md](../guide/sandbox.md). |

## `server.yaml` — Server and Worker

```yaml
log_level: info
port: 5678
jwt_secret: ""                       # inject via BUILDMAX_JWT_SECRET in production
# allow_signup: true                 # default false; accounts are created with `buildmax-server user create`
access_token_ttl: 168h               # signed, unstored — this is how long a leaked one works
refresh_token_ttl: 720h              # a stored row, so a session can be revoked before it expires
refresh_rotation_grace: 30s          # window for processes sharing one credentials file to refresh at once
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

# llm:                               # team model policy; catalog is in the DB
#   aliases:
#     default: lm_xxxxxxxxxxxxxxxxxxxx
#   default_alias: default

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

# audit:                             # governance trail retention
#   retention_days: 365              # default 0 — keep every event forever

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

The two token lifetimes are not interchangeable. An access token is signed and
never stored, so nothing can retire one early — `access_token_ttl` is the window
in which a leaked one still works. A refresh token is a database row, so
`refresh_token_ttl` is how long a session can be renewed, not how long it is
beyond reach. See [deploy/authentication.md](../deploy/authentication.md).

People sign in with an email address and a password. `allow_signup` defaults to
**false**, so nobody registers themselves; create accounts from the server and
hand over a login code, which the person redeems and then replaces with a
password of their own — see
[deploy/authentication.md](../deploy/authentication.md):

```bash
buildmax-server user create alice@example.com
buildmax-server user login-code alice@example.com
```

The same code is how someone who forgot their password gets back in. Login
attempts are not rate limited; see the warning in that document before exposing
a server to an untrusted network.

The worker reads the same `server.yaml` and needs at minimum `worker.server_url`,
`worker.token`, `workspaces_dir`, and the `storage` block — it talks to blob
storage directly rather than proxying through the server.

`audit.retention_days` expires events in the governance trail. It defaults to
**0**, which keeps everything: a deployment that has not chosen a retention
policy has not decided to discard evidence. Setting it starts an hourly sweep
that removes events older than the window, and each sweep that removed anything
writes an `audit.pruned` event naming the range and the count — so a trail that
begins partway through says that policy shortened it rather than leaving a
reader to wonder. Nothing else in BuildMax deletes an audit event, and there is
no way to delete a particular one.

A team owner can download their space's trail from space settings, and a System
Administrator can download the deployment-wide one, filtered, from `#/admin`.
Both come as CSV or JSONL, and both are recorded in the trail as
`audit.exported` — reading the whole record is itself an action on it.

### Pointing At Dependencies You Already Run

A private deployment usually has a database and an object store already. Three
settings decide whether BuildMax reaches them the way that environment expects.

**`database.tls`** is the go-sql-driver TLS mode. Unset means `preferred`: TLS
whenever the server offers it, without verifying the certificate. That upgrades
an in-cluster connection for free and behaves exactly as a plaintext connection
against a server with no TLS at all.

Point at a managed database — RDS, Aurora, Cloud SQL — and set it to `true`,
which requires TLS and verifies the certificate against the system roots.
`skip-verify` requires TLS and accepts any certificate; `false` never uses it.

**`storage.minio.endpoint`** decides which kind of store BuildMax is talking to.
Set it for something you run or a vendor's S3-compatible service. Leave it
empty for AWS S3, so the SDK resolves the regional endpoint itself.

That also decides bucket addressing, because the two cases need opposite
answers: a compatible store needs bucket-in-path, and AWS S3 has not supported
that form for buckets created since 2020. `storage.minio.path_style` overrides
the derivation, which is only needed for a compatible store that uses
virtual-host addressing.

**`storage.minio.access_key` / `secret_key`** may both be left empty. The client
then falls through to the AWS SDK's default credential chain, which is how a
pod reaches a bucket through IRSA, workload identity, or an instance profile —
no long-lived key for the deployment to store, ship to workers, or rotate. Set
them for a store that has no such mechanism, such as MinIO.

### Managed models — the `llm_model` table and `llm` policy

The managed LLM gateway designed in
[design/llm-gateway.md](../design/llm-gateway.md) has two halves, kept apart on
purpose:

- **The catalog** is the `llm_model` database table: which models exist, where
  each is reached, and with what credential. It is not in `server.yaml` because
  it holds provider keys and changes while the server runs.
- **The policy** is the `llm` block below: which of those models each team may
  name, and under what alias.

Edit the catalog with `buildmax-server model`, on the machine that already holds
the database credentials:

```bash
buildmax-server model add --name Fast \
    --api-url https://openrouter.ai/api/v1 \
    --api-key your-openrouter-api-key \
    --model openai/gpt-4o-mini --context-window 128000
buildmax-server model list
buildmax-server model disable --id lm_xxxxxxxxxxxxxxxxxxxx
```

| Key | Meaning |
|---|---|
| `llm.aliases` | Maps a team-facing alias to an `llm_model` ID. **Leaving it empty means no team may use the gateway** — a catalog says which models exist, not who may call them. |
| `llm.default_alias` | Alias used when a caller names none. Required when more than one alias exists. |
| `conversation.model_target` | Runs Tier 1 on a catalog model instead of `conversation.model`. An `llm_model` ID, not an alias: the server picks its own model rather than being granted one. |

A server with no `llm` block behaves exactly as before: `conversation.model`
serves Tier 1, and no team has managed access. `conversation.model` also stays
the bootstrap path — a fresh deployment answers conversations before its catalog
has a single row.

An alias naming a model that does not exist does **not** stop the server. The
catalog is edited independently of the policy, so such an alias fails its own
calls and is skipped in model listings while every other alias keeps working.

Managed calls need a database for two reasons: the catalog lives there, and
every call is recorded in the `llm_call` ledger. Without a store the routes
answer `503` rather than serving inference nobody can account for.

Credentials are stored in the `llm_model` table and read by exactly one query,
the one that builds a provider client. They are never returned by a model
listing, an API response, or an error. Note what that implies for operations:
database backups and read replicas carry provider keys, so treat them the way
you treat the database password.

### Managed models for task runs — the `worker.llm` block

Task runs default to calling a provider themselves. Point them at the gateway
instead, and the worker stops needing an upstream key:

| Key | Meaning |
|---|---|
| `worker.llm.transport` | `direct` (default) or `buildmax`. Under `buildmax`, `BUILDMAX_CONVERSATION_MODEL_API_KEY` is withheld from the worker. |
| `worker.llm.alias` | Which alias a run calls. Empty uses `llm.default_alias`. Must be one of `llm.aliases`. |
| `worker.llm.context_window`, `worker.llm.call_timeout` | Describe the alias to the run; the protocol does not report them per call. |
| `worker.run_token_ttl` | How long a run's credential stays valid. Defaults to 24h. Every run gets one, managed or not. |
| `worker.run_timeout` | How long a run may stay `SCHEDULED` or `RUNNING` before the server records it as abandoned. Defaults to 6h. |

The server states the transport and alias; a worker never chooses its own model,
and is told nothing else about it — endpoint, upstream identifier, and
credential all stay server-side. Each run is dispatched with its own credential
in `BUILDMAX_RUN_TOKEN`, which authorizes that run and nothing else.

`./make compose smoke managed` runs the whole path against a mock upstream and
needs no provider key.

Two things to know before enabling it:

- `transport: buildmax` with an empty `llm.aliases`, or an alias no team may
  call, **stops the server at startup**. That configuration parses cleanly and
  would otherwise fail every run at its first model call.
- The run token is not renewable. `run_token_ttl` must outlast your longest
  run; a run that outlives it loses its remaining model calls.

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
