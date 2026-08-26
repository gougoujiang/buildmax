# Changelog

All notable changes to BuildMax are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
BuildMax is currently in alpha, so incompatible changes may still occur between
pre-releases and must be called out in release notes.

## [Unreleased]

Unreleased entries live one per file under
[`docs/changelog/`](docs/changelog/README.md), so two branches adding one never
touch the same line. `./make changelog` prints what they currently say, and
release preparation folds them into a dated section here.

## [0.2.0-alpha.2] - 2026-08-26

### Highlights

- A conversation can go back: `/rewind` in the TUI moves it to an earlier
  message, `/fork` branches a new session off one, and Desktop offers both
  behind a **History** button. Each names the tools that ran in the span you
  are choosing across, because rewinding moves the conversation and does not
  undo the files those tools wrote.
- A session is now a folder — metadata, an append-only conversation journal,
  and its own run traces — written as the turn happens rather than rewritten
  after it. An interrupted run keeps everything up to the moment it stopped,
  and a session can only be open in one place at a time.
- Logins are kept in the operating system's credential store (Keychain,
  Credential Manager, Secret Service) instead of as plaintext in `auth.json`.
- A turn ends with a dim recap of what it did, and the answer you are likely
  about to type is offered as ghost text when the agent asks you something.
- `openapi.json` now describes every route the server registers — 117
  operations instead of 40 — held to an exact match by a test in both
  directions.

### Upgrade notes

- **Sessions from earlier versions are ignored.** The move to `sessions/<id>/`
  ships with no conversion, and the `traces/` root is gone with it. Existing
  conversations and their traces stay on disk untouched; BuildMax will not
  read them. Delete them, or keep them for reference.
- A plaintext `auth.json` is moved into the credential store the first time it
  is read, so a login survives the upgrade. A machine with no usable store
  falls back to the file as before, and `BUILDMAX_CREDENTIAL_STORE=file` keeps
  the previous behavior deliberately. `buildmax login`, `whoami`, and `doctor`
  say which one a login is actually using.
- API clients generated from `openapi.json` will see far more than the routes
  change: `created_at`, `started_at`, and `ended_at` are typed as RFC 3339
  strings rather than integers, which is what the API has always sent, and
  managed inference is documented at `/api/llm/models` and
  `/api/llm/completions` rather than the team-scoped paths that never existed.
- A Compose stack that moves the Portal off `8080` should set
  `BUILDMAX_CORS_ORIGIN`, which now follows `BUILDMAX_PORTAL_PORT`, instead of
  hand-editing `server.yaml`.

### Added

- Desktop offers rewind and fork through a **History** button in the chat status
  bar: one list of messages, with a tab for whether choosing one moves this
  conversation back or starts a new session from it. Each names the tools that
  ran in the span you are choosing across, because their effects stay on disk
  either way. Both are refused while a run is in flight, and say so.

- `/fork` in the TUI branches a new session off an earlier message and switches
  to it, leaving the original untouched — for trying a second approach without
  losing the first. The two are independent from that point on, so deleting one
  never affects the other. It shares the picker `/rewind` uses, and names the
  tools that ran after the fork point, because their effects are on disk and the
  new session's history will not mention them.

- CLI and Desktop now keep a login's access and refresh tokens in the
  operating system's credential store (Keychain, Credential Manager, Secret
  Service) instead of as plaintext in `auth.json`. A file written before this
  change is moved on first read, and a machine with no usable credential store
  falls back to the file as before; `buildmax login`, `buildmax whoami`, and
  `buildmax doctor` say which one a login is actually using. Set
  `BUILDMAX_CREDENTIAL_STORE=file` to keep the previous behavior.

- `/rewind` in the TUI moves the conversation back to an earlier message. It
  says which tools ran in the part you are about to drop before you choose, and
  again afterwards, because rewinding moves the conversation and does not undo
  the files it wrote or the commands it ran. Nothing is deleted: the messages
  you rewind past stay on disk, and the next reply starts a new branch.

- The CLI TUI and Desktop now end a turn with a dim recap of what it did, and
  offer the answer you are likely about to type as ghost text in the input box
  when the agent asks you something — `tab` accepts it. Neither enters the
  conversation. Configure with `agent.turn_digest` in `settings.yaml`.

### Changed

- `./make help` now lists every command, grouped by what it is for, with the
  contributor path under it. It used to open on six commands and keep the rest
  — including `eval`, `models`, and the deployment tasks — behind
  `./make help all`, which is now an alias for the same list.

- Enabling or disabling a catalog model with `buildmax-server model enable` or
  `model disable` now records the model's name in the audit trail, which the
  equivalent `/api/admin` route already did. The trail distinguishes a catalog
  change by who made it, not by where it was made.

- Each session is now a folder under `sessions/<id>/` holding its metadata, an
  append-only conversation journal, and its own run traces, replacing the single
  JSON file per session and the `traces/` root. The conversation is written as
  it happens rather than rewritten after each reply, so an interrupted run keeps
  everything up to the moment it stopped, and BuildMax can tell a tool call that
  never started from one that may already have changed something. A session can
  be open in one place at a time; opening one already in use says so instead of
  letting two runs overwrite each other. Sessions from earlier versions are not
  migrated and are ignored.

### Fixed

- Session files and the session index are now replaced atomically, so a crash,
  a full disk, or a machine failure part-way through a save leaves the previous
  conversation intact instead of an unreadable file.

- Prevented stale-run recovery and late worker reports from overwriting a task
  run's committed outcome or leaving its task and artifact list out of sync.

- The Compose stack derives `cors_origin` from `BUILDMAX_PORTAL_PORT` through
  the new `BUILDMAX_CORS_ORIGIN` override, so moving the Portal off `8080` — to
  run it beside a kind cluster, which cannot move — no longer needs a hand edit
  of `server.yaml` to keep the browser from blocking every request.

- A team membership record with no role is now read as a member everywhere.
  Team-scoped routes previously refused such a record entirely while resource
  routes admitted it, so the same account could be a member for one request and
  a stranger for the next. No release could create one, so this affects only a
  database written before the role was defaulted.

- A background run on a deployment that leaves `worker.llm.model` unset reaches
  the deployment's default model again. The run was assembled with no model at
  all and failed with `model not found: ""`; naming a model in `worker.llm` was
  the only way around it.

- Starting an Issue's assigned Agent no longer leaves an empty conversation
  behind when the run is refused. A team at its quota limit collected one on
  every attempt: the conversation was created before the task, the task was
  what checked the allowance, and nothing deletes a conversation.

- `openapi.json` now describes every route the server registers, 117 operations
  instead of 40, and corrects the schemas that called a timestamp an integer:
  `created_at`, `started_at`, and `ended_at` are RFC 3339 strings, which is what
  the API has always sent. Tests now hold the document to an exact match with
  the registered routes.

- The served OpenAPI document no longer describes routes that do not exist.
  Managed inference is `GET /api/llm/models` and `POST /api/llm/completions`,
  not the team-scoped paths it listed, and listing or creating a conversation is
  team-scoped rather than `/api/conversations`.

- Correct model administration and kind-seeding guidance to use the
  deployment-wide catalog instead of removed aliases and managed settings.

- Fixed TUI shutdown so Ctrl+C cancels and joins the active agent run instead
  of leaving stream senders or background goroutines behind.

## [0.2.0-alpha.1] - 2026-08-24

### Highlights

- The local agent can work in the background: `Bash`, `Task`, and the new
  `Monitor` tool detach into jobs, `JobList`, `JobOutput`, and `JobStop`
  inspect them, and a completion or a monitor line can wake the conversation
  in the TUI and in Desktop. Every job writes a durable, redacted event log.
- The CLI and Desktop now have exactly two modes, decided by whether you are
  signed in: local models from `settings.yaml` straight to their provider, or
  the models a deployment offers through its gateway. Neither mode covers for
  the other.
- Local inference through Ollama, in a local session and through a deployment
  alike, with no provider key on either path.
- Runs report what they cost: per-model `pricing`, per-call cost in traces,
  cache tokens on every surface that shows spend, `buildmax stats` for a
  session, and prompt caching as a policy rather than a boolean.
- Teams activate published plugin releases, an agent names the plugins its
  background runs load, and a worker materializes exactly those. Portal gains
  a Plugins section under Space.
- Identity and storage were rebuilt: opaque unprefixed entity identifiers,
  numeric relational keys underneath them, `DATETIME(6)` timestamps, and an
  API resource that names its own identifier `id`.

### Upgrade notes

- **An existing Alpha database and object store must be recreated.** Opaque
  entity identifiers, numeric relational keys, text `public_id` columns, and
  `DATETIME(6)` timestamps all land in this release with no conversion
  migration. Every existing identifier, access token, refresh session, worker
  token, Portal link, bookmark, and stored object key is invalid.
- API clients that read a resource's own identifier from a type-named field
  (`task_id`, `user_id`, `team_id` on the resource itself) must read `id`.
  Relationships keep their semantic names.
- The deployment-wide `worker.token` is gone. Remove the secret from your
  manifests and upgrade the server before the worker image: a run dispatched
  without a run token now fails rather than falling back.
- Managed models are named by catalog name. Replace `llm.aliases` and
  `llm.default_alias` with `llm.default_model`, `worker.llm.alias` with
  `worker.llm.model`, and `/api/teams/{team_id}/llm/...` with `/api/llm/...`;
  a `transport: buildmax` entry drops `team_id` and names the catalog model.
- `models[].transport` and `models[].server_url` are removed from
  `settings.yaml`; a session's mode follows `buildmax login`/`logout`, and
  `default_model` names the entry a signed-out session starts with.
- The `prompt_cache` boolean is removed. Write `cache_control: {mode: off}`
  where it said `false`; Anthropic agent turns now cache by default.
- The server stops in order within `shutdown_grace` (default 25s). Deployments
  should set a matching `terminationGracePeriodSeconds` and `preStop` pause, as
  the reference manifests do.
- Native Desktop bundles are still not published by the release workflow. The
  tagged source contains Desktop; GitHub Release artifacts contain the CLI and
  server binaries.

### Added

- A Portal agent names the plugins it loads for a background run, and loads only
  those: nothing is inherited from what its team activated. The selection
  versions with the rest of the definition, so an earlier revision still says
  what that agent named, and restoring one brings its selection back.

- Background events can wake the conversation in the TUI: `run_in_background`
  calls accept `deliver_result` to have the completion delivered as its own
  turn, and a `Monitor` started with `react` sends each delivered line back
  for analysis. Delivered payloads are marked as untrusted observations,
  recorded with non-user provenance, and never run user-prompt hooks.

- Background subagents in the TUI and Desktop: `Task` accepts
  `run_in_background` to delegate investigation without blocking the
  conversation. The final reply is read with `JobOutput`, the job stops with
  `JobStop`, and traces link the subagent run to the tool call that launched
  it.

