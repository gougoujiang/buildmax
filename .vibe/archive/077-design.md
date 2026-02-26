# Design 077: Task detail page redesign

Technical design for task [077](077.md): make the Portal chat detail page conversation-centric with a two-section layout (chat history + chat input), chat-style message alignment, and removal of legacy sections.

## Goal

- Single view: **(1) Chat history** (conversation, user right / assistant and tool left), **(2) Chat input** at bottom.
- Minimal header: chat title + status and time (no separate Details section; Restore can stay disabled).
- Remove: Result, What changed, Evidence/Data used, Details sections.
- No API or backend changes; same data flow (getChatConversation, createChatRun, poll, refetch).

## Modules and structure

| Layer | Responsibility |
|-------|----------------|
| **ChatDetail.tsx** | Page component: header, chat history (message list with role-based layout), chat input; same hooks and submit/poll logic. |
| **chat-detail.css** | Layout for chat bubbles (user right, assistant/tool left); header and input styles; markdown/tool-call content styles; remove styles for deleted sections. |

No new files. No changes to `lib/api`, `lib/types`, router, or other pages.

## Component structure (ChatDetail.tsx)

**DOM order:**

1. **Header**  
   - Chat title (`chat.title` or "Chat").  
   - Subtitle or small text: status and time (e.g. `chat.status` + `chat.timeLabel`) so run state is visible.  
   - Optional: keep Restore button disabled (no behavior change).

2. **Chat history**  
   - Wrapper with a single class (e.g. `page-chat__history`) that will hold messages and states.  
   - **Loading**: show "Loading conversation…" (same copy as today).  
   - **Error**: show error message (same as today).  
   - **Empty**: when `session !== null` and `session.messages.length === 0`, show short text (e.g. "No messages yet. Use the input below to start.").  
   - **Messages**: one block per `session.messages` item. Each message:  
     - Container: role-based modifier for alignment (e.g. `page-chat__msg--user` right, `page-chat__msg--assistant`, `page-chat__msg--tool` left).  
     - Optional role label (e.g. "You" / "Assistant" / "Tool") for accessibility or minimal UX.  
     - Content: same as today — `msg.content` rendered with Markdown (remarkGfm); if `msg.tool_calls` present, render list of name + arguments (same structure as current `page-chat__session-toolcalls`).

3. **Chat input**  
   - Single section at bottom: textarea + submit button.  
   - Same behavior: `followUpInput`, `handleFollowUpSubmit` (createChatRun → poll getChats until terminal status → refetch session, onRefetch).  
   - Label/copy: e.g. "Send" or "Ask follow-up" (no "Follow-up" section heading required; can be a single input row).  
   - Disabled when `followUpLoading`; button shows "Running…" when loading.  
   - Error: show `followUpError` below input (same as today).

**Data and hooks (unchanged):**

- `useFetch(getChatConversation(workspaceId, chat.id, token!), ...)` — session, sessionLoading, sessionError, refetchSession.
- Local state: `followUpInput`, `followUpLoading`, `followUpError`.
- `handleFollowUpSubmit`: createChatRun → setInterval poll getChats → on terminal status clear interval, setFollowUpLoading(false), onRefetch(), refetchSession().
- Props: `chat`, `workspaceId`, `onRefetch`; types unchanged (`Chat` from `../lib/types`).

## Message list design

- **Alignment**:  
  - `role === "user"` → container has modifier for right alignment (e.g. flex parent with `justify-content: flex-end`, or block with `margin-left: auto` and max-width).  
  - `role === "assistant"` or `role === "tool"` → container has modifier for left alignment (default or explicit left).

- **Content**:  
  - Keep existing rendering: `Markdown remarkPlugins={[remarkGfm]}` for `msg.content`.  
  - Tool calls: same `<ul>` of `<li>` with tool name and optional `<pre>` for arguments; reuse or rename classes (e.g. `page-chat__msg-content`, `page-chat__msg-toolcalls`, `page-chat__msg-args`) so CSS can stay consistent.

