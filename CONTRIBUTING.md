# Contributing to BuildMax

Thanks for helping improve BuildMax. Small, focused contributions are the
easiest to review and keep the shared runtime dependable across CLI, Desktop,
Portal, and worker execution.

## Before You Start

- Search existing issues and [design documents](docs/design/) before proposing a
  large change. [docs/README.md](docs/README.md) explains how the documentation
  is organized.
- Open an issue or discussion first for product changes, new runtime providers,
  new tools, or changes that affect security, persistence, or public APIs.
- Never include credentials, customer data, or private deployment details in an
  issue, pull request, test fixture, or commit.
- Report vulnerabilities privately as described in [SECURITY.md](SECURITY.md).
- Use [SUPPORT.md](SUPPORT.md) to choose the right channel, and follow
  [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) in every project space.
- Project decisions and maintainer responsibilities are described in
  [GOVERNANCE.md](GOVERNANCE.md) and [MAINTAINERS.md](MAINTAINERS.md).
- Maintainers follow [docs/contribute/releasing.md](docs/contribute/releasing.md)
  for version tags, release verification, and recovery.

## First Contributions

Good first pull requests are small, easy to verify, and close to an existing
issue. Look for issues labeled `good first issue`, `help wanted`, or
`documentation`; if none are available, start with a bug report, documentation
gap, or discussion instead of opening a large speculative PR.

Before picking work, read the [support matrix](docs/start/support.md). It names
the surfaces and deployment paths the project supports today, plus the alpha
non-goals that should not become surprise PR scope. Documentation fixes, focused
tests, small CLI/TUI usability fixes, and reproducible bug fixes are the best
entry points.

Use the issue templates to give maintainers the context they need:

- **Bug report:** reproducible behavior that differs from docs or expectations.
- **Feature request:** a user problem and expected outcome, before implementation.
- **Documentation:** missing, unclear, or stale docs.

## Prerequisites

- **Go** — the version in `go.mod`. The CLI, server, worker, and desktop backend
  are all Go; there is no Python or Node runtime dependency for the CLI.
- **Node** — only for the frontends: the shared `gui/` package, `portal/`, and
  `desktop/frontend/`.
- **An LLM API key** — add a model to `~/.buildmax/settings.yaml`; see
  [docs/reference/configuration.md](docs/reference/configuration.md). Free
  OpenRouter models rate-limit with HTTP 429 when called too frequently; if runs
  start failing in bursts, that is usually why.
- **Docker** — for the Compose deployment smoke and container changes.
- **kind and kubectl** — only for Kubernetes worker, RBAC, shared-storage, or
  manifest changes. The task runner reports missing tools; it does not install
  system packages for you.

## Build, Test, and Run

The root `./make` script is the primary local workflow — on Windows, `make.bat`.
Both are one-line shims around the task runner in [`cmd/mk`](cmd/mk), so every
platform runs the same code. `.env` at the repo root is loaded automatically.

```bash
./make build          # CLI, server, worker, gui, desktop app
./make build cli      # CLI only
./make test           # go test ./... with BUILDMAX_HOME=./testing-sandbox
./make run server     # build and run buildmax-server
./make run portal     # Portal dev server (builds gui if needed)
./make run desktop    # Wails desktop app in dev mode
./make compose smoke  # full local-process TaskRun and artifact smoke
./make kind up        # full Kubernetes stack plus the same smoke assertions
./make clean          # binaries, desktop build dir, node_modules, dist
```

`./make help` lists every command. To add or change one, edit `cmd/mk` rather
than the shims. `setup` and `deploy` are compatibility aliases for `kind up`;
`unsetup` aliases `kind down`.

`./make test` writes to `./testing-sandbox` instead of `~/.buildmax`, so tests
never touch your real data directory. The sandbox is created on demand and is
gitignored.

Frontend packages also build from their own directories; see
[README.md](README.md), [portal/README.md](portal/README.md), and
[gui/README.md](gui/README.md).

`./make smoke` and `./make run server` are for manual local checks. Do not use
them in automated CI.

### The `desktop` Build Tag

The desktop frontend bundle lives in `desktop/frontend/dist/`, a Vite build
artifact that is not checked in. Embedding it unconditionally would make
`go build ./...`, `go vet ./...`, and `go test ./...` fail on a fresh clone,
so the `//go:embed` directive sits behind the `desktop` build tag:

- without the tag, `desktop/assets_stub.go` compiles and embeds nothing, which
  is what every standard Go command and your editor use
- with `-tags desktop`, `desktop/assets_embed.go` compiles and embeds the bundle

`./make build` passes the tag and builds the frontend first, so you rarely think
about it. A desktop binary built without the tag refuses to start and prints how
to rebuild it, rather than opening a blank window.

One exception is worth knowing before you touch this area: Wails treats
`desktop` as one of its own reserved mode tags and strips it from the throwaway
binary it compiles to generate JS/TS bindings. That binary therefore always sees
`Embedded == false`, and the refuse-to-start guard in
`cmd/buildmax-desktop/main.go` skips it via the `bindings` build tag. See
`cmd/buildmax-desktop/bindings_on.go` — without it, every `wails build` fails
during binding generation.

