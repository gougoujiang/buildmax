# Design 071: Side panel redesign and remove Project concept

## Goal (from task)

Redesign the Portal left sidebar and remove the Project concept: tasks are workspace-scoped only. Sidebar provides New Chat, Workspace block (name + switcher + new), and under it Agents and Chats. Workspace home is a simple landing (no project list). Task route is `#<workspaceId>/task/<taskId>`. Default workspace for new users unchanged (no backend change).

---

## 1. Modules and structure

### 1.1 Route and types (portal)

- **Route type** (`lib/types.ts`): Remove `project` route variant. Task route becomes `{ name: "task"; workspaceId: string; taskId: string }` only (no `projectId`). Keep `workspace`, `activity`, `explore`, `agents`, `agentList`.
- **WorkspaceScope** (`lib/types.ts`): Remove `projectId` from scope (or keep optional for backward compat in data layer). Scope for task is `{ workspaceId, taskId }`.
- **Project** and **Task** types: Keep `Project` type for API/artifact compat if needed; `Task.projectId` remains optional. No UI dependency on project for navigation.

### 1.2 Router (hash)

- **parseHash** (`lib/router.ts`): For segment `task`, accept **one** segment after `task`: `#<workspaceId>/task/<taskId>`. Stop parsing `#<workspaceId>/task/<projectId>/<taskId>`. Return `{ name: "task", workspaceId, taskId }`.
- **buildHash** (`lib/router.ts`): For `route.name === "task"`, return `#${route.workspaceId}/task/${route.taskId}`. Remove handling of `project` route.
- **SEGMENT**: Keep `task`; can remove `project` constant if unused.

### 1.3 Workspace helpers

- **getTaskForDetail** (`lib/workspace.ts`): Signature becomes `(workspaceTasks: Task[], taskId: string): Task | undefined`. Find task by `taskId` in `workspaceTasks` only. Callers no longer pass `tasks` (project-scoped) or `projectId`.
- **getProjectById** / **getProjectName**: Keep for now if artifacts or API still reference project; remove from Breadcrumbs and Activity. Can be deleted in a later cleanup if unused.

### 1.4 Sidebar (LeftSidebar)

- **Component**: `LeftSidebar` in `portal/src/components/LeftSidebar.tsx`.
- **New props**:
  - `workspaceId: string`
  - `route: Route`
  - `currentWorkspace: { id: string; name: string }`
  - `workspaces: { id: string; name: string }[]`
  - `onWorkspaceChange: (workspaceId: string) => void`
  - `onNewWorkspace?: () => void`
  - `onNewChat?: () => void`
  - `workspaceTasks: Task[]`
- **Structure (DOM order)**:
  1. **New Chat**: Button/link at top. `onClick` → `onNewChat?.()`. Disabled or hidden if `!onNewChat`.
  2. **Workspace block**: Heading/label with current workspace name; `<select>` for workspace switcher (same options as TopBar); “+” button for `onNewWorkspace`. Use same navigation on change: `onWorkspaceChange(id)` and `navigate({ name: "workspace", workspaceId: id })`.
  3. **Agents**: Link to `{ name: "agents", workspaceId }`. Active when `route.name === "agents" || route.name === "agentList"`.
  4. **Chats**: Sub-list of `workspaceTasks`. Show bounded list (e.g. `workspaceTasks.slice(0, 15)`). Each item: link to `{ name: "task", workspaceId, taskId: task.id }`; label = task.title or “New chat”. Optional “See all” link to `{ name: "activity", workspaceId }`.
  5. **Explore**: Link to `{ name: "explore", workspaceId }`.
  6. **Activity**: Link to `{ name: "activity", workspaceId }`.
- **Styling**: Reuse `sidebar`, `sidebar__nav`, `sidebar__nav-item`; add classes e.g. `sidebar__new-chat`, `sidebar__workspace`, `sidebar__workspace-select`, `sidebar__chats`, `sidebar__chat-item`. Keep aria and keyboard support.

### 1.5 AppShell and App

- **AppShell** (`components/AppShell.tsx`): Extend props with `workspaceTasks: Task[]`, `onNewChat?: () => void`. Pass to `LeftSidebar` with `currentWorkspace`, `workspaces`, `onWorkspaceChange`, `onNewWorkspace`, `onNewChat`, `workspaceTasks`, `route`, `workspaceId`. Pass `projects` to Breadcrumbs only if Breadcrumbs still need them (see below); otherwise can stop passing.
- **App** (`App.tsx`): In `AppContent`, get `workspaceTasks` and `refetchWorkspaceTasks` from `useWorkspace()`. Define `handleNewChat`: call `createTask(workspaceId, { input: "" }, token)` (no `project_id`); on success, get `task` from response, then `navigate({ name: "task", workspaceId, taskId: task.id })` and `refetchWorkspaceTasks()`. On failure, keep user on current page (optional: show error). Pass `workspaceTasks` and `onNewChat={handleNewChat}` to `AppShell`.

