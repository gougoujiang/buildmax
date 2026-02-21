# Design 076: Remove artifact table

Technical design for task [076](076.md): drop the `artifact` table, key run output by `chat_run_id`, and use a single blob path per run.

## Goal

- One "artifact" per successful chat run; identity is `chat_run_id`. No separate `artifact` table or `artifact_id`.
- File metadata lives in a table keyed by `chat_run_id` (e.g. `chat_run_output_file`).
- Blob layout: `.../artifacts/<chatID>/<chatRunID>/<relPath>` (no artifactID segment).
- API and Portal keep current routes and response shape; `id` in artifact endpoints is `chat_run_id`; list returns run-based rows with `artifact_id` field set to `chat_run_id` for compatibility.

## Modules and boundaries

| Package / layer | Responsibility |
|-----------------|----------------|
| `internal/model` | Remove `Artifact`; add `ChatRunOutputFile`; refactor `ArtifactWithChat` to run-based DTO (chat_run_id, chat_id, workspace_id, created_at, chat_input_snippet). Remove `LastArtifactID`, `ArtifactSeq` from `Chat`. |
| `internal/storage/entity` | Drop artifact table and artifact_item; add chat_run_output_file. New interface for "run output" (list by workspace, get run, list files for run, record files on completion). Store implements it; remove ArtifactStore. |
| `internal/storage/blob` | ArtifactStorage: remove artifactID from interface and keys. Key layout: `.../artifacts/<chatID>/<chatRunID>/result.md` and `.../artifacts/<chatID>/<chatRunID>/<relPath>`. |
| `internal/config` | ArtifactDir → RunOutputDir(workspaceID, chatID, chatRunID) or keep ArtifactDir with three args (no artifactID). |
| `internal/executor` | reportRunSuccess: no artifact_id; use chat_run_id; PutResult/PutArtifactFile with (workspaceID, chatID, chatRunID) only. WorkspacePaths: drop ArtifactDir(..., artifactID); add RunOutputDir(workspaceID, chatID, chatRunID) if needed for blob. |
| `internal/workerapi` | ArtifactPayload: drop artifact_id; keep relative_paths (and optional relative_path for backward compat). |
| `internal/server` | OnRunComplete(chatRunID, relativePaths); artifact handlers resolve by chat_run_id; list = list runs with output; keep ArtifactStore name in config but type becomes "run output" methods on Store (or new minimal interface). |
| `portal` | Use same API; id is chat_run_id; keep `artifact_id` in API response as chat_run_id so no type renames required. |

## 1. Database

**New table: `chat_run_output_file`** (singular)

- `chat_run_id` varchar(64) NOT NULL, index
- `relative_path` varchar(512) NOT NULL
- Primary key: `(chat_run_id, relative_path)` or add `id` and unique `(chat_run_id, relative_path)`. Use composite PK for simplicity.

**Dropped**

- Table `artifact`.
- Table `artifact_item` (replaced by `chat_run_output_file`).

**Chat table changes**

- Remove columns: `last_artifact_id`, `artifact_seq`.

**Migration strategy**

- GORM AutoMigrate does not drop tables/columns. Use a one-time migration (run from code or script):
  1. Create `chat_run_output_file` (AutoMigrate with new model).
  2. Copy data: `INSERT INTO chat_run_output_file (chat_run_id, relative_path) SELECT a.chat_run_id, i.relative_path FROM artifact_item i JOIN artifact a ON i.artifact_id = a.artifact_id`.
  3. Drop tables: `artifact_item`, `artifact`.
  4. Drop columns from `chat`: `last_artifact_id`, `artifact_seq`.
- In Store.New, after AutoMigrate, call a migration helper that checks a flag or schema version and runs the above if needed (or run once manually and remove Artifact/ArtifactItem from AutoMigrate).

**Ongoing AutoMigrate**

- Add `ChatRunOutputFile`; remove `Artifact`, `ArtifactItem`. Ensure `Chat` model no longer has `LastArtifactID`/`ArtifactSeq` so new deploys don’t recreate those columns (GORM will add new columns only; dropping must be done in the one-time migration).

## 2. Model (`internal/model/models.go`)

**Remove**

- `Artifact` struct and `TableName`.
- `ArtifactItem` struct and `TableName` (replaced by `ChatRunOutputFile`).

**Add**

- `ChatRunOutputFile`: `ChatRunID`, `RelativePath`; table name `chat_run_output_file`. JSON snake_case.

**Change**

