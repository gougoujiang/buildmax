# Design 103 - Login from desktop and TUI

## Goal

Add a shared Go auth package and wire login/logout into the CLI (Cobra subcommands) and desktop app (Wails bindings + frontend login screen) so users can authenticate against the BuildMax server from any client.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/auth** | Credential persistence and HTTP auth client. No UI or CLI dependency. | `Credentials`, `Client`, `LoginResponse`, `LoginUser` |
| **internal/config** | Path helper for auth file. | `AuthPath()` |
| **internal/cmd/cli** | Cobra subcommands for login, logout, whoami. | `login.go` (3 subcommands) |
| **internal/cmd/desktop** | Wails bindings for auth flow. | Auth methods on `App` struct |
| **desktop/frontend/src** | Login page UI in the Wails app. | `LoginPage.jsx` |

## Structure

**Directory / files**

- `internal/auth/` — new package
  - `credentials.go` — `Credentials` struct, `Save`, `Load`, `Clear`, `IsValid`
  - `client.go` — `Client` struct, `RequestOTP`, `Login`
- `internal/config/`
  - `config.go` — add `AuthPath()` (one function)
- `internal/cmd/cli/`
  - `login.go` — new file: `newLoginCommand()`, `newLogoutCommand()`, `newWhoamiCommand()`
  - `root.go` — modified: add three subcommands
- `internal/cmd/desktop/`
  - `app.go` — modified: add `RequestOTP`, `Login`, `Logout`, `GetAuthStatus` methods
- `desktop/frontend/src/`
  - `LoginPage.jsx` — new file: two-step login form
  - `App.jsx` — modified: check auth on startup, conditional render

**Main types and interfaces**

- **Credentials** (auth): Persisted auth state — server URL, token, user info, saved timestamp. JSON file on disk.
- **Client** (auth): Stateless HTTP client for `/api/otp/request` and `/api/login`. Takes base URL, returns typed responses.
- **LoginResponse** (auth): Token + user from login endpoint. Mirrors the server's response shape.
- **LoginUser** (auth): User subset (id, email, name). Shared between client response and credentials.
- **AuthStatus** (desktop): Frontend-facing struct with `LoggedIn`, `ServerURL`, `User` fields.

## Method design

### internal/auth

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| — | Save | `(creds *Credentials, path string) error` | Marshal credentials to JSON, write to path (create dir if needed) |
| — | Load | `(path string) (*Credentials, error)` | Read and unmarshal; return nil credentials (not error) if file doesn't exist |
| — | Clear | `(path string) error` | Remove the auth file; no error if already absent |
| `*Credentials` | IsValid | `() bool` | True when Token is non-empty |
| — | NewClient | `(baseURL string) *Client` | Create client with server base URL |
| `*Client` | RequestOTP | `(ctx context.Context, email, intent string) error` | POST /api/otp/request; return error with server message on non-2xx |
| `*Client` | Login | `(ctx context.Context, email, otp string) (*LoginResponse, error)` | POST /api/login; return token + user on 200, error with message otherwise |

### internal/config

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| — | AuthPath | `() string` | Return `filepath.Join(DataDir(), "auth.json")` |

### internal/cmd/cli (login.go)

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| — | newLoginCommand | `() *cobra.Command` | Cobra `login` subcommand; interactive OTP flow, saves credentials |
| — | newLogoutCommand | `() *cobra.Command` | Cobra `logout` subcommand; clears credentials |
| — | newWhoamiCommand | `() *cobra.Command` | Cobra `whoami` subcommand; prints user info or "not logged in" |

### internal/cmd/desktop (app.go — new methods)

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| `*App` | RequestOTP | `(serverURL, email, intent string) error` | Create auth.Client, call RequestOTP |
| `*App` | Login | `(serverURL, email, otp string) (*AuthStatus, error)` | Call auth.Client.Login, save credentials, return status |
| `*App` | Logout | `() error` | Clear credentials |
| `*App` | GetAuthStatus | `() (*AuthStatus, error)` | Load credentials, return current auth state |

## How they work together

### Data/control flow — CLI login

```
User runs: buildmax login
  │
  ├─ 1. Cobra RunE → prompt server URL (default from env or "http://localhost:5678")
  ├─ 2. Prompt email
  ├─ 3. auth.NewClient(serverURL).RequestOTP(ctx, email, "login")
  │     └─ POST /api/otp/request {email, intent:"login"}
  │     └─ On error: print message, exit
  ├─ 4. Prompt OTP
  ├─ 5. auth.NewClient(serverURL).Login(ctx, email, otp)
  │     └─ POST /api/login {email, otp}
  │     └─ Returns LoginResponse{Token, User{ID, Email, Name}}
  ├─ 6. Build Credentials{ServerURL, Token, UserID, Email, Name, SavedAt}
  ├─ 7. auth.Save(&creds, config.AuthPath())
  │     └─ Writes ~/.buildmax/auth.json
  └─ 8. Print "Logged in as <email> on <serverURL>"
```

### Data/control flow — CLI logout / whoami

```
buildmax logout
  ├─ auth.Clear(config.AuthPath())
  └─ Print "Logged out"

buildmax whoami
  ├─ auth.Load(config.AuthPath())
  ├─ If nil or !IsValid → print "Not logged in"
  └─ Else → print "Logged in as <email> on <serverURL>"
```

