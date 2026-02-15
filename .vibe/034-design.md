# Design 034 — Portal layout details (sidebar + Activity/Explore)

## Goal

Add the **primary left sidebar** with Home / Projects / Activity / Explore and the new **activity** and **explore** routes and pages. Top Bar and Assistants are out of scope; all data remains mock.

## Modules

| Module | Responsibility | Owns |
|--------|----------------|------|
| **portal/src** | Sidebar nav, new routes, new pages, mock helpers | types, router, mockData, App, AppShell, LeftSidebar (or refactored sidebar), Breadcrumbs, ActivityPage, ExplorePage, CSS |

## Structure

**Directory / files (relevant to this task)**

- `portal/src/types.ts` — extend `Route` with `{ name: "activity"; workspaceId: string }` and `{ name: "explore"; workspaceId: string }`.
- `portal/src/router.ts` — parseHash: recognize `/<workspaceId>/activity` and `/<workspaceId>/explore`; buildHash: emit same. No API change to navigate / useHashRoute.
- `portal/src/mockData.ts` — add `listTasksForWorkspace(workspaceId)`, `listArtifactsForWorkspace(workspaceId)` (aggregate from existing projects/tasks/artifacts).
- `portal/src/App.tsx` — resolve `activity` and `explore` in renderPage(); fix selectedProjectId so it is only set when route is project/task/artifact (not for workspace/activity/explore).
- `portal/src/components/AppShell.tsx` — replace `ProjectsSidebar` with a single left sidebar component that includes primary nav (Home, Projects, Activity, Explore) and the project list under Projects. Same props from App (workspaceId, projects, selectedProjectId, route, etc.).
- **New** `portal/src/components/LeftSidebar.tsx` — primary left sidebar: top = nav links (Home, Projects, Activity, Explore) with active state from route; below = project list (existing list + “+ New Project”) under a “Projects” heading. Active state: Home when `route.name === "workspace"`, Projects when project/task/artifact, Activity when activity, Explore when explore.
- `portal/src/components/ProjectsSidebar.tsx` — either removed (logic inlined into LeftSidebar) or kept as a sub-component that only renders the project list (no heading), used inside LeftSidebar. Prefer **single LeftSidebar** that contains both nav and list to avoid prop drilling and keep one place for “active” logic.
- `portal/src/components/Breadcrumbs.tsx` — handle `activity` and `explore`: for activity route show Home > Activity; for explore route show Home > Explore. No project crumb.
- **New** `portal/src/pages/ActivityPage.tsx` — workspace-level activity: receives `workspaceId`, uses `listTasksForWorkspace(workspaceId)` (and optionally getProjectById for project name); renders a list of tasks (title, project name, timeLabel, status). Mock only.
- **New** `portal/src/pages/ExplorePage.tsx` — workspace browse: receives `workspaceId`, uses `listArtifactsForWorkspace(workspaceId)` (and getProjectById for project name); renders list of artifacts with links to `#<workspaceId>/artifact/<projectId>/<artifactId>`. Mock only.
- `portal/src/index.css` — add classes for primary nav in sidebar (e.g. `.sidebar__nav`, `.sidebar__nav-item`, `.sidebar__nav-item--active`) and for Activity/Explore page layout; reuse existing `.sidebar` where possible.

**Route type (extended)**

```ts
export type Route =
  | { name: "workspace"; workspaceId: string }
  | { name: "project"; workspaceId: string; projectId: string }
  | { name: "task"; workspaceId: string; projectId: string; taskId: string }
  | { name: "artifact"; workspaceId: string; projectId: string; artifactId: string }
  | { name: "activity"; workspaceId: string }
  | { name: "explore"; workspaceId: string }
```

**Hash format**

- Activity: `#<workspaceId>/activity`
- Explore: `#<workspaceId>/explore`

Parse order: after reading workspaceId (first segment), if second segment is `"activity"` → activity route; if `"explore"` → explore route; else existing project/task/artifact logic.

## Method design

### Router (`router.ts`)

| Function | Signature | Responsibility |
|----------|-----------|----------------|
| parseHash | `(hash: string) => Route` | After workspaceId (parts[0]), if parts[1] === "activity" return `{ name: "activity", workspaceId }`; if parts[1] === "explore" return `{ name: "explore", workspaceId }`. Otherwise unchanged (project/task/artifact/workspace). |
| buildHash | `(route: Route) => string` | For activity: `#${route.workspaceId}/activity`. For explore: `#${route.workspaceId}/explore`. Other cases unchanged. |
| navigate | (unchanged) | No signature change. |
| useHashRoute | (unchanged) | No signature change; returns extended Route. |

### Mock data (`mockData.ts`)

| Export | Signature | Responsibility |
|--------|-----------|----------------|
| listTasksForWorkspace | `(workspaceId: string) => Task[]` | Return tasks for all projects in the workspace: listProjectsForWorkspace(workspaceId), then flatMap to listTasksForProject(p.id); optionally sort by a stable order (e.g. keep array order). |
| listArtifactsForWorkspace | `(workspaceId: string) => Artifact[]` | Return artifacts for all projects in the workspace: listProjectsForWorkspace(workspaceId), then flatMap to listArtifactsForProject(p.id). |

No new entity types; reuse Task and Artifact. Projects already have workspaceId, so filtering is consistent.

### LeftSidebar (`LeftSidebar.tsx`)

| Prop | Type | Responsibility |
|------|------|----------------|
| workspaceId | string | Current workspace for building routes. |
| projects | Project[] | List to render under “Projects”. |
| selectedProjectId | string \| null | Which project link is active (project/task/artifact routes). |
| route | Route | Used to compute active nav item (workspace → Home, project/task/artifact → Projects, activity → Activity, explore → Explore). |

