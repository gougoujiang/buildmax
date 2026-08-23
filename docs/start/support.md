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
| Portal frontend | **Beta** | Team UI for conversations, issues, workflows, agents, files, usage, and artifacts. Password and login-code flows work; wider public exposure remains unsupported. |
| Server + local-process worker | **Beta** | Useful for trusted private deployments and development. The Compose path is covered by a full TaskRun and artifact smoke test. |
| Kubernetes worker mode | **Beta** | The local kind path exercises MySQL, MinIO, Ingress, a worker Job, and artifact retrieval end to end. Deployment APIs may still change. |
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
| OpenAI Responses API | **Supported** | Set `provider: openai`; text, tools, streaming, reasoning state, prompt-cache usage, and image input use the shared LLM contract. |
| Anthropic Messages API | **Supported** | Set `provider: anthropic`; the native adapter supports the same shared contract, including reasoning state and prompt caching. |
| Built-in model hosting | Not supported | Bring your own provider, gateway, or local inference server. |
| Multi-modal generation, voice, or browser automation | Not supported | Current runtime tools are text, files, shell, MCP, hooks, skills, and subagents. |

## Deployment And Security

| Capability | Status | Notes |
|---|---|---|
| Local single-user CLI/TUI | **Supported** | Start in a git working tree you can diff and revert. |
| Trusted private server deployment | **Beta** | Suitable for local labs or trusted networks after reading deployment docs. |
| Docker Compose quickstart | **Beta** | Fast contributor and single-machine path. Uses a local-process worker and local filesystem storage. |
| Local kind deployment | **Beta** | Kubernetes contribution path, with its own MySQL and MinIO. A development environment, not a deployment template. |
| Private Kubernetes deployment against your own dependencies | **Beta** | `deployment/production/` is a plain-YAML reference plus the contract each dependency has to meet. Written to be read and adapted; not applied as-is, and not yet exercised against a real cloud account. |
| Public internet server exposure | Not supported | Password and operator-issued login-code flows exist, but login is not rate limited and there is no SSO or second factor. Put an identity-aware and rate-limiting boundary in front before wider exposure. |
| Operator-issued login codes | **Beta** | Single-use account-claim and recovery credential, delivered out of band because BuildMax has no mail channel. |
| JWT user API and team membership authorization | **Beta** | User API uses JWT; team membership is the resource boundary. |
| Run-token worker auth | **Beta** | Every dispatched worker receives a credential scoped to one task run, and it is the only credential the worker routes accept. The old shared worker token is removed. |
| Bash sandbox | **Beta** | Off by default on every surface. Covers `Bash` subprocesses, not every tool or process on the host. |
| Worker pod containment | **Beta** | Worker Jobs run non-root with no service-account token, all capabilities dropped, and a read-only root filesystem. This — not the sandbox — is the boundary a Kubernetes deployment relies on. Workers still receive object-storage credentials. |
| Runtime hooks | **Beta** | Can observe or block selected lifecycle/tool events. Hook failures fail open. |
| Durable run traces | **Supported** | On by default, bounded and redacted; failures do not break runs. |
| Audit log | **Beta** | Records sign-ins, membership changes, model catalog changes, and refused requests. Owner-only, in the API and in Portal. A failed write is logged and dropped, so it records what happened while the database was reachable rather than guaranteeing every action was recorded. |
| Approval workflow | Not supported | Planned; no gate exists today. |

## Compatibility

What an upgrade may and may not do to a deployment. Where a promise does not
exist yet, this says so rather than implying one.

### Database schema — a promise, and it is enforced

The schema moves **forward only**. There are no down migrations, and the
`Migration` type has no `Down` field to write one in.

What is supported is rolling the **binary** back one release: schema version N
keeps serving code from release N-1. That puts one requirement on every change
— nothing is removed in the same release that stops using it, so a removal
takes two — and it is why an upgrade that goes wrong is recovered by
redeploying the previous image tag.

Rolling a **database** back is not supported. Recovery from a bad schema change
is a restore from backup, so take one before an upgrade that crosses a release
carrying migrations. A binary that meets migrations from a later release logs a
warning and keeps running, because that is the N-1 promise working; a binary
several releases behind has no such promise.

### HTTP API — no version, so expect change

There is no `/v1` prefix and no negotiated version. `internal/server/handlers/routes.go`
is the list of routes and `openapi.json` describes them, but neither is a
stability contract during Beta.

Expect additions to be additive and safe. Do not assume a route, a field, or a
status code will survive a release without reading the changelog. A breaking
change will be called out there; it will not be prevented.

The one exception is the managed inference wire contract, which carries an
explicit version (`llmwire.Version`) precisely because CLI, Desktop, and worker
builds move independently of the server. A change to its shape needs a new
version rather than a silent edit.

### Configuration — additive, with removals announced

New keys are added with defaults that preserve existing behaviour, and
`config-examples/server.example.yaml` carries every key the server reads — a
test fails when it does not.

Removals and renames are called out in the changelog. There is no deprecation
period and no compatibility shim, so a key you rely on can disappear in one
release with a note rather than two releases with a warning.

Credentials never gain defaults. A setting that decides *where* a deployment
connects or *as whom* is left unset rather than pointed somewhere plausible.

### Stored data

Run artifacts, run state, and durable traces are written under a documented key
layout in object storage and are not rewritten by an upgrade. BuildMax never
deletes them; retention is the operator's to configure.

The audit trail is the one exception, and only when asked: setting
`audit.retention_days` in `server.yaml` expires events older than that window,
and each sweep records what it removed as an `audit.pruned` event. The default
is to keep everything. The trail can be downloaded as CSV or JSONL — a space
owner gets their own, a System Administrator gets the deployment's — and a
download is itself recorded.

There is no export or import command for a deployment's data as a whole, so
moving one means moving its database and its bucket together.

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

## How To Read This Page

When a path is **Supported**, it should be reasonable for an early adopter to
try and report bugs against it. When a path is **Beta** or **Experimental**, bug
reports are welcome, but fixes may include interface changes. When a path is
listed as **Not supported**, issues are still useful as signal, but the project
will not treat the gap as a release blocker until the roadmap says otherwise.