- `Chat`: remove `ArtifactSeq`, `LastArtifactID`.
- `ArtifactWithChat`: rename to `RunOutputWithChat` or keep name; fields: `ChatRunID` (replaces ArtifactID), `ChatID`, `WorkspaceID`, `CreatedAt`, `ChatInputSnippet`. Drop `Seq` if not needed for ordering (ordering by run created_at); or keep a seq derived from run order.

**DTO for list**

- Keep API response shape: artifact_id → now holds chat_run_id. So `ArtifactWithChat` can keep field `ArtifactID string` in JSON for API compatibility but in code it is the chat_run_id (or rename to ChatRunID in DTO and map to artifact_id in JSON). Design decision: keep `ArtifactWithChat.ArtifactID` in JSON as `artifact_id` and assign it the value of chat_run_id so the frontend keeps using "artifact_id" as the id.

## 3. Entity store (`internal/storage/entity`)

**Interfaces (interfaces.go)**

- Remove `ArtifactStore` entirely.
- Extend `ChatRunStore` with:
  - `OnRunComplete(ctx, chatRunID string, relativePaths []string) error` — creates `chat_run_output_file` rows (one per path), updates chat denormalized from run (last_run_id, status, output, started_at, ended_at, error_message, session_id). No last_artifact_id or artifact_seq.
  - Optionally keep a small "run output" interface used by server: `ListRunOutputsByWorkspace(ctx, workspaceID string, chatID *string) ([]ArtifactWithChat, error)`, `GetChatRunOutputFiles(ctx, chatRunID string) ([]ChatRunOutputFile, error)`. Those can live on Store and be called from server; the server’s dependency on "ArtifactStore" becomes a dependency on Store with these two methods (Store already implements ChatRunStore, so add these to Store and have server take Store for artifact-like listing).

**Concrete**

- Store implements:
  - `ListRunOutputsByWorkspace(ctx, workspaceID, chatID *string) ([]ArtifactWithChat, error)` — query chat_run with status=SUCCEEDED joined to chat_run_output_file (exists) and chat for workspace_id; join chat_run for created_at, input snippet. Return DTOs with ArtifactID = ChatRunID, ChatID, WorkspaceID, CreatedAt, ChatInputSnippet.
  - `GetChatRunOutputFiles(ctx, chatRunID string) ([]ChatRunOutputFile, error)` — list rows from chat_run_output_file where chat_run_id = ?.
  - `OnRunComplete(ctx, chatRunID, relativePaths []string)` — in a transaction: get run; insert chat_run_output_file rows; update chat (last_run_id, status, output, started_at, ended_at, error_message, session_id from run). No artifact or last_artifact_id.
- Remove `CreateArtifactWithItem`, `ListArtifactsByWorkspace`, `GetArtifactByID`, `ListArtifactItems` and the old `OnRunComplete(ctx, chatRunID, artifactID, relativePaths)` from ChatRunStore.
- Remove artifact.go (or replace with run_output.go containing the new queries).
- Chat: remove IncrementChatSeq usage and the method if unused elsewhere.

**Types (entity/types.go)**

- Remove Artifact, ArtifactItem, ArtifactWithChat type aliases that point to model; add alias for ChatRunOutputFile. ArtifactWithChat stays in model as the list DTO (with artifact_id in JSON = chat_run_id).

## 4. Blob storage (`internal/storage/blob`)

**Key layout**

- Current: `ArtifactResultKey(prefix, workspaceID, chatID, chatRunID, artifactID)` → `.../artifacts/<chatID>/<chatRunID>/<artifactID>/result.md`.
- New: `.../artifacts/<chatID>/<chatRunID>/result.md` and `.../artifacts/<chatID>/<chatRunID>/<relPath>`.

**keys.go**

- `ArtifactResultKey(prefix, workspaceID, chatID, chatRunID string)` — return `path.Join(prefix, workspaceID, "artifacts", chatID, chatRunID, "result.md")`.
- `ArtifactFileKey(prefix, workspaceID, chatID, chatRunID, relPath string)` — return `path.Join(prefix, workspaceID, "artifacts", chatID, chatRunID, cleanRelPath)`.
- Remove artifactID parameter from both.

**ArtifactStorage interface (interfaces.go)**

- `PutResult(ctx, workspaceID, chatID, chatRunID string, data []byte) error`
- `GetResult(ctx, workspaceID, chatID, chatRunID string) ([]byte, error)`
- `PutArtifactFile(ctx, workspaceID, chatID, chatRunID, relPath string, r io.Reader) error`
- `GetArtifactFile(ctx, workspaceID, chatID, chatRunID, relPath string) ([]byte, error)`

**Implementations**

