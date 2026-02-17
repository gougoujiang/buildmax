# Design 053: Add artifact concept

## Goal

Introduce a first-class artifact concept and **reorganize workspace paths** so all three dir types (persist, tasks, artifacts) live under one workspace root per workspace. New DB tables (`artifact`, `artifact_item`), per-task `artifact_seq` column, and `last_artifact_id` for easy lookup of the latest artifact. On each successful task run, the executor increments the task’s seq, generates an artifact_id, writes the result file under the artifact dir, and inserts one artifact row and one artifact_item row. Artifact creation is best-effort (log on failure, do not fail the task). No new HTTP/CLI API in this task.

## Workspace path layout (reorg)

```
<data dir>/workspaces/<workspace id>/
  - persist/                              # 用户上传/长期资料 (user uploads, long-term)
  - tasks/<task id>/                       # runtime/scratch (ephemeral, can be cleaned)
  - artifacts/<task id>/<artifact id>/      # 每次会话继续后的快照产物 (snapshot output per run)
```

- **WorkspacesDir()**: returns the **parent** of all workspace roots: `DataDir()/workspaces` (or `BUILDMAX_WORKSPACES_DIR` if set). Workspace root = `filepath.Join(WorkspacesDir(), workspaceID)`.
- **PersistentWorkspaceDir(workspaceID)** = workspace root + `persist/`.
- **RuntimeWorkspaceDir(workspaceID, taskID)** = workspace root + `tasks/<task_id>/`.
- **ArtifactDir(workspaceID, taskID, artifactID)** = workspace root + `artifacts/<task_id>/<artifact_id>/`.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/config** | Workspace path API | WorkspacesDir (reorg); PersistentWorkspaceDir; RuntimeWorkspaceDir(workspaceID, taskID); ArtifactDir(workspaceID, taskID, artifactID) |
| **internal/store** | Task + artifact persistence | Task.ArtifactSeq, Task.LastArtifactID; IncrementTaskSeq; Artifact, ArtifactItem models; CreateArtifactWithItem (also sets task.last_artifact_id); AutoMigrate new tables |
| **internal/executor** | Task run + artifact capture | After success: increment seq, write to artifact dir, create artifact + item (best-effort); extended TaskStore interface; use new config paths |
| **internal/server** | HTTP API for workspaces, upload, files | Use PersistentWorkspaceDir (unchanged usage); ensureWorkspaceDirs uses new layout |

## Structure

**internal/config**

- **WorkspacesDir()**: change to return the parent of workspace roots: `DataDir()/workspaces` (or `BUILDMAX_WORKSPACES_DIR` if set). So existing override still applies but now points to the parent dir; each workspace is a subdir of it.
- **PersistentWorkspaceDir(workspaceID)**: return `filepath.Join(WorkspacesDir(), workspaceID, "persist")` (was `filepath.Join(WorkspacesDir(), workspaceID)` when WorkspacesDir was persist root).
- **RuntimeWorkspaceDir(workspaceID, taskID)**: change signature to take both IDs; return `filepath.Join(WorkspacesDir(), workspaceID, "tasks", taskID)`. All callers (executor) must pass workspaceID and taskID.
- Add **ArtifactDir(workspaceID, taskID, artifactID string) string**: return `filepath.Join(WorkspacesDir(), workspaceID, "artifacts", taskID, artifactID)`. Used by executor to create one dir per artifact.

**internal/store**

