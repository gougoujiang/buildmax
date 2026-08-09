# P0 Agent Core Stability

## Status

**All planned P0 items complete. P0-2 (lightweight) and P0-3 are intentionally deferred.**

| Item | Status | Commit |
|------|--------|--------|
| P0-1 Permission and approval model | ✅ Done | `2a2c293` policy, `fef37c5` TUI approval, `9d6785e` desktop approval |
| P0-2 Sandbox and network boundary | ✅ Done (lightweight) | bash risk classification (Ask/Deny), sensitive file Ask, merged into `safety.go` |
| P0-3 Patch, diff, and rollback | ⏭ Deferred | — |
| P0-4 Context management / compaction | ✅ Done | `f85c117` |
| P0-5 Unified runtime events | ✅ Done | EventSink in agent core; CLI TUI + Desktop surface integration |
| P0-6 Local/remote runtime parity | ✅ Done | Worker MCP now config-driven |
| P0-7 Failure recovery and loop guards | ✅ Done | `c8710d6` |
| P0-8 Subagent maturity | ✅ Done | Policy inheritance, model routing, MaxIter, reply truncation |
| P0-9 LLM client resilience | ✅ Done | Per-call timeout, error classification, retry/backoff |
| P0-10 Agent eval coverage | ✅ Done | 13-task benchmark harness (5 simple / 5 medium / 3 complex), token efficiency stats |

### P0-2 实现范围说明

进程级沙箱（cgroup/seccomp/容器）属于部署层责任，不在 Go 代码中实现。实际落地了三项：

- **bash 命令风险分类**：catastrophic（`rm -rf /`、raw device write）→ Deny；risky（`rm`、`curl`、`npm`、`chmod`、`sudo` 等）→ Ask；其余 → Allow
- **敏感文件保护**：`read_file`、`write_file`、`edit_file`、`grep` 对 `.env*`、`*.pem/key`、`id_rsa`、`.aws/credentials` 等路径返回 Ask
- **Ask 语义**：有 ApprovalHandler（CLI/Desktop 交互模式）→ 弹审批；无 ApprovalHandler（Worker 无人值守）→ 自动折叠为 Deny，fail safe

env var 过滤暂不做。进程沙箱由部署侧（Docker `--network none` + 资源限制）保障。

This document supports the P0 item in [../ROADMAP.md](../../ROADMAP.md). It
describes what BuildMax must improve in the shared Agent Core before we expand
Portal workflows, team collaboration, or advanced workspace/versioning features.

## Product Position

BuildMax is an out-of-the-box, privately deployable enterprise Agent platform.

The same Go Agent Core should power:

- CLI/TUI and Desktop local execution
- Portal conversation and issue execution
- Worker task runs

Therefore, P0 is not about making one surface better. It is about making the
shared Agent Core safe, stable, observable, recoverable, and consistent across
all surfaces.

## Reference Baseline

Community-leading coding agents such as Codex CLI, Claude Code, and OpenCode
have converged around a similar runtime shape:

- a local or private execution harness around the model
- explicit permissions and approval modes
- filesystem and network boundaries
- patch/diff based editing and review
- context compaction for long-running work
- event logs, tool traces, and auditability
- memory/instruction files loaded by scope
- MCP, skills/plugins, and subagents as extension mechanisms

Useful references:

