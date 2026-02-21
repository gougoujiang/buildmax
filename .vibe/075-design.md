# Design 075: Workspace structure redesign

Technical design for task [075](075.md): rename workspace `persist` → `home` and adopt per-run layout `chats/<chatID>/<chatRunID>/{home,artifacts,global}`.

## Goal

- Workspace-level uploads live under `home` (not `persist`); config and blob keys use segment `home`.
- Each run uses one directory `chats/<chatID>/<chatRunID>/` with subdirs: `home` (materialized workspace home), `artifacts` (run outputs, e.g. `result.md`), `global` (BUILDMAX_HOME). Agent runs with cwd = run dir and BUILDMAX_HOME = run `global`; result is written to run `artifacts/result.md` and archived; **run `global` is still uploaded to blob store (MinIO)** after each run—same as the previous `buildmax` dir, only the on-disk name and blob key segment change to `global`.

## Modules and boundaries

| Package / layer | Responsibility |
|-----------------|----------------|
| `internal/config` | Path helpers: workspace home dir, run dir, run home/artifacts/global. Single source of path segments. |
| `internal/executor` | WorkspacePaths interface and impl; RunTask creates run dirs, materializes to run home, runs buildmax, writes result to run artifacts, archives result, uploads run global. |
| `internal/storage/blob` | Key builders: workspace segment `home`; chat run segment `global` (or keep `buildmax`). PersistStorage interface unchanged; implementations use config-supplied root (which will point to `home`). |
| `internal/server` | Workspace dir creation and file/upload use `home`; conversation loader reads run data from blob using chatID + chatRunID. |
| `internal/storage/setup` | BuildPersistStorage continues to receive a root func from config; that func will return `.../home`. |
| `internal/workercmd` | No interface change; continues to call executor.RunTask with paths from config. |

## Target directory structure (reference)

```
<workspace_root>/<workspace_id>/
├── home/                                    # Uploads (replaces persist)
├── chats/<chat_id>/<chat_run_id>/
│   ├── .buildmax/                           # Optional; out of scope
│   ├── home/                                # Materialized workspace home
│   ├── artifacts/
│   │   ├── result.md
│   │   └── (other generated files)
│   └── global/                              # BUILDMAX_HOME (sessions, logs)
└── artifacts/<chat_id>/<chat_run_id>/<artifact_id>/   # Archive (unchanged)
```

## 1. Config (`internal/config`)

**PersistentWorkspaceDir(workspaceID string) string**  
- Change: return `filepath.Join(WorkspacesDir(), workspaceID, "home")` (today: `"persist"`).
- Callers: server (ensureWorkspaceDirs, persistentWorkspaceDir, createWorkspaceHandler), setup.BuildPersistStorage, executor tests. No signature change.

**RuntimeWorkspaceDir(workspaceID, chatID string) string**  
- Keep: `.../chats/<chatID>`. Still used for grouping; optional for future list-dir. Can be used to derive run dir.

**RuntimeChatRunDir(workspaceID, chatID, chatRunID string) string**  
- Keep: `.../chats/<chatID>/<chatRunID>`. This becomes the run root (cwd for buildmax).

**New helpers (add):**  
- `RuntimeChatRunHomeDir(workspaceID, chatID, chatRunID string) string` → `RuntimeChatRunDir(...) + "/home"`.  
- `RuntimeChatRunArtifactsDir(workspaceID, chatID, chatRunID string) string` → `RuntimeChatRunDir(...) + "/artifacts"`.  
- `RuntimeChatRunGlobalDir(workspaceID, chatID, chatRunID string) string` → `RuntimeChatRunDir(...) + "/global"` (replaces role of RuntimeChatRunBuildmaxDir).

**Deprecate / remove from use:**  
- `RuntimeChatBuildmaxDir`: remove or leave returning `.../chats/<chatID>/buildmax` (unused after switch).  
- `RuntimeChatRunBuildmaxDir`: replace all usages with `RuntimeChatRunGlobalDir`.  
- `RuntimeChatWSDir`: replace all usages with run-scoped `RuntimeChatRunHomeDir` (materialize target).

**ArtifactDir**  
- Unchanged: `.../artifacts/<chatID>/<chatRunID>/<artifactID>`.

## 2. WorkspacePaths interface (`internal/executor/paths.go`)