- Render order: primary nav (Home, Projects, Activity, Explore), then project list under “Projects” (same list and “+ New Project” as today). Each nav item: button or link that calls `navigate(...)` with the appropriate route. Active class when route matches.
- Home → `navigate({ name: "workspace", workspaceId })`
- Projects → navigate to first project or workspace home; “Projects” nav item active when `route.name` is project/task/artifact. Clicking “Projects” could go to workspace home or first project; spec says “Projects list appears” and “active when route is project/task/artifact”, so we only need to highlight “Projects” when one of those is active. Clicking “Projects” can navigate to `#<workspaceId>` (workspace home, where project list is also visible) or leave as “no single destination” and just highlight when a project is selected — **recommended**: clicking “Projects” goes to workspace home so there is a clear target.
- Activity → `navigate({ name: "activity", workspaceId })`
- Explore → `navigate({ name: "explore", workspaceId })`

### ActivityPage (`ActivityPage.tsx`)

| Prop | Type | Responsibility |
|------|------|----------------|
| workspaceId | string | Workspace to show activity for. |

- Call `listTasksForWorkspace(workspaceId)`. For each task, get project name via `getProjectById(task.projectId)`. Render a list: task title, project name, timeLabel, status (reuse Task status display if desired). No pagination; mock list only.

### ExplorePage (`ExplorePage.tsx`)

| Prop | Type | Responsibility |
|------|------|----------------|
| workspaceId | string | Workspace to show content for. |

- Call `listArtifactsForWorkspace(workspaceId)`. For each artifact, get project name via `getProjectById(artifact.projectId)`. Render list with link to `navigate({ name: "artifact", workspaceId, projectId: artifact.projectId, artifactId: artifact.id })`. Show artifact title, kind, project name. Mock only.

### Breadcrumbs (`Breadcrumbs.tsx`)

- If `route.name === "activity"`: crumbs = [Home, Activity]. Home links to `{ name: "workspace", workspaceId }`; Activity is current.
- If `route.name === "explore"`: crumbs = [Home, Explore]. Same pattern.
- No change to other route handling.

### App (`App.tsx`)

- **selectedProjectId**: set only when route has a project: `(route.name === "project" || route.name === "task" || route.name === "artifact") ? route.projectId : null`. Today’s logic uses `route.name !== "workspace"` which would give undefined projectId for activity/explore; fix to the above.
- **renderPage()**: add `case "activity": return <ActivityPage workspaceId={route.workspaceId} />` and `case "explore": return <ExplorePage workspaceId={route.workspaceId} />`.

### AppShell (`AppShell.tsx`)

- Replace `<ProjectsSidebar ... />` with `<LeftSidebar workspaceId={route.workspaceId} projects={projects} selectedProjectId={selectedProjectId} route={route} />`. Props already available; no new props needed. Top Bar unchanged.

## How they work together

1. User loads app; route from hash may be workspace, project, task, artifact, activity, or explore. App validates workspaceId (existing redirect logic).
2. App computes selectedProjectId only for project/task/artifact; passes route, projects, workspaceId to AppShell. AppShell renders TopBar (unchanged) and LeftSidebar with route so nav highlights correctly.
3. LeftSidebar renders Home, Projects, Activity, Explore; then project list. Clicking Home/Activity/Explore navigates to the corresponding hash; clicking a project navigates to project (existing). “Projects” nav item is active when route is project/task/artifact; clicking “Projects” navigates to workspace home.
4. App renderPage() switches on route.name; for activity and explore, renders ActivityPage and ExplorePage with workspaceId. Those pages use listTasksForWorkspace and listArtifactsForWorkspace.
5. Breadcrumbs show Home > Activity or Home > Explore for the new routes; other routes unchanged.
6. Workspace switch: activity and explore hashes include workspaceId, so switching workspace (TopBar) changes workspaceId and redirects to same route name with new id (existing pattern); we only need to ensure parseHash/buildHash for activity/explore use workspaceId, which they do.

## Changes for review

- **Modified** `portal/src/types.ts` — add Route variants `activity` and `explore` with `workspaceId`.
- **Modified** `portal/src/router.ts` — parseHash: handle `/activity` and `/explore` after workspaceId; buildHash: return `#<wid>/activity` and `#<wid>/explore`.
- **Modified** `portal/src/mockData.ts` — add `listTasksForWorkspace(workspaceId)`, `listArtifactsForWorkspace(workspaceId)`.
- **Modified** `portal/src/App.tsx` — selectedProjectId only for project/task/artifact; renderPage() adds cases for activity and explore with ActivityPage and ExplorePage.
- **Modified** `portal/src/components/AppShell.tsx` — use LeftSidebar instead of ProjectsSidebar; pass route, workspaceId, projects, selectedProjectId.
- **New** `portal/src/components/LeftSidebar.tsx` — primary nav (Home, Projects, Activity, Explore) + project list; active state from route; navigate on click.
- **Removed** or **inlined**: `portal/src/components/ProjectsSidebar.tsx` — if we inline the project list into LeftSidebar, remove ProjectsSidebar; otherwise keep as internal sub-component. Design recommends single LeftSidebar with list inlined to avoid two sidebar components.
- **Modified** `portal/src/components/Breadcrumbs.tsx` — add branches for activity and explore (Home > Activity, Home > Explore).
- **New** `portal/src/pages/ActivityPage.tsx` — workspace activity feed (mock tasks + project names).
- **New** `portal/src/pages/ExplorePage.tsx` — workspace browse (mock artifacts + links to artifact viewer).
- **Modified** `portal/src/index.css` — styles for sidebar primary nav (nav items, active state) and any page-specific styles for Activity/Explore pages.