- `localfs_artifact.go`: artifactDir func becomes `func(workspaceID, chatID, chatRunID string) string`; PutResult writes to `dir/result.md`; GetResult reads `dir/result.md`; Put/GetArtifactFile use `dir/relPath`.
- `s3_artifact.go`: use new keys (no artifactID).
- `config`: ArtifactDir(workspaceID, chatID, chatRunID, artifactID) → RunOutputDir(workspaceID, chatID, chatRunID) or keep name with three args; signature `func(workspaceID, chatID, chatRunID string) string`.

**Config (internal/config)**

- Replace `ArtifactDir(workspaceID, chatID, chatRunID, artifactID string)` with `RunOutputDir(workspaceID, chatID, chatRunID string) string` returning `filepath.Join(WorkspacesDir(), workspaceID, "artifacts", chatID, chatRunID)`. Update all callers (setup.BuildArtifactStorage, executor paths, tests).

## 5. Executor (`internal/executor`)

**WorkspacePaths (paths.go)**

- Remove `ArtifactDir(workspaceID, chatID, chatRunID, artifactID string)`.
- Add `RunOutputDir(workspaceID, chatID, chatRunID string) string` if blob local FS needs a dir; or pass a func with three args from config.

**executor.go**

- `reportRunSuccess`: do not generate artifact_id. Call `artifactStorage.PutResult(ctx, workspaceID, chatID, run.ChatRunID, output)` and `uploadRunArtifactsToStorage(..., run.ChatRunID, ...)`. Build relativePaths from upload. Call `updater.UpdateRunStatus(ctx, run.ChatRunID, ..., &workerapi.ArtifactPayload{RelativePaths: relativePaths})` (no ArtifactID field).
- `uploadRunArtifactsToStorage`: signature drop artifactID; use (workspaceID, chatID, chatRunID); call PutResult and PutArtifactFile with three run identifiers only.

**ChatRunUpdater**

- Same interface; ArtifactPayload in the call no longer has ArtifactID (or it’s optional and ignored).

## 6. Worker API (`internal/workerapi/types.go`)

**ArtifactPayload**

- Remove `ArtifactID` field.
- Keep `RelativePaths []string` and optionally `RelativePath string` for backward compat (server can coalesce to RelativePaths).

**Server worker handler (worker_handlers.go)**

- On PATCH with status SUCCEEDED and Artifact payload: call `ChatRunStore.OnRunComplete(ctx, chatRunID, req.Artifact.RelativePaths)` (or coalesce RelativePath into RelativePaths). No artifact_id passed.

## 7. Server HTTP and config

**Handlers (artifacts.go)**

- `listWorkspaceArtifactsHandler`: call `Store.ListRunOutputsByWorkspace(ctx, workspaceID, chatIDPtr)`. Response shape unchanged; each item has artifact_id = chat_run_id, chat_id, workspace_id, created_at, seq (optional), chat_input_snippet.
- `listArtifactItemsHandler`: path param `artifact_id` is chat_run_id. Call `Store.GetChatRunOutputFiles(ctx, chatRunID)` and validate run belongs to workspace (get run → get chat → workspace_id). Return list of relative_path.
- `artifactContentHandler`: path param artifact_id is chat_run_id. Resolve run by chat_run_id; get chat and check workspace; then call ArtifactStorage.GetResult(ctx, workspaceID, chat.ChatID, chatRunID) or GetArtifactFile(ctx, workspaceID, chat.ChatID, chatRunID, pathParam). No artifactID in blob calls.

**Server config (server.go)**

- Remove `ArtifactStore` from config if list/get are on Store. Handlers use `s.cfg.Store` (or whatever exposes ListRunOutputsByWorkspace and GetChatRunOutputFiles). If we keep a separate interface for "run output listing", add e.g. `RunOutputLister` or keep reusing Store and pass Store for both ChatRunStore and "artifact" handlers.
- Keep `ArtifactStorage` (blob) with updated signature.

**servercmd (run.go)**

- Wire Store into server; remove ArtifactStore wiring. Ensure artifact handlers get Store and ArtifactStorage.

## 8. Portal (frontend)

- No URL or route changes. API response for list artifacts still has `artifact_id`; value is now chat_run_id. Frontend continues to use that id for GET .../artifacts/{id}/items and .../content. No type renames if backend keeps `artifact_id` in JSON.

## 9. How they work together

1. **Run succeeds (worker)**  
   Executor calls `artifactStorage.PutResult(workspaceID, chatID, chatRunID, output)` and `PutArtifactFile(..., chatRunID, relPath)` for each file (no artifactID). Then `updater.UpdateRunStatus(..., &ArtifactPayload{RelativePaths: relativePaths})`.

2. **Server PATCH handler**  
   On status SUCCEEDED and Artifact payload, calls `ChatRunStore.OnRunComplete(ctx, chatRunID, relativePaths)` which inserts `chat_run_output_file` rows and updates chat denormalized fields (no artifact table).

