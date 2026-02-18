# Design 062: Id solution refine

## Goal

Use **ULID** for task and artifact IDs (time-ordered) and **uppercase base36** for user, workspace, and project IDs. All IDs remain strings; no API or schema change.

## Modules

| Module (package) | Change |
|------------------|--------|
| **internal/util** | Add `NewULID() string` (26-char ULID). Change `NewID()` to use uppercase base36 alphabet (`0-9A-Z`). Add tests for both. |
| **internal/storage/entity** | `CreateTask`: set `TaskID` with `util.NewULID()` instead of `util.NewID()`. No interface change. |
| **internal/executor** | Where `artifactID := util.NewID()` is used, use `util.NewULID()`. |
| **go.mod / go.sum** | Add dependency `github.com/oklog/ulid/v2`. |

## Structure

**Files to modify**

- **internal/util/id.go**
  - Change `idAlphabet` from `"0123456789abcdefghijklmnopqrstuvwxyz"` to `"0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"`.
  - Update `NewID` doc comment to state it is for non–time-ordered entities (user, workspace, project) and produces 25-char uppercase base36.
  - Add `NewULID() string`: use `ulid.MustNew(ulid.Now(), rand.Reader)` (or equivalent from `oklog/ulid/v2`), return `.String()`. Add package doc or comment that `NewULID` is for time-ordered entities (task, artifact).
- **internal/storage/entity/task.go**
  - In `CreateTask`, replace `TaskID: util.NewID()` with `TaskID: util.NewULID()`.
- **internal/executor/executor.go**
  - Replace `artifactID := util.NewID()` with `artifactID := util.NewULID()`.
- **go.mod / go.sum**
  - `go get github.com/oklog/ulid/v2` (or add require in go.mod and run `go mod tidy`).

**Tests**

- **internal/util/id_test.go** (new or extend existing): (1) `TestNewID` — assert length 25, all chars in `0-9A-Z`. (2) `TestNewULID` — assert length 26, valid ULID character set (Crockford base32: 0-9, A-Z excluding I/L/O/U), or use library’s parse to validate. No tests in entity or executor assert ID format today; entity tests use a fixed `artifactID` for CreateArtifactWithItem, so no change there.

## Method design

| Receiver / Scope | Symbol | Responsibility |
|------------------|--------|----------------|
| util | **NewID() string** | Generate 25-char uppercase base36 ID (UUID v4 → big.Int → base36). For user, workspace, project. |
| util | **NewULID() string** | Generate 26-char ULID via oklog/ulid/v2. For task_id and artifact_id (time-ordered). |
| entity.Store | **CreateTask** | Set `TaskID: util.NewULID()`; rest unchanged. |
| executor | **RunTask** (artifact ID) | Set `artifactID := util.NewULID()` before PutResult and UpdateTaskStatus. |

## How they work together

- **User / Workspace / Project creation**: Unchanged call sites (user.go, workspace.go, project.go) keep calling `util.NewID()`. They now receive uppercase base36 IDs.
- **Task creation**: Server or internal callers call `entity.CreateTask`; Store assigns `TaskID` via `util.NewULID()`. API and DB still see string `task_id`; value format becomes ULID.
- **Artifact creation (executor)**: After a task run, executor generates `artifactID := util.NewULID()`, writes to artifact storage, then calls `UpdateTaskStatus(..., ArtifactPayload{ArtifactID: artifactID, ...})`. Server persists artifact via existing flow; artifact_id format becomes ULID.

No change to HTTP routes, JSON field names, or DB column types (varchar(64) fits both 25-char base36 and 26-char ULID).

## ULID dependency

- **Library**: `github.com/oklog/ulid/v2`.
- **Usage**: `ulid.MustNew(ulid.Now(), rand.Reader)` (or `crypto/rand`). Return `ulid.ULID.String()` (26 chars, Crockford base32).

## Changes for review

| Item | Change |
|------|--------|
| **internal/util/id.go** | `idAlphabet` → uppercase; doc for NewID; add `NewULID() string` using oklog/ulid/v2. |
| **internal/util/id_test.go** | New file (or add to existing): TestNewID (len 25, charset 0-9A-Z), TestNewULID (len 26, valid ULID). |
| **internal/storage/entity/task.go** | CreateTask: `TaskID: util.NewULID()`. |
| **internal/executor/executor.go** | `artifactID := util.NewULID()`. |
| **go.mod / go.sum** | Add `github.com/oklog/ulid/v2`. |
