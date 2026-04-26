# Portal Navigation Conversation / Recent Refactor

## Status

- type: `refactor`
- status: `proposed`
- roadmap: [design/010-team-task-workflow-roadmap.md](./010-team-task-workflow-roadmap.md)
- context: off-roadmap cleanup after Phase 4
- created_at: `2026-04-26`

---

## 1. Goal

Align the Portal top-level navigation with the team/work-management product model.

The current `Recent` sidebar section was designed for the earlier personal conversation UX. After Team, Issue, and Workflow support landed, it now competes with the primary work-system concepts and makes conversations look like a top-level product area.

Target top-level navigation:

- `Conversations`
- `Issues`
- `Workflows`
- `Agents`

`Recent` conversation access should move under `Conversations`, where it already naturally belongs.

---

## 2. Current State

Before this refactor, the sidebar mixed product areas and a conversation recency widget:

- `New Conversation`
- `Recent`
- `Agents`
- `Workflows`
- `Issues`

Relevant files:

- `portal/src/layout/Sidebar.tsx`
- `portal/src/pages/NewConversation.tsx`
- `portal/src/components/AppRouter.tsx`
- `portal/src/router.ts`
- `portal/src/pages/Tasks.tsx`
- `portal/src/css/sidebar.css`
- `portal/src/css/pages/home.css`
- `portal/src/css/pages/activity.css`

Current behavior:

- `New Conversation` routes to `#/` and renders `NewConversation`.
- `Recent` renders in the sidebar via `RecentList` from `@buildmax/gui`.
- Sidebar `Recent` uses the same `conversations` data that is passed to `NewConversation`.
- `NewConversation` already has a `Conversations` tab that lists all conversations.
- `Recent` "See all" navigates to route `chats`, whose hash is `#/conversations`.
- `chats` renders `Tasks`, even though the page title and content are conversations.

This creates three issues:

1. `Recent` is presented as a top-level navigation item even though it is just a filtered conversation list.
2. The conversation list exists in both the sidebar and `NewConversation`.
3. Naming is inconsistent: `chats`, `conversations`, and `Tasks` all refer to the same conversation list area.

---

## 3. Product Direction

The roadmap defines the stable user-facing concepts as:

- `Team`
- `Issue`
- `Agent`
- `Workflow`

Conversation remains important as the Tier 1 interaction surface, but it should not dominate the main navigation now that BuildMax is evolving from an agent chat runtime into a work orchestration system.

Recommended mental model:

- `Conversations` starts new conversational interaction and exposes recent conversation history.
- `Issues` is the main place to manage visible work.
- `Workflows` defines reusable execution plans.
- `Agents` defines digital team members.

This makes `Recent` a local feature of `Conversations`, not a product area.

---

## 4. Proposed Design

### 4.1 Sidebar

Remove the top-level `Recent` section from `Sidebar`.

The sidebar should contain:

1. Space switcher
2. `Conversations`
3. `Issues`
4. `Workflows`
5. `Agents`
6. User menu

The preferred order is `Issues`, `Workflows`, `Agents` after `Conversations`, because `Issues` is the daily work center while `Workflows` and `Agents` are reusable capabilities.

If we want the lowest-friction implementation, we can keep the existing `Agents`, `Workflows`, `Issues` order for this refactor and change ordering in a separate UI polish pass. However, the product-aligned target order is:

```text
Conversations
Issues
Workflows
Agents
```

### 4.2 Conversations Page

Make the existing `NewConversation` route/page the single home for conversation recency, while labeling the top-level navigation as `Conversations`.

Keep the existing tab structure, but rename and clarify the first tab:

- `Conversations` -> `Recent Conversations`
- `Files` remains `Files`

The conversation list should continue to navigate to `#/conversation/:conversationId`.

The page should keep the composer as the primary action. Recent conversations are secondary content below the composer.

### 4.3 Conversation List Route

The existing `#/conversations` route can remain for compatibility, but the naming should be cleaned up.

Recommended changes:

- Rename route type `chats` to `conversations`.
- Rename `portal/src/pages/Tasks.tsx` to `portal/src/pages/Conversations.tsx`.
- Update `AppRouter` and breadcrumbs to use `conversations`.

Compatibility options:

- Keep `#/conversations` as the canonical hash.
- Optionally accept old internal route names only during the refactor, but do not expose `chats` in new code.

If implementation scope needs to stay small, the first pass can leave `#/conversations` and `Tasks.tsx` intact but remove all UI entry points to it. The recommended implementation should still rename it, because `Tasks` is now misleading in a codebase that also has low-level `task` and user-facing `Issue`.

### 4.4 Shared GUI Usage

`RecentList` is currently only useful for sidebar recency. After removing sidebar `Recent`, there are two options:

1. Reuse `RecentList` inside `NewConversation` for visual consistency.
2. Keep the current `page-activity__list` markup in `NewConversation` and remove the `RecentList` import from `Sidebar`.