**Interface changes:**  
- Keep: `PersistentWorkspaceDir(workspaceID string)`, `ArtifactDir(workspaceID, chatID, chatRunID, artifactID string)`.  
- Remove: `RuntimeWorkspaceDir`, `RuntimeChatBuildmaxDir`, `RuntimeChatWSDir` (no longer used by RunTask).  
- Add: `RuntimeChatRunDir(workspaceID, chatID, chatRunID string) string`, `RuntimeChatRunHomeDir(...)`, `RuntimeChatRunArtifactsDir(...)`, `RuntimeChatRunGlobalDir(...)`.

**workspacePathsRoot:**  
- `PersistentWorkspaceDir`: return `filepath.Join(p.root, workspaceID, "home")`.  
- Implement new run-scoped methods with segments `home`, `artifacts`, `global` under `.../chats/<chatID>/<chatRunID>/`.  
- Remove implementations for RuntimeWorkspaceDir, RuntimeChatBuildmaxDir, RuntimeChatRunBuildmaxDir, RuntimeChatWSDir.

## 3. Executor (`internal/executor/executor.go`)

**RunTask flow (updated):**  
1. Resolve paths: runDir = paths.RuntimeChatRunDir(workspaceID, chatID, chatRunID), runHome = paths.RuntimeChatRunHomeDir(...), runArtifacts = paths.RuntimeChatRunArtifactsDir(...), runGlobal = paths.RuntimeChatRunGlobalDir(...).  
2. ensureRunDirs(runHome, runArtifacts, runGlobal) — create run dir and the three subdirs.  
3. restoreSessionFromPreviousRun(..., runGlobal, persist) — write session into runGlobal/sessions.  
4. persist.MaterializeToDir(ctx, workspaceID, runHome) — materialize workspace home into run home.  
5. runBuildmaxCmd(ctx, run, runDir, runGlobal, sessionID): cmd.Dir = runDir, BUILDMAX_HOME = runGlobal.  
6. Write result: run `artifacts/result.md` under runArtifacts (path runArtifacts + "/result.md"); also keep in-memory output for reportRunSuccess.  
7. uploadChatBuildmax(ctx, runGlobal, workspaceID, chatID, chatRunID, persist) — upload run `global` dir to blob (key layout: see blob section).  
8. reportRunSuccess(..., resultFilename "result.md", ...): artifactStorage.PutResult(..., output) unchanged; RelativePath in payload = "result.md".

**ensureRunDirs:**  
- Signature: `ensureRunDirs(runHome, runArtifacts, runGlobal string) error`. Create each with os.MkdirAll(0755).

**restoreSessionFromPreviousRun:**  
- Parameter buildmaxDir → runGlobalDir. Write session file under runGlobalDir/sessions.

**runBuildmaxCmd:**  
- Signature: `runBuildmaxCmd(ctx, run, runDir, runGlobalDir, sessionID) ([]byte, error)`. cmd.Dir = runDir, BUILDMAX_HOME = runGlobalDir.

**persistRunResult:**  
- Write to runArtifactsDir + "/result.md" (not next to buildmax dir). Call from RunTask with runArtifacts path.

**uploadChatBuildmax:**  
- Parameter buildmaxDir → globalDir. **Must still run after each run**: upload the full contents of the run’s `global` dir (sessions/, logs/, settings.json, etc.) to blob store (MinIO) via `persist.PutChatBuildmax`, so conversation and logs can be read back from blob. Same behavior as today with `buildmax`; only the on-disk dir is now `global` and the blob key segment is `global` (see blob keys section).

## 4. Blob storage keys (`internal/storage/blob/keys.go`)

**Note:** The run’s BUILDMAX_HOME dir (now named `global` on disk) is still uploaded to blob/MinIO after every run. Keys use the segment `global` instead of `buildmax`; upload/read behavior is unchanged.

**PersistObjectKey(prefix, workspaceID, relPath):**  
- Change segment from `"persist"` to `"home"`.

**PersistPrefix(prefix, workspaceID):**  
- Change segment from `"persist"` to `"home"`.