### 1.6 WorkspaceRouter

- **Routes to handle**: `workspace`, `activity`, `explore`, `agents`, `agentList`, `task`. Remove `project` branch entirely.
- **workspace**: Render new **WorkspaceHome** component (see below), not Projects. No project list.
- **task**: Resolve task with `getTaskForDetail(workspaceTasks, route.taskId)`. If not found, redirect to workspace home or Activity. Render `<TaskDetail task={task} workspaceId={route.workspaceId} onRefetch={...} />` (no `projectId`).
- **Remove**: `fallbackProject`, Project view, any `getProjectById`/project resolution. Remove imports for `Project` view and `getProjectById` for route logic.
- **Activity**: Pass `workspaceTasks` and `artifacts`; remove `getProjectName`. Activity component updated to not use project (see below).
- **Agents**: Remove `getProjectName` prop; Agents page updated to not show project (see below).

### 1.7 WorkspaceHome (workspace landing)

- **New or repurposed component**: Replace “Projects” as the content of `route.name === "workspace"`. Options: (A) New `WorkspaceHome.tsx` with prompt + recent chats/artifacts, or (B) Simplify existing `Projects.tsx` by removing project list and create-project UI, keeping only prompt + recent artifacts.
- **Recommended**: New `WorkspaceHome.tsx`. Props: `workspaceId`, `workspaceTasks` (or pass from context), `artifacts`, `token`, `onRefetchWorkspaceTasks`, `onViewArtifact?`. Content: optional prompt area (“Start a new chat” that could focus sidebar New Chat or create task and navigate); list of recent workspace tasks (chats) with links to task detail; optional recent artifacts. No project list, no “New project” or CreateProjectModal.
- **Router**: For `route.name === "workspace"`, render `<WorkspaceHome ... />` with data from `useWorkspace()`.

### 1.8 Task detail

- **TaskDetail** (`pages/TaskDetail.tsx`): Remove `projectId` from props (or keep optional and do not use). All links and polling use `workspaceId` + `taskId` only. Polling: `getTasks(workspaceId, token)` (no third argument). Navigation back: e.g. to workspace or activity using `navigate({ name: "workspace", workspaceId })` or activity.

### 1.9 Breadcrumbs

- **Breadcrumbs** (`components/Breadcrumbs.tsx`): Remove project from crumb chain. For `task` route: e.g. “Chats” (or workspace name) → task title or “Task”. For `workspace`: single crumb or “Home”. No `projects` prop needed if we never show project in crumbs; can keep prop for type compat and pass empty array, or remove and simplify crumb logic to never reference project.
- **Props**: `route`, optionally `projects` (can be removed), and for task label we need task title — either pass `task?: Task` or keep a simple “Task” label. Prefer passing current task when on task route so breadcrumb shows title.

### 1.10 Activity and Agents pages

- **Activity** (`pages/Activity.tsx`): Remove `getProjectName` prop. Show all `tasks` (workspace tasks); each task links to `navigate({ name: "task", workspaceId, taskId: task.id })`. Do not filter by `task.projectId`; show all workspace tasks. Subtitle text: e.g. “All tasks and artifacts in this workspace.” Remove project name from each row.
- **Agents** (`pages/Agents.tsx`): Remove `getProjectName` prop. When displaying tasks (e.g. in agent run list), do not show project; show only task title and meta (time, status). If task has no title, use “Task” or “New chat”.

### 1.11 Data and hooks

- **useWorkspaceData** (`hooks/useWorkspaceData.ts`): No longer derive `projectIdFromRoute` from route for navigation (route has no projectId). Optional cleanup: call `useTasks(workspaceId, token, undefined)` or rely only on `useWorkspaceTasks` for task list to avoid duplicate fetch; either is fine. Keep `useProjects` if artifacts or other code still filter by project; otherwise can leave as-is for minimal change.
- **WorkspaceContext**: No change to provider; it already exposes `workspaceTasks` and `projects`. Consumers stop using project for nav.

### 1.12 Backend and default workspace

- **No backend change** for default workspace: GET /api/workspaces continues to call `EnsureDefaultWorkspaceForUser` (already in place). No change to task create/list API; Portal simply omits `project_id` when creating tasks and does not filter by project in UI.

---

## 2. Method and contract summary

