# Design 033 — Workspace switch in portal

## Goal

Enable **workspace switching** in the portal: multiple mock workspaces (UUID ids), workspace in the URL, TopBar switcher, and centralized mock data so the UI reflects the selected workspace and the data layer can later be replaced by an API.

## Modules

| Module | Responsibility | Owns |
|--------|----------------|------|
| **portal/src** | Portal UI, routing, mock data | types.ts, router.ts, mockData.ts, App.tsx, components, pages, index.css |
| **portal/** (root) | Vite entry/build | index.html, main.tsx, package.json (unchanged) |

## Structure

**Directory / files (relevant to this task)**

- `portal/src/`
  - `types.ts` — extend `Route` so every variant includes `workspaceId: string`
  - `router.ts` — parseHash/buildHash/navigate/useHashRoute updated for workspace-in-hash
  - `mockData.ts` — 3 workspaces (UUID ids), expanded projects/tasks/artifacts; add `listWorkspaces`, `getWorkspaceById`; remove or replace `MOCK_WORKSPACE`
  - `App.tsx` — resolve workspace from route, validate and redirect if invalid; pass current workspace, list, and navigate callback into shell
  - `components/AppShell.tsx` — accept current workspace, workspaces list, and onWorkspaceChange; pass to TopBar
  - `components/TopBar.tsx` — workspace switcher (dropdown or select) using current workspace + list + onSelect → navigate to workspace home
  - `components/Breadcrumbs.tsx` — build all routes with `route.workspaceId` so links stay in same workspace
  - `components/ProjectsSidebar.tsx` — pass `workspaceId` when calling `navigate({ name: "project", workspaceId, projectId })`
  - `index.css` — styles for workspace switcher in TopBar (no new deps)

**Entity types (unchanged)**

- `Workspace`: `{ id: string; name: string }` — id is UUID in mock.
- `Project`, `Task`, `Artifact`: unchanged; `Project.workspaceId` already exists.

**Route type (extended)**

- Every variant includes `workspaceId: string`:
  - `{ name: "workspace"; workspaceId: string }`
  - `{ name: "project"; workspaceId: string; projectId: string }`
  - `{ name: "task"; workspaceId: string; projectId: string; taskId: string }`
  - `{ name: "artifact"; workspaceId: string; projectId: string; artifactId: string }`

## Method design

### Router (`router.ts`)

| Function | Signature | Responsibility |
|----------|-----------|----------------|
| parseHash | `(hash: string) => Route` | Strip `#`/`#/`, split by `/`. First segment = `workspaceId` (use as-is; if missing, use `""`). Remaining segments: `project/<id>`, `task/<projectId>/<taskId>`, `artifact/<projectId>/<artifactId>` → corresponding Route with that `workspaceId`. Unknown or incomplete → `{ name: "workspace", workspaceId }` with parsed workspaceId or `""`. |
| buildHash | `(route: Route) => string` | Return `#${route.workspaceId}` for workspace; `#${route.workspaceId}/project/${route.projectId}` for project; `#${route.workspaceId}/task/${route.projectId}/${route.taskId}` for task; `#${route.workspaceId}/artifact/${route.projectId}/${route.artifactId}` for artifact. |
| navigate | `(route: Route) => void` | Set `window.location.hash = buildHash(route)`. |
| useHashRoute | `() => Route` | Same as today; returns Route (now with workspaceId). No API change. |

### Mock data (`mockData.ts`)

| Export | Signature | Responsibility |
|--------|-----------|----------------|
| listWorkspaces | `() => Workspace[]` | Return all mock workspaces (3 items, UUID ids, distinct names). |
| getWorkspaceById | `(id: string) => Workspace \| undefined` | Find workspace by id. |
| (existing) | listProjectsForWorkspace, getProjectById, listTasksForProject, listArtifactsForProject, getTaskById, getArtifactById | Unchanged. |
| (remove) | MOCK_WORKSPACE | Replaced by listWorkspaces()[0] or first workspace for default. |

**Mock content**

- **Workspaces**: 3 entries with fixed UUID strings (e.g. `"a1b2c3d4-..."`) and distinct names (e.g. "Sales Team", "Engineering", "Marketing").
- **Projects**: At least 2 per workspace; `workspaceId` set to the owning workspace UUID. Keep existing project ids (e.g. p1, p2) or scope per-workspace (e.g. ws1-p1, ws2-p1) so ids are unique globally.
- **Tasks / Artifacts**: Spread across those projects; no type change. Ensure each workspace has enough content that switching visibly changes sidebar and pages.

### App (`App.tsx`)

- Call `useHashRoute()` to get `route`.
- Get `workspaces = listWorkspaces()`, `defaultWorkspaceId = workspaces[0]?.id ?? ""`.
- If `route.workspaceId` is empty or `getWorkspaceById(route.workspaceId)` is undefined, call `navigate({ name: "workspace", workspaceId: defaultWorkspaceId })` and render nothing (or same shell with default) so one redirect happens.
- Else: `currentWorkspace = getWorkspaceById(route.workspaceId)!`, `projects = listProjectsForWorkspace(route.workspaceId)`.
- Pass to AppShell: `currentWorkspace`, `workspaces`, `projects`, `selectedProjectId` (from route), `route`, and `onWorkspaceChange(workspaceId)` that calls `navigate({ name: "workspace", workspaceId })`.
- Page resolution: same as today but all route variants now have `workspaceId`; use it when resolving project/task/artifact (and ensure project belongs to current workspace if needed). Fallback to workspace home when entity missing.

### AppShell (`AppShell.tsx`)

| Prop | Type | Responsibility |
|------|------|-----------------|
| currentWorkspace | Workspace | The active workspace (id + name). |
| workspaces | Workspace[] | All workspaces for the switcher. |
| projects | Project[] | Projects for current workspace. |
| selectedProjectId | string \| null | As today. |
| route | Route | As today. |
| onWorkspaceChange | (workspaceId: string) => void | Called when user selects another workspace; implement by calling `navigate({ name: "workspace", workspaceId })`. |
| children | ReactNode | As today. |

- Replace `workspaceName` with `currentWorkspace` and pass `workspaces` and `onWorkspaceChange` to TopBar.

### TopBar (`TopBar.tsx`)

| Prop | Type | Responsibility |
|------|------|-----------------|
| currentWorkspace | Workspace | Display name and value for current selection. |
| workspaces | Workspace[] | Options in the switcher. |
| onWorkspaceChange | (workspaceId: string) => void | Called when user picks a different workspace. |

- Replace the static "Workspace: {name}" span with a **workspace switcher**: a `<select>` (or dropdown built with buttons/list) that shows `currentWorkspace.name` and lists all `workspaces`; `onChange`/`onSelect` calls `onWorkspaceChange(workspace.id)`.
- Accessibility: label the control (e.g. "Workspace"), use `aria-label` or `<label>` as needed.

### Breadcrumbs (`Breadcrumbs.tsx`)

- Build crumbs using `route.workspaceId` for every route: Home → `{ name: "workspace", workspaceId: route.workspaceId }`; Project → `{ name: "project", workspaceId: route.workspaceId, projectId }`; etc. No new props; derive from `route`.

### ProjectsSidebar (`ProjectsSidebar.tsx`)

- Receive `workspaceId: string` (current workspace) so that project link includes it: `navigate({ name: "project", workspaceId, projectId: p.id })`. AppShell already has `route.workspaceId`; pass it down as `workspaceId` prop.

## How they work together

**Data/control flow**

1. App calls `useHashRoute()` → Route (with workspaceId). App gets `listWorkspaces()`, validates `route.workspaceId` via `getWorkspaceById`; if invalid or empty, navigates to first workspace home.
2. App loads `currentWorkspace`, `projects` for that workspace, and passes `currentWorkspace`, `workspaces`, `projects`, `route`, and `onWorkspaceChange` to AppShell.
3. AppShell passes `currentWorkspace`, `workspaces`, `onWorkspaceChange` to TopBar; passes `workspaceId={route.workspaceId}` and `projects`, `selectedProjectId` to ProjectsSidebar; passes `route` to Breadcrumbs.
4. TopBar renders the switcher; on select, calls `onWorkspaceChange(workspaceId)` → App’s handler calls `navigate({ name: "workspace", workspaceId })` → hash changes → useHashRoute updates → App re-renders with new workspace.
5. Breadcrumbs and ProjectsSidebar build routes that always include `route.workspaceId`, so all links stay in the current workspace.
6. Page resolution in App uses `route.workspaceId` and existing lookups; invalid/missing entity → fallback to workspace home (with same workspaceId).

**Dependencies**

- `router.ts` depends only on `types.ts` (no mockData).
- App depends on router, mockData, types, and shell/pages; App owns validation and redirect.
- AppShell/TopBar/Breadcrumbs/ProjectsSidebar depend on types and (where needed) router; they receive workspaceId/route/callbacks from App.

**Key data structures**

- **Route**: All variants include `workspaceId`. Created by parseHash (from URL); consumed by App, AppShell, Breadcrumbs, ProjectsSidebar. Used by navigate() to build hash.
- **Workspace[]**: From listWorkspaces(); consumed by App and TopBar for switcher and default.

## Changes for review

- **Modified** `portal/src/types.ts` — extend Route so every variant includes `workspaceId: string`.
- **Modified** `portal/src/router.ts` — parseHash: first segment = workspaceId, then project/task/artifact segments; buildHash: prefix all hashes with workspaceId; navigate unchanged signature; useHashRoute returns extended Route.
- **Modified** `portal/src/mockData.ts` — replace MOCK_WORKSPACE with 3 workspaces (UUID ids); add listWorkspaces(), getWorkspaceById(); expand MOCK_PROJECTS (and tasks/artifacts) so each workspace has ≥2 projects and visible content; keep existing lookup helpers.
- **Modified** `portal/src/App.tsx` — validate route.workspaceId, redirect to first workspace if invalid; pass currentWorkspace, workspaces, onWorkspaceChange to AppShell; use route.workspaceId for all data resolution.
- **Modified** `portal/src/components/AppShell.tsx` — props: currentWorkspace, workspaces, projects, selectedProjectId, route, onWorkspaceChange, children; pass workspaceId to ProjectsSidebar; pass currentWorkspace, workspaces, onWorkspaceChange to TopBar.
- **Modified** `portal/src/components/TopBar.tsx` — props: currentWorkspace, workspaces, onWorkspaceChange; implement workspace switcher (select or dropdown).
- **Modified** `portal/src/components/Breadcrumbs.tsx` — build all crumb routes with route.workspaceId.
- **Modified** `portal/src/components/ProjectsSidebar.tsx` — add prop workspaceId; use it in navigate({ name: "project", workspaceId, projectId: p.id }).
- **Modified** `portal/src/index.css` — add styles for workspace switcher (e.g. .topbar__workspace-select) to match existing TopBar layout.
