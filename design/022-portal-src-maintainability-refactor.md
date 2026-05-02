# Portal Source Maintainability Refactor

## Status

- phase: `portal_refactor`
- name: `Portal Source Maintainability Refactor`
- status: `proposed`
- created_at: `2026-05-02`
- scope: `portal/src`

---

## 1. Goal

Reduce long-term maintenance cost in `portal/src` by making module boundaries explicit, moving domain logic out of route pages, and standardizing how the Portal handles routing, authenticated API access, data loading, DTO mapping, and realtime events.

This refactor should not change the product surface. It is an internal structure refactor intended to make future Portal work easier:

- adding a new page should not require copying manual `useState` / `useEffect` / `Promise.all` loading code
- adding a new backend field should have one clear mapping boundary
- changing route behavior should be backed by route tests
- feature domains should own their own API calls, mappers, hooks, and local UI
- foundational modules should not import upward from feature modules

---

## 2. Problem Statement

The Portal currently has useful separation between `pages`, `features`, `contexts`, `lib`, and `layout`, but the separation is only partial.

The highest maintenance cost comes from:

- unclear import direction between `lib/api` and `features`
- route model drift between `Route`, `parseHash`, `buildHash`, breadcrumbs, and routed pages
- repeated page-level loading and mutation code
- mixed token passing through props and direct `useAuth` / `useApp` usage
- DTO mapping split between shared mappers and ad hoc page helpers
- large files that mix data loading, permissions, form state, event handling, and rendering
- custom fetch hooks that solve similar problems in different ways
- realtime conversation behavior coupled directly into a feature detail hook

These issues are not severe bugs by themselves. The risk is cumulative: every new Portal feature increases the number of places a developer must inspect before making a safe change.

---

## 3. Current State

### 3.1 Existing Top-Level Shape

Current `portal/src` shape:

```text
portal/src/
  App.tsx
  main.tsx
  router.ts
  components/
  contexts/
  css/
  features/
  hooks/
  icons/
  layout/
  lib/
  pages/
```

This shape is workable, but the responsibilities are uneven:

- `pages/` contains route components, data loading, mutation logic, local mappers, permissions, and large render trees.
- `features/` contains API modules for most domains, but not consistently hooks, mappers, or domain components.
- `lib/api/` contains foundational API client code and a compatibility barrel that re-exports feature APIs.
- `contexts/` owns app-wide auth/team/socket/route state, but some contexts import feature helpers directly.
- `hooks/` contains generic fetch helpers and `useConversations`, while other feature fetch logic is embedded in pages.

### 3.2 Current Code Anchors

- app/provider shell:
  - [portal/src/App.tsx](../portal/src/App.tsx)
- route parser and navigation:
  - [portal/src/router.ts](../portal/src/router.ts)
- route rendering:
  - [portal/src/components/AppRouter.tsx](../portal/src/components/AppRouter.tsx)
- API foundation and compatibility barrel:
  - [portal/src/lib/api/client.ts](../portal/src/lib/api/client.ts)
  - [portal/src/lib/api/index.ts](../portal/src/lib/api/index.ts)
  - [portal/src/lib/api/types.ts](../portal/src/lib/api/types.ts)
  - [portal/src/lib/api/mappers.ts](../portal/src/lib/api/mappers.ts)
- shared UI/domain types:
  - [portal/src/lib/types.ts](../portal/src/lib/types.ts)
- repeated fetch helpers:
  - [portal/src/hooks/useFetch.ts](../portal/src/hooks/useFetch.ts)
  - [portal/src/hooks/useAsyncList.ts](../portal/src/hooks/useAsyncList.ts)
- high-density pages:
  - [portal/src/pages/IssueDetail.tsx](../portal/src/pages/IssueDetail.tsx)
  - [portal/src/pages/Issues.tsx](../portal/src/pages/Issues.tsx)
  - [portal/src/pages/Workflows.tsx](../portal/src/pages/Workflows.tsx)
  - [portal/src/pages/WorkflowDetail.tsx](../portal/src/pages/WorkflowDetail.tsx)
- large layout component:
  - [portal/src/layout/Sidebar.tsx](../portal/src/layout/Sidebar.tsx)
- conversation realtime hook:
  - [portal/src/features/conversations/hooks/useConversationDetail.ts](../portal/src/features/conversations/hooks/useConversationDetail.ts)

