# Design 010 - TUI wiring

## Goal

Wire the Bubble Tea TUI to the agent and session so users can run `buildmax` (or `buildmax --resume <id>`), chat in a scrollable layout (banner + history, input, footer), send messages to the agent, see replies and tool-call lines, and have the session persisted.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **cmd/buildmax** | CLI entry; TUI vs prompt mode; build agent, session, and TUI deps; pass them into app. | root command, runRoot, runTUI (or inlined TUI startup), runPromptMode. |
| **internal/app** | Bootstrap TUI: build `tea.Model` from dependencies (agent, session, display config). | NewModel(opts) that delegates to tui. |
| **internal/tui** | Bubble Tea model: viewport (banner + chat), input, footer; Update handles keys and async agent result; format messages for display. | Model, viewport, input component, format helpers, custom Msg types. |
| **internal/agent** | (existing) Process(ctx, sess, userMessage) → reply; mutates session. | No change. |
| **internal/session** | (existing) Session, LoadFromDir, SaveToDir, Messages(), Append. | No change. |
| **internal/config** | (existing) LoadLLM(), DataDir(). | No change. |
| **internal/llm** | (existing) Message, ToolCall types. | No change. |

## Structure

**Directory / files**

- `cmd/buildmax/`
  - `root.go` — Relax `--resume` so it can be used without `-p`; add TUI startup that creates agent + session (new or load), ensures sessions dir exists, then calls `app.NewModel(opts)` and `tea.NewProgram(..., tea.WithAltScreen())`.
- `internal/app/`
  - `app.go` — NewModel accepts a single opts struct (or equivalent) containing: agent, session, model name, workspace path, version, sessions dir; returns `tea.Model` by calling `tui.NewModel(opts)`.
- `internal/tui/`
  - `model.go` — Model struct (viewport, input, opts refs, busy, width/height); NewModel(opts); Init, Update, View; handle KeyMsg, WindowSizeMsg, and custom agent-done message.
  - `messages.go` (or in model.go) — Custom tea.Msg types: e.g. `agentDoneMsg struct { Reply string; Err error }`.
  - `format.go` — Pure helper: given `llm.Message`, return display lines (e.g. "you: ...", "assistant: ...", "* tool_name (args)"); testable without Bubble Tea.
  - `model_test.go` — Unit tests for format helper (and optionally View contains "BUILDMAX").
- `go.mod` — Add dependency: `github.com/charmbracelet/bubbles` (viewport, textarea) for scrollable content and input field.

**Main types and interfaces**

- **Model** (internal/tui): Root Bubble Tea model. Fields: viewport (bubbles/viewport or equivalent), input (bubbles/textarea or textinput), agent (*agent.Agent), session (*session.Session), modelName, workspace, version, sessionsDir, busy bool, width/height int. Implements tea.Model.
- **TUIOpts** (internal/app or internal/tui): Struct passed from cmd to app to tui: Agent *agent.Agent, Session *session.Session, ModelName, WorkspacePath, Version, SessionsDir string. Keeps constructor signatures stable.
- **agentDoneMsg** (internal/tui): tea.Msg sent when agent.Process finishes; holds Reply string and Err error so Update can save session, clear busy, and optionally show error.

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|-----------------|
| **root (cmd)** | runRoot | (cmd, args) error | If `-p` set → runPromptMode; else build agent+session (new or LoadFromDir if --resume), MkdirAll(sessionsDir), run TUI with app.NewModel(opts). Relax validation: --resume without -p starts TUI with that session. |
| **root (cmd)** | runTUI | (opts) error | Build tea.Program(app.NewModel(opts)), Run(); return error. Optional: factor from runRoot for clarity. |
| **app** | NewModel | (opts TUIOpts) tea.Model | Return tui.NewModel(opts). |
| **tui** | NewModel | (opts TUIOpts) Model | Build Model with viewport (content = banner + formatted messages), input component, opts stored; set initial dimensions from 0 or default; return Model. |
| **Model** | Init | () tea.Cmd | Return nil (no async init). |
| **Model** | Update | (tea.Msg) (tea.Model, tea.Cmd) | KeyMsg: ctrl+c / q → tea.Quit; Enter in input → submit text, set busy, return Cmd(runAgent(text)) that sends agentDoneMsg. WindowSizeMsg: store width/height, resize viewport, return nil. agentDoneMsg: if Err != nil show error in UI; else session already updated by Process, call session.SaveToDir(sess, sessionsDir), clear busy; return updated model, nil. Delegate key handling to viewport when focus there, to input when focus there. |
| **Model** | View | () string | Compose: viewport.View() (scrollable banner + chat), then input.View(), then footer line "model: ... \| workspace \| ctrl+c: quit". Ensure viewport height = total height minus input and footer lines. |
| **tui** | formatMessage | (m llm.Message) []string | Return display lines: user → ["you: "+content]; assistant → ["assistant: "+content] plus for each ToolCalls one line " * "+name+" ("+shortArgs+")"; tool → optional [" * result: ..."] or omit. shortArgs = e.g. first arg or truncated JSON. |
| **tui** | buildViewportContent | (session, version string) string | Return banner line "BUILDMAX "+version plus newline plus all lines from session.Messages() via formatMessage; single string for viewport.SetContent. |

