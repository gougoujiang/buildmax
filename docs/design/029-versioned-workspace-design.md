# Versioned Workspace Design

## Status

- roadmap_priority: `P5`
- status: `ready_for_review`
- follows: [028-team-governance-foundation.md](./028-team-governance-foundation.md)
- roadmap: [../ROADMAP.md](../../ROADMAP.md)
- created_at: `2026-05-17`

## 1. Decision

P5 should define the minimum versioned workspace model before broad
implementation begins.

This is the long-term product center from [001-about-portal.md](./001-about-portal.md):

- user expresses intent
- agent changes workspace state
- system explains what changed
- user can go back

The first design must stay small. It should derive from the worker layout that
already exists instead of introducing a second workspace engine.

## 2. Product Goal

Users should feel:

> BuildMax changed my team's workspace, showed me what changed, and I can
> restore a previous state.

They should not need to know:

- Git commits
- branch names
- object-store keys
- run directories
- internal task IDs

Git can be the hidden version engine, but the product surface is:

- snapshots
- changes
- timeline
- restore
- result/output links

## 3. Current Baseline

Current storage and run model:

- each team has a persistent file space, materialized as `home/`
- each task run has:
  - `home/`: materialized team home
  - `artifacts/`: produced files such as `result.md`
  - `global/`: run-scoped `BUILDMAX_HOME`
- worker materializes team files before a run
- worker uploads artifacts and run global state
- Portal has team-scoped file browsing
- object storage supports local FS and MinIO/S3

Relevant code:

- `internal/agentapp/taskrun/runtime.go`
- `internal/agentapp/taskrun/paths.go`
- `internal/infra/objectstore`
- `internal/server/handlers/files.go`
- `internal/server/handlers/artifacts.go`
- `internal/service/task`
- `internal/core/model/task.go`

Important constraint:

- P5 follows P2. Results and outputs should be visible first, so workspace
  versioning can attach to meaningful user outcomes instead of raw runs.

## 4. Main Gaps

### 4.1 No Durable Snapshot Boundary

The system can materialize and persist team files, but it does not yet define a
product-level snapshot:

- before a run
- after a run
- user upload state
- restore target

### 4.2 No Workspace Change Model

The system does not yet record a durable summary like:

- files added
- files modified
- files deleted
- semantic summary
- producing task/run/issue

### 4.3 No Restore Model

Users can view files and artifacts, but cannot restore team workspace state to a
known previous version.

### 4.4 Git Is Not Yet Formalized As Hidden Engine

The vision says Git should be hidden behind product concepts. P5 needs to
define how Git maps to snapshots and changes without exposing Git UI.

## 5. In Scope

### 5.1 Workspace Snapshot Model

Define a durable snapshot entity for team workspace state.

Recommended model:

```go
type WorkspaceSnapshot struct {
	ID             uint    `json:"-"`
	SnapshotID     string  `json:"snapshot_id"`
	TeamID         string  `json:"team_id"`
	SourceType     string  `json:"source_type"`
	SourceID       string  `json:"source_id"`
	GitCommit      string  `json:"git_commit,omitempty"`
	Summary        string  `json:"summary"`
	CreatedBy      string  `json:"created_by"`
	CreatedAt      int64   `json:"created_at"`
}
```

Use prefix `ws_` or `snap_` after deciding the entity naming convention. The
existing ID guidance allows new short prefixes when needed.

Source types:

- `upload`
- `task_run`
- `restore`
- `manual`

### 5.2 Workspace Change Model

Define a durable change entity attached to a snapshot.

Recommended model:

```go
type WorkspaceChange struct {
	ID          uint   `json:"-"`
	ChangeID    string `json:"change_id"`
	SnapshotID  string `json:"snapshot_id"`
	TeamID      string `json:"team_id"`
	Path        string `json:"path"`
	ChangeType  string `json:"change_type"`
	OldHash     string `json:"old_hash,omitempty"`
	NewHash     string `json:"new_hash,omitempty"`
	Summary     string `json:"summary,omitempty"`
	CreatedAt   int64  `json:"created_at"`
}
```

Change types:

- `added`
- `modified`
- `deleted`
- `renamed` later, if needed

### 5.3 Snapshot Timing

Create snapshots at clear boundaries:

- initial team home creation
- after upload/import changes
- before task run starts, if needed for restore safety
- after task run succeeds and team home changes are persisted
- after restore completes

The MVP can start with post-change snapshots only, plus the previous snapshot
reference needed for diffs.

### 5.4 Hidden Git Engine

Use Git internally for:

- content versioning
- diff calculation
- restore

Do not expose:

- commits
- branches
- staging
- merge conflicts

Product concepts:

- snapshot
- change
- restore point
- timeline event

### 5.5 Restore Design

Define restore before building broad UI.

MVP restore behavior:

- admin/owner selects a snapshot
- system creates a restore operation
- restore writes team home back to snapshot state
- restore creates a new snapshot with source type `restore`
- timeline records who restored what and when
- current files reflect restored state

Do not mutate history. Restore is a new state.

## 6. Out Of Scope

- Collaborative live editing.
- Branch UI.
- User-managed Git remotes.
- Merge conflict UI.
- Partial file restore in first slice.
- Binary diff rendering.
- Large semantic summarization pipeline.
- Full approval workflow.

## 7. Product Surface

### 7.1 Issue Detail

After P2, issue detail has `Results`. P5 can add:

- workspace changes produced by this issue
- link to snapshot
- changed files
- restore point when applicable

### 7.2 Conversation Detail

Conversation can show:

- output cards
- workspace changes caused by background tasks
- "changed files" summary for completed runs

### 7.3 Files / Explore

Team files page should eventually show:

