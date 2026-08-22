# Repository Layout

> **Audience:** contributors · **Status:** current
>
> **This file is the single source of truth for the repository tree.** README,
> `AGENTS.md`, and the architecture reference link here rather than repeating
> it, so there is one place to update when a package moves.

## Top Level

```text
buildmax/
├── cmd/                  Binary entry points (main.go only)
├── internal/             All Go implementation
├── portal/               Portal web app (React 19 + Vite + TypeScript)
├── desktop/frontend/     Desktop frontend (React 19 + Vite); src/lib holds the
│                      pure helpers, src/components the panels and modals
├── gui/                  Shared React package @buildmax/gui, used by both
├── docs/                 Documentation
├── config-examples/      settings.yaml / server.yaml / hooks.yaml examples
├── deployment/           Deployment manifests, Compose, Dockerfiles, dev-kind
├── eval/                 Agent benchmark task catalog
├── sample-data/          Seed datasets to upload into a workspace or point the CLI at
├── .github/              CI workflows, issue and PR templates, community health files
├── .buildmax/            This repository's own workspace agent config — see .buildmax/README.md
├── make, make.bat        One-line shims around the task runner in cmd/mk
└── *.md, LICENSE         README, CONTRIBUTING, SECURITY, CHANGELOG, AGENTS
```

Generated and never committed: `bin/` (`./make build` output), `dist/`
(GoReleaser), `NOTICE-THIRD-PARTY`, `testing-sandbox/` (`./make test` data
directory), and every `node_modules/` and frontend `dist/`.

Root Markdown, and who each file is for:

