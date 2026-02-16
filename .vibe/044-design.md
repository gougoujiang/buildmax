# Design 044: Workspace file explore

## Goal

Add two backend endpoints for browsing workspace files (directory tree + file content) and wire the portal Explore page to use them instead of mock data.

## Modules

| Module | Package / Path | Role |
|--------|---------------|------|
| File handlers | `internal/server/files.go` (new) | HTTP handlers for tree and file content |
| Route registration | `internal/server/server.go` (edit) | Register two new routes |
| Portal API | `portal/src/lib/api.ts` (edit) | `getFileTree`, `getFileContent` functions |
| Portal types | `portal/src/lib/types.ts` (edit) | Adjust `ExploreNode` file variant (content optional) |
| Explore page | `portal/src/pages/ExplorePage.tsx` (edit) | Replace mock data with API calls |
| Mock data | `portal/src/data/mockExplore.ts` (delete) | No longer needed |

## Structure

### Backend — `internal/server/files.go`

```go
package server

// --- Types ---

// fileNode is the JSON shape for a directory tree node.
type fileNode struct {
    ID       string      `json:"id"`       // relative path from workspace root; "." for root
    Name     string      `json:"name"`     // base name (or "Workspace" for root)
    Type     string      `json:"type"`     // "folder" or "file"
    Children []*fileNode `json:"children,omitempty"` // only for folders
}

// --- Handlers (receivers on *Server) ---

// filesTreeHandler handles GET /api/workspaces/{workspace_id}/files
// Returns the full directory tree as nested JSON.
func (s *Server) filesTreeHandler(w http.ResponseWriter, r *http.Request)

// fileContentHandler handles GET /api/workspaces/{workspace_id}/files/{path...}
// Returns the raw file content as text/plain.
func (s *Server) fileContentHandler(w http.ResponseWriter, r *http.Request)

// --- Internal helpers ---

// buildTree recursively walks a directory and returns a fileNode tree.
// root is the absolute workspace dir; dir is the current absolute dir being walked.
// relPrefix is the relative path of dir from root (empty string for root itself).
// Sorts: folders first, then files, both alphabetically.
func buildTree(root, dir, relPrefix string) (*fileNode, error)
```

### Route registration — `internal/server/server.go`

Two new lines in `New()`:

```go
mux.HandleFunc("GET /api/workspaces/{workspace_id}/files", s.filesTreeHandler)
mux.HandleFunc("GET /api/workspaces/{workspace_id}/files/{path...}", s.fileContentHandler)
```

The Go 1.22 mux distinguishes exact match (`/files`) from wildcard (`/files/{path...}`), so these don't conflict.

### Portal types — `portal/src/lib/types.ts`

The `ExploreNode` file variant currently requires `content: string`. Since the tree endpoint returns files without content, and content is loaded lazily, make `content` optional:

```ts
export type ExploreNode =
  | { id: string; name: string; type: "folder"; children: ExploreNode[] }
  | { id: string; name: string; type: "file"; content?: string }
```

### Portal API — `portal/src/lib/api.ts`

Two new functions following the existing pattern:

```ts
// Fetch the full directory tree for a workspace.
export async function getFileTree(
  workspaceId: string,
  token: string,
): Promise<ExploreNode>

// Fetch file content as plain text.
export async function getFileContent(
  workspaceId: string,
  filePath: string,
  token: string,
): Promise<string>
```

### Portal Explore page — `portal/src/pages/ExplorePage.tsx`

Replace mock data with real API calls:

- **State**: Add `tree: ExploreNode | null`, `treeLoading: boolean`, `treeError: string | null`, `fileLoading: boolean`.
- **On mount + after upload**: Call `getFileTree(workspaceId, token)` → set `tree`.
- **On file click**: Call `getFileContent(workspaceId, node.id, token)` → set file content in state; display in viewer.
- **Helper functions**: `getExploreChildren` and `getExploreNodeById` move inline (or to a local util) since they operate on the fetched tree, not the mock.
- **Root id**: Changes from `"root"` to `"."` (matching the backend).

## Method design

### `filesTreeHandler`

```
Receiver: (s *Server)
Signature: filesTreeHandler(w http.ResponseWriter, r *http.Request)
```

1. `requireAuth(w, r, s.cfg.JWTSecret)` → `userID`
2. `r.PathValue("workspace_id")` → `workspaceID`; validate non-empty
3. `s.userOwnsWorkspace(r, userID, workspaceID)` → ownership check
4. `wsDir := filepath.Join(config.WorkspacesDir(), workspaceID)`
5. `os.MkdirAll(wsDir, 0755)` — ensure dir exists (empty workspace case)
6. `buildTree(wsDir, wsDir, "")` → `tree *fileNode`
7. Set root node: `tree.ID = "."`, `tree.Name = "Workspace"`
8. `writeJSON(w, 200, tree)`

