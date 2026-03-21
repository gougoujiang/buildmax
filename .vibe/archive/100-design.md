# Design 100: Basic chat in desktop app

## Goal

Wire the Wails desktop app to the shared agent runtime so the UI shows real sessions and messages, and the user can send a message and receive one assistant reply with session persistence (CLI-compatible).

## Modules

| Module | Responsibility |
|--------|----------------|
| `internal/cmd/desktop` | App struct with Wails bindings: ListSessions, GetSession, SendMessage. Uses `config`, `session`, `agentrun`. |
| `desktop/frontend/src` | React app: load sessions/messages from Go, message input + send, loading and error state, New chat flow. |

No new packages. Backend stays in `internal/cmd/desktop`; frontend in existing `desktop/frontend/`.

## Backend design

### Types (Go → JSON for frontend)

All types exposed to the frontend must be exported structs with `json` tags so Wails can serialize them.

- **Session list item** — Reuse `session.ListEntry` from `internal/session`: `ID`, `Title`, `Workspace`, `CreatedAt` (RFC3339). Already has `json:"id"`, etc. The desktop package will return `[]session.ListEntry` from ListSessions; no new type.

- **Session detail (for GetSession)** — New type in `internal/cmd/desktop` to avoid pulling `llm.Message` into bindings and to keep a minimal display shape:
  - `SessionDetail` struct: `ID string`, `Title string`, `CreatedAt string` (RFC3339), `Messages []MessageDisplay`.
  - `MessageDisplay` struct: `Role string`, `Content string`. Only role and content for display; tool messages can be included with role `"tool"` and a short content (e.g. result snippet) or omitted for minimal scope. Task says minimum user + assistant; include tool messages as simple role+content for robustness.

- **SendMessage result** — New struct in `internal/cmd/desktop`: `SendMessageResult` with `Reply string`, `SessionID string`. Error returned as Wails/Go error (frontend receives it as rejected promise or error return).

### App methods (Wails bindings)

Receiver: `*App` in `internal/cmd/desktop/app.go`. All methods take no context (Wails passes context internally if needed); use `context.Background()` for agentrun.

1. **ListSessions() ([]session.ListEntry, error)**  
   - Get sessions dir from `config.SessionsDir()`.
   - Call `session.LoadList(sessionsDir)`.
   - Return the slice and nil, or nil and error (e.g. dir read failure). Return empty slice if file missing (LoadList returns empty slice when file not found).

2. **GetSession(sessionID string) (SessionDetail, error)**  
   - If sessionID is empty, return error (e.g. "session ID required").
   - Get sessions dir from `config.SessionsDir()`.
   - Call `session.LoadFromDir(sessionsDir, sessionID)`. On `ErrSessionNotFound` return a clear error.
   - Build `SessionDetail`: ID = session.ID(), Title = session.Title(), CreatedAt = session.CreatedAt().Format(time.RFC3339), Messages = map each `llm.Message` to `MessageDisplay{Role: m.Role, Content: m.Content}` (tool messages included as-is; content may be long — acceptable for basic chat).
   - Return the struct and nil, or zero value and error.

3. **SendMessage(sessionID string, prompt string) (SendMessageResult, error)**  
   - If prompt is empty, return error (e.g. "prompt required").
   - Resolve workspace: `os.Getwd()` (same as CLI when WorkspaceDir empty).
   - Call `agentrun.Open(agentrun.OpenInput{WorkspaceDir: workspace, SessionID: sessionID, ModelSelector: ""})`. This creates a new session when sessionID is empty, or loads existing from disk.
   - Call `rt.RunPrompt(ctx, agentrun.RunInput{Prompt: prompt, Stream: nil})`. RunPrompt persists session and updates list internally.
   - On success return `SendMessageResult{Reply: out.Reply, SessionID: out.SessionID}` and nil. On error return zero value and error (e.g. wrap with fmt.Errorf so message is clear to the user).

All methods are synchronous so the frontend can await and show loading state.

