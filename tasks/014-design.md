# Design 014: Large TUI refactor

## Goal

Refactor the TUI so the root Bubble Tea model uses a pointer receiver everywhere and viewport vs input are separated into grouped state structs (Option A), improving maintainability without changing behavior or layout.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/tui** | Bubble Tea models and views for the chat TUI | Model (root), ViewportBlock, InputBlock, TUIOpts, message types, Init/Update/View and handlers |
| **internal/tui** (format, banner) | Viewport content building and message formatting | buildViewportContent, formatMessage, wrapLine, bannerWithVersion (unchanged) |
| **internal/app** | TUI program bootstrap | NewModel(opts) → tea.Model (unchanged signature; returns tui.NewModel which will be *Model) |

## Structure

**Directory / files**

- `internal/tui/` — TUI package
  - `model.go` — Root Model (pointer receiver), NewModel (returns *Model), Init, Update, View, FocusInput, message handlers (handleKeyMsg, handleWindowSize, etc.), tea.Msg types (agentDoneMsg, carouselTickMsg, scrollIdleMsg), constants (footerLines, carouselTick, scrollIdleDelay). Model holds ViewportBlock, InputBlock, opts, busy, err, width, height, carouselDots, focusInput, lastScrollID.
  - `viewport_block.go` — **New.** ViewportBlock struct (holds viewport.Model), RefreshAndGotoBottom(sess, version, width, busy, carouselDots), SetSize(width, height), Update(msg) tea.Cmd, View() string.
  - `input_block.go` — **New.** InputBlock struct (holds textarea.Model); constants inputMinLines, inputMaxLines; desiredInputHeight(value, width) using wrapLine from format.go; SyncHeight(), SetWidth(w), Focus(), Blur(), Reset(), Value() string, Height() int, Update(msg) tea.Cmd, View() string.
  - `format.go` — Unchanged. buildViewportContent, formatMessage, wrapLine, etc.
  - `banner.go` — Unchanged.
  - `model_test.go` — Updated: NewModel(opts) type-assert to *Model; chained Update calls use *Model.

- `internal/app/` — Unchanged layout.
  - `app.go` — NewModel still returns tea.Model; body remains `return tui.NewModel(opts)` (which will now be *Model).

**Main types and interfaces**

- **Model** (tui): Root Bubble Tea model. Fields: opts TUIOpts, viewportBlock ViewportBlock, inputBlock InputBlock, busy bool, err string, width int, height int, carouselDots int, focusInput bool, lastScrollID int. All methods use pointer receiver (*Model). Satisfies tea.Model.
- **ViewportBlock** (tui): Groups viewport state. Field: viewport viewport.Model. Methods: RefreshAndGotoBottom(sess, version, width, busy, carouselDots), SetSize(width, height), Update(msg) tea.Cmd, View() string.
- **InputBlock** (tui): Groups input state. Field: input textarea.Model. Methods: SyncHeight(), SetWidth(w), Focus(), Blur(), Reset(), Value() string, Height() int, Update(msg) tea.Cmd, View() string.
- **TUIOpts** (tui): Unchanged.

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|-----------------|
| *(Model) | (none) | `NewModel(opts TUIOpts) *Model` | Build ViewportBlock and InputBlock, set initial focus and size, call viewportBlock.RefreshAndGotoBottom(…), return &m. |
| *Model | Init | `() tea.Cmd` | Batch textarea.Blink and inputBlock.Focus(). |
| *Model | Update | `(msg tea.Msg) (tea.Model, tea.Cmd)` | Type-switch; call handleKeyMsg(m, msg), handleWindowSize(m, msg), etc. All handlers take *Model; return (m, cmd). |
| *Model | View | `() string` | Compute vpHeight; viewportBlock.SetSize(m.width, vpHeight); write viewportBlock.View(); write input box (inputBoxStyle + busy placeholder or inputBlock.View()); write footer from opts and m.err. |
| *Model | FocusInput | `() bool` | Return m.focusInput. |
| *Model | scheduleScrollIdleReturn | `() tea.Cmd` | Increment lastScrollID, return Tick(scrollIdleDelay, scrollIdleMsg{id}). |
| ViewportBlock | RefreshAndGotoBottom | `(sess *session.Session, version string, width int, busy bool, carouselDots int)` | content := buildViewportContent(…); m.viewport.SetContent(content); m.viewport.GotoBottom(). |
| ViewportBlock | SetSize | `(width, height int)` | Set m.viewport.Width, m.viewport.Height. |
| ViewportBlock | Update | `(msg tea.Msg) tea.Cmd` | m.viewport, cmd = m.viewport.Update(msg); return cmd. |
| ViewportBlock | View | `() string` | Return m.viewport.View(). |
| InputBlock | SyncHeight | `()` | want := desiredInputHeight(m.input.Value(), m.input.Width()); if want != m.input.Height() { m.input.SetHeight(want) }. |
| InputBlock | SetWidth | `(w int)` | m.input.SetWidth(w). |
| InputBlock | Focus / Blur / Reset | `()` | Delegate to m.input. |
| InputBlock | Value | `() string` | Return m.input.Value(). |
| InputBlock | Height | `() int` | Return m.input.Height(). |
| InputBlock | Update | `(msg tea.Msg) tea.Cmd` | m.input, cmd = m.input.Update(msg); return cmd. |
| InputBlock | View | `() string` | Return m.input.View(). |

