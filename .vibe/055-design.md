# Design 055 - Alternative workspace persist: MinIO

## Goal

Introduce pluggable storage backends for **persistent workspace files** and **artifact files** so BuildMax can use either **local filesystem** (current) or **S3-compatible object storage (MinIO now, AWS S3 later)**, selected via env vars, while keeping **task runtime workspace** on local disk.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/workspacestorage** | Storage interfaces + implementations for persist + artifact files | Interfaces, local-fs impl, S3 impl (MinIO compatible), path/key mapping helpers |
| **internal/config** | Parse env config and construct storage implementations | `WorkspaceStorageConfig`, `LoadWorkspaceStorageConfig()`, factory functions |
| **internal/server** | HTTP handlers delegate file ops to persist/artifact storage | Upload + Explore use PersistStorage; artifact content uses ArtifactStorage |
| **internal/executor** | Executor uses PersistStorage to prepare runtime dir, ArtifactStorage to write artifact files | Copy-in from persist; write artifact result to selected backend |
| **internal/cmd** | Wire server + executor with selected providers | Construct providers from config and inject into server/executor |

## Structure

**Directory / files**

- `internal/workspacestorage/` — backend-agnostic workspace file storage
  - `interfaces.go` — core interfaces and small DTOs
  - `relpath.go` — relative-path validation + normalization (reject traversal)
  - `localfs_persist.go` — filesystem persist implementation
  - `localfs_artifact.go` — filesystem artifact implementation
  - `s3_client.go` — small wrapper interface around the AWS S3 client (for test fakes)
  - `s3_persist.go` — S3/MinIO persist implementation
  - `s3_artifact.go` — S3/MinIO artifact implementation
  - `keys.go` — bucket key layout helpers (prefix + workspace/task/artifact mapping)
- `internal/config/workspace_storage.go` — env parsing + provider factory
- `internal/server/` — modified handlers to use storage abstractions instead of direct FS operations
- `internal/executor/` — modified executor to call storage abstractions

**Main types and interfaces**

- **PersistStorage** (`internal/workspacestorage`): read/write/list workspace persistent files and materialize them into a local dir for execution.
- **ArtifactStorage** (`internal/workspacestorage`): write/read artifact content files (initially: `result.md` only).
- **S3Client** (`internal/workspacestorage`): minimal subset of S3 operations used by the S3 implementations.
- **WorkspaceStorageConfig** (`internal/config`): provider selection + S3/MinIO connection config.

## Method design

### internal/workspacestorage

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| (package) | CleanRelPath | `(p string) (string, error)` | Normalize/validate relative paths: reject `""`, absolute paths, `..` traversal, backslashes; return slash-separated clean path. |
| **PersistStorage** | Put | `(ctx context.Context, workspaceID string, relPath string, r io.Reader) error` | Write/overwrite one persistent file at `relPath`. |
| **PersistStorage** | Get | `(ctx context.Context, workspaceID string, relPath string) ([]byte, error)` | Read one persistent file; error should distinguish not-found. |
| **PersistStorage** | ListFiles | `(ctx context.Context, workspaceID string) ([]string, error)` | Return all file relative paths under persistent workspace (files only; dirs derived by server). |
| **PersistStorage** | MaterializeToDir | `(ctx context.Context, workspaceID string, dstDir string) error` | Download/copy all persistent files into `dstDir` (used by executor before run). Missing workspace is treated as empty. |
| **ArtifactStorage** | PutResult | `(ctx context.Context, workspaceID, taskID, artifactID string, data []byte) error` | Write the artifact result file as `result.md` for the given artifact. |
| **ArtifactStorage** | GetResult | `(ctx context.Context, workspaceID, taskID, artifactID string) ([]byte, error)` | Read `result.md` content for the artifact. |
| (package) | PersistObjectKey | `(prefix, workspaceID, relPath string) (string, error)` | Map to S3 key: `<prefix>/<workspace_id>/persist/<relPath>` (uses CleanRelPath). |
| (package) | ArtifactResultKey | `(prefix, workspaceID, taskID, artifactID string) string` | Map to S3 key: `<prefix>/<workspace_id>/artifacts/<task_id>/<artifact_id>/result.md`. |

