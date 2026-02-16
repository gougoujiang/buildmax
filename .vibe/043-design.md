# Design 043 - Upload file to workspace

## Goal

Add a multipart file upload endpoint that saves files to the workspace directory on disk, wire it into the portal Explore page, and make Explore the first sidebar item.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/server** | HTTP handler for multipart upload; route registration | `upload.go`, route in `server.go` |
| **portal/src/lib** | API client function for upload | `uploadFiles` in `api.ts` |
| **portal/src/pages** | Upload button UI on Explore page | `ExplorePage.tsx` |
| **portal/src/components** | Sidebar nav order | `LeftSidebar.tsx` |

## Structure

**Directory / files**

- `internal/server/` — existing server package
  - `upload.go` — **new** — upload handler
  - `server.go` — **modified** — register upload route
- `portal/src/lib/`
  - `api.ts` — **modified** — add `uploadFiles` function
- `portal/src/pages/`
  - `ExplorePage.tsx` — **modified** — add upload button and status message
- `portal/src/components/`
  - `LeftSidebar.tsx` — **modified** — reorder sidebar nav

**Main types and interfaces**

- **uploadResponse** (server): `{ uploaded []string }` — JSON response listing saved filenames.
- No new types needed in portal; `uploadFiles` returns `Promise<{ uploaded: string[] }>`.

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| **Server** | uploadHandler | `(w http.ResponseWriter, r *http.Request)` | Auth, ownership check, parse multipart, validate file count (max 10), save files to workspace dir, return JSON response |
| *(free fn)* | uploadFiles (TS) | `(workspaceId: string, files: File[], token: string) => Promise<{ uploaded: string[] }>` | Build FormData with files under key `files`, POST to `/api/workspaces/{id}/upload`, parse response or throw |

### `Server.uploadHandler` — detailed flow

1. Call `requireAuth(w, r, s.cfg.JWTSecret)` → `(userID, ok)`. Return on 401.
2. Extract `workspace_id` from `r.PathValue("workspace_id")`. Return 400 if empty.
3. Call `s.userOwnsWorkspace(r, userID, workspaceID)`. Return 403 if not owned.
4. Call `r.ParseMultipartForm(32 << 20)` (32 MB memory limit). Return 400 on error.
5. Retrieve `r.MultipartForm.File["files"]`. Return 400 if missing or empty.
6. Check `len(files) > 10`. Return 400 with `"too many files (max 10)"`.
7. Build destination dir: `filepath.Join(config.WorkspacesDir(), workspaceID)`. Ensure dir exists with `os.MkdirAll`.
8. For each file header:
   - Sanitize filename: use `filepath.Base(header.Filename)` to strip path components. Skip if result is `.` or empty.
   - Open the multipart file via `header.Open()`.
   - Create destination file at `filepath.Join(destDir, sanitizedName)` via `os.Create`.
   - Copy contents with `io.Copy(dst, src)`.
   - Close both. On error: return 500 and stop.
   - Append sanitized name to the `uploaded` list.
9. Return 200 with `{"uploaded": ["file1.txt", "file2.png"]}`.

### `uploadFiles` (TypeScript) — detailed flow

1. Create a `FormData` instance.
2. For each `File` in the array, call `formData.append("files", file)`.
3. Send `fetch(url, { method: "POST", headers: { Authorization: "Bearer ..." }, body: formData })`.
   - Do **not** set `Content-Type` — the browser sets `multipart/form-data` with the boundary automatically.
4. On non-ok response, parse error JSON and throw (same pattern as other API functions).
5. Return parsed JSON as `{ uploaded: string[] }`.

### `ExplorePage` — upload UI

1. Add state: `uploading: boolean`, `uploadMsg: string | null`.
2. Add a hidden `<input type="file" multiple>` and a visible "Upload Files" button that triggers it.
3. On file selection (`onChange`):
   - If more than 10 files selected, set error message and return.
   - Call `uploadFiles(workspaceId, selectedFiles, token)` (token from `useAuth()`).
   - On success: set message like `"Uploaded 3 file(s)"`.
   - On error: set error message.
   - Clear message after a few seconds with `setTimeout`.
4. Keep the existing mock tree/viewer completely untouched.

### `LeftSidebar` — reorder

Move the Explore entry to the first position in the `SIDEBAR_NAV` array:

```typescript
const SIDEBAR_NAV = [
  { label: "Explore", name: "explore" },
  { label: "Projects", name: "workspace" },
  { label: "Activity", name: "activity" },
]
```

## How they work together

**Data/control flow**

1. User clicks "Upload Files" on the Explore page → file picker opens.
2. User selects 1–10 files → `ExplorePage` calls `uploadFiles(workspaceId, files, token)`.
3. `uploadFiles` builds `FormData`, sends `POST /api/workspaces/{workspace_id}/upload` with `Authorization: Bearer <token>`.
4. CORS middleware sets headers (already handles `Content-Type` and `Authorization` in allowed headers).
5. `Server.uploadHandler` authenticates, checks ownership, parses multipart form.
6. Handler saves each file to `~/.buildmax/workspaces/<workspace_id>/<filename>`.
7. Handler responds with `200 { "uploaded": [...] }`.
8. `ExplorePage` shows success or error message.

**Dependencies**

- `internal/server/upload.go` depends on `internal/config` (for `WorkspacesDir()`), `internal/server` helpers (`requireAuth`, `userOwnsWorkspace`, `writeJSON`, `writeJSONError`).
- `portal/src/pages/ExplorePage.tsx` depends on `portal/src/lib/api.ts` (`uploadFiles`) and `portal/src/contexts/AuthContext` (`useAuth`).
- No new Go packages or npm dependencies required.

**Key data structures**

- **multipart form data** (HTTP): created by browser from `FormData`, consumed by Go `r.ParseMultipartForm`. Field name: `files`.
- **uploadResponse** `{ "uploaded": ["name1", "name2"] }`: created by handler, consumed by portal `uploadFiles`.

## Changes for review

- **New**: `internal/server/upload.go` — `uploadHandler` method; multipart parse, file save, JSON response.
- **Modified**: `internal/server/server.go` — add route `POST /api/workspaces/{workspace_id}/upload`.
- **Modified**: `portal/src/lib/api.ts` — add `uploadFiles()` function.
- **Modified**: `portal/src/pages/ExplorePage.tsx` — add upload button, status message, `useAuth` for token.
- **Modified**: `portal/src/components/LeftSidebar.tsx` — move Explore to first position in `SIDEBAR_NAV`.
