# Design 052: Separate runtime workspace from persistence workspace

## Goal

Introduce two workspace roots: **persistent** (user uploads and stored files) and **runtime** (ephemeral per-task directory). Executor copies persistent → runtime before run, runs the agent in the runtime dir, then copies the result file back to the persistent workspace. Server and config expose only the persistent root for upload/files/workspace creation; executor uses both roots.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/config** | Workspace path API | PersistentWorkspaceDir, RuntimeWorkspaceDir; WorkspacesDir redefined as persistent root |
| **internal/executor** | Task execution with copy-in/copy-out | Runner (no workspacesDir field); copy persist→runtime, run, copy result back (runtime dir left on disk; cleanup in separate task) |
| **internal/server** | HTTP API for workspaces, upload, files | All handlers use persistent workspace path only |
| **cmd/buildmax (internal/cmd)** | Server wiring | Pass only TaskStore to executor.New |

## Structure

**internal/config**

- Add **PersistentWorkspaceDir(workspaceID string) string**: returns `filepath.Join(WorkspacesDir(), workspaceID)`. Used for upload destination, file tree root, file content root, and ensureWorkspaceDirs / create workspace.
- Add **RuntimeWorkspaceDir(taskID string) string**: returns `filepath.Join(DataDir(), "workspace", "task", taskID)`. No env override in this task.
- Change **WorkspacesDir() string**: return value becomes the **persistent workspace root** only. Default `filepath.Join(DataDir(), "workspace", "persist")`. Keep existing env `BUILDMAX_WORKSPACES_DIR` as override for this root so one env still controls persistent location. So: if `BUILDMAX_WORKSPACES_DIR` set, return `filepath.Clean(dir)`; else return `filepath.Join(DataDir(), "workspace", "persist")`. This breaks existing installs that use default `DataDir()/workspaces`; document that new layout is `workspace/persist` and `workspace/task`; migration of existing data is out of scope (doc or follow-up).

**internal/executor**

- **Runner**: remove field `workspacesDir`. Constructor becomes **New(store TaskStore) *Runner** (single arg). Executor will import `buildmax/internal/config` and call config in executeTask.
- **executeTask** flow:
  1. `persistDir := config.PersistentWorkspaceDir(task.WorkspaceID)`; `runtimeDir := config.RuntimeWorkspaceDir(task.TaskID)`.
  2. Mark task RUNNING (unchanged).
  3. Create runtime dir: `os.MkdirAll(runtimeDir, 0755)`; on failure, failTask and return.
  4. Copy persistent workspace into runtime: call a private helper `copyWorkspaceContents(persistDir, runtimeDir)`. If `persistDir` does not exist or is not a directory, treat as empty (no error); copy only regular files and directories (no symlink following requirement; simple recursive copy).
  5. Run buildmax with `cmd.Dir = runtimeDir`; write `result-<task_id>.md` under runtimeDir as today (unchanged).
  6. Copy result file from runtime to persistent: copy `runtimeDir/result-<task_id>.md` to `persistDir/result-<task_id>.md` (overwrite). Use `os.ReadFile` + `os.WriteFile` or `io.Copy`; ensure `persistDir` exists (`MkdirAll`) before write.
  7. Update task status (SUCCEEDED/FAILED) as today.
  - **Do not** remove the runtime dir after the run; runtime workspace cleanup will be implemented in a separate task.
- **copyWorkspaceContents(src, dst string) error**: private helper. If src is missing or not a dir, return nil (no-op). Walk src recursively; for each file create corresponding path under dst and copy bytes; for each dir create under dst. Ignore symlinks (or copy as regular files; implementation choice). Return first error from MkdirAll or copy.

**internal/server**

- **workspaces.go**: `ensureWorkspaceDirs(config.WorkspacesDir(), ids)` unchanged (WorkspacesDir() now returns persistent root). `createWorkspaceHandler`: replace `destDir := filepath.Join(config.WorkspacesDir(), ws.WorkspaceID)` with `destDir := config.PersistentWorkspaceDir(ws.WorkspaceID)`.
- **upload.go**: replace `destDir := filepath.Join(config.WorkspacesDir(), workspaceID)` with `destDir := config.PersistentWorkspaceDir(workspaceID)`.
- **files.go**: replace `filepath.Join(config.WorkspacesDir(), workspaceID)` with `config.PersistentWorkspaceDir(workspaceID)` for wsDir and for `util.Workspace{Root: ...}`.

**internal/cmd (server.go)**

- Replace `runner := executor.New(st, config.WorkspacesDir())` with `runner := executor.New(st)`.

## Method design