Error cases:
- Auth/ownership → 401/403 (handled by helpers)
- Filesystem error → 500

### `fileContentHandler`

```
Receiver: (s *Server)
Signature: fileContentHandler(w http.ResponseWriter, r *http.Request)
```

1. `requireAuth(w, r, s.cfg.JWTSecret)` → `userID`
2. `r.PathValue("workspace_id")` → `workspaceID`; validate non-empty
3. `s.userOwnsWorkspace(r, userID, workspaceID)` → ownership check
4. `r.PathValue("path")` → `filePath`; validate non-empty
5. Create `util.Workspace{Root: filepath.Join(config.WorkspacesDir(), workspaceID)}`
6. `ws.ResolvePath(filePath)` → `absPath`; on error → 400 (path traversal)
7. `os.Stat(absPath)` → check exists and is not a directory; 404 otherwise
8. `os.ReadFile(absPath)` → `data`
9. `w.Header().Set("Content-Type", "text/plain; charset=utf-8")`
10. `w.WriteHeader(200)`; `w.Write(data)`

Error cases:
- Path traversal → 400
- Not found / is directory → 404
- Read error → 500

### `buildTree`

```
Signature: buildTree(root, dir, relPrefix string) (*fileNode, error)
```

1. `os.ReadDir(dir)` → `entries`
2. Separate entries into two slices: dirs and files (by `entry.IsDir()`)
3. Sort each slice alphabetically by name
4. For each dir entry:
   - `childRel := path.Join(relPrefix, entry.Name())` (use `path` not `filepath` for slash consistency in JSON)
   - Recurse: `buildTree(root, filepath.Join(dir, entry.Name()), childRel)`
   - Append to `node.Children`
5. For each file entry:
   - `childRel := path.Join(relPrefix, entry.Name())`
   - Append `&fileNode{ID: childRel, Name: entry.Name(), Type: "file"}` to `node.Children`
6. Return `&fileNode{ID: relPrefix, Name: filepath.Base(dir), Type: "folder", Children: children}`

### Portal `getFileTree`

```
Signature: getFileTree(workspaceId: string, token: string): Promise<ExploreNode>
```

1. `fetch(${getApiBase()}/api/workspaces/${workspaceId}/files, { headers: { Authorization } })`
2. Standard error handling (same as `getWorkspaces`)
3. `return res.json() as Promise<ExploreNode>`

### Portal `getFileContent`

```
Signature: getFileContent(workspaceId: string, filePath: string, token: string): Promise<string>
```

1. `fetch(${getApiBase()}/api/workspaces/${workspaceId}/files/${encodeURIPath(filePath)}, { headers: { Authorization } })`
2. Standard error handling
3. `return res.text()` (plain text, not JSON)

Note: `filePath` segments need proper encoding. Use `filePath.split("/").map(encodeURIComponent).join("/")` to encode each segment while preserving the path structure.

### Portal `ExplorePage` changes

| Current | New |
|---------|-----|
| `const root = MOCK_EXPLORE_TREE` | `useEffect` → `getFileTree()` → `setTree(data)` |
| `getExploreChildren(root, id)` | Same logic, inline, operates on `tree` state |
| `getExploreNodeById(root, id)` | Same logic, inline, operates on `tree` state |
| `setSelectedFile(node)` | `setSelectedFile(node)` + `getFileContent()` → update content |
| Root id: `"root"` | Root id: `"."` |
| Import `mockExplore` | Remove import; delete `mockExplore.ts` |

Loading states:
- While tree loads: show a loading indicator in the tree panel
- While file content loads: show "Loading..." in the viewer panel

## How they work together

```
Portal loads Explore page
  → useEffect calls getFileTree(workspaceId, token)
    → GET /api/workspaces/{id}/files
      → filesTreeHandler: auth → ownership → buildTree(wsDir) → JSON response
  → Portal renders tree + file list from the returned ExploreNode

User clicks a file
  → getFileContent(workspaceId, node.id, token)
    → GET /api/workspaces/{id}/files/docs/readme.md
      → fileContentHandler: auth → ownership → ResolvePath → read file → text/plain
  → Portal displays content in the viewer panel

User uploads files
  → uploadFiles() (existing)
  → On success, re-call getFileTree() → tree refreshes with new files
```

## Changes for review

| Change | File | Type |
|--------|------|------|
| New file: tree + content handlers, `buildTree` helper, `fileNode` type | `internal/server/files.go` | New |
| Register 2 new routes | `internal/server/server.go` | Edit (2 lines) |
| Add `getFileTree`, `getFileContent` | `portal/src/lib/api.ts` | Edit |
| Make `ExploreNode` file `content` optional | `portal/src/lib/types.ts` | Edit (1 line) |
| Replace mock data with API calls, add loading states | `portal/src/pages/ExplorePage.tsx` | Edit |
| Delete mock data file | `portal/src/data/mockExplore.ts` | Delete |
