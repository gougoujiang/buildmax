# Design 045 - Allow upload directory

## Goal

Extend the upload endpoint and portal UI so users can upload an entire directory (with nested structure preserved) to a workspace.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/server** (`upload.go`) | Handle multipart upload; when `paths` field is present, save files at relative paths with subdirectory creation | `uploadHandler`, constants |
| **portal/src/lib** (`api.ts`) | Send files + optional paths to the upload endpoint | `uploadFiles` function |
| **portal/src/pages** (`ExplorePage.tsx`) | Folder upload button, directory picker, wire to API | UI, `handleFolderUpload` callback |

## Structure

**Directory / files**

- `internal/server/`
  - `upload.go` — **edit**: add `paths` field handling, path validation, subdirectory creation, dual file limits
- `portal/src/lib/`
  - `api.ts` — **edit**: add optional `paths` parameter to `uploadFiles`
- `portal/src/pages/`
  - `ExplorePage.tsx` — **edit**: add folder upload button, second hidden input with `webkitdirectory`, handler

**Main types (no new types)**

- **uploadResponse** (server): unchanged — `{ "uploaded": ["sales/2024/sales.2024.csv", ...] }` values now include relative paths for directory uploads.
- **UploadResponse** (portal): unchanged — `{ uploaded: string[] }`.

## Method design

### Backend — `internal/server/upload.go`

| Receiver | Method / Func | Signature | Responsibility |
|----------|---------------|-----------|----------------|
| `*Server` | `uploadHandler` | `(w http.ResponseWriter, r *http.Request)` | **Modified**: detect `paths` form field; when present use paths as relative destinations with validation; otherwise keep existing flat behavior |

#### `uploadHandler` — updated logic

Current flow (unchanged when no `paths` field):

1. Auth + ownership check (unchanged)
2. `ParseMultipartForm(32 << 20)` (unchanged)
3. Get `fileHeaders` from `files` field (unchanged)
4. Validate file count against `maxUploadFiles` (10) (unchanged)
5. Save each file with `filepath.Base(fh.Filename)` (unchanged)

New flow (when `paths` field is present — directory upload):

1. Auth + ownership check (same)
2. `ParseMultipartForm(64 << 20)` — increase to 64 MB for directory uploads
3. Get `fileHeaders` from `files` field (same)
4. Get `paths` from `r.MultipartForm.Value["paths"]`
5. **Validate**: `len(paths) == len(fileHeaders)` — mismatch → 400
6. **Validate**: file count against `maxUploadDirFiles` (200) — too many → 400
7. For each file `i`:
   a. `relPath := paths[i]` — the relative path from the portal (e.g. `sales/2024/sales.2024.csv`)
   b. Create `ws := &util.Workspace{Root: destDir}`
   c. `absPath, err := ws.ResolvePath(relPath)` — rejects traversal → 400
   d. `os.MkdirAll(filepath.Dir(absPath), 0755)` — create intermediate dirs
   e. Open source, create dest at `absPath`, copy (same as current loop)
   f. Append `relPath` (not just base name) to `uploaded`
8. Return `{ "uploaded": [...] }`

**Key detail**: The `paths` field acts as a switch. When absent, the handler behaves exactly as today (flat, 10-file limit). When present, it enables directory mode (relative paths, 200-file limit). This keeps the existing upload flow fully backward-compatible.

#### Constants

```go
const maxUploadFiles    = 10  // existing — flat file upload
const maxUploadDirFiles = 200 // new — directory upload
```

### Portal — `portal/src/lib/api.ts`

| Function | Signature | Responsibility |
|----------|-----------|----------------|
| `uploadFiles` | `(workspaceId: string, files: File[], token: string, paths?: string[]): Promise<UploadResponse>` | **Modified**: when `paths` is provided, append each as a `paths` field in FormData |

#### `uploadFiles` — updated logic

```ts
export async function uploadFiles(
  workspaceId: string,
  files: File[],
  token: string,
  paths?: string[],
): Promise<UploadResponse> {
  const formData = new FormData()
  for (const file of files) {
    formData.append("files", file)
  }
  if (paths) {
    for (const p of paths) {
      formData.append("paths", p)
    }
  }
  // ... fetch unchanged ...
}
```

### Portal — `portal/src/pages/ExplorePage.tsx`

| Element | What | Responsibility |
|---------|------|----------------|
| `folderInputRef` | `useRef<HTMLInputElement>(null)` | Second hidden input ref for folder picker |
| `handleFolderUpload` | `(e: React.ChangeEvent<HTMLInputElement>) => void` | Collect files + `webkitRelativePath`, call `uploadFiles` with `paths` |
| "Upload Folder" button | JSX | Triggers `folderInputRef.current?.click()` |

#### `handleFolderUpload` — new callback

