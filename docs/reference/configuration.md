# Configuration Reference

> **Audience:** users and operators · **Status:** current

BuildMax is configured by **YAML files inside the data directory**, not by a
long list of environment variables. Only a handful of bootstrap values stay in
the environment, because they must be known before any file can be read.

| File | Read by | Purpose |
|---|---|---|
| `<BUILDMAX_HOME>/settings.yaml` | CLI, Desktop | Models, hooks, sandbox, log level |
| `<BUILDMAX_HOME>/server.yaml` | Server, Worker | Port, auth, database, storage, worker, Tier 1 model |
| `<BUILDMAX_HOME>/policy.yaml` | CLI, Desktop, Worker | Operator policy: sandbox settings that override `settings.yaml`, and which plugin sources may load |
| `<workspace>/.buildmax/hooks.yaml` | CLI, Desktop | Per-workspace hook overlay, additive to global hooks |
| `<BUILDMAX_HOME>/mcp.json` | CLI, Desktop, Worker | MCP servers, merged with the workspace file |
| `<workspace>/.buildmax/mcp.json` | CLI, Desktop | Per-workspace MCP servers; wins on a duplicate server id |
| `<BUILDMAX_HOME>/plugins/<name>/` | CLI, Desktop, Worker | An installed local plugin, or an exact Team-activated release materialized into a run-scoped worker home; see [guide/plugins.md](../guide/plugins.md) |
| `<workspaces_dir>/.marketplace/` | Server | Published plugin packages, when the deployment has no object store |

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
| `BUILDMAX_SERVER_URL` | — | Address this process uses to reach `buildmax-server`. Overrides `settings.yaml` `server_url` for CLI/Desktop and `server.yaml` `worker.server_url` for workers. |
| `BUILDMAX_JWT_SECRET` | — | Overrides `jwt_secret` in `server.yaml`. Inject this at deploy time rather than committing the secret to a file. |
| `BUILDMAX_CORS_ORIGIN` | — | Overrides `cors_origin` in `server.yaml`. It has to name the origin the Portal is served from, which is a host port the deployment picks — the Compose stack derives it from `BUILDMAX_PORTAL_PORT`, so moving that port is one change rather than two. |
| `BUILDMAX_SANDBOX_ENABLED` | — | Overrides user `sandbox.enabled`, below per-run CLI and operator policy. Accepts `1/true/yes/on` or `0/false/no/off`. |
| `BUILDMAX_TRACE_DISABLED` | — | Disables durable run traces when truthy. Traces are on by default. |
| `BUILDMAX_CREDENTIAL_STORE` | — | Set to `file` to keep a CLI or Desktop login's access and refresh tokens in `auth.json` instead of the OS credential store (Keychain, Credential Manager, Secret Service). `buildmax login`, `buildmax whoami`, and `buildmax doctor` report which one a login actually used. |
| `BUILDMAX_RUN_TOKEN` | — | One task run's credential for every `/api/worker/*` route. Minted per run by the scheduler and placed in the worker process or Job pod — not something an operator sets. |
| `BUILDMAX_RUN_INTERRUPT_GRACE` | — | How long a worker asked to stop may spend reporting what its run produced. Set per dispatch by the scheduler from `shutdown_grace`, so the two windows nest — not something an operator sets. |
| `BUILDMAX_TEST_DSN` | — | MySQL DSN for store integration tests. Unset skips those tests. |
| `BUILDMAX_CACHE_QUALIFY_PROVIDER` | — | Provider for `./make cache-qualify`, which calls a real paid provider. Unset skips the suite. |
| `BUILDMAX_CACHE_QUALIFY_MODEL` | — | Model identifier for that suite. |
| `BUILDMAX_CACHE_QUALIFY_API_KEY` | — | Credential for that suite. |
| `BUILDMAX_CACHE_QUALIFY_BASE_URL` | — | Endpoint override for that suite. |
| `BUILDMAX_CACHE_QUALIFY_SLOW` | — | Include the qualification scenarios that wait out a retention window. Truthy values only; they take minutes of wall clock. |

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
| `BUILDMAX_SERVER_URL` | Reaches the server that owns the task run |
| `BUILDMAX_STORAGE_MINIO_ACCESS_KEY` / `_SECRET_KEY` | Reads and writes run state and artifacts |
| `BUILDMAX_CONVERSATION_MODEL_API_KEY` | Calls a provider directly — **withheld** when `worker.llm.transport` is `buildmax` |
| `BUILDMAX_SANDBOX_ENABLED`, `BUILDMAX_TRACE_DISABLED` | Runtime toggles |

`BUILDMAX_RUN_TOKEN` reaches a worker by a different route. It is not inherited
from the server — the filter above strips it, so a stale value cannot be picked
up — and is added to the process or pod at dispatch, naming the one run it
authorizes. It is what a worker presents on every `/api/worker/*` route, and the
only credential those routes accept, so a run can only read and write its own
record. A run dispatched without one fails at startup; see
[design/worker-run-token.md](../design/worker-run-token.md).

