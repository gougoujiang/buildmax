# BuildMax Current State

> **Audience:** maintainers and contributors · **Status:** current as of 2026-09-02

This document is a code-first assessment of BuildMax. Its full sweep was made
at `origin/main` commit `67e9e4df77d42351c435fd21d74422c67a9f8a38`; the
sections have since been amended in place as the boundaries they describe
moved, most recently against `c8ef5bd`. It answers what the repository actually
implements and how close those implementations are to dependable use. It is
not derived from the roadmap, proposals, design records, or feature copy.

The code remains the source of truth. The maturity percentages below are
engineering judgments, not mechanically calculated completion scores or release
promises. Update this assessment when a material capability or readiness
boundary changes; put future sequencing in the [roadmap](ROADMAP.md).

## Executive Assessment

BuildMax is no longer a prototype. It contains a substantial shared Agent
runtime, complete local surfaces, a broad team/server domain, background
workers, a Portal, deployment assets, and an unusually serious test and release
harness for an Alpha project.

It is also not yet a production-safe multi-tenant Agent platform. Two gaps now
dominate the assessment: unattended worker execution now selects and passes
the worker sandbox baseline against the production pod's own hardening, but
MCP child processes and cluster-level network egress remain outside it,
and the reference deployment runs multiple Server replicas while live
coordination remains process-local. Pull-request tests do now exercise the real
MySQL store; what is left there is the breadth of the cases, not the gate.

| Target | Current maturity | Assessment |
|---|---:|---|
| Local general-purpose Agent | 80–85% | Useful and broadly implemented; remaining work is mostly reliability evidence, edge cases, and surface polish. |
| End-to-end private team platform | 60–65% | The vertical path exists across Server, Portal, worker, artifacts, traces, and deployment, but several operational loops are incomplete. |
| Production-safe multi-tenant platform | 45–55% | Authorization and governance foundations exist, worker Bash is confined, and persistence is in the pull-request evidence path. Multi-instance correctness, the MCP child-process boundary, cluster-level egress, and operating evidence from a real deployment are still below the production bar. The range is unchanged because what closed was mechanism, and what is left is the part no code change can supply. |

The most accurate short description is:

> BuildMax is a capable Alpha Agent runtime with a broad, working platform
> shell. Its next milestone should be proving safety and operational
> correctness, not adding another large feature family.

## Evidence Snapshot

The repository contains a shared Go runtime and four shipped binaries, a
server API described by the checked OpenAPI document, an authoritative GORM
schema, Go and frontend test suites, and BuildMax-owned black-box evaluation
tasks. Exact file, route, row, and test counts are intentionally omitted: they
change too frequently to be useful maintained facts.

The reassessment exercised the contributor environment, full build, Go and
frontend checks, CI-equivalent checks, and the fast CLI and Desktop end-to-end
suites. They passed. Go statement coverage measured 59.7% with a MySQL DSN
configured, so the store's own tests ran rather than skipping. The database
package measured 53.3% that way and 3.5% without a DSN — the difference is the
whole reason `./make test mysql` exists, and every pull request now runs it.

The reassessment did not start the Compose, local deployment, or kind suites,
because those mutate Docker or cluster state. The store integration tests still
skip under a plain `./make test`, which is why `./make test mysql` exists and
runs them on every pull request instead; a passing default suite remains no
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

### P0 — Worker Sandbox Wired And Verified Against The Production Pod Security Context; Cluster Egress Still Open

The worker task runtime now selects `config.SandboxSurfaceWorker` and applies
an agent-declared network/filesystem tier in
[`internal/agentapp/taskrun/runtime.go`](../internal/agentapp/taskrun/runtime.go),
resolved by the server at claim time and pinned onto the run for audit, per
[`docs/design/agent-sandbox-policy.md`](design/agent-sandbox-policy.md).

