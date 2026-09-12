---
id: plugin-search-inspect-tools
title: Add read-only PluginSearch and PluginInspect agent tools
roadmap: R5
source: docs/design/plugin-space-distribution.md#16-task-scoped-autonomous-acquisition
depends_on: []
verification: ["./make test", "./make e2e cli"]
claim:
---

## Outcome

An Agent that discovers its selection lacks a capability first needs to see what
is available and what a candidate package would introduce, before any install is
considered. This adds the two read-only typed tools for that, with no acquisition
or state change.

## Scope

- Add `PluginSearch` and `PluginInspect` tool-name constants to
  `internal/tool/names.go` and their implementations.
- `PluginSearch` queries the catalog within the Space activation ceiling and
  returns bounded, LLM-meaningful results on hit and miss.
- `PluginInspect` returns the sanitized capability report already produced by
  `internal/service/plugininspect` (skills, subagents, MCP, hooks, env refs) for
  a named release.
- Tool output is written for the LLM and is meaningful on success and failure.

## Out Of Scope

- `PluginInstall` and the pending environment revision (task 84).
- The Space acquisition policy gate — search/inspect are read-only and bounded by
  the existing activation ceiling, not by acquisition authority (task 84 owns the
  acquisition gate).
- The environment revision model (task 80); these tools do not need it.

## Acceptance Criteria

- Both tools are registered and appear in `internal/tool/names.go`.
- Results stay within the Space activation ceiling and never leak packages the
  Space cannot see.
- Inspection reuses `plugininspect` output rather than a second report path.
- Output is bounded and useful to the model on both hit and miss.

## Verification

- `go-unit`: tool registration; ceiling enforcement; hit/miss output shape;
  inspection parity with the existing report.

## Notes

Independent of task 80 and low risk, so it can run in parallel. `PluginInstall`
is deliberately split into task 84 because it introduces state and an
authorization gate that search/inspect do not.