**Local filesystem implementations**

- `localFSPersistStorage` holds `persistRoot func(workspaceID string) string`
  - `Put/Get/ListFiles/MaterializeToDir` operate under `persistRoot(workspaceID)`
  - `MaterializeToDir` reuses a private `copyDirContents(src, dst) error` similar to current executor `copyWorkspaceContents`
- `localFSArtifactStorage` holds `artifactDir func(workspaceID, taskID, artifactID string) string`
  - `PutResult` ensures dir exists and writes `result.md`
  - `GetResult` reads `result.md`

**S3 (MinIO/AWS) implementations**

- Use **AWS SDK for Go v2** S3 client (`github.com/aws/aws-sdk-go-v2/service/s3`) because:
  - It works with **AWS S3** and **S3-compatible** endpoints (MinIO) by configuration.
  - It supports standard S3 APIs we need: `PutObject`, `GetObject`, `ListObjectsV2`.
- `s3PersistStorage` holds: `client S3Client`, `bucket string`, `prefix string`
  - `Put`: `PutObject(bucket, key, body)` with key from `PersistObjectKey`
  - `Get`: `GetObject(bucket, key)` read all bytes
  - `ListFiles`: list objects under prefix `<prefix>/<workspace_id>/persist/`; return rel paths by stripping that base
  - `MaterializeToDir`: list + download each file; write to `dstDir/<relPath>` (create parent dirs)
- `s3ArtifactStorage` holds: `client S3Client`, `bucket string`, `prefix string`
  - `PutResult`: `PutObject` to `ArtifactResultKey(...)`
  - `GetResult`: `GetObject` from that key

### internal/config

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| (package) | LoadWorkspaceStorageConfig | `() WorkspaceStorageConfig` | Read env vars for provider selection + S3/MinIO settings and apply defaults. |
| (package) | BuildPersistStorage | `(cfg WorkspaceStorageConfig, persistRoot func(workspaceID string) string, s3 S3Client) (workspacestorage.PersistStorage, error)` | Construct selected persist storage implementation. |
| (package) | BuildArtifactStorage | `(cfg WorkspaceStorageConfig, artifactDir func(workspaceID, taskID, artifactID string) string, s3 S3Client) (workspacestorage.ArtifactStorage, error)` | Construct selected artifact storage implementation. |
| (package) | BuildS3Client | `(cfg WorkspaceStorageConfig) (S3Client, error)` | Build an AWS SDK v2 S3 client compatible with MinIO (custom endpoint) and AWS S3 (no endpoint override). |

Notes:

- Provider selection env vars (per task spec):
  - `BUILDMAX_PERSIST_STORAGE=local_fs|minio` (default `local_fs`)
  - `BUILDMAX_ARTIFACT_STORAGE=local_fs|minio` (default `local_fs`)
- “minio” provider is implemented as **S3 API client** with custom endpoint (so it naturally extends to AWS S3 later).

### internal/server

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| **Server** | uploadHandler | *(existing)* | For each uploaded file, call `PersistStorage.Put(...)` instead of `os.Create(...)` (still validate rel paths). |
| **Server** | filesTreeHandler | *(existing)* | Call `PersistStorage.ListFiles(...)`, then build `fileNode` tree in-memory from returned relative file paths. |
| **Server** | fileContentHandler | *(existing)* | Call `PersistStorage.Get(...)` for the requested rel path and return bytes. |
| **Server** | artifactContentHandler | *(existing)* | Call `ArtifactStorage.GetResult(...)` and return bytes as `text/markdown`. |

Server config additions:

- Add to `internal/server/server.go` `Config`:
  - `PersistStorage workspacestorage.PersistStorage`
  - `ArtifactStorage workspacestorage.ArtifactStorage`
