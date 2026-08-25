# TUI

> **Audience:** contributors · **Status:** current
>
> User-facing key and slash-command reference: [reference/cli.md](../../reference/cli.md)

## Purpose

The Bubble Tea terminal UI, inside `internal/interface/cli`. `tui.go` and
`tui_model.go` hold the root model; the `chat_*.go` files hold the pieces —
input, formatting, styles, approval, and one file per slash panel.

## Layout

```text
┌────────────────────────────────────────────┐
│  BUILDMAX v1.2.3                           │  banner
│  user: add pagination to the list endpoint │  history
│  assistant: I'll start by reading…         │  live streaming text
│  ⟳ Grep(pattern: "func List")              │  in-flight tool activity
├────────────────────────────────────────────┤
│ ╭────────────────────────────────────────╮ │
│ │ Type here…                             │ │  input
│ ╰────────────────────────────────────────╯ │
│ model: gpt-4o (local) | @~/proj (|-main) … │  footer line 1
│ 12.4k/128k · 2 tools | ctrl+c: quit | …    │  footer line 2
└────────────────────────────────────────────┘
```

Footer line 1: model with the mode it runs in, workspace with git branch,
sandbox tag when active, logged in email. The mode tag is `local` or the host of
the deployment this session is signed in to, and it is always shown: "where does
this send my prompts" must never depend on a tag being absent. Where prompts go
is a property of the app, not of the model entry, so `/model` switching models
mid-session does not move it — see
[client modes](../../design/client-modes.md). Line 2: run status (context usage,
token counts, tool calls), key hints, and a panel-specific hint when a slash
panel is open.

## Model State

`Model` (`tui_model.go`) carries the usual Bubble Tea state — dimensions, busy,
focus — plus the parts that make a live run legible:

| Field | Role |
|---|---|
| `streamingBuffer` | Assistant text accumulated so far this turn |
| `activeTools` | Tool calls in flight, keyed by call id so overlapping calls keep their own arguments |
| `streamChannel` | `chan tea.Msg` carrying deltas, tool events, and completion |
| `runStatus` | Context and token counters shown in the footer |
| `pendingApproval` | An approval request awaiting a keypress |
| `slash*` / `activePanel` | Slash panel state |
| `queue` | Messages typed during the run, waiting for their own turn |
| `pendingRecap` | A turn recap held until the reply it describes has printed |
| `inputBlock.ghost` | The predicted answer on offer, shown as the input's placeholder |

## How A Turn Runs

The agent runs on a background goroutine; everything it produces reaches the UI
through one channel, which keeps Bubble Tea's single-threaded update loop intact.

```text
Enter ─▶ append user message ─▶ busy = true
                                   │
              ┌────────────────────┴─────────────────────┐
              │ goroutine: agentapp run                  │
              │   StreamSink  ──▶ streamDeltaMsg         │
              │   EventSink   ──▶ tool / status messages │──▶ streamChannel ──▶ Update
              │   returns     ──▶ agentDoneMsg           │
              └──────────────────────────────────────────┘
```

- `streamSinkToChannel` implements `llm.StreamSink`, forwarding content deltas.
- `eventSinkToChannel` translates `agent.Event` values into UI messages —
  `EventLLMStart` becomes a run-status update, tool events become the activity
  line.

Both are adapters over the agent loop's existing seams; the TUI adds no
agent-side machinery of its own.

`tuiRunOwner` (`tui_runs.go`) owns the context for every foreground turn and
background delivery, plus the wait group that joins them. Closing the model
cancels that root context before `AgentApp.Close`, and channel sends select on
the same context. The goroutine that creates a stream channel is the only one
that closes it, after the run returns, so quitting cannot strand a sender or
close a channel while a producer still owns it.

## Typing During A Run

The input stays visible and editable while the agent works. `Enter` queues the
message instead of submitting it: the model appends it to `queue`
(`agent.MessageQueue`, capacity 10), prints a dim `⏸ queued #n` line to
scrollback, and shows the depth in the busy hint and the footer. `Esc` clears the
input, or takes back the last queued message when there is nothing to clear.
Slash commands are refused rather than queued — they act on live UI state, which
a turn later is not the state the user was looking at.

The queue is handed to the run as `RunPromptOpts.Pending`, so a queued message
normally joins the turn already in progress at its next iteration boundary rather
than waiting for the whole run: `EventUserInput` arrives on the stream channel and
the message is printed as sent. `agentDoneMsg` still returns a `drainQueueMsg`,
which starts anything queued after the run stopped reading — going through a
message rather than a direct call orders that next turn behind whatever the
finished one was still printing. A failed run drains too; see
[Queued messages](../../design/queued-messages.md).

## After A Turn

`RunPromptOpts.Digest` asks `agentapp` for a `TurnDigest` once the turn is done:
a short recap of what the turn did, and the answer the user is likely about to
type when the reply ended by asking them something. It costs one extra model
call, made by `AgentApp.runTurn` while it still holds the session, so what the
call spends is folded into the session's usage the same way title generation's
is. The TUI is the only surface that sets the flag.

Neither half is part of the conversation. The recap goes to scrollback as a dim
`❯❯` notice, printed under the reply it describes: `pendingRecap` holds it back
when the reply is only rendered at `agentDoneMsg`, because a recap above its own
turn reads as a recap of the previous one. The suggestion becomes the
textarea's placeholder, which is what makes it disappear the moment the user
types — the placeholder and a ghost suggestion have the same rule, so nothing
has to watch for it. `Tab` accepts it into the input; starting a turn or
pressing `Esc` withdraws it.

`agent.turn_digest` in `settings.yaml` switches either half off.

## Approval

`TUIApprovalHandler` (`chat_approval.go`) implements `agent.ApprovalHandler`. When
the tool policy returns "ask", the loop blocks on it, the model shows the pending
request, and a keypress resolves it. `n`, `N`, or `esc` deny.

This is the one place the TUI participates in the agent loop rather than
observing it — and it is why print mode, which passes no handler, never hangs
waiting for input.

## Slash Panels

`/model`, `/sessions`, `/tools`, `/skills`, `/mcp`, `/diff`. Each has a
`chat_*.go` file and its own state struct, unified behind the `slashPanel`
interface (`activePanel`, `openPanel`, `closeActivePanel`) so key handling and
the footer hint work the same for all of them.

## Keys

| Key | Action |
|---|---|
| Enter | Submit, confirm in a panel, or queue the message during a run |
| Esc | Clear input, dismiss a panel, deny an approval, or unqueue the last message |
| Tab | Accept the ghost suggestion; no-op when none is on offer |
| Ctrl+C | Quit |
| `/` then ↑↓ | Slash command completion |

## Dependencies

- **Uses**: `internal/agentapp`, `internal/core/agent` (Event, StreamSink,
  ApprovalHandler), `internal/core/session`
- **External**: `bubbletea`, `bubbles`, `lipgloss`, `glamour` (markdown rendering,
  styled by the theme detected once at startup)

## Notes

- Runs with `tea.WithAltScreen()`.
- The session is persisted after each assistant reply, not on quit, so a crash
  loses at most the turn in flight.
- See also: [CLI](cli.md), [Agent Loop](agent-loop.md), [Session](session.md).
