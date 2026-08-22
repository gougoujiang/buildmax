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

## Entity IDs Are Opaque Public Handles

A server entity has two identifiers with one role each. `id` is a
`bigint unsigned` primary key, is the relational key inside MySQL, and never
leaves `internal/infra/db`. `public_id` is the handle every boundary sees: 96
bits of crypto-random data, written as 20 lowercase base32 characters and
stored in that same text form (`char(20) ascii_bin`), so a direct database
query reads the handle every other boundary shows.

```text
ivyoh5qcfu6ypfkhyedq
```

Generate one with `util.NewPublicID`, which returns an error rather than
panicking, and validate one with `util.CanonicalPublicID`, which accepts either
case and rejects any non-canonical spelling. Type prefixes are gone: the route, the
JSON field, and the column already name the type, and nothing dispatched on the
prefix.

Not every row earns a handle. A join row, a revision, and a catalog release are
addressed by their parent plus a natural key instead. Which tables have one,
which references become numeric, and which stay opaque strings are decided in
[../design/entity-identity.md](../design/entity-identity.md); read it before
adding a table.

In JSON, a resource names its own handle `id` and keeps semantic names for
relationships — `{"id": ..., "team_id": ..., "conversation_id": ...}`. Order
rows by `created_at`, never by ID.

There is one identifier format. A login chain, a trace file, and a Desktop
project use it unprefixed, the same as any entity. The single exception is a
local background job, which reads as `jb_<public id>`: its ID reaches the model
as a bare string inside tool output, and free prose is the one place a type
prefix says something the surrounding context does not. Agent session IDs are
UUIDs, because they name a file rather than anything this codec identifies.

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
- Add a changelog entry for anything a user or operator would notice: new or
  changed behavior, new configuration, removals, and fixes to released behavior.
  Internal refactors, test-only changes, and documentation edits do not need one.
- **An entry is a new file**, `docs/changelog/<category>/<slug>.md`, holding the
  one Markdown list item it will become. Branches that each add a file never
  conflict; branches that each added a line to `## [Unreleased]` in
  `CHANGELOG.md` conflicted every time. `./make changelog` previews the section
  they will fold into; the full rules are in
  [`docs/changelog/README.md`](../changelog/README.md).

## Related

- [CONTRIBUTING.md](../../CONTRIBUTING.md) — prerequisites, build and test, pull requests
- [repo-layout.md](repo-layout.md) — the tree and the dependency direction
- [documentation.md](documentation.md) — what to update when behavior changes
