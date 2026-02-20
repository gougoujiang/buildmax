# Architecture Refactor Opportunities — portal/

**Scope:** `portal/` (React frontend and its boundary with the Go server API).  
**Focus:** Package/module boundaries, dependency direction, layering, and structural clarity.  
**Date:** 2025-02-20.

---

## Summary

Analysis of the Portal found **five** architecture-level opportunities: (1) split the monolithic `api.ts` into transport, DTOs, and mappers; (2) introduce a shared API contract (OpenAPI-driven or documented DTOs) between server and portal; (3) reduce prop drilling with a workspace-scoped context; (4) decompose `useWorkspaceData` into smaller, single-purpose hooks; (5) clarify the boundary between “route” and “workspace scope” so pages consume a single abstraction. The current layout (lib vs pages vs components, AuthContext, hash routing) is coherent; the main improvements are clearer layering inside the frontend and a single source of truth for the API contract.

---

## 1. Split api.ts into transport, DTOs, and mappers

| Field | Content |
|-------|--------|
| **Title** | Split api.ts into transport, DTOs, and mappers |
| **Scope** | `portal/src/lib/api.ts` (and new modules under `lib/` or `lib/api/`) |
| **Current state** | `api.ts` is a single ~360-line file that mixes: (a) **transport**: `getApiBase()`, `checkUnauthorized`, `throwIfNotOk`, `UNAUTHORIZED_EVENT`, and every `fetch` call; (b) **API DTOs**: `ApiWorkspace`, `ApiProject`, `ApiTask`, `ApiArtifact`, `ApiSession`, etc. (snake_case, matching server JSON); (c) **UI mappers and formatting**: `formatRelativeTime`, `apiArtifactToArtifact`, `apiTaskToTask`, `apiProjectToProject`, `taskStatusToUI`. So the “API layer” both talks to the server and knows about UI types (`Artifact`, `Task`, `Project` from `types.ts`), and callers (e.g. `useWorkspaceData`, pages) import one big module. |
| **Proposed change** | Split into three clear layers under `portal/src/lib/`: (1) **api/client.ts** (or **api/transport.ts**): base URL, 401 handling, `throwIfNotOk`, and a small `request()` helper that adds auth and handles errors; no DTOs or UI types. (2) **api/types.ts**: all `Api*` interfaces (request/response shapes) used by the server; no UI types. (3) **api/mappers.ts** (or keep in **api.ts** as a thin barrel): `formatRelativeTime`, `api*To*` functions that take `Api*` and return UI types from `types.ts`; this module imports both `api/types` and `types`. The existing **api.ts** can become a barrel that re-exports from client, types, and mappers, plus the public fetch functions that use client + types and return DTOs (or optionally already-mapped UI types where that’s the only consumer). Pages and hooks then import from `api` or from the submodules as needed. |
| **Benefit** | Clear separation: transport is testable without UI; DTOs are the single place for “what the server returns”; mappers are the only place that ties API shape to UI shape. Easier to add new endpoints or change response shapes without touching UI code. |
| **Impact** | Medium |

---

## 2. Introduce a shared API contract (server ↔ portal)

| Field | Content |
|-------|--------|
| **Title** | Introduce a shared API contract (server ↔ portal) |
| **Scope** | `internal/server` (Go response structs), `portal/src/lib/api` (TypeScript DTOs), and optionally OpenAPI spec |
| **Current state** | Server defines response types in Go (e.g. `WorkspaceResponse`, `ProjectResponse`, `TaskResponse`, `ArtifactResponse`, `fileNode`) with `json:` tags. The portal defines parallel TypeScript interfaces (`ApiWorkspace`, `ApiProject`, etc.) that must be kept in sync manually. There is no single source of truth: adding or renaming a field on one side can drift. The server already exposes `GET /openapi.json` and Swagger UI, but the portal does not use them for code generation. |
| **Proposed change** | **Option A:** Treat OpenAPI as the contract. Maintain (or generate) an OpenAPI spec that describes the user-facing API (workspaces, projects, tasks, artifacts, files, upload, auth). Use a code generator (e.g. openapi-typescript, or a small script) to produce TypeScript types and optionally client stubs from the spec. Server continues to serialize with the same JSON tags; ensure the spec matches. **Option B:** If codegen is not desired, introduce a single document (e.g. `design/004-portal-api-contract.md` or a `.vibe/kb` doc) that lists each endpoint and the exact request/response shape (field names, types). Both server and portal implementations reference this doc; any change requires updating the doc first. Prefer Option A if the team is willing to maintain the spec; otherwise Option B reduces drift by making the contract explicit and reviewable. |
| **Benefit** | Single source of truth for the API surface; no silent drift between Go and TypeScript; safer refactors and new fields. |
| **Impact** | Medium |