Selecting it unconditionally was tried first and broke CI outright: a bare
Linux host without `bwrap` installed, and every native-Windows worker (no
sandbox backend exists there at all), both hit
`SandboxSurfaceWorker`'s own `fail_if_unavailable: true` and refused to run
any task — caught by `evaluation`'s black-box worker-surface tests and a
Windows CI run, not by local development on a Mac, where Seatbelt is always
present and the failure never reproduces. `config.WorkerSandboxSurface`
now selects the strict baseline only when
`BUILDMAX_SANDBOX_BACKEND_INSTALLED` is set — an `ENV` line in
`Dockerfile.buildmax`/`Dockerfile.release`, present in every container built
from either image and therefore inside a `k8s_job` worker pod, absent on a
bare host, CI, or native Windows, which keep the CLI baseline exactly as
before this work started. An operator who has installed `bwrap` themselves
on a bare host can still opt in explicitly via `BUILDMAX_SANDBOX_ENABLED`.

Selecting the surface alone was not enough. Reproducing the worker Job's
initial non-root `PodSecurityContext` in a real pod and running
`bwrap` inside it failed outright: `RuntimeDefault` drops the
`unshare`/`setns`/`mount`/`umount2`/`pivot_root`/`clone`/`clone3` rules a
container's own default profile gates behind `CAP_SYS_ADMIN`, and an empty
capability set drops the gated rule from the compiled filter entirely, not
just the capability. `internal/infra/k8s/job.go` now requests a `Localhost`
profile built for exactly this — [`deployment/seccomp/worker-bwrap.json`](../deployment/seccomp/worker-bwrap.json),
Docker's own default profile with those seven syscalls made unconditional —
distributed to every node by a `DaemonSet`
([`deployment/buildmax-deploy.yaml`](../deployment/buildmax-deploy.yaml),
[`deployment/production/buildmax.yaml`](../deployment/production/buildmax.yaml)).
See [`deployment/seccomp/README.md`](../deployment/seccomp/README.md) for the
full root-cause chain.

A second, independent failure surfaced once namespace creation worked:
mounting a fresh `/proc` inside `--unshare-pid` triggered the kernel's "mount
too revealing" VFS protection (`SB_I_USERNS_VISIBLE`), reproducible even with
seccomp fully disabled and real root — a genuine container-runtime mount
namespace restriction, not a seccomp or capability gap.
[`internal/infra/sandbox/bwrap_linux.go`](../internal/infra/sandbox/bwrap_linux.go)
now re-binds the parent's `/proc` read-only instead of mounting a fresh one;
the accepted cost is a sandboxed process seeing the host container's process
list under `/proc` rather than an isolated one.

Both fixes were verified against a real pod carrying the worker's exact
security context and a `DaemonSet`-delivered profile, not a relaxed
stand-in: the full `bwrap` invocation `bwrap_linux.go` builds ran a real
command, correctly confined to the bound workspace and denied a write
outside it.