- **Task** struct: add field `ArtifactSeq int` with tag `gorm:"column:artifact_seq" json:"artifact_seq"` (default 0 in DB). Add field `LastArtifactID *string` with tag `gorm:"type:varchar(64)" json:"last_artifact_id,omitempty"` (nullable; set after each artifact creation for easy lookup).
- **TaskStore** interface: add `IncrementTaskSeq(ctx context.Context, taskID string) (newSeq int, err error)`. Atomically increments task.artifact_seq and returns the new value.
- **Artifact** struct (new): table name `artifact` (singular). Fields: `ID` (uint, PK, autoIncrement, json:"-"), `TaskID` (string, not null, index), `ArtifactID` (string, unique, not null), `CreatedAt` (int64), `Seq` (int, not null). JSON tags snake_case: `task_id`, `artifact_id`, `created_at`, `seq`.
- **ArtifactItem** struct (new): table name `artifact_item` (singular). Fields: `ID` (uint, PK, autoIncrement, json:"-"), `ArtifactID` (string, not null, index), `RelativePath` (string, not null, varchar 512). JSON tags: `artifact_id`, `relative_path`.
- **ArtifactStore** interface (new): single method `CreateArtifactWithItem(ctx context.Context, taskID, artifactID string, seq int, relativePath string) error`. In a transaction: insert Artifact, insert ArtifactItem, update task SET last_artifact_id = artifactID WHERE task_id = taskID.
- **Store**: implement IncrementTaskSeq (transaction: SELECT artifact_seq FOR UPDATE, UPDATE task SET artifact_seq = artifact_seq + 1, return new value). Implement CreateArtifactWithItem in a transaction (insert Artifact, insert ArtifactItem, update task.last_artifact_id). Add `&Artifact{}, &ArtifactItem{}` to AutoMigrate in New(). Existing Task rows get artifact_seq 0, last_artifact_id null.
- **store.New**: AutoMigrate must include Task (with new ArtifactSeq and LastArtifactID columns), Artifact, ArtifactItem.

**internal/executor**

- **TaskStore** interface: extend with `IncrementTaskSeq(ctx context.Context, taskID string) (int, error)` and `CreateArtifactWithItem(ctx context.Context, taskID, artifactID string, seq int, relativePath string) error`. So the executor depends on a store that implements task polling, status update, seq increment, and artifact creation.
- **Runner**: holds this extended store (same field `store`). No new fields.
- **New**: signature remains `New(store TaskStore) *Runner`; the passed store must implement the two new methods (e.g. *store.Store).
- **executeTask**:
  - **Path resolution**: `persistDir := config.PersistentWorkspaceDir(task.WorkspaceID)`; `runtimeDir := config.RuntimeWorkspaceDir(task.WorkspaceID, task.TaskID)`. (RuntimeWorkspaceDir now takes workspaceID and taskID.)
  - After writing result to runtime dir and copying result to persist dir (unchanged), and only when the run is successful (before UpdateTaskStatus(SUCCEEDED)):
  1. Call `newSeq, err := r.store.IncrementTaskSeq(ctx, task.TaskID)`. On error: slog.Warn and skip artifact creation.
  2. Generate `artifactID := id.New()`.
  3. Build artifact dir: `artifactDir := config.ArtifactDir(task.WorkspaceID, task.TaskID, artifactID)`. `os.MkdirAll(artifactDir, 0755)`; on error: slog.Warn and skip artifact creation.
  4. Copy the result file from `filepath.Join(runtimeDir, resultFilename)` to `filepath.Join(artifactDir, "result.md")` (single file per artifact; stable name). On error: slog.Warn and skip artifact creation (and optionally skip DB insert).
  5. Call `r.store.CreateArtifactWithItem(ctx, task.TaskID, artifactID, newSeq, resultFilename)`. On error: slog.Warn. Do not fail the task.
  - Order: increment seq first, then create dir and copy file, then insert DB. If DB insert fails, the file may already be on disk (acceptable; future task could add cleanup or reconciliation).
  - Use existing `resultFilename := fmt.Sprintf("result-%s.md", task.TaskID)` for relativePath in CreateArtifactWithItem.

## Method design

