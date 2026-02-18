# Design 064: Upload session file and log file to MinIO after task finish

## Goal

After a task run (success or failure), upload the worker’s buildmax directory (logs, sessions, settings) to object storage when persist storage is MinIO, using keys under `prefix/workspaceID/tasks/taskID/buildmax/...`. Local FS persist remains no-op for this upload.

## Modules

| Module | Responsibility | Owns |
|--------|----------------|------|
| **internal/storage/blob** | Key layout for task buildmax; PersistStorage interface and implementations (LocalFS, S3). | `keys.go`, `interfaces.go`, `localfs_persist.go`, `s3_persist.go` |
| **internal/executor** | Run task (materialize, run buildmax -p, update status); **new**: after agent run, call upload of buildmax files. | `executor.go`, `executor_test.go` |

No new packages. Config unchanged (reuse `BUILDMAX_PERSIST_STORAGE`).

## Structure

**Key layout (blob)**

- Existing: `PersistObjectKey(prefix, workspaceID, relPath)` → `prefix/workspaceID/persist/<relPath>`.
- **New**: `TaskBuildmaxObjectKey(prefix, workspaceID, taskID, relPath string) (string, error)` → `prefix/workspaceID/tasks/taskID/buildmax/<relPath>`, with `relPath` validated via `CleanRelPath` (no `..`, no absolute, forward slashes). Same package `blob`; same validation rules as persist keys.

**PersistStorage interface (blob)**

- **New method**: `PutTaskBuildmax(ctx context.Context, workspaceID, taskID, relPath string, r io.Reader) error`.
  - Writes one file under the task buildmax key space. Used only by the executor after the agent run.
- **LocalFSPersistStorage**: Implement as no-op: return `nil` immediately (task buildmax files already live on worker disk; we do not duplicate them under the persist root).
- **S3PersistStorage**: Compute key with `TaskBuildmaxObjectKey(s.prefix, workspaceID, taskID, relPath)`; call `s.client.PutObject(ctx, s.bucket, key, r)`. Reuse existing `CleanRelPath` so path escape is rejected.

**Executor (internal/executor)**

- **New helper** (package-private): `uploadTaskBuildmax(ctx, buildmaxDir, workspaceID, taskID string, persist blob.PersistStorage)`.
  - **Inputs**: `buildmaxDir` = `paths.RuntimeTaskBuildmaxDir(workspaceID, taskID)`; `persist` = same as passed to `RunTask`.
  - **Behaviour**:
    1. Fixed relPaths to try: `"logs/buildmax.log"`, `"settings.json"`.
    2. List files in `buildmaxDir/sessions` (if dir exists); for each regular file, relPath = `"sessions/" + name` (e.g. `sessions/sessions.json`, `sessions/<id>.json`).
    3. For each relPath, fullPath = `filepath.Join(buildmaxDir, filepath.FromSlash(relPath))`. If `os.Stat(fullPath)` succeeds and is a regular file, open it and call `persist.PutTaskBuildmax(ctx, workspaceID, taskID, relPath, reader)`. On any error (open, read, PutTaskBuildmax), log with `slog.Warn` and continue (best-effort; do not return error to caller).
  - **Invocation**: From `RunTask`, call `uploadTaskBuildmax(...)` once after `cmd.CombinedOutput()` returns, regardless of success or failure — i.e. after writing the result file and before the two branches (FAILED vs SUCCEEDED). So both failed and succeeded tasks get their buildmax dir uploaded when persist is MinIO.
- **RunTask** signature unchanged: still receives `persist blob.PersistStorage`; no new parameters. Behavior is driven by the implementation (LocalFS no-op, S3 upload).

## Method design

**blob/keys.go**

- `TaskBuildmaxObjectKey(prefix, workspaceID, taskID, relPath string) (string, error)`
  - Call `CleanRelPath(relPath)`; if err != nil return err.
  - Return `path.Join(prefix, workspaceID, "tasks", taskID, "buildmax", clean)`, nil.

**blob/interfaces.go**

- Add to `PersistStorage`: `PutTaskBuildmax(ctx context.Context, workspaceID, taskID, relPath string, r io.Reader) error`.

**blob/localfs_persist.go**

- `func (s *LocalFSPersistStorage) PutTaskBuildmax(ctx context.Context, workspaceID, taskID, relPath string, r io.Reader) error { return nil }`.

**blob/s3_persist.go**

- `func (s *S3PersistStorage) PutTaskBuildmax(ctx context.Context, workspaceID, taskID, relPath string, r io.Reader) error`
  - key, err := TaskBuildmaxObjectKey(s.prefix, workspaceID, taskID, relPath); if err != nil return err
  - return s.client.PutObject(ctx, s.bucket, key, r)