Helpers (same package, take *Model where they mutate):

- handleKeyMsg(m *Model, msg tea.KeyMsg) (tea.Model, tea.Cmd): quit, Tab (toggle focusInput, inputBlock.Focus/Blur, scheduleScrollIdleReturn), viewport vs input key routing, Enter submit, Escape reset. Call m.viewportBlock.Update(msg) or m.inputBlock.Update(msg) and m.inputBlock.SyncHeight() as today; call viewportBlock.RefreshAndGotoBottom(m.opts.Session, m.opts.Version, m.width, busy, carouselDots) where refreshViewportAndGotoBottom is used today.
- handleWindowSize(m *Model, msg tea.WindowSizeMsg): set m.width, m.height; vpHeight := m.height - m.inputBlock.Height() - footerLines; m.viewportBlock.SetSize(m.width, vpHeight); m.inputBlock.SetWidth(m.width - 4); m.inputBlock.SyncHeight().
- handleAgentDone(m *Model, msg agentDoneMsg): set m.busy, m.carouselDots, m.err; on success call m.viewportBlock.RefreshAndGotoBottom(…), session.PersistAfterReply(…).
- handleCarouselTick(m *Model, msg carouselTickMsg): if m.busy, advance carouselDots, m.viewportBlock.RefreshAndGotoBottom(…), return Tick.
- handleMouseMsg(m *Model, msg tea.MouseMsg): wheel up/down → viewport scroll, focus to viewport, scheduleScrollIdleReturn; else m.viewportBlock.Update(msg).
- handleScrollIdle(m *Model, msg scrollIdleMsg): if msg.id == m.lastScrollID && !m.focusInput, set m.focusInput = true, m.inputBlock.Focus().

- **desiredInputHeight**: Move to `input_block.go` as a package-level helper. It uses `wrapLine` from format.go. Define `inputMinLines` and `inputMaxLines` in `input_block.go` (single source) so InputBlock.SyncHeight is self-contained. Model.go keeps `footerLines`, `carouselTick`, `scrollIdleDelay`; it no longer defines `inputMinLines`/`inputMaxLines` or `desiredInputHeight`.
- **buildViewportContent**: Stays in format.go. ViewportBlock.RefreshAndGotoBottom calls it.

## How they work together

**Data/control flow**