- current workspace state
- last changed time
- last changed by/source
- timeline link

### 7.4 Workspace Timeline

Add a team workspace timeline:

- upload/import snapshots
- task-run snapshots
- restore snapshots
- file-change summaries

This timeline is not a Git log. It is a product narrative.

## 8. Storage Design

### 8.1 Repository Location

For local FS:

```text
<workspaces_dir>/<team_id>/repo/
<workspaces_dir>/<team_id>/home/
```

For MinIO/S3:

- continue storing team home blobs through `PersistStorage`
- store Git pack/repo state either:
  - in a local server-managed volume, or
  - as object-store-backed snapshot archives

P5 should decide this explicitly. The first implementation may use local server
repo storage for simplicity, but production deployment must account for
persistence and backup.

### 8.2 Source Of Truth

Recommended long-term:

- Git repo is the version engine
- object storage remains the team home/blob distribution layer
- snapshots connect DB records to Git commits

When a worker finishes:

1. worker writes run artifacts
2. worker persists any intended workspace changes
3. server records or imports new workspace state
4. server creates snapshot and change records

### 8.3 Write Ownership

Avoid letting multiple components mutate team home without a snapshot boundary.

Recommended rule:

- server owns committing workspace state
- worker can produce changes in run `home/`
- server or a workspace service applies accepted changes to team home and
  creates snapshot

If current worker already persists directly, P5 should either:

- wrap that direct persist with snapshot creation, or
- move persist-finalization into a server-side workspace service

## 9. Backend Plan

### M1. Workspace Version Service Design Skeleton

Add a service boundary:

```go
type WorkspaceVersionService struct {
	Snapshots WorkspaceSnapshotStore
	Changes   WorkspaceChangeStore
	Persist   blob.PersistStorage
}
```

Initial methods:

- `CreateSnapshotForTeam`
- `ListSnapshots`
- `GetSnapshot`
- `ListChanges`
- `RestoreSnapshot`

Implementation can be stubbed behind the interface until Git plumbing lands.

### M2. Persistence Contracts

Add model/store contracts in `internal/core/model`:

- `WorkspaceSnapshot`
- `WorkspaceChange`
- `WorkspaceSnapshotStore`
- `WorkspaceChangeStore`

Add GORM rows:

- `workspace_snapshot`
- `workspace_change`

Keep table names singular.

### M3. Snapshot Creation For Uploads

Start with uploads because they are simpler than agent changes.

After team upload succeeds:

- create a snapshot
- record added/modified files if known
- show snapshot in timeline

Acceptance:

- user upload creates visible workspace history

### M4. Snapshot Creation For Task Runs

After task run succeeds:

- compare previous team home state with new state
- create snapshot
- record changed paths
- connect snapshot to task run, task, issue, and conversation where possible

Acceptance:

- completed agent work produces output plus workspace change history

### M5. Restore MVP

Add restore endpoint:

```text
POST /api/teams/{team_id}/workspace/snapshots/{snapshot_id}/restore
```

Authorization:

- owner/admin only for MVP

Behavior:

- restore whole workspace to selected snapshot
- create new restore snapshot
- record event if P4 team events exist

## 10. Frontend Plan

### M1. Workspace Timeline

Add a timeline section to Files or Team Settings:

- snapshot summary
- source
- changed file count
- created time
- actor

### M2. Changed Files View

Add a snapshot detail panel:

- list changed files
- change type badge
- source link
- result/output link where available

### M3. Issue/Conversation Change Cards

After task-run snapshots exist:

- show changed files near issue Results
- show changed files near conversation output cards

### M4. Restore UI

Add restore action:

- owner/admin only
- confirmation modal
- clear warning that restore creates a new current state
- success state links to new snapshot

## 11. Validation

Backend:

```sh
go test ./internal/service/... ./internal/server/handlers ./internal/infra/db ./internal/infra/objectstore
```

Frontend:

```sh
cd portal && npm run build
```

Full:

```sh
./make test
```

Manual scenarios:

1. Upload file to team home.
2. Snapshot appears in workspace timeline.
3. Run an agent task that changes workspace files.
4. Snapshot records changed files.
5. Issue/conversation links to the workspace change.
6. Restore an older snapshot as owner/admin.
7. Current file tree reflects restored state.
8. Restore creates a new timeline entry.

## 12. Risks

- **Source-of-truth confusion**: decide whether Git repo or object-store home is
  authoritative at each step.
- **Concurrent writes**: task runs and uploads may race; snapshot service needs
  a lock or transaction boundary.
- **Large workspaces**: naive full snapshot diffs may be expensive.
- **Binary files**: MVP should record path/hash changes, not render binary diffs.
- **Restore surprises**: restore must be explicit, permissioned, and create a
  new state rather than rewriting history.
- **Worker/server split**: direct worker persistence may bypass server snapshot
  creation unless wrapped.

## 13. Open Questions

1. Should the server own applying worker workspace changes, or should the worker
   persist and notify the server to snapshot?
2. Should the first Git repo live on local disk, in object storage, or both?
3. What lock prevents simultaneous upload/task-run writes for the same team?
4. Should restore be owner-only or owner/admin?
5. Should P5 include semantic summaries generated by LLM, or only file-level
   summaries in the first implementation?
6. Should snapshot IDs use `snap_`, `ws_`, or another prefix?

## 14. Recommended First PR

The first P5 PR should be design-to-code scaffolding, not full restore:

1. Add workspace snapshot/change model contracts.
2. Add DB rows and store methods.
3. Create snapshots for team uploads.
4. Add a read-only workspace timeline endpoint.
5. Add a simple Portal timeline section.

That proves the product language and persistence shape before task-run diffs
and restore make the problem sharper.
