# P1 Local Agent Experience

## Status

Mostly complete for the current local UX push. Remaining unfinished items are
deferred because they are not high priority today.

This document supports the P1 item in [../ROADMAP.md](../../ROADMAP.md). It assumes
P0 Agent Core Stability is near completion and describes how CLI/TUI and Desktop
should become a complete local Agent experience on top of that core.

### Implementation progress

| # | Gap | Status |
|---|-----|--------|
| P1-1 | Local-only mode | ✅ Done — removed login gate from CLI and Desktop; replaced with model config check |
| P1-2 | Shared project/session model | Deferred — keep CLI workspace-first; revisit only when path-based Desktop grouping is not enough |
| P1-3 | Session management depth | ✅ Done (minimal slice) — filter, rename, delete, Desktop pin and clear-project sessions |
| P1-4 | Run progress visibility | ✅ Done (status-line slice) — shared ctx/tokens telemetry added; noisy activity timeline deferred |
| P1-5 | Local output and artifact model | Ignored — local outputs stay in the workspace; richer artifact browsing belongs to Portal/background runs |
| P1-6 | File and diff awareness | ✅ Done (lightweight slice) — git-backed changed-file and diff viewer in TUI `/diff` and Desktop Diff drawer |
| P1-7 | Settings and capability control | Ignored — current `/model`, `/mcp`, `/skills`, and `/tools` visibility is enough for the near-term local scope |
| P1-8 | CLI scriptability | ✅ Done — `-p` print mode adds `--output {text,json,jsonl}`, `--no-stream`, `--quiet`, `--workspace`, `--include-deltas`, and stable exit codes (0/2/3/4/6) |
| P1-9 | Desktop workbench maturity | Deferred after 9a — run cancel landed; keyboard shortcuts, richer empty/error states, per-project settings, and App.jsx extraction are not high priority today |
| P1-10 | Onboarding and diagnostics | Deferred — useful follow-up, but not high priority today |
| P1-11 | Local UX regression coverage | Deferred — useful follow-up, but not high priority today |
| P1-12 | TUI transcript mode and markdown rendering | ✅ Done — TUI uses terminal scrollback transcript mode with rendered Markdown assistant replies |

## Product Position

CLI/TUI and Desktop are the direct expression of what one BuildMax Agent can do
for one user on one machine.

They are not lightweight companions to Portal. They are first-class product
surfaces:

- CLI/TUI is the fast, terminal-native surface for developers and automation.
- Desktop is the local workbench for users who want project/session management,
  streaming chat, approvals, and local visibility without living in a terminal.
- Portal is optional for team operation, background execution, governance, and
  shared outcomes.

The local product promise is:

> A user can install BuildMax, configure a model, open a local workspace, and get
> a complete useful Agent experience without deploying or logging into Portal.

## Relationship To P0

P1 should consume the stable Agent Core rather than reimplement Agent behavior in
each surface.

P0 provides:

- shared tool execution and policy checks
- interactive and non-interactive approval modes
- context compaction and token-budget behavior
- model-call resilience and failure classification
- MCP, skills, and subagent behavior
- worker/local runtime parity
- an eval harness for core behavior

P1 turns those capabilities into a local user experience:

- project and session ergonomics
- visible tool progress and approvals
- local results, artifacts, and file changes
- settings and capability visibility
- scriptable CLI output
- Desktop workbench polish

## Current BuildMax Baseline

BuildMax already has a meaningful local foundation.

CLI/TUI:

- `buildmax` starts the Bubble Tea TUI.
- `buildmax -p QUERY` runs print mode.
- `-r`, `--continue`, and `--session-id` support session resume.
- `--model` can select a configured model.
- login/logout/whoami commands exist.
- TUI streams assistant output.
- TUI supports interactive tool approval.
- Slash panels exist for `/diff`, `/mcp`, `/model`, `/session`, `/skills`, and `/tools`.
- Footer shows model, workspace, branch, and user information.

Desktop:

- Wails app runs the same Go AgentApp.
- Projects are local folders saved under BuildMax data.
- Sessions are listed and grouped under projects.
- Chat streams assistant output through Wails events.
- Desktop supports interactive tool approval.
- Desktop exposes model, MCP, skills, agents, git branch helpers, and a Diff drawer.
- Desktop has local project create, rename, delete, and folder picker flows.