A worker clears `BUILDMAX_RUN_TOKEN` from its own environment once it has read
it, keeping the value in memory only. The sandbox would strip secret-shaped
variables from a child process, but it is off by default, so a model-chosen
`printenv` would otherwise print it.

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
| `resources.cpu_request` / `cpu_limit` / `memory_request` / `memory_limit` | none — required | Kubernetes quantity strings such as `500m`, `2`, `512Mi`, or `4Gi`. All four are required under `k8s_job`; BuildMax chooses no numbers for you, because the right ones depend on the work a deployment runs. |

The server refuses to start when a bound is missing, is not a Kubernetes
quantity, is zero or negative, or names a limit below its own request. The error
names the key to edit. This is deliberate: an unbounded worker pod runs
model-chosen shell commands, so one runaway build starves everything else on the
node, and a bound that was silently dropped for a typo looks exactly like a bound
that is in force.

This applies to `run_mode: k8s_job`. Under `local_process` the worker is a child
process of the server — same host, same uid, same filesystem — so the two are
one trust domain by construction. The `BUILDMAX_*` filtering above still
applies, and everything outside that prefix is inherited from the server
process, so a credential an operator happened to export reaches the worker as
well. Neither fact is worth fixing on its own: a worker that goes looking reads
the server's environment and `server.yaml` whatever it was handed. Single-machine
deployments are supported on those terms. A deployment that needs the server
separated from the code a model chooses runs `k8s_job`, which is where that
boundary is built; `local_process` is deliberately not being hardened towards
one.

### Local development `.env`

`./make` and `make.bat` load a `.env` file from the repository root before
running anything, so a local `BUILDMAX_*` value applies to every task without
exporting it in your shell. This is a **development convenience only** — a
released binary never reads `.env`; it reads the environment it is given.

The file is gitignored. A committed [`.env.example`](../../.env.example) lists
the optional personal credentials consumed by developer and operator tasks;
copy it when you need those tasks, then fill only the entries you use. It does
not duplicate the supported BuildMax configuration surface in `settings.yaml`
and `server.yaml`. Put in `.env` only what genuinely belongs to your machine:

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

The DigitalOcean qualification command reads these task-runner variables. Its
full lifecycle and credential scope are in
[deploy/digitalocean.md](../deploy/digitalocean.md):

| Variable | Default | Purpose |
|---|---|---|
| `DIGITALOCEAN_TOKEN` | — | Manages the disposable DOKS and MySQL resources and reads the persistent Project and VPC. |
| `SPACES_ACCESS_KEY_ID` | — | Reads the persistent Spaces bucket and later authenticates BuildMax to it. |
| `SPACES_SECRET_ACCESS_KEY` | — | Secret half of the bucket-scoped Spaces key. |
| `BUILDMAX_OCEAN_PROJECT` | `buildmax-beta` | Existing DigitalOcean Project to reuse. |
| `BUILDMAX_OCEAN_VPC` | `buildmax-beta` | Existing VPC to reuse. |
| `BUILDMAX_OCEAN_BUCKET` | `buildmax-beta` | Existing Spaces bucket to reuse. |
| `BUILDMAX_OCEAN_REGION` | `sgp1` | Shared region for the existing and disposable resources. |
| `BUILDMAX_OCEAN_DATABASE_VERSION` | `8.4` | Pinned DigitalOcean Managed MySQL version. |
| `BUILDMAX_OCEAN_STATE_DIR` | `~/.buildmax/qualification/ocean` | Owner-only directory outside Git holding state, plans, providers, and kubeconfig. |

The Compose stack is separate and does not use the root `.env`. It reads
`deployment/compose/.env`, which `deployment/compose/generate-env.sh` creates
with generated secrets and the host ports `BUILDMAX_SERVER_PORT` and
`BUILDMAX_PORTAL_PORT`; `./make compose up` generates it on first run. Those two
ports are self-contained — `compose.yaml` derives the Portal's API base and the
server's `BUILDMAX_CORS_ORIGIN` from them. See
[deploy/compose.md](../deploy/compose.md).

## `settings.yaml` — CLI and Desktop

```yaml
log_level: info                      # debug | info | warn | error | off
server_url: http://localhost:5678    # default offered by `buildmax login`;
                                      # BUILDMAX_SERVER_URL overrides it

models:                              # first entry is the default model
  - model: openai/gpt-3.5-turbo
    name: GPT-3.5-turbo
    api_url: https://openrouter.ai/api/v1
    api_key: your-api-key
    context_window: 16385            # 0 = built-in default (32000)
    call_timeout: 300                # seconds; 0 = default (300)

  # - model: claude-sonnet-4-5       # Anthropic's own endpoint, not a gateway
  #   name: Claude Sonnet 4.5
  #   provider: anthropic
  #   api_url: https://api.anthropic.com
  #   api_key: your-api-key
  #   context_window: 200000
  #   max_tokens: 8192               # 0 = built-in default (8192)

default_model: GPT-5.6 Luna          # which entry a new session starts with;
                                     # omit and the first one is used

hooks: {}                            # see guide/hooks.md
sandbox: {}                          # see guide/sandbox.md
```