---

## 3. Reduce prop drilling with a workspace-scoped context

| Field | Content |
|-------|--------|
| **Title** | Reduce prop drilling with a workspace-scoped context |
| **Scope** | `portal/src/App.tsx`, `portal/src/components/WorkspaceRouter.tsx`, `portal/src/components/AppShell.tsx`, and pages (e.g. `Project`, `TaskDetail`, `Explore`, `Activity`) |
| **Current state** | After login, `AppContent` holds route and the full result of `useWorkspaceData` (workspaces, projects, tasks, workspaceTasks, artifacts, loadingWorkspaces, and six refetch callbacks). It passes a large subset of these as props to `AppShell` and `WorkspaceRouter`. `WorkspaceRouter` receives route, projects, tasks, workspaceTasks, artifacts, token, and four callback props; it switches on route and renders pages, passing again token, workspaceId, refetch callbacks, and data. Pages like `TaskDetail`, `Project`, `Explore` each receive `workspaceId`, `token`, and various refetch/data props. So the same “workspace scope” (current workspace id, token, and ways to refetch) is threaded through many layers. |
| **Proposed change** | Introduce a **WorkspaceScope** (or **WorkspaceContext**) provider that wraps the authenticated app content below `AuthProvider`. The context value includes: current `workspaceId` (from route), `token`, and a small **workspace API** object (e.g. `refetchProjects`, `refetchTasks`, `refetchArtifacts`, `refetchWorkspaceTasks`, and optionally cached lists like `projects`, `tasks` if they are always needed). The provider can consume `useHashRoute` and `useWorkspaceData` (or the decomposed hooks from opportunity 4) and expose only what children need. Then `AppShell` and `WorkspaceRouter` no longer need to pass token and refetch callbacks down; pages that need them call `useWorkspaceScope()` (or similar). Reduce props on `WorkspaceRouter` to route and the data needed for routing (e.g. projects, tasks, workspaceTasks, artifacts) if still needed for fallbacks, or move that data into the context as well so the router only receives route and renders pages that pull data from context. |
| **Benefit** | Fewer props through the tree; a single place that defines “what is in scope” for the current workspace; easier to add new refetch or scoped data without touching every intermediate component. |
| **Impact** | Medium |

---

## 4. Decompose useWorkspaceData into smaller hooks

