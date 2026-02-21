# Smell Proposal - portal

## Scope

**Analyzed**

- Paths / packages: `portal/` (React + Vite + TypeScript frontend)
- Criteria: Tactical, local code smells under `portal/src`: duplication, long components, unclear names, mixed concerns, repeated patterns, testability.

## Summary

- **Proposals**: 8
- **High priority**: 2
- **Overview**: Main findings are duplicated “relative time” formatting in the API layer, a large `AppContent` with embedded routing and helpers, repeated fetch-then-set state patterns in the data hook and pages, duplicated create-modal and artifact-view patterns, and inline helpers that could be extracted for clarity and reuse.

---

## Proposals

### 1. Deduplicate relative time formatting in api.ts

**Location**: `portal/src/lib/api.ts` — `artifactTimeLabel` (lines 341–353), `taskTimeLabel` (lines 334–347)

**Current state**

- Two nearly identical functions: both take a timestamp (seconds or from `ApiTask`), build “Today HH:MM”, “Yesterday HH:MM”, or full `toLocaleString()` based on comparing `toDateString()` with today/yesterday. The only difference is the input shape (number vs `ApiTask` with `ended_at ?? created_at`). This duplicates logic and makes future i18n or format changes harder.

**Proposed change**

- Export a single `formatRelativeTime(secondsSinceEpoch: number): string` (or keep a private helper and have `artifactTimeLabel` and `taskTimeLabel` call it with the appropriate number). Remove the duplicated if/else blocks.

**Benefit**: Single place for relative time formatting; easier to change format or add locale later.

**Priority**: Medium

---

### 2. Extract page router from App.tsx into a dedicated component or hook

**Location**: `portal/src/App.tsx` — `AppContent`, especially `renderPage()` and inline helpers (lines 65–170)

**Current state**

- `AppContent` is long and mixes: auth gate, workspace redirect, workspace/workspace-data consumption, and a large `renderPage()` that switches on `route.name` and builds different page trees. Helpers like `getProjectById`, `getProjectName`, `getTaskForDetail`, `onWorkspaceChange`, `handleNewWorkspace`, `handleCreateWorkspace` live in the same component. `fallbackHome` and `fallbackProject` are defined inside `renderPage()` and close over many props. This makes the component hard to scan and test; routing and “which page to show” are intertwined with data and callbacks.

**Proposed change**

- Extract a `WorkspaceRouter` (or `MainContent`) component that receives `route`, workspace data (projects, tasks, artifacts, refetch callbacks), and a single `onViewArtifact(workspaceId, artifactId)` callback. Move `renderPage()` logic into this component so `AppContent` only handles: auth, redirect when no workspace, and composing `AppShell` + `WorkspaceRouter` + modals. Optionally extract `getProjectById` / `getProjectName` / `getTaskForDetail` into a small `useWorkspaceLookup(projects, tasks, workspaceTasks)` hook or pure helpers in a `lib/workspace.ts` file so `AppContent` stays thin.

**Benefit**: Clearer separation of “layout + auth + redirect” vs “route → page”; easier to test routing and to add new routes.

**Priority**: High

---

### 3. Reduce duplication in useWorkspaceData (refetch + useEffect per resource)

**Location**: `portal/src/hooks/useWorkspaceData.ts` — refetch callbacks and useEffects for workspaces, projects, tasks, workspaceTasks, artifacts (lines 41–135)

**Current state**

- For each resource (workspaces, projects, tasks, workspaceTasks, artifacts) the pattern is repeated: a `refetchX` callback that checks token (and sometimes route), calls an API, then `setX` with mapped result or `[]` on catch; and a `useEffect` that does the same when token/route deps change, plus clearing state when deps are invalid. This is five similar blocks with only the API function and state setter differing. The hook is long and repetitive.

**Proposed change**

- Introduce a small internal helper, e.g. `function useFetchOnDeps<T, D>(deps: D[], fetchFn: (deps: D) => Promise<T[]>, map: (raw: T[]) => unknown[], guard: (deps: D) => boolean): [state, refetch]`, or a simpler pattern: one generic “fetch when deps valid, else clear” helper used per resource so the effect and refetch share the same fetch logic. Alternatively, keep the current structure but extract the repeated “if !guard return; setLoading?; fetch().then(map).catch([]).finally(…)” into a single `useAsyncResource`-style helper to cut duplication.