Shared local foundations:

- CLI and Desktop both use `internal/agentapp`.
- Sessions live under the shared BuildMax data directory.
- MCP, skills, subagents, model config, and policy are assembled through the
  AgentApp layer.

## Gaps

| Priority | Gap | Current State | Why It Matters |
| --- | --- | --- | --- |
| P1-1 | Local-only mode | CLI and Desktop currently require login before local chat. | The product promise says users can use local surfaces without deploying Portal. Portal login should unlock sync/team features, not gate local Agent use. |
| P1-2 | Shared project/session model | Desktop has local projects. CLI only has workspace + sessions. Sessions are shared, but project ownership and metadata are not a unified local concept. | Users should understand where work happened and resume it reliably across CLI and Desktop. |
| P1-3 | Session management depth | CLI `/session` shows sessions, but does not provide a full picker/rename/delete/search flow. Desktop lists sessions, but lacks stronger management such as search, delete, rename, pin, and recent filters. | Local Agent value compounds through resumable work. Session friction makes the product feel disposable. |
| P1-4 | Run progress visibility | Local surfaces show streamed text and some tool calls, but not a complete event timeline with model calls, approvals, tool start/end, compaction, retries, and usage. | Users need to know what the Agent is doing, especially when tools and approvals are involved. |
| P1-5 | Local output and artifact model | Deferred for local surfaces: generated files, summaries, test results, and assets should live directly in the user's workspace. Portal/background runs keep the first-class artifact browser need. | Local users should not need a separate artifact model when the workspace is already the durable output surface. |
| P1-6 | File and diff awareness | CLI/TUI and Desktop now provide a lightweight git-backed changed-files and diff viewer. Run-level file-change summaries remain out of scope for this slice. | File mutation is the core trust moment for a local coding/file assistant. |
| P1-7 | Settings and capability control | `/model`, `/mcp`, `/skills`, and `/tools` expose visibility, but editing model/MCP/policy settings still requires manual config knowledge. | Local users need an ergonomic setup and troubleshooting loop. |
| P1-8 | CLI scriptability | Print mode is useful, but lacks structured output modes, event output, quiet/no-stream modes, and predictable exit-code semantics for automation. | CLI must be good for humans and scripts. |
| P1-9 | Desktop workbench maturity | Project/session/chat flows exist, but run cancel, multi-run state, richer empty states, keyboard shortcuts, and per-project settings are incomplete. | Desktop should feel like a real local Agent workbench, not only a chat shell. |
| P1-10 | Onboarding and diagnostics | Local setup errors for auth, model config, MCP, skills, and workspace permissions are still scattered. | The first-run path should be boring: configure, verify, run. |
| P1-11 | Local UX regression coverage | There are component and unit tests, but no dedicated local-product smoke suite across CLI and Desktop flows. | P1 will touch interaction-heavy surfaces; regressions are easy without scripted coverage. |

## P1 Task Plan

### 1. Local-Only Mode

Make local Agent usage possible without Portal login.

Tasks:

- split local Agent availability from server authentication
- allow CLI/TUI, print mode, and Desktop chat to run with local model settings
- show login as an optional "connect to server" path for team/Portal features
- preserve current login commands for server-backed workflows
- define which features require login and which do not

Acceptance:

- a fresh local user can configure a model and run a local chat without a server
- missing login never blocks purely local work
- server-connected state is visible but not intrusive

### 2. Shared Local Project And Session Model

Create a local project/session model shared by CLI and Desktop.

Decision: defer this task for now. The intended CLI experience is that users
open a terminal in a workspace and run `buildmax` directly; introducing a
user-facing project concept in CLI would make that flow heavier. For the
near-term P1 scope, treat the resolved workspace path as the shared local
identity and keep Desktop's project model as a Desktop-facing folder/session
organizer. Revisit a persisted shared `project_id` only if path-based grouping
becomes insufficient.

Tasks:

- define `Project` as a local workspace folder plus display metadata
- associate sessions with project/workspace consistently
- let CLI discover or create a project record for the current workspace
- let Desktop reuse the same project/session metadata
- store `last_used_at`, model override, and local settings per project where useful