---

## 4. Key Findings

### 4.1 `lib/api` Imports Upward From Features

`portal/src/lib/api/index.ts` is labeled as a compatibility barrel, but it re-exports implementation functions from `features/*`.

That makes `lib/api` both:

- a low-level API foundation used by feature APIs
- a high-level facade that imports feature APIs

This creates an implicit cycle at the architecture level:

```text
features/* -> lib/api/client, lib/api/types
lib/api/index -> features/*
```

This makes it harder to answer simple questions:

- Should a page import from `../lib/api` or `../features/issues`?
- Can `lib/api` be treated as a stable foundation?
- Where should a new API function live?

### 4.2 Route Model Drift

`Route` includes a `conversations` route variant, and `buildHash` can emit `#/conversations`, but `parseHash` currently parses `#/conversations` as `{ name: "home" }`.

Consequences:

- route state can differ before and after a hash parse round trip
- breadcrumbs can link to a route that is not preserved by parsing
- dead pages such as `pages/Conversations.tsx` are easy to keep accidentally
- future routing changes become risky because behavior is not test-backed

### 4.3 Page-Level Fetching Is Repeated

Several pages manually implement the same lifecycle:

```text
if missing token/team:
  clear local state
  stop loading
else:
  set loading
  clear error
  Promise.all(...)
  map DTOs
  set state
  catch error
  finally stop loading
```

This appears in pages such as:

- `Issues`
- `IssueDetail`
- `Workflows`
- `WorkflowDetail`
- `AgentList`

The code is understandable in each file, but expensive across the app:

- loading states are implemented slightly differently
- errors are handled slightly differently
- cache invalidation is manual
- mutations need to remember which local state or refetch function to update
- page files grow as more related resources are needed

### 4.4 Token and Team Access Are Mixed

Some route pages receive `token` as a prop from `AppRouter`.

Other code reads `token` from `useAuth` or `useApp`.

Team id usually comes from `useTeam`, but some hooks accept `teamId` as an argument while also reading `useTeam` internally for reset behavior.

This increases cognitive load:

- a page author must decide whether auth/team are props or context
- tests and story-like rendering need different setup per page
- hooks can end up with overlapping sources of truth

### 4.5 DTO Mapping Is Not Feature-Owned

`lib/api/mappers.ts` maps many API DTOs to UI/domain types. This gives one place to inspect, but it also centralizes churn from unrelated domains.

`IssueDetail` already has a local `mapIssueFlow` helper because issue flow is domain-specific. This is a sign that mappers naturally belong near each feature once the feature is non-trivial.

The current mixed pattern creates two questions for every new mapper:

- should it go in `lib/api/mappers.ts`?
- should it live beside the feature?

### 4.6 UI Types Mix API Naming and UI Naming

Most UI/domain types in `lib/types.ts` use camelCase fields such as `createdAt`.

`Conversation` still exposes `created_at`, which leaks API naming into UI state.

That small inconsistency matters because it weakens the boundary between:

- API DTOs, which should match backend JSON naming
- UI/domain types, which should follow frontend naming conventions

### 4.7 Large Components Carry Too Many Responsibilities

`IssueDetail.tsx` combines:

- issue flow loading
- agents/workflows/members loading
- issue edit form state
- assignee formatting
- permission checks
- workflow run action
- agent run action
- timeline construction
- rendering all sections

`Sidebar.tsx` combines:

- collapsed state
- logo rendering
- team switcher
- primary nav
- user menu
- create-space dialog trigger
- outside-click behavior

These files are not impossible to work with today, but they are likely to become recurring merge-conflict and regression hotspots.

### 4.8 Realtime Conversation Logic Is Tightly Coupled

`useConversationDetail` currently owns:

- HTTP message loading
- WebSocket event subscriptions
- send behavior
- optimistic user message state
- streaming assistant content
- scroll behavior
- initial message auto-send

The hook is useful, but the transport details are not isolated. If realtime events expand to tasks, workflow runs, or issue updates, this pattern will be repeated and become harder to test.

---

## 5. Design Principles

### 5.1 Strict Layer Direction

Allowed direction:

```text
app -> pages -> features -> lib
layout -> lib
contexts -> lib, feature APIs only where explicitly app-level
features -> lib
lib -> no app/pages/features imports
```

Important rule:

```text
lib/ must never import from features/
```

`lib` should be boring and foundational:

- API client primitives
- generated or hand-written DTOs
- small pure utilities
- storage helpers
- cross-domain formatting helpers only when genuinely shared

Domain feature modules should not move under `lib`.

Rationale:

- `lib` should mean stable foundation, not product behavior.
- feature modules usually contain product concepts, API calls, hooks, mappers, permissions, mutations, and feature UI.
- moving `features` into `lib` would make `lib` a mixed foundation/product layer and weaken import rules.
- keeping `features` as a sibling of `lib` makes dependency direction easy to reason about.

If the word `features` becomes too generic as the Portal grows, a future rename to `domains` is acceptable:

```text
portal/src/
  domains/
    issues/
    workflows/
    conversations/
  lib/
```

That rename is optional. The architectural decision is that domain-owned code remains outside `lib`.

### 5.2 Feature Modules Own Domain Behavior

Each non-trivial feature should own:

```text
features/<feature>/
  api.ts
  mappers.ts
  hooks/
  components/
  types.ts        optional, if feature-local types are useful
  index.ts        public feature exports
```

Examples:

```text
features/issues/
  api.ts
  mappers.ts
  hooks/
    useIssuesPageData.ts
    useIssueDetail.ts
  components/
    IssueList.tsx
    IssueDetailHeader.tsx
    IssueAssignmentForm.tsx
    IssueExecutionActions.tsx
    IssueFlowTimeline.tsx
    IssueActivityTimeline.tsx
```

```text
features/workflows/
  api.ts
  mappers.ts
  hooks/
    useWorkflowsPageData.ts
    useWorkflowDetail.ts
    useWorkflowRunDetail.ts
  components/
    WorkflowList.tsx
    WorkflowRunList.tsx
    WorkflowStepList.tsx
```

Pages should compose these pieces, not own them.

### 5.3 Pages Should Be Thin Route Compositions

Target route page responsibilities:

- read route params from props
- call one feature hook
- render feature-level sections
- handle route-level navigation

Target page size is not a hard rule, but a healthy page should usually fit in one screen of reasoning. If a page needs local helper types, timeline construction, multiple mutation lifecycles, and several large JSX sections, those concerns probably belong in a feature hook or component.

Pages should also be organized by route/product domain instead of staying as one flat directory.

Recommended shape:

```text
pages/
  auth/
    Login.tsx
    SignUp.tsx
  conversations/
    NewConversation.tsx
    ConversationDetail.tsx
  issues/
    Issues.tsx
    IssueDetail.tsx
  workflows/
    Workflows.tsx
    WorkflowDetail.tsx
    WorkflowRunDetail.tsx
  agents/
    AgentList.tsx
  explore/
    Explore.tsx
  settings/
    AccountSettings.tsx
    SpaceSettings.tsx
    shared.tsx
```

The distinction is:

- `pages/<domain>` contains route entry points and route composition.
- `features/<domain>` contains reusable product/domain logic and feature UI.
- `lib` contains low-level foundations with no product ownership.

### 5.4 One Data Loading Model

The Portal should use one consistent model for server state.

Preferred: adopt TanStack Query.

Why:

- explicit query keys
- request deduplication
- cache invalidation after mutations
- less custom hook code
- simpler loading/error states
- easier background refetching
- well-known pattern for future contributors

Alternative: keep a local lightweight query abstraction.

If avoiding a dependency is preferred, replace `useFetch` and `useAsyncList` with one internal `useQuery`-style helper that takes:

- query key
- enabled flag
- async function
- default data
- optional stale/refetch behavior

The important decision is not the library. The important decision is that there is one idiom.

### 5.5 API DTOs Stay At The Boundary

Backend DTOs should remain snake_case and live under API boundary modules.

UI/domain types should be camelCase and stable for React code.

Mapping should happen once per API response before data enters feature UI state.

Rule:

```text
pages/components should not reach deeply into raw Api* DTOs unless rendering an API-boundary-only debug view.
```

### 5.6 Realtime Channels Should Be Isolated

WebSocket transport should be wrapped in feature-level channel hooks or modules.

For conversations:

```text
features/conversations/realtime.ts
features/conversations/hooks/useConversationMessages.ts
features/conversations/hooks/useConversationSend.ts
```