**Benefit**: Less duplication, easier to add a new workspace-scoped resource, and a single place to adjust loading/error behavior if needed.

**Priority**: Medium

---

### 4. Unify create-modal pattern (CreateWorkspaceModal / CreateProjectModal)

**Location**: `portal/src/components/CreateWorkspaceModal.tsx`, `portal/src/components/CreateProjectModal.tsx`

**Current state**

- Both modals share the same structure: `open`, `loading`, `error`, `onClose`, `onCreate`; reset local state when `open` becomes true; form with label + input(s), error paragraph, Cancel + submit button with loading state. CreateProjectModal adds a second field (description). The structure and class names (`modal__body`, `modal__label`, `modal__error`, `modal__actions`, `modal__btn`) are duplicated. No shared “create entity modal” abstraction.

**Proposed change**

- Extract a generic `CreateEntityModal` (or keep both but introduce a shared `ModalForm` presentational component) that accepts: title, fields (array of { id, label, type, placeholder, optional, value, onChange, maxLength }), error, loading, submitLabel, onClose, onSubmit. CreateWorkspaceModal and CreateProjectModal become thin wrappers that define fields and call the shared component. Alternatively, a single `CreateResourceModal` with a discriminated union on resource type (“workspace” | “project”) and field config to avoid two almost-identical files.

**Benefit**: One place for modal layout and behavior; consistent UX and easier to add “create X” modals later.

**Priority**: Low

---

### 5. Centralize “onViewArtifact” callback shape and usage

**Location**: `portal/src/App.tsx` (lines 114, 126, 138), `portal/src/pages/Projects.tsx`, `portal/src/pages/Project.tsx`, `portal/src/pages/Activity.tsx` — `onViewArtifact?: (artifactId: string) => void` and parent setting `setViewArtifact({ workspaceId, artifactId })`

**Current state**

- Multiple pages receive `onViewArtifact(artifactId: string)` and the parent (App) has to close over `route.workspaceId` to build `{ workspaceId, artifactId }` for the modal. The callback shape only carries artifact id; workspace id is inferred from route in the parent. That’s consistent but repeated in three places (Projects, Project, Activity) with the same inline arrow.

**Proposed change**

- Either keep the current shape and document that “parent must bind workspaceId when passing onViewArtifact”, or change to `onViewArtifact?: (params: { workspaceId: string; artifactId: string }) => void` so pages can pass both when they have workspace in scope (reducing parent’s need to close over route). Then in App, pass a single `handleViewArtifact` that accepts that object and call it from all three pages with consistent arguments. Not a large change but reduces repeated “artifactId => setViewArtifact({ workspaceId: route.workspaceId, artifactId })” and makes the contract explicit.

**Benefit**: Clearer API contract, less repeated closure code, easier to add more artifact viewers (e.g. from TaskDetail) later.

**Priority**: Low

---

### 6. Extract Explore tree and file logic into smaller components/hooks

**Location**: `portal/src/pages/Explore.tsx` — `TreePanel`, `findNodeById`, `getChildren`, and the main component (lines 10–341)

**Current state**

- Explore.tsx is long (~340 lines) and contains: type guards and tree helpers (`isFolder`, `findNodeById`, `getChildren`), a recursive `TreePanel` component, and a large `Explore` component with many useState/useCallback (tree, loading, error, expandedIds, selectedFolderId, selectedFileId, fileContent, uploading, uploadMsg). File and folder upload handlers are similar. The component does tree rendering, selection, file content fetching, and upload in one place. Hard to test tree logic in isolation and the file is a single large unit.

**Proposed change**

- Move `isFolder`, `findNodeById`, `getChildren` to `lib/explore.ts` (or `lib/tree.ts`) so they can be unit-tested and reused. Optionally extract `TreePanel` to `components/ExploreTree.tsx`. Extract a `useFileTree(workspaceId, token)` hook that returns `{ tree, loading, error, refetch }`, and a `useFileContent(workspaceId, token)` or inline “load content when selectedFileId changes” so the page component mainly composes hooks and layout. Consider a small `useUpload(workspaceId, token, onSuccess)` that handles both file and folder upload and message state to deduplicate handleUpload and handleFolderUpload.

