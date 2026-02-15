# App

## Purpose

The `internal/app` package is a thin bootstrap layer for the TUI. It provides `NewModel(opts tui.TUIOpts) tea.Model`, which returns the root Bubble Tea model used when running in TUI mode. All real logic lives in `internal/tui`; app only wires the TUI options into the model.

## Key Types

| Name | Kind | Role |
|------|------|------|
| **NewModel** | func | `(opts tui.TUIOpts) tea.Model` — builds the root TUI model |

## Dependencies

- **Uses**: `internal/tui`, `github.com/charmbracelet/bubbletea`
- **Used by**: `internal/cmd` (TUI runner calls `app.NewModel`)

## Notes

- See [TUI](tui.md) for the actual model, viewport, and input handling.