The goal is to keep event names, payload shapes, subscribe/unsubscribe logic, and send payload construction out of route-level UI hooks.

---

## 6. Target Architecture

### 6.1 Target `portal/src` Shape

```text
portal/src/
  app/
    App.tsx
    AppProviders.tsx
    AppRouter.tsx
    routeResetPolicy.ts
  main.tsx
  router/
    routes.ts
    hashRouter.ts
    router.test.ts
  contexts/
    AuthContext.tsx
    TeamContext.tsx
    WebSocketContext.tsx
  features/
    agents/
      api.ts
      mappers.ts
      hooks/
      components/
      index.ts
    conversations/
      api.ts
      mappers.ts
      realtime.ts
      hooks/
      components/
      index.ts
    files/
      api.ts
      hooks/
      components/
      index.ts
    issues/
      api.ts
      mappers.ts
      hooks/
      components/
      index.ts
    teams/
      api.ts
      mappers.ts
      hooks/
      index.ts
    workflows/
      api.ts
      mappers.ts
      hooks/
      components/
      index.ts
  layout/
    Layout.tsx
    Sidebar.tsx
    sidebar/
      Logo.tsx
      PrimaryNav.tsx
      TeamSwitcher.tsx
      UserMenu.tsx
  lib/
    api/
      client.ts
      common.ts
      generated.ts       optional future generated DTOs
      types.ts           current hand-written DTOs until generated
      sse.ts
      ws.ts
    storage/
      authStorage.ts
      currentTeamStorage.ts
    query/
      keys.ts            if TanStack Query or local query helper is used
    format/
      time.ts
    errorMessage.ts
    cn.ts
  pages/
    auth/
      Login.tsx
      SignUp.tsx
    conversations/
      NewConversation.tsx
      ConversationDetail.tsx
    issues/
      Issues.tsx
      IssueDetail.tsx
    workflows/
      Workflows.tsx
      WorkflowDetail.tsx
      WorkflowRunDetail.tsx
    agents/
      AgentList.tsx
    explore/
      Explore.tsx
    settings/
      AccountSettings.tsx
      SpaceSettings.tsx
      shared.tsx
```

This target can be reached incrementally. It is not necessary to move all files at once.

### 6.2 Dependency Rules

Hard rules:

- `lib/**` must not import from `features/**`, `pages/**`, `contexts/**`, or `app/**`.
- `features/<feature>/**` may import from `lib/**`.
- `features/<feature>/**` should not import from another feature except through a deliberate shared composition hook or page-level composition.
- `pages/**` may import from `features/**`, `layout/**`, `contexts/**`, `router/**`, and `lib/**`.
- `contexts/**` may import from `lib/**`; if a context needs feature API access, it should be treated as an app-level service and documented.
- `layout/**` should avoid domain API imports.

Soft rules:

- Prefer importing feature public APIs from `features/<feature>` rather than deep paths.
- Keep cross-feature composition in pages or explicit app-level hooks.
- Keep mutation side effects close to the feature hook that owns the queried data.

---

## 7. Detailed Refactor Plan

### Phase 0: Inventory And Safety Net

Status: `recommended_first`

Goal: make current behavior observable before changing structure.

Tasks:

1. Add route unit tests for `parseHash` and `buildHash`.
2. Cover route round trips for:
   - `#/`
   - `#/login`
   - `#/signup`
   - `#/conversation/<id>`
   - `#/conversations`
   - `#/explore`
   - `#/agents`
   - `#/account`
   - `#/account/usage`
   - `#/account/webhook`
   - `#/space`
   - `#/space/members`
   - `#/space/members/new`
   - `#/workflows`
   - `#/workflow/<id>`
   - `#/workflow-run/<id>`
   - `#/issues`
   - `#/issue/<id>`
3. Decide canonical behavior for `conversations`:
   - option A: remove `conversations` route and treat home/new conversation as canonical
   - option B: keep `conversations` as a real route and wire `pages/Conversations.tsx`
4. Audit unused route pages:
   - `pages/Home.tsx`
   - `pages/Conversations.tsx`
5. Audit CSS imports in `index.css` against actually routed pages.

Acceptance criteria:

- `npm run build` passes in `portal`.
- route tests document the intended canonical hash behavior.
- dead pages are either removed or intentionally wired.
- no product behavior changes except deliberate route canonicalization.