**Benefit**: Smaller, testable units; clearer separation of tree logic, data fetching, and UI; easier to reuse tree or upload elsewhere.

**Priority**: Medium

---

### 7. Reusable “fetch with loading/error and cancel” pattern

**Location**: `portal/src/pages/TaskDetail.tsx` (session fetch, lines 29–57), `portal/src/components/ArtifactContentModal.tsx` (items and content fetch, lines 29–82), `portal/src/pages/Explore.tsx` (fetchTree and file content)

**Current state**

- Several places use the same pattern: useEffect with `cancelled` flag, setLoading(true) and setError(null), call API, in .then/.catch check !cancelled and set state, .finally setLoading(false), return cleanup that sets cancelled=true. This is repeated in TaskDetail (session), ArtifactContentModal (items + content), and Explore. No shared hook or helper for “fetch on deps, support cancel, loading and error state”.

**Proposed change**

- Add a small hook, e.g. `useFetch<T>(fetchFn: () => Promise<T>, deps: unknown[])` that returns `{ data: T | null, loading: boolean, error: string | null, refetch: () => void }`, handles cancel on unmount or deps change, and sets loading/error consistently. Use it in TaskDetail for session, in ArtifactContentModal for items and for content (two instances), and optionally in Explore for tree and file content. This reduces boilerplate and ensures consistent behavior (e.g. no setState after unmount).

**Benefit**: Less duplication, consistent loading/error/cancel behavior, easier to add new “fetch on deps” features.

**Priority**: High

---

### 8. Replace createTaskRun 409 parsing with shared error parsing

**Location**: `portal/src/lib/api.ts` — `createTaskRun` (lines 258–268) inlines JSON parse and error message extraction; `throwIfNotOk` (lines 21–30) does similar parsing for non-ok responses.

**Current state**

- `throwIfNotOk` already parses response body for `{ error?: string }` and throws with that message. `createTaskRun` special-cases 409 with a second, nearly identical block that parses JSON for `error` or falls back to a default message. So “parse JSON body for error message” is duplicated.

**Proposed change**

- Export or reuse a small helper, e.g. `async function parseErrorResponse(res: Response, defaultMessage: string): Promise<string>`, that does `const text = await res.text(); try { const j = JSON.parse(text) as { error?: string }; return j.error ?? defaultMessage } catch { return text || defaultMessage }`. Use it in `throwIfNotOk` (with default from res.statusText) and in `createTaskRun` for the 409 branch. Then 409 handling becomes: if (res.status === 409) throw new Error(await parseErrorResponse(res, "A run is already in progress…")).

**Benefit**: Single place for “response body → error string”; consistent behavior and less duplication.

**Priority**: Low

---

## Suggested order

1. **Reusable fetch with loading/error and cancel** (proposal 7) — implement first; then refactor TaskDetail, ArtifactContentModal, and optionally Explore to use it.
2. **Extract page router from App.tsx** (proposal 2) — reduces App size and clarifies routing vs layout.
3. **Deduplicate relative time formatting** (proposal 1) and **Replace createTaskRun 409 parsing** (proposal 8) — small, independent api.ts cleanups.
4. **Reduce duplication in useWorkspaceData** (proposal 3) — improves hook maintainability.
5. **Extract Explore tree and file logic** (proposal 6) — makes Explore easier to extend and test.
6. **Centralize onViewArtifact** (proposal 5) and **Unify create-modal pattern** (proposal 4) — UX and consistency; can be done in any order.

## Out of scope

- **Portal backend (Go server)** — Smell run is scoped to `portal/` frontend only; server smells are in the internal smell proposal.
- **E2E or integration tests** — No tests exist in portal yet; adding tests is a separate task.
- **State management (Redux/Zustand)** — Current prop drilling and hooks are acceptable for current size; arch-level reconsideration belongs in `/vibe arch`.