1. App: tea.NewProgram(tui.NewModel(opts)) — NewModel returns *Model; program runs with *Model.
2. Init: *Model.Init() runs inputBlock.Focus() and textarea.Blink.
3. Update: Bubble Tea passes msg to *Model.Update(msg). Model type-switches and calls handleKeyMsg(m, msg) (or other handler). Handlers mutate m (viewportBlock, inputBlock, focusInput, busy, etc.), call m.viewportBlock.RefreshAndGotoBottom(…), m.inputBlock.SyncHeight(), m.viewportBlock.Update(msg), m.inputBlock.Update(msg) as appropriate, and return (m, cmd).
4. View: *Model.View() calls viewportBlock.SetSize(width, vpHeight), viewportBlock.View(), then input box (inputBlock.View() or placeholder), then footer.
5. Tests: NewModel(opts) returns *Model; m2, _ := m.Update(msg); mod := m2.(*Model); mod.FocusInput().

**Dependencies**

- internal/tui (model.go) depends on internal/tui (viewport_block.go, input_block.go), internal/agent, internal/session, internal/llm, bubbles/textarea, bubbles/viewport, bubbletea, lipgloss.
- internal/tui (viewport_block.go) depends on internal/tui (format.go for buildViewportContent), bubbles/viewport, session.
- internal/tui (input_block.go) depends on internal/tui (format.go for wrapLine), bubbles/textarea.
- internal/app depends on internal/tui, bubbletea.

**Key data structures**

- Model: root state; holds ViewportBlock, InputBlock, opts, dimensions, focusInput, busy, err, carouselDots, lastScrollID. Created by NewModel; passed by pointer through Init/Update/View.
- ViewportBlock: holds viewport.Model. Created in NewModel; refreshed via RefreshAndGotoBottom(session, version, width, busy, carouselDots).
- InputBlock: holds textarea.Model. Created in NewModel; SyncHeight and Focus/Blur/Update delegated from Model handlers.

## Changes for review

- **New**: `internal/tui/viewport_block.go` — ViewportBlock struct; NewViewportBlock(width, height int) returns ViewportBlock (viewport.New, MouseWheelEnabled = true); RefreshAndGotoBottom(sess, version, width, busy, carouselDots), SetSize(width, height), Update(msg) tea.Cmd, View() string.
- **New**: `internal/tui/input_block.go` — InputBlock struct; constants inputMinLines, inputMaxLines; desiredInputHeight(value, width) (uses wrapLine from format); NewInputBlock() returns InputBlock with textarea configured (Prompt, Placeholder, SetHeight(inputMinLines), SetWidth(76), Focus()); SyncHeight(), SetWidth(w), Focus(), Blur(), Reset(), Value(), Height(), Update(msg) tea.Cmd, View() string.
- **Modified**: `internal/tui/model.go` — Model struct: replace viewport viewport.Model and input textarea.Model with viewportBlock ViewportBlock and inputBlock InputBlock. NewModel(opts) return *Model; allocate Model, set viewportBlock = NewViewportBlock(80, 20), inputBlock = NewInputBlock() (or NewInputBlock(76) for initial width), call m.viewportBlock.RefreshAndGotoBottom(m.opts.Session, m.opts.Version, m.width, false, 0), return &m. All methods on Model change to pointer receiver (m *Model). Remove refreshViewportAndGotoBottom (replaced by viewportBlock.RefreshAndGotoBottom). Remove syncInputHeight (replaced by inputBlock.SyncHeight). handleKeyMsg, handleWindowSize, handleAgentDone, handleCarouselTick, handleMouseMsg, handleScrollIdle: change to take (m *Model, msg); update to use m.viewportBlock and m.inputBlock (e.g. m.viewportBlock.Update(msg), m.inputBlock.SyncHeight(), m.viewportBlock.RefreshAndGotoBottom(...)). View(): use m.viewportBlock.SetSize, m.viewportBlock.View(), m.inputBlock.Height(), m.inputBlock.View(). desiredInputHeight: move to input_block.go (used only by InputBlock.SyncHeight). Init(): use m.inputBlock.Focus().
- **Modified**: `internal/tui/model_test.go` — Tests that call NewModel(opts) and type-assert Update result: use *Model (m2.(*Model)). Chained Update: mod and mod2 are *Model; no other test logic change.
- **Unchanged**: `internal/app/app.go` — Still `return tui.NewModel(opts)`; no signature change.
- **Unchanged**: `internal/tui/format.go`, `internal/tui/banner.go` — No changes.