### Phase 1: Stabilize Module Boundaries

Goal: remove architecture-level cycles and make imports predictable.

Tasks:

1. Change `lib/api/index.ts` to export only foundational API items:
   - `UNAUTHORIZED_EVENT`
   - `parseErrorResponse`
   - `getApiBase`
   - API DTO types
   - shared low-level helpers if still needed
2. Stop re-exporting feature APIs from `lib/api/index.ts`.
3. Update call sites to import from feature modules directly:
   - auth calls from `features/auth`
   - usage calls from `features/usage`
   - agents calls from `features/agents`
   - issues calls from `features/issues`
   - workflows calls from `features/workflows`
   - conversations calls from `features/conversations`
   - files calls from `features/files`
4. Move current team localStorage helpers from `features/teams/storage.ts` to `lib/storage/currentTeamStorage.ts`.
5. Update `AuthContext` and `TeamContext` imports accordingly.
6. Normalize `Conversation` UI type from `created_at` to `createdAt`.
7. Update conversation mapper and all call sites.

Acceptance criteria:

- `lib/**` has no imports from `features/**`.
- `Conversation` UI type uses camelCase.
- TypeScript build passes.
- login/logout/team persistence behavior remains unchanged.

### Phase 2: Standardize Server State

Goal: make loading, error, cache, and mutation behavior consistent.

Preferred implementation:

1. Add TanStack Query to Portal dependencies.
2. Add a small query provider in the app provider stack.
3. Define query key helpers:
   - `teamsKeys`
   - `conversationKeys`
   - `issueKeys`
   - `workflowKeys`
   - `agentKeys`
   - `fileKeys`
4. Convert one low-risk feature first, preferably `workflows` list.
5. Convert `issues` list next.
6. Convert `issue detail` after list behavior is stable.
7. Remove or reduce use of `useFetch` and `useAsyncList` once no longer needed.

Alternative implementation:

1. Create `lib/query/useQuery.ts` and `lib/query/useMutation.ts` local helpers.
2. Replace `useFetch` and `useAsyncList` with the new local query model.
3. Use explicit query keys even if caching is minimal.

Recommended decision:

Use TanStack Query unless dependency minimization is more important than frontend maintainability. Portal already depends on React/Vite/React Markdown; this is a standard frontend infrastructure dependency and directly addresses the repeated patterns.

Acceptance criteria:

- one feature page no longer owns manual loading/error boilerplate
- mutation invalidation is explicit and centralized
- disabled queries behave correctly when token/team is missing
- no duplicate request storm during navigation

### Phase 3: Move Domain Logic Into Feature Hooks

Goal: make pages thin and move domain behavior next to each feature.

#### 3.1 Issues List

Create:

```text
features/issues/hooks/useIssuesPageData.ts
features/issues/components/IssueList.tsx
features/issues/components/IssueCreateDialog.tsx
```

Move out of `pages/Issues.tsx`:

- issue list loading
- agents/members/workflows loading
- page count calculation
- assignee label formatting
- create issue mutation
- create-then-patch behavior for non-default status/assignee

Target `pages/Issues.tsx`:

```text
read user id / route needs
call useIssuesPageData
render IssueList and IssueCreateDialog
```

#### 3.2 Issue Detail

Create:

```text
features/issues/mappers.ts
features/issues/hooks/useIssueDetail.ts
features/issues/components/IssueDetailHeader.tsx
features/issues/components/IssueEditForm.tsx
features/issues/components/IssueExecutionActions.tsx
features/issues/components/IssueFlowSummary.tsx
features/issues/components/IssueTimeline.tsx
```

Move out of `pages/IssueDetail.tsx`:

- `mapIssueFlow`
- latest run selection
- timeline construction
- assignee labels
- edit form state
- save mutation
- run workflow mutation
- run agent mutation
- owner/admin workflow assignment rule

Target `pages/IssueDetail.tsx`:

```text
call useIssueDetail(issueId)
render header/form/actions/flow/timeline sections
handle route-level back navigation
```

#### 3.3 Workflows

Create:

```text
features/workflows/hooks/useWorkflowsPageData.ts
features/workflows/components/WorkflowList.tsx
features/workflows/components/WorkflowCreateDialog.tsx
```

Move out of `pages/Workflows.tsx`:

- workflow/agent loading
- workflow count label
- create workflow mutation
- owner/admin management rule

