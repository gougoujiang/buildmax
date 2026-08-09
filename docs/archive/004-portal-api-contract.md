# Portal API Contract

This document is the **single source of truth**

## Route vs scope (frontend)

- **Route** = URL state (hash). Drives navigation and which view is shown. Type: `Route` (workspace | project | task | activity | explore).
- **Scope** = Derived context for the current view: `{ workspaceId, projectId?, taskId? }`. Use for data fetching and display. Obtain via `getWorkspaceScope(route)` or `useWorkspaceScope()` inside `WorkspaceProvider`. See `portal/src/lib/types.ts` and `portal/src/lib/router.ts`.

--- for the HTTP API used by the Portal (React frontend). The Go server (`internal/server`) implements these endpoints; the Portal types in `portal/src/lib/api/types.ts` must stay in sync with the response shapes below. When adding or changing a field, update this doc and both server and portal.

- All JSON uses **snake_case** for object keys.
- Workspace-scoped endpoints require `Authorization: Bearer <jwt>` and validate that the user owns the workspace (path param `workspace_id`).
- Error responses use `{ "error": "message" }` with an appropriate HTTP status.

---

## Auth (no token)

### POST /api/otp/request

**Request:** `{ "email": string, "intent": "signup" | "login" }`

**Response 200:** `{ "message": string }` (e.g. `"otp_sent"`)

### POST /api/login

**Request:** `{ "email": string, "otp": string }`

**Response 200:** `{ "token": string, "user": { "id": string, "email": string, "name": string } }`

---

## Workspaces (Bearer token)

### GET /api/workspaces

**Response 200:** `[{ "id": string, "name": string, "owner_user_id": string, "created_at": number }]`

### POST /api/workspaces

**Request:** `{ "name": string }`

**Response 200:** `{ "id": string, "name": string, "owner_user_id": string, "created_at": number }`

---

## Projects (workspace-scoped)

### GET /api/workspaces/{workspace_id}/projects

**Response 200:** `[{ "id": string, "workspace_id": string, "name": string, "description": string, "created_at": number }]`

### POST /api/workspaces/{workspace_id}/projects

**Request:** `{ "name": string, "description"?: string }`

**Response 200:** `{ "id": string, "workspace_id": string, "name": string, "description": string, "created_at": number }`

---

## Tasks (workspace-scoped)

### GET /api/workspaces/{workspace_id}/tasks?project_id={id}

**Response 200:** `[{ "id": string, "workspace_id": string, "project_id": string | null, "session_id": string | null, "status": string, "input": string, "output": string | null, "created_by": string, "created_at": number, "started_at": number | null, "ended_at": number | null, "error_message": string | null }]`

Status values: `PENDING`, `RUNNING`, `SUCCEEDED`, `FAILED`, `CANCELED`.

### POST /api/workspaces/{workspace_id}/tasks

**Request:** `{ "input": string, "project_id"?: string }`

**Response 200:** Same shape as one task in the list above.

### POST /api/workspaces/{workspace_id}/tasks/{task_id}/runs

**Request:** `{ "input": string }`

**Response 200:** `{ "run_id": string, "task_id": string }`

**Response 409:** `{ "error": string }` when a run is already in progress.

### GET /api/workspaces/{workspace_id}/tasks/{task_id}/conversation

**Response 200:** `{ "id": string, "title": string, "created_at": string, "messages": [{ "role": string, "content": string, "tool_call_id"?: string, "tool_calls"?: [{ "id": string, "name": string, "arguments"?: string }] }] }`

**Response 404:** Task not found or no conversation yet.

---

## Artifacts (run outputs, workspace-scoped)

### GET /api/workspaces/{workspace_id}/artifacts?task_id=

**Response 200:** `[{ "task_run_id": string, "task_id": string, "workspace_id": string, "created_at": number, "task_input_snippet": string }]`

### GET /api/workspaces/{workspace_id}/artifacts/{task_run_id}/items

**Response 200:** `[{ "relative_path": string }]`

### GET /api/workspaces/{workspace_id}/artifacts/{task_run_id}/content?path=

**Response 200:** Plain text body (file content). Optional `path` for a specific file (default `result.md`).

---

## Upload & Files (workspace-scoped)

### POST /api/workspaces/{workspace_id}/upload

**Request:** `multipart/form-data` with `files` (File[]) and optional `paths` (string[]) for relative paths.

**Response 200:** `{ "uploaded": string[] }` (filenames).

### GET /api/workspaces/{workspace_id}/files

**Response 200:** Nested tree: `{ "id": string, "name": string, "type": "folder" | "file", "children"?: <same shape>[] }`. Root has `id: "."`, `name: "Workspace"`, `type: "folder"`.

### GET /api/workspaces/{workspace_id}/files/{path...}

Path is slash-separated; each segment is URL-encoded. **Response 200:** Plain text file content.

---

## TypeScript DTOs

Portal DTOs live in `portal/src/lib/api/types.ts`. Names and field types must match the JSON above. When in doubt, compare with Go structs in `internal/server/*.go` (e.g. `WorkspaceResponse`, `TaskResponse`, `ArtifactResponse`).
