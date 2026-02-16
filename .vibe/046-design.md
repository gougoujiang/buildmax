# Design 046 — Create new workspace

## Goal

Allow authenticated users to create a new workspace via `POST /api/workspaces` and trigger it from the portal's existing "+ New" button.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/store** | Workspace persistence — add `CreateWorkspace` to the store interface and implementation. | `WorkspaceStore` interface, `*Store.CreateWorkspace` method. |
| **internal/server** | HTTP handler for `POST /api/workspaces` — auth, validation, store call, disk dir, response. | `createWorkspaceHandler`, `createWorkspaceRequest`, route registration. |
| **portal/src/lib/api.ts** | Portal API client — add `createWorkspace` function. | `createWorkspace()`. |
| **portal/src/App.tsx** | Wire `onNewWorkspace` callback — prompt, call API, refetch, navigate. | `handleNewWorkspace` function, prop threading. |

## Structure

**Directory / files**

- `internal/store/`
  - `store.go` — Add `CreateWorkspace` to `WorkspaceStore` interface; add implementation on `*Store`.
- `internal/server/`
  - `workspaces.go` — Add `createWorkspaceRequest` struct and `createWorkspaceHandler` method.
  - `server.go` — Register `POST /api/workspaces` route.
- `portal/src/lib/`
  - `api.ts` — Add `createWorkspace` function.
- `portal/src/`
  - `App.tsx` — Add `handleNewWorkspace`, pass to `AppShell`.

**Main types and interfaces**

- **WorkspaceStore** (store): Extended with `CreateWorkspace(ctx, userID, name) (*Workspace, error)`.
- **createWorkspaceRequest** (server): JSON request body `{ "name": "..." }`.
- **WorkspaceResponse** (server): Already exists — reused for the 201 response.
- **ApiWorkspace** (portal): Already exists — reused as the return type of `createWorkspace`.

## Method design

| Receiver / Location | Method / Function | Signature | Responsibility |
|---------------------|-------------------|-----------|----------------|
| `*Store` | `CreateWorkspace` | `(ctx context.Context, userID string, name string) (*Workspace, error)` | Generate UUID, build `Workspace` struct, insert row, return created workspace. |
| `*Server` | `createWorkspaceHandler` | `(w http.ResponseWriter, r *http.Request)` | Auth via `requireAuth`; decode body; validate name non-empty; call `WorkspaceStore.CreateWorkspace`; create workspace dir on disk; return `201` with `WorkspaceResponse`. |
| portal `api.ts` | `createWorkspace` | `(body: { name: string }, token: string) => Promise<ApiWorkspace>` | POST `/api/workspaces` with JSON body and Bearer token; parse and return response. |
| `App.tsx` | `handleNewWorkspace` | `() => Promise<void>` | Prompt user for name; call `createWorkspace`; refetch workspaces; navigate to the new workspace. |

## How they work together

**Data / control flow**

1. User clicks "+ New" in TopBar → calls `onNewWorkspace` → `handleNewWorkspace` in `App.tsx`.
2. `handleNewWorkspace` opens a `window.prompt` for the workspace name. If empty/cancelled, abort.
3. Calls `createWorkspace({ name }, token)` in `api.ts`.
4. `api.ts` sends `POST /api/workspaces` with `{ "name": "..." }` and `Authorization: Bearer <token>`.
5. `createWorkspaceHandler` in the server:
   a. Calls `requireAuth` — extracts `userID` from JWT. Returns `401` if invalid.
   b. Checks `WorkspaceStore` is configured. Returns `503` if nil.
   c. Decodes JSON body into `createWorkspaceRequest`. Returns `400` if malformed.
   d. Validates `name` is non-empty. Returns `400` if blank.
   e. Calls `WorkspaceStore.CreateWorkspace(ctx, userID, name)`.
   f. Creates the workspace directory: `os.MkdirAll(filepath.Join(config.WorkspacesDir(), ws.WorkspaceID), 0755)`.
   g. Returns `201 Created` with `WorkspaceResponse{ID: ws.WorkspaceID, Name: ws.Name, OwnerUserID: ws.OwnerUserID, CreatedAt: ws.CreatedAt}`.
6. `Store.CreateWorkspace`:
   a. Builds `Workspace{WorkspaceID: newUUID(), OwnerUserID: userID, Name: name, CreatedAt: time.Now().Unix()}`.
   b. `db.WithContext(ctx).Create(&w)`.
   c. Returns `&w, nil` on success.
7. Portal receives `ApiWorkspace` response → refetches full workspace list via `getWorkspaces(token)` → updates state → navigates to `#<newWorkspaceId>`.

**Dependencies**

- `internal/server` depends on `internal/store` for `WorkspaceStore.CreateWorkspace`.
- `internal/server` depends on `internal/config` for `WorkspacesDir()`.
- Portal `App.tsx` depends on `api.ts` for `createWorkspace` and `getWorkspaces`.
- No new cross-package dependencies introduced.

**Key data structures**

- `createWorkspaceRequest` (server): Created by JSON decoder from request body; consumed by handler to extract `Name`.
- `Workspace` (store): Created by `CreateWorkspace`; consumed by handler to build `WorkspaceResponse`.
- `WorkspaceResponse` (server): Already exists; serialized as JSON to the client.

## Changes for review

- **Modified**: `internal/store/store.go` — Add `CreateWorkspace(ctx, userID, name) (*Workspace, error)` to `WorkspaceStore` interface; add implementation on `*Store`.
- **Modified**: `internal/server/workspaces.go` — Add `createWorkspaceRequest` struct and `createWorkspaceHandler` method.
- **Modified**: `internal/server/server.go` — Register `POST /api/workspaces` route (one line, after the existing `GET /api/workspaces`).
- **Modified**: `portal/src/lib/api.ts` — Add `createWorkspace(body, token)` function.
- **Modified**: `portal/src/App.tsx` — Add `handleNewWorkspace` function; pass `onNewWorkspace={handleNewWorkspace}` to `AppShell`.