- `./make cache-qualify` checks prompt caching against a real provider. Every
  other cache test runs against a fake upstream, which proves what BuildMax
  sends and nothing about what a provider does with it — a request can be
  perfectly shaped while the provider declines to cache it, for a minimum prefix
  length, an unsupported model, or an expired retention window. The suite runs
  first write, sequential read, changed prefix, long-history lookback,
  streaming, concurrent cold starts, and retention, and prints what the provider
  reported for each. Name the target with `BUILDMAX_CACHE_QUALIFY_PROVIDER`,
  `_MODEL`, `_API_KEY`, and optionally `_BASE_URL`; it calls a paid provider, no
  check runs it, and it skips when none is named. A model entry can also name an
  `integration` for an OpenAI-compatible gateway whose cache behaviour has been
  qualified — none has, so every value is currently refused.

- Desktop delivers background events into the conversation: a completion
  requested with `deliver_result` or a `react` monitor line runs as its own
  turn when the owning session is on screen and idle, and is parked — not
  lost — while that session is busy or another one is open. The transcript
  labels delivered events as background observations, collapsed by default.

- Background jobs write a durable event log under `<traces>/jobs/`: launch
  provenance (owning session, parent run and tool call, sandbox fact),
  monitor lines with drop accounting, and the terminal state. Logs are
  redacted, bounded, and always end with how the job ended.

- `./make kind seed` fills the local kind cluster's model catalog from the
  repository-root `settings.local.yaml` and grants the deployment's teams an
  alias for each model, so the CLI and Desktop can drive the managed transport
  against real inference. The cluster's own Portal conversations and task runs
  keep answering from the deterministic mock.

- Background commands in the TUI and Desktop: `Bash` accepts
  `run_in_background` to detach long builds, tests, or servers as local jobs,
  and the new `JobList`, `JobOutput`, and `JobStop` tools inspect and stop
  them. Jobs pass the normal permission and sandbox checks before detaching
  and end with the application.

- Local models through Ollama: `provider: ollama` on a model entry calls a
  local daemon's own API with no `api_key` at all. It sends the context window
  on every call, so the daemon no longer applies its own default and quietly
  truncates the system prompt and tool definitions out of a longer request —
  the failure that made small local models look like they could not call tools.
  `buildmax init --ollama` writes the entry, `buildmax models --local` lists
  what is installed and which models can call tools, and `buildmax doctor`
  reports a daemon that is not running or a model that is not pulled with the
  command that fixes it.

- A deployment can serve a local Ollama model: `--provider ollama` on
  `buildmax-server model add`, or `provider: ollama` under
  `conversation.model`, with no credential in either place. Real inference and
  real tool calls reach the gateway, the `llm_call` ledger, and quota without a
  provider key or a bill. The daemon stays on the host — a pod cannot use the
  host's GPU — and the deployment names an address that reaches it, which under
  Docker Desktop is `host.docker.internal`.

- New `Monitor` tool in the TUI and Desktop: watch logs, files, or CI by
  running a command whose stdout lines become bounded events. Lines are
  rate-limited and truncated, dropped lines are counted, and the watcher
  passes the same permission and sandbox checks as `Bash`.

- OpenAI Responses calls now carry a scoped `prompt_cache_key`, and can ask for
  24-hour retention with `cache_control: {ttl: 24h}`. The API caches on its own
  either way, so the key does not turn caching on — it decides which prefixes
  are looked up together, which matters because callers sharing a credential
  otherwise share one bucket. The key is derived from the credential, the model,
  the team on a managed call, and fingerprints of the system prompt and tool
  definitions; it carries none of them in readable form and is never written to
  a ledger, trace, or log. Retention vocabulary is per provider: `5m` and `1h`
  are Anthropic's and `24h` is OpenAI's, and asking for one where it is not
  documented is refused at startup rather than sent and ignored.

- Add `./make models list`, `./make models info <model>`, and
  `./make models check` to look up configured and OpenRouter-catalog model
  details, and catch context_window drift, from the terminal instead of the
  openrouter.ai models page.

- `buildmax -p --output json` and `--output jsonl` now report `trace_id` and
  `trace_path`, so a script can open the trace that run wrote instead of
  guessing at the newest file in the session's trace directory.

- A run can now report what it cost. A model entry takes a `pricing` block —
  currency plus four decimal rates per million tokens, for fresh input, cache
  reads, cache writes, and output — and the CLI prints a `Cost(session)` line,
  `--format json` carries the same figures, and the session file keeps a running
  total. A managed deployment sets the same rates per catalog model with
  `--currency`, `--input-price`, `--cache-read-price`, `--cache-write-price`,
  and `--output-price` on `buildmax-server model add`; the rates in force are
  copied onto each call's ledger row when it is accepted, so repricing a model
  does not restate what a team already spent. Portal's run view shows the run's
  cost and what caching saved against an uncached baseline. Cost is shown only
  where every rate needed for it was recorded — anything else reads
  `unavailable` rather than zero — and a saving is reported only when caching
  actually saved: a run that wrote cache entries nothing read back paid more
  than it would have uncached, and is shown as the cost it was.

- Run details now name which revision of an agent definition a run executed
  under, and say when the definition has been edited since. Editing an agent
  changes what its next run does, which is intended; until now nothing recorded
  which text produced a given result.

- Run details now open with where the run came from. A background run records
  the conversation message it was asked for in, and the Portal shows that
  message next to the instruction the worker was actually given — so a
  constraint missing from the instruction can be told apart from one that was
  never asked for. It is shown even for a run that wrote no trace.

- `buildmax stats [session-id]` reports one session's spend with its cache
  breakdown, how close it came to the context window, how many bytes each tool
  put back into that window, the split between model time and tool time, and
  what its delegated runs cost; `--json` emits the whole record. `/stats` in
  the TUI shows the same figures for the session on screen.

- A team can activate published plugin releases for its background runs, pinned
  to an exact version and digest. A team either curates that list or leaves it
  open, in which case naming a plugin in an agent activates it. Releases
  contributing hooks or MCP servers cannot be activated yet.
  `buildmax plugin activations --team <id>` reads what a team activated.

- Portal gains a Plugins section under Space: what this team has activated and
  at which version, what each release contributes, which agents name it, whether
  a newer release is available, and whether the team curates its list or opens
  the whole catalog. Owners and admins can activate, update, and suspend from
  there; any member can read it.

- A run trace now records what each model call cost, not just what the run did.
  `llm_end` carries that call's own token counts and its estimated cost;
  `run_end` carries the run's. The per-call figures matter because the running
  totals cannot answer which turn was expensive, and subtracting consecutive
  records to find out goes wrong the moment a call in between failed. It also
  makes the shape of caching visible: the turn that writes a cache entry costs
  more than it would have uncached, and only a later turn reading it back puts
  the run ahead. Costs are absent when the model was unpriced, which is not the
  same fact as a call that cost nothing, and `cost_incomplete` on `run_end`
  says a call did work that could not be priced.

- A background run now loads the plugins its agent names. The server resolves
  the team's activations when the worker claims the run, and the worker fetches
  exactly those releases, verifies each against its pinned digest before
  extraction, and refuses to start rather than run without one it was told to
  have.

### Changed

- An API resource now names its own identifier `id` rather than repeating its
  type — a task returns `{"id": ..., "team_id": ...}` where it used to return
  `{"task_id": ..., "team_id": ...}`. Relationships are unchanged and keep their
  semantic names, so only the field naming the resource itself moved. Users,
  teams, artifacts, audit events, managed model catalog entries, system grants,
  managed LLM call records, and webhook keys are affected; most other resources
  already used `id`. Agent and workflow revisions no longer return an identifier
  at all: a revision is addressed by its parent plus its revision number, which
  is what the restore route already used. Catalog plugins and plugin releases
  likewise drop theirs, being addressed by name and by name plus version.

- The CLI and Desktop now run in one of two modes, decided by whether you are
  signed in. Signed out, the models are the ones in `settings.yaml` and each
  call goes straight to its provider; signed in, they are the ones that
  deployment offers and every prompt goes there. `buildmax login` and
  `buildmax logout` switch, and nothing else configures it — `models[].transport`
  and `models[].server_url` are gone, because a session is in one mode or the
  other rather than holding both kinds of entry. A new `default_model` key names
  which entry a session starts with while signed out; a deployment names its
  own. `buildmax models`, the `/model` pickers, and the TUI footer all say which
  mode you are in, and `buildmax doctor` reports it as a check of its own.
  Desktop no longer opens on a sign-in form: local mode is a working state, so
  the workbench opens directly and signing in is an action in the account menu.
  Neither mode covers for the other — a deployment that is down, or a login that
  has expired, stops the session and says so rather than quietly sending the
  next prompt to a provider you did not choose for it.

- `./make doctor` now warns that Portal test dependencies and Playwright
  browsers are unavailable when npm is missing, instead of reporting cached
  browser state as ready for `./make e2e`.

- The server now stops in order on SIGINT or SIGTERM: it reports itself
  unready so a load balancer stops sending it work, ends the streams watching a
  run so the Portal reopens them elsewhere, refuses new conversation turns and
  waits for the ones running, drains in-flight requests, and only then stops its
  background loops. The whole budget is `shutdown_grace` in `server.yaml`,
  default 25s; the reference manifests set a matching
  `terminationGracePeriodSeconds` and a `preStop` pause.

- `buildmax --help` no longer lists cobra's auto-generated `completion`
  command. `buildmax completion <shell>` still prints the shell script for
  anyone who wants it.

- BuildMax now identifies its outbound LLM requests with a versioned
  `User-Agent` header.

- `worker.run_mode: local_process` is now documented as what it is — a
  single-machine topology where a task run is a child process of the server
  under the same uid, sharing its trust domain — instead of being called a
  development path the Compose deployment contradicts. The startup warning says
  the same. Nothing about how a worker is launched changed; a deployment that
  needs its server separated from model-chosen code still runs
  `worker.run_mode: k8s_job`.

- A managed model is now named by its catalog name rather than by a team alias.
  `server.yaml` loses `llm.aliases` and `llm.default_alias` and gains
  `llm.default_model`, which names one of the models `buildmax-server model add`
  created, or is left empty to use the first enabled one. `worker.llm.alias`
  becomes `worker.llm.model`. A name that matches no catalog row now stops the
  server at startup, while an empty catalog does not — rows are added while the
  server runs. In `settings.yaml`, a `transport: buildmax` entry drops `team_id`
  and puts the catalog name in `model`; every model a deployment offers is
  available to every user of it, so `buildmax models --team <id>` becomes
  `buildmax models --server`. The gateway routes move from
  `/api/teams/{team_id}/llm/...` to `/api/llm/...`, and a managed call is
  recorded against the person who made it rather than against a team.

