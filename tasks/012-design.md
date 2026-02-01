# Design 012 - TUI scrollable banner and chat history

## Goal

Enable the user to scroll the banner-and-chat viewport to see old messages (mouse and keyboard), with input and footer fixed at the bottom, by adding a focus toggle (e.g. Tab) so scroll keys move the viewport when viewport has focus and do not steal arrow keys from the input when input has focus.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/tui** | TUI model and view: viewport (banner + chat), input, footer; focus state; key/mouse routing. | `model.go`, `format.go`, `banner.go`; layout and Update/View logic. |
| **github.com/charmbracelet/bubbles/viewport** | Scrollable content region; scroll position; KeyMap (Up/Down/PgUp/PgDown/Home/End). | Used as-is; we only route keys and mouse to it. |

No new packages. Format and banner stay unchanged; only `model.go` (and optionally `model_test.go`) change.

## Structure

**Directory / files**

- `internal/tui/` — TUI package (unchanged boundary)
  - `model.go` — **Modified**: add focus field, Tab handling, key routing to viewport when viewport focused; keep layout (viewport → input → footer) and MouseMsg → viewport.
  - `model_test.go` — **Modified**: add test(s) for focus toggle (Tab toggles focus state).
  - `format.go`, `banner.go` — Unchanged.

**Main types**

- **Model** (internal/tui): Add a boolean field for focus. Suggested name: `focusInput bool` (true = input has focus, false = viewport has scroll focus). Default true so behaviour stays “input focused” on start.
- **viewport.Model** (bubbles): Unchanged; already has `MouseWheelEnabled`, and will receive `tea.KeyMsg` when viewport has focus so its default KeyMap can scroll.
- **textarea.Model** (bubbles): Unchanged; when `focusInput` is true it receives keys as today. Optionally call `Blur()` when switching to viewport focus and `Focus()` when switching back so cursor/blink reflect focus (recommended for clarity).

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|-----------------|
| **Model** | Update | `(msg tea.Msg) (tea.Model, tea.Cmd)` | **Extended**: (1) Global keys (Ctrl+C, q) → quit unchanged. (2) Tab (or chosen key) → toggle `focusInput`; do not forward to input or viewport. When toggling to viewport focus: optionally `m.input.Blur()`; when toggling to input focus: optionally `m.input.Focus()`. (3) When `!focusInput` (viewport focused): forward `tea.KeyMsg` to `m.viewport.Update(msg)` and return viewport cmd; do not update input. (4) When `focusInput`: existing behaviour (Enter → submit, Escape → clear, KeyRunes → input, etc.). (5) `tea.MouseMsg` → viewport always (unchanged). (6) Other message types unchanged. |
| **Model** | View | `() string` | Unchanged: viewport area (m.viewport.View()), then input box, then footer. Layout remains fixed; only viewport content scrolls. |
| (none)   | NewModel | `(opts TUIOpts) Model` | Initialize new Model with `focusInput: true` (or equivalent) so input is focused by default. |

No new public APIs. Init() remains `tea.Batch(textarea.Blink, m.input.Focus())`.

## How they work together

**Data/control flow**

1. User runs TUI; `focusInput == true`. Keys go to input; mouse wheel goes to viewport. Layout: viewport (top) → input → footer (bottom).
2. User presses Tab: `Update` toggles `focusInput` to false; optionally Blur input. Subsequent KeyMsg (Up/Down/PgUp/PgDown/Home/End) are passed to `viewport.Update`; viewport scrolls. Mouse wheel still goes to viewport. Input does not receive keys.
3. User presses Tab again (or Enter/Esc, if implemented): `focusInput` to true; optionally Focus input. Keys go to input again; Tab does not insert a character.
4. Ctrl+C and q always quit in KeyMsg branch before focus-based routing.
5. On agentDoneMsg / carouselTickMsg / WindowSizeMsg: behaviour unchanged; viewport content and GotoBottom() unchanged. Layout (viewport height = height − input − footer) unchanged.

**Dependencies**

- internal/tui depends on bubbles/viewport and bubbles/textarea, and continues to use them as today. No new dependencies.

**Key data structures**

- **focusInput** (bool on Model): true = input has focus (default); false = viewport has scroll focus. Drives key routing in Update.
- **tea.KeyMsg**: When viewport focused, forwarded to viewport so bubbles viewport KeyMap can scroll (Up/Down, etc.). When input focused, handled as today (Enter submit, etc., rest to input).

## Out of scope (per task)

- Horizontal scrolling.
- Search / jump in chat.
- Styling or layout changes (beyond keeping layout fixed).
- Persisting scroll position.

## Changes for review

- **Modified**: `internal/tui/model.go` — Add `focusInput bool` to `Model` (default true). In `NewModel`, set `focusInput: true`. In `Update(tea.KeyMsg)`: handle Tab (or chosen key) to toggle `focusInput`; when `!focusInput` forward KeyMsg to viewport and return viewport cmd; when `focusInput` keep current key handling (Enter, Escape, KeyRunes→input). Optionally on toggle: `input.Blur()` / `input.Focus()`. Keep MouseMsg → viewport. Do not change View() or layout.
- **Modified**: `internal/tui/model_test.go` — Add test (e.g. `TestModelFocusToggle`): build minimal Model (or use a helper that creates Model with known opts); send `tea.KeyMsg` with Tab; assert `focusInput == false` (or equivalent); send Tab again; assert `focusInput == true`. Use a Model with exported focus field for testing, or a getter, or test via behaviour (e.g. send Tab then Up and assert viewport scroll position changed if possible). Prefer testing focus state directly if the field is exported or a getter is added for tests.
- **Unchanged**: Layout in `View()` — viewport, then input box, then footer; input and footer stay fixed. Viewport height = total − input height − footerLines. No changes to format.go or banner.go.
