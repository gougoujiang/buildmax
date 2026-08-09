# BuildMax Documentation

Organized by what you are trying to do.

## Start Here

| | |
|---|---|
| [start/install.md](start/install.md) | Get the binaries |
| [start/quickstart.md](start/quickstart.md) | First agent run, in five minutes |
| [start/concepts.md](start/concepts.md) | Surfaces, the agent loop, teams, and the two tiers |

## Use It

| | |
|---|---|
| [guide/tools.md](guide/tools.md) | The built-in tools, their arguments, and the path boundary |
| [guide/agents-md.md](guide/agents-md.md) | Give the agent project instructions on every run |
| [guide/skills-and-subagents.md](guide/skills-and-subagents.md) | Reusable workflows, and delegating with a restricted tool set |
| [guide/mcp.md](guide/mcp.md) | Connect MCP servers for tools BuildMax does not ship |
| [guide/hooks.md](guide/hooks.md) | Observe or block prompts, tool calls, and compaction |
| [guide/sandbox.md](guide/sandbox.md) | Confine `Bash` by filesystem path and network domain |
| [guide/sessions-and-traces.md](guide/sessions-and-traces.md) | Resume conversations; inspect what a run actually did |
| [guide/troubleshooting.md](guide/troubleshooting.md) | When something does not work |

## Run It For A Team

| | |
|---|---|
| [deploy/overview.md](deploy/overview.md) | Topology, requirements, configuration, containers |
| [deploy/authentication.md](deploy/authentication.md) | **Read before exposing a server** — login is disabled by default |
| [deploy/local-kind.md](deploy/local-kind.md) | One-command local cluster: kind, MinIO, MySQL, Redis |

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
| [contribute/repo-layout.md](contribute/repo-layout.md) | The repository tree and dependency direction |
| [contribute/architecture/](contribute/architecture/README.md) | How each subsystem works today |
| [contribute/documentation.md](contribute/documentation.md) | Documentation conventions |
| [contribute/dependency-licenses.md](contribute/dependency-licenses.md) | License audit and how to re-run it |

## Why It Is Like This

| | |
|---|---|
| [../ROADMAP.md](../ROADMAP.md) | Active priorities and sequencing |
| [design/](design/README.md) | Numbered design records — specifications and roadmap plans |
| [../SECURITY.md](../SECURITY.md) | Vulnerability disclosure and operator responsibilities |

## Conventions

Every document opens with its audience and status, so you can tell in one line
whether it can be trusted:

```markdown
> **Audience:** operators · **Status:** current
```

There is no archive directory — retired documents are deleted, and git history
keeps them. The rules for writing and retiring documentation are in
[contribute/documentation.md](contribute/documentation.md).