- Entity relationships are stored as numeric keys rather than as repeated
  identifier strings. Every reference that names exactly one kind of row is now
  a `bigint`, the public handle each row shows the outside world is a separate
  `binary(12)` column, and translating between the two happens inside the store
  and nowhere else. Nothing about the API changed — a handle is still what
  every request, response, token, log line, and object key carries. What
  changed is underneath: a team's task list is answered by reading an index
  backwards instead of sorting rows, usage aggregation is answered from indexes
  without reading rows at all, and identity no longer depends on the database's
  text collation. References that cannot be one number — a polymorphic actor, a
  provider's tool-call ID, an agent session naming a file — stay text
  deliberately, and a test refuses a new one added without that reason.

- Login-chain, trace-file, and Desktop-project identifiers lost their `as_`,
  `rt_`, and `p_` prefixes and are now ordinary opaque IDs, leaving one
  identifier format in the codebase. None of the three is read by a person or
  dispatched on; they had kept a prefix only because they are not database
  rows, which was where the previous change stopped rather than a reason to
  keep one. Background jobs are the exception and keep `jb_`: a job ID reaches
  the model as a bare string inside tool output, and free prose is the one
  place a type prefix says something the surrounding context does not.

- Entity identifiers are now opaque and unprefixed: `ivyoh5qcfu6ypfkhyedq`
  rather than `t_9f3k2m8x1qwe7rt4zy0p`. Each is 96 bits of crypto-random data
  written as 20 lowercase base32 characters, case-insensitive on input and safe
  unchanged in a URL, a filename, a Kubernetes name, and a shell. The type
  prefix is gone because nothing dispatched on it — a route, a JSON field, and a
  column already name the type — and because it was the only thing standing
  between a presentation choice and the database schema. Agent and workflow
  revisions, catalog plugins, and plugin releases carry no identifier at all
  now; they are addressed by their parent plus a revision number, by name, and
  by name plus version. Login-chain, trace-file, and Desktop-project identifiers
  keep their prefixes: none of them names a database row. **This is a breaking
  change with no compatibility path.** Every existing identifier, access token,
  refresh session, worker token, Portal link, bookmark, and stored object key is
  invalid. Recreate the database and object store rather than upgrading.

- Sub-agent delegations to a read-only agent type such as `explore` now run in
  parallel with each other and no longer prompt for approval, and a sub-agent's
  own tool calls honour `agent.max_parallel_tools` instead of always running one
  at a time.

- Prompt caching is now a policy rather than a boolean, and Anthropic agent
  turns cache by default. A model entry takes `cache_control: {mode, ttl}` —
  `mode: auto` (the new default) asks on an agent turn, whose prefix goes out
  again on the next iteration, and never on a one-shot call such as title
  generation or compaction, where a cache write costs more than it can ever
  save; `off` never asks and `force` always does. `ttl` selects retention where
  the provider documents it — `5m` or `1h` on Anthropic, `24h` on OpenAI — and
  is refused at startup anywhere else rather than sent and ignored, as is
  `force` on a provider that takes no cache instructions at all. The
  `prompt_cache` boolean it replaces is removed rather than kept as a
  shorthand; write `cache_control: {mode: off}` where it said `false`. Managed
  deployments get the same policy per catalog model through `--cache-mode` and
  `--cache-ttl` on `buildmax-server model add`.

- Prompt-cache token counts now reach every surface that shows what a run
  spent. Providers already reported them and the managed ledger already stored
  them, but they stopped there: run statistics, run traces, session totals, the
  CLI's summary and `--format json` output, Desktop's run status, the team
  run-ledger route, and Portal's run-spend view all dropped them, so a cached
  run was indistinguishable from an uncached one. Cached counts remain a
  breakdown of the prompt rather than an addition to it, and each surface shows
  them only where a provider actually reported some — a provider that reports
  nothing is not a provider that missed.

- Server `public_id` columns now store the 20-character canonical text form
  instead of raw bytes, so direct database queries show the same IDs the API
  does. Breaking for existing server databases: there is no migration —
  recreate the database (`./make kind down && ./make kind up`, or drop the
  Compose MySQL volume).

- The deployment-wide `worker.token` is gone. Every `/api/worker/*` route now
  takes only the run token the server mints at dispatch, so a worker can read
  and write its own run and nothing else. `worker.token` and
  `BUILDMAX_WORKER_TOKEN` are no longer read, and the secret has been dropped
  from the Compose and Kubernetes manifests — remove it from yours. Upgrade the
  server before the worker image: a run dispatched without a run token now
  fails immediately instead of falling back.

- Stopping a server in `local_process` worker mode no longer waits for the agent
  run it dispatched. The scheduler stops claiming immediately, asks the worker it
  spawned to stop — which reports what the run produced — and gives up on a
  worker that will not go, within the same `shutdown_grace` budget. In
  `k8s_job` mode the Jobs already outlive the server, and still do.

- Every stored moment in time is now a `DATETIME(6)` column and an RFC 3339
  string in the API, replacing Unix seconds in `bigint` columns and JSON
  numbers. Audit `since` and `until` query parameters take RFC 3339 too. An
  existing Alpha database is recreated rather than converted; no conversion
  migration ships.

- A task run whose worker is shut down — a drained node, an evicted pod, a
  restarted deployment — now stops, uploads what it produced, and reports
  `FAILED` with a message naming the shutdown, instead of sitting in `RUNNING`
  until the stale-run reaper closes it hours later. Its output and artifacts are
  kept and shown the way a cancelled run's are.

### Fixed

- Editing an agent no longer fails with a duplicate-key error. The update wrote
  a row rebuilt from the domain model, which no longer carries the database's
  own key, so it was saved as a new agent rather than as a change to the
  existing one; the unique index caught it and refused the edit. The update now
  addresses the agent by its identifier.

- The Portal conversation list no longer fills up with machinery. A workflow
  step and an issue agent run each create a conversation because a task requires
  one, and those were listed alongside conversations people actually hold — on a
  team that runs either, they pushed real conversations off the page. They are
  still kept, and a link straight to one still opens it.

- A finished background task now always gets reported back to its conversation.
  The reply used to be a one-shot attempt: a model call that failed, a busy
  conversation, or a server restart between the task finishing and the reply
  being written meant the conversation was simply never told. The report is now
  recorded as owed and retried until it succeeds, or until it has failed enough
  times to be given up on with the reason kept. The result itself was never at
  risk — the task's card reads it directly.

- Closing the runtime now waits for the background job trace writer to drain,
  so a job's final record always lands in `traces/jobs/` before exit.

- Refresh the built-in context_window fallback table: several ids had drifted
  from what providers now report (some by a lot, e.g. deepseek-r1), one
  Anthropic id had a hyphen where the real id uses a dot and never matched,
  and OpenAI/Anthropic now cover their fast/mid/premium tiers instead of one
  model each.

- The TUI `/model` and `/tasks` panels scroll. With more entries than fit the
  terminal each listed the first few and a `… N more` row, while the arrow keys
  kept moving a selection through the ones it was not showing — so a model past
  the fold could be switched to, and a job past it stopped with `s`, without
  ever being seen. Both lists are now a window that follows the selection, and
  `/model` opens on the model in use rather than at the top.

- A Portal conversation now shows the background tasks it started. Each task
  gets a card in the thread carrying its status, what the run produced, its
  files, its run details, and stop or run-again, placed among the messages by
  when it was created. The cards are read from the server, so they survive a
  refresh and a dropped connection, and a task result is no longer displayed as
  if the user had typed it.

- The Portal image now ships third-party license attributions: the licenses of
  every npm production dependency are served at `/third-party-notices.txt`, and
  the image carries the Apache-2.0 text under `/usr/share/licenses`.

- Pressing esc on the `/` command popup now closes it for good. The popup was
  rebuilt from the input on every message, so the next cursor blink brought it
  straight back; it now stays closed until the typed command changes.

- Session token counts and cost now include the work a run delegated to a
  subagent and what each context compaction cost; both were previously spent
  and never counted, so long sessions and ones that used `Task` under-reported
  themselves. A subagent's trace is also filed under the session that started
  it rather than under a discarded id, and `tool_end` records how a call failed.

- A finished background task now reports back whether or not anyone is watching.
  The reply used to be sent through the creator's first open browser connection,
  which meant it was skipped entirely when they had none and reached only one
  tab when they had several. Every connection on the team is now told the task
  changed, and the reply itself is written to the conversation independently of
  any connection.

- A run trace now reaches disk one record at a time, so a run that is
  interrupted — killed, crashed, or given up on mid-turn — keeps its final
  records instead of losing up to 4KB of them. Previously the tail stayed
  buffered until the run closed cleanly, which made the last recorded event
  appear seconds before the last one that actually happened. Shutting the
  runtime down also closes the trace of a run still in flight, marking it
  abandoned rather than leaving it open with no `run_end`.

- A TUI panel such as `/model` or `/tools` now trims its list to what the
  terminal can show. It listed a fixed number of rows whatever the height was,
  so on a short terminal the panel pushed the input box and the footer off the
  top of the screen, and nothing could scroll them back. Panel lines also no
  longer wrap inside the panel border, which was quietly doubling the height of
  the `/tools`, `/skills`, and `/diff` panels.

- Closing a TUI panel such as `/model` no longer leaves its box drawn above the
  input. The terminal renderer the CLI depends on stopped erasing the lines a
  shrinking frame vacates, so every dismissed panel stayed on screen; the
  dependency is pinned back to a working revision.

- The inbound webhook's `202` response no longer labels a run identifier
  `task_id`. It was returning the task run's ID under the task's name and no
  task identifier at all, so a caller that wanted to follow the work had to
  guess which of the two it had been handed. The body now carries both, matching
  what creating a run through the API already returned:
  `{"task_id": "...", "task_run_id": "..."}`.

## [0.1.0-alpha.2] - 2026-08-22

### Highlights

- Desktop can now run locally against models from `settings.yaml`, while a
  signed-in session still provides managed models and team work from a BuildMax
  deployment.
- Private deployments gain a System Administrator surface, account controls,
  deployment health and configuration views, model administration, audit
  search and export, retention controls, and quota warnings.
- Task runs can use operator-approved models through the managed LLM gateway,
  publish durable artifacts, expose model-call and trace details, and be
  stopped or retried from Portal.
- Issues now support comments and a two-level sub-issue hierarchy; agents and
  workflows keep append-only revision histories and pin the definitions used
  by a run.
- Plugins can contribute skills, agents, MCP servers, and hooks from local
  directories or an immutable private marketplace, with source policy and
  management surfaces in Portal and Desktop.
- The local agent loop gains queued mid-run input, parallel read-only tool
  calls, durable notes and tasks, provider-native model protocols, reasoning,
  prompt caching, and image results from MCP tools.