Acceptance:

- a session created in CLI appears under the correct Desktop project
- a Desktop project can be resumed from CLI by workspace or project ID
- project metadata uses the repository persistence conventions such as
  snake_case JSON keys

### 3. Session Management

Turn sessions into a first-class local workflow.

Decision: keep this slice small. CLI/TUI supports filtering, selecting,
renaming, and deleting from `/session`; Desktop adds sidebar search, per-session
rename/delete/pin, and robust project-level clear sessions for sessions shown
under a project. Pinning is local sidebar ordering only, not a global favorites
system.

Tasks:

- CLI `/session` becomes a navigable picker, not only a display panel
- add session commands for list, open/resume, rename, delete, and search
- Desktop supports search, rename, delete, and pin/recent filters
- expose session metadata: project, model, created time, updated time, token usage
- preserve compatibility with existing session files

Acceptance:

- users can find and resume previous local work quickly
- deleting/renaming sessions is available in both CLI and Desktop
- session operations do not mutate conversation history accidentally

### 4. Run Progress And Event Timeline

Use the P0 runtime event model as the local progress surface.

Decision: keep the default surface quiet. Users usually do not need a full
activity timeline, but they do benefit from lightweight runtime telemetry. The
completed P1-4 slice adds a shared status line for CLI/TUI and Desktop:
`ctx: 38% (12.4k/32k) | tokens: 23k input / 183 output`. Fuller activity
timelines remain a later progressive-disclosure feature.

Tasks:

- render tool start/end events in CLI and Desktop
- show approvals inline with the run
- expose retries, context compaction, loop guards, and model-call errors
- distinguish model thinking/progress from final assistant reply
- persist event metadata with the run or session where appropriate

Acceptance:

- users can answer "what is the Agent doing right now?"
- users can inspect "what did the Agent do in this run?"
- CLI remains concise while Desktop can show richer details

### 5. Local Outputs And Artifacts

Create a local output model for Agent runs.

Decision: defer this for local CLI/TUI and Desktop. For local usage, the
workspace is the output model: files produced by the Agent should be written to
the project folder where the user can inspect, version, and edit them with their
normal tools. A separate artifact browser is more appropriate for Portal and
background task runs, where outputs may be produced remotely and need an
application-managed retrieval surface.

Tasks:

- keep local generated outputs in the workspace
- avoid introducing a separate local artifact directory or browser
- leave first-class artifact browsing to Portal/background task-run surfaces

Acceptance:

- local work produces durable outputs as normal workspace files
- users can use git, `/diff`, Desktop Diff, and their IDE to inspect outputs
- local surfaces do not duplicate Portal's artifact model

### 6. File And Diff Awareness

Make file changes visible and reviewable locally.

Decision: keep this slice intentionally lightweight and familiar. The local
surfaces provide a read-only, IDE-like two-pane changed-files view backed by the
current git workspace state. The left pane is a flat file list with status glyphs
for added, modified, deleted, and renamed files; the right pane shows the
selected file's unified diff with line numbers. TUI opens it with `/diff`;
Desktop opens it through a dedicated Diff button. BuildMax does not track which
tool changed each file, connect file changes to approvals, or attempt to replace
the user's IDE for deeper review.

Tasks:

- show changed files in the current git workspace
- provide a read-only diff viewer in CLI/TUI and Desktop
- use the same two-pane layout in both local surfaces
- support added, modified, deleted, and renamed file states
- keep advanced review/edit/revert flows in the user's IDE

Acceptance:

- users can quickly see what changed without leaving TUI or Desktop
- local surfaces support the trust loop around file mutation
- diff UI is read-only unless/until checkpoint/rollback is implemented

### 7. Local Settings And Capability Surface

Make model, MCP, skills, tools, policy, and subagents understandable locally.

Tasks:

- add a local settings view or command group for model configuration
- show MCP server health and allow refresh/reload
- show skills and where they were loaded from
- show subagent definitions and effective tool permissions
- show current approval/policy mode
- provide actionable diagnostics when config is invalid

Acceptance:

- users can tell which capabilities are active in the current project
- common setup problems are visible without reading code
- settings visibility is consistent across CLI and Desktop

