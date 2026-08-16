# buildmax

[![CI](https://github.com/gougoujiang/buildmax/actions/workflows/ci.yml/badge.svg)](https://github.com/gougoujiang/buildmax/actions/workflows/ci.yml)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

Build Everything with AI.

BuildMax is an out-of-the-box, privately deployable Agent platform. One Go agent
runtime, three ways to reach it:

- **CLI/TUI** — one user, one directory, one terminal
- **Desktop** — the same local capability in a native Wails app
- **Portal** — a team: shared work, background execution, results, governance

All three run the same agent loop, the same tools, and the same MCP, skill, and
subagent behavior. Use only the local surfaces, deploy only the Portal for your
company, or both.

> **Status: Alpha.** Interfaces, deployment guidance, and runtime behavior may
> change before a stable release. Server authentication is bootstrap-level —
> an operator creates accounts and issues single-use login codes, and there is
> no password, SSO, or recovery flow. Read
> [docs/deploy/authentication.md](docs/deploy/authentication.md) before exposing
> a server.

## Quickstart

Download a binary from [Releases](https://github.com/gougoujiang/buildmax/releases),
or:

```bash
go install github.com/gougoujiang/buildmax/cmd/buildmax@latest
```

Configure a model — this writes `~/.buildmax/settings.yaml`:

```bash
buildmax init --api-key sk-your-key-here
buildmax doctor
```

That sets up `openai/gpt-4o-mini` through OpenRouter. Any OpenAI-compatible
endpoint works; `buildmax init --model llama3.1 --api-url http://localhost:11434/v1`
points it at a local one instead. Omit `--api-key` to fill the key in later.
`buildmax doctor` checks the local setup without contacting a model provider.

Then run it against a directory:

```bash
buildmax -p "Summarize what this project does"   # one prompt, print the answer
buildmax                                         # interactive TUI
```

The current directory is the agent's workspace — it reads, greps, edits files,
and runs shell commands there, for real. Start in a git tree you can revert, or
in [`sample-data/`](sample-data/README.md) — fifteen throwaway datasets that
exist so you can point the agent at something and watch it work.

Full walkthrough: **[docs/start/quickstart.md](docs/start/quickstart.md)**.

## Documentation

**[docs/](docs/README.md)** is the index.

| | |
|---|---|
| [Install](docs/start/install.md) · [Quickstart](docs/start/quickstart.md) · [Support matrix](docs/start/support.md) · [Concepts](docs/start/concepts.md) | Getting started |
| [Hooks](docs/guide/hooks.md) · [Sandbox](docs/guide/sandbox.md) | Controlling what the agent may do |
| [Compose quickstart](docs/deploy/compose.md) · [Local kind](docs/deploy/local-kind.md) · [Deployment](docs/deploy/overview.md) · [Authentication](docs/deploy/authentication.md) | Running it for a team |
| [Configuration](docs/reference/configuration.md) · [CLI](docs/reference/cli.md) · [Webhook](docs/reference/webhook.md) | Reference |
| [docs/ROADMAP.md](docs/ROADMAP.md) · [Design records](docs/design/README.md) | Where it is going, and why |
| [Contributing](CONTRIBUTING.md) · [Support](.github/SUPPORT.md) · [Changelog](CHANGELOG.md) | Project participation and releases |

## How It Works

The Portal separates talking from doing:

```text
Tier 1  conversation  ──creates──▶  Tier 2  task / task_run
   ▲                                            │
   └──────────── reports back ──────────────────┘
```

Tier 1 is the conversation orchestrator and the only voice to the user. Tier 2
is background execution: a worker materializes the team's files into a run
directory, runs the shared agent runtime, writes artifacts, and reports back. A
long job never blocks the conversation, and its result always returns through
the conversation that started it.

More: [docs/start/concepts.md](docs/start/concepts.md) ·
[docs/contribute/architecture/](docs/contribute/architecture/README.md)

## Build From Source

```bash
./make doctor     # check contributor tool versions without changing anything
./make build      # strict CLI, server, worker, GUI, Portal, Desktop build
./make test       # go test ./... against ./testing-sandbox
./make run server # run the already-built buildmax-server
./make run portal # Portal dev server
```

On Windows use `make.bat` with the same commands — both forward to the Go task
runner in `cmd/mk`. `./make help` shows the common contributor path;
`./make help all` shows advanced, deployment, and release commands. None
of build, test, or lint needs a model API key.

Two directories in the tree are fixtures rather than product code:
[`sample-data/`](sample-data/README.md) holds the datasets above — upload them
into a team workspace to give a fresh Portal deployment something to work on, or
point the CLI at one — and `eval/` holds the agent benchmark's
deliberately-broken Go modules, run by `./make eval`.

New here? **[docs/contribute/first-pr.md](docs/contribute/first-pr.md)** is the
whole path from clone to pull request. Repository tree:
[docs/contribute/repo-layout.md](docs/contribute/repo-layout.md).

## Security

BuildMax invokes model-selected tools and shell commands. Treat every runtime
configuration as an execution boundary: dedicated credentials, least-privilege
workspace access, an explicit network policy. The [bash
sandbox](docs/guide/sandbox.md) and [runtime hooks](docs/guide/hooks.md) tighten
that boundary, but do not replace reviewing what a deployment is allowed to
reach. Never commit credentials.

Report vulnerabilities privately: [SECURITY.md](SECURITY.md).

For setup questions and early ideas, use
[GitHub Discussions](https://github.com/gougoujiang/buildmax/discussions).

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md) for development checks, architectural
boundaries, and pull request guidance. Community participation follows the
[Code of Conduct](.github/CODE_OF_CONDUCT.md); support routes and project decision rules
are documented in [SUPPORT.md](.github/SUPPORT.md) and [GOVERNANCE.md](.github/GOVERNANCE.md).

## License And Name

Licensed under the [Apache License 2.0](LICENSE). The BuildMax name and logo are
not granted by that license; see [TRADEMARKS.md](.github/TRADEMARKS.md).
