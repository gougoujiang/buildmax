# Team-Scoped Files / Upload Alignment

## Status

- status: `done`
- owners: `portal + storage + executor`
- related_roadmap: [design/010-team-task-workflow-roadmap.md](./010-team-task-workflow-roadmap.md)
- related_phase: [design/012-phase-2-team-foundation.md](./012-phase-2-team-foundation.md)
- related_designs:
  - [design/archive/009-user-scoped-ownership-refactor.md](./archive/009-user-scoped-ownership-refactor.md)
  - [design/003-store-workspacestorage-reorg.md](./003-store-workspacestorage-reorg.md)
- created_at: `2026-04-26`
- completed_at: `2026-04-26`

---

## 1. Goal

Align uploaded files and file browsing with the post-Phase-2 team ownership model.

After this change:

- uploaded files belong to a `team`
- file browsing reflects the current team
- task and workflow execution materialize the current team's files, not the triggering user's personal files
- personal usage still works through the default personal team (`My Space`)

This design is a boundary-alignment fix. It does not introduce a new product concept.

---

## 2. Problem

The roadmap says Team is now the ownership boundary for working resources, but the files/upload system is still implemented as a legacy user-scoped subsystem.

Today:

- portal upload and file routes are still top-level:
  - `POST /api/upload`
  - `GET /api/files`
  - `GET /api/files/{path...}`
- portal handlers authenticate to `user_id` only and do not resolve `team_id`
- persistent blob storage keys are partitioned by `user_id`
- executor materializes files by `task.CreatedBy`
- portal file UI does not carry `currentTeamId`

This creates inconsistent behavior in multi-team usage:

- switching teams does not change the visible files
- a shared team does not have a shared file namespace
- two members of the same team can trigger the same issue/workflow and get different runtime inputs
- upload/files no longer match the ownership model used by issues, agents, conversations, tasks, and workflows

---

## 3. Desired Outcome

The system should behave as if files are just another team-owned working resource.

Concretely:

- each team has one persistent `home/` file space
- the Portal file explorer reads and writes the current team's `home/`
- worker runtime materializes the task's team `home/` into the run directory
- personal users still see one simple file space through their personal team
- permission checks for file access follow team membership

### 3.1 Non-Goals

This design does not include:

- per-folder or per-file ACL
- file version history UI
- cross-team file sharing
- audit/event logging for file operations
- changing task-run artifacts ownership model

---

## 4. Core Decisions

### 4.1 Team Owns Persistent Files

Persistent uploaded files should move from `user` ownership to `team` ownership.

This means the persistent namespace becomes:

```text
team
└── home
    └── files...
```

instead of:

```text
user
└── home
    └── files...
```

### 4.2 Personal Space Remains A Team

No special case should be added for personal uploads.

The user's default personal team remains the storage owner for that user's private files.

This preserves the product rule introduced in Phase 2:

- personal usage is a single-member team
- no parallel ownership model should be reintroduced

### 4.3 Runtime Inputs Must Follow Task Team

Task and workflow execution must materialize files by `task.TeamID`, not `task.CreatedBy`.

This is the most important behavioral fix because it determines what the agent actually sees when it runs.

### 4.4 Team-Aware Routes Should Match Other Team Resources

Upload and file browsing routes should move under the explicit team path:

- `POST /api/teams/{team_id}/upload`
- `GET /api/teams/{team_id}/files`
- `GET /api/teams/{team_id}/files/{path...}`

This keeps the API consistent with conversations, issues, agents, workflows, and tasks.

### 4.5 No Dual Ownership Long Term

We should not keep both:

- user-scoped persist files
- team-scoped persist files

as permanent first-class models.

If temporary compatibility read/write behavior is needed during migration, it should be explicitly transitional.

---

## 5. Proposed Plan

Implementation should happen in the following order.

### Step 1. Introduce Team-Scoped Persist Boundary

Change the persistent storage abstraction from user-keyed operations to team-keyed operations.

Current shape:

- `Put(ctx, userID, relPath, ...)`
- `Get(ctx, userID, relPath)`
- `ListFiles(ctx, userID)`
- `MaterializeToDir(ctx, userID, dstDir)`

Target shape:

- `Put(ctx, teamID, relPath, ...)`
- `Get(ctx, teamID, relPath)`
- `ListFiles(ctx, teamID)`
- `MaterializeToDir(ctx, teamID, dstDir)`

Recommended implementation note:

- rename parameters and comments to `teamID`
- keep method names unchanged if that minimizes churn
- update all call sites in portal and executor in the same change

### Step 2. Move Portal Upload / Files Routes To Team Scope

Replace top-level routes with team-scoped routes:

- `POST /api/teams/{team_id}/upload`
- `GET /api/teams/{team_id}/files`
- `GET /api/teams/{team_id}/files/{path...}`

Handler behavior:

- require auth
- resolve explicit `team_id`
- verify membership through existing team auth helpers
- read/write files under the resolved team namespace

Recommended compatibility policy:

- do not keep old top-level routes unless migration pressure requires it
- if temporary compatibility is needed, keep them only as short-lived shims to the user's personal team

### Step 3. Make Portal File UI Carry Current Team

Thread `currentTeamId` through the frontend file APIs and hooks.

Target changes:

- `uploadFiles(..., teamId, token, ...)`
- `getFileTree(teamId, token)`
- `getFileContent(teamId, filePath, token)`
- `FilesExplorer` reads `currentTeamId` from `TeamContext`

Expected behavior:

- switching current team changes the file tree
- uploads land in the visible team's namespace
- `Explore` and the new-conversation file panel stay consistent with the selected team

### Step 4. Materialize Team Files In Worker Runtime

Change executor runtime materialization from:

