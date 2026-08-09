# TUI

## Purpose

The TUI lives inside `internal/interface/cli` and uses Bubble Tea
(charmbracelet). It provides an interactive chat experience with a scrollable
viewport, text input, status footer, and built-in slash commands.

## Key Types and Interfaces

| Name | Kind | Role |
|------|------|------|
| **Model** | struct | Root Bubble Tea model: viewport, input, state (busy, focus, dimensions) |
| **TUIOpts** | struct | Configuration: agent app/session, model name, workspace, version, sessions dir |
| **ViewportBlock** | struct | Scrollable area displaying banner and chat history |
| **InputBlock** | struct | Text input area at the bottom for user messages |
| **agentDoneMsg** | struct | Internal message: agent reply or error |
| **carouselTickMsg** | struct | Internal message: animate "..." while waiting for agent |

## How It Works

### Layout

```
┌──────────────────────────┐
│  BUILDMAX v0.0.1         │  ← Banner (in viewport)
│                          │
│  user: Hello             │  ← Chat history (in viewport)
│  assistant: Hi there!    │
│                          │
├──────────────────────────┤
│ ╭──────────────────────╮ │  ← Input box (rounded border, light sky blue)
│ │ Type here...         │ │
│ ╰──────────────────────╯ │
│ model: ... | @/path | .. │  ← Footer (one line)
└──────────────────────────┘
```

### Message Flow (Bubble Tea Architecture)

1. **User types and presses Enter**: `handleKeyMsg` captures the text, appends a user message to the session, refreshes the viewport, sets `busy = true`, and starts two concurrent commands:
   - A **carousel tick** timer for the "..." animation.
   - A **background goroutine** running the shared agent runtime through `internal/agentapp`.

2. **While busy**: Input shows "Waiting for reply...", carousel dots animate in the viewport.

3. **Agent finishes**: `agentDoneMsg` is received. The viewport refreshes with the assistant reply, session is persisted via `PersistAfterReply`, and `busy` is set to false.

### Focus Management

- **Input focus** (default): Keystrokes go to the textarea. Scroll keys (Up/Down/PgUp/PgDown) switch focus to viewport.
- **Viewport focus**: Arrow keys scroll. Enter/Escape return focus to input. After `scrollIdleDelay` (1500ms) of no scrolling, focus auto-returns to input.
- **Tab** toggles focus manually.
- **Mouse wheel** scrolls the viewport (temporarily steals focus).

### Key Bindings

| Key | Action |
|-----|--------|
| Enter | Submit message (input focus) / Return to input (viewport focus) |
| Escape | Clear input (input focus) / Return to input (viewport focus) |
| Ctrl+C or q | Quit |
| Tab | Toggle focus between input and viewport |
| Up/Down/PgUp/PgDown/Home/End | Scroll viewport |

### Supporting Components

- **ViewportBlock** (`viewport_block.go`): Wraps Bubble Tea's viewport. `RefreshAndGotoBottom()` rebuilds content from session history and scrolls to bottom.
- **InputBlock** (`input_block.go`): Wraps Bubble Tea's textarea. Auto-syncs height based on content.
- **Banner** (`banner.go`): Renders the "BUILDMAX" ASCII banner with version.
- **Format** (`format.go`): `formatMessage()` renders chat messages, `buildViewportContent()` assembles banner + history, `wrapLine()` handles line wrapping.

## Dependencies

- **Uses**: `internal/agentapp`, `internal/core/session`, `internal/core/llm`
- **External**: `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/bubbles` (textarea, viewport), `github.com/charmbracelet/lipgloss` (styling)
- **Used by**: `internal/interface/cli`, `cmd/buildmax` (starts TUI)

## Notes

- The TUI uses `tea.WithAltScreen()` for full-screen mode.
- Session is persisted after every assistant reply (not on quit).
- The CLI starts the TUI through `internal/interface/cli`.
- See also: [Agent Loop](agent-loop.md), [Session](session.md), [CLI](cli.md).