| File | Audience |
|---|---|
| `README.md` | Anyone landing on the repository |
| `CONTRIBUTING.md` | Contributors — prerequisites, build, test, pull requests |
| `SECURITY.md` | Vulnerability reporters and operators |
| `CHANGELOG.md` | Users and operators, per release |
| `AGENTS.md` | The agent, on every run in this workspace ([agents.md](https://agents.md/) convention). `CLAUDE.md` points at it. |
| `.github/CODE_OF_CONDUCT.md`, `SUPPORT.md`, `GOVERNANCE.md`, `MAINTAINERS.md`, `TRADEMARKS.md` | Community health files. GitHub surfaces them from `.github/` exactly as it does from the root. |

`deployment/` holds everything needed to run a deployment rather than to build
one binary:

| Path | Contents |
|---|---|
| `deployment/docker/` | `Dockerfile.buildmax` (Go binaries from source), `Dockerfile.portal` (Portal via nginx), `Dockerfile.release` (packages GoReleaser's cross-compiled binaries). All three take the **repository root** as their build context. |
| `deployment/compose/` | Single-machine Compose stack — a **real deployment path**, running published GHCR images; see [deploy/compose.md](../deploy/compose.md) |
| `deployment/dev-kind/` | Manifests that stand up the **local development** kind cluster — kind config, ingress-nginx, MySQL, MinIO. Never part of a real deployment; applied by `cmd/mk/kind.go` behind `./make kind up`. |
| `deployment/production/` | The private deployment reference: one plain-YAML manifest written to be read and adapted, plus the dependency contract it assumes. Deliberately not a chart or a kustomize base, so it converts to whatever a cluster is already managed with. Nothing applies it; `internal/architecture` parses it so it cannot rot |
| `deployment/smoke/` | Overlays and the mock model that make the Compose and kind smokes deterministic |
| `deployment/migrations/` | One-off SQL migrations |
| `deployment/buildmax-deploy.yaml` | Working Kubernetes manifest used by `./make kind up` |

**The `dev-` prefix means "not a deployment path".** `dev-kind` carries it
because that cluster exists only so a contributor can exercise the Kubernetes
worker locally; `compose` does not, because an operator is meant to run it — its
audience is operators, `README.md` files it under "Running it for a team", and
`compose.yaml` pulls `ghcr.io/gougoujiang/buildmax`. Renaming `compose/` for
symmetry would tell operators the opposite of the truth. `smoke/` is test
scaffolding shared by both smokes and keeps its plain name for that reason.

There is no `scripts/` directory. Repository tooling — release-archive
verification, third-party notice generation, npm license checks — lives in
`cmd/mk` as `./make` commands, so every task runs the same way on macOS, Linux,
and Windows and is covered by the same tests. CI and GoReleaser invoke those
commands as `go run ./cmd/mk <task>`, which needs no shell.

## Nested Go Modules

Two kinds of directory sit outside the root module, each with its own `go.mod`:

- **`gui/`, `portal/`, `desktop/frontend/`** contain no Go code. Their `go.mod`
  is a boundary. The Go tool has no special case for `node_modules` the way it
  does for `testdata`, so without it every `go build ./...`, `go vet ./...`,
  `go test ./...`, and `go mod tidy` at the root compiles whatever Go sources
  npm packages happen to ship — the `flatted` package, pulled in transitively
  by ESLint, ships one. Any directory that installs npm dependencies needs one;
  `internal/architecture` has a test that enforces this.
- **Each `eval/NNN-*/` fixture** is its own module so the benchmark's
  deliberately-broken code is never built or linted with the project's own.

## Binaries

| Path | Binary | Role |
|---|---|---|
| `cmd/buildmax` | `buildmax` | CLI/TUI |
| `cmd/buildmax-server` | `buildmax-server` | HTTP API + in-process scheduler |
| `cmd/buildmax-worker` | `buildmax-worker` | Executes one task run, then exits |
| `cmd/buildmax-desktop` | — | Wails desktop app; embeds `desktop/frontend` |
| `cmd/buildmax-eval` | `buildmax-eval` | Agent benchmark runner over `eval/` |
| `cmd/local-test-mcp-server` | — | Small MCP server for testing MCP integration |
| `cmd/mk` | — | Task runner behind `./make` and `make.bat`, plus the repository tooling CI and GoReleaser call; dev only, not released |

Every `cmd/*` package is a thin `main.go` that delegates to `internal/`.

## `internal/`

```text
internal/
├── bootstrap/          Process startup and dependency wiring (server, worker, objectstore)
├── config/             YAML + env config loading and path resolution
│
├── core/               Pure domain layer — no infra imports
│   ├── model/          Domain entities and repository contracts
│   ├── apierr/         Why a service refused: a Kind a transport maps to a status
│   ├── llm/            LLM contracts (Message, ToolDef, ToolCall, Usage, LLMClient),
│   │                   the Tool contract, ToolRegistry, and tool policy
│   ├── hook/           The hooks configuration shape, its events and transports
│   ├── mcp/            The mcp.json document shape and its validation rules
│   ├── agent/          The tool-calling loop, events, hooks, sandbox contract
│   ├── plugin/         Plugin manifest, version arithmetic, and the layer
│   │   │               vocabulary discovery, resolution, and publication share
│   │   ├── archive/    Packing and hardened extraction of a plugin package
│   │   └── inspect/    What a plugin package contributes, sanitized for a
│   │                   catalog record and shared with `plugin validate`
│   ├── subagent/       The subagent definition file shape and its frontmatter
│   └── session/        Local session model; persistence lives in agentapp
│
├── agentapp/           Agent runtime assembly: LLM client cache, tool registry,
│   │                   MCP, hooks, sandbox, traces, skills, sessions, workspace
│   └── taskrun/        One task run inside its run-scoped directory (worker)
│
├── service/            Application services: coordinate stores, enforce rules
│   ├── conversation/   Tier 1 orchestrator — the single voice to the user
│   │   ├── channel/    Normalized turn types and channel adapters (webhook)
│   │   ├── runtime/    Turn-loop mechanics: replay, tool assembly, streaming
│   │   └── tool/       Tier 1 tools and the task-runner bridge
│   ├── agent/          Agent definitions, their revisions, and the delete guard
│   ├── issue/          Issue service
│   ├── task/           Task and task_run service
│   ├── workflow/       Workflow and workflow-run orchestration
│   ├── audit/          Records that a sensitive action happened (governance,
│   │                   not diagnostics — see the package doc)
│   ├── plugin/         Marketplace publication and catalog lifecycle
│   ├── team/           Membership: who is in a team and who may change that
│   ├── quota/          Team quota enforcement
│   └── llmgateway/     Model catalog, team aliases, routing, and managed calls
│
├── tool/               Runtime agent tools: Read, Write, Edit, Bash, Glob, Grep,
│                       WebFetch, TodoWrite, Skill, Task, and the MCP gateway.
│                       names.go is the single source of truth for tool names.
│
├── infra/              External-system implementations
│   ├── db/             MySQL/GORM implementation of the core repositories
│   ├── objectstore/    Local FS and S3/MinIO persist + artifact storage
│   ├── llm/            LLMClient over the wire protocols BuildMax speaks:
│   │                   OpenAI Chat Completions, OpenAI Responses, Anthropic Messages
│   ├── llmwire/        Versioned wire contract for managed inference
│   ├── llmremote/      LLM client that calls a BuildMax managed gateway
│   ├── mcp/            MCP protocol, client transport, registry
│   ├── hook/           Hook transports: command, http, mcp_tool, prompt
│   ├── sandbox/        Seatbelt/bwrap backends, egress proxy, violations
│   ├── trace/          Durable run-trace recorder (bounded, redacted JSONL)
│   ├── k8s/            Kubernetes worker job launcher
│   ├── workerclient/   Worker-side HTTP client for the server worker API
│   ├── httpclient/     Decodes the server's error envelope for its Go clients
│   ├── git/            Branch and diff helpers
│   └── log/            slog + lumberjack logging
│
├── interface/          Local user-facing entry points
│   ├── cli/            Cobra CLI, Bubble Tea TUI, print mode
│   ├── desktop/        Wails app bridge
│   ├── auth/           Login client and credential persistence
│   └── client/         HTTP client for the BuildMax server API
│
├── server/             HTTP API for Portal and worker callbacks
│   ├── handlers/       Route handlers
│   │   ├── admin/      Deployment-scoped routes; a Config that cannot reach a team
│   │   ├── auth/       Establishing a session: login, refresh, logout, password
│   │   ├── auditexport/  CSV export shared by the team and admin audit routes
│   │   ├── llmhttp/    Managed gateway over HTTP, shared by the team and worker routes
│   │   ├── runterminal/  Announces a finished run to whoever is watching
│   │   ├── team/       What a team owns: members, agents, keys, usage, audit
│   │   ├── work/       Issues, workflows, tasks, conversations, and their runs
│   │   └── worker/     Worker API; authenticates with a run token, not a session
│   ├── access/         Who is calling, which team, and whether they may
│   ├── authtoken/      Signs and verifies the run token a worker presents
│   ├── httputil/       Shared request/response helpers
│   ├── scheduler/      Claims pending task runs and spawns workers
│   ├── websocket/      The live connection, the stream hub, and the protocol
│   ├── turnqueue/      Serializes a conversation's turns across both paths
│   └── static/         Embedded OpenAPI and Swagger assets
│
├── agenteval/          Evaluation harness: task catalog and runner
├── architecture/       Architectural constraint tests (import boundaries)
├── e2e/                End-to-end suites that drive a built binary
│   └── cli/            CLI golden paths: real binary, temporary home, scripted model
├── mock/               Test-only in-memory stores
├── testsupport/        Test-only helpers that must not ship (JWT signing)
│   └── mockllm/        Scripted model replies over the three LLM wire protocols
└── util/               ID generation, prefixed IDs, workspace path resolution,
                        small string and time helpers
```

## Dependency Direction

```text
bootstrap ──▶ interface / server / service / agentapp / infra ──▶ core
```

- `core` imports nothing from `config`, `infra`, `service`, `server`,
  `agentapp`, or `interface`. It is pure domain.
- `config` does env and file loading only; it does not import infra
  implementations.
- `infra` imports nothing from `bootstrap`, `interface`, or `server`.
- `server` imports nothing from `bootstrap`, `config`, or `interface`.
- `service` imports nothing from `bootstrap`, `interface`, or `server`. A
  service is reached by a transport and never reaches back for one.
- `agentapp` imports nothing from `bootstrap`, `interface`, or `server`. Every
  surface that assembles it sits above it.
- `gorm.io` is imported only by `infra/db`. Above that boundary, "no such row"
  is `model.ErrNotFound`, which the store translates to.
- `mock` and `testsupport` are imported only from `_test.go` files. Neither may
  be reached from code that ships.
- `internal/tool` is not pure — it imports infra (MCP, git) as needed.

These rules are enforced by tests in `internal/architecture`. If a change trips
one, the import is the problem, not the test.

One exception is recorded rather than enforced: `service/conversation/runtime`
imports `agentapp` for `NewNonInteractivePolicy`. That one call is the whole
dependency and the policy it returns needs only `core`, so the import is
removable — but it is real today, and a rule that fails teaches contributors to
ignore the suite.

## Frontends

| Directory | Package | Notes |
|---|---|---|
| `gui/` | `@buildmax/gui` | Shared presentational React components and theme. Build output in `gui/dist/`. |
| `portal/` | — | Depends on gui via `"@buildmax/gui": "file:../gui"` |
| `desktop/frontend/` | — | Depends on gui via `file:../../gui` |

Portal and Desktop share **widgets**, not logic — data, auth, and routing are
each app's own. Both run React 19.

## Related

- [architecture/](architecture/README.md) — what each subsystem does
- [CONTRIBUTING.md](../../CONTRIBUTING.md) — build, test, and run