| Key | Default | Notes |
|---|---|---|
| `log_level` | `info` | Logs go to `<BUILDMAX_HOME>/logs/buildmax.log` only, never to the terminal, so the TUI stays clean. |
| `server_url` | — | Only used as the prompt default for `buildmax login`; `BUILDMAX_SERVER_URL` overrides it. |
| `models[]` | — | One model the CLI can run while signed out. Select one per run with `--model <id or name>`. |
| `default_model` | first entry | Which entry a new session starts with, by name or model id. Applies while signed out; a deployment names its own default. |
| `models[].provider` | `openai_compatible` | The wire protocol the endpoint speaks — see below. |
| `models[].max_tokens` | `0` | Cap on one response. `0` means the protocol's default; `anthropic` requires the field, so `0` there sends the built-in 8192. |
| `models[].reasoning` | `off` | How much the model reasons before answering: `off`, `low`, `medium`, or `high` — see below. No effect on `openai_compatible`, which carries none. |
| `models[].cache_control` | `auto` | Which calls ask the provider to cache the stable prefix of a request, and for how long — see below. |
| `models[].pricing` | — | What this model charges, so a run can report its cost — see below. |
| `models[].integration` | — | A qualified OpenAI-compatible gateway. None is qualified, so any value is refused today. |
| `models[].vision` | `false` | This model accepts image input. Leave it off and an image a tool returns is described in text rather than sent. |
| `models[].keep_alive` | — | How long a local runtime keeps the model loaded after a call: a duration such as `30m`, `0` to unload at once, `-1` to stay resident. Only `ollama` reads it. |

### Model providers

`provider` names the **wire protocol** an endpoint speaks, not a vendor. Which
value to use follows from the endpoint's API, not from who made the model:
Claude served through OpenRouter is `openai_compatible`, and Claude served from
`api.anthropic.com` is `anthropic`.

| Value | API | Typical endpoint |
|---|---|---|
| `openai_compatible` | OpenAI Chat Completions | OpenRouter, LiteLLM, vLLM, LM Studio, and other compatible gateways. The default, and what every entry written before this option existed keeps using. |
| `openai` | OpenAI Responses | OpenAI's own `api.openai.com`. Runs stateless: BuildMax sends the whole conversation on each call and stores nothing server-side. |
| `anthropic` | Anthropic Messages | `api.anthropic.com` |
| `ollama` | Ollama's own `/api/chat` | A local Ollama daemon, by default `http://localhost:11434`. Needs no `api_key`. See [local models](#local-models-with-ollama). |

Text, tool calling, streaming, and token usage work the same on all four. So do
reasoning, prompt caching, and image input, each described below — what differs
is only how much a given protocol can do.

### Reasoning

`reasoning` sets how much the model reasons before answering, and carries that
reasoning forward, so a run spanning several tool calls keeps the thread instead
of starting over on each one. The levels are `off` (the default), `low`,
`medium`, and `high`.

| Provider | What a level other than `off` does |
|---|---|
| `openai_compatible` | Nothing. The protocol has no reasoning state. |
| `openai` | Sets the reasoning effort, requests encrypted reasoning content, and replays it on later turns. |
| `anthropic` | Enables adaptive extended thinking at that effort and replays the thinking blocks. |
| `ollama` | Turns thinking on for a model that supports it. This protocol's switch has no levels, so `low`, `medium`, and `high` all mean on, and it carries no state to replay. |

It is off by default, because it changes what a call costs and some older models
reject the request outright. Turning it on for a model that does not support it
fails the call with the provider's own error rather than silently doing nothing.
An unrecognized level is refused before any call is made.

The reasoning itself never reaches the transcript. BuildMax stores it as opaque
state alongside the assistant message and sends it back unread — a signature
covers the content, so editing it is worse than omitting it. State is tagged
with the protocol that produced it, so continuing a session under a different
provider drops what that provider cannot use and keeps everything else. Both the
CLI session file and Portal conversations persist it, and the managed gateway
carries it, so a run that resumes after a restart keeps its continuity.

### Prompt caching

`cache_control` asks the provider to cache the part of a request that does not
change between calls — the tool definitions and system prompt — so the rest of a
run pays a reduced rate for them.

```yaml
models:
  - name: sonnet
    provider: anthropic
    model: claude-sonnet-5
    cache_control:
      mode: auto             # auto (default), off, force
      ttl: provider_default  # provider_default (default), 5m, 1h
```

`mode` decides **which calls** ask:

| Mode | Agent turns | One-shot calls (title, compaction, probes) |
|---|---|---|
| `auto` (default) | Ask | Do not ask |
| `off` | Do not ask | Do not ask |
| `force` | Ask | Ask |

The split is what makes `auto` safe as a default. Writing a cache entry costs
more than not caching and only pays back if a later call reads it, so a run of
many calls over one stable prefix is better off and a single short call is worse
off. An agent turn's prefix goes out again on the next iteration; a generated
title's never does. `force` is for a caller that knows something the runtime
cannot see.

`ttl` selects retention, and only where the provider documents it. Anything
other than `provider_default` on a provider that does not document it is
refused at startup rather than sent and ignored.

| Provider | What a request carries | Retention |
|---|---|---|
| `anthropic` | Breakpoints after the tools and system prompt and at the end of the request. Nothing is cached unless the request says where. | `provider_default`, `5m`, `1h` |
| `openai` | A scoped `prompt_cache_key`. Responses caches on its own, so the key does not turn caching on — it says which bucket the prefix belongs in. | `provider_default`, `24h` |
| `openai_compatible` | Nothing. Speaking the protocol is not a promise to implement its cache fields, and an untested gateway may reject or ignore them. | `provider_default` only |
| `ollama` | Nothing. A local runtime reuses its own cache between calls, with no request-side control and no counts to report. | `provider_default` only |

