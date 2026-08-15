# Support Matrix

> **Audience:** users and operators · **Status:** current
>
> BuildMax is alpha. This page defines what the project tries to support today,
> what is best-effort, and what is deliberately out of scope for now.

## Maturity Levels

| Level | Meaning |
|---|---|
| **Supported** | Intended to work for early users; covered by regular local or CI checks. |
| **Beta** | Usable, but interfaces or deployment shape may still change. |
| **Experimental** | Implemented enough to try, but not a stability promise. |
| **Best effort** | May work, but is not a release-blocking path today. |
| **Not supported** | Known gap or explicit non-goal. |

## Product Surfaces

| Surface | Status | What to expect |
|---|---|---|
| CLI print mode, `buildmax -p` | **Supported** | Primary local entry point. Reads, edits, greps, and runs commands in one workspace. |
| TUI, `buildmax` | **Supported** | Primary interactive local experience: sessions, slash panels, streaming, model/workspace visibility. |
| `buildmax init` and `buildmax doctor` | **Supported** | First-run configuration and local setup checks. |
| Local sessions and run traces | **Supported** | Session persistence and bounded JSONL traces under `BUILDMAX_HOME`. |
| Desktop app | **Beta** | Local chat/session experience using the shared runtime. Built from source; unsigned; not distributed as an end-user installer. |
| Portal frontend | **Beta** | Team UI for conversations, issues, workflows, agents, files, usage, and artifacts. Production auth is not done. |
| Server + local-process worker | **Beta** | Useful for trusted private deployments and development. Requires MySQL, blob storage, worker token, and model config. |
| Kubernetes worker mode | **Experimental** | Manifest and job generation are tested, but the in-cluster path is not covered by end-to-end CI. |
| Inbound webhooks | **Beta** | Authenticated by per-user webhook keys; payload extraction is configurable. |

## Operating Systems

| Platform | CLI/TUI | Server/worker | Desktop | Sandbox | Notes |
|---|---|---|---|---|---|
| macOS arm64 | **Supported** | **Supported** | **Beta** | **Supported** with Seatbelt | Primary local development platform. |
| macOS amd64 | **Supported** | **Supported** | **Beta** | **Supported** with Seatbelt | Release archive target. |
| Linux amd64 | **Supported** | **Supported** | Not supported | **Supported** with `bwrap` | Primary deployment target. |
| Linux arm64 | **Supported** | **Supported** | Not supported | **Supported** with `bwrap` | Release archive and container target. |
| Windows amd64 | **Beta** | **Beta** | **Beta** | Not supported | CI builds and tests Windows. Shell behavior differs from Unix; use WSL2 for setup/deployment workflows. |
| WSL2 | **Best effort** | **Best effort** | Not supported | **Supported** with `bwrap` | Recommended Windows path for Unix shell workflows. |

## Distribution

| Artifact | Status | Notes |
|---|---|---|
| Release archives for CLI/server/worker | **Supported** | Linux amd64/arm64, macOS amd64/arm64, Windows amd64. |
| `go install github.com/gougoujiang/buildmax/cmd/buildmax@latest` | **Supported** | CLI only. Uses the module version, without release archive provenance metadata. |
| `ghcr.io/gougoujiang/buildmax` | **Beta** | Contains CLI, server, and worker binaries. |
| `ghcr.io/gougoujiang/buildmax-portal` | **Beta** | Static Portal image; API base URL is configured at container start. |
| Desktop binary releases | Not supported | Build from source. Published, signed installers are not part of the alpha release path. |
| npm package for `@buildmax/gui` | Not supported | The shared GUI package is consumed by this repository through local `file:` dependencies. |

## Runtime And Model Providers

| Area | Status | Notes |
|---|---|---|
| OpenAI-compatible chat completions | **Supported** | Configured with `models:` entries in `settings.yaml` or `server.yaml`. |
| OpenRouter | **Supported** | Default quickstart path. |
| OpenAI-compatible local gateways | **Beta** | Works when the endpoint implements compatible chat completion behavior. |
| Provider-specific APIs | Not supported | BuildMax uses the OpenAI-compatible surface instead of provider-native SDK features. |
| Built-in model hosting | Not supported | Bring your own provider, gateway, or local inference server. |
| Multi-modal generation, voice, or browser automation | Not supported | Current runtime tools are text, files, shell, MCP, hooks, skills, and subagents. |

## Deployment And Security

| Capability | Status | Notes |
|---|---|---|
| Local single-user CLI/TUI | **Supported** | Start in a git working tree you can diff and revert. |
| Trusted private server deployment | **Beta** | Suitable for local labs or trusted networks after reading deployment docs. |
| Public internet server exposure | Not supported | `POST /api/login` is disabled by default and production auth is not wired. |
| Development fixed OTP | **Experimental** | One code signs in every registered user. Use only on a laptop or trusted network. |
| JWT user API and team membership authorization | **Beta** | User API uses JWT; team membership is the resource boundary. |
| Worker token auth | **Beta** | Guards worker routes by task run id. Treat it as sensitive infrastructure credential. |
| Bash sandbox | **Beta** | Off by default for local runs. Covers `Bash` subprocesses, not every tool or process on the host. |
| Runtime hooks | **Beta** | Can observe or block selected lifecycle/tool events. Hook failures fail open. |
| Durable run traces | **Supported** | On by default, bounded and redacted; failures do not break runs. |
| Audit log and approval workflow | Not supported | Planned, but not implemented as a governance surface. |

## Non-Goals For The Alpha

- Production identity management. Real OIDC/OAuth/SAML login, invite flows, and
  passwordless OTP delivery are not part of the current server.
- Multi-tenant public SaaS hosting. BuildMax is aimed at local use and private
  deployments, not running an untrusted public shared service.
- A guarantee that model-selected code is safe. Treat every run as executing
  untrusted commands with your credentials and network access.
- Full sandboxing for every operation. The sandbox targets `Bash`; file tools,
  MCP tools, hooks, and provider calls have separate boundaries.
- A stable public plugin marketplace or hosted integration catalog. Use MCP,
  local skills, hooks, and configuration for extension today.
- A workflow engine replacement for Airflow, Temporal, GitHub Actions, or CI.
  Workflows are lightweight reusable agent plans, not a general orchestrator.
- A Git hosting or IDE replacement. Git is used for visibility and recovery;
  BuildMax does not replace review, merge, or repository management workflows.
- Native mobile apps, browser extensions, or published desktop installers.
- Backward-compatible storage/schema guarantees before a stable release.

## How To Read This Page

When a path is **Supported**, it should be reasonable for an early adopter to
try and report bugs against it. When a path is **Beta** or **Experimental**, bug
reports are welcome, but fixes may include interface changes. When a path is
listed as **Not supported**, issues are still useful as signal, but the project
will not treat the gap as a release blocker until the roadmap says otherwise.
