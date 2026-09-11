---
id: worker-mcp-diagnostics
title: Show worker MCP treatment in TaskRun diagnostics
roadmap: R0
source: docs/design/trust-harness.md#6-acceptance
depends_on: [10-worker-stdio-mcp-policy.md]
verification:
  - ./make test ./internal/infra/trace
  - ./make test ./internal/server/handlers/work
  - ./make check portal
  - ./make e2e local
  - ./make kind smoke
  - ./make e2e kind
claim:
---

## Outcome

An operator diagnosing a TaskRun can see the worker profile's MCP treatment
beside the recorded sandbox boundary, without inferring it from configuration or
assuming that an older trace enforced a policy it never recorded.

## Scope

- Record a bounded, machine-readable summary of the resolved worker MCP
  treatment in the durable run trace for runs that reach Agent execution.
- Extend the trace summary and TaskRun trace response with that summary while
  preserving the distinction between recorded treatment and an older or absent
  record.
- Present the treatment next to the existing sandbox-boundary description in
  Portal Run Details.
- Make the pre-run refusal from the prerequisite task equally understandable in
  the TaskRun failure outcome when no trace could be opened.
- Add Go summary/handler tests, Portal unit tests, and a browser journey for the
  diagnostic presentation.
- Update current-state, architecture, user documentation, and the changelog.

## Out Of Scope

- Persisting a second MCP-policy copy in a database row when the trace and
  terminal TaskRun outcome already cover the two lifecycle cases.
- Exposing MCP command lines, arguments, environment values, URLs containing
  credentials, or tool results in the summary.
- General MCP health administration, per-tool activity views, Pod-wide egress,
  or candidate-release qualification.
- Changing which transports the worker accepts; the prerequisite task owns that
  policy.

## Acceptance Criteria

- A current worker trace says explicitly that stdio is disabled by the
  unattended-worker profile and identifies the resolved remote transport kinds,
  if any, without containing configuration secrets or command details.
- A trace written before this record existed is shown as unknown rather than as
  allowed, disabled, or confined.
- The trace summary API preserves the new structured fields and its existing
  bounds and redaction behavior.
- Portal displays the MCP treatment beside the sandbox boundary with distinct
  copy for enforced, unknown, and pre-run refusal cases.
- A worker rejected before trace creation leaves a terminal TaskRun error that
  names the policy and remediation; Portal does not hide that failure behind an
  empty trace state.
- Existing local traces and TaskRun diagnostics render without regression.

## Verification

Run the trace and handler packages first, then `./make check portal`. Use
`./make e2e local` for the browser edit loop. Because the task changes both the
worker-side record and its presentation, finish against an isolated kind
cluster:

```text
BUILDMAX_KIND_EPHEMERAL=1 ./make kind up
./make kind smoke
./make e2e kind
./make kind down
```

These checks prove the producer, API contract, and operator presentation
separately. No provider credential or real model is required.

## Notes

The existing path is `sandbox_boundary` in `internal/infra/trace`, summarized
by `internal/infra/trace/summary.go`, served by the work trace handler, and
rendered by `portal/src/features/runs/RunTraceModal.tsx`. Extend that path rather
than creating a second diagnostic endpoint. Keep absence meaningful, as the
current boundary summary already does for older traces.