### 8. CLI Human And Script Modes

Strengthen CLI as both an interactive and automation surface.

Decision: a single slice landed the scripting surface. Print mode keeps the
existing human-readable streaming default and adds:

- `--output {text,json,jsonl}` — `text` (default) is today's streaming reply
  plus stats footer; `json` emits a single envelope at end of run; `jsonl`
  emits one JSON event per line from the agent runtime followed by a final
  `{"type":"result", ...}` envelope.
- `--no-stream` — suppress incremental stdout writes; reply is printed at end.
- `--quiet` / `-q` — suppress the `---` stats footer in text mode.
- `--workspace DIR` — override the agent workspace (default: current directory).
- `--include-deltas` — opt-in to emit `llm_delta` events in `jsonl` (off by
  default to keep volume manageable).

Exit codes:

| Code | Meaning |
| --- | --- |
| 0 | Success |
| 1 | Other / unexpected error |
| 2 | Usage or config error (bad flag, missing model, invalid session-id) |
| 3 | Tool blocked by configured policy |
| 4 | LLM / agent runtime error |
| 5 | Reserved for future tool-execution failure classification |
| 6 | User cancelled (SIGINT / SIGTERM) |

The JSON envelope is the stable contract for scripts:

```json
{
  "session_id": "...",
  "model": "...",
  "workspace": "...",
  "reply": "...",
  "tool_calls": 2,
  "duration_ms": 1234,
  "usage": {"prompt": 10, "completion": 20, "total_prompt": 100, "total_completion": 200},
  "context": {"tokens": 5000, "window": 32000},
  "exit_code": 0,
  "error": null
}
```

In `jsonl` mode the same envelope is emitted as the last line with an extra
`"type": "result"` field. Per-event lines use the type tags `llm_start`,
`llm_end`, `tool_start`, `tool_end`, `tool_denied`, `context_compacted`, and
`run_end`. Field naming follows the repo's snake_case persistence convention.

Acceptance:

- shell scripts can invoke BuildMax predictably (`buildmax -p Q --output json`
  produces one parseable line on stdout; non-zero exit signals what went wrong)
- humans still get streaming and readable stats by default

### 9. Desktop Workbench Polish

Make Desktop feel like a focused local Agent workbench.

Decision: ship as small slices rather than one bundled refactor. Sliced as
9a run cancel, 9b keyboard shortcuts, 9c empty/error states (overlaps with
P1-10), 9d per-project model/settings entry, 9e App.jsx component extraction.
9a landed first because the missing Stop button was the most user-visible
footgun.

Tasks:

- improve project picker and recent project behavior
- ✅ 9a — add run cancel and disabled states for concurrent sends
  (cooperative `context.Cancel`; agent loop returns partial reply via existing
  ctx-cancel path; shared `ChatComposer` gained an optional `onCancel` prop
  that swaps the loading-state Send button for a Stop button; one in-flight
  run per project enforced server-side; empty assistant placeholder dropped
  when cancellation produces no content)
- add per-project model/settings entry points
- add keyboard shortcuts for send, new chat, search, and command palette
- improve empty/error states for missing model config, missing folder, and MCP errors
- keep shared GUI components aligned with Portal where they are purely presentational

Acceptance:

- Desktop supports the common local loop: choose project, chat, approve, inspect,
  resume, and review results
- Desktop remains local-first and does not duplicate Portal team administration

### 11. TUI Transcript Mode And Markdown Rendering

Replace the current `WithAltScreen` full-screen mode with a hybrid terminal
model: chat history prints to the normal terminal scrollback buffer, while only
the bottom input and status bar are managed by Bubble Tea.

**Motivation.** The `WithAltScreen` approach runs in an alternate screen buffer.
That buffer is invisible to the terminal's native scroll history and its contents
cannot be selected or copied. Users routinely want to copy a code block the Agent
just wrote or scroll back to a tool result from earlier in the session. The current
mode makes both impossible without extra steps.

**Hybrid model.** Bubble Tea supports a "transcript" or "print" rendering mode
alongside a persistent live view. The design is:

- Chat messages (user turns, assistant reply deltas, tool call/result output) are
  printed directly to stdout as they arrive, becoming part of the terminal's normal
  scrollback history.
- The bottom strip — input composer, status line, active approval prompt, and
  drawer panels — is rendered by Bubble Tea as a compact live region anchored to
  the bottom of the terminal.
- The full-screen alternate buffer is removed. The terminal is never hijacked; the
  user's working directory and previous output remain visible above.

This model matches how tools like `git log | less` or `npm install` behave: output
accumulates above, the interactive prompt stays at the bottom.

**Markdown rendering.** Printing to scrollback also unlocks per-message Markdown
rendering. Flow:

1. While an assistant turn is streaming, raw text is printed incrementally to
   scrollback (same as today but not in the alt-screen buffer). This preserves
   low-latency first-token display.
2. When the turn's stream ends, the completed raw text is cleared from the visible
   scrollback and replaced with a fully rendered Markdown version using
   `charmbracelet/glamour` (or equivalent Go terminal Markdown renderer). The
   rendered output includes styled headers, bold/italic, fenced code blocks with
   syntax highlighting, and bullet lists.
3. Tool output and status messages are printed as plain text; Markdown rendering
   applies only to assistant reply turns.

**Implementation tasks:**

- Replace `WithAltScreen` in the Bubble Tea program with a non-alt-screen startup
  option and use `tea.Println` / `fmt.Fprintf(os.Stdout, ...)` for scrollback
  content.
- Implement a bottom-anchored Bubble Tea view that only renders the input
  composer, status/footer bar, and optional drawer (approval, slash panels).
- Pipe streaming assistant deltas through a print path; accumulate the full turn
  text in a buffer.
- On turn end, emit ANSI cursor-up/erase sequences to replace the streamed raw
  text with the `glamour`-rendered output. If the terminal does not support ANSI
  erase (e.g. redirected stdout), skip the replacement and leave raw text.
- Add a `--no-render` flag to disable Markdown rendering for scripting or minimal
  terminals.
- Preserve existing slash panel behavior (open panel replaces bottom region, not
  a full-screen takeover).

**Acceptance:**

- users can select and copy any chat text using the terminal's native selection
- terminal native scroll (trackpad, keyboard scroll, `tmux` scroll-mode) works
  through the full chat history
- assistant replies containing code blocks are syntax-highlighted in capable
  terminals
- the bottom input strip remains live and responsive while history scrolls above
- `buildmax -p QUERY` (print/non-interactive mode) is unaffected

### 10. Local UX Smoke Suite

Add regression coverage for local product flows.

Initial cases:

- CLI print mode without server login
- CLI TUI starts and loads current workspace
- CLI session picker resumes a session
- CLI model selection changes the active model
- Desktop creates a project and starts a chat
- Desktop resumes a session created by CLI
- Desktop approval allow/deny works
- Desktop shows MCP/skills/model diagnostics
- local run output is persisted and discoverable

Acceptance:

- P1 local UX changes can be merged without manually retesting every flow

## Suggested Implementation Order

Decision: the remaining unfinished local UX items are deferred for now. Keep
this order as historical context for when the local surface becomes a priority
again.

1. Local-only mode and first-run model diagnostics.
2. Shared local project/session metadata.
3. CLI session picker and session management commands.
4. Desktop project/session management polish.
5. Runtime event rendering in CLI and Desktop.
6. File/diff awareness.
7. Local settings and capability surface.
8. CLI structured output and exit-code semantics.
9. TUI transcript mode and Markdown rendering.
10. Local UX smoke suite.

## Non-Goals For P1

- Portal issue/workflow/team administration inside Desktop
- team-scoped governance UI
- full IDE replacement features
- complex Git restore UI
- remote worker orchestration from local-only mode
- adding many new tools before current local capabilities are visible and reliable

## Decision Summary

P1 should make BuildMax feel complete when used locally.

The first milestone is:

- local-only Agent usage without Portal login
- shared project/session model across CLI and Desktop
- better session management
- visible run progress and approvals
- local outputs and file-change awareness
- settings/capability diagnostics

After P1, a user should be able to live entirely in CLI/Desktop for personal
Agent work, while still having a clean path to connect Portal when team
operation becomes useful.