#### 3.4 Workflow Detail And Run Detail

Create:

```text
features/workflows/hooks/useWorkflowDetail.ts
features/workflows/hooks/useWorkflowRunDetail.ts
features/workflows/components/WorkflowDetailHeader.tsx
features/workflows/components/WorkflowRunHistory.tsx
features/workflows/components/WorkflowRunSteps.tsx
```

Move out of route pages:

- detail loading
- run loading
- edit mutation
- run mutation
- step formatting

Acceptance criteria:

- route pages are materially smaller
- feature tests or focused component tests can exercise domain behavior without route shell setup
- no UI copy or CSS behavior changes unless intentional

### Phase 4: Decompose Layout Hotspots

Goal: reduce layout churn and make navigation easier to evolve.

Create:

```text
layout/sidebar/SidebarLogo.tsx
layout/sidebar/PrimaryNav.tsx
layout/sidebar/TeamSwitcher.tsx
layout/sidebar/UserMenu.tsx
layout/sidebar/useOutsideClick.ts
```

Move out of `Sidebar.tsx`:

- team grouping logic
- team switcher rendering
- user menu rendering
- outside-click behavior
- nav item active-state helpers

Keep in `Sidebar.tsx`:

- collapsed state
- high-level aside structure
- composition of sidebar subcomponents

Acceptance criteria:

- adding a new primary nav item touches only the nav config/component
- changing user menu behavior does not require scanning team switcher code
- collapsed behavior stays unchanged

### Phase 5: Isolate Conversation Realtime

Goal: make conversation streaming behavior testable and reusable.

Create:

```text
features/conversations/realtime.ts
features/conversations/hooks/useConversationMessages.ts
features/conversations/hooks/useConversationComposer.ts
features/conversations/hooks/useConversationStream.ts
```

Move out of `useConversationDetail`:

- WebSocket event names
- payload interfaces
- subscribe/unsubscribe logic
- send payload construction
- stream accumulation behavior

Keep detail composition hook if useful:

```text
useConversationDetail = useConversationMessages + useConversationComposer + useConversationStream
```

Acceptance criteria:

- transport event names are defined in one module
- message loading can be tested independently from WebSocket streaming
- initial-message auto-send remains exactly-once per conversation mount
- optimistic message behavior remains unchanged

### Phase 6: API Type Hardening

Goal: reduce backend/frontend drift.

Options:

1. Generate TypeScript DTOs from backend OpenAPI.
2. Keep hand-written DTOs but add status normalization helpers.
3. Add runtime validators for high-risk enum-like fields.

Recommended staged path:

1. Move mappers into feature modules first.
2. Add explicit normalization functions for statuses:
   - issue status
   - workflow status
   - workflow run status
   - workflow step status
   - task status
3. Evaluate OpenAPI generation after the feature mapper split, so generated types replace only DTOs and do not disrupt UI/domain types.

Acceptance criteria:

- no direct `api.status as Domain["status"]` casts in feature mappers without a normalization function
- unexpected backend status has a clear fallback behavior
- generated DTO adoption, if chosen, does not force generated types into UI components

---

## 8. Proposed Migration Order

Recommended order:

1. `router` tests and canonical route cleanup
2. `pages/<domain>` directory reorganization
3. `lib/api` barrel cleanup
4. current team storage relocation
5. `Conversation.createdAt` normalization
6. data-loading model decision
7. workflows list conversion
8. issues list conversion
9. issue detail conversion
10. workflow detail/run detail conversion
11. sidebar decomposition
12. conversation realtime isolation
13. API type hardening

Reasoning:

- route tests and boundary cleanup are low-risk and improve confidence
- page directory reorganization is mostly import-path churn, so it should happen before deeper page rewrites
- workflows list is smaller than issue detail and is a good first data-loading conversion
- issues list introduces related resources and pagination
- issue detail should wait until the data-loading pattern is proven
- realtime isolation should wait until feature boundaries are clearer

---

## 9. Non-Goals

This refactor should not:

- redesign Portal visual UI
- change backend API routes
- change auth semantics
- change team ownership semantics
- merge Portal and Desktop frontend logic
- move shared presentational widgets out of `gui`
- rewrite all CSS
- introduce a full app framework such as React Router unless there is a separate routing design