3. **List artifacts**  
   GET /api/workspaces/{id}/artifacts → Store.ListRunOutputsByWorkspace → returns runs with output, each with artifact_id = chat_run_id. Optional chat_id filter.

4. **Get artifact items / content**  
   GET .../artifacts/{id}/items → id = chat_run_id; Store.GetChatRunOutputFiles(chatRunID); return relative_path list.  
   GET .../artifacts/{id}/content?path=... → ArtifactStorage.GetResult or GetArtifactFile(workspaceID, chatID, chatRunID, path) (no artifactID).

5. **Blob keys**  
   All artifact blob keys use only (prefix,) workspaceID, chatID, chatRunID and relPath. No artifactID segment.

## 10. Changes for review

| Area | Files | Change |
|------|-------|--------|
| Model | `internal/model/models.go` | Remove Artifact, ArtifactItem. Add ChatRunOutputFile. Chat: remove LastArtifactID, ArtifactSeq. ArtifactWithChat: ArtifactID holds chat_run_id (or rename to ChatRunID and map to artifact_id in JSON). |
| Entity | `internal/storage/entity/types.go` | Remove Artifact, ArtifactItem aliases; add ChatRunOutputFile. |
| Entity | `internal/storage/entity/interfaces.go` | Remove ArtifactStore. Extend ChatRunStore: OnRunComplete(ctx, chatRunID, relativePaths). Add Store methods ListRunOutputsByWorkspace, GetChatRunOutputFiles. |
| Entity | `internal/storage/entity/artifact.go` | Remove or replace with run_output.go (ListRunOutputsByWorkspace, GetChatRunOutputFiles). |
| Entity | `internal/storage/entity/chat_run.go` | OnRunComplete(ctx, chatRunID, relativePaths): create chat_run_output_file rows; update chat from run; no artifact/last_artifact_id. Remove old OnRunComplete(artifactID, ...). |
| Entity | `internal/storage/entity/chat.go` | Remove IncrementChatSeq; remove artifact_seq/last_artifact_id updates. |
| Entity | `internal/storage/entity/store.go` | AutoMigrate: add ChatRunOutputFile; remove Artifact, ArtifactItem. One-time migration: copy artifact_item→artifact to chat_run_output_file; drop artifact, artifact_item; drop chat.last_artifact_id, chat.artifact_seq. |
| Blob | `internal/storage/blob/keys.go` | ArtifactResultKey(prefix, workspaceID, chatID, chatRunID); ArtifactFileKey(prefix, workspaceID, chatID, chatRunID, relPath). No artifactID. |
| Blob | `internal/storage/blob/interfaces.go` | ArtifactStorage: PutResult/GetResult/PutArtifactFile/GetArtifactFile without artifactID param. |
| Blob | `internal/storage/blob/localfs_artifact.go`, `s3_artifact.go` | Implement new interface; dir/key with three run args only. |
| Config | `internal/config/config.go` | ArtifactDir → RunOutputDir(workspaceID, chatID, chatRunID string). |
| Executor | `internal/executor/paths.go` | Remove ArtifactDir(..., artifactID); add RunOutputDir(workspaceID, chatID, chatRunID). |
| Executor | `internal/executor/executor.go` | reportRunSuccess: no artifact_id; use chat_run_id in blob and payload (RelativePaths only). uploadRunArtifactsToStorage(..., chatRunID). |
| Worker API | `internal/workerapi/types.go` | ArtifactPayload: remove ArtifactID. |
| Server | `internal/server/worker_handlers.go` | OnRunComplete(ctx, chatRunID, req.Artifact.RelativePaths). |
| Server | `internal/server/artifacts.go` | List: Store.ListRunOutputsByWorkspace. Items/Content: resolve by chat_run_id; Store.GetChatRunOutputFiles; ArtifactStorage.GetResult/GetArtifactFile(..., chatRunID, ...). |
| Server | `internal/server/server.go` | Remove ArtifactStore from config; artifact handlers use Store + ArtifactStorage. |
| Server | `internal/servercmd/run.go` | Wire Store for artifact handlers; remove ArtifactStore. |
| Setup | `internal/storage/setup/setup.go` | BuildArtifactStorage: use RunOutputDir (three args). |
| Tests | entity, executor, server helpers_test, artifacts_test | Replace ArtifactStore mocks with Store methods; use chat_run_id; update blob mocks (no artifactID). |

No new packages. Portal: optional type renames only if API renames artifact_id to chat_run_id (recommend keeping artifact_id in JSON as chat_run_id value).