(Agent.Process, session.SaveToDir, session.LoadFromDir, config.LoadLLM, config.DataDir remain unchanged; used by cmd and by tui.)

## How they work together

**Data/control flow**

1. User runs `buildmax` or `buildmax --resume <id>`. runRoot builds LLM config, cwd, read_file tool, client, agent; creates or loads session; builds TUIOpts; calls tea.NewProgram(app.NewModel(opts), tea.WithAltScreen()) and Run().
2. TUI Init runs; no command. View renders viewport (banner + chat from session.Messages()), input, footer.
3. User types in input and presses Enter. Update handles KeyMsg: submit text, set busy, return tea.Cmd(func() tea.Msg { reply, err := agent.Process(ctx, sess, text); return agentDoneMsg{reply, err} }).
4. Program runs Process in background; when done, agentDoneMsg is sent to Update. Update: if err != nil, store error for View to show and clear busy; else session already has new messages from Process, call session.SaveToDir(sess, sessionsDir), clear busy. View re-renders with new history.
5. On WindowSizeMsg, Model stores dimensions and resizes viewport so scrollable area height = total - input height - footer lines; viewport shows banner + chat and scrolls as one.
6. On ctrl+c or q, Update returns tea.Quit. Optionally before quit, if there are unsaved changes, save session (task says "optionally on quit").

**Dependencies**

- cmd/buildmax depends on internal/app, agent, session, config, llm, tools; creates TUIOpts and passes to app.
- internal/app depends on internal/tui; passes TUIOpts to tui.NewModel.
- internal/tui depends on internal/agent, internal/session, internal/llm (for Message), and github.com/charmbracelet/bubbles (viewport, textarea). Does not depend on config or tools (receives model name and paths via opts).

**Key data structures**

- **TUIOpts**: Built in cmd from agent, session, cfg.Model, cwd, Version, sessionsDir; passed to app.NewModel and into tui.NewModel so TUI can call agent.Process and session.SaveToDir and display model/workspace/version.
- **agentDoneMsg**: Created inside the tea.Cmd that runs agent.Process; sent back to Update so the main loop can persist session and clear busy without blocking the UI.
- **Viewport content string**: Built in tui from "BUILDMAX "+version and session.Messages() via formatMessage; set on viewport so banner and chat scroll together.

## Changes for review

- **New**: `cmd/buildmax/root.go` — Relax flag validation: allow `--resume <id>` without `-p`; when starting TUI, build agent and session (new or LoadFromDir), ensure sessionsDir exists, build TUIOpts, call app.NewModel(opts) and tea.NewProgram(..., tea.WithAltScreen()). Run program and return error.
- **New**: `internal/app` — TUIOpts type (or accept a struct from cmd). NewModel(opts) returns tui.NewModel(opts). App may define TUIOpts or receive it from cmd; if defined in app, cmd builds it and passes to app.NewModel.
- **Modified**: `internal/app/app.go` — NewModel signature and body: accept opts (agent, session, modelName, workspace, version, sessionsDir), return tui.NewModel(opts).
- **New**: `internal/tui` — Model holds viewport, input, opts (or individual refs), busy, width, height. NewModel(opts) constructs Model. Init returns nil. Update handles KeyMsg (quit, submit → run agent Cmd), WindowSizeMsg (resize), agentDoneMsg (save session, clear busy). View composes viewport + input + footer.
- **New**: `internal/tui/messages.go` (or in model.go) — agentDoneMsg struct { Reply string; Err error } implementing tea.Msg.
- **New**: `internal/tui/format.go` (or in model.go) — formatMessage(m llm.Message) []string; buildViewportContent(session, version) string. Pure functions for chat display.
- **New**: `internal/tui/model_test.go` — Unit test(s) for formatMessage: given user message, assistant with tool_calls, tool message; assert output contains "you:", "assistant:", "* read_file (...)".
- **New**: `go.mod` — Add `github.com/charmbracelet/bubbles` (viewport, textarea) for scrollable content and input field.
- **Modified**: `AGENTS.md` (or README) — Document that `buildmax` starts TUI; `buildmax --resume <id>` starts TUI with that session; layout (scrollable banner+chat, input, footer) and session persisted after each reply.
