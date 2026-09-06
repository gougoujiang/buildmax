# Task Workspace Checkpoints

> **简体中文：** [阅读中文镜像](../zh-CN/design/task-workspace-checkpoints.md)

> **Audience:** contributors, product designers, and operators · **Status:** planned — direction accepted, implementation not started

Related: [product vision](product-vision.md),
[Agent execution and Task threads](agent-execution-and-task-threads.md),
[graceful shutdown](graceful-shutdown.md),
[unified artifacts](unified-artifacts.md),
[worker run token](worker-run-token.md),
[enterprise deployment](enterprise-deployment.md), and
[data model](../contribute/architecture/data-model.md).

Created: 2026-09-06

## Contents

- [1. Decision](#1-decision)
- [2. Problem And Current Baseline](#2-problem-and-current-baseline)
- [3. Goals And Non-Goals](#3-goals-and-non-goals)
- [4. Ownership And Vocabulary](#4-ownership-and-vocabulary)
- [5. Continuity Contract](#5-continuity-contract)
- [6. TaskRun Lifecycle](#6-taskrun-lifecycle)
- [7. Storage And Archive Format](#7-storage-and-archive-format)
- [8. Commit Protocol And Consistency](#8-commit-protocol-and-consistency)
- [9. Relational Model](#9-relational-model)
- [10. Package And Interface Boundaries](#10-package-and-interface-boundaries)
- [11. Kubernetes Runtime And Storage Profiles](#11-kubernetes-runtime-and-storage-profiles)
- [12. Security, Limits, And Retention](#12-security-limits-and-retention)
- [13. Failure And Recovery Matrix](#13-failure-and-recovery-matrix)
- [14. API, Portal, And Observability](#14-api-portal-and-observability)
- [15. Delivery Plan](#15-delivery-plan)
- [16. Verification](#16-verification)
- [17. Alternatives Rejected](#17-alternatives-rejected)
- [18. Deferred Questions](#18-deferred-questions)

## 1. Decision

BuildMax will keep a TaskRun's active filesystem on local scratch storage and
persist a Task's recoverable workspace as immutable checkpoints in the
configured object store.

The default execution profile is:

```text
Task workspace checkpoint in S3 / MinIO / local persistent storage
                              |
                              | materialize
                              v
                  TaskRun emptyDir/workspace
                              |
                              | capture after quiescence
                              v
                  immutable next checkpoint
```

The checkpoint, not the Pod volume, is authoritative. Kubernetes `emptyDir`
remains the default working disk because it is fast, per-Pod, portable, and
already used by the worker Job. A persistent volume may later accelerate large
workspaces, but it does not replace the object-store checkpoint or change Task
semantics.

This record introduces a deliberately narrow capability: **Task workspace
continuity**. It does not revive the withdrawn generic versioned-workspace
product. There is no workspace timeline, arbitrary rollback, branch model,
change-set review, merge, or automatic write-back into shared Space files in this
design.

The recovery unit is a complete, immutable snapshot of `workspace/`. The first
implementation uses a versioned compressed archive. It does not attempt an
incremental filesystem protocol before measurements show that complete
archives are inadequate.

## 2. Problem And Current Baseline

The worker already separates one run into four filesystem areas:

| Area | Current role | Current durability |
|---|---|---|
| `home/` | Materialized copy of Space Home; the files the Agent works on | Not uploaded after the run |
| `artifacts/` | Run output, including `result.md` and deliberately produced files | Uploaded at terminal reporting |
| `global/` | Run-scoped `BUILDMAX_HOME`: session, trace, logs, settings, plugins | Selected files uploaded at terminal reporting |
| `oshome/` | Empty private operating-system home for tools and credential files | Never uploaded |

`internal/agentapp/taskrun.prepareRunWorkspace` creates those directories,
restores the previous Agent session when one exists, and materializes current
Space Home into `home/`. `reportPersistedRunState` uploads `global/` and
`artifacts/`, but not `home/`.

That layout is the baseline this design replaces, not the target layout. In
particular, the runtime currently passes the whole TaskRun directory as the
Agent workspace root. File tools, artifact publication, workspace-local
configuration discovery, Bash's working directory, and the sandbox therefore
do not agree that `home/` is the workspace. Prompt text asks the Agent not to
write `global/`, but nesting runtime state below the writable tool root is not
an access boundary. It also permits a valid top-level write which a future
`home/`-only checkpoint would silently omit.

Consequently, a continued Task can restore model-visible history which says a
file was changed while receiving a fresh copy of Space Home in which that change
does not exist. The model and filesystem can disagree even though both restore
paths individually report success.

The existing session restore also fails open without recording its outcome. A
missing half of the bundle is discarded and the run starts fresh. That behavior
was acceptable before Continue became a user-visible contract; it is not
acceptable for a Task which claims continuity.

Kubernetes does not close the gap. An `emptyDir` survives a container restart
inside the same Pod, but is permanently deleted when the Pod is removed. A Job
controller can create a replacement Pod after a Pod failure, and Kubernetes
explicitly requires Job applications to handle temporary files, locks, partial
outputs, and possible duplicate starts. The durable boundary therefore belongs
in BuildMax rather than in an assumption about one Pod's lifetime.

## 3. Goals And Non-Goals

### 3.1 Goals

- A Continue run receives the exact committed workspace head of its Task.
- A Retry receives the exact workspace base used by the run it repeats.
- Session history, workspace state, and the Plugin environment restore from
  explicit compatible predecessors; none may silently fall back while the Task
  claims continuity.
- A Pod may be deleted and a later TaskRun may execute on another node or in
  another cluster attached to the same database and object store.
- Every recovery outcome is queryable without reading logs or object-store
  keys.
- Checkpoint publication is safe across process death, request retry, and the
  lack of a transaction spanning MySQL and object storage.
- The default remains portable across private deployments and requires no CSI
  driver beyond what Kubernetes itself needs for scratch storage.
- The Agent's writable workspace, runtime-managed `BUILDMAX_HOME`, and hidden
  process scratch have enforced filesystem boundaries rather than prompt-only
  conventions.
- Space files seed a private Task workspace snapshot; later Space mutations do
  not change an existing Task and Agent edits do not write back implicitly.
- Artifact publication remains an independent service and requires no
  `artifacts/` or `output/` directory in the Agent filesystem.
- An Agent may acquire a Plugin for later execution without making an expanded
  `plugins/` directory the durable record.

### 3.2 Non-Goals

- Resuming the same model invocation, token stream, or in-progress Bash
  process.
- Automatically re-executing a TaskRun after its worker has claimed it.
- Zero-RPO recovery from `SIGKILL`, OOM kill, node loss, or storage partition.
- A user-facing history of every workspace state.
- Arbitrary rollback, file-level restore, branching, merging, or collaborative
  editing.
- Synchronizing a Task workspace back into mutable Space files.
- Treating a checkpoint as an Artifact or publishing its storage location.
- Persisting `buildmax-home/`, an Artifact staging directory, OS home, or hidden
  runtime scratch inside the workspace archive.
- Hot-loading a newly acquired Plugin into an already assembled TaskRun.
- Git as a required or hidden persistence engine.
- Large-workspace acceleration in the first implementation.

## 4. Ownership And Vocabulary

The durable concepts remain separate:

| Concept | Owner | Meaning |
|---|---|---|
| Space files (current Space Home) | Space | Mutable shared input files managed outside a Task |
| Task workspace | Task | Private continuing file state for one Agent objective |
| Agent session | Task | Model-visible execution history and compacted continuity state |
| Plugin environment | Task by default | Immutable set of Plugin package pins available to the Task |

A **workspace checkpoint** is an immutable, complete representation of the
Task's `workspace/` at one boundary. It is server metadata plus one storage payload.
It is not directly editable.

A **base checkpoint** is the workspace a TaskRun is authorized to read and
modify. It is fixed before the first model or tool call. A **result checkpoint**
is the workspace captured after a run has quiesced. A **partial checkpoint** is
a result captured after cancellation, interruption, or execution failure; it is
retained as recovery evidence but is not the Task head by default.

The **Task workspace head** is the latest checkpoint accepted as the Task's
recoverable state: initially its seed, then each successful result checkpoint.
It is a database pointer, never a mutable object-store key.

Task owns the linear workspace lineage for the same reason it owns the Agent
session lineage: Task is the durable thread. TaskRun owns the base it received,
the result it attempted to publish, and the recovery facts for that execution.
Space remains the authorization boundary for all of them.

Artifacts are deliberately absent from this layout. `UploadArtifact` publishes
a chosen regular file from `workspace/` through the Artifact service. The
runtime does not create an `artifacts/` or `output/` directory, and the final
TaskRun reply is persisted directly by the TaskRun result service.

### 4.1 Target Runtime Directory Boundaries

The Agent runtime has two logical roots:

```text
<task-run-root>/
├── workspace/       # Agent cwd; freely readable and writable; checkpointed
└── buildmax-home/   # runtime-managed session, Plugin, settings, and diagnostics
```

The implementation may also need an OS home, archive staging, sockets, locks,
and temporary files. Those are hidden process scratch, not a third Agent-facing
directory, and are never checkpointed.

`workspace/` is the one root supplied to Read, Write, Edit, Glob, Grep, Bash,
workspace configuration discovery, the sandbox, and `UploadArtifact`. The
model does not receive general filesystem-tool access to `buildmax-home/`.
Runtime components use that directory through typed stores and configuration
APIs. A mode bit or instruction saying "do not write" is not the boundary.

No generated TaskRun `AGENTS.md` is written above the workspace. The runtime
prompt carries runtime context, while `workspace/AGENTS.md` and
`workspace/.buildmax/` are discovered naturally from the actual workspace
root.

### 4.2 `buildmax-home/` Persistence Policy

`buildmax-home/` is disposable as a directory. The system persists only typed,
allow-listed state:

| Data | Persist | Restore into a later run | Authority |
|---|---:|---:|---|
| Session `meta.json` and `history.jsonl` | yes | yes | Session checkpoint |
| Notes, todos, compaction, tool outcomes, and session head | yes, inside the journal | yes | Session checkpoint |
| Run trace and bounded logs | by retention policy | no | TaskRun evidence |
| Resolved model, Agent, sandbox, permission, MCP, and Plugin revisions | redacted manifest | resolve/materialize again | Server and catalogs |
| Plugin package bytes | once when not already available | materialize by immutable package reference | Plugin Package Store |
| Explicit future Plugin mutable state | only under a declared scope and schema | according to that scope | Plugin State service |
| Indexes, locks, sockets, downloads, caches, and temporary files | no | no | Recreated or discarded |
| Credentials, run tokens, Secret grants, and generated auth files | never | reacquire through the Secret/credential authority | Secret service |

Saving a diagnostic is not permission to restore it. In particular, traces and
logs remain evidence, while raw `settings.yaml`, `mcp.json`, and expanded Plugin
directories are replaced by redacted resolved manifests plus immutable source
references. Long-term Agent memory, if introduced, is its own authorized
domain resource rather than an accidental subdirectory of `BUILDMAX_HOME`.

## 5. Continuity Contract

### 5.1 First Run And Space Seed

The first TaskRun starts from a snapshot of the Space's shared files. The
worker:

1. claims the TaskRun;
2. materializes the selected Space file snapshot into an empty `workspace/`,
   preserving relative paths at the workspace root;
3. captures and publishes a `seed` checkpoint;
4. records that checkpoint as the TaskRun's base; and only then
5. starts the Agent runtime.

The materialized files become a private, writable fork. Agent edits never
write back to the Space implicitly. Later Space uploads, replacements, and
deletions never change the existing Task: Continue restores its Task workspace
checkpoint instead of importing the latest shared files again. A refresh or
publish-back operation would be an explicit future product action with conflict
and authorization semantics.

The seed is the exact directory the first run observed. The checkpoint records
the source Space snapshot identity when the source can provide one; otherwise
it records the materialized file manifest and digest. An upload racing a legacy
non-transactional materialization may still produce a mixture of Space writes,
but the committed seed freezes and identifies that exact copy before the Agent
causes an external side effect.

If the seed cannot be committed, the Agent does not start. The run fails before
model or tool execution because it has no reproducible base.

### 5.2 Continue

Continue fixes both predecessor references when it creates the new TaskRun:

- `previous_task_run_id` supplies the Agent session; and
- `workspace_base_checkpoint_id` supplies the Task workspace.

Both restore operations complete before the Agent starts, and both outcomes are
recorded. A missing, corrupt, unauthorized, or unsupported checkpoint fails the
run closed. A missing or torn session bundle does the same. Starting from a
fresh session or current Space files while presenting the operation as Continue
is forbidden.

An explicit future recovery action may allow a user to continue from the last
durable workspace head without the immediately previous session, or vice versa.
That is a new user decision and must not be an implicit fallback.

### 5.3 Retry

Retry repeats an execution attempt, so its workspace base is the base of the
TaskRun it names:

```text
run A: base C1 ---- execution ---- partial or result C2
retry A: base C1 -- new execution -- result C3
```

Retry does not start from C2, even when C2 is available. The failed attempt may
have left a repository halfway through a migration, a generated file beside an
old manifest, or an external side effect which cannot be inferred from files.
Reusing its partial state would change Retry into Resume.

Retry still restores the Task's session under the existing retry rules. The
session, workspace, and Plugin environment references recorded on the retry
must describe the combination actually supplied; the UI must not imply that
retry reconstructs external effects.

### 5.4 Plugin Environment Continuity

`buildmax-home/plugins/` is a materialized projection, never a checkpoint. The
durable object is a **Plugin environment revision**: an immutable ordered set of
`plugin_id`, version, package digest, package reference, source, installer,
declared scope, and resolved permission revision.

An Agent may use typed `PluginSearch`, `PluginInspect`, and `PluginInstall`
capabilities. It may not make a durable installation by writing into
`buildmax-home/plugins/` with Bash. A catalog package is referenced without
uploading its bytes again. A private or Agent-produced package is normalized,
inspected, digest-verified, and stored once in the Plugin Package Store before
an environment revision may reference it. Extracted directories, downloads,
caches, credentials, and undeclared Plugin state are not uploaded.

The assembled tool registry, system prompt, MCP servers, hooks, sandbox rules,
and Secret requirements remain immutable within one TaskRun. Installing a
Plugin therefore creates a capability-change boundary: the worker quiesces,
commits the current session and workspace, records the new Plugin environment,
and a new TaskRun materializes it before continuing the same Task. Product UI
may present this as automatic continuation, but the execution record retains
the boundary.

Autonomous installation defaults to `task` scope and still requires the Space
to permit autonomous acquisition; ordinary Task execution authority does not
imply Plugin-management authority. Promotion to an Agent or the whole Space
requires an explicit authorized action. Plugins which introduce
executable tools, MCP servers, hooks, broader filesystem or network access, or
Secret requirements pass the corresponding policy and approval gates; package
installation never grants a Secret implicitly.

Continue uses the Task's current Plugin environment head. Retry uses the base
Plugin environment of the TaskRun it repeats. A failed or interrupted
installation may leave an unreferenced package for garbage collection but does
not advance the Task's Plugin environment head.

### 5.5 Successful Completion

After the Agent loop and every child process it owns have stopped, the worker
captures `workspace/`, uploads the payload, verifies it, and asks the Server to
commit a `successful` result checkpoint.

The successful checkpoint descriptor travels with the terminal TaskRun report.
The Server records the checkpoint and advances
`task.workspace_head_checkpoint_id` in the same transaction which accepts that
report. A stale or duplicate terminal report may return success idempotently,
but it cannot change the head selected by the first accepted report.

TaskRun status and workspace persistence are related but distinct facts. If the
Agent produced a successful answer and final checkpoint publication fails, the
run remains `SUCCEEDED` and records `workspace_checkpoint_status=failed`. Its
final reply and any independently published Artifacts remain valid. The Task
workspace head does not advance, and Continue is refused until the user
explicitly chooses a recovery base or the checkpoint is repaired. Marking the
whole run failed would invite a Retry which can repeat external effects merely
because storage failed after execution.

### 5.6 Failure, Cancellation, And Interruption

For an execution failure, user cancellation, or graceful process interruption,
the worker may publish a `partial` checkpoint within the same bounded reporting
window used for terminal reporting.

A partial checkpoint:

- is linked from its TaskRun;
- never advances the Task workspace head automatically;
- is not used by Retry;
- is retained for diagnosis and a future explicit Resume action; and
- may be absent when the reporting deadline is too short.

The TaskRun's terminal status remains authoritative. Checkpoint failure cannot
erase or rewrite a terminal outcome already determined by execution.

### 5.7 Hard Worker Loss

`SIGKILL`, OOM kill, node loss, forced Pod deletion, and a dead node provide no
cleanup opportunity. The liveness reaper closes the TaskRun as it does today.
The last committed Task head and the run's immutable base remain recoverable;
the uncommitted work of the lost process does not.

BuildMax does not silently dispatch the same TaskRun again. Model-chosen tools
may already have written outside the workspace, called an API, pushed a branch,
or opened a pull request. Only an explicit Retry or future Resume creates a new
execution attempt.

## 6. TaskRun Lifecycle

The target lifecycle is:

```text
PENDING
   |
   v
SCHEDULED
   |
   | atomic worker claim
   v
RUNNING
   |
   | restore or create workspace and session bases
   | materialize the immutable Plugin environment
   | record restore outcome
   | no model/tool call before this gate
   v
EXECUTING (logical phase; not a new public TaskRun status)
   |
   | quiesce Agent and owned children
   | capture and upload result/partial checkpoint
   v
terminal TaskRun status + workspace/session/Plugin environment outcomes
```

No new public TaskRun execution status is required. Workspace preparation,
checkpointing, and Plugin environment materialization are subsidiary state
recorded in dedicated columns. Adding
`PREPARING` or `CHECKPOINTING` to the TaskRun status vocabulary would make
every status consumer understand storage implementation detail and would still
not express success-with-checkpoint-failure correctly.

The worker heartbeat starts after the atomic RUNNING claim and continues during
materialization, execution, and checkpoint upload. The liveness reaper can
therefore detect a worker lost in any of those phases. Long object-store work
must not appear as silence.

The runtime must quiesce before capture:

- no new tool call may start;
- foreground tool calls have returned or been canceled;
- child processes owned by the runtime have exited or been killed; and
- the session journal is closed before its separate upload.

A process deliberately daemonized outside the runtime's ownership is an
unsupported source of workspace mutation. Checkpointing a tree while an
untracked writer is active cannot provide a coherent boundary.

## 7. Storage And Archive Format

### 7.1 Payload Layout

The first payload format is `tar.zst.v1`: one Zstandard-compressed tar archive
per checkpoint. The storage key is content-addressed inside the owning Space:

```text
<prefix>/<space_id>/workspace/blobs/sha256/<64-lowercase-hex>
```

The digest covers the exact compressed bytes. A checkpoint row records the
payload format, digest, stored size, uncompressed regular-file bytes, and entry
count. The object-store adapter derives the key; no API or trace exposes it.

Space-scoped content addressing permits safe reuse inside one authorization
boundary without revealing whether another Space stored the same content. The
first implementation may use `ObjectExists` to avoid a duplicate upload. It
must still verify a fetched payload against the recorded digest.

A single archive is chosen before per-file content-addressed manifests because
source trees often contain many small files. One archive requires one streamed
GET and avoids a LIST plus thousands of request round trips. The format column
leaves room for a later chunked or manifest-backed representation without
changing the checkpoint domain model.

### 7.2 Canonical Archive Rules

Archive creation walks paths in lexical slash-separated order. Every header
uses a path relative to `workspace/`. The implementation preserves:

- regular-file bytes;
- directories;
- executable and ordinary permission bits;
- modification time; and
- safe relative symbolic links.

It does not preserve owners, groups, ACLs, extended attributes, sparse-file
layout, device nodes, sockets, or FIFOs. UID and GID are normalized to zero and
names are empty. Setuid, setgid, and sticky bits are stripped.

Absolute paths, `..` traversal, duplicate normalized paths, hard links, device
nodes, sockets, FIFOs, absolute symlinks, and relative symlinks which escape
the archive root make capture fail. Extraction applies the same validation
independently; trusting an archive merely because BuildMax produced it would
turn object-store corruption into a filesystem escape.

There are no implicit ignore rules in version 1. `.git`, generated files, and
caches inside `workspace/` are part of the checkpoint because silent exclusion would
make “exact workspace” false. Operators bound admissible size instead. An
explicit exclusion policy may be designed later, but its omitted paths and
recovery consequences must be visible on the TaskRun.

### 7.3 Materialization

Restore streams the archive into a new temporary directory on the same
filesystem as the target, validates every entry and the aggregate limits,
verifies the payload digest, and only then renames the directory into place.
The Agent never sees a half-extracted workspace.

The worker does not discover a checkpoint through object-store listing. It
receives an immutable descriptor from the Server and performs a direct GET.
LIST consistency and unrelated objects therefore cannot affect restoration.

### 7.4 Why The Archive Is Not An Artifact

A checkpoint is automatic execution state and may contain source, caches, or
private intermediate files. An Artifact is an intentional publication with a
stable user-facing identity. Automatically registering checkpoint archives as
Artifacts would violate the explicit-publication decision in
`unified-artifacts.md` and would expose an implementation format as product
content.

## 8. Commit Protocol And Consistency

MySQL and object storage cannot participate in one transaction. Publication
therefore uses **immutable bytes first, authoritative pointer second**:

1. capture the archive to a bounded local temporary file while computing its
   SHA-256 and counters;
2. upload it to the content-addressed key, or confirm identical bytes already
   exist;
3. read metadata or stream the object as needed to verify the exact key;
4. send a seed descriptor through its preparation call, or carry a result or
   partial descriptor on the terminal TaskRun report, over the
   run-token-authenticated worker API;
5. validate Space, Task, TaskRun, checkpoint kind, digest, format, and bounds;
6. insert the checkpoint and update the TaskRun and optional Task head in one
   MySQL transaction; for a result or partial checkpoint this is the same
   transaction which accepts the terminal run outcome; and
7. return the committed checkpoint identity to the worker.

The finalization call is idempotent. Repeating the same TaskRun, checkpoint
kind, digest, and metadata returns the existing checkpoint. Repeating a kind
with different bytes is a conflict; it never rewrites the accepted checkpoint.

Only the first valid terminal transition can advance the Task head. The update
checks the TaskRun's expected status, the Task's active run, and the base
checkpoint recorded on that run. This prevents a late worker, duplicate Pod,
or retried HTTP request from moving the head behind or around a later run.

An uploaded payload with no committed checkpoint row is an orphan, not a
checkpoint. A retention sweep may delete unreferenced blobs older than a
conservative grace period. The sweep must derive liveness from database
references and must tolerate a concurrently completing upload; recent objects
are never candidates.

A database row never points at bytes which have not been verified. If database
commit fails after upload, the old Task head remains authoritative. If the
response is lost after commit, idempotent finalization returns the same row.

AWS S3 provides atomic updates to an individual key and strong read-after-write
consistency, but BuildMax also supports S3-compatible stores. The protocol uses
immutable keys and explicit verification rather than treating AWS behavior as
an undocumented requirement on every compatible backend.

## 9. Relational Model

### 9.1 `workspace_checkpoint`

`workspace_checkpoint` is a server entity because a recovery choice may need
to name it across an API boundary. It receives an opaque public handle from
`util.NewPublicID`; internal relationships use numeric keys.

Target row:

| Column | Type | Null | Notes |
|---|---|---:|---|
| `id` | `bigint unsigned` | no | Internal primary key |
| `public_id` | `char(20) ascii_bin` | no | Unique public handle |
| `space_id` | `bigint unsigned` | no | Authorization owner, indexed |
| `task_id` | `bigint unsigned` | no | Workspace owner, indexed with creation order |
| `source_task_run_id` | `bigint unsigned` | no | Run which captured it |
| `base_checkpoint_id` | `bigint unsigned` | yes | Lineage predecessor |
| `kind` | `varchar(32)` | no | `seed`, `successful`, or `partial` |
| `payload_format` | `varchar(32)` | no | Initially `tar.zst.v1` |
| `payload_sha256` | `char(64) ascii_bin` | no | Digest of stored bytes |
| `storage_key` | `varchar(1024)` | no | Private backend-relative key, never serialized |
| `size_bytes` | `bigint` | no | Stored payload bytes |
| `uncompressed_bytes` | `bigint` | no | Sum of regular-file sizes |
| `entry_count` | `bigint` | no | Regular files, directories, and symlinks |
| `created_at` | `datetime(6)` | no | UTC commit time |

Uniqueness is `(source_task_run_id, kind)`. A TaskRun has at most one seed, one
successful result, and one partial checkpoint. `payload_sha256` is not globally
unique because separate checkpoint rows can intentionally name the same
immutable payload and retain separate provenance.

`storage_key` follows the same rule as Artifact storage keys: it is
infrastructure data and is absent from domain JSON, worker-visible TaskRun
responses, logs, and traces.

### 9.2 `task`

Add nullable `workspace_head_checkpoint_id`. It points at the latest checkpoint
accepted as this Task's recoverable state: its initial seed or a later
successful result. It is a projection maintained in the same transaction as
checkpoint finalization and, for a result, TaskRun terminal transition.

The pointer is nullable for a Task whose first run has not committed its seed
or successful result. A seed can become the head before the first execution so
that a hard loss still leaves a deterministic retry base.

Add nullable `plugin_environment_head_id`. It points at the immutable Plugin
environment revision used by the next Continue. A Task with no autonomous
installation may continue to derive its environment from the Agent revision
and Space activation. Promotion beyond Task scope goes through the owning Agent
or Space service rather than mutating this Task pointer.

### 9.3 `task_run`

Add:

| Column | Meaning |
|---|---|
| `workspace_base_checkpoint_id` | Immutable base selected before execution |
| `workspace_result_checkpoint_id` | Successful result checkpoint, when committed |
| `workspace_partial_checkpoint_id` | Partial checkpoint, when committed |
| `workspace_restore_status` | `not_requested`, `pending`, `restored`, or `failed` |
| `workspace_restore_error` | Bounded operator-facing reason |
| `workspace_checkpoint_status` | `not_requested`, `pending`, `committed`, or `failed` |
| `workspace_checkpoint_error` | Bounded operator-facing reason |
| `plugin_environment_base_id` | Immutable Plugin set materialized for execution |
| `plugin_environment_result_id` | New Plugin set requested by this run, when committed |
| `plugin_environment_status` | `unchanged`, `pending`, `committed`, or `failed` |
| `plugin_environment_error` | Bounded installation or materialization reason |

The status fields are stored because a missing pointer alone cannot distinguish
“not requested” from “attempted and failed”. Errors are bounded text intended
for diagnosis, not raw provider responses containing endpoints or credentials.

Session restore receives equivalent recorded outcome in the same ownership
change. A TaskRun detail must be able to answer whether workspace, session, and
Plugin environment were all supplied as promised without reconstructing the
answer from logs.

### 9.4 State Rules

`internal/core/task` owns these rules:

- a workspace base becomes immutable before execution;
- Continue selects the Task head fixed by its predecessor;
- Retry selects the named run's base;
- a seed establishes the first Task head, and only a successful result advances
  an existing head automatically;
- a Plugin environment base is immutable within one TaskRun;
- Continue uses the Task Plugin environment head while Retry reuses the named
  run's base environment;
- partial checkpoints never become the workspace head implicitly; and
- a checkpoint can only belong to the Space and Task of its source TaskRun.

The database store applies those transitions atomically but does not redefine
them. Handlers, workers, and schedulers delegate to the same owner.

## 10. Package And Interface Boundaries

The capability lands without reversing the repository dependency direction:

| Area | Responsibility |
|---|---|
| `internal/core/task` | Checkpoint kinds, restore/checkpoint outcome values, and Task/TaskRun transition rules |
| `internal/service/task` | Select Continue and Retry bases; expose recovery choices |
| `internal/service/workspace` | Validate and finalize checkpoint metadata; retention reference queries |
| `internal/service/plugin` | Validate autonomous acquisition, create immutable Plugin environment revisions, and authorize promotion scope |
| `internal/agentapp/taskrun` | Materialize, capture, quiesce, and invoke the worker client; no authorization decisions |
| `internal/infra/objectstore` | Stream archive payloads, derive private keys, verify digests, and implement local/S3 stores |
| `internal/infra/db` | Row shapes and atomic checkpoint/TaskRun/Task updates |
| `internal/infra/workerclient` | Run-token-authenticated checkpoint prepare/finalize DTOs |
| `internal/server/handlers/worker` | Authenticate and delegate worker checkpoint calls |
| `internal/infra/k8s` | Scratch-volume and ephemeral-storage resource bounds |

`internal/service/workspace` is justified as a boundary only if it owns the
cross-storage commit and retention capability described here. Archive encoding
alone remains infrastructure and must not create a generic “workspace manager”
package.

Suggested consumer-owned interfaces:

```go
type CheckpointPayloadStore interface {
    Put(ctx context.Context, spaceID, sha256 string, src io.Reader) (storageKey string, err error)
    Open(ctx context.Context, storageKey string) (io.ReadCloser, int64, error)
    Exists(ctx context.Context, spaceID, sha256 string) (bool, error)
    Delete(ctx context.Context, storageKey string) error
}

type CheckpointFinalizer interface {
    RecordBase(ctx context.Context, in RecordBaseInput) (*task.WorkspaceCheckpoint, error)
    FinalizeResult(ctx context.Context, in FinalizeResultInput) (*task.WorkspaceCheckpoint, error)
}
```

Exact interfaces belong near their consumers and may differ from this sketch.
There is no requirement to merge them with existing Home, Run, Artifact,
Plugin-package, or session storage merely because every implementation
eventually calls S3.

## 11. Kubernetes Runtime And Storage Profiles

### 11.1 Default: Object Checkpoint + `emptyDir`

The default worker Job keeps `emptyDir` mounted for `workspace/`,
`buildmax-home/`, and hidden scratch. It restores the workspace checkpoint and
materializes the Plugin environment before execution, then publishes the next
workspace checkpoint through the application protocol.

This profile preserves free Pod placement, works across nodes and clusters,
uses the object store the production topology already requires, and has no
per-Task PVC lifecycle. Its cost is one archive download per run and, when the
workspace changes, one archive upload at the boundary.

Both the `k8s_job` and `local_process` runners use the same checkpoint
semantics. A local filesystem implementation stores the same archive and
metadata contract without pretending that the worker directory itself is the
durable record.

### 11.2 Optional Later Profile: Persistent Cache

Large-workspace evidence may justify a `persistent_cache` profile. It may mount
a per-Task CSI volume and use it as a materialization cache or active working
disk. The object-store checkpoint remains authoritative and is still committed
at TaskRun boundaries.

When strict single-Pod mounting matters, the access mode is
`ReadWriteOncePod`, not `ReadWriteOnce`: RWO constrains mounting to one node and
can permit several Pods on that node. RWOP requires CSI support and is stable in
modern Kubernetes.

The cache profile must validate that the mounted state matches the Task's
checkpoint digest before use. A stale, missing, or foreign cache is discarded
and rebuilt; it never moves the database head.

### 11.3 Profiles Not Offered Initially

- RWX is unnecessary for the linear Task contract, which permits at most one
  active TaskRun. It adds a shared-filesystem dependency without a concurrent
  writer use case.
- Local PV and `hostPath` are caches at most. Node loss cannot be a supported
  loss of authoritative Task state.
- JuiceFS, CephFS, NFS, and cloud NAS may implement an operator's persistent
  cache or object-store layer, but BuildMax does not encode those products into
  its domain model.
- Object-store FUSE is not the default workspace filesystem. A local POSIX tree
  plus explicit checkpoints gives clearer write, rename, and failure
  semantics.

### 11.4 Sidecars And Lifecycle Hooks

A sidecar may provide compression or upload acceleration later, but it cannot
own correctness. `PreStop` runs only for managed termination while the
container is still alive, shares the Pod termination grace period, and cannot
run after hard process or node loss.

The TaskRun process owns quiescence and checkpoint finalization. SIGTERM
reporting is an optimization over recovery from the last committed checkpoint,
not an alternative source of truth.

## 12. Security, Limits, And Retention

### 12.1 Filesystem Safety

Capture and restore never follow a path outside `workspace/`. The extractor creates
files with controlled APIs rather than invoking the system `tar` binary. It
refuses unsupported entry types and strips privilege-bearing mode bits.

The archive contains `workspace/` only. `buildmax-home/`, the expanded Plugin
tree, OS home, archive staging, credentials, logs, traces, caches, and other
runtime scratch remain outside it. There is no Artifact or output directory to
exclude.

Space file upload is not an implicit runtime-configuration grant. Control-
bearing paths such as `AGENTS.md` and `.buildmax/` are either reserved from the
ordinary file-upload surface or writable only through an explicit Space
configuration permission and audit event. A shared file contributor must not
gain prompt, hook, MCP, or Plugin authority merely by choosing a filename.

Environment Secret grants can still be copied deliberately into `workspace/` by a
model-chosen command. The Space already authorizes the Agent to read those
values, so checkpointing does not create a new confidentiality boundary, but
it lengthens retention. Before shipping, the threat model must decide whether
exact granted-secret values found in regular workspace files cause a warning,
checkpoint refusal, or redacted diagnostic. Silent deletion would corrupt the
workspace and is not allowed.

### 12.2 Object-Store Authority

The Worker receives only the run token at the HTTP boundary, but the current
production storage path may also give the Pod workload identity access to the
bucket. Checkpoint keys stay inside the run's Space namespace and the Server
derives or validates every storage reference before committing it.

The long-term least-privilege improvement is a run-scoped upload/download lease
or pre-signed request, not exposing arbitrary bucket keys through the worker
API. This record does not claim that improvement has shipped.

### 12.3 Resource Limits

The worker Job must gain both:

- container `ephemeral-storage` request and limit; and
- `emptyDir.sizeLimit` for the workspace, runtime home, and `/tmp` volumes.

The Pod's storage admission limit accounts for the materialized workspace,
expanded Plugin packages, the temporary compressed payload, logs, and ordinary
tool scratch space. The checkpoint's own limits account for `workspace/` only.
Checkpoint capture stops before exhausting the Pod's storage limit and records
a bounded failure rather than waiting for kubelet eviction.

Checkpoint limits include at least:

- stored bytes;
- uncompressed regular-file bytes;
- entry count;
- maximum path length;
- maximum individual file size; and
- decompression expansion ratio.

Defaults are not chosen in this record. They must come from benchmark evidence
over representative source, build, and data workspaces and be documented in
the configuration reference when implemented.

### 12.4 Retention

Successful checkpoints follow Task retention. Seed checkpoints remain while a
TaskRun or a later checkpoint references them. Partial checkpoints may have a
shorter operator-configured retention, but are never removed while a visible
recovery action names them.

Content-addressed blobs are deleted only when no checkpoint row references the
storage key and the blob is older than the orphan grace period. Database and
bucket backup/restore remain a pair: restoring one without the other can leave
metadata with no payload or payload with no metadata.

Checkpoint bytes count against a distinct workspace-state quota. They do not
count as Artifact bytes and do not appear in Artifact quota or listings.

## 13. Failure And Recovery Matrix

| Failure point | Durable observation | Automatic behavior | Recovery base |
|---|---|---|---|
| Before worker claim | Run stays `SCHEDULED` until existing dispatch handling resolves it | Kubernetes may retry startup before claim | None required |
| Space file snapshot materialization fails | Restore status `failed`; no model/tool call | Run fails closed | No Task workspace yet |
| Seed upload or commit fails | Checkpoint status `failed`; no model/tool call | Run fails closed | None, because execution never started |
| Continue workspace restore fails | Restore status and bounded error | Run fails closed | Explicit repair or recovery choice |
| Continue session restore fails | Session restore status and bounded error | Run fails closed | Explicit repair or recovery choice |
| Plugin environment materialization fails | Plugin environment status and bounded error | Run fails closed | Recorded base environment |
| Autonomous Plugin installation fails | Installation status and bounded error | Current run keeps its immutable base; no capability switch | Recorded base environment |
| Agent/tool execution fails | Normal FAILED outcome; partial capture attempted | No automatic retry | Recorded base |
| User cancels | CANCELED outcome; partial capture attempted | No automatic retry | Recorded base; partial only by explicit future Resume |
| SIGTERM or eviction | FAILED/interrupted outcome; bounded partial capture attempted | Process exits after reporting | Recorded base, plus partial if committed |
| SIGKILL/OOM/node loss | Liveness reaper records lost worker | No automatic re-dispatch | Recorded base or prior Task head |
| Result payload upload fails | Run outcome still reported; checkpoint status `failed` | Task head does not advance | Previous Task head |
| Upload succeeds, DB commit fails | Old head remains; payload is an orphan | Idempotent retry may commit it | Previous Task head until commit |
| DB commits, response is lost | Checkpoint and head are durable | Idempotent retry returns existing row | Newly committed checkpoint |
| Object later disappears/corrupts | Restore verification fails visibly | No fallback | Operator restores bucket or chooses another base |
| Database restored without bucket | Missing payload is visible | No fallback | Paired backup restoration |

The first implementation's recovery objectives are deliberately bounded:

| Event | RPO | RTO driver |
|---|---|---|
| Successful terminal run | Final committed checkpoint | Archive download and extraction |
| Graceful cancel/interruption | Partial checkpoint if it fits the reporting budget; otherwise recorded base | Object-store availability and archive size |
| Hard worker or node loss | Last committed base/head; all current uncommitted filesystem work may be lost | Liveness grace plus new-run scheduling and restore |

No numeric RTO is promised until the implementation is benchmarked against the
supported deployment profiles. The Portal reports measured capture and restore
duration so an operator can set an evidence-based objective.

## 14. API, Portal, And Observability

### 14.1 Worker API

The exact routes remain registration-time implementation details, but the
worker needs three typed operations:

1. obtain the immutable base descriptor selected for the run;
2. record base restoration success or failure; and
3. record a seed checkpoint, or carry a successful/partial checkpoint descriptor
   on the terminal TaskRun report.

All require the run token. Base retrieval is allowed for `RUNNING` while the
run is preparing. Finalization is allowed only for the same run and one legal
checkpoint kind. The Server derives Space and Task from the token's TaskRun; the
worker never supplies authority-bearing owner IDs.

Large payload bytes continue to flow directly to object storage. The Server
receives and validates metadata rather than proxying a multi-gigabyte archive
through its HTTP process.

### 14.2 Task And TaskRun API

Task detail exposes the public identity and summary of its current workspace
head, not a storage key. TaskRun detail exposes:

- base, successful result, and partial checkpoint identities when present;
- session, workspace, and Plugin environment restore outcomes;
- checkpoint publication outcome;
- stored and uncompressed size plus entry count;
- capture and restore durations; and
- a bounded error when either operation failed.

The first implementation does not add a general checkpoint listing or download
route. Those would turn internal continuity state into a user-facing version
history and require a separate product decision.

### 14.3 Portal

The Task page shows one compact workspace state beside each run:

- **Restored** — base identity and restore duration;
- **Saved** — result checkpoint and size;
- **Partial state saved** — failure/cancel only, not used automatically;
- **Workspace not saved** — visible warning with the last durable head; or
- **Restore failed** — run did not execute.

Continue is disabled when the required session, workspace, or Plugin
environment cannot be restored.
The error offers an explicit recovery action only after that action is designed
and authorized; it never silently substitutes current Space files or a
different Plugin environment.

### 14.4 Trace And Metrics

The trace records bounded metadata events, never archive content or private
storage keys:

- `workspace.restore.started/completed/failed`;
- `workspace.checkpoint.started/completed/failed`;
- `plugin.environment.materialize.started/completed/failed` and
  `plugin.install.requested/committed/failed`;
- checkpoint kind, public ID after commit, sizes, entry count, digest prefix,
  duration, and source run; and
- whether capture was skipped because the reporting budget expired.

Metrics include capture/restore latency, compressed and uncompressed bytes,
compression ratio, failures by bounded category, orphan count, and cache hit
when a later persistent cache profile exists.

## 15. Delivery Plan

### Phase 0 — Accept The Contract

- Amend product vision so generic versioned workspace remains absent while
  Task workspace continuity is explicitly planned.
- Amend Agent execution continuity so Continue binds session and workspace to
  one predecessor.
- Add this record to the design index and give it a roadmap priority before
  implementation begins.
- Amend Plugin distribution so an expanded run directory is a projection of an
  immutable environment revision, not its durable state.

### Phase 1 — Payload And Metadata Foundations

- Add archive creation/extraction with adversarial path and expansion tests.
- Add the checkpoint payload store for local filesystem and S3-compatible
  storage.
- Add `workspace_checkpoint` and Task/TaskRun fields everywhere at once:
  domain, rows, store, wire DTOs, data-model documentation, and tests.
- Add idempotent metadata finalization without changing runtime behavior.

### Phase 2 — Deterministic Bases

- Capture the first run's seed before Agent execution.
- Fix Continue and Retry base selection in `internal/service/task`.
- Restore workspace and session and materialize the Plugin environment as one
  fail-closed preparation gate.
- Record all three outcomes and expose them on TaskRun detail.

### Phase 3 — Result And Partial Checkpoints

- Quiesce the runtime and capture successful result checkpoints.
- Advance the Task head atomically with accepted terminal reporting.
- Attempt partial checkpoint capture for failure, cancellation, and graceful
  interruption without changing their terminal statuses.
- Surface checkpoint failure without hiding result or Artifact success.
- Add typed Plugin search, inspection, and Task-scoped installation; a committed
  installation creates a new TaskRun boundary before the new capability loads.

### Phase 4 — Kubernetes And Product Evidence

- Add ephemeral-storage requests/limits and `emptyDir.sizeLimit`.
- Add Portal workspace state and Continue gating.
- Add orphan and retention sweeps.
- Exercise normal continuation, cancellation, SIGTERM, SIGKILL, object-store
  denial, database ambiguity, and paired restore through the deployment suite.

### Phase 5 — Evidence-Gated Acceleration

Only benchmark evidence may start this phase. Candidates include archive
parallelism, changed-file indexes, chunked content-addressed payloads, and a
per-Task RWOP persistent cache. The domain remains checkpoint-based whichever
physical optimization wins.

## 16. Verification

### 16.1 Unit And Property Tests

- archive round-trip preserves every supported entry property;
- archive creation and extraction reject traversal, absolute paths, unsafe
  links, special files, duplicate paths, over-limit inputs, and decompression
  bombs;
- digest mismatch never materializes a visible workspace;
- an interrupted extraction leaves the target absent, not partial;
- Continue and Retry choose the specified bases;
- Continue and Retry choose the specified Plugin environments;
- writing an expanded Plugin directory cannot create durable installation
  state, and package bytes are accepted only after digest verification;
- partial checkpoints never advance Task head; and
- finalization is idempotent and conflicts on changed metadata.

### 16.2 MySQL Integration

`./make test mysql` must prove:

- checkpoint insert, TaskRun terminal transition, and Task head update commit
  together;
- two finalizers racing for one run produce one checkpoint and one head;
- a late report cannot replace the head of a later run;
- cross-Space and cross-Task checkpoint references are refused;
- Continue admission cannot select a missing or foreign head; and
- retention queries do not delete referenced payloads.

### 16.3 Object-Store Integration

Against the committed MinIO fixture and the S3 adapter fake:

- streamed upload/download handles archives larger than memory budgets;
- exact-key verification works without LIST;
- repeated content-addressed upload is harmless;
- upload success followed by DB failure produces a reclaimable orphan;
- corruption and deletion are detected on restore; and
- local filesystem and S3 implementations obey the same contract.

### 16.4 Runtime And Deployment Evidence

The kind suite must demonstrate:

1. run 1 edits `workspace/`; run 2 Continue sees the exact bytes;
2. Retry starts from the failed run's base, not its partial state;
3. graceful cancellation and SIGTERM preserve partial state when the reporting
   budget permits;
4. force-deleting the worker Pod loses only uncommitted state and explicit
   Retry succeeds from the recorded base;
5. denying object-store writes does not silently advance the Task head;
6. denying object-store reads prevents execution rather than substituting Space
   files;
7. a Task-scoped autonomous Plugin installation continues in a new run with
   the pinned package, while Retry reconstructs the original Plugin base;
8. a replacement Pod cannot execute an already claimed run; and
9. `ephemeral-storage` limits fail predictably and leave a diagnosable run.

Before handoff, run the relevant `./make test`, `./make test mysql`,
`./make check go`, `./make check docs`, `./make lint`, and `git diff --check`
scopes. The deployment evidence is not replaced by unit tests.

## 17. Alternatives Rejected

### 17.1 One PVC Per Task As The Default

A PVC preserves filesystem state after Pod deletion, but makes every Task own a
Kubernetes storage object, its capacity decision, garbage collection, backup,
topology, attach latency, and CSI availability. It also does not provide
cross-cluster recovery by itself. This is too deployment-specific for the
portable default and remains a possible cache profile.

### 17.2 One RWX Space Workspace

Different Tasks would become concurrent writers to shared mutable state. The
linear single-writer invariant is per Task, not per Space, and RWX does not
provide application-level conflict or provenance semantics. This option solves
mounting while making ownership less correct.

### 17.3 Node Affinity With Local PV Or `hostPath`

This optimizes the normal path by making node loss unrecoverable. Scheduling,
maintenance, and capacity pressure become correctness dependencies. Local disk
may cache a verified checkpoint but cannot be its authority.

### 17.4 A Distributed Filesystem As A Required Platform Component

JuiceFS, CephFS, NFS, and cloud NAS can be useful at scale, but requiring one
would add an operational control plane and narrow private-deployment
portability before BuildMax has workload evidence demanding it.

### 17.5 InitContainer Download And Sidecar Upload As The Protocol

An InitContainer can materialize bytes and a sidecar can transfer them, but
neither owns TaskRun authorization, quiescence, lineage, terminal status, or
the MySQL pointer. Lifecycle hooks cannot run after hard loss. They may
implement part of the data path later; they cannot define the commit protocol.

### 17.6 Git As The Workspace Authority

Not every Task workspace is a Git repository, and Git does not naturally own
large binaries, generated trees, arbitrary uploads, or the exact non-Git state
an Agent observed. Making Git hidden would not remove its repository and merge
semantics; it would only hide them. Git remains workspace content when present,
not BuildMax's durability engine.

### 17.7 Upload Every File Individually

The current Space-file (Space Home) adapter lists and downloads objects one file at a time.
Repeating that design for Task checkpoints makes request count proportional to
file count and performs badly for source trees. A single versioned archive is a
smaller first correctness surface. Incremental manifests remain evidence-gated.

### 17.8 Automatic Resume Of A Lost TaskRun

Filesystem state cannot say whether a tool's external side effect happened.
Automatically restarting after worker loss can duplicate an email, API call,
commit, issue comment, or deployment. Recovery supplies an explicit base for a
new attempt; it does not erase the need for Retry or future Resume semantics.

### 17.9 Checkpoint The Expanded Plugin Directory

Uploading `buildmax-home/plugins/` would preserve an Agent's acquired package
bytes, but it would also preserve extraction artifacts, caches, undeclared
mutable state, possible credentials, and bytes which no longer prove their
catalog digest. It duplicates the same immutable package for every run and
bypasses installation policy on restore. BuildMax instead stores a normalized
private package once when necessary, records immutable pins in a Plugin
environment revision, and re-materializes the directory for each TaskRun.

## 18. Deferred Questions

These questions do not block the first complete-archive implementation:

1. Which measured workspace sizes and file counts justify a chunked manifest
   format or persistent cache profile?
2. Should a user ever be allowed to Resume from a partial checkpoint, and what
   warning and authorization does that require?
3. Should exact Space Secret values found in `workspace/` warn or fail checkpoint
   publication?
4. Does the Space file service need its own atomic revision so the first Task
   seed represents one upload transaction rather than the exact materialized
   manifest and copy the worker saw?
5. Which Task deletion or retention operation first makes checkpoint payload
   reclamation user-visible?
6. Should Workflow steps pass a predecessor Task workspace explicitly, or do
   their Tasks remain independently seeded from Space files?
7. Which evidence would justify checkpointing at safe tool boundaries rather
   than only at TaskRun boundaries?

The following are not deferred implementation work under this record: generic
workspace history, timeline restore, merging, cross-device reconstruction, and
automatic Space-file write-back. Taking any of them up requires a separate
accepted product design and roadmap decision.

External semantics referenced by this decision:

- [Kubernetes volumes and `emptyDir`](https://kubernetes.io/docs/concepts/storage/volumes/)
- [Kubernetes Job failure handling](https://kubernetes.io/docs/concepts/workloads/controllers/job/)
- [Kubernetes PersistentVolume access modes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)
- [Kubernetes container lifecycle hooks](https://kubernetes.io/docs/concepts/containers/container-lifecycle-hooks/)
- [Kubernetes local ephemeral storage](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/)
- [Amazon S3 consistency model](https://docs.aws.amazon.com/AmazonS3/latest/userguide/Welcome.html)
