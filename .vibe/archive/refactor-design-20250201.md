# Design – Refactor Proposals 1–6 (2025-02-01)

## Goal

Implement refactor proposals 1–6 from `refactor-proposal-20250201.md`: extract viewport refresh helper, unify session persist flow, split TUI `Update` into message handlers, extract tool-call handling in agent `processLoop`, remove or document unused `llm.Chat`, and share root command setup between `runTUI` and `runPromptMode`. Proposal 7 (system prompt file split) is out of scope.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/tui** | TUI models and views; viewport refresh helper; Update dispatcher and per-message handlers. | Model, refreshViewportAndGotoBottom, handleKeyMsg, handleAgentDone, etc. |
| **internal/session** | Session + list persistence; single “persist after reply” API. | Session, ListEntry, SaveToDir, LoadFromDir, PersistAfterReply, EnsureTitleFromFirstUserMessage, UpsertListEntry. |
| **internal/agent** | Agent loop; one-tool-call execution helper. | Agent, processLoop, processOneToolCall. |
| **internal/llm** | LLM client; ChatWithTools only (Chat removed or documented). | Client, ChatWithTools. |
| **cmd/buildmax** | Root command; shared setup; runTUI / runPromptMode. | setupAgentAndSession, runTUI, runPromptMode. |

## Structure

**Directory / files**

- `internal/tui/`
  - `model.go` — Model, NewModel, viewport refresh helper, Update dispatcher, message handlers (handleKeyMsg, handleWindowSize, handleAgentDone, handleCarouselTick, handleMouseMsg, handleScrollIdle); View, helpers.
- `internal/session/`
  - `session.go` — Session, SaveToDir, LoadFromDir, etc. (unchanged layout).
  - `list.go` — ListEntry, LoadList, UpsertListEntry, EnsureTitleFromFirstUserMessage (unchanged).
  - **New**: `persist.go` (or add to `session.go` / `list.go`) — `PersistAfterReply`.
- `internal/agent/`
  - `agent.go` — Agent, processLoop, **processOneToolCall**, toolCallsSummary.
- `internal/llm/`
  - `client.go` — Client, NewClient, **Chat removed** (or comment-only per Option B), ChatWithTools.
- `cmd/buildmax/`
  - `root.go` — newRootCommand, runRoot, **setupAgentAndSession**, runTUI, runPromptMode (both call setupAgentAndSession; runPromptMode returns error and uses session.PersistAfterReply).

**Main types and interfaces**

- **refreshViewportAndGotoBottom** (tui): function `(m *Model, busy bool, carouselDots int)` — builds viewport content, sets it on viewport, GotoBottom.
- **PersistAfterReply** (session): function `(s *Session, dir, workspace string, maxTitleLen int) error` — EnsureTitleFromFirstUserMessage, SaveToDir, build ListEntry, UpsertListEntry.
- **processOneToolCall** (agent): function `(ctx context.Context, a *Agent, sess *session.Session, tc llm.ToolCall)` — resolve tool, parse args, execute, append assistant message (if not already) and one tool message; no return for “continue” cases.
- **setupAgentAndSession** (cmd/buildmax): function `(resumeID string) (agent *agent.Agent, sess *session.Session, sessionsDir, cwd string, err error)` — load config, cwd, create readFile tool and client, NewAgent, sessionsDir MkdirAll, load or create session.

## Method design

