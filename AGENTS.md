# BuildMax Agent Guide

This file is loaded into agent sessions that work in this repository. It is a
compact map of the project and its non-negotiable constraints, not a duplicate
of the documentation. Follow links to the source of truth before changing a
subsystem.

## Product And Priorities

BuildMax is a general-purpose AI Agent runtime. It should be quick to run,
portable, configurable across models and tools, and suitable for local or
private deployment.

The active priority order is in [`docs/ROADMAP.md`](docs/ROADMAP.md). Current
code plus the roadmap wins when an older design record disagrees. Design
records under [`docs/design/`](docs/design/README.md) explain rationale;
current behavior belongs in user or contributor documentation.

The primary implementation language is Go. The CLI/TUI must remain usable as a
single binary without Node. Portal and Desktop have React frontends; this is an
intentional exception, not a reason to add another runtime to the Go core.

## Find The Right Source Of Truth

- Documentation index: [`docs/README.md`](docs/README.md)
- Repository tree and package ownership:
  [`docs/contribute/repo-layout.md`](docs/contribute/repo-layout.md)
- Current architecture: [`docs/contribute/architecture/overview.md`](docs/contribute/architecture/overview.md)
- Contribution process: [`CONTRIBUTING.md`](CONTRIBUTING.md)
- Naming, IDs, tool output, commits, and changelog rules:
  [`docs/contribute/conventions.md`](docs/contribute/conventions.md)
- Configuration and environment variables:
  [`docs/reference/configuration.md`](docs/reference/configuration.md)
- This workspace's optional skills, subagents, and MCP configuration:
  [`.buildmax/README.md`](.buildmax/README.md)

Do not restate the repository tree outside `docs/contribute/repo-layout.md`.
When a package moves, update that file and the relevant architecture document.

## Architecture Boundaries

The dependency direction is:

```text
bootstrap --> interface / server / service / agentapp / infra --> core
```

`internal/core` is pure domain code. It must not import config, infra, service,
server, agentapp, or interface packages. `internal/config` loads files and
environment only; it does not assemble infrastructure. These boundaries are
enforced by tests under `internal/architecture`.

Important ownership boundaries:

- `internal/core/agent` owns the shared LLM/tool-calling loop.
- `internal/agentapp` assembles the runtime used by CLI, Desktop, eval, and
  workers: models, tools, MCP, hooks, sandbox, traces, skills, sessions, and
  workspace resolution.
- `internal/service/conversation` is Tier 1, the Portal-facing orchestrator and
  single voice to the user.
- Task plus TaskRun is Tier 2, durable background execution. Tier 2 reports
  results to Tier 1; it does not speak directly to the user.
- `internal/tool/names.go` is the source of truth for LLM-facing runtime tool
  names. Hook matchers and subagent `tools:` entries use those exact strings.
- `internal/server/handlers/routes.go` is the source of truth for HTTP routes.
- `internal/config/env_spec.go` is the source of truth for bootstrap environment
  variables.
- The `xxxRow` structs in `internal/infra/db` are the source of truth for the
  database schema; `AutoMigrate` in `store.go` applies them. The full table
  reference and the rules for changing them are in
  [`docs/contribute/architecture/data-model.md`](docs/contribute/architecture/data-model.md).
  GORM stays inside that package: above it, "no such row" is `model.ErrNotFound`.
- `internal/mock` and `internal/testsupport` are test-only. Production code must
  not import either; a test enforces it.

Team is the ownership and authorization boundary for Portal resources. Issue is
the primary user-facing work object. Workflows are team-scoped reusable linear
plans. Conversations orchestrate foreground turns and background tasks. The
Desktop `Project` concept is local UI state, not a server domain entity.

Read the relevant architecture document before making a cross-package change:

- Agent loop, tools, sessions, CLI, TUI, Desktop, server, store, Portal, config,
  logging, and utilities are indexed in
  [`docs/contribute/architecture/`](docs/contribute/architecture/README.md).
- Durable specifications for hooks, sandbox boundaries, and traces are indexed
  in [`docs/design/README.md`](docs/design/README.md).
- Portal product intent and surface positioning live in
  [`docs/design/product-vision.md`](docs/design/product-vision.md) and
  [`docs/design/surface-positioning.md`](docs/design/surface-positioning.md).

## Runtime Invariants

- CLI commands and the Bubble Tea TUI live in `internal/interface/cli`; binary
  entry points under `cmd/` stay thin.
- Settings use `<BUILDMAX_HOME>/settings.yaml`; server/worker settings use
  `<BUILDMAX_HOME>/server.yaml`. The default data directory is `~/.buildmax`.
- An optional workspace-root `AGENTS.md` is appended to the core system prompt.
- Runtime hooks merge global settings with `<workspace>/.buildmax/hooks.yaml`.
  Hook failures fail open; gating contracts are documented in
  [`docs/design/hook-system.md`](docs/design/hook-system.md).