An organic run closes the loop: the deployment smoke now arms its mock model
(`internal/testsupport/mockllm`'s queued one-shot tool-call override, `GET
/control/requests` to read a tool result back) to make a real dispatched
task call `Bash` through the actual server → worker → Kubernetes Job path,
then asserts the *tool result* — not the task's scripted final text, which
answers the same regardless of what a tool did — shows the command ran and a
write outside the workspace was denied
(`tools/mk/deploy_smoke.go`'s `assertWorkerSandboxConfines`). It is not a
pull-request gate — kind and compose suites never are — but it is free, mock
model only, and now runs automatically every `./make kind up` or `./make
compose smoke`, closing the gap that let the bwrap/seccomp break above ship
unnoticed in the first place. The worker container images also now install
`bubblewrap` and `socat` in
[`deployment/docker/Dockerfile.buildmax`](../deployment/docker/Dockerfile.buildmax)
and
[`deployment/docker/Dockerfile.release`](../deployment/docker/Dockerfile.release),
which the images lacked entirely before this pass and which the Linux
sandbox backend requires regardless of the profile question.

What this closes: a worker run is no longer built with an empty
`SandboxSurface` resolving to the permissive CLI baseline, `bwrap` now
functions under the worker pod's actual production hardening rather than
merely being installed and unable to run, that functioning is now proven by
an organic run rather than a one-off manual pod reproduction, and an agent
author can request the `registries` or `open` network tier and a shared
read/external-write filesystem tier without an operator hand-editing
`policy.yaml` per agent.

Process resource limits (`sandbox.process.{max_cpu_seconds,max_memory_mb,
max_processes,max_open_files}`) are also now implemented as `ulimit`
statements prefixed onto the wrapped command, verified against real Alpine
and macOS shells (`max_memory_mb` is a documented no-op on macOS, which has
no `RLIMIT_AS`) — closing [`sandbox-boundaries.md`](design/sandbox-boundaries.md)
§13.1 gap 2.

The `command` and `http` hook transports now also consult `SandboxView`
(§13.1 gap 3): a hook's command runs through the same `WrapBashCommand` call
and scrubbed environment `Bash` uses, and a hook's HTTP request is checked
against the same `HostAllowed` policy `WebFetch` uses, with no
`dangerously_disable_sandbox`-equivalent escape hatch, since hooks are
config-authored automation rather than an LLM-chosen call an operator is
watching turn by turn. Verified against a real `sandbox.Manager` (Seatbelt),
not only a test double.

Portal's agent editor now exposes both tiers as selectors beside name and
instructions, defaulting to "Team default" (the empty string, which inherits
the team's own default and only then falls through to the strictest
baseline) rather than a hardcoded strictest choice, and a team's Plugins
settings tab gains a "Sandbox defaults" section, visible to any member and
editable by owner or admin, that sets what an agent declaring nothing
inherits (`PUT /api/teams/{team_id}/sandbox-defaults`,
`internal/service/team.SetSandboxDefaults`, resolved into the worker's
`GetTaskRun` response alongside the agent's own declaration). An agent's own
declared tier still always overrides the team default. This closes both
halves of [`agent-sandbox-policy.md`](design/agent-sandbox-policy.md) §9/§10
that were previously not started.

What remains open: the cluster-level `NetworkPolicy` question
[`trust-harness.md`](design/trust-harness.md) §3.9 leaves open — a worker
pod reaches whatever the cluster's network allows, independent of the
in-process sandbox this section covers — is untouched by this pass;
`buildmax sandbox overrides` is still unimplemented; and neither plugin pins
nor the resolved sandbox tiers are yet surfaced in a task run's own detail
view in Portal, only in the API response and audit trail.

The non-root configuration turned out to be incompatible with `bwrap`
actually running on a real cluster. A container
runtime lands a capability added to a *non-root* pod (`Capabilities.Add`) in
that pod's capability bounding set only, never its effective set at exec
time — confirmed by isolating every other variable (this section's own
seccomp and `/proc` fixes, an AppArmor override, `no-new-privileges`, file
capabilities on `bwrap` itself) one at a time against a real Deployment
smoke run and a throwaway container carrying the same configuration. The
worker Job pod now runs root with `SYS_ADMIN` added, which does not have
this gap; see `docs/reference/configuration.md`'s "How A Worker Pod Is
Confined" for the current pod security context and
`internal/infra/k8s/job.go`'s `containerSecurityContext` for the full
finding. The Compose target's `local_process` worker needs the equivalent
fix (`cap_add: SYS_ADMIN` plus the same seccomp and an `apparmor:unconfined`
override, since it also runs root and Docker's default seccomp *and*
AppArmor profiles each independently blocked a different syscall `bwrap`
needs) — see `deployment/compose/compose.yaml`.

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

### P0 — The Pull-Request Gate Now Proves MySQL Behavior, For The Cases That Are Written

`./make test mysql` (`tools/mk/test_mysql.go`) runs the store scope against a
real server: it requires `BUILDMAX_TEST_DSN` rather than skipping without one,
creates and drops a uniquely named database so it never writes to the one the
DSN names, and fails when a test in the scope skips for the DSN's absence
anyway — the property that keeps the gate from going green by testing nothing.
A pinned `mysql:8.0` service container runs it on every pull request
([`.github/workflows/ci.yml`](../.github/workflows/ci.yml)), and `./make check
ci` runs it when a DSN is present and says it did not when one is absent.

The gate justified itself on its first run. `CreateTeam` returned a `Team`
whose `PluginCuration` was empty while `GetTeam` answered `open` for the same
row, so a create and a later read disagreed and the API omitted the field on
one path. `TestSetTeamPluginCurationRoundTrips` had asserted that since
2026-08-23 and skipped every time; the defect sat on `main` for 387 commits
because nothing ever ran the test.

Four store methods claim in their own comments that exactly one of several
simultaneous callers may win — `ClaimTask`, `TransitionTaskRun`,
`ClaimTaskResultDelivery`, and `RequestTaskRunCancel` — and each implements
that as a conditional UPDATE resting on the server serializing two writes to
one row. `internal/infra/db/concurrency_test.go` now tests all four under
contention, and each was checked by mutation: replacing the conditional UPDATE
with a read-then-write makes its test fail. That check mattered. The first
task-claim test passed against a deliberately broken implementation, because
MySQL reports rows *changed* rather than rows *matched*, so the seven losing
callers writing a status the row already held were counted as zero affected
rows — a concurrency test nobody has watched fail proves nothing.

What remains is case breadth, not mechanism.
[`design/verification-program.md`](design/verification-program.md) §4.2 still
lists retry attempts, workflow revision advancement, delivery restart
recovery, cross-team store lookups, and artifact tombstoning. The N-1 migration
fixture is blocked rather than deferred: the explicit migration list is empty
after the identity cutover, so a fixture would encode a history no database
ever had. The quota bullet in that list is withdrawn — there is no reservation
to test, only a rolling-window read, which §4.2 now records. The database
package measures 53.3% under the gate against 3.5% without it, though coverage
is not yet reported per critical package the way §4.3 asks.

## Product And Operating Gaps

These are material, but they should follow the P0 boundaries above unless a
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
- System administration, quotas, role checks, and audit exist. Team-level
  approvals do not, and that is a decision rather than a backlog item:
  [`design/team-governance.md`](design/team-governance.md) §6 lists approval
  workflows as out of scope and §11 gives the reason — avoid custom roles and
  approvals until basic traceability lands.
  [`design/team-membership-lifecycle.md`](design/team-membership-lifecycle.md)
  §6 declines to reopen it. Read a missing approval loop as unbuilt on
  purpose, pending a concrete team's need for one.

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

1. Decide the cluster-level `NetworkPolicy` question
   [`trust-harness.md`](design/trust-harness.md) §3.9 leaves open, and give
   MCP stdio child processes a boundary. Everything else in the in-process
   sandbox is closed: the worker sandbox surface, its interaction with the
   pod's hardening, process resource limits, the command/http hook boundary,
   a backend self-test that fails closed, and an organic Bash-calling run
   through the real server → worker → Job path are all proven and exercised
   automatically by the deployment smoke. MCP is the one child process the
   boundary still does not reach — `internal/infra/mcp/transport.go` execs a
   stdio server directly.
2. Make the production topology honest: one supported Server replica now, or
   shared coordination before horizontal scaling.
3. Widen the persistence gate's cases. The gate runs on every pull request and
   the contention cases are written; what is missing is retry, workflow
   revision, delivery restart recovery, and artifact tombstoning per
   [`verification-program.md`](design/verification-program.md) §4.2.
4. Close what remains of account and team operations. Less remains than this
   position suggests: team role lifecycle, ownership transfer, and
   member-scoped recovery are done, signup leaving an account without a
   credential is deliberate, and team approvals are out of scope by decision.
   What is left is whether a deployment's own experience argues for reopening
   either decision.
5. Expand product-owned qualification from an architectural slice into a
   representative release suite.
6. Deepen workflows, real channel adapters, executable team plugins, Portal
   performance, Desktop automation, and throughput based on observed demand.

This ordering treats safety, consistency, and evidence as product capability.
It deliberately does not make another broad feature area the next milestone.