### Upgrade notes

- Every worker API route now prefers a short-lived credential scoped to one
  task run. The deprecated deployment-wide `worker.token` remains accepted for
  this release with a warning so a rolling upgrade can finish; it is scheduled
  for removal in the next release.
- MCP tools without a true `readOnlyHint` are now treated as writes. Interactive
  CLI/TUI and Desktop sessions ask before write tools run; autonomous surfaces
  refuse calls that require a person to approve them. Review MCP annotations
  and `tools.permissions` before upgrading unattended workloads.
- Database migrations remain forward-only and are applied at server startup.
  Back up the database before upgrading; rolling back the database schema is
  not supported.
- Native Desktop bundles are still not published by the release workflow. The
  tagged source contains Desktop local mode, while GitHub Release artifacts
  contain the CLI and server binaries.

### Added

- System Administrators get an account API under `/api/admin`: list and search
  accounts, inspect one with its teams, roles, and live session count, create
  one, issue a login code, revoke every session, and grant or revoke the
  administrator role itself. Every one of those is recorded in the audit trail
  against the administrator who did it. The API refuses to revoke the
  deployment's last grant — that is what the operator command is for — and an
  administrator cannot disable their own account.

- `GET /api/admin/audit-events` searches the audit trail across every team,
  filtered by team, actor, action, and time window. It is the only way to read
  the events that have no team at all — logins, administrator grants, account
  actions — which the team-scoped trail could never return; ask for those with
  `team_id=none`. `GET /api/admin/teams` and `/api/admin/teams/{team_id}` list
  teams with their size, quota tier, and usage, and name their members and
  roles. Both are metadata: an administrator learns that a team exists and how
  large it is, never what is in it, and reaching a team's own resources still
  requires membership.

- The administration area has a Models page, and `GET /api/admin/llm/models`
  behind it, showing which upstreams the deployment will call and which aliases
  point at each one — a model that is enabled with no alias is unreachable by
  every team, which is the most common reason an operator's model appears not
  to work. Models can be retired and restored from there. Adding one stays
  `buildmax-server model add`: it carries a provider credential, and doing that
  over HTTP puts the key in a request body, a proxy log, and whatever the
  browser did with the form.

- `GET /api/admin/system` and `GET /api/admin/config` report what a deployment
  is doing: version, applied schema migrations, dependency health as `/readyz`
  sees it, worker run mode and model transport, task runs by status, and the
  effective `server.yaml` with every credential reduced to whether it is set.
  Not its length, not a prefix — presence only. The configuration view also
  computes warnings for states that are trade-offs elsewhere in the project:
  self-registration left open, the deprecated shared worker token still set,
  managed worker inference with no aliases, local-process run mode, a run
  timeout that outlives its run token, and run output on local disk.

- Agents and workflows keep a numbered, append-only history. Every edit records
  the definition it produced and who wrote it, `GET
  /api/teams/{team_id}/agents/{agent_id}/revisions` and the matching workflow
  route read it back, and `POST .../revisions/{revision}/restore` writes an
  earlier version's content back — which appends a new revision rather than
  erasing the ones after it. Saving without changing anything records nothing.
  Restoring a workflow leaves its `draft`/`published`/`archived` state alone, so
  bringing back an old definition cannot unpublish a workflow a team is running,
  and the definition is revalidated so a version whose agents were since deleted
  is refused rather than restored into a plan that cannot run. A workflow run
  records the workflow revision it expanded and each step records the agent
  revision it ran under. Existing agents and workflows are given a revision 1
  holding their current content on upgrade. Portal shows the history on the
  workflow page and on each agent, with restore in place.

- The audit trail can be kept for a fixed window. `audit.retention_days` in
  `server.yaml` expires events older than it; the default is 0, which keeps
  everything, because a deployment that never chose a retention policy has not
  decided to discard evidence. Every sweep that removed anything records an
  `audit.pruned` event naming the range and the count — a trail that begins
  partway through says that policy shortened it, rather than leaving a reader
  to wonder whether somebody truncated it. Nothing else deletes an audit event,
  and there is no way to delete a particular one.

- The audit trail can be downloaded. A space owner exports their own from space
  settings, a System Administrator exports the deployment's — under the same
  filters as the search, including the events that belong to no space — from
  `#/admin`. Both come as CSV or JSONL. An export is itself recorded as
  `audit.exported` with the number of events that actually left, because
  reading the whole record is an action on it; an administrator's export
  narrowed to one space is recorded in that space's trail too, so its owner can
  see that the deployment read it.

- The desktop app can be used without a BuildMax server. Its sign-in screen now
  offers local mode, which runs the agent on this machine against the models in
  `settings.yaml` — the same thing the CLI does without `buildmax login`.
  Signing in is still there, and still what adds managed models and a team's
  work; the choice is remembered, and either one can be left from the sidebar.

- Issues have a comment thread. Team members can comment, edit their own
  comments, and delete their own; team owners can delete any. Deletion is
  permanent. When an agent run finishes on an issue it posts one comment with
  what it reported and a link to the run.

- Read-only tool calls from the same model message now run at the same time.
  When the model asks for several files or searches at once, they no longer
  queue behind each other -- three 100ms reads take about 100ms rather than
  300ms, and `WebFetch` batches gain the most. Only calls a tool declares
  read-only overlap: writes, shell commands, `Task`, and MCP calls still run
  alone and in order, calls are never reordered, and the message history a run
  produces is identical whatever the limit. Tune with `agent.max_parallel_tools`
  in `settings.yaml` (default 4, range 1-16; 1 restores one-at-a-time).

- Plugins: a directory under `~/.buildmax/plugins` holding `skills/`,
  `agents/`, `mcp.json`, and `hooks.yaml` now loads on the next run, so a
  team can share a workflow by cloning one repository. `buildmax plugin
  list`, `status`, `validate`, `enable`, and `disable` show what each one
  contributes, which checkout it came from, and what overrode it.

- Private plugin Marketplace: a System Administrator publishes a plugin
  directory with `buildmax plugin publish`, and anyone signed in to that
  deployment installs it by name with `buildmax plugin install`. Releases are
  immutable and identified by version and SHA-256 digest, which the client
  verifies against the catalog before anything is unpacked.

- Operators can restrict where plugins may come from with a
  `plugins.allowed_sources` block in `policy.yaml`. A plugin an operator
  excluded shows as `refused` with the reason rather than disappearing, and a
  directory whose provenance cannot be established does not pass a policy that
  names one.

- Portal and Desktop manage plugins: an administration section for the
  deployment's catalog and its releases, a browsable list of what is published
  with the command that installs it, and a Desktop panel showing what a
  project's runtime loaded, with install, update, disable, and remove.

- Portal has an administration area at `#/admin`, visible only to a System
  Administrator: deployment health and version, accounts with disable, enable,
  login-code and session controls, spaces with their size and usage, and the
  deployment-wide audit trail. It is a separate area from space settings rather
  than another tab in it, because authority over the deployment is not authority
  inside a space — every page says what it does not show, and there is no link
  from a space into its contents.

- A message typed while the agent is working is now queued instead of refused,
  on the CLI/TUI, Desktop, and Portal alike. Up to ten can wait per
  conversation; past that the message is refused rather than the oldest being
  dropped, so nothing the user believes is scheduled disappears. In the TUI the
  input stays visible and editable during a run, `Enter` queues, and `Esc` takes
  the last queued message back; Portal and Desktop show what is waiting at the
  end of the thread. Stopping a run discards everything queued behind it — those
  messages were written for work that has just been called off. A run that
  *fails* keeps the queue, and each message still gets its turn.

- On the CLI/TUI and Desktop a queued message no longer waits for the whole run.
  The agent picks it up at its next step — as soon as the tool batch it is
  running finishes — so a correction reaches the model while the work it
  corrects is still in progress, instead of after it. Such a message passes the
  same `UserPromptSubmit` hook as any other prompt, and appears in the run trace
  as `user_input`: a message that entered a run after it started is part of what
  that run was told to do. Portal keeps delivering a queued message as its own
  turn — its foreground turns are short, and its queue is shared by every client
  watching the conversation.

- Quota now says something before it starts refusing work. Passing 80% of a
  space's run or token limit records `quota.threshold_reached`, and work
  refused at the limit records `quota.exceeded` — at most once per limit per
  period, so a space that keeps submitting does not fill its own trail with
  retries. The actor is the deployment, not whoever submitted the work that
  tipped the total over. Portal states the same thing in space settings, and a
  space with no limits reports nothing rather than reporting comfort.

- A finished task run can be run again. Portal's Issue Detail shows **Retry Run**
  once a run is over, backed by
  `POST /api/teams/{team_id}/tasks/{task_id}/retry`. The retry carries the same
  input the previous run had, so recovering from a worker that died or a model
  that timed out no longer means retyping the instructions — and no longer
  invites retyping them slightly differently. The new run records which run it
  repeats, and the run it repeats is left exactly as it was, so what went wrong
  stays readable next to the attempt that followed it. A run still in flight is
  not retried but stopped first; a task that has never finished a run has
  nothing to repeat; and a workflow step is refused, because its workflow
  advances from that step's outcome and a run started outside it would report a
  second one.

- Portal's **Run details** now shows the managed model calls a run made, beside
  the trace the run wrote itself. The ledger has had a route since it became
  readable, and nothing displayed it: the two records answer different
  questions, and the difference is the point. The trace is what the agent did;
  this is what the deployment was asked to serve, on which operator-approved
  alias, and it is what a team's quota is computed from. An empty list is not
  reported as "spent nothing" — a run whose trace shows model calls that the
  ledger never saw is a run that reached a provider directly, and it says so.
  A call whose provider reported no usage is counted as unreported rather than
  as zero.

- Issues can be broken down into sub-issues. An issue may have one parent in the
  same team, the hierarchy is two levels deep, and a parent shows how many of its
  sub-issues are done. Sub-issue status is never rolled up into the parent —
  closing a parent with open sub-issues is allowed and warned about, not blocked.

- Durable traces now record each subagent run and link it to the immediate
  parent run, so a delegated run can be traced in either direction.

- A deployment can now have System Administrators: an authority over the
  deployment itself, held by an account and separate from every Team role.
  `buildmax-server admin grant | revoke | list` manages it on the machine that
  already holds the database credentials, which is also what recovers a
  deployment that has lost every administrator — there is no configuration
  value and no break-glass credential. A grant carries no access to any team's
  issues, conversations, artifacts, files, or run traces; those stay behind
  team membership. Granting, revoking, and — new — account creation, password
  setting, and login-code issuance from `buildmax-server user` are all recorded
  in the audit trail, which those three commands previously wrote nothing to.