| Package / layer | Component | Method / function | Signature / contract |
|----------------|-----------|-------------------|----------------------|
| **config** | (package) | WorkspacesDir | `WorkspacesDir() string`. Returns persistent workspace root: `DataDir()/workspace/persist` or `BUILDMAX_WORKSPACES_DIR` if set. |
| **config** | (package) | PersistentWorkspaceDir | `PersistentWorkspaceDir(workspaceID string) string`. Returns `filepath.Join(WorkspacesDir(), workspaceID)`. |
| **config** | (package) | RuntimeWorkspaceDir | `RuntimeWorkspaceDir(taskID string) string`. Returns `filepath.Join(DataDir(), "workspace", "task", taskID)`. No env override. |
| **executor** | Runner | New | `New(store TaskStore) *Runner`. No workspacesDir argument. |
| **executor** | Runner | executeTask | Uses config.PersistentWorkspaceDir(task.WorkspaceID) and config.RuntimeWorkspaceDir(task.TaskID). Creates runtime dir; copyWorkspaceContents(persistDir, runtimeDir); run buildmax in runtimeDir; write result in runtimeDir; copy result file to persistDir; update status. Runtime dir is left on disk (cleanup in separate task). |
| **executor** | (package) | copyWorkspaceContents | `copyWorkspaceContents(src, dst string) error`. If src missing or not dir, return nil. Recursively copy files/dirs from src to dst. |
| **server** | workspaces | ensureWorkspaceDirs | Keep `ensureWorkspaceDirs(root string, workspaceIDs []string)`; callers pass `config.WorkspacesDir()` so root is persistent root. |
| **server** | workspaces | createWorkspaceHandler | Use `config.PersistentWorkspaceDir(ws.WorkspaceID)` for destDir. |
| **server** | upload | uploadHandler | Use `config.PersistentWorkspaceDir(workspaceID)` for destDir. |
| **server** | files | filesTreeHandler, fileContentHandler | Use `config.PersistentWorkspaceDir(workspaceID)` for wsDir and Workspace.Root. |

## How they work together

**Task run (executor)**

1. Poll yields task (workspace_id, task_id, input).
2. persistDir = config.PersistentWorkspaceDir(task.WorkspaceID); runtimeDir = config.RuntimeWorkspaceDir(task.TaskID).
3. MkdirAll(runtimeDir). copyWorkspaceContents(persistDir, runtimeDir).
4. exec buildmax with Dir = runtimeDir; write result to runtimeDir/result-<task_id>.md.
5. MkdirAll(persistDir) if needed; copy result file from runtimeDir to persistDir.
6. UpdateTaskStatus. (Runtime dir is not removed; cleanup will be a separate task.)

**Upload / file list / file content (server)**

- All resolve workspace root via config.PersistentWorkspaceDir(workspaceID). User sees uploads and result file in the same workspace.

**Create workspace / list workspaces (server)**

- ensureWorkspaceDirs uses config.WorkspacesDir() (persistent root) and creates root/id for each id. createWorkspaceHandler creates config.PersistentWorkspaceDir(ws.WorkspaceID).

**Startup (cmd)**

- executor.New(st) only; no path passed. Executor reads paths from config at run time.

## Errors and edge cases

- **persistDir missing**: copyWorkspaceContents treats as empty; run proceeds with empty runtime dir.
- **MkdirAll(runtimeDir) fails**: failTask with message and return.
- **Copy result to persistDir fails**: log warning; still update task status; result remains only in DB (output field). Result file may also remain in runtime dir (cleanup handled separately).

## Tests

- **config**: Test PersistentWorkspaceDir and RuntimeWorkspaceDir return paths under DataDir()/workspace/persist and .../workspace/task with correct suffix. Test WorkspacesDir returns persistent root (with or without BUILDMAX_HOME set).
- **executor**: Test executeTask with a persistent dir containing a file (e.g. `in.txt`); after run, assert persistDir contains `result-<task_id>.md` and optionally still has `in.txt`. Test that agent runs in runtime dir and result is copied to persist (runtime dir is left on disk). Test copyWorkspaceContents: src missing → nil; src dir with one file → file under dst; src empty dir → empty dst dir.
- **server**: Existing tests that depend on workspace paths should still pass; ensure test config or temp dir uses the new path layout (no change to API contract, only internal paths).

## Backward compatibility note

- Default path for persistent workspaces changes from `DataDir()/workspaces` to `DataDir()/workspace/persist`. Deployments that already have data under `workspaces/` will not see it in the new layout; document in README or release notes. Migration (copy `workspaces/<id>/*` → `workspace/persist/<id>/`) is out of scope for this task.

---

## Changes for review

| Area | Change |
|------|--------|
| **internal/config** | Add PersistentWorkspaceDir(workspaceID), RuntimeWorkspaceDir(taskID). Change WorkspacesDir() to return DataDir()/workspace/persist (keep BUILDMAX_WORKSPACES_DIR as override). Add tests for the two new functions and WorkspacesDir. |
| **internal/executor** | Runner: drop workspacesDir field. New(store TaskStore). executeTask: resolve persistDir and runtimeDir via config; add copyWorkspaceContents(persist, runtime); run in runtimeDir; copy result file to persistDir (do not remove runtime dir; cleanup in separate task). Add private copyWorkspaceContents. Add tests for copy-in and result copy-out. |
| **internal/server** | workspaces.go: createWorkspaceHandler use PersistentWorkspaceDir. ensureWorkspaceDirs keep signature, call with config.WorkspacesDir(). upload.go: destDir = config.PersistentWorkspaceDir(workspaceID). files.go: wsDir and Workspace.Root = config.PersistentWorkspaceDir(workspaceID). |
| **internal/cmd** | server.go: executor.New(st) instead of executor.New(st, config.WorkspacesDir()). |