**Chat buildmax / run global:**  
- Option A (recommended): Add `ChatRunGlobalObjectKey(prefix, workspaceID, chatID, chatRunID, relPath)` returning `.../chats/<chatID>/<chatRunID>/global/<relPath>`.  
- Option B: Keep `ChatBuildmaxObjectKey` but have it return `.../global/...` (rename semantically to “run global”).  
- S3 and local FS persist implementations that upload/read run data must use the same key layout. S3 persist: today uses `ChatBuildmaxObjectKey` for PutChatBuildmax; switch to the new key (global). Local FS: PutChatBuildmax is no-op; GetChatBuildmax returns ErrNotFound — server conversation loader will read from local path when not using S3; that local path must be run `global` dir: `.../chats/<chatID>/<chatRunID>/global/...`.

**Decision:** Use segment `global` in blob keys for run BUILDMAX_HOME data: `chats/<chatID>/<chatRunID>/global/<relPath>`. Add `ChatRunGlobalObjectKey` and use it in S3 persist; keep GetChatBuildmax/PutChatBuildmax semantics but key layout uses `global`. Alternatively rename ChatBuildmaxObjectKey to ChatRunGlobalObjectKey and change the segment to `global` (one function, new name and segment).

## 5. Blob implementations

**Local FS persist:**  
- Root is still from config (PersistentWorkspaceDir → now `.../home`). Put/Get/ListFiles/MaterializeToDir use that root; no code change except config. PutChatBuildmax remains no-op; GetChatBuildmax remains ErrNotFound.

**S3 persist:**  
- PutChatBuildmax: build key with `.../chats/<chatID>/<chatRunID>/global/<relPath>` (use new key helper).  
- GetChatBuildmax: same key layout.  
- MaterializeToDir: source remains workspace “persist” prefix; in keys.go that prefix is now `home`.

**Artifact storage:**  
- No key layout change: `artifacts/<chatID>/<chatRunID>/<artifactID>/result.md`. PutResult/GetResult unchanged. Executor still calls PutResult once per run with result.md content.

## 6. Server (`internal/server`)

**persistentWorkspaceDir(workspaceID):**  
- Return `filepath.Join(s.workspacesDir(), workspaceID, "home")` (today: `"persist"`).

**ensureWorkspaceDirs(root, workspaceIDs):**  
- Create `filepath.Join(root, id, "home")` for each workspace (today: `"persist"`).

**createWorkspaceHandler:**  
- Uses s.persistentWorkspaceDir(ws.WorkspaceID); no change except the path segment inside that helper.

**files.go (list/download):**  
- Uses persist storage and workspace root; root comes from config/persistentWorkspaceDir → `home`. No interface change.

**upload.go:**  
- Uses PersistStorage.Put; keys are under workspace “persist” prefix, which becomes `home`. No change except key segment in blob.

**loadChatConversationData (chats.go):**  
- GetChatBuildmax(..., lastRunID, relPath) unchanged; blob key for that run will use `global` segment. Local fallback path: `filepath.Join(s.workspacesDir(), chat.WorkspaceID, "chats", chat.ChatID, lastRunID, "global", "sessions", sessionID+".json")` (today: `"buildmax"`).

## 7. Setup and workercmd

**setup.BuildPersistStorage(cfg, persistRoot, s3Client):**  
- Caller passes config.PersistentWorkspaceDir; that func will now return `.../home`. No change in setup package.

**workercmd:**  
- Builds paths via config (or executor’s WorkspacePaths from root). Uses executor.RunTask. No change except that paths implementation uses new run-scoped dirs.

## 8. Tests

**internal/config/config_test.go:**  
- TestPersistentWorkspaceDir: expect `".../home"` instead of `".../persist"`.  
- TestRuntimeChatRunBuildmaxDir: replace with TestRuntimeChatRunGlobalDir expecting `.../chats/.../global`.  
- TestRuntimeChatWSDir: remove or replace with TestRuntimeChatRunHomeDir expecting `.../chats/.../runID/home`.

**internal/executor/executor_test.go:**  
- testWorkspacePaths: implement new interface (RuntimeChatRunDir, RuntimeChatRunHomeDir, RuntimeChatRunArtifactsDir, RuntimeChatRunGlobalDir); remove RuntimeWorkspaceDir, RuntimeChatBuildmaxDir, RuntimeChatRunBuildmaxDir, RuntimeChatWSDir.  
- Test RunTask: ensure materialize target is run home, buildmax dir is run global, result written under run artifacts/result.md, upload uses run global.

