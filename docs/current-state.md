# BuildMax Current State

> **简体中文：** [阅读中文镜像](zh-CN/current-state.md)
>
> **Audience:** users, operators, and contributors · **Status:** current as of 2026-09-12

This assessment was checked against repository code at `0bd7e5bf`. It describes
implemented behavior, test coverage, and remaining limits. Priority and future
sequencing belong in the [roadmap](ROADMAP.md), not in a second priority list
here. Design records explain decisions; their unfinished checklists are not
proof that code is missing.

## Assessment And Evidence Scope

BuildMax remains Alpha. Local Agent execution and the private Space execution
path are implemented, including direct Task threads, persistent workspace
checkpoints, managed inference, and operator administration. This is not yet
proof of production multi-tenant readiness or of a qualified Beta candidate.
The [Beta readiness record](deploy/beta-readiness.md) remains unqualified.

MCP stdio child processes run outside the Bash sandbox today. The supported
worker profile must confine or disable them before Beta; this is the remaining
R0 engineering rule, not merely a qualification checkbox. Worker-wide network
egress is a documented, accepted limit for the first private Beta. Durable
Workflow reconciliation, trace retention, and candidate failure/recovery
evidence also remain open. Shared Redis coordination is implemented, including
distributed lease fencing at message-history writes. The worker API already
has a separate listener, TLS support, and a shipped ingress NetworkPolicy; that
bounded network slice must not be confused with unrestricted worker egress.

This review inspected implementation, assembly, manifests, and test assertions.
It does not reuse old full-build results, coverage percentages, mutation-test
claims, or maturity percentages as evidence for this revision. The verification
performed for this update is recorded at the end. A test file's existence means
coverage is implemented, not that its deployment or database prerequisites were
exercised in this review.

## Shared Runtime And Local Surfaces

CLI/TUI, Desktop, and workers assemble the shared Agent runtime. The core has a
streamed model/tool loop, tool error recovery, parallel read-only tool execution,
permissions, approvals, compaction and checkpoints, hooks, bounded redacted
traces, usage statistics, sessions, notes, todos, Project Memory, subagents,
worktrees, and background jobs. Model assembly supports OpenAI-compatible chat,
OpenAI Responses, Anthropic, and Ollama.

Interactive Desktop turns now use `agentapp.RunScheduler`, which serializes one
run per session key, queues later prompts in order, and gives queued background
events the same lifecycle. The Server TaskRun scheduler remains a separate
durable execution-plane concern.

Local inspection includes `buildmax info`, TUI `/info`, and the Desktop `/info`
panel for one session; `buildmax usage` sums token and cost totals across
sessions, grouped by day, workspace, or model. Desktop is intentionally
read-only for memory: users edit the Markdown files directly, delete or clear
with `buildmax project forget`, and disable memory for one run with
`--no-project-memory`; separate Desktop edit/delete/enable controls are not
planned.

Local execution does not require a Server. Signed-in clients can use the managed
model catalog; a server-rejected credential is treated as an expired login, and
`buildmax logout` returns the client to local mode. See
[`internal/interface/auth/models.go`](../internal/interface/auth/models.go) and
its [tests](../internal/interface/auth/models_test.go).

The shared LLM request contract does not yet expose a provider-neutral
structured-output schema. Tool-argument JSON schemas are a separate capability;
see [`internal/core/llm/llm.go`](../internal/core/llm/llm.go).

## Tasks, Results, And Workspace Continuity

Direct Agent execution creates a Task and TaskRun without requiring a
Conversation. Continue appends a run to that Task; Retry creates a new attempt
with explicit lineage. TaskRun is authoritative for the result. A Conversation
may create a Task but is not its authorization or storage parent. The previous
result-delivery queue and mandatory foreground summary attempt are removed.

**Direct Task streaming is implemented.** The worker appends deltas by Task ID,
the Task SSE handler subscribes to that ID, and Portal's Task page reads the
stream while polling durable run state. This path does not depend on a
Conversation. The page also opens a stored run trace. Sources:

- [`internal/server/handlers/worker/worker.go`](../internal/server/handlers/worker/worker.go)
- [`internal/server/handlers/work/stream.go`](../internal/server/handlers/work/stream.go)
- [`portal/src/pages/tasks/TaskDetail.tsx`](../portal/src/pages/tasks/TaskDetail.tsx)

Stream behavior depends on the coordination mode: local mode buffers in memory;
Redis mode shares bounded streams across replicas with expiration. Neither is
an indefinite replay log. The [Portal Task-thread test](../portal/e2e/task-thread.spec.ts)
covers direct execution, Continue, and Retry through the UI. The
[two-replica streaming test](../internal/server/handlers/work/stream_multireplica_test.go)
uses miniredis to exercise cross-replica deltas and buffered output. These tests
do not prove every streaming, trace, or managed-usage failure scenario.