- `persist.MaterializeToDir(..., task.CreatedBy, ...)`

to:

- `persist.MaterializeToDir(..., task.TeamID, ...)`

This ensures:

- issue-triggered agent runs use team files
- workflow runs use team files
- conversation-created tasks use team files

### Step 5. Migrate Persistent Storage Layout

Update local FS and object-storage key layout to use `team_id`.

Recommended target layout:

- local FS persist root: team-scoped path from the configured root function
- object storage key prefix: `<prefix>/<team_id>/home/...`

This design intentionally keeps the visible `home/` concept unchanged.

### Step 6. Add Tests And Validation

Add backend and frontend coverage for:

- team route auth
- file tree isolation between teams
- upload visibility within the same team
- executor materialization using `task.TeamID`
- portal team switching behavior

---

## 6. Detailed Design

## 6.1 API Changes

### New canonical routes

```text
POST /api/teams/{team_id}/upload
GET  /api/teams/{team_id}/files
GET  /api/teams/{team_id}/files/{path...}
```

### Request behavior

- all routes require Bearer auth
- `team_id` must resolve through existing team membership checks
- path validation stays the same:
  - no absolute paths
  - no `..`
  - folder upload still uses `paths[]`

### Response behavior

No product-level response shape change is required.

Keep:

- upload response: `{ "uploaded": string[] }`
- file tree response: current tree structure
- file content response: plain text body

### Compatibility

Recommended default:

- remove old top-level file routes from Portal API registration

Acceptable transitional alternative:

- keep old routes temporarily and resolve them to the caller's personal team only
- mark them deprecated in comments and OpenAPI

The preferred option is removal because the team model is already live.

## 6.2 Storage Changes

### Persist storage contract

The contract should conceptually become team-scoped, even if the interface name remains `PersistStorage`.

Comments and naming should be updated from:

- user files
- user persist root

to:

- team files
- team persist root

### Key layout

Current object layout:

```text
<prefix>/<user_id>/home/<relPath>
```

Target object layout:

```text
<prefix>/<team_id>/home/<relPath>
```

### Local filesystem layout

Current local behavior is effectively rooted by `config.PersistentWorkspaceDir(userID)`.

Target behavior should root by team identifier instead.

Recommended naming:

- keep `home/` inside the team root for conceptual consistency
- avoid reintroducing `workspace` terminology

## 6.3 Executor Changes

Executor currently materializes persistent files using the creator identity.

That should change to:

- resolve runtime inputs from `task.TeamID`
- keep run directory structure otherwise unchanged

This design does not require changing task-run artifact storage in the same step.

Artifacts are already run-scoped and do not have the same collaboration bug as persistent inputs.

## 6.4 Frontend Changes

Portal files UI should adopt the same team-aware calling convention already used by:

- conversations
- issues
- workflows
- agents

Affected surfaces:

- `Explore`
- new conversation page file panel
- any future upload picker that reuses the files feature module

UI copy can stay mostly unchanged, though "Browse your files" should be reconsidered later if we want the wording to reflect team ownership more clearly.

---

## 7. Migration Strategy

## 7.1 Data Model Reality

Persistent files are blob-backed, not DB rows, so migration is primarily a storage-key migration.

That means the main migration choices are:

1. hard cutover with no compatibility
2. compatibility read from old user keys, write to new team keys
3. one-time copy/migration job

## 7.2 Recommended Migration Approach

Recommended approach:

- short-lived compatibility migration

Implementation shape:

1. write all new uploads to team-scoped keys
2. when reading team files for a personal team, optionally fall back to legacy user-scoped keys if the team namespace is empty
3. provide a one-off migration helper or script later if needed
4. remove fallback after the migration window

Reason:

- current development stage is still early
- this avoids blocking the code correction on a full migration utility
- it reduces the chance of confusing empty file spaces for existing users

## 7.3 Shared Team Semantics During Migration

Fallback must be used carefully.

Important rule:

- legacy fallback is only acceptable for a personal team that maps to the same user
- legacy fallback must not make one user's old files automatically appear in a newly shared multi-member team

So for non-personal teams:

- no legacy user fallback
- team namespace only

---

## 8. Validation

The implementation is acceptable when all of the following are true.

### Backend acceptance

- team-scoped upload/files routes exist and pass authz checks
- two different teams for the same user can have different file trees
- two members of the same team see the same uploaded files
- file reads/writes reject access when the user is not a team member

### Runtime acceptance

- a task created in a team run sees the team's uploaded files in `home/`
- two different members triggering the same team issue/workflow get the same team file inputs

### Frontend acceptance

- switching current team refreshes the file tree
- uploading in Team A does not show up in Team B
- uploading in a shared team is visible to another team member

### Regression acceptance

- default personal team still behaves like the old single-user experience
- folder upload path handling still works
- path sanitization behavior remains unchanged

---

## 9. Open Questions

### 9.1 Should webhook-triggered runs use team files later?

This design does not change webhook key ownership yet.

If webhook execution becomes team-scoped later, it should use the same team-owned file namespace.

### 9.2 Do we want user-facing wording to say "team files"?

The backend ownership should become team-scoped now.

UI wording can stay simple for the first pass, but later we may want:

- personal team: "Files"
- shared team: "Team Files"

### 9.3 Should old top-level routes survive briefly?

Preferred answer:

- no, unless migration friction appears immediately

If they survive temporarily, they should be explicitly deprecated and mapped only to the personal team.

---

## 10. Recommended Execution Split

To keep implementation risk manageable, execute in two PR-sized steps.

### Slice A

- backend route changes
- persist storage contract change
- executor team materialization
- backend tests

### Slice B

- portal file API team threading
- team-switch refresh behavior
- frontend build/test validation

If the repo is quiet, these can also land together in one focused change.