Retention vocabulary is per provider, not global: `5m` and `1h` mean something
to Anthropic and nothing to the Responses API, and `24h` the other way round.
Asking for one where it is not documented is refused at startup rather than sent
and ignored.

The `prompt_cache_key` is derived, not configured. It is an opaque digest of the
credential, the model, the team (managed calls only), and fingerprints of the
system prompt and tool definitions — the things that all have to match for the
provider to hit. It carries none of them in readable form, changes when any of
them changes, and is never written to the ledger, a trace, a log, or the CLI.
Two teams granted the same model share a credential, and the team in the key is
what keeps their prompts out of one another's bucket.

`mode: force` is refused at startup on any provider that takes no cache
instructions. Serving it as no caching at all would answer a question nobody
asked. `mode: auto` is accepted everywhere, because most models are like this.

Cached tokens are reported as `cache_read_tokens` and `cache_write_tokens`,
which **break the prompt count down rather than adding to it**. A spend report
that summed all of them alongside `prompt_tokens` would count the same tokens
twice.

They are visible wherever a run's tokens are: the CLI prints a `Cache(read/write)`
line and shows the same figures in the TUI status bar, `--format json` carries
them under `usage`, the run trace records them, the session file keeps the
per-session totals, and a managed deployment records them on the `llm_call`
ledger row for Portal's run-spend view.

Each of those shows the breakdown only when a provider reported one. Most
providers report nothing at all, and a permanent `0 / 0` would read as a
measured miss rather than an absent measurement.

### Model pricing

`pricing` is what a model charges, so a run can say what it cost. Without it a
run reports its cost as `unavailable` rather than as zero — BuildMax does not
know what any provider charges, and a guess dressed as a number is worse than
silence.

```yaml
models:
  - name: sonnet
    provider: anthropic
    model: claude-sonnet-5
    pricing:
      currency: USD
      input_per_mtok: "3.00"
      cache_read_per_mtok: "0.30"
      cache_write_per_mtok: "3.75"
      output_per_mtok: "15.00"
```

Rates are decimal strings quoted per million tokens, written the way providers
publish them so a configured value can be checked against a price page without
arithmetic. The four are separate because caching prices them differently: a
cache read is cheaper than fresh input and a cache write is dearer, which is the
whole reason caching is a decision rather than a free win.

A price list must be complete enough to be trusted. Rates with no `currency`, or
a `currency` with no rate, are refused at load — an estimate assembled from half
a price list looks authoritative and is not. A rate of `"0"` is a real price and
is accepted.

Where the cost appears:

| Surface | What it shows |
|---|---|
| CLI | A `Cost(session)` line after a run, with what caching saved when it saved anything |
| CLI `--format json` | `usage.cost`, in nano-units of the currency |
| Session file | A running total, accumulated as the session ran |
| Run trace | Each call's own cost on `llm_end`, and the run's on `run_end` — which turn was expensive, not just the total |
| Portal run view | Per-run estimated cost and the saving against an uncached baseline |

The session total is accumulated turn by turn rather than recomputed on read,
because the model — and so the rates — can change mid-session. A total derived
later from whatever is configured then would restate turns already paid for at a
different price. When part of a session cannot be priced, or a second currency
appears, the total is labelled partial rather than quietly understating the run.

For a managed deployment the operator sets the same four rates per catalog
model, with `--currency`, `--input-price`, `--cache-read-price`,
`--cache-write-price`, and `--output-price` on `buildmax-server model add`. The
rates in force are copied onto each `llm_call` row when the call is accepted, so
repricing a model does not restate what a team already spent.

A saving is reported only when caching actually saved. A run that wrote cache
entries nothing read back paid more than it would have uncached, and that is
shown as the cost it was, not as a small win.

### Image input

`vision: true` says the model accepts images. It matters because an MCP server
can return one — a screenshot, a rendered chart — and what happens next depends
on whether the model can read it.

| `vision` | What the model receives |
|---|---|
| `false` (default) | A line of text saying what came back, such as `(image: image/png, 43.2 KB)`. The image is not sent. |
| `true` | The same text, plus the image itself. |

The default is off because a model without image support **rejects** a request
carrying one rather than ignoring it. Both branches send a usable tool result,
so turning it on is a capability statement, not a repair.

Where the image lands depends on the protocol: `anthropic` puts it inside the
tool result, while the OpenAI protocols and `ollama` cannot and send it as a
short user turn immediately after. Managed deployments declare this with the
`image_input` capability on a catalog model.

### Local models with Ollama

