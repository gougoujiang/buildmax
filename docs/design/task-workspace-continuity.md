# Task Workspace Continuity

> **Audience:** contributors, product designers, and operators · **Status:** proposed — direction for discussion, implementation not started. This is an independent proposal; a separate draft (`task-workspace-checkpoints.md`, on another branch) explores the same problem with a whole-`home/` snapshot. The two agree on the durability contract and disagree on the filesystem split (§4) and the default restore-failure policy (§6); §17 records both.

Related: [product vision](product-vision.md),
[Agent execution and Task threads](agent-execution-and-task-threads.md),
[graceful shutdown](graceful-shutdown.md),
[unified artifacts](unified-artifacts.md),
[plugin team distribution](plugin-team-distribution.md), and
[data model](../contribute/architecture/data-model.md).

Created: 2026-09-06

## Contents

- [1. Decision](#1-decision)
- [2. Problem And Current Baseline](#2-problem-and-current-baseline)
- [3. Goals And Non-Goals](#3-goals-and-non-goals)
- [4. The Three-Way Filesystem Split](#4-the-three-way-filesystem-split)
- [5. Continuity Contract](#5-continuity-contract)
- [6. Restore Outcome And Failure Policy](#6-restore-outcome-and-failure-policy)
- [7. Snapshot Scope, Format, And Ignore Policy](#7-snapshot-scope-format-and-ignore-policy)
- [8. Storage, Keying, And Commit Protocol](#8-storage-keying-and-commit-protocol)
- [9. Relational Model](#9-relational-model)
- [10. Reading A Task's Prior Outputs](#10-reading-a-tasks-prior-outputs)
- [11. Package And Interface Boundaries](#11-package-and-interface-boundaries)
- [12. Security, Limits, And Retention](#12-security-limits-and-retention)
- [13. Failure And Recovery](#13-failure-and-recovery)
- [14. API, Portal, And Observability](#14-api-portal-and-observability)
- [15. Delivery Plan](#15-delivery-plan)
- [16. Verification](#16-verification)
- [17. Alternatives Considered](#17-alternatives-considered)
- [18. Deferred Questions](#18-deferred-questions)

## 1. Decision

A Task carries one linear, forward-only workspace snapshot as durable
Task-thread state, restored atomically with the Agent session from the same
predecessor TaskRun. The snapshot lives in the configured object store; the
Pod's local disk is scratch and never authoritative.

The snapshot covers **only a Task-private working directory**, kept physically
separate from the run's materialized Team Home and from published Artifacts.
Team Home stays a fresh, current, read-only input each run; the Task workspace
is the private state that continues; Artifacts remain explicit immutable
publications. These three have different owners, lifecycles, and trust levels,
so they get three directories, not one.

This is a deliberately narrow capability: **Task workspace continuity**. It is
the file-state counterpart of the Agent-session continuity already promised by
[agent execution and Task threads](agent-execution-and-task-threads.md). It
does not revive the withdrawn versioned-workspace product — there is no
workspace history, no arbitrary-point restore, no branch or change-set model,
no rollback, and no write-back into Team Home. One snapshot moves forward per
thread; the superseded one becomes garbage.

The short form of the contract is:

> A Continue run sees the workspace its own restored conversation describes.
> When it cannot, the run says so — it never pretends.

## 2. Problem And Current Baseline

A worker run lays out four directories under one run-scoped path
`<workspacesDir>/<team>/tasks/<task>/<run>/`, and `WorkspaceDir` is the run
directory itself:

| Area | Current role | Current durability |
|---|---|---|
| run dir (workspace root) | Where the Agent's file tools read and write | Not uploaded; lost with the Pod |
| `home/` | Read-only copy of Team Home, materialized in each run | Not uploaded; re-materialized next run |
| `artifacts/` | `result.md` plus deliberately produced files | Uploaded at terminal reporting, keyed per run |
| `global/` | Run-scoped `BUILDMAX_HOME`: session, trace, logs, settings, plugins | Allowlisted files uploaded (includes `sessions/`) |

`prepareRunWorkspace` (`internal/agentapp/taskrun/runtime.go`) creates those
directories, restores the previous Agent session when one exists, and
materializes Team Home into `home/`. `reportPersistedRunState` uploads
`artifacts/` and the `global/` allowlist. It never uploads the workspace.

Two concrete gaps follow, and they compound:

- **Gap A — the workspace vanishes.** Files the Agent created or edited outside
  `artifacts/` are neither persisted nor restored. A continued run restores a
  conversation asserting a file was written, into a directory where that file
  does not exist. Both restore paths report success while the model and the
  filesystem disagree.
- **Gap B — even outputs are invisible.** `artifacts/` *is* persisted, but it is
  keyed per run, never materialized into the next run, and no tool lets a run
  read a prior run's artifacts. So even a correctly published deliverable cannot
  be picked up and continued.

The current session restore also fails open silently. `restoreSessionFromPreviousRun`
returns nothing; a missing or torn bundle is discarded and the run starts
fresh, with no recorded outcome. That was acceptable before Continue was a
user-visible contract. It is not acceptable for a thread that claims
continuity, and it is exactly what
[agent execution §6.4](agent-execution-and-task-threads.md) forbids.

Kubernetes does not close the gap. `emptyDir` survives a container restart in
the same Pod but is deleted when the Pod is removed, and a Job may replace a
Pod on another node. The durable boundary must be BuildMax's, not an assumption
about one Pod's lifetime.

## 3. Goals And Non-Goals

### 3.1 Goals

- A Continue run receives the exact workspace state its Task last committed.
- A Retry run receives the exact workspace base the run it repeats started from,
  not that run's partial output.
- Session and workspace restore from one explicit predecessor; neither silently
  falls back while the operation is presented as Continue.
- Every restore outcome is a queryable fact, not a log line or a key probe.
- A Pod may be deleted and a later run may execute on another node or cluster
  attached to the same database and object store.
- Team Home updates made between a Task's runs remain visible to that Task's
  next run. Continuity carries Task-private state, not a frozen copy of team
  input.
- Publication is safe across process death and request retry without a
  transaction spanning MySQL and the object store.
- The default stays portable across private deployments and needs no CSI driver
  beyond scratch storage.

### 3.2 Non-Goals

- Resuming an in-flight model invocation, token stream, or Bash process.
- Automatically re-executing a run after its worker claimed it.
- Zero-RPO recovery from `SIGKILL`, OOM, node loss, or a storage partition.
- A user-facing history of workspace states, arbitrary rollback, file-level
  restore, branching, merging, or collaborative editing.
- Writing a Task workspace back into mutable Team Home.
- Treating a workspace snapshot as an Artifact or exposing its storage key.
- Persisting `home/`, `global/`, `artifacts/`, or `oshome/` inside the
  workspace snapshot.
- Git as a required or hidden persistence engine.
- Incremental or large-workspace acceleration in the first implementation.

## 4. The Three-Way Filesystem Split

This is the load-bearing structural decision, and it is where this proposal
diverges from the whole-`home/` snapshot alternative.

Today `home/` does double duty: it is both the copy of Team Home (shared team
input) and the place the Agent edits (private working state). Snapshotting all
of `home/` conflates them and produces a surprising result: because a snapshot
is restored on the next Continue, a Portal edit to Team Home made after the
Task started is masked by the checkpoint. The team updates a shared file; the
Task never sees it again.

This design keeps the two apart, and makes Artifacts the third, already-distinct
concept explicit:

```text
run dir/
  workspace/    Task-private working state   -> snapshotted, carried forward
  home/         Team Home input (current)    -> materialized fresh each run, read-only
  artifacts/    published deliverables       -> uploaded per run, immutable
  global/       BUILDMAX_HOME (session/trace/logs/plugins) -> allowlist upload
  oshome/       OS HOME for tools            -> ephemeral
```

`WorkspaceDir` becomes `run dir/workspace/` rather than the run dir itself.

The convention the runtime prompt and `AGENTS.md` state:

- **Read team inputs from `home/`.** It is the current Team Home every run,
  read-only by convention. To evolve a team input across turns, copy it into
  `workspace/` and work there.
- **Do durable work in `workspace/`.** It is what continues to the next run.
- **Publish deliverables with the artifact tool.** Artifacts are for people to
  keep, immutable and independently retained.

The cost is one explicit copy when an Agent wants to mutate a team input across
turns. The benefit is that each of the three concepts keeps its own semantics:
team input stays current, Task state continues, publications stay immutable. A
whole-`home/` snapshot is more convenient for edit-in-place but pays for it by
freezing team input and by carrying re-materializable team files in every
snapshot.

`internal/core` is unaffected: the split is runtime-assembly and worker
concern. `internal/core/task` gains a workspace-lineage pointer (§9) but no new
agent-loop concept.

## 5. Continuity Contract

### 5.1 First Run

The first run of a Task has no predecessor workspace. The worker:

1. claims the run;
2. materializes current Team Home into `home/` (read-only input);
3. creates an empty `workspace/`;
4. records the run's workspace base as `empty`; and
5. starts the Agent runtime.

There is no seed snapshot of team input, because team input is not the Task's
workspace. `workspace/` starts empty and becomes the Task's first committed
state when the run succeeds (§5.4). An Agent that needs a team file to work on
copies it from `home/` into `workspace/` — an explicit, recorded act rather
than an implicit freeze.

### 5.2 Continue

Continue fixes two predecessor references atomically when it creates the new
run:

- `previous_task_run_id` supplies the Agent session; and
- `workspace_base_snapshot_id` supplies the Task workspace.

Both must name the **same** predecessor run, so session and files describe one
coherent past. Both are restored before the Agent starts, and both outcomes are
recorded (§6). Team Home is materialized fresh and current, independent of the
snapshot.

### 5.3 Retry

Retry repeats an attempt, so its workspace base is the base of the run it
names, never that run's output:

```text
run A:    base S1 -- execution -- result or partial S2
retry A:  base S1 -- new execution -- result S3
```

Retry does not start from S2 even when S2 exists. A failed attempt may have left
`workspace/` halfway through a change whose external side effects cannot be
inferred from files; reusing it would turn Retry into Resume. The retry records
the session and workspace references it actually received, and the UI must not
imply that Retry reconstructs external effects.

### 5.4 Successful Completion

After the Agent quiesces and the run is about to report success, the worker
captures `workspace/` into an immutable snapshot, commits it, and advances the
Task's workspace head to it (§8). A run that fails or is canceled may capture a
**partial** snapshot as recovery evidence, retained but never made the Task head
by default (§13).

## 6. Restore Outcome And Failure Policy

Every continuation records what actually happened, as a typed field on the run,
readable without touching logs or object storage:

| Outcome | Meaning |
|---|---|
| `restored` | Session and workspace both restored from the named predecessor |
| `fresh` | No predecessor (first run), or a deliberate new-session start |
| `failed_session` | The named session bundle was missing, torn, or unreadable |
| `failed_workspace` | The named workspace snapshot was missing, corrupt, or unauthorized |

**Default policy: fail closed on Continue.** If a Continue names a predecessor
and either restore fails, the run does not start the Agent. Starting from a
fresh session or a fresh workspace while presenting the operation as Continue is
forbidden.

This reverses an earlier lean toward degraded-visible continuation, and the
reason is the storage change this design makes. Once the workspace lives in a
durable object store rather than on ephemeral Pod disk, a restore failure means
real loss or corruption, not the routine cache miss it would have been. The
dangerous case — a session that confidently references files a degraded run
silently dropped — is precisely the integrity violation the contract exists to
prevent. When failure is rare and its silent form is harmful, fail-closed is the
honest default.

Fail-closed is not a dead end. An **explicit** recovery action may let a user
continue from the last durable workspace head without the immediately previous
session, or continue the session while accepting an empty workspace. Each is a
new, recorded user decision surfaced after a failure — never an implicit
fallback the Continue path takes on its own. A later per-Agent policy may opt a
specific Agent into degraded-visible continuation where its work tolerates it;
that is evidence-gated and not the default.

## 7. Snapshot Scope, Format, And Ignore Policy

The snapshot covers `workspace/` only. `home/`, `artifacts/`, `global/`, and
`oshome/` are excluded by construction, not by filter.

Within `workspace/`, an ignore policy excludes reconstructible bulk so a
snapshot stays small and fast:

- built-in defaults: `node_modules/`, `.venv/`, `__pycache__/`, `target/`,
  `dist/`, `build/`, and similar derived directories;
- a `.gitignore` present in `workspace/` is honored; and
- a `.buildmaxignore` may add or override entries.

A per-Task **size ceiling** bounds the committed snapshot. Exceeding it is a
typed, surfaced outcome — a warning that names what pushed it over, and, past a
hard limit, a failure to commit the snapshot rather than a silent truncation. A
run whose snapshot cannot be committed still reports its result and artifacts;
the *next* Continue is then `failed_workspace` and handled by §6.

The first implementation captures a complete, versioned compressed archive of
the surviving tree. It does not attempt an incremental filesystem protocol
before measurements show complete archives are inadequate (§17, §18).

## 8. Storage, Keying, And Commit Protocol

A snapshot is stored beside the run that produced it, in the same run-global key
space the session bundle already uses:

```text
<team>/tasks/<task>/<run>/global/workspace/snapshot.tar.zst
```

Restore for run N+1 reads the snapshot of the run named by
`workspace_base_snapshot_id`, exactly as session restore reads the bundle of the
run named by `previous_task_run_id`. Same-predecessor keying guarantees session
and workspace come from one coherent past.

There is no transaction spanning object storage and MySQL, so the protocol is
ordered to be safe across process death and retry:

1. write the immutable snapshot payload to a content-addressed or run-scoped key
   (a repeated write of identical content is a no-op);
2. in one database write, record the snapshot row and advance the Task's
   `workspace_head_snapshot_id` from an expected current value (compare-and-set);
   and
3. a crash between (1) and (2) leaves an orphan payload, reclaimed by retention
   (§12); a crash before (1) leaves the head unchanged and the run is retried
   from its recorded base.

The head is a database pointer, never a mutable object key. The payload is
immutable and never rewritten.

## 9. Relational Model

The `xxxRow` structs in `internal/infra/db` remain schema authority; this is the
target shape.

- `task` gains `workspace_head_snapshot_id` (nullable) — the latest committed
  snapshot accepted as the Task's recoverable state.
- `task_run` gains:
  - `workspace_base_snapshot_id` (nullable) — the snapshot this run started
    from; null means an empty base;
  - `workspace_result_snapshot_id` (nullable) — the snapshot this run committed,
    if any;
  - `continuity_outcome` — the §6 enum.
- A `task_workspace_snapshot` row records id, Task, producing run, kind
  (`result` | `partial`), storage key, byte size, content hash, and creation
  time. Snapshots are immutable; the Task head is a pointer among them.

`previous_task_run_id` (already present) and `workspace_base_snapshot_id` are
set together at Continue admission and must name the same predecessor.

## 10. Reading A Task's Prior Outputs

Carrying `workspace/` forward closes Gap A. Gap B — reading a prior run's
published Artifacts — is closed separately, because Artifacts are immutable
publications, not working state, and should not be copied into every snapshot.

A run may read its own Task's earlier Artifacts through a bounded tool
(`ListArtifacts` / `ReadArtifact`), scoped to the run's Team and Task and
refusing any other Task or Team. Auto-materializing all prior artifacts into
each run is rejected: an artifact set can be large, and most runs need none of
it. The tool makes the access explicit and bounded.

This item is independently shippable and is the smallest real user-visible win;
it does not wait on the snapshot machinery.

## 11. Package And Interface Boundaries

| Responsibility | Owner |
|---|---|
| Workspace directory layout and the `workspace/` root | `internal/agentapp/taskrun` (run paths) |
| Capture, restore, and ignore policy | `internal/agentapp/taskrun` |
| Snapshot payload storage | `internal/infra/objectstore` (a workspace key space beside run-global) |
| Snapshot rows, head pointer, compare-and-set advance | `internal/infra/db` |
| Continuity outcome as a domain fact | `internal/core/task` (a field, no new loop concept) |
| Base/head resolution at admission and retry | `internal/service/task` |
| Prior-artifact read tool | `internal/tool` |
| Team-scoped API and Portal surfacing | `internal/server/handlers`, `portal/` |

The worker resolves the base from server-provided run state, never from
model-provided arguments, consistent with the worker trust boundary in
[agent execution §10](agent-execution-and-task-threads.md).

## 12. Security, Limits, And Retention

- A snapshot is Team- and Task-scoped; restore validates that the base snapshot
  belongs to the Task being continued. A snapshot key is never exposed to a
  model or a user as a handle.
- Snapshot bytes obey the size ceiling (§7); redaction rules that apply to
  traces do not apply to workspace files, so the ceiling and the ignore policy
  are the only bounds — the workspace holds the Agent's own work, not the
  redacted trace.
- **Retention / GC.** Continue needs only the snapshot on the Task's current
  head. Once a later run commits a new head and succeeds, superseded `result`
  snapshots and any orphan payloads from an interrupted commit are eligible for
  reclamation. Partial snapshots are retained for a bounded recovery window,
  then reclaimed. This keeps storage at roughly one snapshot per live Task
  thread, not one per run forever — the property that separates a continuity
  carry from a version store.
- Team Home retention is unchanged; Artifacts keep their own retention. A
  workspace snapshot is none of these and is never presented as an Artifact.

## 13. Failure And Recovery

| Event | Behavior |
|---|---|
| Run succeeds | Capture `workspace/`, commit snapshot, advance head (§8) |
| Run fails or is canceled | Optionally capture a `partial` snapshot as evidence; head unchanged |
| Snapshot commit fails after result | Result and artifacts still reported; next Continue is `failed_workspace` (§6) |
| Continue base missing/corrupt | Fail closed; user offered explicit recovery (§6) |
| Pod deleted mid-run | No result snapshot; Task head unchanged; a new run starts from the recorded base, not the lost disk |
| Two workers race a commit | Compare-and-set head advance; one wins, the other is a no-op; no head regresses |
| Orphan payload from crash between write and pointer | Reclaimed by retention; never referenced because no row points to it |

Recovery is bounded by the last committed head, not by Pod lifetime. The design
makes no zero-RPO promise: work done after the last successful run and before a
crash is lost, and that is stated, not hidden.

## 14. API, Portal, And Observability

- The Task page shows, per turn, the continuity outcome (§6) and whether the run
  continued, retried, or started fresh — so a `failed_workspace` turn reads as a
  handled state, not a mysterious empty directory.
- A failed Continue surfaces the explicit recovery choices (§6) rather than a
  bare error.
- Task-scoped API responses expose the continuity outcome and the base/result
  snapshot references as opaque ids; never the storage key.
- Operators get snapshot count, bytes per Task, orphan-reclaim lag, and
  continuity-outcome distribution, kept separate from model-task quality
  metrics.

## 15. Delivery Plan

BuildMax is Alpha; each phase changes row structs, domain types, worker wire
types, handlers, OpenAPI, Portal, tests, and docs together, with no
compatibility interpreter for the old shapes.

**Phase 1 — make the current gap honest, and close Gap B.** Give session
restore a recorded outcome; persist and surface `continuity_outcome`; state in
context and UI that the workspace does not yet carry (fail closed is not yet
possible because nothing is stored, so the honest state today is an explicit
"prior files are not available"). Add the Task-scoped prior-artifact read tool.
This phase needs no snapshot storage and delivers the visibility fix and the
smallest UX win on its own.

**Phase 2 — the workspace carry.** Introduce `workspace/` as the working root
and the three-way split (§4). Capture on success, restore on Continue from the
same predecessor, commit protocol and head pointer (§8), the four continuity
outcomes with fail-closed default (§6), ignore policy and size ceiling (§7), and
retention/GC (§12).

**Phase 3 — evidence-gated.** Incremental or content-addressed snapshots if
complete-archive cost proves inadequate; per-Agent degraded-visible opt-in; a
git-backed snapshot path only if measurement shows Task workspaces are
dominated by cloned repositories.

## 16. Verification

- **Object-store / MySQL (`./make test mysql`):** snapshot commit and head
  compare-and-set are atomic and idempotent; a duplicate commit is a no-op; two
  racing commits leave one head with no regression; an orphan payload is
  reclaimed and never referenced; base and head resolve to the right predecessor.
- **Runtime:** capture honors the ignore policy and size ceiling; restore
  reproduces `workspace/` byte-for-byte; a missing or torn snapshot yields
  `failed_workspace` and does **not** start the Agent; session and workspace
  always restore from the same predecessor.
- **Service:** Continue names one coherent predecessor; Retry resolves to the
  named run's base, not its result; the prior-artifact tool refuses cross-Task
  and cross-Team access.
- **End-to-end:** a two-run Task edits a file in run 1 and reads it back in run
  2; a run whose snapshot base is deleted fails closed and offers recovery; a
  Team Home file changed between runs is visible to run 2 (proving the split of
  §4).
- Scoped documentation, the OpenAPI exact-match test, architecture tests, and
  `git diff --check` pass with the same change.

## 17. Alternatives Considered

### 17.1 Snapshot the whole `home/`

Treat the materialized-Team-Home directory as the working area and snapshot all
of it (the companion proposal's choice). It is more convenient — the Agent edits
team files in place with no copy step. Rejected as the default here because it
conflates team input with Task state: restoring the snapshot on Continue masks
any Team Home update the team made after the Task started, and every snapshot
carries a copy of re-materializable team files. The three-way split trades one
explicit copy for keeping team input current and snapshots small.

### 17.2 Degraded-visible continuation as the default

Continue with a marker when restore fails, rather than failing closed. Rejected
as the default: with durable object-store snapshots a restore failure is real
loss, not a routine miss, and a session that references dropped files is the
integrity violation the contract exists to prevent. Retained as an explicit,
recorded user recovery action and a possible per-Agent opt-in (§6, §18).

### 17.3 Team Home as the carry mechanism

Write the Task workspace back into Team Home between runs. Rejected: Team Home is
shared, Portal-written team state; agent write-back raises concurrency,
provenance, and authorization questions that are a separate governance design.
Home stays read-only from runs.

### 17.4 Artifacts as the carry mechanism

Use published Artifacts as the working set. Rejected: Artifacts are immutable,
run-scoped publications for people to keep, with their own retention. A mutable
working set has different lifecycle and trust; conflating them would either make
Artifacts mutable or make the working set immutable. Gap B is closed with a read
tool instead (§10).

### 17.5 Git-backed snapshots

Init a repo in `workspace/` and carry a pack. Rejected as the default: worker
workspaces are plain non-git directories, Task workspaces may themselves contain
cloned repositories (nested git is a hazard), and the local git-worktree feature
already excludes workers. Kept as an evidence-gated Phase 3 option if repos
dominate real workspaces.

### 17.6 A persistent volume as the source of truth

Make a Pod's persistent volume authoritative and skip object-store snapshots.
Rejected: it ties a Task to a node or a CSI driver, breaks the "run anywhere
attached to the same store" goal, and conflicts with the portable
single-binary/private-deployment baseline. A persistent volume may later
accelerate large workspaces without becoming the durable boundary.

## 18. Deferred Questions

- The size ceiling and default ignore set, which real workspaces should tune.
- Whether complete archives are adequate or an incremental protocol is worth its
  complexity.
- Whether any Agent's work genuinely tolerates degraded-visible continuation
  enough to warrant the per-Agent opt-in.
- How long partial snapshots stay warm before reclamation.
- Whether the explicit copy from `home/` to `workspace/` needs a runtime
  affordance (a tool or a convention) beyond the prompt instruction.
- Whether a future Team Home revision or transaction design changes how first-run
  input is frozen, independent of Task snapshot semantics.
