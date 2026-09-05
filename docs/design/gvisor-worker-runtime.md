# gVisor Worker Runtime

> **Audience:** contributors and operators · **Status:** planned — qualification required before support

Related: [Agent Core trust harness](trust-harness.md), [Sandbox
boundaries](sandbox-boundaries.md), [Agent-scoped sandbox
policy](agent-sandbox-policy.md), [Worker API network
boundary](worker-api-network-boundary.md), and [Enterprise
deployment](enterprise-deployment.md).

External references: [gVisor overview](https://gvisor.dev/docs/), [security
model](https://gvisor.dev/docs/architecture_guide/security/), [Kubernetes
integration](https://gvisor.dev/docs/user_guide/quick_start/kubernetes/), and
[application compatibility](https://gvisor.dev/docs/user_guide/compatibility/).

## Contents

- [1. Status](#1-status)
- [2. Problem](#2-problem)
- [3. Decision](#3-decision)
- [4. Boundary Model](#4-boundary-model)
- [5. Why gVisor](#5-why-gvisor)
- [6. Composition With Bubblewrap](#6-composition-with-bubblewrap)
- [7. Worker Pod Security Profiles](#7-worker-pod-security-profiles)
- [8. Kubernetes Integration](#8-kubernetes-integration)
- [9. Configuration And Provenance](#9-configuration-and-provenance)
- [10. Failure And Lifecycle Semantics](#10-failure-and-lifecycle-semantics)
- [11. Compatibility And Performance](#11-compatibility-and-performance)
- [12. Options Considered](#12-options-considered)
- [13. Implementation Plan](#13-implementation-plan)
- [14. Validation](#14-validation)
- [15. Risks And Open Questions](#15-risks-and-open-questions)
- [16. Documentation Changes](#16-documentation-changes)

## 1. Status

- roadmap_priority: `R0` — contain unattended worker execution
- status: planned; BuildMax does not currently set `runtimeClassName`, install
  `runsc`, or claim gVisor compatibility
- decision_date: `2026-09-05`
- first_gate: run the exact BuildMax worker and `bwrap` sandbox probe under
  gVisor before adding a supported configuration surface
- scope: add an outer Kubernetes runtime boundary around a complete worker Pod

This is a conditional implementation plan, not a claim that setting
`runtimeClassName: gvisor` is already safe. gVisor implements a broad but not
complete Linux ABI, and BuildMax's current worker deliberately exercises
namespace, mount, process, and capability behavior that ordinary application
containers do not. The compatibility gate in §13 M0 decides whether the target
can proceed without weakening either boundary.

## 2. Problem

A worker executes model-selected commands, repository content, plugin code,
hooks, and MCP child processes. The Kubernetes Job currently runs as root with
`SYS_ADMIN`, a custom Localhost seccomp profile, and AppArmor unconfined because
those properties were required for `bubblewrap` to create the inner namespace
that confines Bash commands.

That configuration made `bwrap` work and is tested against a real Pod. Its
boundary is still the host Linux kernel:

```text
model command
    |
    v
bwrap namespaces and policy
    |
    v
worker container -- syscalls --> host Linux kernel
```

Three gaps remain even when the `bwrap` command sandbox behaves correctly:

| Gap | Consequence |
|---|---|
| Container processes use the host kernel directly | A kernel or container-runtime escape has a short path to the node |
| `bwrap` does not wrap the whole worker | A defect in the worker, an unwrapped MCP child, or another process still relies on the ordinary container boundary |
| Root and `SYS_ADMIN` are effective in the native container | A successful escape begins with a broad capability set relative to the container namespace |

Seccomp, namespaces, AppArmor, a read-only root filesystem, and capability
reduction remain valuable, but each is enforced by the same host kernel whose
attack surface the untrusted workload reaches. Adding more syscall exceptions
to make nested tools work can also weaken the very layer being used as the
outer boundary.

## 3. Decision

BuildMax will support an operator-selected Kubernetes RuntimeClass for worker
Jobs. The first runtime to qualify is gVisor's `runsc`.

The intended hardened topology is:

```text
host Linux kernel
    |
    v
gVisor runsc
    |- Sentry application kernel
    |- Gofer filesystem mediation
    `- userspace network stack
          |
          v
    complete buildmax-worker Pod
          |
          v
    bwrap command sandbox
          |
          v
    model-selected command
```

The runtime selection belongs to the deployment operator, not a Team, Agent,
Task, or model. An Agent may request network and filesystem tiers inside the
run; it cannot choose whether the Pod uses `runc`, `runsc`, Kata, or another
cluster runtime.

The configuration names both a BuildMax runtime kind and a Kubernetes
RuntimeClass, and carries the class into every worker Pod. When the runtime kind
is `gvisor`:

- BuildMax never removes it or retries the Job under the cluster default;
- a missing or unavailable runtime fails closed;
- the requested value is recorded with the TaskRun's worker provenance; and
- the production support claim applies only to runtime classes BuildMax has
  qualified with its own worker test matrix.

An empty value continues to select the cluster's default OCI runtime. That is
the portable baseline, not an implicit claim of the stronger outer boundary.
The production reference may recommend a qualified gVisor RuntimeClass, but it
must not manufacture one: installing and patching the node runtime belongs to
the cluster operator.

## 4. Boundary Model

### 4.1 What gVisor Adds

gVisor's Sentry is a userspace application kernel. Workload system calls are
implemented there instead of being passed directly to the host kernel. The
Sentry itself runs with a restricted host syscall surface, while filesystem
access is mediated by a Gofer and networking normally uses gVisor's own
netstack. See the upstream [architecture](https://gvisor.dev/docs/) and
[security model](https://gvisor.dev/docs/architecture_guide/security/).

For BuildMax, that makes the entire worker Pod one outer sandbox. An unwrapped
MCP process is still dangerous to the run's data and credentials, but it no
longer has the ordinary native-container syscall relationship with the node.

Capabilities requested in the Pod spec are capabilities inside the gVisor
sandbox rather than host Linux capabilities. Upstream demonstrates nested
container workloads receiving `SYS_ADMIN` without granting that capability to
the host. This makes the permission `bwrap` may still need materially less
dangerous than the same field under `runc`; it does not make the permission
irrelevant or remove the need to minimize it. See [Docker in
gVisor](https://gvisor.dev/docs/tutorials/docker-in-gvisor/).

### 4.2 What gVisor Does Not Add

| Concern | Owner after this design |
|---|---|
| Which Server route a worker may call | Internal worker listener, TLS, NetworkPolicy, and run token |
| Which Team or TaskRun a request belongs to | Server-side run authorization |
| Which paths a model command may read or write | `bwrap` and BuildMax sandbox policy |
| Which domains a model command may reach | BuildMax sandbox proxy and future cluster egress policy |
| Which Secret a run may receive | Agent revision, Team ownership, and Secret materialization state |
| CPU and memory exhaustion | Kubernetes requests, limits, and host cgroups |
| Hardware side channels | Host, hardware, and cloud platform controls |

gVisor is not a destination firewall. Its netstack isolates implementation but
still sends packets permitted by the Pod network. Upstream explicitly requires
container-level network policy for destination control. It is also not a
resource-limit mechanism; Kubernetes cgroups remain authoritative.

### 4.3 Trusted Components

The node runtime, gVisor release, host kernel, Kubernetes control plane, CNI,
mounted files, and BuildMax Server remain trusted. gVisor narrows the interface
between the worker and host; it does not remove the host or platform from the
trusted computing base.

A Kubernetes or node administrator can inspect or alter the Pod, RuntimeClass,
run token delivery, and mounted content. This design makes no protection claim
against them.

## 5. Why gVisor

The worker shape favors gVisor's trade-off:

- one short-lived Job already creates a natural sandbox unit;
- workers run arbitrary language runtimes and command-line tools rather than a
  fixed syscall-minimal service;
- process-like startup and elastic resource use fit better than reserving one
  complete VM per run;
- the workload is security-sensitive enough to justify more isolation than a
  native container; and
- `runsc` implements the OCI runtime contract, so Kubernetes selects it through
  `RuntimeClass` without BuildMax owning a container launcher.

The security benefit is diversity as well as reduction. A host Linux exploit
does not directly receive attacker-controlled workload syscall arguments; an
escape generally has to cross the independently implemented Sentry boundary
and then its restricted host boundary. gVisor describes this as defense in
depth, not equivalence to a hardware VM.

## 6. Composition With Bubblewrap

### 6.1 The Two Boundaries Are Not Substitutes

`bwrap` and gVisor have different subjects:

| Boundary | Subject | Protects |
|---|---|---|
| gVisor | Whole worker Pod | Node and host kernel from the worker workload |
| `bwrap` | One model-selected command | Worker process, workspace boundary, selected host paths, and command network policy |

Removing `bwrap` because a Pod uses gVisor would let a model-selected command
read any file visible to the worker process, including run credentials and
implementation state, and bypass BuildMax's agent-scoped path and domain
policy. That is not an acceptable simplification.

### 6.2 Compatibility Is A Gate

The current `bwrap` invocation uses user, mount, PID, IPC, and UTS namespace
operations plus bind mounts and `/proc` handling. gVisor does not implement
every Linux syscall, ioctl, filesystem, or namespace behavior. Its support for
nested Docker demonstrates that nested isolation is possible, not that this
specific `bwrap` program and argument set works.

The first milestone therefore runs the exact argv constructed by
`internal/infra/sandbox/bwrap_linux.go` inside the exact worker image and Pod
security context under `runsc`. A reduced reproduction is diagnostic evidence,
not acceptance.

### 6.3 No Policy Downgrade

If `bwrap` cannot enforce the existing filesystem and network policy under
gVisor, BuildMax does not:

- disable `bwrap`;
- set `fail_if_unavailable` false;
- retry the TaskRun under the cluster default runtime;
- enable gVisor host networking or direct filesystem access to make the test
  pass; or
- document gVisor as supported.

The design is reopened instead. An alternative inner backend must demonstrate
the same policy contract before it can replace `bwrap` for gVisor workers.

## 7. Worker Pod Security Profiles

### 7.1 Runtime-Specific Construction

The current Pod security context is evidence for `runc` plus `bwrap`, not a
universal worker profile. A gVisor worker must not blindly inherit the custom
Localhost seccomp profile, AppArmor Unconfined, root UID, and host-facing
capability assumptions that were derived from native-container failures.

`internal/infra/k8s` will build the Pod security context from a qualified
runtime profile:

| Profile | RuntimeClass | Security context |
|---|---|---|
| `native-bwrap` | empty | Existing tested Localhost seccomp, AppArmor and capability set |
| `gvisor-bwrap` | qualified gVisor class | The smallest context proven to start the worker and enforce the exact `bwrap` probe under `runsc` |

RuntimeClass name alone does not select arbitrary security settings. The
operator may name a qualified class whose handler is `runsc`; BuildMax owns the
corresponding worker profile and refuses a runtime/profile combination it does
not understand.

### 7.2 Qualification Order

The gVisor profile starts with:

- non-root worker UID;
- every Linux capability dropped;
- no AppArmor `Unconfined` override;
- no BuildMax Localhost seccomp profile;
- no privilege escalation;
- read-only root filesystem; and
- the existing explicit writable `emptyDir` mounts.

The spike adds back only an in-sandbox permission that a failing organic probe
demonstrates is necessary. If root or `SYS_ADMIN` is still required, the final
record must state that it is virtualized by `runsc`, and the test must show the
same Pod cannot affect node mounts, processes, or files.

The following gVisor escape hatches are outside the initial profile because
they reduce the intended boundary:

- host networking or network passthrough;
- direct host filesystem access in place of Gofer mediation;
- host devices other than ones reviewed for a concrete worker requirement;
- privileged containers; and
- host PID, IPC, or network namespaces.

### 7.3 Admission Policy

An operator may use a validating admission policy to require the qualified
RuntimeClass and security profile for Pods carrying the BuildMax worker label.
BuildMax's production reference documents that recommendation but does not
install a cluster-wide admission controller.

The policy must reject, not mutate, a mismatched worker Pod. Mutation makes the
Job spec and TaskRun provenance disagree about what actually ran.

## 8. Kubernetes Integration

### 8.1 RuntimeClass

The operator installs `runsc` on eligible nodes and creates a RuntimeClass,
for example:

```yaml
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata:
  name: gvisor
handler: runsc
```

Managed platforms may provide this class or use another documented name. The
class should carry RuntimeClass scheduling constraints when only a dedicated
node pool has `runsc`; BuildMax does not copy node selectors into every Job.
The upstream [Kubernetes guide](https://gvisor.dev/docs/user_guide/quick_start/kubernetes/)
describes both managed and containerd installations.

Every worker Job then carries:

```yaml
spec:
  template:
    spec:
      runtimeClassName: gvisor
```

Only worker Jobs use this class. Server, Portal, storage, and database workloads
stay on their existing runtime because this threat model and compatibility cost
do not apply to them.

### 8.2 Runtime Platform

`runsc` platforms such as `systrap` and KVM are node-runtime configuration, not
BuildMax Agent settings. BuildMax records the RuntimeClass name and qualifies
the externally observable behavior; it does not pass `runsc` flags from
`server.yaml`.

The operator chooses a platform appropriate to its nodes. Upstream recommends
testing this choice because KVM, nested virtualization, systrap, filesystem
mode, and networking mode have different security and performance trade-offs.
See the [platform guide](https://gvisor.dev/docs/user_guide/platforms/) and
[production guide](https://gvisor.dev/docs/user_guide/production/).

### 8.3 Server Permission

When a RuntimeClass is configured, Server startup reads that cluster-scoped
object and refuses to start the scheduler when it is absent. The reference RBAC
adds only `get` on `runtimeclasses.node.k8s.io`; the Server does not create,
update, list, or delete RuntimeClasses.

Existence is not proof that every node can run the handler. RuntimeClass
scheduling constraints, node provisioning, and the deployment smoke establish
that. The startup check prevents the common failure where Kubernetes accepts a
Job but cannot create any Pod and the TaskRun stays `SCHEDULED` until the stale
run timeout.

### 8.4 Dedicated Nodes

A separate worker node pool is recommended because it:

- keeps untrusted execution away from Server and data workloads;
- makes RuntimeClass handler availability explicit;
- allows node-level patch and rollout policy for `runsc`;
- bounds noisy-neighbor effects; and
- gives admission and scheduling policy a stable target.

It is not a BuildMax prerequisite. A small private deployment may install
`runsc` on every worker-capable node.

## 9. Configuration And Provenance

### 9.1 Configuration Shape

The two new operator fields separate BuildMax's security profile from the
cluster object's name:

```yaml
worker:
  run_mode: k8s_job
  k8s:
    runtime_kind: gvisor
    runtime_class_name: gvisor
```

Both empty means the cluster default runtime and BuildMax's existing native
profile. `runtime_kind: gvisor` requires a non-empty, valid
`runtime_class_name`, which must resolve at Server startup. Neither field is
valid under `local_process`, because no Kubernetes Pod exists to apply it to.

There is deliberately no `allow_runtime_fallback` field. A fallback from
`runsc` to `runc` changes the security boundary while preserving the same task
status and is therefore indistinguishable from a successful secure run to a
reader.

### 9.2 Qualified Names

BuildMax support is not inferred from a class name containing `gvisor`.
`runtime_kind` maps explicitly to the `gvisor-bwrap` profile, while
`runtime_class_name` identifies the operator's cluster object. The initial
portable shape is:

```yaml
worker:
  k8s:
    runtime_class_name: gvisor
    runtime_kind: gvisor
```

`runtime_kind` selects BuildMax's tested Pod profile; `runtime_class_name`
selects the cluster object. Keeping them separate supports managed platforms
whose class is named `sandboxed` without treating any operator-provided class
as gVisor by spelling convention.

The accepted implementation may replace these two strings with a small typed
object, but it must preserve that semantic separation and reject unknown kinds.

### 9.3 TaskRun Evidence

TaskRun provenance records:

- requested RuntimeClass name;
- BuildMax runtime profile kind;
- worker image digest when the cluster reports one;
- Job name and creation time; and
- whether the worker reached its first authenticated heartbeat.

The RuntimeClass field is a requested and admitted configuration, not remote
attestation. A worker heartbeat proves a Pod started and reached the Server; it
does not prove the node runtime or image is uncompromised. Portal and audit copy
must not call it attested.

The durable trace repeats the runtime class and profile as non-secret sandbox
metadata so an operator can compare runs without inspecting Kubernetes after
Jobs are deleted.

## 10. Failure And Lifecycle Semantics

### 10.1 Fail Closed

| Failure | Result |
|---|---|
| Configured RuntimeClass does not exist | Server refuses to start its scheduler and names the missing class |
| No eligible node supports the class | Job remains unscheduled; deployment diagnostics identify the scheduling constraint and the run is failed within the existing bound |
| `runsc` cannot start the image | TaskRun becomes `FAILED`; never retry under the default runtime |
| `bwrap` backend probe fails inside gVisor | Worker fails before model execution and reports the sandbox reason |
| RuntimeClass is removed during operation | New Jobs fail closed; running Jobs continue under the runtime that admitted them |
| gVisor-specific profile is unavailable on this BuildMax build | Server rejects configuration before dispatch |

The error shown on TaskRun must distinguish runtime admission, image startup,
and inner sandbox failure. Reporting all three as "worker disappeared" makes
the security setting operationally unusable.

### 10.2 Job Observation

The current liveness reaper sees only a missing heartbeat and cannot explain
why a Pod never started. Supporting a required RuntimeClass includes observing
Job and Pod conditions sufficiently to turn `FailedCreate`, image, scheduling,
and runtime-handler failures into bounded sanitized TaskRun errors.

This observer reads only Jobs and Pods the scheduler recorded for BuildMax
runs. It does not execute recovery by mutating a Pod or changing its runtime.
Retry remains an explicit new TaskRun.

### 10.3 Upgrade

Changing `runtime_class_name`, runtime profile, `runsc` version, or node handler
does not alter a running Job. New TaskRuns use the new deployment snapshot;
existing TaskRuns finish where they started.

A RuntimeClass rollout follows this order:

1. install and qualify `runsc` on the target nodes;
2. create the RuntimeClass with scheduling constraints;
3. run the BuildMax deployment probe against the exact image and profile;
4. configure BuildMax to request the class;
5. observe new worker Jobs before draining native worker capacity; and
6. remove the old runtime only after every Job using it is terminal.

There is no mixed-runtime retry of one TaskRun. A deliberate retry after an
operator changes the deployment creates a new TaskRun and records the new
runtime provenance.

## 11. Compatibility And Performance

### 11.1 Expected Compatibility Pressure

Most Go, Python, Node.js, Java, and ordinary CLI workloads are expected to run,
but the only useful compatibility statement is one backed by BuildMax's own
tasks. Upstream documents incomplete support for specialized syscalls,
`io_uring`, filesystems, packet filtering, devices, and nested container
features. See [application compatibility](https://gvisor.dev/docs/user_guide/compatibility/).

BuildMax places particular pressure on:

- namespace and mount operations used by `bwrap`;
- Git checkout and many-small-file workloads;
- Go, npm, and Python package installation;
- Unix sockets and stdio MCP child processes;
- long-lived HTTPS streaming to the worker API and model endpoints;
- filesystem watchers used by development tools;
- subprocess and signal behavior during cancellation; and
- object-storage SDK networking and checksums.

Docker-in-Docker, FUSE, eBPF, KVM inside the worker, custom kernel modules, and
arbitrary host devices are not part of the initial supported worker contract.
An Agent that requires one must fail with an explicit incompatibility or run in
a separately approved runtime profile; it must not broaden the default gVisor
profile.

### 11.2 Performance

gVisor adds syscall, filesystem, and network mediation. Upstream identifies
filesystem and network-heavy workloads as the most likely to regress, while
CPU-heavy work often sees less overhead. That makes repository checkout,
dependency installation, artifact traversal, and model streaming the relevant
BuildMax measurements rather than a synthetic CPU benchmark alone. See the
[production guide](https://gvisor.dev/docs/user_guide/production/).

The qualification report records, for native and gVisor runs:

- Job creation to first heartbeat;
- workspace materialization time;
- representative Git, Go, npm, and Python task duration;
- model stream throughput and latency;
- artifact upload duration;
- peak memory and CPU charged by Kubernetes; and
- cancellation to terminal report latency.

No universal percentage budget is chosen before measurement. The support
decision must publish the observed cost and identify workloads that require a
different qualified profile, rather than hiding a large regression behind one
average.

## 12. Options Considered

### 12.1 Keep Native Containers And Add More Seccomp Rules

Retained as the portable baseline, rejected as the stronger boundary. It keeps
the complete worker on the host kernel and makes every compatibility exception
part of the host-facing policy.

### 12.2 gVisor Plus Bubblewrap

Chosen target. It composes Pod-to-host isolation with command-to-worker policy
and fits the existing one-Job-per-run model.

### 12.3 gVisor Without Bubblewrap

Rejected. It protects the node but does not stop a model command from reading
other files and credentials visible inside the worker Pod or bypassing
BuildMax's domain policy.

### 12.4 Kata Containers Or One MicroVM Per Run

Deferred, not rejected. A hardware-virtualized guest can provide a stronger and
more familiar kernel boundary with wider Linux compatibility, at the cost of
node support, startup, memory, image plumbing, and operational complexity. It
is the next comparison if gVisor cannot run the existing policy or if threat
evidence requires a VM boundary.

### 12.5 Run gVisor Inside The Worker Container

Rejected for the Kubernetes path. The node's OCI runtime should own the outer
sandbox. Nesting `runsc` inside an ordinary worker container complicates
privileges, lifecycle, cgroups, networking, and observability while leaving the
outer Pod under the native runtime.

### 12.6 Automatically Fall Back To runc

Rejected. Availability cannot silently replace the configured isolation class.
A secure run that did not start is a failure; an insecure run reported as the
same work is a false security claim.

## 13. Implementation Plan

### M0. Compatibility Spike

- Install a pinned `runsc` on a disposable kind or equivalent test node.
- Create a RuntimeClass and run the released BuildMax worker image under it.
- Reproduce the exact `bwrap` backend probe organically through a dispatched
  TaskRun.
- Exercise the current root, capability, seccomp, AppArmor, mount, `/proc`, and
  read-only-root settings one variable at a time.
- Determine the smallest gVisor-specific Pod security profile.
- Record incompatibilities and performance evidence in the design status.

Exit criteria: the exact filesystem denial and allowed workspace operation pass
under `runsc`, with no host networking, directfs, privileged Pod, or native
runtime fallback. If they cannot, stop and reopen §6; do not proceed to M1.

### M1. Configuration And Job Construction

- Add typed `runtime_kind` and `runtime_class_name` fields under `worker.k8s`.
- Validate their combinations and reject them under `local_process`.
- Set `PodSpec.RuntimeClassName` for every dispatched worker Job.
- Select the qualified runtime-specific security context.
- Unit-test the full Job shape and absence of fallback.

Exit criteria: a configured class appears exactly once in the Pod template, an
empty class leaves the native profile unchanged, and an unknown runtime kind
stops Server startup.

### M2. Cluster Readiness And Failure Reporting

- Read the configured RuntimeClass at Server startup with get-only RBAC.
- Add Job and Pod condition observation for pre-heartbeat failures.
- Record requested runtime class and profile with TaskRun provenance and trace.
- Keep retry explicit and run-scoped.

Exit criteria: a missing class, unsupported handler, unschedulable node, and
`runsc` startup failure each produce a prompt, distinct failure rather than a
silent native run or a six-hour `SCHEDULED` wait.

### M3. Deployment Integration

- Add an optional gVisor RuntimeClass reference to kind and production
  deployment documentation.
- Keep runtime installation outside BuildMax manifests.
- Add worker node labels, RuntimeClass scheduling guidance, and an admission
  policy example.
- Preserve native Compose and `local_process`; gVisor is Kubernetes-only.

Exit criteria: an operator can adapt the production reference without changing
BuildMax code, and an installation without gVisor remains truthful about using
the native profile.

### M4. Qualification And Support Decision

- Run the functional and adversarial matrix in §14.
- Compare native and gVisor performance on representative tasks.
- Exercise upgrade, node drain, cancellation, OOM, and runtime-handler failure.
- Update the support matrix only after the normal smoke carries the evidence.

Exit criteria: gVisor moves from experimental to supported only when the
committed smoke proves both the outer runtime and inner `bwrap` boundary. The
production reference may make it recommended only after that support decision.

## 14. Validation

### 14.1 Functional Matrix

| Path | Evidence |
|---|---|
| Worker startup | Job reaches authenticated heartbeat with the requested RuntimeClass |
| Filesystem sandbox | Workspace read/write succeeds; write and denied-read outside it fail |
| Network sandbox | allowed domain succeeds; denied domain and direct bypass fail |
| Managed inference | Worker uses internal API without receiving provider credentials |
| Team Secrets | Declared grant reaches the command; BuildMax credentials do not |
| Plugins | Pinned package downloads, verifies, and loads |
| MCP | stdio child starts and remains inside the outer sandbox |
| Artifacts and persistence | workspace state, trace, result, and selected artifact survive normally |
| Cancellation | SIGTERM and user cancel stop the run and preserve a bounded terminal report |
| Limits | CPU, memory, process, and output limits still bind the workload |

### 14.2 Adversarial Matrix

On a disposable node, prove that a worker cannot:

- read a host path not mounted into the Pod;
- see or signal host processes;
- create a host mount or alter host namespace state;
- obtain a Kubernetes ServiceAccount token;
- bypass the internal Worker API network boundary;
- reach a sandbox-denied domain through an alternate client; or
- continue under the native runtime when `runsc` is missing or broken.

The test reports the Pod spec, RuntimeClass, `runsc` version, BuildMax image
digest, command outcome, and node/runtime diagnostics. A model-authored claim
that it was sandboxed is not evidence.

### 14.3 Compatibility Matrix

At minimum, qualification covers:

- Go build and test;
- npm install and a Node build;
- Python virtual environment and package installation;
- Git clone, checkout, diff, and worktree operations;
- archive creation and extraction;
- HTTP streaming and DNS;
- Unix sockets and stdio subprocesses; and
- large and many-small-file artifact paths.

Unsupported kernel-dependent operations are listed explicitly in user and
operator documentation rather than treated as arbitrary task failures.

### 14.4 Repository Checks

- `internal/infra/k8s` unit tests compare native and gVisor Job specs.
- Architecture tests keep runtime settings in config/bootstrap/infra and out of
  `internal/core`.
- Production manifest parsing proves the configured RuntimeClass reaches the
  Job runner.
- The kind smoke's own TaskRun performs the sandbox probe; a one-off manually
  launched Pod is supplemental evidence only.
- `git diff --check`, docs checks, normal tests, and the relevant kind E2E suite
  pass before handoff.

## 15. Risks And Open Questions

| Question | Initial answer |
|---|---|
| Will the exact `bwrap` invocation work? | Unknown until M0; this is the gate, not an implementation detail |
| Should gVisor become the production default? | Not before M4 qualification and measured operator cost |
| Is root plus virtual `SYS_ADMIN` acceptable? | Only if the exact probe requires it and the adversarial test shows it cannot affect the node |
| Which `runsc` platform should BuildMax recommend? | Deployment-specific; qualify the RuntimeClass behavior, leave platform selection to the node operator |
| Does gVisor close the MCP boundary gap? | It improves MCP-to-host isolation, but not MCP-to-worker files, Secrets, or policy; an inner MCP boundary remains useful |
| Does gVisor replace NetworkPolicy? | No; netstack is isolation, not BuildMax destination authorization |
| Does the Server need cluster-wide RBAC? | Get-only access to the configured RuntimeClass; no runtime mutation |
| Can one Team choose a stronger runtime? | No initially; runtime is deployment policy and all workers use the same configured class |
| What if one Agent needs an unsupported kernel feature? | Fail explicitly or use a separately reviewed deployment profile; never silently widen the shared profile |
| When should Kata be evaluated? | If gVisor cannot preserve `bwrap`, compatibility blocks representative work, or the threat model requires hardware virtualization |

The central risk is treating `runtimeClassName` as proof. A class name can
exist while nodes are misconfigured, the image fails, or the inner sandbox no
longer enforces policy. Support therefore follows the organic TaskRun evidence,
not the YAML field.

## 16. Documentation Changes

When support ships:

- [Configuration reference](../reference/configuration.md) documents
  `runtime_kind`, `runtime_class_name`, validation, and no-fallback behavior;
- [Production deployment](../../deployment/production/README.md) documents
  node installation, RuntimeClass, scheduling, admission, upgrade, and
  diagnostics without pretending BuildMax installs `runsc`;
- [Worker seccomp profile](../../deployment/seccomp/README.md) separates the
  native and gVisor Pod profiles;
- [Sandbox guide](../guide/sandbox.md) explains outer runtime isolation versus
  inner command policy;
- [Server architecture](../contribute/architecture/server.md) records runtime
  validation and Job failure observation;
- [Current state](../current-state.md) reports the exact qualification evidence
  and measured limitations;
- [Support matrix](../start/support.md) distinguishes native and qualified
  gVisor workers; and
- [Trust harness](trust-harness.md) marks the Pod-to-host runtime slice closed
  while leaving destination egress and inner MCP policy accurately open.
