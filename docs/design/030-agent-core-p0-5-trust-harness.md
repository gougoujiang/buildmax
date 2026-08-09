# Agent Core P0.5 Trust Harness

## Status

- roadmap_priority: `P0.5`
- status: `draft`
- follows: [024-agent-core-stability.md](./024-agent-core-stability.md), [025-local-agent-experience.md](./025-local-agent-experience.md), [026-portal-outcome-surface.md](./026-portal-outcome-surface.md)
- roadmap: [../ROADMAP.md](../../ROADMAP.md)
- created_at: `2026-05-23`

## 1. Purpose

P0 made the shared Agent Core stable enough for CLI, Desktop, Portal, and worker
task runs. P0.5 should make the core more trustworthy and easier to operate.

This document intentionally stays at the product-capability level. It lists the
key things BuildMax should support next, without prescribing detailed
implementation shape.

## 2. Direction

P0.5 should focus on the shared Agent Core first. CLI, Desktop, Portal, and
worker should expose the same core capabilities in surface-appropriate ways.

The goal is:

> Users and operators can understand, control, and debug Agent runs.

## 3. Key Capabilities To Support

### 3.1 Runtime Hooks — shipped ✅

The runtime hook system is implemented; detail design lives in
[031-hook-system-v2.md](./031-hook-system-v2.md). Highlights:

**Configuration locations** — entries from both layers merge additively:
- Global: `<BUILDMAX_HOME>/settings.yaml` under `hooks:`
- Workspace: `<workspace>/.buildmax/hooks.yaml`

**Transports** — every entry chooses one via `type:`:
- `command` (default) — shell command, JSON on stdin
- `http` — POST JSON to a URL
- `mcp_tool` — invoke a tool on a connected MCP server
- `prompt` — single-turn LLM judge

**Events shipped (13)**:

| Event | Gating? | Anchor |
|---|---|---|
| `SessionStart` / `SessionEnd` | no | `agentapp.OpenSession` / `CloseSession` |
| `UserPromptSubmit` | **yes** | `agentapp.RunPrompt` (before history append) |
| `PreToolUse` | **yes** | `applyPolicyAndExecute` (after policy, before exec) |
| `PostToolUse` / `PostToolUseFailure` | no | tool success / error paths |
| `Notification` | no | around the approval flow (`approval_required`, `permission_denied`) |
| `PreCompact` | **yes** | before context compaction |
| `PostCompact` | no | after a successful compaction |
| `SubagentStart` / `SubagentStop` | no | subagent runner |
| `Stop` | no | main-agent successful exit |
| `StopFailure` | no | any error exit (main or subagent) |

Use cases unlocked: formatting / linting (`post_tool_use`), policy checks
(`pre_tool_use`), external approvals (`notification` + `pre_tool_use`),
audit export (`stop` / `subagent_stop` / `post_tool_use_failure`).

**Subagent inheritance**: subagents share the parent HookManager; every
event payload from a subagent run is stamped with `is_subagent` and
`agent_type` so audit hooks can attribute. Subagents cannot bypass parent
hooks.

**Deferred** to follow-ups: `agent` transport (CC experimental), skill /
subagent frontmatter hooks (session-scoped lifetime), `async` command flag,
`buildmax hooks` inspector. See doc 031 §10 (Implementation steps phase F).

### 3.2 Sandbox And Execution Boundaries — local sandbox shipped ✅, worker hardening open

Explicit sandbox modes for command execution now exist. Detail design lives in
[032-sandbox-and-execution-boundaries.md](./032-sandbox-and-execution-boundaries.md);
its phases A–E are implemented. The sandbox isolates **bash subprocesses**
(Seatbelt on macOS, `bwrap` on Linux/WSL2, unavailable elsewhere); non-bash
tools keep their existing permission boundary. Config resolves from
`settings.yaml` + `policy.yaml` + `BUILDMAX_SANDBOX_ENABLED` with per-surface
defaults, and `buildmax sandbox status|deps|mode|enable|disable` plus the TUI
footer make the active mode visible.

Boundary coverage against the list above:

- workspace filesystem access — ✅ OS backend bind/profile rules
- external directory access — ✅ `filesystem.allow_write` / `deny_read` etc.
- network access — ✅ Go-side HTTP/SOCKS proxy with domain allow/deny
- environment variable exposure — ✅ secret-shaped vars scrubbed from bash env
- process execution limits — ❌ not implemented (no rlimits; 032 §13 phase D)
- worker/container execution mode — ❌ **the stricter worker default is not
  wired**: `agentapp/taskrun` builds its AgentApp without
  `SandboxSurface: SandboxSurfaceWorker`, so worker runs fall back to the
  local CLI baseline (`enabled: false`). 032 §13 phase F.

Also still open from 032: `command` / `http` hook transports do not consult
`SandboxView` yet (032 §9, §12), and `buildmax sandbox overrides` (032 §8) is
not implemented. §3.2 is therefore **not** closed — the enforcement engine
landed, the operator-facing worker hardening did not.

### 3.3 Durable Run Trace — phase 1 shipped ✅

A durable run trace now persists the runtime event stream for every run. Detail
design lives in [034-durable-run-trace.md](./034-durable-run-trace.md).
Phase 1 shipped: a bounded, redacted JSONL trace written at the single
`agentapp.RunPrompt` chokepoint, so CLI/TUI, Desktop, eval, and worker runs all
produce traces with no per-surface code. Each run writes
`<DataDir>/traces/<session_id>/<run_id>.jsonl` (run id prefix `rt_`) with a
`run_start` record, per-iteration `llm_*`/`tool_*`/`context_compacted` records,
and a terminal `run_end`. Disable via `BUILDMAX_TRACE_DISABLED`. Fail-open: a
trace failure never breaks or slows a run.

