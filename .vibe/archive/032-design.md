# Design 032 — Portal layout

## Goal

Implement a **Portal app shell** (top bar + projects sidebar + main content) plus 4 mock-data pages (Workspace Home, Project Dashboard, Task Detail, Artifact Viewer) with **minimal navigation and no new dependencies**.

## Modules

| Module | Responsibility | Owns |
|--------|----------------|------|
| **portal/src** | Portal UI, navigation state, and mock data | App shell, pages, hash routing, types, mock data, CSS |
| **portal/** (root) | Vite entry/build wiring | `index.html`, `main.tsx`, `package.json` (unchanged) |

## Structure

**Directory / files**

- `portal/src/`
  - `App.tsx` — app entry: resolves route, loads mock data, renders app shell + current page
  - `index.css` — extend styles to support shell layout (top/sidebar/main) + page-specific blocks
  - `types.ts` — shared types: Workspace/Project/Task/Artifact + route types
  - `mockData.ts` — mock workspace/projects/tasks/artifacts + small lookup helpers
  - `router.ts` — tiny hash router: parse/build/navigate + `useHashRoute()` hook
  - `components/`
    - `AppShell.tsx` — layout frame: top bar + sidebar + main slot
    - `TopBar.tsx` — branding + workspace switcher placeholder + profile/settings placeholder
    - `ProjectsSidebar.tsx` — project list (mock) + “+ New Project” placeholder
    - `Breadcrumbs.tsx` — simple breadcrumb/back for navigation context
  - `pages/`
    - `WorkspaceHome.tsx` — project list + quick actions + “prompt” section (mock)
    - `ProjectDashboard.tsx` — overview + prompt + latest result + artifacts + tasks/activity list
    - `TaskDetail.tsx` — result + what changed + evidence + restore placeholder
    - `ArtifactViewer.tsx` — shows selected artifact content (mock)

**Notes on existing components**

- The existing landing components (`Header.tsx`, `PromptArea.tsx`, `RecentActivity.tsx`, `mockActivity.ts`) can be:
  - **Reused** inside `WorkspaceHome` (preferred if it reduces churn), or
  - **Replaced** by new shell/page components if their structure no longer fits.
  
This design assumes we **keep** `PromptArea` and `RecentActivity` as generic components, and replace `Header` with `TopBar` inside the shell.

**Main types (TypeScript)**

- **Workspace**: `{ id: string; name: string }`
- **Project**: `{ id: string; workspaceId: string; name: string; status: "active" | "paused" | "archived"; updatedAtLabel: string }`
- **Task**: `{ id: string; projectId: string; title: string; status: "running" | "success" | "failed" | "canceled"; timeLabel: string; summary: string }`
- **Artifact**: `{ id: string; projectId: string; title: string; kind: "report" | "chart" | "data" | "other"; preview: string }`
- **Route** (hash-based):
  - `{"name":"workspace"}`
  - `{"name":"project","projectId":string}`
  - `{"name":"task","projectId":string,"taskId":string}`
  - `{"name":"artifact","projectId":string,"artifactId":string}`

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| (router) | parseHash | `(hash: string) => Route` | Parse `window.location.hash` into a typed `Route`. Unknown hashes fall back to `{"name":"workspace"}`. |
| (router) | buildHash | `(route: Route) => string` | Convert a `Route` into a canonical hash string (e.g. `#project/p1`). |
| (router) | navigate | `(route: Route) => void` | Set `window.location.hash = buildHash(route)`. |
| (router hook) | useHashRoute | `() => Route` | React hook that returns the current `Route` and updates on `hashchange`. |
| (mockData) | getProjectById | `(projectId: string) => Project \| undefined` | Lookup helper for pages; no side effects. |
| (mockData) | listProjectsForWorkspace | `(workspaceId: string) => Project[]` | Used by sidebar and workspace home. |
| (mockData) | listTasksForProject | `(projectId: string) => Task[]` | Used by project dashboard (activity list). |
| (mockData) | listArtifactsForProject | `(projectId: string) => Artifact[]` | Used by project dashboard (artifacts list). |
| (mockData) | getTaskById | `(projectId: string, taskId: string) => Task \| undefined` | Used by task detail route resolution. |
| (mockData) | getArtifactById | `(projectId: string, artifactId: string) => Artifact \| undefined` | Used by artifact viewer route resolution. |

## How they work together

**Data/control flow**

1. `main.tsx` renders `App`.
2. `App` calls `useHashRoute()` to obtain the current `Route`.
3. `App` loads mock workspace/projects/tasks/artifacts from `mockData.ts`.
4. `App` resolves route params to selected entities (project/task/artifact). If missing, it falls back safely (e.g. unknown id → workspace view).
5. `App` renders:
   - `AppShell`
     - `TopBar` (workspace switcher placeholder + profile placeholder)
     - `ProjectsSidebar` (projects list; clicking navigates to `#project/<id>`)
     - `Breadcrumbs` (shows current context; provides “Back” as `navigate(...)`)
     - main content: one of the `pages/*` components based on `Route`
6. Page components render mock sections:
   - `ProjectDashboard` renders a task list and artifacts list; clicking items navigates to `#task/...` / `#artifact/...`.
   - `TaskDetail` shows “Restore” as a placeholder button (disabled or no-op).

**Dependencies**

- `App.tsx` depends on `router.ts`, `mockData.ts`, and `types.ts`.
- `pages/*` depend on `types.ts` (and may use `PromptArea`/`RecentActivity` if reused).
- `components/*` depend on `types.ts` and `router.ts` (for navigation callbacks).
- No package depends on Go code; `portal/` remains standalone.

**Key data structures**

- `Route`: created by `useHashRoute()` (from URL), consumed by `App` to decide which page to render.
- Mock entity arrays: created in `mockData.ts`, consumed by sidebar and pages.

## Changes for review

- **New** `.vibe/032-design.md` — portal layout technical design.
- **New** `portal/src/router.ts` — minimal hash router + hook.
- **New** `portal/src/types.ts` — route + entity types.
- **New** `portal/src/mockData.ts` — mock workspace/projects/tasks/artifacts + lookup helpers.
- **New** `portal/src/components/AppShell.tsx` — top/sidebar/main layout frame.
- **New** `portal/src/components/TopBar.tsx` — brand + workspace switcher placeholder + profile placeholder.
- **New** `portal/src/components/ProjectsSidebar.tsx` — mock projects list navigation.
- **New** `portal/src/components/Breadcrumbs.tsx` — simple breadcrumb/back UI.
- **New** `portal/src/pages/*` — 4 pages: workspace/project/task/artifact views.
- **Modified** `portal/src/App.tsx` — switch from single landing to app shell + routing.
- **Modified** `portal/src/index.css` — add shell layout + page styles.
- **Modified (optional)** existing `portal/src/Header.tsx` / `PromptArea.tsx` / `RecentActivity.tsx` — reuse or adjust; otherwise left as-is and unused.