- The Bash sandbox is available on macOS and Linux but currently defaults off
  on all surfaces. `config.defaultSandbox` defines a stricter
  `SandboxSurfaceWorker` baseline, but no worker path passes that surface —
  `internal/agentapp/taskrun` leaves `AppConfig.SandboxSurface` empty, which
  resolves to the CLI baseline. The baseline is written, not wired. Do not
  claim the deferred worker hardening is active.
- Every run records a bounded, redacted JSONL trace by default. Trace failure is
  fail-open and must not break an agent run.
- Server authentication requires a JWT secret. Login codes are single-use;
  signup is disabled by default. The legacy development OTP is deliberately
  unsafe and causes a startup warning.
- Worker runs materialize the team's persistent `home`, execute in a run-scoped
  directory, write artifacts, and use a run-scoped `BUILDMAX_HOME`.
- Portal and Desktop share presentational components from `@buildmax/gui`, not
  data/auth/routing logic. Both use React 19.

The planned but not implemented areas include team approvals,
versioned workspace/timeline restore, worker sandbox hardening, and complete CI
coverage for Kubernetes and native Windows. Do not document them as shipped.

## Build, Test, And Check

Use the cross-platform task runner from the repository root:

```bash
./make doctor          # read-only contributor environment diagnosis
./make build           # strict full build: Go binaries, gui, Portal, Desktop
./make build cli       # fast CLI-only build
./make test            # Go tests with an isolated BUILDMAX_HOME
./make test race       # the same suite with the race detector
./make lint            # pinned golangci-lint and govulncheck
./make check <scope>   # go, portal, desktop, docs, all, or ci
./make check ci        # everything a pull request runs, except the Windows job
./make e2e <suite>     # one end-to-end suite: cli, desktop, local, compose, kind, all
./make help            # common contributor commands; add `all` for everything
```

End-to-end suites are a local feedback loop, not a pull-request gate. `cli` and
`desktop` need nothing but Go and run in seconds, so `./make test` includes
them; `local` owns a Compose stack for one run; `compose` and `kind` attach to a
deployment someone else started. None needs a provider API key — every suite
answers the model from a committed scenario. Pick a suite, read the artifacts it
leaves in `.artifacts/e2e/`, and see
[`docs/contribute/testing.md`](docs/contribute/testing.md) for which suite
covers what and what each one needs.

`./make agent-smoke` is not a test: it drives the agent's tools with a real
model, needs an API key, and reports a table the model wrote about itself.

On Windows use `make.bat`. Add or change commands under `cmd/mk`; the `make`
and `make.bat` files remain one-line shims. Do not introduce a parallel shell
script workflow.

Go, Node, npm, and Wails versions are pinned by `go.mod`, `.node-version`, the
frontend `packageManager` fields, and the Wails module dependency. Use `npm ci`
for reproducible installs. Normal CLI development has no Node dependency.

Run checks in proportion to the change, and prefer the narrow scope while
iterating. Before handoff, run every relevant scope. A full check requires no
model API key. Tests must not write to a contributor's real `~/.buildmax`.

Safe, local defaults include `doctor`, `build cli`, `test`, `lint`, and scoped
`check`. Commands such as `install`, `release`, `compose`, `kind`, and
publication workflows change the machine, repository, or external systems;
inspect their help and use them only when the task authorizes that effect.

## Change Rules

- Preserve unrelated work in a dirty worktree. Never reset or overwrite another
  contributor's changes to make checks pass.
- Persisted JSON uses explicit `snake_case` tags. Database table names are
  singular. Entity IDs use `NewPrefixedID` from `internal/util` and the
  documented prefix convention.
- Tool output is written for the LLM and must be meaningful on success and
  failure.
- Keep code comments short. Comment the background and the decision — why this
  approach, what was rejected, what breaks if it changes — not what the code
  already says. A comment that restates its own function is noise to maintain;
  delete it rather than update it. Longer rationale belongs in a design record.
- Keep user documentation task-oriented. Keep contributor architecture factual.
  Keep rationale in design records. Follow
  [`docs/contribute/documentation.md`](docs/contribute/documentation.md).
- Add a changelog entry for user-visible changes as a new file,
  `docs/changelog/<category>/<slug>.md`, holding the one list item it will
  become. One file per entry so parallel branches never conflict; the release
  step folds them into `CHANGELOG.md`.
- Commit subjects are one imperative line. Do not add assistant attribution,
  session links, generated-with footers, or tooling trailers to commits or pull
  requests.
- Do not commit local `.vibe/` notes. They are scratch state, not project
  documentation.

When adding or changing dependencies, update lockfiles and run the repository's
license checks. Security-sensitive changes should also be assessed against
[`SECURITY.md`](SECURITY.md) and the sandbox/hook trust boundaries.

## Definition Of Done

A contribution is ready when the requested behavior is implemented, relevant
tests and scoped checks pass, documentation and examples match the code,
generated or lock files are intentional, and `git diff --check` is clean. Report
checks that were not possible rather than silently treating them as passed.