- A team can keep durable files as artifacts. Upload one to
  `/api/teams/{team_id}/artifacts` and it gets a stable `ar_` reference that any
  member can open, preview, or download at `/api/artifacts/{artifact_id}`,
  wherever they saw the reference — the ID is the address, and no team appears
  in it. Content is immutable, deletion takes effect immediately at the
  authorization boundary, and a caller who is not in the owning team cannot tell
  a real artifact from one that never existed. `storage.max_artifact_mb` caps a
  single upload; it defaults to 100 MB.

- Agents can publish a finished file with the `UploadArtifact` tool and cite the
  `ar_` reference in their answer. It appears only where there is a server to
  publish to — a logged-in CLI or Desktop session, or a worker running a team's
  task — so a session running straight against a model provider is not offered
  a tool that could only fail. Nothing is uploaded automatically: the agent
  names the one file, which is what keeps `.env` files, caches, and intermediate
  output out of a team's artifacts. What a run publishes shows up on its issue
  alongside the run's own output.

- Task runs can reach models through the managed LLM gateway instead of calling
  a provider themselves. Set `worker.llm.transport: buildmax` in `server.yaml`,
  optionally with `worker.llm.alias`, and a worker no longer receives
  `BUILDMAX_CONVERSATION_MODEL_API_KEY` at all — it reaches operator-approved
  models through the server. The server states the transport and alias in the
  run's worker-API response, so a worker never chooses its own model, and it is
  told nothing else about it: the endpoint, the upstream model identifier, and
  the credential stay server-side. Direct mode is unchanged and remains the
  default; there is no automatic fallback between the two, because a server
  outage must not silently redirect governed traffic to a personal provider key.
  A deployment that asks for managed runs it cannot serve — `transport:
  buildmax` with no `llm.aliases`, or an alias no team may call — now fails at
  startup rather than at every run's first model call.

### Changed

- Deleting an agent no longer removes the record. Tasks, workflow step runs, and
  revisions all name an agent by ID, so dropping the row left dangling
  references and broke any workflow run still in flight at its next step. The
  agent is now marked deleted: it disappears from listings and from everything
  that starts new work — a run cannot be started with it, an issue cannot be
  assigned to it, and a workflow definition naming it is refused — while records
  that already refer to it still resolve and an in-flight run finishes on the
  snapshot it started with. Deleting an agent a *published* workflow still names
  is refused with `409` naming those workflows, so the breakage surfaces at the
  delete rather than at the next run; draft and archived workflows do not block
  it. There is no undelete: the row exists so references resolve, not as a
  recycle bin.

- The TUI footer says how the current model is reached — `direct` for a provider
  called from this machine, `buildmax <host>` for one reached through a BuildMax
  deployment. Transport is a property of the model entry and `/model` switches
  entries mid-session, so the answer now sits next to the model name instead of
  only in `buildmax models`.

- Log records name their subsystem in a `component` attribute instead of a
  prefix on the message. Four background loops in the scheduler alone had spelled
  it four different ways, one of them not at all, so no filter selected a
  subsystem. Levels follow one rule now — Error means a unit of work failed or
  was lost, Warn means degraded but finished — which makes a threshold select
  something. A worker stamps its run id onto every record it writes, including
  those from the agent loop and the tools.

- MCP calls are gated by what the server says about the tool. `CallMcpTool` now
  reads the `readOnlyHint` annotation from `tools/list`, which BuildMax fetched
  and discarded until now: a tool the server advertises as read-only runs
  unprompted, and anything else asks. **This tightens autonomous surfaces**, the
  one behavior change there — a task run, print-mode run, or Portal conversation
  calling an MCP tool that is not advertised as read-only is now refused rather
  than run silently. A server that omits the annotation is treated the same as
  one that says `false`, because the protocol cannot distinguish them. The hint
  decides whether BuildMax asks; it never grants trust.

- The server answers every request with an `X-Request-Id` header, and its logs
  now record each request when it finishes rather than when it starts — with the
  status, the duration, and that same id. A bug report can name the request, and
  every log line the request produced can be found by that name. The log level
  follows the status, so filtering at Error selects server faults without
  sweeping up refused requests.

- Tool permissions are configurable. A `tools.permissions` block in
  `settings.yaml` sets any tool to `allow`, `ask`, or `deny`, keyed by tool name
  or by the target it dispatches to (`CallMcpTool:github/*`), most specific rule
  winning. `allow` turns off the category prompt but not the safety checks — a
  sensitive path and a risky shell command still prompt, and only `deny`
  outranks them. `ask` means a person must look, so on a surface with no person
  the call is refused rather than run. `buildmax tools status` prints every
  tool's classification, its resolved action, the layer that decided it, and any
  rule ignored for an unrecognised action; `/tools` in the TUI marks tools that
  do not simply run.

- A worker reports what the server actually said when a call to the worker API
  fails, instead of only the HTTP status. A run that could not be claimed or
  patched used to log "500 Internal Server Error" and discard the sentence
  explaining why.

- Tools now classify what a call does, and the ones that write ask before they
  run. On the CLI TUI and Desktop, `Write`, `Edit`, `Task`, and `CallMcpTool`
  request approval where they previously ran unannounced; read-only tools are
  unchanged, and `Bash` keeps its own risk classifier as the authority. Surfaces
  with nobody to ask — print mode, workers, eval, and Portal conversations —
  behave exactly as before: the category prompt is only raised where a person
  can answer it, so a task run's file writes and shell commands are unaffected.
  The prompt itself now offers three answers rather than two — allow once (`y`),
  allow for the rest of the session (`a`), or deny (`n`) — and a session grant
  covers the tool by name, or the specific `server/tool` for an MCP call.
  Grants are held in memory and are gone when the process exits.

### Fixed

- Adding a member to a team on a deployment with no user store answers 503
  rather than 500. Every other "not configured" answer in the API is 503; this
  one was the outlier.

- A run's system prompt can be added to: free text appended as its last layer,
  after the runtime prompt and both `AGENTS.md` files. It is additive rather
  than a replacement, and because it lives in the system prompt it is sent with
  every model call instead of fading as the conversation is compacted. On the
  command line it comes from `--append-system-prompt`,
  `--append-system-prompt-file` (preferred for anything long or private, since
  an argument is readable by every process on the machine), or `--agent NAME`
  for the body of a definition under `.buildmax/agents/` — which supplies prompt
  text only, and does not switch the model or restrict tools. The text is capped
  at 8192 characters and an over-limit value is rejected rather than truncated.
  An `## Invariants` section within it is also restated at the end of every
  request. Resuming a session without one of these flags keeps the text it
  already ran under.

- A long session no longer loses its earliest context outright. Each compaction
  summarized only the messages it was discarding and then replaced the stored
  summary, so after the second compaction everything the first one covered was
  gone rather than condensed — which is why a long-running agent could forget
  what it had been asked to do. A compaction now summarizes the previous summary
  together with the newly discarded messages, so each summary subsumes the one
  before it. The stored summary is also bounded relative to the context window,
  since it lives in the system prompt, which is re-sent in full on every call and
  is never trimmed.

- Compaction summaries are better targeted. The summarizer is now shown the
  session's live notes and open tasks and asked to spend its detail on what
  bears on them, compressing what is already settled. It previously had no
  signal about what was still open.

- The desktop app no longer opens on a blank window. Its frontend bundle shipped
  two copies of React — one reached through the symlinked `@buildmax/gui`
  package — so the first hook of the first render threw and nothing was drawn.

- Cancelling a Desktop run while a tool approval prompt was showing left the
  project stuck. The run goroutine waited forever for an answer nobody would
  give, so its cleanup never ran and every later message was refused with "a run
  is already in progress". Approval prompts now return when the run is
  cancelled.

- The Desktop sign-in form shows the Server URL field instead of hiding it
  behind a collapsed "Server" toggle, and fills it from `settings.yaml`'s
  `server_url` the way `buildmax login` already did. A deployment that does not
  answer on `http://localhost:5678` — the kind cluster serves Portal and API
  from `http://localhost:8080` — is now reachable without hunting for the field.

- The system prompt no longer carries the compaction summary twice. Both the
  caller and the agent loop appended it, so every run after its first in-run
  compaction sent two copies.

- A failed run keeps the reason its worker reported. The worker records the
  failure and then exits non-zero, and the scheduler used to overwrite the
  record with the process error — so "context deadline exceeded calling the
  model" became "exit status 1". The scheduler now leaves a run alone once it
  has reached a terminal status.

- Signing in with a single-use login code no longer spends the code when the
  email address does not match it. The code was redeemed before the address was
  checked, so one mistyped or autofilled address burned it: the retry with the
  right address failed too, and an operator had to issue another code. The
  address is resolved first and the code is only redeemed for that account, and
  the server now logs why a code was rejected — the response still says nothing
  more than `invalid otp`. Whitespace around a pasted address or code is also
  ignored now, rather than failing the same opaque way.

- An MCP server that returns an image — a screenshot, a rendered chart — now
  sends the model the image instead of a base64 blob pasted into the tool
  result. Set `vision: true` on a model that can read images, or `--vision` on a
  catalog model. Left off, which is the default, the tool result says what came
  back (`(image: image/png, 43.2 KB)`) and the image is not sent, because a
  model without image support rejects such a request rather than ignoring it.