- [OpenAI: Running Codex safely at OpenAI](https://openai.com/index/running-codex-safely/)
- [OpenAI: Unrolling the Codex agent loop](https://openai.com/index/unrolling-the-codex-agent-loop/)
- [Claude Code settings](https://code.claude.com/docs/en/settings)
- [Claude Code security](https://code.claude.com/docs/en/security)
- [Claude Code memory](https://code.claude.com/docs/en/memory)
- [Claude Code subagents](https://code.claude.com/docs/en/subagents)
- [OpenCode permissions](https://opencode.ai/docs/permissions)
- [OpenCode MCP servers](https://opencode.ai/docs/mcp-servers/)

## Current BuildMax Baseline

BuildMax already has a working shared Agent loop:

- `internal/core/agent` owns the LLM -> tool call -> tool result loop.
- `internal/agentapp` assembles the LLM client, tools, MCP, skills, subagents,
  sessions, and workspace root.
- `internal/tool` provides `read_file`, `writefile`, `editfile`, `bash`,
  `glob`, `grep`, `webfetch`, `todowrite`, `skill`, `agentdef`, `task`, and MCP
  gateway tools.
- `internal/agentapp/taskrun` runs worker task runs through the same AgentApp
  shape, with run-scoped directories and persisted artifacts.
- `internal/service/conversation` reuses the core loop for Portal conversation
  turns and bridges to background task runs.

This means the foundation exists. The P0 problem is that the harness is still
too thin: tools execute directly, context is trimmed rather than compacted,
events are not unified, and worker/local capabilities are not yet guaranteed to
match.

## Gaps

| Priority | Gap | Current State | Why It Matters |
| --- | --- | --- | --- |
| P0-1 | Permission and approval model | Tools execute directly after model tool calls. File tools are root-scoped, but there is no shared `allow` / `ask` / `deny` policy. | Enterprise deployment needs predictable control over writes, shell commands, network calls, MCP tools, skills, and subagents. |
| P0-2 | Sandbox and network boundary | `bash` runs through the host shell in the workspace directory. There is no process sandbox, network policy, command risk classification, or secret-aware environment filtering. | A privately deployed Agent platform must make low-risk actions easy while blocking or escalating high-risk actions. |
| P0-3 | Patch, diff, and rollback | `writefile` overwrites files and `editfile` does exact replacement in place. There is no first-class patch preview, checkpoint, diff summary, or rollback model. | Users trust coding agents when they can inspect what changed and recover quickly. |
| P0-4 | Context management | `TrimHistory` keeps a suffix of recent messages using a rough token estimate. There is no compaction summary, task state, file-change summary, or tool-result compression. | Long tasks and background worker runs will lose intent and intermediate decisions. |
| P0-5 | Unified runtime events | Local logs, streaming deltas, task status, and Portal artifacts are separate mechanisms. | CLI, Desktop, Portal, worker, and enterprise audit should all consume the same Agent-native event stream. |
| P0-6 | Local/remote runtime parity | CLI/Desktop can enable MCP; worker task runs currently disable MCP. Tier 1, worker, and local tools are assembled through related but not fully declared capability contracts. | The product promise says local and Portal execution share one Agent Core capability model. |
| P0-7 | Failure recovery and loop guards | Tool failures are returned to the model; max iteration stops runaway loops. There is no repeated-call guard, categorized retry, command recovery policy, or resumable interrupted run state. | Stability depends on controlled failure modes, especially for unattended worker runs. |
| P0-8 | Subagent maturity | Built-in and user-defined subagents exist, but per-agent permissions, model selection, event visibility, budget limits, and isolation rules are incomplete. | Subagents are powerful, but they must not bypass the main runtime safety policy. |
| P0-9 | LLM client resilience | OpenAI-compatible model calls work, but model capability metadata, retry/backoff, rate-limit handling, timeout policy, and provider-specific behavior are thin. | "Out of the box" requires graceful behavior under real provider failures. |
| P0-10 | Agent eval coverage | Tool unit tests exist, but there is no Agent behavior regression suite for multi-step coding tasks, permissions, context compaction, worker parity, or rollback. | Without evals, improving the core will repeatedly risk regressions. |

## P0 Task Plan

### 1. Agent Runtime Contract

Define a shared runtime contract used by CLI, Desktop, Portal, and worker:

- `RunContext`: workspace, surface, user/team scope, model, mode, budgets
- `ToolPolicy`: permission, sandbox, network, external-directory, and secret rules
- `RunEvent`: model call, assistant delta, tool request, approval, tool result,
  file change, context compaction, warning, error, usage
- `Transcript`: durable model-facing messages plus runtime events
- `Artifact`: run-produced files and summaries

Acceptance:

- all surfaces can construct a run through the same contract
- capability differences are explicit policy decisions, not hard-coded runtime differences

### 2. Permission and Sandbox MVP

Add a policy layer before tool execution.

Minimum permissions:

- `read`
- `edit`
- `bash`
- `glob`
- `grep`
- `webfetch`
- `mcp`
- `skill`
- `task`
- `external_directory`
- `secret_file`
- `doom_loop`

Each rule should resolve to:

- `allow`
- `ask`
- `deny`

Local interactive surfaces can prompt on `ask`. Worker runs should use a
non-interactive policy: allow known-safe actions, deny or fail closed on unsafe
actions, and record the event.

Acceptance:

- sensitive files such as `.env` are denied by default unless explicitly allowed
- destructive shell commands can be blocked or require approval
- repeated identical tool calls trigger a loop guard
- MCP and subagent tool calls pass through the same policy layer

### 3. Unified Run Event Stream

Introduce structured events emitted from the Agent Core rather than assembled
separately by each surface.

Core events:

- `run_started`
- `model_request_started`
- `model_response_delta`
- `model_response_completed`
- `tool_requested`
- `tool_approval_requested`
- `tool_started`
- `tool_completed`
- `tool_failed`
- `file_changed`
- `context_trimmed`
- `context_compacted`
- `usage_reported`
- `run_completed`
- `run_failed`

Acceptance:

- CLI can show concise progress
- Desktop can render run details
- Portal can render a task/issue timeline
- enterprise deployments can export audit logs from the same event stream

### 4. Patch, Diff, and Checkpoint Editing

Move file mutation toward a reviewable editing model.

Tasks:

- add a patch-first editing path for multi-file changes
- capture pre-change file snapshots or git-backed checkpoints
- emit `file_changed` events with path, operation, and diff metadata
- produce a run-level diff summary
- support rollback to the pre-run checkpoint

Acceptance:

- every Agent-written file can be attributed to a run and tool call
- users can inspect a diff before accepting in local interactive modes
- Portal/worker runs preserve enough state to restore or replay decisions later

### 5. Context Compaction

Replace suffix-only trimming with semantic compaction.

The compacted context should preserve:

- active user goal
- relevant system and project instructions
- current plan/todo state
- files read and why they matter
- files changed and why
- important command/test results
- unresolved errors or decisions

Acceptance:

- long tasks can continue after compaction without losing goal, constraints, or changed-file state
- compaction itself is recorded as a runtime event
- system/project instructions are reloaded or preserved after compaction

### 6. Runtime Parity

Make local and remote execution use the same declared capability set.

Tasks:

- remove hard-coded capability differences where possible
- make worker MCP/skill/subagent availability policy-driven
- expose a capability report for CLI/Desktop/Portal/worker
- add parity tests for shared built-in tools

Acceptance:

- the same task has comparable capability in CLI, Desktop, and worker execution
- differences come from environment and policy, not different Agent implementations

### 7. Reliability Guards

Add defensive behavior around common failure modes.

Tasks:

- categorize LLM errors: auth, rate limit, timeout, provider error, malformed tool call
- add retry/backoff where safe
- set default tool timeouts by risk class
- add repeated tool-call detection
- improve output truncation with preserved head/tail and byte counts
- make cancellation and interruption produce resumable state where possible

Acceptance:

- unattended worker runs fail clearly and recoverably
- local users see actionable errors rather than raw provider/tool failures

### 8. Subagent Hardening

Make subagents first-class but policy-bound.

Tasks:

- per-subagent model, permissions, and tool budget
- event stream nested under parent run
- no implicit bypass of parent policy
- explicit rules for concurrent subagents
- tests for read-only explore agents and restricted shell agents

Acceptance:

- subagents improve task success without weakening runtime safety

### 9. Agent Eval Harness

Build a small regression suite for Agent Core behavior.

Initial eval cases:

- read and summarize a code path
- edit a file using existing conventions
- edit multiple files and produce a diff summary
- run tests and recover from one failure
- deny reading `.env`
- ask/deny risky bash commands
- compact context and continue
- spawn a read-only subagent
- run the same task locally and through worker with comparable results

Acceptance:

- P0 changes can be merged with confidence that core behavior has not regressed

## Suggested Implementation Order

1. Runtime contract and event model.
2. Permission policy wrapper around tool execution.
3. Bash/file safety defaults and sensitive-file deny rules.
4. Patch/checkpoint editing path.
5. Context compaction.
6. Worker/local capability parity.
7. Reliability guards and LLM retries.
8. Subagent hardening.
9. Agent eval harness.

## Non-Goals For P0

- large workflow engine rewrite
- complex enterprise audit UI
- full Git branch/restore product surface
- Portal-only Agent capabilities that bypass the shared runtime
- adding many new tools before the current tools are controllable and observable

## Decision Summary

P0 should make BuildMax's Agent Core a trustworthy execution kernel.

The first milestone is not "more agent features"; it is:

- permission and sandbox policy
- unified event stream
- patch/diff/checkpoint editing
- context compaction
- local/worker runtime parity

After those land, Portal outcomes, enterprise deployment, governance, and
versioned workspace work can build on a much firmer foundation.
