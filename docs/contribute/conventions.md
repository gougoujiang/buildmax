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

## Commit Messages And Pull Requests

The repository's public record carries project content only. Tooling
attribution is noise in a history that anyone can read.

- Commit subjects are a **single imperative line** — `Move the Dockerfiles into
  deployment/docker`, not `moved dockerfiles` or `fix stuff`. Add a body when
  the reason is not obvious from the diff.
- **Pull requests are squash-merged, so the pull request title becomes the
  commit subject on `main`.** Write the title to the rule above: one imperative
  line, specific enough to read on its own in `git log --oneline`. `Fix the
  worker artifact path` — not `Bug fix`, not `WIP`, not an issue number alone.
  The commits on your branch are yours to organize however you like; only the
  title survives.
- Keep the branch to **one coherent change**. Squash merging collapses whatever
  is on the branch into a single commit, so a pull request that does three
  unrelated things becomes one commit that does three unrelated things, and
  reverting one of them means reverting all three. Two changes, two pull
  requests.
- Do **not** add `Co-Authored-By` or `Claude-Session` trailers to commits.
- Do **not** add a "Generated with …" footer or an assistant session link to a
  pull request description.
- Follow [`.github/pull_request_template.md`](../../.github/pull_request_template.md)
  for the shape of a pull request description.
- Add a `CHANGELOG.md` entry under `## [Unreleased]` for anything a user or
  operator would notice: new or changed behavior, new configuration, removals,
  and fixes to released behavior. Internal refactors, test-only changes, and
  documentation edits do not need one.

## Related

- [CONTRIBUTING.md](../../CONTRIBUTING.md) — prerequisites, build and test, pull requests
- [repo-layout.md](repo-layout.md) — the tree and the dependency direction
- [documentation.md](documentation.md) — what to update when behavior changes
