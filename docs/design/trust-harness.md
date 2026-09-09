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
  ingress isolation are implemented. MCP stdio child-process containment,
  worker-wide egress enforcement, candidate boundary evidence, and resolved
  policy presentation remain open
- follows: P0 Agent Core stability, P1 Local agent experience, and P2 Portal outcome surface — all complete; their plans were retired (see git history)
- roadmap: [../ROADMAP.md](../ROADMAP.md)
- created_at: `2026-05-23`

## 1. Purpose

The completed P0 work made the shared Agent Core stable enough for CLI, Desktop,
Portal, and worker task runs. The remaining R0 work should make the core more
trustworthy and easier to operate.

This document intentionally stays at the product-capability level. It lists the
key things BuildMax should support next, without prescribing detailed
implementation shape.

## 2. Direction

R0 should focus on the shared Agent Core first. CLI, Desktop, Portal, and worker
should expose the same core capabilities in surface-appropriate ways.

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
- record enough trace data for Portal diagnostics
- load only the memory and instructions appropriate for the space/run scope
- make denied actions understandable
- avoid hiding local/remote capability drift

### 3.9 Who Chooses A Worker's Boundary — open

Absorbed from the retired *Agent execution policy* proposal. Four things about
worker execution are settled and described above or in
[sandbox-boundaries.md](./sandbox-boundaries.md): what a run holds, what it runs
inside, its resource bounds, and what it records. What is not settled is
**authority**. The boundary is fixed by the deployment's manifests rather than
chosen: a cluster operator hardens every worker equally or not at all, no space
can be given a different one, and nothing defines what happens when a requested
constraint is unavailable. The worker sandbox now fails closed when its
required backend is unavailable, but cluster-level network egress is still not
part of that enforcement.

One part of this is now closed: how a worker reaches the *Server*. The worker
control channel is served on its own internal listener over TLS, fronted by an
internal Service and a NetworkPolicy that admits only labelled worker pods — see
[worker-api-network-boundary.md](./worker-api-network-boundary.md). That bounds
worker-to-Server traffic; it does not decide worker egress to Git hosts,
registries, or model endpoints.

The concrete gap that remains is general network egress. A worker pod reaches
anything the cluster allows, and `deployment/production/README.md` states that
absence rather than implying a boundary it does not have.

The essential outcome is narrower than a general network-policy product:

> An unattended worker cannot connect directly to an external destination that
> the operator did not allow, every process in the pod is inside that boundary,
> and a run does not silently continue when required enforcement is absent.

This matters specifically for a malicious prompt or repository causing a
model-chosen shell command or MCP child process to exfiltrate data. A proxy used
only by cooperative HTTP clients does not contain the whole pod; the network
boundary must also prevent a process from bypassing that proxy with a direct IP
connection or a different DNS resolver.

Four shapes were considered. Per-user runtime settings are disqualified: they
cannot give an operator an authoritative worker boundary. Leaving it entirely to
cluster manifests is coherent, but leaves BuildMax unable to record or explain a
boundary it does not model, which the Beta gate requires. So the live choice is
one deployment-wide profile in `server.yaml` against layered operator/space/task
profiles, and **deployment-wide holds until evidence says otherwise** — it is
materially cheaper, and a per-space boundary should be paid for by an operator
who asks for it rather than assumed.

What remains open, and what each needs:

| Question | What would settle it |
|---|---|
| Is a per-space boundary a real requirement? | An operator statement either way. Until there is one, deployment-wide stands |
| Does an inapplicable profile fail the run or downgrade it with a recorded warning? | Downgrade is defensible now that a trace reports an unsandboxed run as unsandboxed (§3.3). It must be decided before the worker surface is passed, because that baseline sets `FailIfUnavailable: true` |
| Which destinations does a worker legitimately need? | A default-deny NetworkPolicy in the production reference, proven by a kind smoke run that still completes a task. The allow-list has to be grounded in what real runs reach — package registries, Git hosts, whatever a space configures — not assumed. Whether it is a NetworkPolicy alone or a proxy enforcing the host allow-list the sandbox contract already models with `HostAllowed`/`ProxyAddress` is part of the same question |
| Does an approval gate belong here at all? | Unattended scheduled work is a primary use and nothing gates it today, so the burden is on adding one |
| How is a profile change versioned and attached to an existing TaskRun record? | Falls out of whichever shape wins |

#### 3.9.1 Enforcement Options

The available mechanisms solve different parts of the problem and must not be
described as equivalent:

| Mechanism | What it proves | Limit |
|---|---|---|
| Kubernetes `NetworkPolicy` | Portable default-deny, exact internal Pod/namespace destinations, ports, and fixed external CIDRs | It has no hostname or DNS-query selector; a changing public service cannot be represented safely as a static CIDR list |
| Cilium `toFQDNs` | Open-source DNS-aware egress rules, enforced for the whole selected pod, with flow and DNS evidence through Hubble | DNS answers are translated into L3 IP rules; shared CDN IPs and an allowed destination that itself relays traffic make this weaker than strict application-layer hostname authorization |
| Calico Open Source policy | Portable Calico/Kubernetes policy plus richer ordering and selectors | Domain-based egress is a Calico Enterprise/Cloud capability, so it is not the open-source reference for this requirement |
| A dedicated egress proxy plus default-deny `NetworkPolicy` | The pod can reach only the proxy, while the proxy validates HTTP targets or HTTPS `CONNECT` hostnames and centralizes audit evidence | It adds an operated dependency and still needs rules for non-HTTP protocols; TLS interception is a separate, substantially larger trust decision |