| Package / layer | Component | Method / function | Signature / contract |
|-----------------|-----------|-------------------|----------------------|
| **config** | (package) | WorkspacesDir | `WorkspacesDir() string`. Returns `DataDir()/workspaces` or `BUILDMAX_WORKSPACES_DIR` if set (parent of workspace roots). |
| **config** | (package) | PersistentWorkspaceDir | `PersistentWorkspaceDir(workspaceID string) string`. Returns `filepath.Join(WorkspacesDir(), workspaceID, "persist")`. |
| **config** | (package) | RuntimeWorkspaceDir | `RuntimeWorkspaceDir(workspaceID, taskID string) string`. Returns `filepath.Join(WorkspacesDir(), workspaceID, "tasks", taskID)`. |
| **config** | (package) | ArtifactDir | `ArtifactDir(workspaceID, taskID, artifactID string) string`. Returns `filepath.Join(WorkspacesDir(), workspaceID, "artifacts", taskID, artifactID)`. |
| **store** | TaskStore | IncrementTaskSeq | `IncrementTaskSeq(ctx context.Context, taskID string) (newSeq int, err error)`. Atomically increments task.artifact_seq; returns the new seq. Returns error if task not found or update fails. |
| **store** | ArtifactStore | CreateArtifactWithItem | `CreateArtifactWithItem(ctx context.Context, taskID, artifactID string, seq int, relativePath string) error`. In a transaction: insert Artifact, insert ArtifactItem, update task SET last_artifact_id = artifactID WHERE task_id = taskID. |
| **store** | Store | New | Add Artifact, ArtifactItem to AutoMigrate. Task model now has ArtifactSeq (int), LastArtifactID (*string, nullable). |
| **store** | Artifact | (model) | TableName() "artifact". Fields: ID, TaskID, ArtifactID, CreatedAt, Seq. |
| **store** | ArtifactItem | (model) | TableName() "artifact_item". Fields: ID, ArtifactID, RelativePath. |
| **executor** | TaskStore (interface) | — | Extended with IncrementTaskSeq and CreateArtifactWithItem. |
| **executor** | Runner | executeTask | persistDir = PersistentWorkspaceDir(task.WorkspaceID); runtimeDir = RuntimeWorkspaceDir(task.WorkspaceID, task.TaskID). After success: increment artifact_seq (IncrementTaskSeq), artifactDir = ArtifactDir(workspaceID, taskID, artifactID), copy result to artifactDir/result.md, CreateArtifactWithItem (sets last_artifact_id). All best-effort; log on failure. |

## How they work together

**Successful task run (executor)**

1. persistDir = config.PersistentWorkspaceDir(task.WorkspaceID); runtimeDir = config.RuntimeWorkspaceDir(task.WorkspaceID, task.TaskID). Create runtime dir; copy persist → runtime; run buildmax; write result to runtimeDir/result-<task_id>.md; copy that file to persistDir (unchanged).
2. **New:** newSeq, err := store.IncrementTaskSeq(ctx, task.TaskID). If err != nil: log, skip artifact steps.
3. artifactID := id.New(). artifactDir := config.ArtifactDir(task.WorkspaceID, task.TaskID, artifactID). MkdirAll(artifactDir). Copy runtimeDir/result-<task_id>.md → artifactDir/result.md. If any error: log, skip DB insert.
4. store.CreateArtifactWithItem(ctx, task.TaskID, artifactID, newSeq, "result-<task_id>.md"). If err: log.
5. UpdateTaskStatus(SUCCEEDED, ...) as today.

**Failed task run**

- No IncrementTaskSeq, no artifact dir creation, no CreateArtifactWithItem. Task status set to FAILED; task.artifact_seq and task.last_artifact_id unchanged.

**Startup (cmd)**

- executor.New(st) unchanged; st is *store.Store which now implements the extended executor.TaskStore (GetNextPendingTask, UpdateTaskStatus, IncrementTaskSeq, CreateArtifactWithItem).

## IncrementTaskSeq implementation detail

MySQL does not support UPDATE ... RETURNING. Options:

- **Option A (recommended):** Use a transaction: BEGIN; SELECT artifact_seq FROM task WHERE task_id = ? FOR UPDATE; compute newSeq = artifact_seq + 1; UPDATE task SET artifact_seq = newSeq WHERE task_id = ?; COMMIT; return newSeq. This avoids lost updates under concurrent runs (same task should not run concurrently; still safe).
- **Option B:** Single UPDATE: `UPDATE task SET artifact_seq = artifact_seq + 1 WHERE task_id = ?`; then SELECT artifact_seq FROM task WHERE task_id = ? and return it. Two round-trips; small race window if two callers for same task (not expected).

Design chooses Option A for clarity and correctness.

## Errors and edge cases

- **IncrementTaskSeq fails** (e.g. task deleted): log warning; do not create artifact; still mark task SUCCEEDED (result was written to persist).
- **MkdirAll(artifactDir) fails**: log warning; skip copy and DB insert; still mark task SUCCEEDED.
- **Copy to artifact dir fails**: log warning; skip DB insert (do not insert artifact row if file is missing); still mark task SUCCEEDED.
- **CreateArtifactWithItem fails**: log warning; task already SUCCEEDED; artifact file may exist on disk without DB row (acceptable for this task).
- **Existing tasks**: After migration, task.artifact_seq is 0 and last_artifact_id is null. First successful run after deploy will set artifact_seq to 1, last_artifact_id to the new artifact_id, and create one artifact row.

