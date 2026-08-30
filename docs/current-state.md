# BuildMax Current State

> **Audience:** maintainers and contributors · **Status:** current as of 2026-08-30

This document is a code-first assessment of BuildMax at `origin/main` commit
`67e9e4df77d42351c435fd21d74422c67a9f8a38`. It answers what the repository
actually implements and how close those implementations are to dependable use.
It is not derived from the roadmap, proposals, design records, or feature copy.

The code remains the source of truth. The maturity percentages below are
engineering judgments, not mechanically calculated completion scores or release
promises. Update this assessment when a material capability or readiness
boundary changes; put future sequencing in the [roadmap](ROADMAP.md).

## Executive Assessment

BuildMax is no longer a prototype. It contains a substantial shared Agent
runtime, complete local surfaces, a broad team/server domain, background
workers, a Portal, deployment assets, and an unusually serious test and release
harness for an Alpha project.

It is also not yet a production-safe multi-tenant Agent platform. Three gaps
dominate the assessment: unattended worker execution is not wired to the worker
sandbox baseline, the reference deployment runs multiple Server replicas while
live coordination remains process-local, and ordinary pull-request tests do not
exercise the real MySQL store.

| Target | Current maturity | Assessment |
|---|---:|---|
| Local general-purpose Agent | 80–85% | Useful and broadly implemented; remaining work is mostly reliability evidence, edge cases, and surface polish. |
| End-to-end private team platform | 60–65% | The vertical path exists across Server, Portal, worker, artifacts, traces, and deployment, but several operational loops are incomplete. |
| Production-safe multi-tenant platform | 45–55% | Authorization and governance foundations exist, but execution containment, multi-instance correctness, persistence evidence, and account operations are below the production bar. |

The most accurate short description is:

> BuildMax is a capable Alpha Agent runtime with a broad, working platform
> shell. Its next milestone should be proving safety and operational
> correctness, not adding another large feature family.

## Evidence Snapshot

The repository at the assessment base contains:

- 108 Go packages and 1,005 tracked Go files;
- 119 documented HTTP operations across 95 paths;
- 30 row types passed to the store's authoritative `AutoMigrate` call;
- 2,605 Go test, benchmark, and example functions;
- three BuildMax-owned black-box evaluation tasks.

The reassessment exercised the contributor environment, full build, Go and
frontend checks, CI-equivalent checks, and the fast CLI and Desktop end-to-end
suites. They passed. Go statement coverage measured 56.1%; the database package
measured 3.7% without a configured MySQL integration DSN.

The reassessment did not start the Compose, local deployment, or kind suites,
because those mutate Docker or cluster state. The standard store integration
tests skip when `BUILDMAX_TEST_DSN` is absent, so a passing default suite is not
evidence that MySQL behavior was exercised. Real-provider smoke and evaluation
runs were also not repeated: they spend credentials or tokens and answer a
different question from whether code is wired.

## What Is Implemented

### Shared Agent Runtime

The shared Go runtime implements a real streamed model/tool loop rather than a
surface-specific demo. It includes tool-call recovery, parallel read-only tool
execution, permissions and approvals, loop guards, context compaction and
checkpoints, hooks, bounded redacted traces, cost and usage statistics,
sessions, notes, todos, Project Memory, subagents, worktrees, background jobs,
and partial cancellation.

The built-in tool surface covers file reading and mutation, search, Bash, web
fetching, skills, subagents, MCP, notes and todos, memory, worktrees, jobs and
monitors, Issues, and artifacts. Model assembly supports OpenAI-compatible
chat, OpenAI Responses, Anthropic, and Ollama paths.

CLI/TUI and Desktop assemble this shared runtime. They are functional local
Agent products, not thin placeholders for Portal.

### Team And Background Platform

The Server implements authentication, teams, agents and revisions, issues and
comments, workflows and revisions, conversations, tasks and task runs, worker
claim/report flows, artifacts and files, traces, a managed LLM gateway, quota,
audit, system administration, and a plugin catalog and activation model.

Background execution supports local-process and Kubernetes Job launch modes,
direct and managed inference, team-home materialization, run-scoped homes,
artifact publication, heartbeats, cancellation, retry, stale-run recovery, and
result delivery back into Tier 1 conversations.

Portal exposes the main collaboration and administration journeys. The
production tree also includes Compose, kind, Kubernetes, release, SBOM,
vulnerability-scan, smoke, and browser-test infrastructure.

### Evaluation

The evaluation framework is structurally sound: versioned task and trial
contracts, built-binary local and worker adapters, deterministic, command, and
trace graders, repeated and paired experiments, failure bundles, and a pinned
Harbor/Terminal-Bench adapter exist. The oracle smoke and one-task canary verify
that external path for one task only. There is no Terminal-Bench score.

## Readiness Blockers

### P0 — Worker Execution Is Not Contained By Default

The worker task runtime constructs `agentapp.AppConfig` without setting
`SandboxSurface` and uses `AllowAllPolicy` in
[`internal/agentapp/taskrun/runtime.go`](../internal/agentapp/taskrun/runtime.go).
The application builder resolves an empty sandbox surface to the CLI baseline
in [`internal/agentapp/app_builder.go`](../internal/agentapp/app_builder.go).

The stricter worker baseline therefore exists in configuration but is not
selected by worker runs. Kubernetes pod hardening limits the container, but it
does not make the in-process Bash sandbox active. Local-process execution is in
the Server trust domain. This is a release blocker for unattended execution,
not optional defense in depth.