`provider: ollama` runs against a local [Ollama](https://ollama.com) daemon: no
key, no network, no bill. Write the entry with:

```bash
buildmax init --ollama          # configure a model the daemon already holds
buildmax models --local         # list what is installed, and what it can do
buildmax doctor                 # daemon up? model pulled? can it call tools?
```

The entry it writes carries no `api_key` line, because there is no credential
to hold:

```yaml
models:
  - model: qwen3:8b
    name: Qwen3 8B (local)
    provider: ollama
    api_url: http://localhost:11434   # the daemon root, not its /v1 endpoint
    context_window: 32000
```

**Use `ollama`, not `openai_compatible`, for a local daemon.** The same daemon
also serves an OpenAI-compatible endpoint at `/v1`, and that endpoint cannot set
the context window: the runtime then applies its own default and truncates a
longer prompt rather than refusing it. What it drops is the *front* of the
request — the system prompt and the tool definitions — so the model stops
calling tools and starts describing what it would do. `provider: ollama` sends
the window on every call, so the number BuildMax trims history against and the
number the daemon uses are the same one.

`context_window` is that number. Leave it unset and BuildMax asks the daemon
what the model was trained for and takes the smaller of that and the built-in
default, because a full-length window can be more than the machine can allocate.
Raising it is one edit; `buildmax doctor` prints the model's maximum next to
what is configured.

Two things a local model can be wrong about, both reported by `doctor` with the
command that fixes them: the model is not pulled (`ollama pull <model>`), or it
cannot call tools at all, which no amount of prompting works around. Pick one
whose capabilities include `tools` in `buildmax models --local`.

Everything else behaves as it does elsewhere: `max_tokens`, `vision`, and
`reasoning` mean the same, `cache_control` does nothing because a local runtime
takes no cache instructions, and `keep_alive`
controls how long the daemon keeps the model in memory between calls — worth
setting on a machine where reloading a large model costs more than the turn.

A deployment can serve a local model too: `--provider ollama` on a catalog
target, or `provider: ollama` under `conversation.model` in `server.yaml`, with
no credential in either case. See
[a local model in a deployment](#a-local-model-in-a-deployment).

### Managed models

Signing in switches this machine to a deployment's models, and signing out
switches back:

```bash
buildmax login        # models now come from that deployment
buildmax models       # what it offers, and that prompts go there
buildmax logout       # back to the models in settings.yaml
```

There is nothing to configure for it. A deployment holds the provider
credentials and its catalog is fetched on each start, so `settings.yaml`
describes only the models a signed-out session runs on. Every model a deployment
offers is available to every user of it — a team is a collaboration boundary,
not a model authorization boundary.

The credential is never written into `settings.yaml`. It comes from
`buildmax login`, and only the login for the server being called is used — a
mismatch fails rather than sending the token to whatever host was named.

Model selection within a mode is first-match by name or model id, and
`default_model` names which entry a new session starts with. In managed mode the
deployment names its own default.

Three things worth knowing before you rely on this:

- **The two modes never mix, and neither covers for the other.** A signed-in
  session sees only the deployment's models; a signed-out one sees only
  settings.yaml. A server that is down does not quietly become a local call,
  because that would redirect governed traffic to a personal provider key — the
  session refuses to start and says so. `buildmax logout` is the way to local
  models, and it is a decision rather than a fallback.
- **The login renews itself, until it does not.** The access token is refreshed
  automatically before each call that would otherwise use an expired one, so a
  long-lived session keeps working without another login code. When the refresh
  token itself expires or its session is revoked, the session stops and asks you
  to sign in again or sign out.
- **Workers follow the deployment, and the evaluation harness stays direct.** A
  task-run worker uses `worker.llm.transport`: `buildmax` gives it a run-scoped
  credential and no provider key, while `direct` gives it the deployment's
  configured provider access. Evaluation stays direct so results do not move
  with a deployment's catalog or quota.

Prompts, tool schemas, and tool results pass through the server in managed mode.
That is the point of it, and it is a real change in where your data goes — which
is why `buildmax models`, the model pickers, and the TUI footer all name the
mode.
| `hooks` | empty | Lifecycle hooks. Reference: [guide/hooks.md](../guide/hooks.md). |
| `sandbox` | disabled | Bash sandboxing. Reference: [guide/sandbox.md](../guide/sandbox.md). |
| `tools.permissions` | empty | Per-tool approval rules. See below. |
| `agent.max_parallel_tools` | `4` | How many read-only tool calls from one model message may run at once. Range 1-16; 1 disables it. |
| `agent.max_iterations` | `200` | How many times one prompt may call the model before the run stops. Range 1-5000. |
| `agent.turn_digest.recap` | `true` | Print a dim summary of what each turn did, under the reply. |
| `agent.turn_digest.suggest` | `true` | Offer the likely answer as ghost text when a turn ends by asking you something. |

### `tools.permissions`

BuildMax asks before a tool call that changes something, on surfaces where
somebody can answer — the CLI TUI and Desktop. Out of the box `Write`, `Edit`,
`Task`, and non-read-only MCP calls prompt; read-only tools do not, and `Bash`
follows its own risk classifier rather than the category default. A `Task`
delegated to a read-only agent type such as `explore` counts as read-only and
does not prompt — it can only reach tools that would not have prompted on
their own.

Set a rule to change that:

```yaml
tools:
  permissions:
    Write: allow                        # stop asking before file writes
    Task: ask
    Bash: deny                          # no shell at all
    "CallMcpTool:github/*": allow       # trust one server's tools
    "CallMcpTool:jira/delete_issue": deny
```

| Field | Meaning |
|---|---|
| key | A tool name, or a tool plus the target it dispatches to, with an optional trailing `*`. Case-insensitive. |
| value | `allow`, `ask`, or `deny`. An unrecognised value is ignored, and `buildmax tools status` lists it. |

The most specific rule wins: an exact target, then the longest matching
pattern, then the bare tool name.

Two limits worth knowing:

- **`allow` turns off the category prompt, not the safety checks.** Reading a
  sensitive path and running a risky shell command still prompt. Only `deny`
  outranks those.
- **`ask` means a human must look**, so on a surface with no human — print
  mode, a worker, a Portal conversation — the call is refused rather than run.

Answering a prompt with `a` allows that tool for the rest of the session
without writing a rule. Session grants are held in memory and are gone when the
process exits.

Run `buildmax tools status` to see every tool's classification, its resolved
action, and which layer decided it. Design:
[design/tool-permissions.md](../design/tool-permissions.md).

### `agent.max_parallel_tools`

When the model asks for several tool calls in one message, BuildMax can run
them at the same time:

```yaml
agent:
  max_parallel_tools: 4     # 1 disables it; range 1-16
```

Only calls the tool itself declares read-only ever overlap — `Read`, `Glob`,
`Grep`, `Skill`, `WebFetch`, and a `Task` delegated to a read-only agent type
such as `explore`. Writes, shell commands, a `Task` that can write, and MCP
calls always run alone, and calls are never reordered, so a batch means the
same thing at any setting: the message history a run produces is identical
whatever the limit. `buildmax tools status` shows which tools are read-only.

The limit applies inside a sub-agent too, so a delegated exploration schedules
its own reads rather than running them one at a time.

Raise it for read-heavy work over slow storage or many `WebFetch` calls. Lower
it to 1 to make a run reproduce exactly one call at a time. Design:
[design/parallel-tool-execution.md](../design/parallel-tool-execution.md).

### `agent.max_iterations`

One prompt runs the model, executes the tools it asked for, and calls the model
again with the results, until the model answers instead of calling a tool. This
caps how many times that may go round:

```yaml
agent:
  max_iterations: 200       # range 1-5000
```

A run that reaches the cap stops with `agent: max iterations exceeded` and exits
`7` — its own code, so a caller can tell an exhausted budget from a provider
that failed. Work already done stays done: the last iteration ran in full, so
its file edits and commands are on disk.

Raise it for a long unattended task — an overnight job or a benchmark run — where
nobody is there to say "keep going". Lower it to bound what a single prompt can
spend against your credential. `buildmax --max-iterations N` sets it for one run
and outranks this file. Sub-agents keep their own, smaller cap, and neither
setting raises it.

### `agent.turn_digest`

When a turn ends, the CLI TUI and Desktop can spend one small extra model call
to describe it:

```yaml
agent:
  turn_digest:
    recap: true             # dim summary of the turn, printed under the reply
    suggest: true           # predicted answer offered as ghost text; tab accepts
```

In the TUI the recap appears in the scrollback as a dim `❯❯` line; in Desktop it
closes the thread as a dim aside. The suggestion appears inside the input box,
greyed out, and only while the input is empty: press `tab` to accept it and
`enter` to send, or just start typing to ignore it.

Neither is part of the conversation. The model never sees a recap or a
suggestion on a later turn — they are written for you and thrown away.

The call is skipped on turns that could not produce anything: a turn that ran
no tools and answered briefly gets no recap, and a turn that ended without
asking you anything gets no suggestion. What it does spend counts towards the
session's usage — `/stats` in the TUI, the status bar in Desktop. Set either key to `false` to switch that half off, or both
to make the turn end with no extra call at all.

## `server.yaml` — Server and Worker

```yaml
log_level: info
port: 5678
jwt_secret: ""                       # inject via BUILDMAX_JWT_SECRET in production
# allow_signup: true                 # default false; accounts are created with `buildmax-server user create`
access_token_ttl: 168h               # signed, unstored — this is how long a leaked one works
refresh_token_ttl: 720h              # a stored row, so a session can be revoked before it expires
refresh_rotation_grace: 30s          # window for processes sharing one credentials file to refresh at once
shutdown_grace: 25s                  # whole budget for an orderly stop; keep below the orchestrator's kill deadline
cors_origin: http://localhost:5173   # or inject via BUILDMAX_CORS_ORIGIN where the Portal's port is chosen
workspaces_dir: /data/buildmax/workspaces
default_quota_tier: free_trial

conversation:                        # Tier 1 model used by the Portal agent loop
  model:
    model: openai/gpt-4o
    api_url: https://openrouter.ai/api/v1
    api_key: your-api-key
    context_window: 128000
  # provider: openai_compatible      # wire protocol; see settings.yaml above
  # max_tokens: 0                    # cap on one response; 0 = protocol default
  # model_target: fast               # use an llm.targets id for Tier 1 instead

# llm:                               # catalog is in the DB; this names a default
#   default_model: Fast              # a --name from `model list`

database:                            # MySQL
  host: localhost
  port: 3306
  user: buildmax
  password: buildmax
  name: buildmax                     # created on first start if it is missing

webhook:
  message_path: message              # JSON path to the prompt in the request body
  user_id: webhook                   # fallback identity for webhook-created runs

worker:
  binary: buildmax-worker
  run_mode: local_process            # or k8s_job
  server_url: http://localhost:5678  # how the worker reaches the server;
                                      # BUILDMAX_SERVER_URL overrides it
  k8s:
    namespace: buildmax
    image: buildmax:local
    config_map: buildmax-config      # ConfigMap holding server.yaml for worker pods
    home_dir: /buildmax              # BUILDMAX_HOME inside a worker pod

# audit:                             # governance trail retention
#   retention_days: 365              # default 0 — keep every event forever

storage:
  persist_backend: local_fs          # or minio — team uploads
  artifact_backend: local_fs         # or minio — run outputs and artifacts
  max_artifact_mb: 0                 # per-file upload cap; 0 uses the default
  minio:
    endpoint: http://localhost:9000
    region: us-east-1
    access_key: minio
    secret_key: minio123
    bucket: bmstore
    prefix: workspaces
```

Required for a working server: `jwt_secret` (or `BUILDMAX_JWT_SECRET`) and
`database`. Everything else has a usable default for local development. The
worker needs no credential of its own — `jwt_secret` is what signs the run token
the server hands it at dispatch.

The two token lifetimes are not interchangeable. An access token is signed and
never stored, so nothing can retire one early — `access_token_ttl` is the window
in which a leaked one still works. A refresh token is a database row, so
`refresh_token_ttl` is how long a session can be renewed, not how long it is
beyond reach. See [deploy/authentication.md](../deploy/authentication.md).

`shutdown_grace` is the whole budget for stopping the server in order, and
defaults to **25s**. On SIGINT or SIGTERM the server stops reporting ready so a
load balancer takes it out, ends the streams watching a run so the Portal
resubscribes elsewhere, drains the requests it already accepted, and then stops
its background loops. The phases are derived from this number rather than
configured one by one.

Keep it below whatever kills the process if the stop takes too long —
`terminationGracePeriodSeconds` on Kubernetes, `TimeoutStopSec` under systemd —
including any `preStop` hook. The reference manifests in
[`deployment/`](../../deployment/) set both together. Design:
[design/graceful-shutdown.md](../design/graceful-shutdown.md).

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

The worker reads the same `server.yaml` and needs at minimum `worker.server_url`
(or `BUILDMAX_SERVER_URL`), `workspaces_dir`, and the `storage` block — it talks
to blob storage directly rather than proxying through the server.

`storage.max_artifact_mb` caps one artifact upload. It defaults to **0**, which
uses the built-in 100 MB limit. It is a per-file limit and not a team storage
allowance: how many bytes a team may hold in total is a separate decision that
the quota model does not yet express.

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
- **The `llm` block** below says which of them a caller gets when it names none.
  It is not an authorization list: every enabled model is available to every
  user of the deployment.

Edit the catalog with `buildmax-server model`, on the machine that already holds
the database credentials:

```bash
buildmax-server model add --name Fast \
    --api-url https://openrouter.ai/api/v1 \
    --api-key your-openrouter-api-key \
    --model openai/gpt-4o-mini --context-window 128000

buildmax-server model add --name Claude --provider anthropic \
    --api-url https://api.anthropic.com \
    --api-key your-anthropic-api-key \
    --model claude-sonnet-4-5 --context-window 200000 --max-tokens 8192 \
    --reasoning medium --prompt-cache --vision

buildmax-server model list
buildmax-server model disable --id lm_xxxxxxxxxxxxxxxxxxxx
```

`--provider` is the wire protocol the upstream speaks — the same three values
`settings.yaml` uses, described under
[Model providers](#model-providers). It defaults to `openai_compatible`, so a
catalog written before this option existed keeps working unchanged.
`--max-tokens` caps one response; leaving it unset means the protocol's default,
which for `anthropic` is the built-in 8192. `--reasoning`, `--prompt-cache`, and
`--vision` are the catalog equivalents of the `settings.yaml` keys described
under [Reasoning](#reasoning), [Prompt caching](#prompt-caching), and
[Image input](#image-input). Changing either on a running server
takes effect on the next call: the router rebuilds a client whose target's
connection details changed.

| Key | Meaning |
|---|---|
| `llm.default_model` | The `--name` a caller gets when it names none. Empty uses the first enabled model in the catalog, so a single-model deployment needs nothing here. |
| `conversation.model_target` | Runs Tier 1 on a catalog model instead of `conversation.model`. An `llm_model` ID, not a name: the server picks its own model rather than naming one the way a client does. |

A server with no `llm` block serves the catalog with its first enabled model as
the default. `conversation.model` stays the bootstrap path — a fresh deployment
answers conversations, and offers that model by name, before its catalog has a
single row.

A `default_model` naming a model that does not exist **stops the server at
startup**. It parses cleanly and would otherwise fail every session at its first
call, which reads as a model outage rather than a typo. An empty catalog is not
an error: rows are added while the server runs.

Managed calls need a database for two reasons: the catalog lives there, and
every call is recorded in the `llm_call` ledger. Without a store the routes
answer `503` rather than serving inference nobody can account for.

Credentials are stored in the `llm_model` table and read by exactly one query,
the one that builds a provider client. They are never returned by a model
listing, an API response, or an error. Note what that implies for operations:
database backups and read replicas carry provider keys, so treat them the way
you treat the database password.

### A local model in a deployment

A deployment can point at an Ollama daemon the same way the CLI does, with one
difference that decides everything else: **the daemon has to be reachable from
the server, and inside a container `localhost` is the container.**

Either place accepts it, and neither takes a credential:

```bash
# a catalog target teams can be granted
buildmax-server model add --name "Local Qwen" --provider ollama     --api-url http://ollama.ollama.svc.cluster.local:11434     --model qwen3:8b --context-window 32000
```

```yaml
# or Tier 1 conversation, in server.yaml
conversation:
  model:
    model: qwen3:8b
    provider: ollama
    api_url: http://ollama.ollama.svc.cluster.local:11434
    context_window: 32000
```

`--api-key` is not required for this provider and nothing is stored for it. A
key given anyway is ignored.

**Reaching a daemon on the host machine.** For a local Kubernetes cluster,
running the daemon on the host and pointing the deployment at it is usually
better than running it in a pod: a pod cannot use the host's GPU, so inference
falls back to the CPU of whatever VM the cluster runs in.

| Host | Address that reaches it from a pod | Also needed |
|---|---|---|
| Docker Desktop (macOS, Windows) | `http://host.docker.internal:11434` | nothing — the gateway forwards to the host's loopback |
| Linux, cluster in Docker (kind, k3d) | the bridge gateway, `http://172.x.0.1:11434` — `docker network inspect <net>` prints it | `OLLAMA_HOST=0.0.0.0`, or the daemon listens on loopback only |
| A real cluster | the daemon's own Service or an address that routes to it | — |

Two properties worth stating rather than discovering:

- **The endpoint is operator-supplied and is never taken from a client
  request.** A target pointing at a loopback or link-local address means the
  *server's* network, which is a deployment decision. Only a system
  administrator can add one.
- **Managed calls are still metered.** A local target has no cost per token, but
  it lands in the `llm_call` ledger like any other, which is what makes it a
  usable way to exercise the gateway, quota, and audit paths without paying for
  them.

### Managed models for task runs — the `worker.llm` block

Task runs default to calling a provider themselves. Point them at the gateway
instead, and the worker stops needing an upstream key:

| Key | Meaning |
|---|---|
| `worker.llm.transport` | `direct` (default) or `buildmax`. Under `buildmax`, `BUILDMAX_CONVERSATION_MODEL_API_KEY` is withheld from the worker. |
| `worker.llm.model` | Which catalog model a run calls, by `--name`. Empty uses `llm.default_model`. |
| `worker.llm.context_window`, `worker.llm.call_timeout` | Describe the model to the run; the protocol does not report them per call. |
| `worker.run_token_ttl` | How long a run's credential stays valid. Defaults to 24h. Every run gets one, managed or not. |
| `worker.run_timeout` | How long a run may stay `SCHEDULED` or `RUNNING` before the server records it as abandoned. Defaults to 6h. It is the backstop, not the usual detection path: a `RUNNING` run whose worker stops reporting is failed within minutes, and a worker that is asked to stop reports its own outcome. What is left for this timeout is a run that never reached `RUNNING`, or one that never reported at all. |

The server states the transport and model; a worker never chooses its own model,
and is told nothing else about it — endpoint, upstream identifier, and
credential all stay server-side. Each run is dispatched with its own credential
in `BUILDMAX_RUN_TOKEN`, which authorizes that run and nothing else.

`./make compose smoke managed` runs the whole path against a mock upstream and
needs no provider key.

Two things to know before enabling it:

- `worker.llm.model` naming a model the catalog does not have **stops the server
  at startup**, the same way `llm.default_model` does. That configuration parses
  cleanly and would otherwise fail every run at its first model call.
- The run token is not renewable. `run_token_ttl` must outlast your longest
  run; a run that outlives it loses its remaining model calls.

## Data Directory Layout

```text
<BUILDMAX_HOME>/
├── settings.yaml       CLI and Desktop configuration
├── server.yaml         Server and worker configuration
├── policy.yaml         Optional operator sandbox policy (overrides settings.yaml)
├── mcp.json            Optional MCP servers, merged with a workspace file
├── skills/<name>/      Skills available in every workspace
├── agents/             Subagent definitions available in every workspace
├── plugins/<name>/     Installed plugins; .state.json holds their source
├── sessions/           index.json plus one folder per session
│   └── <id>/           meta.json, history.jsonl, traces/<run>.jsonl
└── logs/               Rotating buildmax.log
```

`./make test` sets `BUILDMAX_HOME=./testing-sandbox`, so tests never touch a real
data directory.

## Precedence

For any value that appears in more than one place:

```text
environment variable  >  policy.yaml  >  settings.yaml / server.yaml  >  built-in default
```

Sandbox is the security-sensitive exception: `policy.yaml` > per-run CLI >
`BUILDMAX_SANDBOX_ENABLED` > `settings.yaml` > surface default. A run may enable
the sandbox and select its approval mode, but it cannot disable sandboxing or
override operator policy.

The sandbox block is the only one with a workspace-level layer; hooks are the
only block that merges additively (global hooks and workspace hooks both run)
rather than overriding.