| Field | Content |
|-------|--------|
| **Title** | Decompose useWorkspaceData into smaller hooks |
| **Scope** | `portal/src/hooks/useWorkspaceData.ts`, `portal/src/hooks/useAsyncList.ts`, and call sites (`App.tsx`) |
| **Current state** | `useWorkspaceData(token, route)` does everything: it runs four `useAsyncList` calls (workspaces, projects, tasks, workspaceTasks) with different enabled conditions and mappers, plus a separate `useState` + `useEffect` + `useCallback` for artifacts. It returns 14 values (lists, loading flag, six refetch functions). The hook is the single aggregation point for all workspace-scoped list data, so it has high cohesion but also high complexity and mixed concerns (workspaces are raw `ApiWorkspace[]`; projects/tasks/artifacts are mapped to UI types). Adding a new list (e.g. “recent runs”) would further grow this hook. |
| **Proposed change** | Split into focused hooks: e.g. `useWorkspaces(token)`, `useProjects(workspaceId, token)`, `useTasks(workspaceId, token, projectId?)`, `useWorkspaceTasks(workspaceId, token)`, `useArtifacts(workspaceId, token, options?)`. Each hook uses `useAsyncList` (or a single `useFetch`-style hook) and returns `{ data, loading?, refetch }`. Then either: (a) keep a thin **useWorkspaceData** that calls these hooks with the right arguments from `route` and returns the combined result (so App doesn’t need to change much), or (b) have App (or a WorkspaceScope provider) call the smaller hooks directly and pass only what each subtree needs. Prefer (a) as a first step to avoid a large refactor of App/WorkspaceRouter; the internal decomposition still improves testability and single responsibility. |
| **Benefit** | Each hook has one job; easier to test and reuse (e.g. a page that only needs projects can use `useProjects`); artifacts can follow the same pattern as the others instead of custom state. |
| **Impact** | Low–Medium |

---

## 5. Clarify route vs workspace scope boundary

| Field | Content |
|-------|--------|
| **Title** | Clarify route vs workspace scope boundary |
| **Scope** | `portal/src/lib/types.ts` (Route), `portal/src/lib/router.ts`, and consumers of route/workspaceId |
| **Current state** | **Route** is a discriminated union (`workspace` \| `project` \| `task` \| `activity` \| `explore`) and always carries `workspaceId`; for `project` and `task` it also carries `projectId` and optionally `taskId`. Many components receive both `route` and `workspaceId` (or only `workspaceId`), and some derive `projectId` from `route`. So “current workspace” and “current route” are two concepts that are related but not unified: e.g. Explore only needs `workspaceId` and gets it via props; TaskDetail needs `workspaceId`, `projectId`, `taskId`. There is no single abstraction that says “here is the current workspace scope and the current sub-view.” |
| **Proposed change** | Define a **WorkspaceScope** type (or extend the context from opportunity 3) that represents “what is in scope for the current view”: at minimum `workspaceId`; optionally `projectId` and `taskId` when the route is project or task. Pages and components that only need “current workspace” depend on `workspaceId` (or a context that provides it); components that need “current project” or “current task” get that from the same scope. The **Route** type remains the source of truth for the URL and navigation; the scope is derived from route (and possibly from resolved entity ids). Document that “route” drives the URL and “scope” is the derived context for data fetching and display, so that new contributors don’t duplicate workspaceId in multiple props when a single scope object would do. This can be done in small steps: e.g. add a `useWorkspaceScope(route)` that returns `{ workspaceId, projectId?, taskId? }` and gradually migrate call sites to use it instead of passing workspaceId (and projectId, taskId) separately. |
| **Benefit** | Clear mental model: route = where we are in the app; scope = what we’re allowed to access and what we’re looking at. Reduces duplication of workspaceId/projectId/taskId across props. |
| **Impact** | Low |

---

## What is already in good shape

- **Auth boundary:** `AuthContext` provides token and user; login/logout and 401 handling are centralized; pages that need auth use `useAuth()`. No auth logic scattered across components.
- **Routing:** Hash-based routing in `router.ts` with a single `Route` type and `parseHash`/`buildHash`/`useHashRoute` is clear and easy to follow.
- **Lib vs pages vs components:** `lib/` holds types, API, router, and pure helpers (`workspace.ts`, `explore.ts`); pages are top-level views; components are reusable UI. No circular dependencies.
- **Server API surface:** Workspace-scoped URLs and JWT auth are consistent; server and portal both use snake_case for JSON. The main gap is the lack of a single contract definition (addressed in opportunity 2).
- **Explore tree:** Server returns a tree (`fileNode`); portal types (`ExploreNode`) and helpers (`explore.ts`) align with that shape. No duplication of tree logic.

---

*Proposals are for review and implementation in follow-up tasks; no code changes in this document.*
