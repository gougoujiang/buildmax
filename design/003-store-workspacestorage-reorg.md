# Proposal: Reorganizing `internal/store` and `internal/workspacestorage`

## Current situation

| Package | Responsibility | Backend |
|--------|----------------|--------|
| **internal/store** | Persistence of **domain entities** (User, Workspace, Project, Task, Artifact, ArtifactItem). Structured data, relationships, metadata. | MySQL (GORM) |
| **internal/workspacestorage** | **Blob/file storage** for workspace content: (1) persist files (uploads, Explore), (2) artifact result files (e.g. `result.md`). | Local FS or S3/MinIO |

Both names suggest “where workspace-related data lives,” which makes the split (DB vs. file storage) unclear.

---

## Recommendation: Rename for clarity (minimal reorg)

**Goal:** Make the distinction obvious from package names: one is **entity/database**, the other is **file/blob storage**.

### Option A — Rename `workspacestorage` only (recommended)

- **Keep:** `internal/store` — already reads as “data store” and is used for DB-backed entity persistence.
- **Rename:** `internal/workspacestorage` → **`internal/blobstore`** (or **`internal/filestore`**).

**Rationale:**

- **store** = relational/entity store (MySQL).
- **blobstore** / **filestore** = object/file storage (local FS, S3). Common terms (e.g. “blob storage”) and clearly different from a SQL store.

**Pros:** Small change (one package rename + import updates). Clear naming.  
**Cons:** None significant.

---

### Option B — Rename both for symmetry

- **Rename:** `internal/store` → **`internal/db`** or **`internal/entitystore`**.
- **Rename:** `internal/workspacestorage` → **`internal/blobstore`** (or **`internal/filestore`**).

**Rationale:**

- **db** = database layer; **entitystore** = store of domain entities.
- **blobstore** = file/object storage.

**Pros:** Very explicit.  
**Cons:** Touches more call sites; `internal/db` is a bit generic; `entitystore` is longer.

---

### Option C — Merge under one parent, split by subpackage ✅ Implemented

Keep both concepts but group under a single top-level name:

- **`internal/storage/entity`** — current `store` (User, Workspace, Project, Task, Artifact; MySQL).
- **`internal/storage/blob`** — current `workspacestorage` (PersistStorage, ArtifactStorage; local/S3).

**Pros:** Single “storage” namespace; entity vs. blob is clear from path.  
**Cons:** Deeper paths; more files to move; `storage` is still a bit overloaded.

**Implementation (done):** All code from `internal/store` moved to `internal/storage/entity` (package `entity`). All code from `internal/workspacestorage` moved to `internal/storage/blob` (package `blob`). Imports updated across `internal/config`, `internal/cmd`, `internal/server`, `internal/executor`. Old packages removed.

---

### Option D — Rename by purpose (no merge)

- **Rename:** `internal/store` → **`internal/repository`** (or keep **`internal/store`**).
- **Rename:** `internal/workspacestorage` → **`internal/workspacefiles`** or **`internal/workspaceblob`**.

**Rationale:**

- **repository** = domain persistence (familiar DDD term).
- **workspacefiles** / **workspaceblob** = files/blobs scoped to workspace.

**Pros:** “Workspace” stays in the name for the file layer if you want that.  
**Cons:** “Workspacefiles” is still easy to confuse with “workspace (metadata)” in `store`.

---

## Summary table

| Option | store → | workspacestorage → | Scope |
|--------|--------|--------------------|--------|
| **A** | (unchanged) | `blobstore` or `filestore` | Rename one package |
| **B** | `db` or `entitystore` | `blobstore` or `filestore` | Rename both |
| **C** | `storage/entity` | `storage/blob` | Restructure under `storage` |
| **D** | `repository` or (unchanged) | `workspacefiles` / `workspaceblob` | Rename for purpose |

---

## Suggested choice

- **Option A:** Rename **`internal/workspacestorage`** to **`internal/blobstore`** (or **`internal/filestore`** if you prefer “file” over “blob”).  
- Leave **`internal/store`** as is.

This gives a clear, minimal naming split: **store** = entity/DB, **blobstore** (or **filestore**) = workspace file/object storage, with no structural merge and minimal code churn.

If you want the “workspace” concept in the name for the file layer, Option D (**`internal/workspacefiles`**) is a reasonable alternative; Option A is still clearer for “this is not the DB.”

---

*Option C was implemented: `internal/store` → `internal/storage/entity`, `internal/workspacestorage` → `internal/storage/blob`.*
