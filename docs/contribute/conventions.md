# Code And Commit Conventions

> **Audience:** contributors · **Status:** current

Project-wide rules that are not obvious from reading the code, and that review
will hold a pull request to. Layering rules live in
[repo-layout.md](repo-layout.md#dependency-direction) and are enforced by tests
in `internal/architecture`; documentation rules live in
[documentation.md](documentation.md).

## Persisted Data Uses snake_case

Everything this project writes to disk — session files, config, any JSON — uses
the same naming style.

- **snake_case** for JSON object keys: `created_at`, `tool_call_id`,
  `tool_calls`.
- Give every struct that is serialized to disk explicit `json:"snake_case"`
  tags. Do not rely on Go's default, which is the Go field name.

## Database Tables Are Singular

One table per entity type, named in the singular: `user`, `agent`,
`conversation`, `task`, `task_run`. Never `users` or `tasks`. This applies to
every table the project creates or migrates.

## Entity IDs Are Prefixed

Entity IDs use `<prefix>_<body>`, where the body is 20 characters of lowercase
base36 (`[a-z0-9]`).

| Prefix | Entity | Prefix | Entity |
|---|---|---|---|
| `u_` | user | `c_` | conversation |
| `tm_` | team | `cm_` | conversation message |
| `i_` | issue | `t_` | task |
| `a_` | agent | `r_` | task run |
| `w_` | workflow | `ar_` | artifact |
| `wr_` | workflow run | `f_` | artifact item |
| `wsr_` | workflow step run | `whk_` | webhook key |
| `lc_` | managed LLM call | `lm_` | managed model |
| `ae_` | audit event | `sg_` | system grant |

Generate them with `internal/util.NewPrefixedID(prefix)`, passing the prefix
without the underscore; the constants live in `internal/util/id.go`, which with
its tests is the reference for the format. Order rows by `created_at`, never by
ID. Session IDs are the one exception — they are internal and use UUIDs.

## Tool Output Is Written For The LLM

A tool's return value goes back to the model as a tool-role message. It is not a
log line and not a user-facing string.

- **Succeed loudly.** Say what was done or what was found, concisely enough to
  be worth its tokens.
- **Fail usefully.** `path outside allowed root` and `file not found` let the
  model decide whether to retry, adjust, or tell the user. `error` alone does
  not. The agent prefixes tool errors with `error:` on the way to the model.

## Logs Are Written For Triage

One rule decides the level, so a threshold selects something meaningful:

| Level | Means |
|---|---|
| `Error` | A unit of user work failed or was lost |
| `Warn` | Degraded, but the work finished — including a fail-open path that ignored something the user configured |
| `Info` | Lifecycle and state transitions |
| `Debug` | Per-item detail |

Warn outnumbering Error is expected here rather than a smell: hooks, traces, the
sandbox, and skill loading all fail open by design, and each of those is a
genuine "degraded but finished".

Identity goes in attributes, never in the message text. A subsystem sets
`component` once — `slog.With("component", "scheduler")` — and the message
describes only the event. Build that logger where it is used rather than in a
package variable, which would capture `slog.Default()` before `infra/log`
installs the real handler at startup.

Attribute keys are `snake_case`, and an error is always `"err"`.

Correlation travels on the context. `infra/log.With(ctx, ...)` adds attrs that
the handler puts on every record logged with that context, so call sites use the
stdlib `slog.*Context` functions and import nothing. That indirection is what
lets `internal/core` carry a run's identifiers without importing infra. The
server puts `request_id` on every request and returns it in `X-Request-Id`; a
worker puts `component` and `task_run_id` on the default logger, because it runs
exactly one task run.

Never log a credential, and never log a provider's raw error body — it can carry
account identifiers and request fragments.

## Commit Messages And Pull Requests

The repository's public record carries project content only. Tooling
attribution is noise in a history that anyone can read.

- Commit subjects are a **single imperative line** — `Move the Dockerfiles into
  deployment/docker`, not `moved dockerfiles` or `fix stuff`. Add a body when
  the reason is not obvious from the diff.
- **Pull requests are merged with a merge commit, so every commit on the branch
  lands on `main`.** Each subject follows the rule above, not just the pull
  request title: `Fix the worker artifact path` — not `Bug fix`, not `WIP`, not
  an issue number alone. Organize the branch into the commits you want in the
  history, and rewrite it before merge rather than appending `fix review
  comments` — that commit is permanent.
- **The pull request title becomes the merge commit subject.** It is what the
  history shows for the change as a whole, so write it as one imperative line
  that reads without the branch under it.
- Keep the branch to **one coherent change**. The merge commit is what a revert
  targets, so a pull request that does three unrelated things cannot have one of
  them backed out later. Two changes, two pull requests.
- Do **not** add `Co-Authored-By` or `Claude-Session` trailers to commits.
- Do **not** add a "Generated with …" footer or an assistant session link to a
  pull request description.
- Follow [`.github/pull_request_template.md`](../../.github/pull_request_template.md)
  for the shape of a pull request description.
- Add a `CHANGELOG.md` entry under `## [Unreleased]` for anything a user or
  operator would notice: new or changed behavior, new configuration, removals,
  and fixes to released behavior. Internal refactors, test-only changes, and
  documentation edits do not need one.
- **Append the entry to the end of its section**, not the top. Two branches that
  both insert at the top of `### Added` conflict every time, because git cannot
  order two additions to the same line — and the conflict is guaranteed rather
  than likely, since every pull request that needs an entry touches that line.
  Appending puts each branch on a different line and the merge resolves itself.
  Release preparation is where entries get ordered for readers.

## Related

- [CONTRIBUTING.md](../../CONTRIBUTING.md) — prerequisites, build and test, pull requests
- [repo-layout.md](repo-layout.md) — the tree and the dependency direction
- [documentation.md](documentation.md) — what to update when behavior changes
