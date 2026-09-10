# Agent Core Trust Harness

> **简体中文：** [阅读中文镜像](../zh-CN/design/信任保障.md)

## Contents

- [Status](#status)
- [1. Purpose](#1-purpose)
- [2. Direction](#2-direction)
- [3. Key Capabilities To Support](#3-key-capabilities-to-support)
- [4. Explicitly Out Of Scope For Now](#4-explicitly-out-of-scope-for-now)
- [5. Suggested Priority](#5-suggested-priority)
- [6. Acceptance](#6-acceptance)

## Status

- roadmap_priority: `R0`
- status: `in_progress` — hooks, durable traces, the Bash sandbox, process
  limits, Agent/Space sandbox tiers, worker baseline selection, and worker API
  ingress isolation are implemented. Fail-closed treatment for worker stdio
  MCP, candidate boundary evidence, and resolved policy presentation remain
  open. Pod-wide egress and an outer runtime are conditional post-Beta hardening
- follows: P0 Agent Core stability, P1 Local agent experience, and P2 Portal outcome surface — all complete; their plans were retired (see git history)
- roadmap: [../ROADMAP.md](../ROADMAP.md)
- created_at: `2026-05-23`

## 1. Purpose

The completed P0 work made the shared Agent Core stable enough for CLI, Desktop,
Portal, and worker task runs. The remaining R0 work closes the one known process
path around the worker sandbox and makes the supported boundary verifiable.

This document intentionally stays at the product-capability level. It records
the trust capabilities already delivered and the bounded work that remains.

## 2. Direction

The remaining R0 work focuses on the supported unattended-worker profile. CLI,
Desktop, Portal, and worker should still expose shared core capabilities in
surface-appropriate ways.

The goal is:

> Users and operators can understand, control, and debug Agent runs.

## 3. Key Capabilities To Support

### 3.1 Runtime Hooks — shipped ✅

The runtime hook system is implemented; detail design lives in
[hook-system.md](./hook-system.md). Highlights:

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
`buildmax hooks` inspector. See [hook-system.md](./hook-system.md) §10
(implementation phase F).

### 3.2 Sandbox And Execution Boundaries — local sandbox, worker surface, process limits, and hook boundary shipped ✅, `buildmax sandbox overrides` open

Explicit sandbox modes for command execution now exist. Detail design lives in
[sandbox-boundaries.md](./sandbox-boundaries.md);
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
- process execution limits — ✅ `sandbox.process.{max_cpu_seconds,
  max_memory_mb,max_processes,max_open_files}` become `ulimit` statements
  prefixed onto the wrapped command, one per limit; verified against real
  Alpine and macOS shells, including a CPU-time limit actually killing a
  busy loop on Linux (`max_memory_mb` is a documented no-op on macOS —
  Darwin's `setrlimit` has no `RLIMIT_AS`). See
  [sandbox-boundaries.md](./sandbox-boundaries.md) §13 phase D.
- worker/container execution mode — ✅ **wired and verified against the
  production pod security context**: `agentapp/taskrun` sets
  `SandboxSurface: config.WorkerSandboxSurface()`, which selects
  `SandboxSurfaceWorker` only when `BUILDMAX_SANDBOX_BACKEND_INSTALLED` is
  set (an `ENV` line in both worker Dockerfiles) — selecting the strict
  baseline unconditionally was tried first and broke every worker task on a
  bare Linux host or native Windows outright (`fail_if_unavailable: true`
  with no backend to satisfy it), caught by CI rather than by local
  development on a Mac, where Seatbelt always exists. `internal/infra/k8s/job.go`'s
  `RuntimeDefault` seccomp profile — which drops `bwrap`'s required syscalls
  once the worker pod's capabilities are empty — is replaced by a `Localhost`
  profile built for this
  ([deployment/seccomp/README.md](../../deployment/seccomp/README.md)
  has the full root-cause chain, including a second, independent kernel
  restriction on mounting `/proc` under `--unshare-pid` inside a container).
  Verified against a real pod carrying the worker's exact security context
  and the profile as the reference `DaemonSet` actually distributes it, and
  by an organic end-to-end run the deployment smoke now performs
  automatically: it arms its mock model to make a real dispatched task call
  `Bash` through the actual server → worker → Job path and asserts on the
  tool result, not the task's scripted final text; see
  [sandbox-boundaries.md](./sandbox-boundaries.md) §13 phase F.

`command` and `http` hook transports now consult `SandboxView` too — a hook
mirrors the same `WrapBashCommand`/`HostAllowed` calls `Bash`/`WebFetch`
make, with no `dangerously_disable_sandbox`-equivalent escape hatch, since
hooks are config-authored automation rather than an LLM-chosen call an
operator is watching turn by turn. Verified against a real `sandbox.Manager`
(Seatbelt), not only a test double. Still open in
[sandbox-boundaries.md](./sandbox-boundaries.md): `buildmax sandbox
overrides` (§8) is not implemented. §3.2 is therefore **not** fully closed,
but only that one operator-facing command and its documentation remain —
the enforcement engine, the worker surface, process limits, downgrade
marking, and the hook boundary have all landed.

### 3.3 Durable Run Trace — phase 1 shipped ✅

A durable run trace now persists the runtime event stream for every run. Detail
design lives in [durable-run-trace.md](./durable-run-trace.md).
Phase 1 shipped: a bounded, redacted JSONL trace written at the single
`agentapp.RunPrompt` chokepoint, so CLI/TUI, Desktop, eval, and worker runs all
produce traces with no per-surface code. Each run writes
`<DataDir>/sessions/<session_id>/traces/<run_id>.jsonl` (run id prefix `rt_`) with a
`run_start` record, a `sandbox_boundary` record, per-iteration
`llm_*`/`tool_*`/`context_compacted` records, and a terminal `run_end`. Disable via `BUILDMAX_TRACE_DISABLED`. Fail-open: a
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
- subagent parent/child relationships — ✅ each subagent trace carries its
  immediate parent's `parent_run_id`
- sandbox mode and boundary decisions — partial: a `sandbox_boundary` record is
  written for every run with the resolved enabled/mode/backend and the source
  chain, including an explicit `sandboxed: false` when nothing confined the run.
  Per-command boundary decisions and violations are still ❌ (see §3.2)
- memory and instruction sources used for the run — ❌ deferred

Deferred to follow-ups (see [durable-run-trace.md](./durable-run-trace.md) §7):
activity-view UI, a `buildmax trace` inspector, the records marked ❌ above,
and retention/GC of the traces directory.

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

Keep three contracts distinct: instructions are normative protocol, memory is
fallible Agent-curated recall, and session history is the ordered evidence of a
conversation. A compaction summary is a lossy history projection, not a
long-term memory merely because it helps recall.

Memory should be scoped and visible. Session notes and todos are the shipped
working-memory scope. Shared CLI/Desktop Project identity and bounded Project
Memory are planned in [local-project-memory.md](local-project-memory.md).
Global user memory, space memory, and reusable Agent memory remain separate
future scopes rather than meanings assigned to `AGENTS.md` or agent
instructions.

The Agent should expose which instruction, memory, and history-projection
sources were loaded for a run. Users should be able to inspect, update, delete,
or disable memory. BuildMax should avoid silently persisting sensitive or
surprising information, and Memory must never override instruction sources such
as `AGENTS.md`, skills, subagent definitions, or agent instructions.

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
- reject stdio MCP unless its child process can enter the declared boundary
- record enough trace data for Portal diagnostics
- load only the memory and instructions appropriate for the space/run scope
- make denied actions understandable
- avoid hiding local/remote capability drift

### 3.9 Pod-Wide Worker Egress — deferred conditional hardening

The shipped command boundary already constrains model-selected Bash and WebFetch
network access, and hook transports consult the same policy. The worker control
channel also has its own TLS listener and an ingress NetworkPolicy; see
[worker-api-network-boundary.md](./worker-api-network-boundary.md). These controls
do not impose a destination allow-list on every process in the Pod. General
worker egress and the storage identity available to the worker remain explicit
residual limits.

That wider boundary is not required for the first Beta, which supports one
trusted Space on a private network. Making it an R0 gate would introduce a CNI
or operated proxy before BuildMax has evidence for the legitimate destinations
real runs need, and an allowed Git host, package registry, model endpoint, or
upload service could still receive intentionally exfiltrated data. The Beta
candidate must record the residual limit rather than imply containment it does
not provide.

Reopen Pod-wide destination enforcement when evidence changes the supported
threat model, including any of these conditions:

- mutually untrusted tenants share worker infrastructure;
- arbitrary untrusted repositories are a supported input;
- workers hold high-value credentials whose exfiltration requires a Pod-wide
  control rather than the existing command and hook boundaries; or
- an operator requires and is prepared to operate a deployment-wide egress
  allow-list.

Before selecting an implementation, record the destinations representative runs
actually need and the deployment environments that must support enforcement.
Portable Kubernetes `NetworkPolicy`, Cilium FQDN rules, and a dedicated egress
proxy have materially different portability, hostname-authority, protocol, and
operating costs. None is an unconditional BuildMax dependency until that evidence
selects it. Per-Space policy, alternate-DNS and direct-IP bypass qualification,
and an outer runtime such as gVisor belong to the same conditional hardening
decision, not the current R0.

The immediate process-boundary problem is smaller and does remain in R0: stdio
MCP servers start as direct worker child processes. The supported unattended
worker profile must reject them unless BuildMax can launch them inside the
declared sandbox boundary. Disabling an unsupported transport is sufficient for
the first Beta; BuildMax does not need to create a general Pod-egress product in
order to make that claim.

## 4. Explicitly Out Of Scope For Now

Do not include these in the current R0 trust-boundary scope:

- Pod-wide destination policy, a dedicated egress proxy, or CNI selection
- gVisor or another outer worker runtime
- user-selectable checkpoint rollback or timeline restore
- automatic workspace write-back or merging
- full Portal audit product
- workflow engine rewrite
- plugin marketplace
- IDE extension
- container/seccomp implementation in Go
- broad versioned workspace implementation

Task workspace checkpointing and restore-before-Continue have since shipped
under [task-workspace-checkpoints.md](task-workspace-checkpoints.md). Generic
workspace history, user-selected rollback, merging, and automatic Space-file
write-back remain separate product questions and do not block R0.

## 5. Suggested Priority

The remaining R0 implementation order is:

1. Reject worker stdio MCP unless the child process can use the declared
   sandbox boundary.
2. Present the resolved sandbox and MCP treatment where an operator diagnoses a
   TaskRun.
3. Exercise the existing command, hook, resource, and worker API boundaries
   with the candidate artifacts.

Other trust-harness improvements follow demonstrated user or operator needs and
do not block the first private Beta.

## 6. Acceptance

The remaining R0 trust-boundary work is successful when:

- no stdio MCP child runs outside the boundary claimed by the supported worker
  profile;
- missing required enforcement stops the run before model execution;
- an operator can see the resolved sandbox and MCP treatment for a TaskRun; and
- candidate deployment evidence verifies the already-supported Bash, hook,
  process-limit, and worker API controls.

Pod-wide egress and outer-runtime isolation remain truthful, accepted limits for
the first private Beta. They become release gates only if the supported threat
model changes.