Task workspaces persist as immutable object-store checkpoints. The first run
seeds a base; Continue uses the Task's workspace head, while Retry uses the
repeated run's base. Successful result checkpoints advance the head; partial
checkpoints preserve failed or canceled work without advancing it. The worker
records restoration status, and terminal reporting finalizes available
checkpoints. Checkpoint finalization failure does not rewrite the run outcome.

Implementation and tests span
[`internal/agentapp/taskrun/checkpoint.go`](../internal/agentapp/taskrun/checkpoint.go),
[`internal/service/workspace/checkpoint.go`](../internal/service/workspace/checkpoint.go),
[`internal/infra/db/workspace_checkpoint.go`](../internal/infra/db/workspace_checkpoint.go),
and the worker checkpoint handlers. Worker Jobs have ephemeral-storage limits;
orphan and retention sweeps reclaim unreferenced payloads. Portal displays
checkpoint and restoration state read-only. These mechanisms do not establish
paired database/bucket restore or upgrade-rollback qualification.

## Worker Execution And Network Boundaries

### Bash Sandbox And Child Processes

[`config.WorkerSandboxSurface`](../internal/config/sandbox.go) selects the strict
worker baseline when `BUILDMAX_SANDBOX_BACKEND_INSTALLED` is present. Official
images install `bubblewrap` and `socat` and set this marker. This includes Compose
workers launched as local processes inside the official image. An unmarked bare
host inherits the CLI baseline unless configured otherwise; the code does not
make every possible worker launch fail closed by default.

The selected sandbox resolves settings, policy, run overrides, and Agent tiers.
Worker handlers resolve and pin the effective Agent/Space tiers for audit.
The backend self-test refuses unavailable enforcement when fail-closed policy
is selected. Resource controls prefix wrapped commands with shell limits;
the memory limit is not enforced on macOS. Command hooks use the Bash wrapper
and scrubbed environment; HTTP hooks consult the allowed-host policy.

The Kubernetes worker security context is **root with `SYS_ADMIN` added**, with
a read-only root filesystem and a supplied Localhost seccomp profile. The Linux
Bash wrapper rebinds the container's `/proc` read-only. This is not a non-root
pod or whole-worker isolation equivalent to the command sandbox. Sources:
[`internal/infra/k8s/job.go`](../internal/infra/k8s/job.go),
[`internal/infra/sandbox/bwrap_linux.go`](../internal/infra/sandbox/bwrap_linux.go),
and [seccomp deployment instructions](../deployment/seccomp/README.md).

Deployment smoke contains an actual worker Bash probe that checks successful
execution and denial of an out-of-workspace write
([`tools/mk/deploy_smoke.go`](../tools/mk/deploy_smoke.go)). This is implemented
end-to-end coverage, not a claim that this review ran the cluster smoke.

Remaining limits:

- MCP stdio servers launch with `exec.Command` and do not pass through the Bash
  sandbox ([`internal/infra/mcp/transport.go`](../internal/infra/mcp/transport.go)).
  The supported worker profile must confine or refuse them; that fail-closed
  treatment is not implemented yet.
- `local_process` remains in the Server's host trust domain even when its Bash
  commands are sandboxed.
- `buildmax sandbox overrides` is not implemented. Portal exposes Agent tiers
  and Space defaults. Run Details shows the boundary recorded by the trace and
  the resolved plugin pins, but not the requested/resolved tier pair or stdio
  MCP treatment as distinct diagnostic fields.
- No worker RuntimeClass selection is wired in the Job builder. gVisor remains
  conditional post-Beta hardening, not a shipped supported worker profile or a
  first-Beta requirement.

### Worker API Boundary

**Implemented:** public and worker routes use separate muxes and listeners.
Worker routes are absent from the public listener, independently of a caller's
token. Server bootstrap builds the worker listener's TLS configuration; optional
client-CA configuration enables native mTLS. Workers can use a configured CA and
client identity. Per-run authentication remains required.

The basic and production Kubernetes manifests include a worker API Service and
a Server-ingress NetworkPolicy admitting the worker port only from matching
worker pods in the namespace. The public API port remains open to cluster
traffic under that policy. Enforcement requires a CNI that implements
NetworkPolicy; manifest presence alone is not proof of enforcement.

Sources and coverage:
[`internal/server/server.go`](../internal/server/server.go),
[listener boundary tests](../internal/server/listener_boundary_test.go),
[`internal/bootstrap/worker_tls.go`](../internal/bootstrap/worker_tls.go), and
[production manifest](../deployment/production/buildmax.yaml).

