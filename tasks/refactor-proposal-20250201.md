# Refactor Proposal - 2025-02-01

## Scope

**Analyzed**

- Paths / packages: `internal/` (tui, agent, session, llm, app), `cmd/buildmax/`
- Criteria: All implementation and CLI code under internal/ and cmd/; recent structure from task 013 (session list, TUI save flow).

## Summary

- **Proposals**: 7
- **High priority**: 2
- **Overview**: Main opportunities are reducing duplication (viewport refresh, session persist flow), shortening long functions (TUI Update, agent processLoop, root setup), and removing or clarifying dead/unused code. One low-priority structural option (app package).

---

## Proposals

### 1. Extract viewport content refresh in TUI model

**Location**: `internal/tui/model.go` — `Update` (KeyEnter, agentDoneMsg, carouselTickMsg) and `NewModel`

**Current state**

- The same three-step pattern appears four times: build viewport content from session/version/width/busy/carouselDots, set it on the viewport, then call GotoBottom. Lines 77 (NewModel), 226–228 (KeyEnter submit), 269–271 (agentDoneMsg success), 294–296 (carouselTickMsg). Duplication makes it easy to forget a step (e.g. GotoBottom) when adding a new code path and obscures the single “refresh viewport and scroll to bottom” intent.

**Proposed change**

- Add a helper, e.g. `refreshViewportAndGotoBottom(m *Model, busy bool, carouselDots int)`, that:
  - Calls `buildViewportContent(m.opts.Session, m.opts.Version, m.width, busy, carouselDots)`
  - Sets `m.viewport.SetContent(content)` and `m.viewport.GotoBottom()`
- Replace all four call sites with this helper. NewModel can call it with `busy=false`, `carouselDots=0` (or a small init helper if you prefer to keep NewModel without a full Model pointer).

**Benefit**: Single place for “refresh viewport and scroll”; less duplication and fewer mistakes when adding new message types.

**Priority**: High

---

### 2. Unify session persist flow (save + list upsert)

**Location**: `internal/tui/model.go` (agentDoneMsg block, ~lines 271–287), `cmd/buildmax/root.go` (runPromptMode, ~lines 200–216)

**Current state**

- The same sequence appears in two places: ensure title from first user message, save session to dir, build ListEntry from session + workspace, upsert list entry; each step with its own error handling and logging. Duplication increases the chance of divergence (e.g. different error handling or order) and makes it harder to add behavior (e.g. metrics) in one place.

**Proposed change**

- In `internal/session`, add a function that encapsulates the full “persist after reply” flow, e.g. `PersistAfterReply(s *Session, dir, workspace string, maxTitleLen int) error`. It should:
  - Call `EnsureTitleFromFirstUserMessage(s, maxTitleLen)`
  - Call `SaveToDir(s, dir)`
  - Build `ListEntry{ID: s.ID(), Title: s.Title(), Workspace: workspace, CreatedAt: s.CreatedAt().Format(time.RFC3339)}`
  - Call `UpsertListEntry(dir, entry)`
  - Return the first error (or combine sensibly). Callers can log and surface to UI as they do now.
- In `internal/tui/model.go` (agentDoneMsg) and `cmd/buildmax/root.go` (runPromptMode), replace the inline sequence with a single call to `session.PersistAfterReply(..., 100)` and handle the returned error (set `m.err` in TUI, log and exit in prompt mode).

**Benefit**: One place for “save session and update list”; consistent behavior and easier to extend (e.g. retries, logging).

**Priority**: High

---

### 3. Split TUI Model.Update into message handlers

**Location**: `internal/tui/model.go` — `Update` (lines 162–343)

**Current state**

- `Update` is one large switch (~180 lines) handling KeyMsg (quit, Tab, focus routing, scroll keys, Enter, Escape, typing), WindowSizeMsg, agentDoneMsg, carouselTickMsg, MouseMsg, and scrollIdleMsg. Deep nesting and many branches make it hard to follow and to unit-test individual behaviors (e.g. “on Tab, focus toggles”) without exercising the whole function.

**Proposed change**

- Extract one function per message type (or per logical group), e.g. `handleKeyMsg(m Model, msg tea.KeyMsg) (Model, tea.Cmd)`, `handleAgentDone(m Model, msg agentDoneMsg) (Model, tea.Cmd)`, `handleWindowSize(m Model, msg tea.WindowSizeMsg) (Model, tea.Cmd)`, etc. Each returns (Model, tea.Cmd). `Update` becomes a thin dispatcher: type-switch on msg and call the right handler. Keep handlers in the same file (or same package) so visibility stays simple.

**Benefit**: Shorter, focused functions; easier to read and test (e.g. table-driven tests for key handling). Aligns with “one place per responsibility.”

**Priority**: Medium

---

### 4. Extract tool-call handling in agent processLoop

**Location**: `internal/agent/agent.go` — `processLoop` (for loop over `toolCalls`, ~lines 131–176)

**Current state**

- The loop body is long (lookup tool, parse args, execute, append success/error message to session) with repeated `sess.Append(llm.Message{...})` for unknown tool, invalid args, and execution error. Hard to scan and to test “one tool call” in isolation.