**internal/executor/paths_test.go (if any):**  
- Update expected path segments to `home`, `global`, run-scoped `home`/`artifacts`/`global`.

**internal/storage/blob/keys_test.go:**  
- PersistObjectKey and PersistPrefix: expect segment `home`.  
- ChatBuildmaxObjectKey → ChatRunGlobalObjectKey (or same name with `global` segment): expect `.../global/...`.

## How they work together

1. **Server / upload / files**  
   Workspace root for uploads and file list is `PersistentWorkspaceDir(workspaceID)` → `.../home`. Blob keys use `home`. Ensure workspace creates `.../home`.

2. **Worker run**  
   RunTask gets WorkspacePaths from root (workercmd). Paths: run dir = `chats/<chatID>/<chatRunID>`, run home/artifacts/global under it. Create dirs; materialize workspace home → run home; restore session into run global; run buildmax with Dir=runDir, BUILDMAX_HOME=runGlobal; write result to run artifacts/result.md; upload run global to blob (`.../chats/<chatID>/<chatRunID>/global/...`); report success with artifact (PutResult for result.md).

3. **Conversation load**  
   Server calls GetChatBuildmax(workspaceID, chatID, lastRunID, relPath). Blob key is `.../chats/<chatID>/<chatRunID>/global/<relPath>`. Local fallback: `.../chats/<chatID>/<chatRunID>/global/sessions/<id>.json`.

## Changes for review

| Area | Files | Change |
|------|-------|--------|
| Config | `internal/config/config.go` | PersistentWorkspaceDir → `home`; add RuntimeChatRunHomeDir, RuntimeChatRunArtifactsDir, RuntimeChatRunGlobalDir; keep RuntimeChatRunDir. Remove or retain RuntimeChatBuildmaxDir, RuntimeChatRunBuildmaxDir, RuntimeChatWSDir (retain for compatibility or remove if unused). |
| Config tests | `internal/config/config_test.go` | Update TestPersistentWorkspaceDir; add/update tests for run-scoped paths; adjust/remove Tests for RuntimeChatWSDir, RuntimeChatRunBuildmaxDir. |
| Executor paths | `internal/executor/paths.go` | WorkspacePaths: drop RuntimeWorkspaceDir, RuntimeChatBuildmaxDir, RuntimeChatRunBuildmaxDir, RuntimeChatWSDir; add RuntimeChatRunDir, RuntimeChatRunHomeDir, RuntimeChatRunArtifactsDir, RuntimeChatRunGlobalDir. PersistentWorkspaceDir → `home`. Implement in workspacePathsRoot. |
| Executor | `internal/executor/executor.go` | RunTask: use run-scoped paths; ensureRunDirs(runHome, runArtifacts, runGlobal); materialize to runHome; runBuildmaxCmd(runDir, runGlobal); persistRunResult(runArtifacts, output); uploadChatBuildmax(runGlobal, ...); reportRunSuccess with result.md. Update ensureRunDirs, restoreSessionFromPreviousRun, runBuildmaxCmd, persistRunResult, uploadChatBuildmax signatures/usages. |
| Executor tests | `internal/executor/executor_test.go` | testWorkspacePaths implements new interface; RunTask test expects new paths and result under run artifacts. |
| Blob keys | `internal/storage/blob/keys.go` | PersistObjectKey, PersistPrefix: `persist` → `home`. ChatBuildmaxObjectKey → use segment `global` (rename to ChatRunGlobalObjectKey or change segment); same signature. |
| Blob keys tests | `internal/storage/blob/keys_test.go` | Expect `home`; expect `global` in run key. |
| S3 persist | `internal/storage/blob/s3_persist.go` | PutChatBuildmax/GetChatBuildmax use key with `global` segment (via updated keys helper). MaterializeToDir uses PersistPrefix (now `home`). |
| Server | `internal/server/paths.go` | persistentWorkspaceDir → `"home"`. |
| Server | `internal/server/workspaces.go` | ensureWorkspaceDirs: create `.../home`. |
| Server | `internal/server/chats.go` | loadChatConversationData: local path uses `"global"` instead of `"buildmax"`. |

No new packages. No change to ArtifactStorage interface or artifact key layout.