**Accepted first-Beta limit:** a worker egress NetworkPolicy is absent. The Server-ingress policy does
not constrain all outbound traffic from a worker, sandbox MCP processes, or hide
the storage credentials used by the worker. TLS support also does not mean every
local development configuration requires TLS.

## Server Topology And Persistence

### Shared Coordination Is Implemented

`coordination.mode: local` remains the single-instance default. Redis mode wires
shared Task streams, connection-event fan-out, and renewable Conversation turn
leases through [Server adapters](../internal/server/coordination/coordination.go)
and [Redis primitives](../internal/infra/coordination). Bootstrap rejects an
unreachable configured Redis rather than silently falling back to local mode.

Both the basic/kind and production manifests now configure Redis and two Server
replicas. Architecture tests reject multiple replicas without coordination.
Multi-replica streaming and lease behavior have automated tests; candidate
reconnect, contention, outage, and recovery exercises still need operating
proof. The lease exposes a fencing token, and message-history writes enforce it:
a write carrying a token below the one the conversation has accepted is rejected,
so a stale writer after lease loss cannot append behind the new holder. Lease
renewal itself discards Redis errors and does not cancel the running turn when
ownership is lost, so such a turn runs to a rejected write rather than being
stopped early. See the [coordination design](design/server-coordination.md).

The scheduler has one concurrent dispatch slot per instance. In local-process
mode that slot remains occupied during execution. Kubernetes dispatch returns
after creating a Job, so the same setting does **not** limit the cluster to one
running worker. See
[`internal/server/scheduler/scheduler.go`](../internal/server/scheduler/scheduler.go)
and [`internal/infra/k8s/job.go`](../internal/infra/k8s/job.go).

### Database Coverage And Migrations

`./make test mysql` requires a DSN, creates and drops an isolated database, and
rejects missing-DSN skips. CI supplies a pinned `mysql:8.0` service. A default test
run without a DSN still skips database-dependent tests.

The database coverage is broader than the previous assessment reported:

| Behavior covered by database tests | Evidence |
|---|---|
| Retry lineage and original attempt preservation | [task_run_retry_test.go](../internal/infra/db/task_run_retry_test.go) |
| Continue versus Retry workspace base selection | [task_run_base_test.go](../internal/infra/db/task_run_base_test.go) |
| Direct Tasks, continuation, and idempotency keys | [task_direct_test.go](../internal/infra/db/task_direct_test.go) |
| Task claiming, run transitions, one active run, and cancellation/report races | [concurrency_test.go](../internal/infra/db/concurrency_test.go) |
| Artifact soft deletion, concurrent deletion, expiry, byte accounting, and purge lifecycle | [artifact_retention_test.go](../internal/infra/db/artifact_retention_test.go) |
| Checkpoint head advancement and partial checkpoint retention | [workspace_checkpoint_test.go](../internal/infra/db/workspace_checkpoint_test.go) |
| Workflow guarded run/step transitions and atomic failure finalization | [workflow_test.go](../internal/infra/db/workflow_test.go) |
| Workflow initial revision and revision queries | [revision_query_test.go](../internal/infra/db/revision_query_test.go) |
| Space isolation for secrets and independent invitations | [secret_test.go](../internal/infra/db/secret_test.go), [space_invitation_test.go](../internal/infra/db/space_invitation_test.go) |

The guarded transitions prevent illegal terminal rewrites and make failed-step,
later-step blocking, and failed-run finalization atomic. They do not yet create
a durable reconciler: progress still depends on callbacks, so restart and lost-
callback recovery remain open. This is also not exhaustive proof of cross-Space
store behavior or Workflow revision advancement under edits and contention.
External dependency recovery still needs scenario-specific evidence. The
removed result-delivery queue has no remaining restart-recovery obligation of
its own.

**The explicit migration list is no longer empty.**
[`internal/infra/db/migration.go`](../internal/infra/db/migration.go) contains
`system_grant_live_marker` and `llm_model_credential_encryption`.
The latter drops the old plaintext credential column without migrating its
values; affected models must be re-added. The migration test covers ledger
recording and skipping on a second run. Neither that test nor an N-1 policy in
a design document establishes an exercised old-schema upgrade and binary
rollback. The old explanation that a fixture is blocked by an empty migration
history is obsolete.

Each trace is bounded by field and record caps, but the traces directory has no
retention sweep. A long-lived process therefore needs external capacity
management or manual deletion today; BuildMax cannot yet record that old traces
were removed by policy.

## Account, Space, And Extension Surfaces

Account creation, single-use login codes, password sign-in, system administrator
grants, Space invitations to existing accounts, role changes, ownership
transfer, and member-scoped recovery are implemented. Signup defaults off;
creating an account does not itself issue a credential. See the
[identity service](../internal/service/identity/account.go) and
[Space service](../internal/service/space/service.go).