**executor/executor.go**

- After the block that does `output, err := cmd.CombinedOutput()` and writes `resultPath`, and before the `if err != nil { ... return err }` block, call:
  - `uploadTaskBuildmax(ctx, buildmaxDir, task.WorkspaceID, task.TaskID, persist)`
- Implement `uploadTaskBuildmax` in the same file: accept context, buildmaxDir, workspaceID, taskID, persist; iterate over the fixed relPaths and sessions dir entries; for each existing file, open and call `persist.PutTaskBuildmax`; on error log and continue.

## How they work together

**Flow**

1. Worker runs `RunTask` with task, paths, persist (from workercmd: BuildPersistStorage), artifactStorage, updater.
2. RunTask creates buildmax dir, materializes workspace, runs `buildmax -p` with BUILDMAX_HOME=buildmaxDir. CLI writes logs, sessions, settings under buildmaxDir.
3. After `cmd.CombinedOutput()` returns, RunTask calls `uploadTaskBuildmax(ctx, buildmaxDir, task.WorkspaceID, task.TaskID, persist)`.
4. **LocalFSPersistStorage**: each `PutTaskBuildmax` is no-op; nothing is written.
5. **S3PersistStorage**: each `PutTaskBuildmax` writes one object at `prefix/workspaceID/tasks/taskID/buildmax/<relPath>`.
6. RunTask then continues as today (report FAILED or SUCCEEDED, artifact upload on success). Upload errors in step 4–5 are only logged; they do not change task status.

**Dependencies**

- Executor depends on `blob.PersistStorage` (already). No new dependency on config for provider type; the interface hides it.
- workercmd unchanged: still builds persist via `config.BuildPersistStorage(...)` and passes it to `RunTask`.

**Edge cases**

- Missing file (e.g. no log): `uploadTaskBuildmax` skips that path (stat or open fails → log and continue).
- PutTaskBuildmax fails (e.g. MinIO down): log and continue; do not fail the task.
- sessions dir missing or not a dir: skip listing sessions; still upload logs/buildmax.log and settings.json if present.
- Empty sessions dir: no session files uploaded; fixed paths still attempted.

## Tests

- **blob/keys_test.go**: Add tests for `TaskBuildmaxObjectKey` (valid relPaths, rejection of `..`, empty, absolute).
- **blob**: Optional unit test for S3PersistStorage.PutTaskBuildmax using a mock S3 client that records PutObject keys (or reuse existing S3 test patterns if any). LocalFS: trivial (no-op).
- **executor/executor_test.go**: 
  - Extend `fakePersistStorage` to implement `PutTaskBuildmax` and record calls (e.g. map key `workspaceID+"/"+taskID` → list of relPaths or map of relPath→content) so tests can assert that upload was attempted for the expected files after a run.
  - In an existing or new test that runs RunTask to success or failure, assert that `uploadTaskBuildmax` was invoked (e.g. fake records PutTaskBuildmax for the task’s workspaceID/taskID with relPaths like `logs/buildmax.log`, `sessions/sessions.json`, `settings.json` when those files were created in the buildmax dir). Prefer table-driven or scenario that creates buildmax dir with a subset of files and asserts only those are uploaded.

## Changes for review

- **internal/storage/blob/keys.go**: Add `TaskBuildmaxObjectKey(prefix, workspaceID, taskID, relPath string) (string, error)`; use `CleanRelPath(relPath)` and `path.Join(prefix, workspaceID, "tasks", taskID, "buildmax", clean)`.
- **internal/storage/blob/interfaces.go**: Add `PutTaskBuildmax(ctx context.Context, workspaceID, taskID, relPath string, r io.Reader) error` to `PersistStorage`.
- **internal/storage/blob/localfs_persist.go**: Implement `PutTaskBuildmax` as no-op (return nil).
- **internal/storage/blob/s3_persist.go**: Implement `PutTaskBuildmax` using `TaskBuildmaxObjectKey` and `s.client.PutObject`.
- **internal/executor/executor.go**: Add private `uploadTaskBuildmax(ctx, buildmaxDir, workspaceID, taskID string, persist blob.PersistStorage)`; call it from `RunTask` after `cmd.CombinedOutput()` and result file write, before the success/failure branch. In `uploadTaskBuildmax`, iterate over fixed relPaths and `sessions` dir entries, and for each existing file call `persist.PutTaskBuildmax` (log and continue on error).
- **internal/executor/executor_test.go**: Extend `fakePersistStorage` with `PutTaskBuildmax` and recording; add or extend test to verify buildmax upload is called with expected relPaths when buildmax dir contains the corresponding files.
- **internal/storage/blob/keys_test.go** (or new test file): Add tests for `TaskBuildmaxObjectKey`.