### Data/control flow — Desktop login

```
App startup
  │
  ├─ 1. Frontend calls GetAuthStatus() via Wails binding
  │     └─ Go: auth.Load(config.AuthPath()) → return AuthStatus
  ├─ 2. If !LoggedIn → render LoginPage
  │
  └─ LoginPage (two steps):
       ├─ Step 1: User enters server URL + email → calls RequestOTP(serverURL, email, "login")
       │    └─ Go: auth.NewClient(serverURL).RequestOTP(ctx, email, "login")
       │
       ├─ Step 2: User enters OTP → calls Login(serverURL, email, otp)
       │    └─ Go: auth.NewClient(serverURL).Login(ctx, email, otp)
       │    └─ Go: auth.Save(creds, config.AuthPath())
       │    └─ Returns AuthStatus{LoggedIn:true, ServerURL, User}
       │
       └─ On success → frontend sets state → re-render shows chat UI
```

### Data/control flow — Desktop logout

```
User clicks logout in sidebar
  ├─ Frontend calls Logout() via Wails binding
  │     └─ Go: auth.Clear(config.AuthPath())
  └─ Frontend sets state → re-render shows LoginPage
```

### Dependencies

- `internal/auth` depends on: `net/http`, `encoding/json`, `os`, `path/filepath`, `context` — **no project imports**.
- `internal/config` depends on: `os`, `path/filepath` — no new dependency added.
- `internal/cmd/cli` depends on: `internal/auth`, `internal/config`, `github.com/spf13/cobra`.
- `internal/cmd/desktop` depends on: `internal/auth`, `internal/config` (adds auth; existing deps unchanged).
- Desktop frontend depends on: `@buildmax/gui` (existing), Wails runtime (existing). No new npm dependencies.

### Key data structures

**Credentials** (persisted as `auth.json`):

```json
{
  "server_url": "http://localhost:5678",
  "token": "<JWT>",
  "user_id": "u_abc123...",
  "email": "user@example.com",
  "name": "",
  "saved_at": 1711036800
}
```

Created by CLI `login` command or desktop `Login` binding. Consumed by `whoami`, `GetAuthStatus`, and future auth-gated features.

**LoginResponse** (in-memory, from server):

```go
type LoginResponse struct {
    Token string    `json:"token"`
    User  LoginUser `json:"user"`
}

type LoginUser struct {
    ID    string `json:"id"`
    Email string `json:"email"`
    Name  string `json:"name"`
}
```

Mirrors the server's `POST /api/login` response exactly (same field names, same JSON shape).

**AuthStatus** (desktop binding → frontend):

```go
type AuthStatus struct {
    LoggedIn  bool   `json:"logged_in"`
    ServerURL string `json:"server_url,omitempty"`
    UserID    string `json:"user_id,omitempty"`
    Email     string `json:"email,omitempty"`
    Name      string `json:"name,omitempty"`
}
```

Returned by `GetAuthStatus()` and `Login()`. The frontend uses `LoggedIn` to decide whether to show login screen or chat.

### Desktop frontend LoginPage design

The desktop login page does **not** use `FormModal` (which only supports `text`/`textarea` and is modal-style). Instead it uses a standalone page layout matching the portal's `login-page` design:

- Full-page centered card with BuildMax branding
- Two steps: email → OTP (same UX as portal)
- Calls Go bindings: `window.go.desktop.App.RequestOTP(...)` and `window.go.desktop.App.Login(...)`
- Uses CSS classes from `@buildmax/gui` theme (`theme.css` variables) plus local login-page styles
- Server URL input has a sensible default (`http://localhost:5678`); shown in a collapsible "Advanced" section so it doesn't clutter the common case

### Desktop App.jsx integration

The existing `App.jsx` gains:

1. An `authStatus` state, initialized to `null` (loading)
2. A `useEffect` on mount that calls `GetAuthStatus()` and sets the state
3. Conditional rendering: if `authStatus === null` → loading spinner; if `!authStatus.logged_in` → `<LoginPage onLogin={...} />`; else → existing chat UI
4. A logout button in the sidebar (below theme toggle) that calls `Logout()` and resets `authStatus`

## Changes for review

- **New**: `internal/auth/credentials.go` — `Credentials` struct, `Save`, `Load`, `Clear`, `IsValid`
- **New**: `internal/auth/client.go` — `Client`, `NewClient`, `RequestOTP`, `Login`, `LoginResponse`, `LoginUser`
- **New**: `internal/auth/auth_test.go` — unit tests for credentials and client (httptest)
- **New**: `internal/cmd/cli/login.go` — `newLoginCommand`, `newLogoutCommand`, `newWhoamiCommand`
- **New**: `desktop/frontend/src/LoginPage.jsx` — two-step login page component
- **Modified**: `internal/config/config.go` — add `AuthPath()` function
- **Modified**: `internal/cmd/cli/root.go` — register login, logout, whoami subcommands
- **Modified**: `internal/cmd/desktop/app.go` — add `AuthStatus` type, `RequestOTP`, `Login`, `Logout`, `GetAuthStatus` methods
- **Modified**: `desktop/frontend/src/App.jsx` — auth check on startup, conditional render, logout button
