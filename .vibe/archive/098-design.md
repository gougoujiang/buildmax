# Design 098 — TUI chat history optimization

## Goal

Fix viewport content so that (1) wrapping uses visible content width (no ANSI in wrapped strings), and (2) left/right margins are applied consistently so chat text does not touch the terminal edges.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/tui** | Viewport content building and formatting. | format.go: margin constants, `indentLines`, refactored `buildViewportContent`; banner.go: no change (indent applied in buildViewportContent). |

No new packages. Callers (model.go) continue to pass `ViewportContentOpts{ Width: m.width }`; no API change.

## Structure

**Files**

- **internal/tui/format.go**
  - Add constants: `viewportLeftMargin`, `viewportRightMargin` (both 2).
  - Add helper: `indentLines(s string, spaces int) string` — split `s` by `\n`, prefix each line with `spaces` spaces, rejoin with `\n`. Used for banner and any multi-line block.
  - Refactor `buildViewportContent`: compute content width once; wrap plain text only; output margin + prefix per segment; apply indent to banner.
  - `wrapLine` remains unchanged (still operates on plain runes).
- **internal/tui/banner.go**
  - No change. Banner string is produced as today; margin is applied in `buildViewportContent` via `indentLines(bannerWithVersion(...), viewportLeftMargin)`.
- **internal/tui/model_test.go** (or new **internal/tui/format_test.go**)
  - Add tests for margin and wrap-by-content-width (see Tests section).

## Method / helper design

| Name | Signature | Responsibility |
|------|-----------|----------------|
| **indentLines** | `(s string, spaces int) string` | Split `s` by `\n`; prefix each line with `spaces` spaces (using `strings.Repeat`); rejoin with `\n`. Return empty string if `spaces <= 0`. Used for banner so every line gets left margin. |
| **buildViewportContent** | (existing) `(sess *session.Session, opts ViewportContentOpts) string` | See "Content build algorithm" below. |

**Constants**

- `viewportLeftMargin = 2`
- `viewportRightMargin = 2`
- `prefixWidth = 2` (logical width of `"• "` or `"> "`)

**Content width**

- `contentWidth = opts.Width - viewportLeftMargin - viewportRightMargin - prefixWidth`
- If `contentWidth <= 0`, clamp to 1 so `wrapLine` still receives a positive width.

## Content build algorithm

**Setup**

- `width := opts.Width`; if `width <= 0` then `width = 80`.
- `contentWidth := width - viewportLeftMargin - viewportRightMargin - prefixWidth`; if `contentWidth < 1` then `contentWidth = 1`.
- `marginStr := strings.Repeat(" ", viewportLeftMargin)` (reuse for every line).

**Banner**

- Output `"\n\n"` (unchanged).
- Output `indentLines(bannerWithVersion(opts.Version), viewportLeftMargin)` (so every banner line, including version, has left margin).
- Output `"\n"`.

**Messages loop** (per message, per logical line from `formatMessage(m)`)

- **User**: Wrap plain `line` with `wrapLine(line, contentWidth)`. For each segment `i`: write `marginStr`; if `i == 0` write `messageBarStyle.Render("> ") + userMessageStyle.Render(segment)`; else write two spaces + `userMessageStyle.Render(segment)`. Then write `"\n"`.
- **Assistant**: Wrap plain `line` with `wrapLine(line, contentWidth)`. For each segment `i`: write `marginStr`; if `i == 0` write `messageBarStyle.Render("• ") + segment`; else write two spaces + segment. Then write `"\n"`.
- **Default (e.g. tool)**: Wrap plain `line` with `wrapLine(line, contentWidth)`. For each segment: write `marginStr`, then `"  "`, then segment, then `"\n"`.

**Streaming tail**

- Wrap plain `opts.StreamingTail` with `wrapLine(opts.StreamingTail, contentWidth)`. For each segment `i`: write `marginStr`; if `i == 0` write `messageBarStyle.Render("• ") + segment`; else write two spaces + segment. Then write `"\n"`.

**Carousel** (when Busy and no StreamingTail)

- Single line: write `marginStr`, then `messageBarStyle.Render("• ")`, then `dots[idx]`, then `"\n"`. No wrap needed.

**Inter-message spacing**

- Unchanged: `"\n"` between messages; `"\n"` before first message block after banner; `"\n"` before streaming tail or carousel when there are messages.

## How they work together

1. **Model** passes terminal `m.width` in `ViewportContentOpts.Width` (unchanged). No change in model.go or viewport_block.go.
2. **buildViewportContent** is the single place that builds the viewport string. It now:
   - Computes `contentWidth` from `opts.Width` and margin/prefix constants.
   - Wraps only plain strings with `wrapLine(plainContent, contentWidth)`.
   - Applies style only after wrapping (prefix on first segment, continuation with two spaces).
   - Prefixes every line with `marginStr` and uses banner via `indentLines` so the whole scrollable area has consistent left margin and effective right margin (content never exceeds `contentWidth` per line).

**Dependencies**

- No new imports. format.go continues to use `strings`, `session`, `llm`, `lipgloss`; banner.go unchanged.

## Tests

- **Existing**: `TestBuildViewportContent`, `TestBuildViewportContent_BusyCarousel`, `TestWrapLine` — keep passing. Adjust expectations only if output format changes (e.g. presence of leading spaces); prefer adding new assertions rather than removing old ones.
- **New (in model_test.go or format_test.go)**:
  1. **Margin**: With `Width: 80`, assert that the first line of the first message (or banner) starts with two spaces (or that every line in a fixed snippet contains the margin). E.g. `buildViewportContent(sess, ViewportContentOpts{Width: 80})` and check that after the banner there is a line starting with `"  "` before `">"` or `"•"`.
  2. **Wrap by content width**: Use a long assistant line (e.g. 100 runes of plain ASCII). Call `buildViewportContent` with `Width: 80`. Count runes per line (strip ANSI for counting, or assert that the first wrapped content line has length ≤ contentWidth + 2 + 2 = 74 + 4 = 78 visible/content runes). Simpler: assert that with width 80 and margins 2+2, the first message line (after margin + "• ") has at least 70 runes of content before the first newline (proving we no longer wrap at ~60 due to ANSI).
  3. **Optional**: Test that `wrapLine` is never called with a string containing `\x1b` (ANSI) in the refactored code path (e.g. by inspecting call sites or a small test that builds content and checks no wrapped segment contains ANSI from prefix).

## Changes for review

| Item | Location | Change |
|------|----------|--------|
| Constants | format.go | Add `viewportLeftMargin`, `viewportRightMargin` (2 each). |
| Helper | format.go | Add `indentLines(s string, spaces int) string`. |
| buildViewportContent | format.go | Compute `contentWidth` and `marginStr`; use `indentLines` for banner; for each message/streaming/carousel line: wrap plain text with `wrapLine(..., contentWidth)`; output margin + styled prefix (first segment only) or 2 spaces (continuation) + segment per line. |
| Tests | model_test.go or format_test.go | Add tests: margin present; long line wraps to ~contentWidth per line (no early ANSI wrap). |