- Local filesystem fallback: if these are nil, server may build localfs implementations from `WorkspacesDir` as today (or treat nil as misconfig; prefer explicit injection in `cmd`).

### internal/executor

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| **Runner** | executeTask | *(existing)* | Replace `copyWorkspaceContents(persistDir, runtimeDir)` with `PersistStorage.MaterializeToDir(ctx, workspaceID, runtimeDir)`. Replace local artifact file copy with `ArtifactStorage.PutResult(ctx, workspaceID, taskID, artifactID, outputBytes)` (best-effort). |

Executor constructor changes:

- Extend `executor.New(...)` to accept:
  - `persist workspacestorage.PersistStorage`
  - `artifactFiles workspacestorage.ArtifactStorage`
- Keep existing DB `ArtifactStore` for rows/metadata (unchanged).
- Keep `WorkspacePaths` for runtime dir resolution (still filesystem).

## How they work together

**Data/control flow**

1. **Startup (cmd/server.go)**:
   - `cfg := config.LoadWorkspaceStorageConfig()`
   - `s3 := config.BuildS3Client(cfg)` (only needed when either provider is `minio`)
   - Build:
     - `persistStorage := config.BuildPersistStorage(cfg, serverPersistRootFunc, s3)`
     - `artifactStorage := config.BuildArtifactStorage(cfg, serverArtifactDirFunc, s3)`
   - Inject these into `server.Config` and `executor.New(...)`.
2. **Upload (server)**:
   - For each multipart file: validate `relPath` then `PersistStorage.Put(ctx, workspaceID, relPath, fileReader)`.
3. **Explore tree/content (server)**:
   - Tree: `ListFiles` → build nested `fileNode` structure → JSON response.
   - Content: `Get` → return bytes.
4. **Task execution (executor)**:
   - Create `runtimeDir` (local).
   - Pre-run: `PersistStorage.MaterializeToDir(ctx, workspaceID, runtimeDir)`.
   - Run `buildmax` with `cmd.Dir = runtimeDir`.
   - Artifact: create DB rows via `ArtifactStore.CreateArtifactWithItem` as now; write artifact file via `ArtifactStorage.PutResult(...)` (best-effort).

**Dependencies**

- `internal/server` depends on `internal/workspacestorage` for interfaces.
- `internal/executor` depends on `internal/workspacestorage` for interfaces.
- `internal/workspacestorage` does **not** depend on `internal/server` or `internal/executor`.
- `internal/config` depends on `internal/workspacestorage` (to build implementations) and AWS SDK v2 (for S3 client creation).

**Key data structures**

- `WorkspaceStorageConfig`: provider selection + S3 endpoint/bucket/prefix/creds.
- `PersistStorage` / `ArtifactStorage`: stable boundaries so future AWS S3 support is config-only (no code changes beyond env naming/aliasing).

## Edge cases / notes

- **Empty directories**: S3 has no true empty directories; Explore tree will show folders only if there is at least one file under them. Local FS provider can preserve empties, but for consistency the server tree should be built from the file list (empties may disappear). If this is undesirable, we can keep the current filesystem walk for `local_fs` only.
- **Path safety**: All rel paths from HTTP must be validated via `CleanRelPath` before becoming filesystem paths or S3 keys.
- **Large files**: initial implementation may buffer into memory for simplicity; streaming optimizations are out of scope.

## Changes for review

- **New**: `internal/workspacestorage/` — interfaces + localfs + S3 (MinIO-compatible) implementations.
- **New**: `internal/config/workspace_storage.go` — env parsing + provider factories + S3 client builder.
- **Modified**: `internal/server/server.go` — `Config` gains `PersistStorage` + `ArtifactStorage`.
- **Modified**: `internal/server/upload.go`, `internal/server/files.go`, `internal/server/artifacts.go` — switch from direct filesystem ops to storage interfaces.
- **Modified**: `internal/executor/executor.go` — use `PersistStorage.MaterializeToDir` and `ArtifactStorage.PutResult`.
- **Modified**: `internal/cmd/server.go` — wire providers into server + executor.