References: [Kubernetes NetworkPolicy](https://kubernetes.io/docs/concepts/services-networking/network-policies/),
[Cilium DNS-based policy](https://docs.cilium.io/en/stable/security/policy/layer3/#dns-based),
[Cilium DNS policy and IP discovery](https://docs.cilium.io/en/stable/security/policy/layer7/#dns-policy-and-ip-discovery),
and [Calico Enterprise DNS policy](https://docs.tigera.io/calico-enterprise/latest/network-policy/domain-based-policy).

An FQDN allow-list is therefore a useful containment layer, not proof that an
HTTPS request reached only the named virtual host. The stronger claim requires
the pod to have no direct world egress and to send supported external protocols
through a policy-enforcing proxy. Neither mechanism can make an allowed upload,
Git host, package registry, or model endpoint safe from intentional data sent
to that allowed destination.

#### 3.9.2 Reference Qualification Direction

The next evidence pass should use Cilium in the local kind cluster. Cilium has
an official kind installation path, its open-source FQDN policy exercises the
domain-shaped requirement directly, and Hubble makes accepted and denied flows
observable. This is a qualification choice for the reference environment, not
a requirement that every BuildMax deployment adopt Cilium.

The kind lifecycle has to become:

1. create the cluster with kind's default CNI disabled;
2. install Cilium;
3. wait for nodes to become Ready; and
4. deploy ingress and the BuildMax stack.

Waiting for Ready before installing the replacement CNI cannot work because
nodes intentionally remain NotReady without a CNI. The first worker policy
prototype should select the stable labels already stamped by the Kubernetes Job
builder and default-deny egress, then allow only:

- TCP and UDP DNS to the cluster DNS pods, with DNS queries narrowed to the
  external names the run may use and the exact internal service names it needs;
- the internal worker API on port 5679;
- the configured object store;
- the configured model endpoint; and
- explicitly approved Git hosts, package registries, and other external
  destinations, normally on their required port rather than all ports.

Internal destinations should use Pod, namespace, or Service identities rather
than FQDN-to-public-IP rules. External wildcard entries must be exceptional and
reviewed: `*.example.com` grants every present and future subdomain, while a
bare `example.com` does not imply its subdomains.

The prototype is acceptable evidence only if one automated kind run proves all
of the following against a real Cilium data plane:

- the existing worker task still completes through the worker API, object
  store, and configured model endpoint;
- an allowed external hostname and port succeed;
- a disallowed hostname, a direct public IP, an alternate DNS resolver, and an
  unlisted port fail;
- a Bash subprocess and an MCP stdio child process cannot bypass the pod-wide
  rule;
- accepted and denied flows are inspectable without exposing credentials; and
- removing or failing the required enforcement stops the worker run instead of
  silently restoring unrestricted egress.

The test must also record the shared-IP limitation rather than converting a
green FQDN test into a claim of strict hostname isolation. After this evidence,
the project can decide whether the Cilium profile is sufficient for the first
private Beta or whether the production contract requires the stronger egress
proxy shape.

Out of scope whichever way it lands: a general policy language, and replacing
operating-system, Kubernetes, cloud, or network controls — a profile should
*drive* a NetworkPolicy, not reimplement one.

The cheapest missing input remains a threat model covering a malicious prompt,
a model-chosen shell command, an MCP child process, DNS and direct-IP bypasses,
and abuse of an allowed destination. It must be evaluated against the
containment that now exists rather than against the state before it. The kind
qualification above turns that threat model into executable evidence instead
of a guessed allow-list.

[agent-sandbox-policy.md](./agent-sandbox-policy.md) proposes an answer to the
first two rows above, narrower than "layered per-space profiles" in general: it
reopens deployment-wide-by-default for the `Network`/`Filesystem` axes of
`SandboxConfig` only, moving those two to a fixed, small set of
agent-revision-scoped tiers, while every other axis and the operator's
`policy.yaml` ceiling stay exactly as decided here. The cluster egress row
remains open and belongs to this section, not that document.

## 4. Explicitly Out Of Scope For Now

Do not include these in the current R0 trust-boundary scope:

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

Recommended implementation order:

1. Durable run trace
2. Sandbox and execution boundaries
3. Memory and instructions
4. Activity views
5. Doctor and diagnostics
6. Runtime hooks
7. Subagent traceability
8. Safer worker execution polish

This order gives the space better visibility first, then better control, then
more extensibility.

## 6. Acceptance

The R0 trust-boundary work is successful when:

- users can inspect what happened in a run
- users can understand tool approval and denial decisions
- users and operators can understand active sandbox boundaries
- local setup problems are easy to diagnose
- worker runs produce useful diagnostic traces
- memory is scoped, inspectable, and user-controllable
- hooks can support common automation use cases
- subagent behavior is attributable and policy-bound