| Where | What | Contract |
|-------|------|----------|
| `lib/types.ts` | Route type | Drop `project`; task = `{ name: "task", workspaceId, taskId }`. |
| `lib/router.ts` | parseHash | task: `parts[1]==="task" && parts[2]` → `{ name: "task", workspaceId, taskId: parts[2] }`. |
| `lib/router.ts` | buildHash | task: `#${workspaceId}/task/${taskId}`; remove project case. |
| `lib/workspace.ts` | getTaskForDetail | `(workspaceTasks, taskId) => Task \| undefined`. |
| `LeftSidebar` | Props | workspaceId, route, currentWorkspace, workspaces, onWorkspaceChange, onNewWorkspace, onNewChat?, workspaceTasks. |
| `AppShell` | Props | Add workspaceTasks, onNewChat?; pass through to LeftSidebar. |
| `AppContent` | handleNewChat | createTask(ws, { input: "" }, token) → navigate(task) + refetchWorkspaceTasks. |
| `WorkspaceRouter` | Branch task | getTaskForDetail(workspaceTasks, route.taskId); TaskDetail without projectId. |
| `WorkspaceRouter` | Branch workspace | Render WorkspaceHome (no project list). |
| `WorkspaceRouter` | Remove | project branch, fallbackProject, Project view. |
| `Breadcrumbs` | Crumbs for task | No project; e.g. Chats → task title. |
| `TaskDetail` | Props / polling | projectId optional or removed; getTasks(workspaceId, token). |
| `Activity` | Props / links | No getProjectName; link with { name: "task", workspaceId, taskId }. |
| `Agents` | Props | No getProjectName. |

---

## 3. How they work together

1. User opens app → hash may be `#<workspaceId>` or `#<workspaceId>/task/<taskId>`. parseHash returns Route (no project).
2. AppContent loads workspaces (default workspace ensured by backend); passes workspaceTasks and onNewChat to AppShell. AppShell passes them to LeftSidebar.
3. LeftSidebar shows New Chat, Workspace (name + select + “+”), Agents, Chats (from workspaceTasks), Explore, Activity. New Chat triggers onNewChat → createTask (no project_id) → navigate to new task.
4. WorkspaceRouter: workspace → WorkspaceHome; task → getTaskForDetail(workspaceTasks, taskId) → TaskDetail; no project route.
5. All task links (sidebar Chats, Activity, WorkspaceHome) use `navigate({ name: "task", workspaceId, taskId })`. buildHash produces `#<workspaceId>/task/<taskId>`.
6. Breadcrumbs for task show no project; TaskDetail and polling use workspaceId + taskId only. Default workspace behavior unchanged (GET workspaces on first load).

---

## 4. Changes for review

| Package / file | Change |
|----------------|--------|
| `portal/src/lib/types.ts` | Remove `project` from Route; task route = workspaceId + taskId only. WorkspaceScope: drop projectId or keep optional. |
| `portal/src/lib/router.ts` | parseHash: task with one segment (taskId); buildHash: task two segments; remove project. |
| `portal/src/lib/workspace.ts` | getTaskForDetail(workspaceTasks, taskId) only. |
| `portal/src/components/LeftSidebar.tsx` | New layout and props: New Chat, Workspace block, Agents, Chats list, Explore, Activity. |
| `portal/src/components/AppShell.tsx` | Add workspaceTasks, onNewChat; pass to LeftSidebar; Breadcrumbs: pass task for label or simplify. |
| `portal/src/App.tsx` | handleNewChat; pass workspaceTasks and onNewChat to AppShell. |
| `portal/src/components/WorkspaceRouter.tsx` | Remove project; workspace → WorkspaceHome; task by taskId only; Activity/Agents without getProjectName. |
| `portal/src/pages/WorkspaceHome.tsx` | New: workspace landing (prompt + recent chats/artifacts), no project list. |
| `portal/src/pages/TaskDetail.tsx` | projectId optional/removed; polling getTasks(workspaceId, token). |
| `portal/src/components/Breadcrumbs.tsx` | No project in crumbs; task crumb = Chats + task title (or pass task). |
| `portal/src/pages/Activity.tsx` | No getProjectName; all workspace tasks; link task with { name: "task", workspaceId, taskId }. |
| `portal/src/pages/Agents.tsx` | Remove getProjectName; do not show project in task rows. |
| `portal/src/hooks/useWorkspaceData.ts` | Optional: projectIdFromRoute always undefined; rely on useWorkspaceTasks only. |
| `portal/src/index.css` (or sidebar CSS) | New classes for sidebar structure (new-chat, workspace block, chats list). |
| Backend | No change (default workspace and task API unchanged). |