`buildmax admin` provides authenticated administrator, account, and model-catalog
operations. `buildmax-server` retains database-direct bootstrap and recovery
commands. Model credentials are encrypted under the deployment key-encryption
key; credentialed model creation refuses to store a key without encryption.
Space secrets and Agent secret-consumption declarations also have storage and
worker delivery implementations, with run-scoped authorization. Their presence
does not isolate delivered secrets from the worker process that consumes them.

System administration, quota, audit, role checks, and Space lifecycle UI exist.
Remaining administration gaps include transactional authority audit, admin CLI
Session listing/revocation parity, quota-tier assignment, and runtime metadata
for queue/worker diagnosis. These are tracked in the
[administration operations proposal](proposals/system-administration-operations.md);
proposal status must not be confused with an implemented feature. Plugin
publication remains CLI-only, while Portal can inspect, retire, restore, and
yank catalog releases.

Space approval workflows remain unimplemented and deliberately out of scope;
that is not evidence of an unfinished invitation or ownership-transfer feature.

Workflow definitions remain linear `agent_task` steps. They have versioned
definitions and durable run/step records, but no branching, parallel graph,
manual approval, loops, or typed input/output mapping in the definition contract
([`internal/core/workflow/workflow.go`](../internal/core/workflow/workflow.go)).

Portal and inbound webhook execution are assembled. Telegram remains channel
vocabulary, and the webhook callback sender is not assembled into the Server.
Recurring schedules run an Agent on the Task plane through a `schedule` trigger
source and the `/api/spaces/{space_id}/schedules` API, dispatched by a resident
loop that claims each due time once across replicas, coalesces missed firings
into one catch-up, and pauses a schedule after five consecutive failed firings
or when its creator is disabled. Portal creates and manages schedules on the
Agent detail page and lists every schedule in a Space on a Schedules page; the
pause reason is logged, not shown. They are not a conversation channel
([`internal/core/schedule`](../internal/core/schedule/schedule.go),
[`internal/server/scheduler`](../internal/server/scheduler),
[design](design/scheduled-agent-execution.md)). Space plugin activation supports skill/subagent content but rejects
releases containing hooks or MCP servers
([activation service](../internal/service/plugin/activation.go)). Foreground
Conversations do not load Space plugins.

## Qualification And Operating Evidence

The evaluation contract, built-binary local and worker adapters, graders,
repeated and paired experiments, and pinned Harbor adapter are implemented.
[`evaluation/suite`](../evaluation/suite) contains three product-owned tasks.
That scope cannot qualify all supported surfaces. Historical oracle/canary
reports are not a benchmark result for this revision; this review ran neither
real-model evaluation nor a Terminal-Bench protocol and reports no score.

Portal browser tests now cover direct Task threads, workspaces, canonical Space
routes, loading/error/permission states, responsive layouts, accessibility, and
run provenance. Desktop has bridge and browser-based UI suites under
[`desktop/frontend/e2e`](../desktop/frontend/e2e), plus a packaged-application
launch smoke on macOS and Windows CI. The launch smoke proves that the built
bundle starts and stays alive briefly; it does not drive or visually inspect the
native window. Portal routes remain eagerly imported, with no route-level lazy
loading in the current source. No fresh bundle size or throughput number was
measured in this review.

Deployment smoke includes retry, managed inference and its call ledger,
cancellation of a running worker, and the Bash confinement probe. Scheduler
unit tests cover stale-run handling and cleanup. These are not equivalent to
candidate exercises for hard worker loss, database outage, object-storage
access denial, paired restore, credential rotation, and schema rollback.

Compose, kind, production Kubernetes manifests, release verification, SBOM,
image scanning, and provenance workflows exist. Their presence does not fill
the unsigned [Beta readiness record](deploy/beta-readiness.md).

## Verification For This Review

This is a source-and-tests reassessment, not a fresh deployment qualification.
The latest `main` CI, CodeQL, Windows, and deployment-smoke workflows passed for
`0bd7e5bf`. For this documentation update, `./make check docs`,
`./make check portal`, `./make test ./internal/architecture`, the Go packages
whose comments changed, and `git diff --check` passed locally. Documentation
checks cover links and formatting; they do not prove runtime behavior.

The review did not run the real-MySQL scope (no `BUILDMAX_TEST_DSN` was supplied),
full builds, frontend/browser suites, Compose/kind deployment smoke, external
recovery drills, or paid model evaluation. Database test assertions above were
read, not claimed as executed. Historical coverage and deployment results have
therefore not been carried forward as current measurements.
