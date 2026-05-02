# Design 105 — Desktop mandatory login check

## Goal

Add a Go-side credential check to the desktop app's agent-running bindings (`SendMessage`, `SendMessageStream`) so they refuse to run when the user is not authenticated. Mirrors the pattern from task 104 (TUI/CLI `requireLogin`).

## Modules

| Module (package) | Responsibility | Change |
|-------------------|----------------|--------|
| **internal/interface/desktop** | Desktop Wails bindings | Modified: `app.go` |

No new packages, types, or interfaces. Uses existing `internal/auth` and `internal/config` as-is.

## Structure

**Files changed**

- `internal/interface/desktop/app.go` — Add a private `requireLogin() error` method on `App`. Insert a call to it at the top of `SendMessage` and `SendMessageStream`, before any other logic.

No other files are changed.

## Method design

### internal/interface/desktop/app.go

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| `*App` | `requireLogin` | `() error` | Load credentials via `auth.Load(config.AuthPath())`. If `creds.IsValid()` returns true, return nil. Otherwise return `fmt.Errorf("not logged in")`. |

This is a private method — not exposed to the frontend. It is the single place where agent-path auth is enforced.

### Insertion points

**SendMessage** — Add the check as the first operation, before the prompt-empty check:

```go
func (a *App) SendMessage(sessionID string, prompt string) (SendMessageResult, error) {
    if err := a.requireLogin(); err != nil {
        return SendMessageResult{}, err
    }
    if prompt == "" {
        // ... existing code unchanged ...
    }
    // ... rest unchanged ...
}
```

**SendMessageStream** — Same pattern:

```go
func (a *App) SendMessageStream(sessionID string, prompt string) error {
    if err := a.requireLogin(); err != nil {
        return err
    }
    if prompt == "" {
        // ... existing code unchanged ...
    }
    // ... rest unchanged ...
}
```

### Methods NOT changed

| Method | Reason |
|--------|--------|
| `GetAuthStatus` | Auth query — must work unauthenticated |
| `RequestOTP` | Auth flow step — must work unauthenticated |
| `DoLogin` | Auth flow step — must work unauthenticated |
| `Logout` | Auth flow step — must work unauthenticated |
| `ListSessions` | Local disk read, no LLM call |
| `GetSession` | Local disk read, no LLM call |
| `Startup` / `Shutdown` | Wails lifecycle hooks |

## How they work together

### Flow — SendMessage, not logged in

```
Frontend calls: app.SendMessage("", "hello")
  │
  ├─ requireLogin()
  │   ├─ auth.Load(config.AuthPath()) → nil (no file)
  │   ├─ creds.IsValid() → false
  │   └─ return fmt.Errorf("not logged in")
  │
  └─ return SendMessageResult{}, err
      → Frontend receives error "not logged in"
```

### Flow — SendMessageStream, not logged in

```
Frontend calls: app.SendMessageStream("", "hello")
  │
  ├─ requireLogin()
  │   ├─ auth.Load(config.AuthPath()) → nil
  │   └─ return fmt.Errorf("not logged in")
  │
  └─ return err
      → Frontend receives error, no goroutine spawned
```

### Flow — SendMessage, logged in

```
Frontend calls: app.SendMessage("", "hello")
  │
  ├─ requireLogin()
  │   ├─ auth.Load(config.AuthPath()) → valid credentials
  │   └─ return nil
  │
  ├─ prompt check → ok
  ├─ agentrun.Open → runtime
  ├─ rt.RunPrompt → out
  └─ return SendMessageResult{Reply, SessionID}
```

## Dependencies

- No new imports. `auth` and `config` are already imported in `app.go`.
- No new external dependencies.

## Changes for review

- **Modified**: `internal/interface/desktop/app.go` — add `requireLogin() error` method on `*App`; call it at the top of `SendMessage` and `SendMessageStream`.
