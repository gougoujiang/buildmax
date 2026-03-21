# Design 104 — TUI/CLI interactive mode login check

## Goal

Gate the root command's TUI and print-mode paths behind a credential check. TUI mode runs the interactive login flow when not authenticated; print mode shows an error and exits.

## Modules

| Module (package) | Responsibility | Change |
|-------------------|----------------|--------|
| **internal/cmd/cli** | Root command dispatch; login gate; interactive login | Modified: `root.go`, `login.go` |

No new packages, types, or interfaces. The existing `internal/auth` and `internal/config` packages are used as-is.

## Structure

**Files changed**

- `internal/cmd/cli/login.go` — Extract the interactive prompting logic from `runLogin` into a reusable `interactiveLogin() error` function. `runLogin` becomes a thin wrapper that calls `interactiveLogin`.
- `internal/cmd/cli/root.go` — Add `requireLogin(interactive bool) error` and call it in `runRoot` before dispatch.

No other files are changed. `setup.go`, `tui.go`, `print.go` remain untouched.

## Method design

### internal/cmd/cli/login.go

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| — | interactiveLogin | `() error` | Prompt for server URL, email, request OTP, prompt for OTP, call Login, save credentials. Extracted from current `runLogin` body — same logic, same prompts, same output. |

`runLogin` becomes:

```go
func runLogin(_ *cobra.Command, _ []string) error {
    return interactiveLogin()
}
```

### internal/cmd/cli/root.go

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| — | requireLogin | `(interactive bool) error` | Load credentials via `auth.Load(config.AuthPath())`. If valid, return nil. If not valid and `interactive` is true, call `interactiveLogin()` and return its result. If not valid and `interactive` is false, print `Not logged in. Run "buildmax login" first.` to stderr and return an error. |

### Insertion point in `runRoot`

The call is placed after flag/session-ID resolution, before dispatch. The `interactive` flag is derived from whether print mode is active:

```go
func runRoot(cmd *cobra.Command, _ []string) error {
    // ... version check (unchanged) ...
    // ... flag resolution, session-ID resolution (unchanged) ...

    // --- NEW: login gate ---
    if err := requireLogin(prompt == ""); err != nil {
        return err
    }

    if prompt != "" {
        return runPrintMode(...)
    }
    return runTUI(...)
}
```

When `prompt == ""` → TUI mode → `interactive = true` → runs login flow if needed.
When `prompt != ""` → print mode → `interactive = false` → error + exit if not logged in.

### Why subcommands are unaffected

`login`, `logout`, `whoami`, and `version` (subcommand form) are registered via `root.AddCommand(...)`. Cobra dispatches to their own `RunE`; `runRoot` is **not** called for subcommands. The `--version` flag (`-v`) is handled as an early return inside `runRoot` before the login gate.

## How they work together

### Flow — TUI mode, not logged in

```
User runs: buildmax
  │
  ├─ runRoot
  │   ├─ version check → no
  │   ├─ resolve flags, session ID
  │   ├─ requireLogin(interactive=true)
  │   │   ├─ auth.Load(config.AuthPath()) → nil (no file)
  │   │   ├─ not valid → interactive=true → call interactiveLogin()
  │   │   │   ├─ prompt: Server URL [http://localhost:5678]:
  │   │   │   ├─ prompt: Email:
  │   │   │   ├─ auth.NewClient(serverURL).RequestOTP(email, "login")
  │   │   │   ├─ prompt: OTP:
  │   │   │   ├─ auth.NewClient(serverURL).Login(email, otp)
  │   │   │   ├─ auth.Save(creds, config.AuthPath())
  │   │   │   └─ print "Logged in as <email> on <serverURL>"
  │   │   └─ return nil (login succeeded)
  │   └─ dispatch to runTUI (proceeds normally)
```

### Flow — TUI mode, already logged in

```
User runs: buildmax
  │
  ├─ runRoot
  │   ├─ version check → no
  │   ├─ resolve flags, session ID
  │   ├─ requireLogin(interactive=true)
  │   │   ├─ auth.Load(config.AuthPath()) → valid credentials
  │   │   └─ return nil
  │   └─ dispatch to runTUI (proceeds normally)
```

### Flow — print mode, not logged in

```
User runs: buildmax -p "hello"
  │
  ├─ runRoot
  │   ├─ version check → no
  │   ├─ resolve flags, session ID
  │   ├─ requireLogin(interactive=false)
  │   │   ├─ auth.Load(config.AuthPath()) → nil
  │   │   ├─ not valid → interactive=false
  │   │   ├─ fmt.Fprintln(os.Stderr, "Not logged in. Run \"buildmax login\" first.")
  │   │   └─ return fmt.Errorf("not logged in")
  │   └─ return err → non-zero exit
```

### Flow — subcommands (login, logout, whoami, version)

```
User runs: buildmax login     (or logout, whoami, version)
  │
  ├─ Cobra matches subcommand → calls subcommand's RunE directly
  ├─ runRoot is NOT called
  └─ No login check
```

## Dependencies

- `internal/cmd/cli` adds import `"buildmax/internal/auth"` to `root.go` (new) alongside existing `"buildmax/internal/config"`.
- No new external dependencies.

## Changes for review

- **Modified**: `internal/cmd/cli/login.go` — extract body of `runLogin` into `interactiveLogin() error`; `runLogin` becomes a one-line wrapper.
- **Modified**: `internal/cmd/cli/root.go` — add `import "buildmax/internal/auth"`; add `requireLogin(interactive bool) error`; call it in `runRoot` before dispatch.
