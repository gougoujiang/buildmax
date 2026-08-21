# BuildMax Documentation

Organized by what you are trying to do.

## Start Here

| | |
|---|---|
| [start/install.md](start/install.md) | Get the binaries |
| [start/quickstart.md](start/quickstart.md) | First agent run, in five minutes |
| [start/support.md](start/support.md) | Supported platforms, surfaces, deployment paths, and non-goals |
| [start/concepts.md](start/concepts.md) | Surfaces, the agent loop, teams, and the two tiers |
| [../sample-data/](../sample-data/README.md) | Fifteen throwaway datasets — upload them into a team workspace, or point the CLI at one |

## Use It

| | |
|---|---|
| [guide/tools.md](guide/tools.md) | The built-in tools, their arguments, and the path boundary |
| [guide/agents-md.md](guide/agents-md.md) | Give the agent project instructions on every run |
| [guide/skills-and-subagents.md](guide/skills-and-subagents.md) | Reusable workflows, and delegating with a restricted tool set |
| [guide/mcp.md](guide/mcp.md) | Connect MCP servers for tools BuildMax does not ship |
| [guide/tool-permissions.md](guide/tool-permissions.md) | Control which tool calls stop and ask before running |
| [guide/hooks.md](guide/hooks.md) | Observe or block prompts, tool calls, and compaction |
| [guide/sandbox.md](guide/sandbox.md) | Confine `Bash` by filesystem path and network domain |
| [guide/sessions-and-traces.md](guide/sessions-and-traces.md) | Resume conversations; inspect what a run actually did |
| [guide/troubleshooting.md](guide/troubleshooting.md) | When something does not work |

## Run It For A Team

| | |
|---|---|
| [deploy/compose.md](deploy/compose.md) | A team deployment on one machine, in about five minutes |
| [deploy/overview.md](deploy/overview.md) | Topology, requirements, configuration, containers |
| [deploy/authentication.md](deploy/authentication.md) | **Read before exposing a server** — accounts, login codes, what is missing |
| [deploy/local-kind.md](deploy/local-kind.md) | One-command local cluster and Kubernetes Job smoke |

## Look It Up

| | |
|---|---|
| [reference/configuration.md](reference/configuration.md) | Every config file field and environment variable |
| [reference/cli.md](reference/cli.md) | Commands, flags, slash commands |
| [reference/webhook.md](reference/webhook.md) | Triggering runs from external systems |

The HTTP API describes itself: `GET /openapi.json`, browsable at `/swagger/`.

## Change It

| | |
|---|---|
| [../CONTRIBUTING.md](../CONTRIBUTING.md) | Prerequisites, build, test, code boundaries, pull requests |
| [contribute/first-pr.md](contribute/first-pr.md) | Clone to pull request, start to finish, no API key needed |
| [contribute/conventions.md](contribute/conventions.md) | Naming, IDs, tool output, commit messages, changelog entries |
| [contribute/repo-layout.md](contribute/repo-layout.md) | The repository tree and dependency direction |
| [changelog/README.md](changelog/README.md) | How to add a changelog entry, and how a release folds them |
| [contribute/architecture/](contribute/architecture/README.md) | How each subsystem works today |
| [contribute/documentation.md](contribute/documentation.md) | Documentation conventions |
| [contribute/dependency-licenses.md](contribute/dependency-licenses.md) | License audit and how to re-run it |
| [contribute/releasing.md](contribute/releasing.md) | Versioning, publishing, verification, and release recovery |

## Why It Is Like This

| | |
|---|---|
| [ROADMAP.md](ROADMAP.md) | Active priorities and sequencing |
| [design/](design/README.md) | Product direction, active plans, and subsystem specifications |
| [../SECURITY.md](../SECURITY.md) | Vulnerability disclosure and operator responsibilities |

## Explore Future Directions

| | |
|---|---|
| [proposals/](proposals/README.md) | Early cross-cutting directions that are open for discussion, not roadmap commitments |

## Conventions

Every document opens with its audience and status, so you can tell in one line
whether it can be trusted:

```markdown
> **Audience:** operators · **Status:** current
```

There is no archive directory — retired documents are deleted, and git history
keeps them. The rules for writing and retiring documentation are in
[contribute/documentation.md](contribute/documentation.md).