- A model entry can now name the wire protocol its endpoint speaks, so BuildMax
  reaches a provider's own API instead of only OpenAI-compatible gateways. Set
  `provider` on a `settings.yaml` model — `openai_compatible` (OpenAI Chat
  Completions, the default), `openai` (OpenAI's Responses API), or `anthropic`
  (the Anthropic Messages API) — and optionally `max_tokens` to cap one
  response. The value names a protocol, not a vendor: Claude through OpenRouter
  is `openai_compatible`, and Claude from `api.anthropic.com` is `anthropic`. An
  operator can serve the same three from the managed gateway with
  `buildmax-server model add --provider --max-tokens`, and `model list` now
  shows each row's provider. Existing configuration is unchanged: an entry that
  names no provider keeps calling what it always called.

- A model can now reason before it answers, and keep that reasoning across tool
  calls. Set `reasoning: low`, `medium`, or `high` on a `settings.yaml` model,
  or pass `--reasoning` to `buildmax-server model add`, and an `anthropic` model
  uses extended thinking at that effort while an `openai` model keeps its
  reasoning between turns. `openai_compatible` has no such state and ignores the
  setting. It is off by default, because it changes what a call costs and older
  models reject it. The reasoning itself never appears in the transcript:
  BuildMax stores it beside the assistant message and sends it back unread, and
  a session continued against a different provider drops what that provider
  cannot use rather than failing. CLI sessions, Portal conversations, and
  managed gateway calls all carry it, so a run keeps its continuity across a
  restart.

- Notes are now saved at the moment they would otherwise be lost. Before a
  compaction discards messages, the agent gets a bounded turn — with only the
  note and task-list tools in reach — to move anything it still needs out of
  the material about to go. Until now a note existed only if the agent
  remembered to write one, which it is least likely to do exactly when the
  context is filling up. A write rejected for exceeding the note limit earns
  one correction, since this is the last moment the material exists. A failed
  checkpoint is logged and the compaction proceeds.

- A Portal agent's instructions now reach the agent through its system prompt.
  They were
  rendered into the task input, which is a message like any other, so a long
  run compacted them away — and a task created with its own input dropped them
  entirely. The server resolves them per run, so editing an agent takes effect
  on its next run, and they travel in the worker API response rather than on
  the worker's command line, where every process on the machine could read
  them.

- Prompt caching is now available with `prompt_cache: true` on a model, or
  `--prompt-cache` on a catalog model. For `anthropic` it places cache
  breakpoints around the tool definitions and system prompt, which do not change
  between calls in a run; the OpenAI protocols already cache on their own. It is
  off by default because a cache write costs more than not caching and only pays
  back over several calls. Every provider now reports `cache_read_tokens` and
  `cache_write_tokens`, and a managed deployment records both on the call
  ledger. They break the prompt count down rather than adding to it, so a spend
  report must not sum them alongside it.

- The server creates the database named by `database.name` when the MySQL it
  connects to does not have it, instead of refusing to start. It is attempted
  only after the connection failed for that reason; an account without `CREATE`
  rights gets an error naming the statement to run by hand.

- The agent keeps durable session notes. A new `NoteWrite` tool records short
  entries — decisions and why they were made, approaches already ruled out,
  constraints stated once — and `TodoWrite` now stores its list instead of only
  formatting it. Both are kept on the session rather than in the conversation,
  so they survive the compaction that eventually discards the messages that
  produced them, and both are shown to the agent on every turn. Each call
  carries the complete list and replaces what was stored, which is what forces
  the agent to drop entries it no longer needs; notes are capped at 15 entries
  of 200 characters and an over-limit call is rejected with an explanation. A
  session that writes neither carries nothing extra. A subagent writes to its
  own list and cannot overwrite the one belonging to the run that delegated to
  it.

- A task run whose worker never reported an outcome no longer stays `SCHEDULED`
  or `RUNNING` forever. Only the worker moves a run out of those states, so an
  evicted pod, a killed process, or a run outliving its credential left work
  that Portal showed as in progress and nothing would ever close. The server now
  records such a run as failed after `worker.run_timeout` (6h by default), with
  an error that names the timeout rather than guessing which of those happened.

- A run trace now records which system-prompt layers the run loaded and how
  large each was, so a finished run can say what it was told before the
  conversation started.

- Portal can show a run's trace on deployments that keep run state on local
  disk. `local_fs` is the default persist backend, and it deliberately stores
  no copy of a run's global files — the worker has already written them under
  `workspaces_dir`. The trace endpoint asked only the backend, so it answered
  "this run's trace is no longer in storage" for every run on every default
  deployment, including the Compose quickstart, while the file sat on disk the
  whole time. It now falls back to disk, as the task conversation endpoint
  already did. Deployments backed by S3 were unaffected.

- Refreshing credentials now retries a short-lived Windows file-sharing
  conflict while atomically replacing `auth.json`. Without that retry, a
  concurrent caller could read the old refresh token after a successful
  exchange and spend it a second time.

- On Windows, a command that read the saved login while another one was renewing
  it could fail with "the process cannot access the file because it is being
  used by another process". Replacing `auth.json` is a rename, which a
  concurrent reader survives on macOS and Linux but not on Windows; reads and
  the replacement are now serialized within a process, so a run that renews its
  token no longer trips another caller in the same binary.

- A workflow run now uses the agent definitions it started with. Steps are
  dispatched one at a time as the previous step's task run finishes, and each
  dispatch re-read the agent, so editing an agent while a run was in flight
  changed what its later steps sent to the model — a run could execute two
  different versions of the same agent, with nothing recording that it happened.
  Each step run now stores the agent name, description, and instructions as they
  were when the run started, and dispatches from that copy. Runs already in
  flight when this ships keep the old behavior, since their steps carry no
  snapshot. The captured definition is returned with the workflow run detail and
  shown per step in Portal.

- `--workspace` now applies to the TUI. The flag reached the `--agent`
  definition lookup but never the agent itself, so file tools, `AGENTS.md`, and
  the footer's git branch all ran against the current directory while `--agent`
  resolved somewhere else — one run with two ideas of where it was. Print mode
  was never affected.

### Security

- An account can be disabled, and disabling it stops access immediately rather
  than when a token expires. Every credential the account holds is refused:
  password, login code, refresh token, the access token it is already carrying,
  and its webhook keys. Live sessions are revoked at the same time, and work the
  account queued but that has not started fails at dispatch instead of running.
  Previously there was no way to stop an account short of editing the database,
  and an issued access token stayed usable for its full lifetime — seven days by
  default. Disabling is not deletion: nothing is removed, and enabling reverses
  the state and nothing else.

- Each task run is now dispatched with its own gateway credential rather than
  sharing the deployment-wide worker token. The scheduler mints a short-lived
  run token naming the run's user, team, and task, delivers it in
  `BUILDMAX_RUN_TOKEN`, and the managed inference route accepts nothing else —
  a run token presented against another run's URL is refused, and so is the
  shared worker token. Every managed call a worker makes is therefore recorded
  in the `llm_call` ledger against a user as well as a team, which a shared
  secret could never support. Lifetime is `worker.run_token_ttl`, 24h by
  default; it is not renewable, so it must outlast your longest run. A run token
  cannot be used as a user login, and a user access token cannot be used as a
  run token. The token is cleared from the worker's environment once read, so
  model-chosen shell commands cannot print it — the sandbox that would otherwise
  strip it is off by default.

- `GET /api/teams/{team_id}/task-runs/{task_run_id}/llm-calls` lists what a run
  spent and on which approved alias. The ledger recorded this from the day it
  existed and had no route, so reading it meant querying the database —
  diagnosing a run should not require the database password. It carries no
  prompts or generated content, and omits the catalog entry an alias resolved
  to, which is the operator's routing rather than the team's.

- The run token now authenticates **every** `/api/worker/*` route, not only
  managed inference, so a run can read and write its own record and nothing
  else. The deployment-wide `worker.token` could name any run, which meant any
  worker could read the prompt text of every team's tasks, `PATCH` another
  team's run to SUCCEEDED with output it invented, or push deltas into another
  run's live stream. It is still accepted for one release, with a deprecation
  warning, because a server that has not restarted yet dispatches workers
  without minting a token; the next release removes it. Since every route needs
  one, every dispatched run is now given a token, direct-mode runs included.
  `buildmax-server run-token <task_run_id>` mints one for driving a worker route
  by hand, which is what copying the shared secret used to be for.

- A task run in flight can be stopped. Portal's Issue Detail shows **Stop Run**
  while an agent task is pending or running, backed by
  `POST /api/teams/{team_id}/tasks/{task_id}/cancel`. A run nobody has picked up
  yet ends immediately as `CANCELED`. A run a worker is already executing is
  asked to stop: the worker notices within seconds, ends its agent loop, uploads
  what the run produced, and reports `CANCELED` — so a canceled run keeps its
  output and artifacts instead of throwing away the work already done. The run
  records who asked and when. Nothing is left hanging if the worker is gone:
  the server finishes a run whose cancel goes unconfirmed for two minutes, which
  is the same sweep that closes abandoned runs. Cancelling twice while a run is
  stopping is not an error, and a canceled task can be run again.

## [0.1.0-alpha.1] - 2026-08-17

### Security

- The server pod is now created with the same containment as worker pods:
  non-root with an explicit uid, no added capabilities, no privilege
  escalation, `RuntimeDefault` seccomp, and a read-only root filesystem with a
  writable `/tmp`. It is applied in the local kind manifest as well as the
  production reference, so `./make kind up` exercises it rather than leaving it
  a setting that only appears in a file nobody applies.

- Task-run workers are no longer handed the JWT signing secret or the database
  password. Both reached workers by inheritance — a local worker got the
  server's whole environment, and a Kubernetes worker Job got every `BUILDMAX_*`
  variable the server held — even though a worker reads neither: it talks to the
  server over HTTP with its own worker token and never opens the database. Since
  a worker executes model-chosen shell commands, holding the signing secret
  meant a prompt could mint a token for any user, and holding the database
  password meant it could read every team's data. A worker now receives only the
  variables marked `WorkerNeeds` in `internal/config/env_spec.go`, and an
  unrecognized `BUILDMAX_` variable is withheld by default. Object-storage keys
  are still passed, because workers read and write run state directly; narrowing
  that needs a server-issued, run-scoped credential and is separate work.

- Worker Job pods are now created confined: non-root with an explicit uid, no
  automounted service-account token, all capabilities dropped, `RuntimeDefault`
  seccomp, and a read-only root filesystem with a writable `/tmp`. None of it is
  configurable, because a worker runs model-chosen shell commands. `run_as_user`
  under `worker.k8s` covers clusters that assign their own uid ranges.
  `worker.k8s.resources` adds optional CPU and memory bounds; leaving them unset
  keeps existing deployments unbounded rather than handing them a limit nobody
  chose. `local_process` mode is unchanged and remains a development path.

- The managed inference routes now authenticate before checking whether a
  gateway is configured. They were the only team-scoped routes that answered
  `503 managed inference not configured` to an anonymous caller, which told
  anyone who asked whether a deployment offers managed models.

- Signing in now returns two credentials instead of one. The access token is
  still a signed JWT the server keeps no record of; alongside it comes a refresh
  token, stored as a hash in the new `user_refresh_token` table and exchanged at
  `POST /api/token/refresh` for the next pair. A single 24-hour token meant two
  things at once: nothing could retire it early, and everyone had to sign in
  again every day — which, with no mail channel, meant an operator issuing a
  login code by hand each time. Splitting them separates those questions.
  `access_token_ttl` (7 days) is now the only thing that governs how long a
  leaked token works, and `refresh_token_ttl` (30 days) governs how long a
  session can be renewed without a new login code.

  Each exchange spends the token presented and issues its replacement in the
  same session, so a token appearing twice means two holders. Past
  `refresh_rotation_grace` that revokes the whole session and records an
  `auth.refresh_reuse` audit event — the legitimate holder is signed out too,
  because there is no way to tell the copies apart. The grace window exists
  because the CLI and Desktop share one credentials file across processes, where
  two simultaneous refreshes are ordinary rather than suspicious.

  Every login opens its own session, and `POST /api/logout` revokes one of them
  without touching the others. Expired login codes and refresh tokens are now
  swept hourly; the sweep for login codes had been written but never wired up.
  Existing tokens keep working until they expire, so upgrading does not sign
  anyone out.

  Every client renews rather than expiring. The Portal refreshes when a call
  comes back `401` and replays it, sharing one exchange between requests that
  fail together — several refreshes of the same token would read as a replay
  and revoke the session. Its WebSocket asks for a fresh token before each
  connect, because a rejected upgrade arrives as a close event with nothing to
  read. The CLI and Desktop renew inside `TokenForServer`, so `~/.buildmax/
  auth.json` now holds both credentials and is written atomically; `buildmax
  logout` revokes the session on the server rather than only forgetting it
  locally. Being offline never discards a session — only the server rejecting
  the refresh token does.

  Managed model clients now read the credential per request instead of at
  construction. A client is built once and cached for the life of the process,
  so a token captured there was fine until it expired and useless afterwards,
  with no way back short of a restart.

- People sign in with an email address and a password, hashed with argon2id and
  a per-account salt. Until now the only credential was an operator-issued
  login code, which meant every sign-in on every device went through a person —
  workable for a demo, not for a small team that will not stand up an identity
  provider. Passwords are the one credential that needs no delivery channel,
  which is why they come before SSO rather than after it.

  Login codes keep their job and lose the wrong one: they are now the recovery
  path, not the everyday way in. A code claims a new account or replaces a
  forgotten password, and `POST /api/password` sets one from a signed-in
  session. Changing an existing password requires the current one — a session by
  itself must not be enough, because an access token cannot be revoked before it
  expires and allowing it would turn a stolen token into a permanent takeover.
  Setting the *first* password needs only the session, which came from a code an
  operator issued by hand.

  The minimum is twelve characters and there is no composition rule, because
  "one digit and one symbol" pushes people toward short predictable passwords
  that satisfy it. Every failed password login answers with the same sentence
  and does the same hashing work whether or not the address exists, so the form
  cannot be used to ask who has an account. `user.password_set` and the
  credential each login used now appear in the audit trail.

  **Login is not rate limited.** A reachable server can be brute-forced online;
  the length minimum and a memory-hard hash raise the cost per guess but are not
  a substitute for throttling. Unified rate limiting is separate, planned work.

- `dev_login_otp` is removed, along with `BUILDMAX_DEV_LOGIN_OTP`. It was a
  fixed code that authenticated every registered account — a standing
  authentication bypass kept for the convenience of clicking through the Portal
  locally. A password does that job with no bypass:
  `buildmax-server user set-password dev@local` reads one from stdin, so it
  lands in neither shell history nor the process list. A `dev_login_otp:` left
  in `server.yaml` is ignored, as any unknown key is — the bypass is gone
  either way, but remove the line so nobody reads it as still doing something.

- The Portal's sign-up page is gone. It collected an email address, told the
  person a code had been sent, and sent nothing — BuildMax has no mail channel.
  `allow_signup` still works for the API, and still only creates an account that
  needs an operator-issued code before anyone can use it, which is why no form
  offers it.

### Added

- `deployment/production/`, a private deployment reference for a cluster that
  already runs its own MySQL, object storage, ingress, and certificates. One
  plain-YAML manifest and a README stating the contract each dependency has to
  meet — DDL privileges, `utf8mb4`, a dedicated bucket, one origin for Portal
  and API. It is written to be read and adapted rather than applied: every
  dependency address is a placeholder, so an unedited `kubectl apply` fails
  instead of coming up against the wrong database. Deliberately not a chart or
  a kustomize base, so it converts to whatever a cluster is already managed
  with. Nothing applies it, so `internal/architecture` parses its ConfigMap the
  way the server parses its own config and asserts the settings that make it a
  production reference rather than a copy of the development stack.

- A deployment can now point BuildMax at a database and an object store it
  already runs, which the connection layer previously could not express.
  `database.tls` carries a TLS mode into the DSN, defaulting to `preferred` —
  TLS whenever the server offers it, unverified — so an existing plaintext
  connection keeps working while every server that supports TLS gets it; set
  `true` for a managed database that should be verified. An empty
  `storage.minio.endpoint` now means AWS S3 and lets the SDK resolve the
  regional endpoint, instead of forcing a base endpoint at every store, and
  bucket addressing follows from that rather than being pinned to path style,
  which AWS S3 has not supported for buckets created since 2020;
  `storage.minio.path_style` overrides it. Leaving both storage keys empty
  falls through to the AWS SDK's default credential chain, so a pod can reach a
  bucket through IRSA, workload identity, or an instance profile rather than a
  long-lived key the deployment has to store and rotate.

- Versioned schema migrations. Changes that `AutoMigrate` cannot express — a
  backfill, a drop, a rename — are now an ordered list recorded in a new
  `schema_migration` table, so each runs at most once per database instead of
  probing `information_schema` on every server start forever. The two existing
  one-time migrations became the first two entries and are recorded on upgrade.
  The schema moves **forward only**: `Migration` has no `Down` field, and the
  compatibility promise is that schema version N keeps serving code from
  release N-1 — so a removal takes two releases, one that stops using the
  column and one that drops it. A binary that meets migrations from a later
  release warns and continues, because that is the promise working rather than
  a fault. Rolling a database back is not supported; recovery from a bad
  upgrade is a restore from backup.

- `POST /api/worker/task-runs/{task_run_id}/llm/completions`, the managed
  inference entry point for workers — the last route needed before a task run
  can use operator-approved models without holding an upstream provider key.
  The team, task, and run are derived from server state, so the only thing
  taken from a worker is the prompt it wants answered. A call is accepted only
  while the run is executing: the worker token identifies a worker, not the
  owner of a particular run, so without that any token holder could spend a
  team's quota against a run that finished weeks ago. Nothing calls it yet —
  workers still receive a direct model entry, and switching them over needs a
  decision about which alias a worker resolves.

- `GET /readyz`, which reports whether the server can actually serve traffic by
  probing MySQL and object storage. `/healthz` keeps its old meaning and still
  checks nothing: the two exist because Kubernetes acts on them very
  differently — a failed readiness check stops traffic, a failed liveness check
  restarts the container — and pointing both at a dependency-aware endpoint
  would turn every database blip into a restart of a working server. The
  reference manifest now points readiness at `/readyz` and leaves liveness on
  `/healthz`. The response names the failing dependency but never the reason:
  the endpoint is unauthenticated and connection errors carry DSNs, endpoints,
  and bucket names, so the reason goes to the server log. The storage probe
  only reads, so a backend that accepts reads and refuses writes still reports
  ready.

- A `sandbox_boundary` record in every run trace, written immediately after
  `run_start`, carrying whether the run was sandboxed and — when it was — the
  mode, backend, and the settings/policy/env source chain that decided it. It is
  written for unsandboxed runs too, with an explicit `"sandboxed": false`:
  traces are about to become the basis for answering what confined a run, and a
  missing field would read as "nobody checked" rather than "nothing confined
  it". The Bash sandbox still defaults off on every surface, so most runs record
  `false` today.

- A **Run details** view in Portal, on any issue output produced by a task run.
  It reports the model, duration, model and tool calls, tokens, the files the
  run changed, each tool call with its duration or the reason it was denied,
  and the failure cause. Three things it refuses to leave implicit: a run
  nothing confined says so in words rather than by omission, a run that wrote
  no terminal record is marked as such instead of reading like a success, and a
  bounded tool list says how many calls it is hiding. A run whose trace was
  never recorded and one whose trace has left storage both explain which.

- `GET /api/teams/{team_id}/task-runs/{task_run_id}/trace`, which answers what
  a run used, touched, spent, why it ended, and what confined it — the model,
  the tool calls with their durations, the files it wrote, tokens, the terminal
  error, and the resolved execution boundary. It returns a summary rather than
  the raw trace: model output, tool arguments, and tool results stay in the
  file. A run whose trace was never recorded and one whose trace has gone
  missing from storage are both 404 but say which.

- Worker task runs now upload their run trace and record where it landed, in a
  new `task_run.trace_path` column. The trace was previously written inside the
  run-scoped `BUILDMAX_HOME` and discarded with the run directory:
  `uploadTaskGlobal` uploads a named allowlist, not the whole tree, so nothing
  carried it out. The path is recorded on failure as well as success, since
  diagnosing a failed run is what a trace is for.

- `docs/contribute/architecture/data-model.md`, a full reference for the server
  database: every one of the 18 tables with its columns, types, nullability,
  indexes, and enumerated values, two entity-relationship diagrams, and the
  procedure for adding, renaming, or dropping schema. The schema had only ever
  existed as GORM tags on unexported structs, so a newcomer had to read
  `internal/infra/db` file by file to learn the data model.

- `./make check ci`, which runs everything a pull request runs except the
  Windows job: `check all` plus workflow linting, a Git history secret scan, Go
  and npm production dependency license checks, GoReleaser configuration
  validation, and a Windows cross-build. The tool versions come from
  `.github/workflows/ci.yml`, and a test now fails when the task runner's pins
  and the workflow's disagree. For contributors who would rather spend a
  laptop's time than the repository's Actions minutes. `./make doctor` now also
  reports shellcheck, because actionlint drops its shell script pass without it
  and does not say so, and GoReleaser, whose version it compares against the
  one the workflows run since nothing in `go.mod` pins it.
- `./make kind status` and `./make compose status`, read-only summaries of the
  two local deployment paths. `kind status` prints the selected cluster and
  context, probes the Portal ingress, and lists nodes plus the Deployments,
  Jobs, and Pods in every namespace `kind up` installs. `compose status` lists
  every service, including exited ones, and probes the server and Portal ports
  on the host. Neither creates, builds, or generates anything, so a stack that
  was never started is distinguishable from an unhealthy one without reading
  full container logs.
- A managed model catalog in the new `llm_model` table, edited with
  `buildmax-server model add|list|enable|disable` on the machine that already
  holds the database credentials. Credentials are read by exactly one query, the
  one that builds a provider client, and never appear in a listing, an API
  response, or an error — but note that database backups now carry provider
  keys. An optional `llm` block in `server.yaml` maps team aliases to catalog
  models, and `conversation.model_target` runs Tier 1 on one. An alias naming a
  model that does not exist fails its own calls rather than stopping the server,
  because the catalog is edited independently of the policy. See
  `docs/design/llm-gateway.md`.
- A managed inference gateway: `GET /api/teams/{team_id}/llm/models` lists the
  aliases a team may use, and `POST /api/teams/{team_id}/llm/completions` runs
  one blocking call against an operator-approved model. Clients name an alias,
  never an endpoint, a credential, or a provider model identifier. Every call is
  recorded in a new `llm_call` ledger — identity, model, timing, outcome, and
  token usage, with no prompts or generated content. No BuildMax client uses the
  gateway yet, and a team already over quota is refused, which is accounting
  rather than a spending ceiling.
- CLI and Desktop can use a deployment's managed models. A `settings.yaml` entry
  with `transport: buildmax`, `server_url`, and `team_id` calls a BuildMax
  server instead of a provider, so no provider key sits on the machine; its
  `model` field is a team alias. The credential comes from `buildmax login` and
  is only sent to the server the login belongs to. `buildmax models` lists every
  configured model and where it sends prompts, and `--team` lists a team's
  aliases. The model picker in the TUI and Desktop names the destination for a
  managed entry, and `buildmax doctor` reports one that cannot authenticate.
  There is no automatic fallback between the two modes, and the login expires
  after 24 hours with no refresh yet. Workers and the evaluation harness stay
  direct.
- Managed calls can stream. `stream: true` answers with typed `delta`, `result`,
  and `error` events over SSE; abandoning the call cancels the provider request
  so it stops costing tokens. A call refused before any output is still a plain
  HTTP error, and one that fails after its first delta reports the failure as an
  error event. Repeating a `call_id` answers `409` naming the original call
  instead of running it twice. A reverse proxy in front of the server needs
  response buffering off for this route.
- A Compose stack under `deployment/compose/`: MySQL, the server, and the
  Portal, with a script that generates the secrets. `docs/deploy/compose.md`
  walks from nothing to a signed-in Portal.
- Single-use login codes. `buildmax-server user create` and
  `buildmax-server user login-code` let an operator create an account and issue
  a per-account, expiring code, which is how a deployment signs people in
  without a mail channel. Codes are stored hashed and redeemed atomically.

- The Portal is published as a container image,
  `ghcr.io/gougoujiang/buildmax-portal`, tagged with the release it was built
  from. A separate workflow builds it, so a frontend failure cannot hold up the
  binaries.
- `BUILDMAX_API_BASE` configures the Portal image's API URL at container start,
  which is what lets one published image serve any deployment.

- `buildmax init` writes a starter `settings.yaml`, optionally with the API key
  supplied on the command line.
- golangci-lint, govulncheck, and `-race` tests in CI, plus CodeQL analysis for
  Go and TypeScript that starts once the repository is public.
- ESLint for the Portal and desktop frontends, with `npm run lint` in each.
- Open source governance, support, maintainer, and conduct policies.
- Markdown, npm production license, configuration example, Git history secret
  scanning, and release snapshot checks in CI.
- SPDX SBOM generation, GitHub artifact attestations, and release image
  vulnerability scanning.
- Native Linux, macOS, and Windows release archive smoke checks, including
  checksum and required-content validation.
- A public launch checklist and documented issue triage policy.

- Contributor on-ramp documents: `docs/contribute/first-pr.md` walks clone to
  pull request, and `docs/contribute/conventions.md` collects the naming, entity
  ID, tool-output, commit, and changelog rules that review applies. Neither the
  build, the tests, the lint, nor the deployment smokes need a model API key —
  `CONTRIBUTING.md` now says so instead of listing one as a prerequisite.
- `.gitattributes` and `.editorconfig`, so line endings and editor defaults match
  what the toolchains already enforce, and the language statistics describe the
  project rather than its fixtures.
- `.buildmax/README.md` explains why this repository checks in its own workspace
  agent configuration when `.claude/` and `.vibe/` stay ignored.
- A contributor doctor, scoped `check` tasks, Node/npm/Wails version pins,
  fresh-clone CI, Desktop frontend tests, and an implementation-task issue
  template make both human and agent-assisted contributions reproducible.

- An audit trail for sensitive actions, in a new `audit_event` table with an
  owner-only `GET /api/teams/{team_id}/audit-events`. It records logins, team
  membership changes, model catalog changes, and refused team-scoped
  requests — who did what to which object, and nothing else: no prompts, no
  generated content, no tool output, no credentials. Run diagnostics stay in
  the durable run trace and per-call accounting in `llm_call`, because the same
  fact recorded twice gets two retention policies and two chances to disagree.
  A failed audit write is logged and dropped rather than failing the action
  that caused it, so a logging outage does not become an authentication outage;
  the cost is that this records what happened while the database was reachable,
  not a guarantee that every action was recorded.

- An **Audit** tab in space settings, listing the trail for owners: sign-ins,
  membership changes, model changes, and refused requests, newest first with
  paging. Refusals are set apart from the successful actions around them, since
  a refusal is the entry an owner is usually looking for. An action this Portal
  does not recognise is shown verbatim rather than hidden, so a newer server's
  events never silently disappear from the list. Members see the tab and an
  explanation of why the contents are owner-only, rather than a tab that exists
  for some people and not others.

- Portal browser tests, run by `./make e2e` against a deployment that is
  already up. They deliberately do not repeat the API-level deployment smoke,
  which already drives login, team, task, worker, and artifact — what only a
  browser can show is whether the published bundle works against a real server:
  the runtime API base, hash routing, session restoration, and the views that
  exist only in the UI. `deployment-smoke` runs them after the kind stack comes
  up, and uploads a trace on failure. They found the blank-page defect fixed
  below on their first run.

### Changed

- `docs/start/support.md` gains a compatibility section, and its stale rows are
  corrected. It now states what an upgrade may do to a deployment: the schema
  moves forward only with one release of binary rollback, the HTTP API carries
  no version and may change with a changelog note, configuration is additive
  with removals announced but no deprecation period, and stored data is never
  rewritten or deleted by an upgrade. Where a promise does not exist it says so
  rather than implying one. The audit log, the private Kubernetes reference,
  and worker pod containment are no longer described as missing.

- The frontend toolchain moves to Node 24 and npm 11, from Node 22 and npm 10.
  Node 22 entered maintenance; 24 is the active LTS. This affects `gui/`,
  `portal/`, and `desktop/frontend/` only — normal CLI work still needs no Node
  at all. The three lockfiles were regenerated with npm 11, which changed
  nothing but deduplicating a nested `@eslint/js` copy. `./make doctor` reports
  the versions it wants, and `TestFrontendToolchainPinsAgree` fails if
  `.node-version`, the `packageManager` fields, and the `engines` ranges drift
  apart again.

- `storage.minio` no longer defaults its endpoint, region, and credentials to a
  local MinIO and that server's development user. Those defaults made "unset"
  unreachable, so a deployment that omitted them was silently pointed at
  `localhost:9000` as user `minio` instead of falling through to AWS endpoint
  resolution and the SDK credential chain. A credential should never have a
  default. Nothing in the repository relied on them — Compose uses the local
  filesystem backend and the kind manifest sets all of them explicitly — but a
  deployment that did will now need to state them.

- Full `./make build` is now strict and includes the Portal; frontend or Wails
  failures no longer leave a successful partial build. Portal and Desktop lint
  are zero-warning gates.
- `AGENTS.md` is now a compact, stable navigation and constraint guide backed
  by integrity tests for the repository's `.buildmax` agent configuration.
- Release chores now live under `./make release`, and local image loading is
  `./make kind images`.
- `ROADMAP.md` moved to `docs/ROADMAP.md`, so the repository root keeps only the
  files GitHub and packaging tools expect there.

- **Self-registration is closed by default.** `POST /api/otp/request` refuses
  `intent: signup` with 403 unless `server.yaml` sets `allow_signup: true`.
  Together with `dev_login_otp`, open signup meant anyone who could reach a
  server could create an account and then sign in as it.
- The Portal no longer hard-codes a developer's local kind hostname to pick its
  API URL; the kind manifest sets `BUILDMAX_API_BASE` like any other deployment.
- `buildmax version` falls back to the Go build info, so a binary installed with
  `go install` reports its module version instead of `dev`.
- Startup tells a first-time user what to do: a missing configuration file, a
  file without models, and an unedited placeholder key are now three distinct
  messages instead of one.
- Go 1.26.6, `golang.org/x/net` 0.55.0, `golang.org/x/text` 0.39.0, and
  `goldmark` 1.7.17 — closing 23 vulnerabilities reachable from this code.
- Release tools are pinned to exact versions for reproducible builds.
- Release setup instructions now use the current YAML configuration flow.
- The local kind manifests moved from `setup/` to `deployment/dev-kind/`, next to
  the deployment files they resemble. `./make kind up` is unchanged.
- Community health files — Code of Conduct, Support, Governance, Maintainers,
  Trademarks — moved to `.github/`, where GitHub surfaces them exactly as it does
  from the repository root. The root now carries only README, CONTRIBUTING,
  SECURITY, CHANGELOG, ROADMAP, LICENSE, and the agent instructions.
- `AGENTS.md` no longer duplicates the repository tree or detailed project
  conventions. It keeps only a compact command/constraint guide and routes to
  `docs/contribute/repo-layout.md`, `CONTRIBUTING.md`, and the new
  `docs/contribute/conventions.md` for the full sources of truth.

### Removed

- Dead code across the agent runtime, storage, handlers, and CLI that the new
  linter surfaced.

### Fixed

- `LICENSE` is the verbatim Apache License 2.0 again. The copy shipped in
  `0.1.0-alpha` was missing the closing paragraph of section 5, the one that
  keeps a separately executed license agreement in force over the default
  contribution grant — the same grant `CONTRIBUTING.md` relies on to state that
  BuildMax needs no CLA. Only the appendix copyright line is filled in, which
  the license permits, so automated license detection now identifies the
  repository and every release archive as Apache-2.0 rather than as an unknown
  license.

- Portal rendered a blank page. `@buildmax/gui` is a symlinked workspace package
  that externalises React, so its bare `import "react"` resolved from its own
  real path — and `gui` has React installed as a peer. The bundle therefore
  shipped two React instances and every hook threw
  `Cannot read properties of null (reading 'useState')`. Deduplicating React in
  the Portal's Vite config fixes it, and takes 8 kB off the bundle. This was
  invisible to the build, to TypeScript, and to the API-level deployment smoke;
  the browser tests added alongside it are what found it.

## [0.1.0-alpha] - 2026-08-09

### Added

- Initial alpha release of the BuildMax CLI/TUI, server, and worker binaries.
- Linux, macOS, and Windows archives with checksums and third-party notices.
- Multi-architecture Linux container image published to GHCR.

[Unreleased]: https://github.com/gougoujiang/buildmax/compare/v0.2.0-alpha.1...HEAD
[0.2.0-alpha.1]: https://github.com/gougoujiang/buildmax/compare/v0.1.0-alpha.2...v0.2.0-alpha.1
[0.1.0-alpha.2]: https://github.com/gougoujiang/buildmax/compare/v0.1.0-alpha.1...v0.1.0-alpha.2
[0.1.0-alpha.1]: https://github.com/gougoujiang/buildmax/compare/v0.1.0-alpha...v0.1.0-alpha.1
[0.1.0-alpha]: https://github.com/gougoujiang/buildmax/releases/tag/v0.1.0-alpha