### Dependencies

- `internal/cmd/desktop` will import: `context`, `os`, `time`, `buildmax/internal/config`, `buildmax/internal/session`, `buildmax/internal/app/agentrun`. No new external deps.

## Frontend design

### UI style: borrow from portal

Match the **portal** layout and styling so the desktop app feels consistent:

- **Shell**: Same structure as portal — `shell` → `shell__body` with **sidebar** + **main**. Main area: `shell__main` with optional `shell__top` (e.g. breadcrumb/title + theme toggle) and `shell__content` for the chat area.
- **Side panel**: Same pattern as `portal/src/layout/Sidebar.tsx`:
  - **New Chat** — A single nav item (button) at the top of the nav section: `sidebar__nav`, `sidebar__section`, `sidebar__nav-item` (active when selection is "new"), with icon + "New Chat" text. Clicking it sets `selectedId` to `null`.
  - **Recent** — A collapsible section: `sidebar__chats`, `sidebar__chats-toggle` (icon + "Recent" + chevron), `sidebar__chats-list` containing `sidebar__list` → `sidebar__item` → `sidebar__link` for each session. Each link shows `sidebar__chat-title` (session title) and `sidebar__chat-meta` (e.g. formatted created_at). Clicking a link sets `selectedId` to that session's id. Sessions come from `ListSessions()`.
  - Desktop can omit workspace switcher and user menu; sidebar is just New Chat + Recent. Collapsed state (narrow sidebar with popup) is optional for this task.
- **Main panel**: Same as portal's conversation view — a single **chat** column:
  - **Chat history** — Scrollable message list: container with class `page-chat`, inside it `page-chat__history` (scrollable). Each message: `page-chat__msg-row page-chat__msg-row--user` or `--assistant`, with `page-chat__msg page-chat__msg--user` or `--assistant`, and `page-chat__msg-content` for the text. User messages aligned right, assistant left (portal uses `page-chat__msg-row--user` with `align-self: flex-end`).
  - **Input box** — Pinned to bottom: `page-chat__input` → `page-chat__input-box` containing a `textarea.page-chat__follow-up-input` and a `button.page-chat__follow-up-btn` (Send). Placeholder e.g. "Type a message… (Enter to send, Shift+Enter for new line)". Show error below the box with `page-chat__text page-chat__error`.
- **CSS**: Copy or adapt the relevant rules from portal: `portal/src/css/sidebar.css` (sidebar layout, nav item, Recent section, list/link styles), `portal/src/css/layout.css` (shell, shell__main, shell__content), `portal/src/css/pages/chat-detail.css` (page-chat, history, msg row, input box, follow-up button, error). Use the same CSS variable names (e.g. `--color-bg`, `--color-text`, `--color-border`) so theme toggle continues to work if the desktop already has ThemeContext.

### Data flow

- **On load / when sidebar is shown**: Call `ListSessions()` once (e.g. on mount and after each SendMessage success) to populate the Recent list. "New Chat" nav item sets selection to `null`.
- **When a session is selected**: If selection is "new", show empty message list and focus input. If selection is a session ID, call `GetSession(sessionID)` and set local state to the returned messages (and title).
- **When user sends a message**:  
  - If current selection is "new", call `SendMessage("", prompt)`. On success: take `result.SessionID`, refresh `ListSessions()`, set selected id to `result.SessionID`, and append user message + `result.Reply` to the current view (or call `GetSession(result.SessionID)` to stay in sync).  
  - If current selection is an existing session ID, call `SendMessage(sessionID, prompt)`. On success: append user message + result.Reply to the view; refresh list (title may have been updated).  
  - While request in flight: set loading true, disable send (button shows "Sending…"). On error: show error message below input with `page-chat__error`.

### Components and state (App.jsx)