| Location | Name | Signature | Responsibility |
|----------|------|-----------|-----------------|
| **tui** | refreshViewportAndGotoBottom | `(m *Model, busy bool, carouselDots int)` | Build content via buildViewportContent(m.opts.Session, m.opts.Version, m.width, busy, carouselDots); m.viewport.SetContent(content); m.viewport.GotoBottom(). |
| **tui** | handleKeyMsg | `(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd)` | Handle Ctrl+C, 'q', Tab, focus routing, scroll keys, Enter submit, Escape, typing; return (Model, Cmd). |
| **tui** | handleWindowSize | `(m Model, msg tea.WindowSizeMsg) (tea.Model, tea.Cmd)` | Update width/height, viewport size, input width, syncInputHeight; return (m, nil). |
| **tui** | handleAgentDone | `(m Model, msg agentDoneMsg) (tea.Model, tea.Cmd)` | Set busy=false, carouselDots=0; on error set m.err and log; on success call refreshViewportAndGotoBottom(m, false, 0), then session.PersistAfterReply(..., 100) and set m.err if persist fails. Return (m, nil). |
| **tui** | handleCarouselTick | `(m Model, msg carouselTickMsg) (tea.Model, tea.Cmd)` | If busy: increment carouselDots mod 3, refreshViewportAndGotoBottom(m, true, m.carouselDots), return (m, next tick Cmd). Else return (m, nil). |
| **tui** | handleMouseMsg | `(m Model, msg tea.MouseMsg) (tea.Model, tea.Cmd)` | Wheel: optionally move focus to viewport, scroll, scheduleScrollIdleReturn; else forward to viewport. Return (m, Cmd). |
| **tui** | handleScrollIdle | `(m Model, msg scrollIdleMsg) (tea.Model, tea.Cmd)` | If msg.id == m.lastScrollID && !m.focusInput: set focusInput=true, input.Focus(). Return (m, nil). |
| **tui** | Update | `(m Model, msg tea.Msg) (tea.Model, tea.Cmd)` | Type-switch on msg; call handleKeyMsg, handleWindowSize, handleAgentDone, handleCarouselTick, handleMouseMsg, handleScrollIdle as appropriate; default: forward to input.Update and syncInputHeight. |
| **session** | PersistAfterReply | `(s *Session, dir, workspace string, maxTitleLen int) error` | EnsureTitleFromFirstUserMessage(s, maxTitleLen); SaveToDir(s, dir); build ListEntry{ID, Title, Workspace: workspace, CreatedAt}; UpsertListEntry(dir, entry); return first error. |
| **agent** | processOneToolCall | `(ctx context.Context, a *Agent, sess *session.Session, tc llm.ToolCall)` | Lookup tool by tc.Name; if missing append tool error message and return. Parse tc.Arguments to map[string]any; on parse error append tool error and return. Execute tool; on error append tool error and return. Append tool result message. (Assistant message with toolCalls is already appended by processLoop before the loop.) |
| **agent** | processLoop | `(ctx, sess) (reply, err)` | Unchanged high-level flow; after sess.Append(assistant with toolCalls), loop: for _, tc := range toolCalls { processOneToolCall(ctx, a, sess, tc) }; then next iteration. |
| **llm** | Chat | **Removed** (Option A) or kept with comment (Option B). | N/A or “Chat sends a single user message without tools. Prefer ChatWithTools for agent use.” |
| **cmd/buildmax** | setupAgentAndSession | `(resumeID string) (*agent.Agent, *session.Session, sessionsDir, cwd string, error)` | LoadLLM; if APIKey=="" return error; Getwd; NewReadFile(cwd); NewClient(cfg); NewAgent(client, []Tool{readFile}); sessionsDir = DataDir()/sessions; MkdirAll(sessionsDir); if resumeID load else NewSession(""); return agent, sess, sessionsDir, cwd, nil. |
| **cmd/buildmax** | runTUI | `(resumeID string) error` | agent, sess, sessionsDir, cwd, err := setupAgentAndSession(resumeID); if err return err; build TUIOpts; tea.NewProgram(app.NewModel(opts)); Run(); return. |
| **cmd/buildmax** | runPromptMode | `(prompt, resumeID string) error` | agent, sess, sessionsDir, cwd, err := setupAgentAndSession(resumeID); if err return err. ctx:=Background; reply, err := agent.Process(ctx, sess, prompt); if err return err. session.PersistAfterReply(sess, sessionsDir, cwd, 100); if err return err. fmt.Println(reply); return nil. Caller (runRoot) on error logs and os.Exit(1) or returns error. |

## How they work together

**Data/control flow**

1. **TUI**: User action or tick → Update → type-switch → handleKeyMsg / handleAgentDone / etc. Handlers that need viewport refresh call `refreshViewportAndGotoBottom(m, busy, carouselDots)`. On agent done, handleAgentDone calls `session.PersistAfterReply(m.opts.Session, m.opts.SessionsDir, m.opts.Workspace, 100)` and sets m.err on failure.
2. **Prompt mode**: runRoot → runPromptMode(prompt, resumeID) → setupAgentAndSession(resumeID) → agent.Process → session.PersistAfterReply(sess, sessionsDir, cwd, 100) → print reply. Errors from setupAgentAndSession or Process or PersistAfterReply cause runPromptMode to return error; runRoot handles it (e.g. log + os.Exit(1) for prompt mode).
3. **Agent**: processLoop → LLM call → if toolCalls: append assistant message, then for each tc call processOneToolCall(ctx, a, sess, tc); processOneToolCall appends only the tool result/error message.

**Dependencies**

- **tui** depends on **session** for PersistAfterReply (and existing Session, ListEntry types).
- **cmd/buildmax** depends on **agent**, **session**, **tui**, **config**, **llm**, **tools**; runPromptMode uses session.PersistAfterReply.
- **agent** depends on **session**, **llm**; processOneToolCall uses session.Append and a.toolsByName.

**Key data structures**

- **agentDoneMsg** (tui): unchanged; handleAgentDone receives it and runs refresh + PersistAfterReply on success.
- **ListEntry** (session): built inside PersistAfterReply from Session + workspace; passed to UpsertListEntry.

## Changes for review

### Proposal 1 – Viewport refresh

- **New** (tui): `refreshViewportAndGotoBottom(m *Model, busy bool, carouselDots int)` — build content, SetContent, GotoBottom.
- **Modified** (tui/model.go): NewModel: replace inline build + SetContent + GotoBottom with call to helper (e.g. refreshViewportAndGotoBottom(&m, false, 0) after m is built, or pass *Model to a small init helper). KeyEnter block: replace lines 224–226 with refreshViewportAndGotoBottom(&m, true, 0). agentDoneMsg block: replace lines 266–268 with refreshViewportAndGotoBottom(&m, false, 0). carouselTickMsg block: replace lines 293–295 with refreshViewportAndGotoBottom(&m, true, m.carouselDots).