Completion requires an explicit worker surface, a documented fail-closed or
recorded-downgrade policy when the OS backend is unavailable, resource limits,
and tests that prove the effective boundary. Hook and MCP child processes need
their own boundary; Bash containment alone does not cover them.

### P0 — The Reference Replica Count Exceeds Coordination Semantics

The production manifest configures two Server replicas in
[`deployment/production/buildmax.yaml`](../deployment/production/buildmax.yaml),
but the live stream hub explicitly identifies itself as in-memory in
[`internal/server/websocket/hub.go`](../internal/server/websocket/hub.go).
WebSocket connection registration and per-conversation turn serialization are
also process-local in
[`internal/server/websocket/registry.go`](../internal/server/websocket/registry.go)
and [`internal/server/turnqueue/turnqueue.go`](../internal/server/turnqueue/turnqueue.go).

With multiple Server replicas, a worker update, browser connection, or
conversation turn can land on different processes. Durable database state will
eventually converge, but live deltas, notification delivery, and the
single-conversation serialization guarantee can be missed or split.

Until distributed coordination exists, the supported production topology must
use one Server replica. Alternatively, implement a shared stream/pub-sub,
connection delivery strategy, and distributed conversation lock/queue before
advertising horizontal Server scaling.

### P0 — The Default Gate Does Not Prove MySQL Behavior

The store tests in
[`internal/infra/db/store_test.go`](../internal/infra/db/store_test.go) skip
when `BUILDMAX_TEST_DSN` is not set. Deployment smoke covers important real
paths after merge or when run deliberately, but it is not a pull-request gate.
Low database coverage makes schema, query, transaction, and MySQL-specific
regressions disproportionately likely to escape the default suite.

Completion requires a hermetic MySQL integration scope in CI, migration and
rollback fixtures, and explicit coverage of critical state transitions. This
does not require putting every end-to-end deployment suite on every pull
request.

## Product And Operating Gaps

These are material, but they should follow the three P0 boundaries unless a
deployment partner supplies evidence that changes the order.

### P1 — Account And Team Operations

- Signup can create an account that still has neither a password nor a login
  code. The code states this directly in
  [`internal/service/identity/account.go`](../internal/service/identity/account.go);
  an operator must finish access manually.
- Team policy defines owner, admin, and member roles. The membership service
  now covers the full lifecycle — invitation bounded to an existing account,
  role promotion and demotion, unilateral ownership transfer, and
  member-scoped login-code recovery — in
  [`internal/service/team/service.go`](../internal/service/team/service.go),
  [`internal/server/handlers/team/teams.go`](../internal/server/handlers/team/teams.go),
  and Portal's Space → Members and Account → Invitations surfaces. Bringing in
  someone who has never had a BuildMax account is still deliberately a
  `system_admin` operation, not a team-scoped one — see
  [`design/team-membership-lifecycle.md`](design/team-membership-lifecycle.md)
  §1 for why account creation and team membership are kept as two different
  authorities.
- System administration, quotas, role checks, and audit exist, but team-level
  approvals and a complete sensitive-action governance loop do not.

### P1 — Qualification Breadth

Three product-owned tasks are enough to prove the evaluation architecture, not
the product's capability or reliability claims. Add representative suites for
tool use, context durability, cancellation, failure recovery, permission
boundaries, conversation delivery, and deployment behavior before using
evaluation results for release qualification. Add performance and soak evidence
separately; correctness trials do not measure throughput or resource behavior.

### P2 — Workflow, Channels, And Plugins

- Workflow definitions contain a linear list of `agent_task` steps in
  [`internal/core/workflow/workflow.go`](../internal/core/workflow/workflow.go).
  Branching, parallelism, manual approval, loops, and explicit input/output
  mapping are absent.
- Channel names include Portal, Telegram, cron, and webhook in
  [`internal/service/conversation/channel/types.go`](../internal/service/conversation/channel/types.go),
  but only Portal and inbound webhook paths are assembled. Telegram and cron
  are vocabulary, not shipped adapters; the webhook callback sender is not
  assembled by the Server.
- Team background runs can materialize activated skill and subagent content,
  but plugin releases containing hooks or MCP servers are rejected by
  [`internal/service/plugin/activation.go`](../internal/service/plugin/activation.go),
  and Tier 1 conversations do not load team plugins.

### P2 — Surface And Throughput Evidence

Desktop has useful bridge-level coverage but not full window automation. Portal
has browser coverage, although its production bundle remains a large single
chunk. The local scheduler intentionally dispatches one run at a time; this is
acceptable as a conservative default but is not evidence of sustained
throughput. None of these is a reason to block containment or correctness work.

## Rebased Priority Order

1. Wire and prove the worker execution boundary, including hook and MCP child
   processes.
2. Make the production topology honest: one supported Server replica now, or
   shared coordination before horizontal scaling.
3. Put critical MySQL persistence behavior in the pull-request evidence path.
4. Close what remains of account and team operations: signup still leaves an
   account without a credential until an operator finishes it, by design, and
   team-level approvals are unscheduled. Team role lifecycle, ownership
   transfer, and member-scoped recovery are done.
5. Expand product-owned qualification from an architectural slice into a
   representative release suite.
6. Deepen workflows, real channel adapters, executable team plugins, Portal
   performance, Desktop automation, and throughput based on observed demand.

This ordering treats safety, consistency, and evidence as product capability.
It deliberately does not make another broad feature area the next milestone.
