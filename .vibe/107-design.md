# Design 107: TUI status line refine

## Goal

Split the TUI footer into two lines and show the logged-in user email on line 1.

## Modules

Two files changed, no new packages.

### 1. `internal/tui/model.go` — TUIOpts + renderFooterView

**TUIOpts change**: Add one field.

```go
type TUIOpts struct {
    // ... existing fields ...
    UserEmail string // logged-in user email; empty if not logged in
}
```

**renderFooterView change**: Build two lines and join with `\n`.

```go
func (m *Model) renderFooterView() string {
    // Line 1: model | workspace(branch) [| email]
    workspacePart := "@" + m.opts.Workspace
    if m.opts.Branch != "" {
        workspacePart += " (|-" + m.opts.Branch + ")"
    }
    line1 := footerModelStyle.Render("model: "+m.opts.ModelName) + " | " +
        footerBranchStyle.Render(workspacePart)
    if m.opts.UserEmail != "" {
        line1 += " | " + m.opts.UserEmail
    }

    // Line 2: keyboard shortcuts [| error]
    line2 := "ctrl+c: quit | esc: clear/focus input | opt+mouse: select text"
    if m.err != "" {
        line2 += " | error: " + m.err
    }

    return line1 + "\n" + line2
}
```

No other method in `model.go` changes. `syncViewportSize` already computes `footerHeight` via `lipgloss.Height(m.renderFooterView())`, which counts `\n`, so the viewport will automatically shrink by one row.

### 2. `internal/cmd/cli/tui.go` — pass UserEmail

Load credentials (best-effort) and extract email before building `TUIOpts`.

```go
func runTUI(resumeID string, modelSelector string) error {
    res, err := setupAgentAndSession(resumeID, modelSelector)
    if err != nil {
        return err
    }

    var userEmail string
    if creds, err := auth.Load(config.AuthPath()); err == nil && creds != nil {
        userEmail = creds.Email
    }

    opts := tui.TUIOpts{
        // ... existing fields ...
        UserEmail: userEmail,
    }
    // ... rest unchanged ...
}
```

Credential loading is best-effort: errors are silently ignored (the footer simply omits the email segment). `requireLogin` already ran before `runTUI`, so credentials will normally be present.

## How they work together

1. `runRoot` → `requireLogin` ensures credentials exist → `runTUI`.
2. `runTUI` loads credentials, extracts `Email`, passes it into `TUIOpts.UserEmail`.
3. `Model.renderFooterView()` produces two lines; `syncViewportSize` uses `lipgloss.Height` to correctly size the viewport.
4. `Model.View()` joins viewport, input box, and the two-line footer.

## Changes for review

| File | Change |
|------|--------|
| `internal/tui/model.go` | Add `UserEmail string` to `TUIOpts`; rewrite `renderFooterView` to two lines |
| `internal/cmd/cli/tui.go` | Add `auth` and `config` imports; load credentials; set `UserEmail` in opts |