- **State**: `sessions` (list from ListSessions), `selectedId` (string or null for new), `messages` (array of { role, content } for current session), `loading` (boolean), `error` (string or null).
- **Remove**: All mock data (MOCK_THREADS, MOCK_MESSAGES).
- **Layout**: One outer `div.shell` → `div.shell__body` → `aside.sidebar` + `main.shell__main`. Main contains `div.shell__top` (title + ThemeToggle) and `div.shell__content` (chat area).
- **Sidebar**: New Chat button with `sidebar__nav-item` (active when `selectedId === null`); Recent section with `sidebar__chats`, `sidebar__chats-toggle`, `sidebar__chats-list` and `sidebar__list` / `sidebar__item` / `sidebar__link`; each link shows `sidebar__chat-title` and `sidebar__chat-meta`. On select set `selectedId`; when `selectedId` changes to a real id, call GetSession and set `messages`.
- **Main panel (shell__content)**: Single `div.page-chat`: `section.page-chat__history` (messages) and `section.page-chat__input` (input box + Send + error). Message list uses `page-chat__msg-row`, `page-chat__msg`, `page-chat__msg-content` as in portal. On submit call SendMessage with current `selectedId` (or "" for new), then update state as above.
- **Wails bindings**: Use the project's Wails bridge (e.g. `window.go.main.App`) to call ListSessions, GetSession, SendMessage.

### Styling and a11y

- Use the portal-derived class names above so copied CSS applies. Optional: simple avatar/icon for user and assistant in each message row (e.g. `page-chat__msg-icon`) for visual parity with portal.
- Keyboard: Submit on Enter, Shift+Enter for newline in textarea.
- Message list: `aria-label="Conversation history"`, `role="log"` or `aria-live="polite"` so new messages are announced. Input section: `aria-label="Send a message"`.

## How they work together

1. **Startup**: App.Startup already ensures config.DataDir(). No change to Startup.
2. **User opens app**: Frontend mounts, calls ListSessions(), displays session list; selectedId can default to null ("New chat") or first session.
3. **User selects a session**: Frontend calls GetSession(selectedId), sets messages and title.
4. **User types and sends**: Frontend sets loading, calls SendMessage(selectedId ?? "", prompt). Go opens runtime (new or existing session), RunPrompt runs (LLM + tools, persist), returns Reply and SessionID. Frontend appends user + assistant to view, refreshes list, clears loading and error.
5. **New chat**: User selects "New chat", types, sends; SendMessage("", prompt) creates new session; frontend gets SessionID, refreshes list, selects new session and shows the two messages.

Persistence is entirely inside agentrun.RunPrompt (PersistAfterReply); desktop backend does not persist directly. Session list and files live under config.SessionsDir() and are shared with the CLI.

## Changes for review

| Location | Change |
|----------|--------|
| `internal/cmd/desktop/app.go` | Add types: `SessionDetail`, `MessageDisplay`, `SendMessageResult`. Add methods: `ListSessions() ([]session.ListEntry, error)`, `GetSession(sessionID string) (SessionDetail, error)`, `SendMessage(sessionID, prompt string) (SendMessageResult, error)`. Implement using config, session, agentrun. |
| `internal/cmd/desktop/run.go` | No change (Bind already has app). |
| `desktop/frontend/src/App.jsx` | Remove mocks. Layout: shell, sidebar (New Chat + Recent), main (page-chat with history + input). State: sessions, messages, loading, error. Use portal-style class names (sidebar__*, page-chat__*). Fetch list on mount and after send; on session select call GetSession; add input + Send; on submit call SendMessage; handle new vs existing, loading and error. Use Wails bridge to call Go. |
| `desktop/frontend/src/css/` | Copy or adapt portal CSS: sidebar.css (sidebar, nav item, Recent section, list/link), layout.css (shell, shell__main, shell__content), chat-detail.css (page-chat, history, msg row, input box, Send button, error). Use same CSS variables for theme. |
| `cmd/buildmax-desktop/` | No structural change; app is already bound. |

No new files required if types and methods live in app.go; if app.go grows too large, types can move to a `types.go` in the same package.