Traces are bounded and redacted so they are useful for debugging without
leaking secrets.

Coverage against the full §3.3 target (✅ = in phase 1):

- model calls — ✅ `llm_start` / `llm_end`
- tool calls — ✅ `tool_start` / `tool_end` / `tool_denied`
- context compaction — ✅ `context_compacted`
- errors — ✅ `run_end.error`; retries are not surfaced by the event stream yet
- token usage and timing — ✅ per-call tokens, tool duration, record timestamps
- approval decisions — partial: only `tool_denied` (reason `hook`/`user`)
- hook execution — ❌ needs dedicated hook events
- file changes — ❌ needs file-change events
- subagent parent/child relationships — ❌ `parent_run_id` linkage deferred
- sandbox mode and boundary decisions — ❌ needs sandbox events (see §3.2)
- memory and instruction sources used for the run — ❌ deferred

Deferred to follow-ups (see 034 §7): activity-view UI, a `buildmax trace`
inspector, the records marked ❌ above, and retention/GC of the traces
directory.

### 3.3 Durable Run Trace — phase 1 shipped ✅

A durable run trace now persists the runtime event stream for every run. Detail
design lives in [034-durable-run-trace.md](./034-durable-run-trace.md).
Phase 1 shipped: a bounded, redacted JSONL trace written at the single
`agentapp.RunPrompt` chokepoint, so CLI/TUI, Desktop, eval, and worker runs all
produce traces with no per-surface code. Each run writes
`<DataDir>/traces/<session_id>/<run_id>.jsonl` (run id prefix `rt_`) with a
`run_start` record, per-iteration `llm_*`/`tool_*`/`context_compacted` records,
and a terminal `run_end`. Disable via `BUILDMAX_TRACE_DISABLED`. Fail-open: a
trace failure never breaks or slows a run.

Deferred to follow-ups (see 034 §7): activity-view UI, a `buildmax trace`
inspector, subagent child-trace linkage (`parent_run_id`), dedicated
hook/approval/file-change/sandbox records (need new events), and retention/GC.

### 3.4 Activity Views

Support lightweight activity views in local surfaces.

TUI and Desktop should let users inspect:

- what the Agent is doing now
- what tools were used
- which approvals happened
- what changed
- why a run failed or stopped

Normal chat should remain clean; activity should be progressive disclosure.

### 3.5 Doctor And Diagnostics

Support a local diagnostic flow for setup and runtime problems.

It should check:

- model configuration
- workspace permissions
- git availability
- active sandbox mode
- active memory and instruction sources
- MCP configuration and health
- skill and subagent discovery
- hook configuration
- BuildMax data directory health

Diagnostics should produce actionable messages and a redacted summary that can
be shared when debugging.

### 3.6 Memory And Instructions

Support a clear memory mechanism for persistent instructions and reusable
context.

Memory should be scoped and visible:

- user memory: durable user preferences and working style
- workspace memory: project-specific conventions and recurring facts
- team memory: shared team guidance for Portal and worker runs
- agent memory: guidance attached to a specific Agent capability
- session memory: conversation-specific summary and current task state

The Agent should expose which memory sources were loaded for a run. Users should
be able to inspect, update, and delete memory. BuildMax should avoid silently
persisting sensitive or surprising information.

Memory should complement existing instruction sources such as `AGENTS.md`,
skills, subagents, and agent instructions. It should not replace them.

### 3.7 Subagent Traceability

Support clearer visibility for subagent execution.

Users should be able to see:

- which subagent ran
- why it was invoked
- what memory and instruction sources it received
- what tools it used
- what runtime boundaries applied
- what result it returned
- how it relates to the parent run

Subagents must not bypass parent runtime policy.

### 3.8 Safer Worker Execution

Support worker-specific trust behavior for non-interactive runs.

Worker runs should:

- fail closed when approval would be required
- run with explicit sandbox boundaries
- record enough trace data for Portal diagnostics
- load only the memory and instructions appropriate for the team/run scope
- make denied actions understandable
- avoid hiding local/remote capability drift

## 4. Explicitly Out Of Scope For Now

Do not include these in the current P0.5 scope:

- checkpoint and rollback
- full workspace restore
- full Portal audit product
- workflow engine rewrite
- plugin marketplace
- IDE extension
- container/seccomp implementation in Go
- broad versioned workspace implementation

Checkpoint and rollback can be revisited later with the P5 versioned workspace
work, but they should not block this P0.5 pass.

## 5. Suggested Priority

Recommended implementation order:

1. Durable run trace
2. Sandbox and execution boundaries
3. Memory and instructions
4. Activity views
5. Doctor and diagnostics
6. Runtime hooks
7. Subagent traceability
8. Safer worker execution polish

This order gives the team better visibility first, then better control, then
more extensibility.

## 6. Acceptance

P0.5 is successful when:

- users can inspect what happened in a run
- users can understand tool approval and denial decisions
- users and operators can understand active sandbox boundaries
- local setup problems are easy to diagnose
- worker runs produce useful diagnostic traces
- memory is scoped, inspectable, and user-controllable
- hooks can support common automation use cases
- subagent behavior is attributable and policy-bound
