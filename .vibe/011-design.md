# Design 011 - TUI improvements

## Goal

Improve the Bubble Tea TUI with light sky blue styling, multi-line text wrapping for input and chat, immediate display of the user message on send, and an "assistant: . / .. / ..." carousel while waiting for the LLM.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/agent** | Run agent loop; support entry point when user message is already in session. | `Process`, `ProcessAfterUserAppended`, `processLoop`; no new types. |
| **internal/tui** | Styling (theme, input border, message bar), text wrapping (input height, chat line wrap), submit flow (append user → refresh view → run agent), carousel state and tick. | `model.go`, `format.go`; styles, `wrapLine`, `buildViewportContent(sess, version, width, busy, carouselDots)`. |

## Structure

**Directory / files**

- `internal/agent/`
  - `agent.go` — Add `ProcessAfterUserAppended`, extract `processLoop`; `Process` delegates to `processLoop` after appending user message.

- `internal/tui/`
  - `model.go` — Theme constant `lightSkyBlue`, `inputBoxStyle` with light sky blue border; `inputMaxLines`, `carouselTick`; `carouselDots` and `carouselTickMsg`; submit flow (append user, buildViewportContent with width/busy/carouselDots, GotoBottom, run `ProcessAfterUserAppended` + start tick); when busy show "Waiting for reply…" in input box.
  - `format.go` — `messageBarStyle` (light sky blue "| "); `wrapLine(line, width)`; `buildViewportContent(sess, version, width, busy, carouselDots)` with bar for user/assistant lines, wrap to width, optional carousel line.
  - `model_test.go` — Tests for carousel in viewport content and for `wrapLine`; update `buildViewportContent` call to new signature.

**Main types and interfaces**

- **Model** (tui): Adds `carouselDots int`; same viewport/input/busy/err/width/height; no new external types.
- **agentDoneMsg**, **carouselTickMsg** (tui): Message types for agent completion and carousel tick.

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| **Agent** | Process | `(ctx, sess, userMessage string) (reply string, err error)` | Append user message to session, then call processLoop. (Unchanged contract.) |
| **Agent** | ProcessAfterUserAppended | `(ctx, sess) (reply string, err error)` | Require last message in session is user; do not append; run processLoop. Error if session empty or last message not user. |
| **Agent** | processLoop | `(ctx, sess) (reply string, err error)` | Internal: build system + session messages, call LLM, handle tool_calls, append to session; repeat until final reply or max iterations. |
| **(tui)** | runAgentAfterUserAppended | `(opts TUIOpts) tea.Msg` | Run `Agent.ProcessAfterUserAppended(ctx, opts.Session)` and return agentDoneMsg. |
| **(tui)** | buildViewportContent | `(sess, version string, width int, busy bool, carouselDots int) string` | Banner + all messages (with "| " bar for user/assistant, wrap each line to width); if busy append "assistant: ." / ".." / "..." by carouselDots. |
| **(tui)** | wrapLine | `(line string, width int) []string` | Break line into chunks of at most width runes; return one or more lines. |

## How they work together

**Data/control flow**

1. User presses Enter with non-empty input. TUI: append user message to session via `session.Append(llm.Message{Role: "user", Content: text})`.
2. TUI: build viewport content with `buildViewportContent(sess, version, width, true, 0)` (busy, carousel at "."), set viewport content, GotoBottom, set `busy = true`, clear input.
3. TUI: send two commands in batch: (a) `tea.Tick(400ms, carouselTickMsg)` to advance carousel; (b) `tea.Cmd(runAgentAfterUserAppended(opts))` which calls `Agent.ProcessAfterUserAppended(ctx, sess)` in background.
4. Agent: `ProcessAfterUserAppended` validates last message is user, then runs `processLoop` (same loop as before: system + session → LLM → tool_calls → append assistant/tool → repeat). No duplicate user append.
5. On each `carouselTickMsg`, TUI increments `carouselDots` (0→1→2→0), rebuilds viewport content with busy + new carouselDots, sets content, GotoBottom, and schedules next tick. When `agentDoneMsg` arrives, TUI sets busy false, stops tick, rebuilds content without carousel, saves session, clears carouselDots.
6. When busy, input box shows "Waiting for reply…" instead of "... thinking ...".

**Dependencies**

- **internal/tui** depends on **internal/agent** for `ProcessAfterUserAppended`; on **internal/session** for `Append`, `Messages`, `SaveToDir`; on **internal/llm** for `llm.Message`.
- **internal/agent** unchanged dependencies (session, llm); no dependency on tui.

**Key data structures**

- **carouselTickMsg** (tui): Empty struct; sent by tea.Tick to advance carousel; Model.Update handles it when busy.
- **agentDoneMsg** (tui): Reply string and Err; sent when ProcessAfterUserAppended returns; Model stops carousel and refreshes viewport with final session content.

## Changes for review

- **New**: `internal/agent` — `ProcessAfterUserAppended(ctx, sess) (reply, err)`; `processLoop(ctx, sess) (reply, err)`; `Process` refactored to append user then call processLoop.
- **New**: `internal/agent` tests — `TestProcessAfterUserAppended`, `TestProcessAfterUserAppended_EmptySession`, `TestProcessAfterUserAppended_LastNotUser`.
- **Modified**: `internal/tui/model.go` — `lightSkyBlue`, `inputBoxStyle` border color; `inputMaxLines`, `carouselTick`; `Model.carouselDots`; `carouselTickMsg`; submit flow (append user, buildViewportContent(..., width, true, 0), runAgentAfterUserAppended + tea.Tick); handle `carouselTickMsg`; when busy show "Waiting for reply…"; textarea height set to inputMaxLines.
- **Modified**: `internal/tui/format.go` — `messageBarStyle`; `wrapLine(line, width)`; `buildViewportContent(sess, version, width, busy, carouselDots)` with bar, wrap, and optional carousel line.
- **New**: `internal/tui` tests — `TestBuildViewportContent_BusyCarousel`, `TestWrapLine`; update existing `buildViewportContent` call to 5-arg signature.