### Proposal 2 – Session persist

- **New** (session): `PersistAfterReply(s *Session, dir, workspace string, maxTitleLen int) error` in a new file `persist.go` or in `list.go` (session package). Implement: EnsureTitleFromFirstUserMessage(s, maxTitleLen); SaveToDir(s, dir); entry := ListEntry{...}; UpsertListEntry(dir, entry); return first error.
- **Modified** (tui/model.go): In handleAgentDone (or current agentDoneMsg block), on success: replace EnsureTitleFromFirstUserMessage + SaveToDir + entry build + UpsertListEntry with single call `err := session.PersistAfterReply(m.opts.Session, m.opts.SessionsDir, m.opts.Workspace, 100)`; if err set m.err and log.
- **Modified** (cmd/buildmax/root.go): In runPromptMode, replace EnsureTitleFromFirstUserMessage + SaveToDir + entry build + UpsertListEntry with `if err := session.PersistAfterReply(sess, sessionsDir, cwd, 100); err != nil { ...; return err }`. Make runPromptMode return error; in runRoot, if prompt != "" then `if err := runPromptMode(prompt, resumeID); err != nil { slog.Error(...); os.Exit(1) }` (or return err and let main handle exit).

### Proposal 3 – Split Update into handlers

- **New** (tui/model.go): `handleKeyMsg(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd)`, `handleWindowSize(m Model, msg tea.WindowSizeMsg) (tea.Model, tea.Cmd)`, `handleAgentDone(m Model, msg agentDoneMsg) (tea.Model, tea.Cmd)`, `handleCarouselTick(m Model, msg carouselTickMsg) (tea.Model, tea.Cmd)`, `handleMouseMsg(m Model, msg tea.MouseMsg) (tea.Model, tea.Cmd)`, `handleScrollIdle(m Model, msg scrollIdleMsg) (tea.Model, tea.Cmd)`.
- **Modified** (tui/model.go): `Update(m Model, msg tea.Msg) (tea.Model, tea.Cmd)` becomes a type-switch that calls the appropriate handler; default branch forwards to m.input.Update and syncInputHeight. All existing logic for each message type moves into the corresponding handler (including calls to refreshViewportAndGotoBottom and session.PersistAfterReply).

### Proposal 4 – Agent tool-call extraction

- **New** (agent/agent.go): `processOneToolCall(ctx context.Context, a *Agent, sess *session.Session, tc llm.ToolCall)` — lookup tool, parse args, execute, append one tool message (error or result). Assistant message with toolCalls is appended by processLoop before the for-loop.
- **Modified** (agent/agent.go): In processLoop, replace the for _, tc := range toolCalls { ... } body with a single call `processOneToolCall(ctx, a, sess, tc)` per tc.

### Proposal 5 – llm.Chat

- **Option A**: **Deleted** (internal/llm/client.go): Remove `Chat` method (lines 28–43). Remove any test that calls Chat (grep found no callers in repo).
- **Option B**: **Modified** (internal/llm/client.go): Add comment above Chat: `// Chat sends a single user message without tools. Prefer ChatWithTools for agent use.` No code change.

### Proposal 6 – Root setup

- **New** (cmd/buildmax/root.go): `setupAgentAndSession(resumeID string) (*agent.Agent, *session.Session, sessionsDir, cwd string, err error)` — load config, get cwd, create readFile tool, NewClient, NewAgent, resolve sessionsDir and MkdirAll, load or create session; return all values needed by runTUI and runPromptMode.
- **Modified** (cmd/buildmax/root.go): `runTUI(resumeID string) error` — call setupAgentAndSession(resumeID); on err return err; build opts from returned agent, sess, sessionsDir, cwd; run tea.Program; return.
- **Modified** (cmd/buildmax/root.go): `runPromptMode(prompt, resumeID string) error` — call setupAgentAndSession(resumeID); on err return err; agent.Process(ctx, sess, prompt); on err return err; session.PersistAfterReply(sess, sessionsDir, cwd, 100); on err return err; fmt.Println(reply); return nil. runRoot: when prompt != "", call runPromptMode and if err != nil { log; os.Exit(1) }.

## Suggested implementation order

1. **Proposal 1** (viewport refresh) — small, local to tui; unblocks cleaner handlers in 3.
2. **Proposal 2** (session persist) — add PersistAfterReply; switch tui and root to use it.
3. **Proposal 5** (llm.Chat) — delete or document; no dependency on others.
4. **Proposal 3** (split Update) — extract handlers; each handler uses refreshViewportAndGotoBottom and (for handleAgentDone) PersistAfterReply.
5. **Proposal 4** (agent tool-call) — extract processOneToolCall; independent of 3.
6. **Proposal 6** (root setup) — extract setupAgentAndSession; runTUI and runPromptMode both use it; runPromptMode already uses PersistAfterReply from step 2.

## Out of scope

- **Proposal 7**: Extract default system prompt to a separate file or constant block (excluded per user request).
- Large TUI refactors (e.g. separate sub-models), interfaces for session/agent in cmd for testability, changing Bubble Tea to pointer receiver everywhere — as in the original proposal.