```ts
const handleFolderUpload = useCallback(
  async (e: React.ChangeEvent<HTMLInputElement>) => {
    const selected = e.target.files
    if (!selected || selected.length === 0) return
    if (!token) {
      setUploadMsg({ text: "Not authenticated", isError: true })
      return
    }
    setUploading(true)
    setUploadMsg(null)
    try {
      const files = Array.from(selected)
      const paths = files.map((f) => f.webkitRelativePath)
      const res = await uploadFiles(workspaceId, files, token, paths)
      setUploadMsg({
        text: `Uploaded ${res.uploaded.length} file(s)`,
        isError: false,
      })
      await fetchTree()
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Upload failed"
      setUploadMsg({ text: msg, isError: true })
    } finally {
      setUploading(false)
      if (folderInputRef.current) folderInputRef.current.value = ""
    }
  },
  [workspaceId, token, fetchTree],
)
```

#### Hidden folder input

```tsx
<input
  ref={folderInputRef}
  type="file"
  className="page-explore__file-input"
  onChange={handleFolderUpload}
  {...{ webkitdirectory: "", directory: "" }}
/>
```

Note: React does not recognize `webkitdirectory` / `directory` as standard props, so we spread them as attributes. This is the standard approach for non-standard HTML attributes in React.

#### "Upload Folder" button

Placed next to the existing "Upload Files" button in the upload bar:

```tsx
<button
  type="button"
  className="page-explore__upload-btn"
  disabled={uploading}
  onClick={() => folderInputRef.current?.click()}
>
  {uploading ? "Uploading…" : "Upload Folder"}
</button>
```

The existing `handleUpload` (file upload) stays unchanged — it does not send `paths`, so the backend uses the flat 10-file behavior.

## How they work together

**Data / control flow — directory upload**

```
User clicks "Upload Folder"
  → folderInputRef.current.click() opens native folder picker
  → User selects folder "sales/"
  → Browser enumerates files recursively:
      File { name: "sales.2024.csv", webkitRelativePath: "sales/2024/sales.2024.csv" }
      File { name: "sales.2025.csv", webkitRelativePath: "sales/2025/sales.2025.csv" }
  → handleFolderUpload fires
    → files = Array.from(selected)
    → paths = files.map(f => f.webkitRelativePath)
    → uploadFiles(workspaceId, files, token, paths)
      → FormData: files=<blob>, files=<blob>, paths="sales/2024/sales.2024.csv", paths="sales/2025/sales.2025.csv"
      → POST /api/workspaces/{id}/upload

Backend uploadHandler:
  1. Auth + ownership ✓
  2. ParseMultipartForm
  3. fileHeaders = ["files"] (2 entries)
  4. paths = ["paths"] → ["sales/2024/sales.2024.csv", "sales/2025/sales.2025.csv"]
  5. len(paths) == len(fileHeaders) ✓ → directory mode
  6. len(fileHeaders) <= 200 ✓
  7. For file[0]:
     - relPath = "sales/2024/sales.2024.csv"
     - ws.ResolvePath("sales/2024/sales.2024.csv") → <wsDir>/sales/2024/sales.2024.csv ✓
     - MkdirAll(<wsDir>/sales/2024/) ✓
     - Copy file content → <wsDir>/sales/2024/sales.2024.csv
  8. For file[1]: same with "sales/2025/sales.2025.csv"
  9. Response: { "uploaded": ["sales/2024/sales.2024.csv", "sales/2025/sales.2025.csv"] }

Portal:
  → uploadFiles resolves with { uploaded: [...] }
  → setUploadMsg("Uploaded 2 file(s)")
  → fetchTree() refreshes the file tree
  → Tree shows: Workspace > sales > 2024 > sales.2024.csv, 2025 > sales.2025.csv
```

**Data / control flow — existing flat file upload (unchanged)**

```
User clicks "Upload Files" → selects files → handleUpload
  → uploadFiles(workspaceId, files, token) — no paths param
  → FormData: files=<blob> only
  → Backend: no "paths" field → flat mode, maxUploadFiles=10, filepath.Base(filename)
  → Same behavior as before
```

**Dependencies**

- `internal/server/upload.go` depends on `internal/util` for `Workspace.ResolvePath` (new import)
- `portal/src/lib/api.ts` — no new dependencies
- `portal/src/pages/ExplorePage.tsx` — no new dependencies (already imports `uploadFiles`)

## Changes for review

| Change | File | Type |
|--------|------|------|
| Add `paths` form field handling, `ResolvePath` validation, `MkdirAll` for subdirs, dual file limit constants | `internal/server/upload.go` | Edit |
| Add optional `paths` parameter to `uploadFiles` function | `portal/src/lib/api.ts` | Edit |
| Add `folderInputRef`, `handleFolderUpload`, "Upload Folder" button, hidden `webkitdirectory` input | `portal/src/pages/ExplorePage.tsx` | Edit |
