---
id: worker-stdio-mcp-policy
title: Reject stdio MCP in the unattended worker profile
roadmap: R0
source: docs/design/trust-harness.md#5-suggested-priority
depends_on: []
verification:
  - ./make test ./internal/agentapp
  - ./make test ./internal/agentapp/taskrun
  - ./make test
  - ./make kind smoke
claim:
---

## Outcome

An official unattended worker never starts an stdio MCP child outside the
boundary it reports. A resolved stdio server fails the run before either that
child or the model starts, while local interactive surfaces and remote MCP
transports keep their existing behavior.

## Scope

- Carry an explicit worker-surface policy into AgentApp construction; do not
  infer that a process is a worker from whether the optional sandbox backend is
  installed.
- Inspect the fully resolved MCP configuration, including global, workspace,
  and activated-plugin layers, before constructing the MCP registry.
- Refuse every `stdio` entry on the worker surface and return a deterministic,
  actionable error naming the rejected server IDs and the supported remote
  transports.
- Keep `stdio` available to CLI, TUI, and Desktop runs, and keep worker `http`
  and `sse` transports available.
- Add focused construction and task-run tests proving the fail-closed ordering.
- Update the MCP and worker-boundary documentation and add the required
  user-visible changelog entry.

## Out Of Scope

- Confining stdio children inside the Bash sandbox or building a new process
  sandbox for MCP.
- Pod-wide destination policy, an egress proxy, CNI-specific controls, or an
  outer runtime such as gVisor.
- Removing stdio MCP from local surfaces or rejecting a plugin at publication
  time merely because it declares stdio.
- Candidate-release qualification; the later boundary-evidence task owns that
  exercise after the engineering prerequisites land.

## Acceptance Criteria

- A worker run with one or more resolved stdio MCP servers returns a terminal,
  operator-readable failure before the configured command creates any
  observable side effect and before the first LLM request.
- The refusal applies when the stdio entry came from global configuration,
  workspace configuration, or an activated plugin, and rejected server IDs are
  sorted so diagnostics are stable.
- A worker without the sandbox-backend marker still applies the worker MCP
  policy; the marker may select a sandbox implementation but cannot select the
  security contract.
- Worker `http` and `sse` MCP configurations still initialize, and local stdio
  configurations still initialize, under focused regression tests.
- The error tells an operator that stdio is unsupported by the unattended
  worker profile and that a remote transport is required; it does not print
  command arguments, environment values, or secrets.
- Current-state, user-facing MCP/worker documentation, and a changelog entry
  describe the supported behavior without claiming Pod-wide containment.

## Verification

Start with focused AgentApp and worker task-run tests that use a command with a
detectable side effect and assert that it never executes. Then run `./make test`.
Because this changes worker runtime assembly, use an isolated kind cluster and
finish with its deployment smoke:

```text
BUILDMAX_KIND_EPHEMERAL=1 ./make kind up
./make kind smoke
./make kind down
```

The focused tests prove ordering and surface selection. The ordinary suite
proves shared-runtime regressions are absent. The kind smoke proves the official
worker image still completes an ordinary run under its supported boundary.

## Notes

Current construction starts MCP from `internal/agentapp/app_builder.go`, and
`internal/infra/mcp/transport.go` creates stdio children with `exec.Command`.
Worker task runs enter AgentApp through `internal/agentapp/taskrun/runtime.go`.
The existing `SandboxSurface` cannot by itself identify every worker because
`config.WorkerSandboxSurface` deliberately falls back when the image marker is
absent; this task needs an explicit runtime-surface fact.