**Proposed change**

- Extract a function, e.g. `processOneToolCall(ctx context.Context, a *Agent, sess *session.Session, tc llm.ToolCall)`, that:
  - Resolves the tool by name, parses arguments, executes, and appends exactly one assistant message (if not already appended) and one tool message (result or error) to the session. Return only for fatal/unexpected cases if needed; otherwise the loop continues.
- `processLoop` keeps the “build messages, call LLM, if tool calls then for each call processOneToolCall” structure but the per-call logic lives in one place. Consider keeping `sess.Append(assistant message with toolCalls)` in the loop and only moving the “execute one tc and append tool message” part into the helper if you want minimal change.

**Benefit**: Shorter loop body; clearer separation between “handle LLM response” and “handle one tool call”; easier to unit-test tool execution and error appends.

**Priority**: Medium

---

### 5. Remove or deprecate unused llm.Client.Chat

**Location**: `internal/llm/client.go` — `Chat` (lines 28–43)

**Current state**

- `Client` exposes `Chat(ctx, userMessage)` but no caller in the repo uses it; the agent uses only `ChatWithTools`. Dead code adds noise and can confuse future readers about the intended API.

**Proposed change**

- **Option A**: Delete `Chat` and its tests (if any). If prompt-mode or other code ever needs a single-message call, it can use `ChatWithTools` with a single user message and no tools.
- **Option B**: Keep the method but add a short comment, e.g. `// Chat sends a single user message without tools. Prefer ChatWithTools for agent use.` and leave it for possible future CLI/script use. No behavioral change.

**Benefit**: Clearer API surface (option A) or documented intent (option B); less dead code.

**Priority**: Medium

---

### 6. Share root command setup between runTUI and runPromptMode

**Location**: `cmd/buildmax/root.go` — `runTUI` (lines 89–140), `runPromptMode` (lines 153–218)

**Current state**

- Both paths duplicate: load LLM config, get cwd, create readFile tool, create LLM client, create agent, resolve sessionsDir, load or create session. Only the “run” part differs (TUI program vs Process + print + exit). Duplication makes it harder to add a new step (e.g. another tool or config) in one place and increases the risk of divergent behavior (e.g. runPromptMode exits on missing API key while runTUI returns error).

**Proposed change**

- Extract shared setup into a function, e.g. `setupAgentAndSession(resumeID string) (agent *agent.Agent, sess *session.Session, sessionsDir, cwd string, err error)`, that:
  - Loads config, gets cwd, creates readFile tool and client, builds agent, ensures sessionsDir exists, loads or creates session. Returns all values needed by both runTUI and runPromptMode.
- `runTUI(resumeID)` and `runPromptMode(prompt, resumeID)` call `setupAgentAndSession(resumeID)` and then do only the TUI run or the Process + persist + print. runPromptMode can be changed to return an error and let the root command’s RunE (or main) handle os.Exit(1) so error handling style is consistent.

**Benefit**: Single place for “create agent and session”; easier to add flags or tools; consistent error handling.

**Priority**: Medium

---

### 7. Extract default system prompt to a separate file or constant block

**Location**: `internal/agent/agent.go` — `DefaultSystemPrompt` (lines 19–23)

**Current state**

- A long multi-line string constant sits in the middle of the file and dominates the first screen of the package. Editing or reviewing the prompt requires scrolling past it when working on types and functions.

**Proposed change**

- Move `DefaultSystemPrompt` to a separate file in the same package, e.g. `internal/agent/prompt.go`, containing only the constant (and optionally a short comment). Alternatively, keep it in `agent.go` but move it to the bottom of the file so the main types and logic stay at the top. No change to behavior or tests.

**Benefit**: Readability; prompt edits don’t clutter the main agent logic.

**Priority**: Low

---

## Suggested order

1. **Proposal 1 (viewport refresh)** — Small, localized change; reduces duplication in the same file and simplifies later edits to Update.
2. **Proposal 2 (session persist)** — Removes duplication across two packages and defines a clear “persist after reply” contract; call sites become trivial.
3. **Proposal 5 (llm.Chat)** — Quick cleanup; no dependency on others.
4. **Proposal 3 (split Update)** — Easier after viewport refresh is a helper; handlers can call the same helper.
5. **Proposal 4 (agent tool-call)** — Independent; can be done before or after 3.
6. **Proposal 6 (root setup)** — Touches both runTUI and runPromptMode; do after 2 so persist flow is already behind session.PersistAfterReply.
7. **Proposal 7 (system prompt file)** — Optional; do anytime for readability.

## Out of scope

- **Large TUI refactor** (e.g. separate models for viewport vs input, or full Bubble Tea sub-models): Would improve structure but is a bigger change; left for a dedicated task if desired.
- **Introducing interfaces for session/agent in cmd** for testability: Would allow mocking in root tests but adds indirection; deferred unless integration tests are added.
- **Changing Bubble Tea Model to pointer receiver everywhere**: Would require a broader API change; not proposed here.
