# Choose A Contribution Area

> **Audience:** contributors · **Status:** current
>
> BuildMax is an open-source Agent runtime for both local work and private team
> deployment. This page maps the problems where outside experience is most
> useful; the active priority order remains in [ROADMAP.md](../ROADMAP.md).

## Where Help Matters

### Agent Runtime

The shared runtime is the capability every surface depends on. Work here should
make CLI, Desktop, Portal conversations, and worker task runs better together.

Useful experience:

- Go concurrency, cancellation, streaming, and error handling
- LLM protocols and tool-calling behavior
- context durability and model state
- MCP, skills, subagents, plugins, and traces

Start with the [Agent Core architecture](architecture/agent-loop.md), then look
for open
[`help wanted`](https://github.com/gougoujiang/buildmax/issues?q=is%3Aissue+is%3Aopen+label%3A%22help+wanted%22)
or
[`agent-ready`](https://github.com/gougoujiang/buildmax/issues?q=is%3Aissue+is%3Aopen+label%3A%22agent-ready%22)
issues that name the runtime.

### Local Experience

CLI/TUI and Desktop are complete local entry points, not trial screens for the
Portal. They should make one Agent useful in one real workspace without a
BuildMax Server.

Useful experience:

- terminal UX and Bubble Tea
- Wails and React
- workspace, session, and result handling
- cross-platform packaging and diagnostics

Read the [CLI](architecture/cli.md), [TUI](architecture/tui.md), or
[Desktop](architecture/desktop.md) architecture for the surface you want to
change. Small usability fixes found while following the
[local quickstart](../start/quickstart.md) make good first contributions.

### Enterprise Platform

The Server, Portal, and workers turn the same Agent Core into a private team
platform: shared work, background execution, managed models, results, and
governance.

Useful experience:

- private Kubernetes and container operations
- APIs, persistence, schedulers, and background jobs
- React product surfaces
- identity, authorization, quota, and audit systems

Start with the [architecture overview](architecture/overview.md), then read the
specific active plan linked from the [Roadmap](../ROADMAP.md). A deployment
change should preserve the local path; a Portal change should not create a
second Agent implementation.

### Trust And Security

An agent reads files, calls remote systems, and executes model-selected code.
The project needs boundaries that are real, visible, testable, and honest about
their limits.

Useful experience:

- Linux and macOS process sandboxing
- credential scoping and secret handling
- authorization testing and threat modeling
- audit, redaction, retention, and runtime observability

Read the [trust harness](../design/trust-harness.md),
[sandbox boundaries](../design/sandbox-boundaries.md), and
[Security Policy](../../SECURITY.md) before changing a boundary. Report a
vulnerability privately; do not turn it into a public starter issue.

### Documentation And Contributor Experience

Documentation is part of the product contract. A mismatch between code and a
current document is a real bug, and a reproducible setup or test failure is
valuable even when the fix is small.

Useful starting points:

- [`good first issue`](https://github.com/gougoujiang/buildmax/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22)
- the [documentation rules](documentation.md)
- the [first pull request](first-pr.md) walkthrough
- gaps found by running `./make doctor`, `./make test`, or a quickstart

## How To Choose Work

Use the narrowest label that matches where you are:

| Label | What it promises |
|---|---|
| `good first issue` | Small enough for a first contribution, with a known direction |
| `help wanted` | Maintainers want outside help; it may require subsystem experience |
| `agent-ready` | Scope, acceptance criteria, and verification are explicit enough to implement without guessing at product intent |
| `documentation` | The expected behavior is known and the public explanation needs work |

`agent-ready` describes the issue, not the contributor. You may solve it by
hand, with an AI coding agent, or with both.

Before starting a large feature, new provider, new tool, persistence change, or
security-boundary change, open a Discussion or Issue first. Small fixes can go
straight to a focused pull request. [CONTRIBUTING.md](../../CONTRIBUTING.md)
contains the review and verification contract.

## Current Direction

The near-term center is not adding unrelated features to one surface. It is:

1. make Agent runs explainable and their execution boundaries visible;
2. make private deployment repeatable and diagnosable;
3. complete the practical team governance loop;
4. keep the local Agent experience complete while those enterprise layers grow.

Those statements are a summary, not a second roadmap. Check
[ROADMAP.md](../ROADMAP.md) and the linked design record before taking a task,
because shipped and open work changes faster than this orientation page.