Hash routing can remain for now. The immediate problem is route consistency and test coverage, not the routing mechanism itself.

---

## 10. Risk Register

### 10.1 Route Regression

Risk:

- existing deep links or breadcrumbs can change behavior when `home` / `conversations` is canonicalized

Mitigation:

- add route tests before changing behavior
- manually test all primary nav links and breadcrumbs

### 10.2 Auth And 401 Regression

Risk:

- changing API imports or fetch wrappers can break `UNAUTHORIZED_EVENT` handling

Mitigation:

- keep one API client path for 401 dispatch
- test expired-token behavior manually
- avoid creating parallel fetch clients

### 10.3 Team Switch Regression

Risk:

- current app-level team switch behavior resets pending conversation and navigates away from team-scoped detail pages

Mitigation:

- keep `App.tsx` route reset behavior unchanged until it has tests
- manually test switching teams from conversation, issue detail, workflow detail, and workflow run detail

### 10.4 Duplicate Fetches

Risk:

- converting to a query library or new query helper can cause duplicate network requests under React Strict Mode

Mitigation:

- use stable query keys
- use `enabled` gates for token/team
- inspect network behavior during manual QA

### 10.5 Mapper Drift

Risk:

- moving mappers from shared `lib/api/mappers.ts` to feature modules can accidentally alter field mapping

Mitigation:

- migrate one feature at a time
- keep mapper names stable during moves
- use TypeScript errors to update imports
- compare UI behavior before and after

### 10.6 Large-File Split Regression

Risk:

- extracting components from `IssueDetail` can break subtle state interactions

Mitigation:

- first extract pure presentational sections
- move mutation logic only after rendering sections are stable
- preserve prop names and behavior during the first split

---

## 11. Validation Plan

### 11.1 Automated Validation

Run from `portal/`:

```bash
npm run build
```

If tests are added:

```bash
npm test
```

If no test runner exists yet, add the minimal test infrastructure needed for router tests before broad refactors.

Recommended tests:

- route parse/build round trips
- route aliases and legacy hashes
- status normalization helpers
- issue flow mapper
- workflow run mapper

### 11.2 Manual QA

Manual route and product smoke:

- login
- signup path still renders
- logout clears token and current team id
- switch team from home
- switch team while viewing a conversation
- switch team while viewing an issue detail
- switch team while viewing a workflow detail
- start a new conversation
- send a message and observe streaming
- open recent conversation
- open issues list
- create issue
- edit issue
- assign issue to person
- assign issue to agent
- assign issue to workflow as owner/admin
- verify workflow assignment message for member
- run issue agent
- run issue workflow
- open workflow list
- create workflow
- edit workflow
- run workflow
- open workflow run detail
- open agents list
- create/edit/delete agent
- open file explorer
- open account usage
- open webhook keys
- open space settings and members

### 11.3 BuildMax-Level Validation

This is a frontend refactor, so Go tests are not required for every phase.

Run full repo tests only when touching shared contracts, generated API output, or backend OpenAPI generation:

```bash
./make test
```

---

## 12. Success Criteria

The refactor is successful when:

- `lib/api` no longer imports from `features`
- route parsing/building has tests and canonical behavior
- route pages are grouped under `pages/<domain>`
- route pages are mostly composition, not domain orchestration
- issue and workflow logic is feature-owned
- one server-state loading model is used consistently
- UI/domain types use camelCase consistently
- API DTOs stay at API boundaries
- `Sidebar` is decomposed into smaller layout widgets
- conversation realtime event wiring is isolated from UI detail composition
- future Portal changes have fewer files to touch and fewer patterns to choose from

---

## 13. First Recommended Slice

Implement this first slice as one reviewable PR:

1. Add router tests.
2. Decide and encode canonical `home` / `conversations` behavior.
3. Remove or wire `Home.tsx` and `Conversations.tsx`.
4. Move flat route pages into `pages/<domain>` directories and update imports.
5. Make `lib/api/index.ts` foundational only.
6. Move current team storage helper to `lib/storage/currentTeamStorage.ts`.
7. Normalize `Conversation.createdAt`.
8. Run `cd portal && npm run build`.

This slice has high leverage because it cleans the architectural foundation before any large page extraction.

It also has bounded risk: it touches import paths, route behavior, page placement, and a small UI type inconsistency, but does not yet rewrite issue/workflow screens.