- **Keys**: Continue using index in map for `key={i}` (messages are ordered and not reordered; no id in ApiSessionMessage). If backend later adds stable ids, switch to them.

## CSS design (chat-detail.css)

**Layout:**

- **Page**: `.page-chat` remains root.  
- **Header**: `.page-chat__header` — keep flex; add optional `.page-chat__subtitle` or reuse `.page-chat__meta` for status + time under the title.  
- **History**: New block `.page-chat__history` — flex column, gap between messages; optional max-height + overflow auto so input stays on screen.  
- **Message row**: New block `.page-chat__msg` with modifiers:  
  - `.page-chat__msg--user`: align self to end (right), bubble style (e.g. background, border-radius, max-width).  
  - `.page-chat__msg--assistant`, `.page-chat__msg--tool`: align left, bubble style distinct from user (e.g. different background).  
- **Input**: Reuse or rename follow-up block (e.g. `.page-chat__input` or keep `.page-chat__follow-up` but drop the section heading in JSX). Single row: textarea + button; styles can stay similar to current follow-up (full-width textarea, button beside or below).

**Removals:**

- Delete or leave unused: `.page-chat__section-heading` usage for Result, What changed, Evidence, Details (no DOM for those sections).  
- Remove styles that only applied to removed sections (e.g. `.page-chat__change-list`, `.page-chat__evidence-list` if present).  
- Keep: `.page-chat__markdown`, `.page-chat__session-content`-like (rename to `.page-chat__msg-content` if desired), `.page-chat__session-toolcalls` → `.page-chat__msg-toolcalls`, `.page-chat__session-args` → `.page-chat__msg-args` for message content and tool calls so markdown and code stay readable.

**BEM naming:**

- Prefer `page-chat__history`, `page-chat__msg`, `page-chat__msg--user`, `page-chat__msg--assistant`, `page-chat__msg--tool`, `page-chat__msg-content`, `page-chat__msg-toolcalls`, `page-chat__msg-args` for the new chat layout so "session" is not overloaded (session = API concept; msg = UI bubble).

## How they work together

1. **Load**  
   User opens chat detail → ChatDetail mounts → useFetch loads conversation → session/messages or loading/error shown in chat history area.

2. **Render**  
   Messages rendered in order; each message’s role drives CSS class (user → right, assistant/tool → left). Content and tool_calls rendered as today. Empty state when messages.length === 0.

3. **Follow-up**  
   User types and submits → createChatRun → polling starts → on terminal status, refetch conversation and parent refetches chat list → new messages appear in history; input re-enabled.

4. **Header**  
   Title and status/time always visible from `chat` prop; no separate Details section.

## Changes for review

| Area | File | Change |
|------|------|--------|
| Page | `portal/src/pages/ChatDetail.tsx` | Replace current sections with: header (title + status/time; optional Restore); single chat history block (loading/error/empty/messages with role-based alignment classes); single chat input block (same submit/poll/refetch logic). Remove Result, What changed, Evidence/Data used, Details. Use new BEM classes for message list (e.g. `page-chat__history`, `page-chat__msg`, `page-chat__msg--user` / `--assistant` / `--tool`). |
| CSS | `portal/src/css/pages/chat-detail.css` | Add layout for chat history and message bubbles (user right, assistant/tool left). Add/rename classes: `page-chat__history`, `page-chat__msg`, `page-chat__msg--user`, `page-chat__msg--assistant`, `page-chat__msg--tool`, `page-chat__msg-content`, `page-chat__msg-toolcalls`, `page-chat__msg-args`. Optional: `.page-chat__subtitle` for status/time. Remove or simplify styles for follow-up section heading and removed sections; keep markdown and tool-call content styles. |

No new types, no API changes, no changes outside `ChatDetail.tsx` and `chat-detail.css`.