## Local Infrastructure

Choose the smallest deployment path that exercises your change.

For server, Portal, API, scheduler, or local-process worker changes, use
Compose. It runs MySQL, Portal, server, and a deterministic mock model; workers
share the server container and local filesystem storage:

```bash
./make compose smoke
./make compose logs
./make compose down
```

For Kubernetes Job, RBAC, Ingress, MinIO, or deployment manifest changes, use
kind. `up` creates the cluster, builds and loads images, deploys every
dependency, and runs the same TaskRun and artifact assertions:

```bash
./make kind up
./make kind smoke
./make kind logs
./make kind down
```

[docs/deploy/local-kind.md](docs/deploy/local-kind.md) documents what it
installs. Override the cluster name with `BUILDMAX_KIND_CLUSTER` (default
`buildmaxdev`); every kubectl call uses that cluster's explicit context.

### Container Images

Two Dockerfiles live at the repository root: `Dockerfile.buildmax` for the Go
binaries and `Dockerfile.portal` for the Portal. To build both and load them
into the kind cluster:

```bash
./make pub_images
```

Set `BUILDMAX_IMAGE_PLATFORM` to cross-build — for example
`BUILDMAX_IMAGE_PLATFORM=linux/amd64 ./make pub_images` on Apple Silicon.

`deployment/buildmax-deploy.yaml` is the readable Kubernetes baseline. It
carries no credentials: copy
`deployment/buildmax-secret.example.yaml` to `buildmax-secret.local.yaml`,
fill it in, and apply it separately for a non-smoke deployment. The
`.local.yaml` file is gitignored — never commit real values.

## Code Boundaries

Keep changes aligned with the existing layering:

- shared agent behavior belongs in `internal/core/agent` or `internal/agentapp`
- infrastructure adapters belong in `internal/infra`
- user-facing orchestration belongs in `internal/service` or `internal/interface`
- `internal/core` must not import application, infrastructure, or interface layers
- persisted JSON uses explicit `snake_case` field names

The layering rules are enforced by tests in `internal/architecture`, so a
violating import fails the build rather than sliding through review.

## Pull Requests

- Keep each pull request focused on one user-visible outcome or engineering concern.
- Add or update focused tests for behavioral changes.
- Update documentation alongside the code:
  - behavior or package boundaries change → update the matching document in
    [docs/contribute/architecture/](docs/contribute/architecture/README.md)
  - a package moves → update
    [docs/contribute/repo-layout.md](docs/contribute/repo-layout.md), which is
    the only place the tree is written down
  - direction changes → add a numbered document to
    [docs/design/](docs/design/README.md), and delete the superseded one
  - user-facing behavior or configuration changes → update `docs/guide/`,
    `docs/reference/`, and `config-examples/`

  The full rules are in
  [docs/contribute/documentation.md](docs/contribute/documentation.md).
- Explain the problem, the approach, verification performed, and any remaining
  limitations in the pull request description.
- Preserve existing public behavior unless the pull request explicitly documents
  a breaking change.

CI runs `gofmt`, a `go mod tidy` cleanliness check, build, vet, golangci-lint,
govulncheck, and the test suite — with `-race` on Linux — for Go on Linux and
Windows. It builds all three frontends, lints Portal and the desktop frontend,
runs Portal tests, scans Git history for secrets, checks Go and npm production
dependency licenses, and lints Markdown. CodeQL analyzes Go and TypeScript once
the repository is public. Pull requests validate the GoReleaser configuration;
pushes to `main` and manual CI runs build and smoke-test a non-publishing
release snapshot on Linux, macOS, and Windows. Deployment-related changes also
run the Compose and kind end-to-end smoke paths.

Locally:

```bash
./make test              # go test with BUILDMAX_HOME=./testing-sandbox
./make lint              # golangci-lint and govulncheck, CI's pinned versions
./make build             # every binary, including the frontends
go test -race ./...      # what CI runs on Linux
```

When editing a workflow, lint it the way CI does. `go run
github.com/rhysd/actionlint/cmd/actionlint@v1.7.12` silently skips its
shellcheck pass when shellcheck is not on your PATH, so a `run:` block can pass
locally and fail on the runner. The published image carries shellcheck:

```bash
docker run --rm -v "$PWD:/repo" -w /repo rhysd/actionlint:1.7.12 -color
```

The linter set and the reasoning behind each exclusion are in
[`.golangci.yml`](.golangci.yml). For the frontends, `npm run lint` in `portal/`
and `desktop/frontend/`; `gui/` has no ESLint step because typescript-eslint
does not yet support the TypeScript version it builds with, and its own build
type-checks it.

A govulncheck failure usually means the Go toolchain or a dependency needs a
patch release. Bump the `go` directive in `go.mod` or the module, do not add a
suppression.

## Contribution License

By submitting a contribution, you agree that it is your original work and that
you license it under the [Apache License 2.0](LICENSE). This is the default
contribution grant described in Section 5 of that license; no separate CLA is
required at this stage.