## Tests

- **config**: Test WorkspacesDir() returns path ending with `workspaces` under DataDir() (or env override). Test PersistentWorkspaceDir(workspaceID) ends with `<workspaceID>/persist`; RuntimeWorkspaceDir(workspaceID, taskID) ends with `<workspaceID>/tasks/<taskID>`; ArtifactDir(workspaceID, taskID, artifactID) ends with `<workspaceID>/artifacts/<taskID>/<artifactID>`.
- **store**: Test IncrementTaskSeq: create task, call IncrementTaskSeq twice, assert artifact_seq 1 then 2; test CreateArtifactWithItem inserts artifact and artifact_item and updates task.last_artifact_id. Test that CreateArtifactWithItem in a transaction rolls back artifact, artifact_item, and task.last_artifact_id on error if we inject a failure.
- **executor**: Test successful run produces artifact dir and artifact + artifact_item rows (mock or integration with real store); test that when IncrementTaskSeq or CreateArtifactWithItem fails, task is still SUCCEEDED and no panic. Test failed run does not call IncrementTaskSeq or CreateArtifactWithItem.

## Backward compatibility

- New columns task.artifact_seq (default 0) and task.last_artifact_id (nullable): existing rows get artifact_seq 0, last_artifact_id null (GORM AutoMigrate).
- **Path reorg is breaking**: Current layout is `DataDir()/workspace/persist/<workspace_id>` and `DataDir()/workspace/task/<task_id>`. New layout is `DataDir()/workspaces/<workspace_id>/persist`, `.../tasks/<task_id>/`, `.../artifacts/<task_id>/<artifact_id>/`. Deployments with existing data under `workspace/persist` or `workspace/task` will not see it in the new layout; migration (copy/move) is out of scope for this task; document in README or release notes.
- No change to server API or executor.New signature (executor still receives one store; that store must implement the extended interface).

---

## Changes for review

| Area | Change |
|------|--------|
| **internal/config** | **Path reorg:** WorkspacesDir() returns DataDir()/workspaces (or BUILDMAX_WORKSPACES_DIR). PersistentWorkspaceDir(workspaceID) returns WorkspacesDir()/workspaceID/persist. RuntimeWorkspaceDir(workspaceID, taskID) — new signature with workspaceID; returns WorkspacesDir()/workspaceID/tasks/taskID. Add ArtifactDir(workspaceID, taskID, artifactID). Unit tests for all. |
| **internal/store** | Task: add ArtifactSeq int (column artifact_seq), LastArtifactID *string (last_artifact_id). Artifact, ArtifactItem structs and TableName. TaskStore: add IncrementTaskSeq(ctx, taskID) (int, error). ArtifactStore: add CreateArtifactWithItem(ctx, taskID, artifactID, seq, relativePath). Store: implement both; add Artifact, ArtifactItem to AutoMigrate. IncrementTaskSeq: transaction on artifact_seq. CreateArtifactWithItem: transaction (insert artifact, insert artifact_item, update task.last_artifact_id). |
| **internal/executor** | Use PersistentWorkspaceDir(task.WorkspaceID), RuntimeWorkspaceDir(task.WorkspaceID, task.TaskID). Extend TaskStore interface with IncrementTaskSeq and CreateArtifactWithItem. In executeTask, after copying result to persist and only on success: call IncrementTaskSeq; generate artifactID; artifactDir = ArtifactDir(workspaceID, taskID, artifactID); MkdirAll(artifactDir); copy result to artifactDir/result.md; CreateArtifactWithItem. All best-effort with slog.Warn. Add id import. |
| **internal/server** | ensureWorkspaceDirs: ensure workspace root exists (WorkspacesDir()/workspaceID) and optionally persist subdir. createWorkspaceHandler: create WorkspacesDir()/workspaceID and persist subdir. upload, files: use PersistentWorkspaceDir(workspaceID) (unchanged API usage; path value changes). |
| **internal/cmd** | No change (executor.New(st) still; st implements extended interface). |