Prefer option 2 for the first refactor because it avoids changing the page layout more than necessary. Revisit shared list presentation later if Portal and Desktop need a common conversation history component.

---

## 5. Implementation Plan

### Step 1: Remove Sidebar Recent

Update `portal/src/layout/Sidebar.tsx`:

- remove `RecentList` import
- remove `createPortal` import if no longer needed
- remove `RecentIcon` import
- remove `CHATS_LIMIT`
- remove `conversationsCollapsed`
- remove `conversationsPopupOpen`
- remove `conversationsTriggerRef`
- remove `conversationsCloseTimeoutRef`
- remove recent popup helper functions
- remove derived `conversations` and `hasMoreConversations`
- remove the entire `.sidebar__chats` block

Keep the `conversations` prop only if `Layout` or future sidebar logic still needs it. If not, remove it from:

- `SidebarProps`
- `Layout`
- `App`

### Step 2: Reorder Primary Navigation

Update `Sidebar` primary nav order to:

1. `Conversations`
2. `Issues`
3. `Workflows`
4. `Agents`

Keep active-state helpers for detail routes:

- `issue` should keep `Issues` active.
- `workflow` and `workflowRun` should keep `Workflows` active.
- `agents` should keep `Agents` active.

Also update `isWorkflowsActive` so `workflowRun` is active under `Workflows`.

### Step 3: Rename Conversations Tab

Update `portal/src/pages/NewConversation.tsx`:

- sidebar/top-level entry text: `New Conversation` -> `Conversations`
- tab text: `Conversations` -> `Recent Conversations`
- tablist aria label: `Conversations and files` -> `Recent conversations and files`
- empty state can remain `No conversations yet.`

No API changes are needed.

### Step 4: Clean Conversation Route Naming

Recommended:

- rename route type `{ name: "chats" }` to `{ name: "conversations" }`
- keep hash segment `conversations`
- rename `Tasks` page component to `Conversations`
- rename `portal/src/pages/Tasks.tsx` to `portal/src/pages/Conversations.tsx`
- update imports in `AppRouter`
- update breadcrumbs from `chats` to `conversations`

If we do not expose a "See all" entry in the first implementation, this route can remain reachable by direct URL only.

### Step 5: Remove Dead Styles

Update `portal/src/css/sidebar.css`:

- remove `.sidebar__chats`
- remove `.sidebar__chats-toggle`
- remove `.sidebar__chats-chevron`
- remove `.sidebar__chats-list`
- remove `.sidebar__chats-list--portal`
- remove `.sidebar__chats-see-all`
- remove collapsed-sidebar rules that only exist for recent chat popup behavior

Keep page-level conversation list styles in `home.css` and `activity.css`.

---

## 6. Non-Goals

This refactor should not:

- change backend conversation APIs
- change conversation detail behavior
- change issue execution or workflow execution behavior
- add a new search experience for conversation history
- introduce a new global command palette
- redesign the whole sidebar visual language
- remove conversations as a runtime concept

---

## 7. Validation

Frontend validation:

- `cd portal && npm run build`
- focused lint check for edited frontend files if available in the IDE

Manual checks:

- sidebar shows exactly the intended top-level items
- `Conversations` starts a new conversation
- recent conversations are still visible under `Conversations`
- clicking a recent conversation opens `#/conversation/:conversationId`
- `Issues` remains active on issue detail pages
- `Workflows` remains active on workflow detail and workflow run detail pages
- team switching still redirects away from team-scoped detail pages correctly
- collapsed sidebar no longer shows a recent-conversation popup

---

## 8. Risks

### Loss of Global Recent Shortcut

Removing sidebar `Recent` means users can no longer jump to a recent conversation from any page in one click.

Mitigation:

- `Conversations` remains one click away.
- Conversation history remains visible there.
- If this becomes painful, add a future command palette or top-level search, rather than restoring `Recent` as a product nav item.

### Route Naming Churn

Renaming `chats` to `conversations` touches route types, router, breadcrumbs, and page imports.

Mitigation:

- keep the hash URL `#/conversations`
- only rename internal code symbols
- avoid changing persisted data or backend contracts

### Conversation vs Issue Product Center

`Conversations` remains the first nav item, which may still imply chat-first usage.

Mitigation:

- this is acceptable for now because conversation is still the main intent entry point
- make `Issues` the next primary item and the first work-management item
- revisit default landing behavior once issue dashboards become richer

---

## 9. Open Decisions

1. Should the first implementation reorder nav to `Conversations`, `Issues`, `Workflows`, `Agents`, or keep the current `Agents`, `Workflows`, `Issues` order and only remove `Recent`?
2. Should `#/conversations` remain a direct page, or should it redirect to `#/` with the `Recent Conversations` tab active?
3. Should `RecentList` be reused inside `NewConversation`, or should `NewConversation` keep its current page list markup?

Recommended answers:

1. Reorder now to `Conversations`, `Issues`, `Workflows`, `Agents`.
2. Keep `#/conversations` for now, but clean its internal names.
3. Keep current `NewConversation` markup for the first pass.
