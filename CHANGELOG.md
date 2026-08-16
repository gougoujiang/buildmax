# Changelog

All notable changes to BuildMax are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
BuildMax is currently in alpha, so incompatible changes may still occur between
pre-releases and must be called out in release notes.

## [Unreleased]

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

### Changed

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

## [0.1.0-alpha] - 2026-08-09

### Added

- Initial alpha release of the BuildMax CLI/TUI, server, and worker binaries.
- Linux, macOS, and Windows archives with checksums and third-party notices.
- Multi-architecture Linux container image published to GHCR.

[Unreleased]: https://github.com/gougoujiang/buildmax/compare/v0.1.0-alpha...HEAD
[0.1.0-alpha]: https://github.com/gougoujiang/buildmax/releases/tag/v0.1.0-alpha
