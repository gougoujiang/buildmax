# Design 036: User login

## Goal

Enable the BuildMax server to authenticate users via a login API (email lookup + JWT) and the portal to show a login page when unauthenticated, then the main app with user name and logout after login.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/store** | User persistence and DB lifecycle | User model, UserStore interface, MySQL connection, auto-migration |
| **internal/server** | HTTP API and route registration | Config (incl. login deps), login handler, OpenAPI spec |
| **internal/cmd** | Server bootstrap | Resolve DB + JWT from env, open store, pass to server |
| **portal (React)** | Login UX and auth state | Auth context, Login page, route guard, TopBar profile/logout |

## Structure

**Backend**

- `internal/store/`
  - `store.go` — User model (snake_case JSON), UserStore interface, Store struct, New and UserByEmail.
  - `mysql.go` (or in store.go) — Open MySQL from DSN, run GORM AutoMigrate for User.
- `internal/server/`
  - `server.go` — Config extended; New registers POST /api/login; openAPISpec includes /api/login.
  - `login.go` — loginHandler: parse JSON body, call UserStore.UserByEmail, issue JWT, write response or 401.
- `internal/config/`
  - Optional: add MySQL DSN builder and JWT secret from env (or keep env reads in cmd and store).
- `internal/cmd/`
  - `server.go` — runServer: build MySQL DSN from env, open store, build server.Config with Store + JWTSecret, call server.New and Run.

**Portal**

- `portal/src/`
  - `contexts/AuthContext.tsx` (or `lib/auth.tsx`) — React context: { user, token, login, logout }; persist token (and optionally user) in localStorage; load on init.
  - `pages/LoginPage.tsx` — Form: email, password; submit → POST apiBase/api/login with { email }; on success call login(token, user), on error show message.
  - `App.tsx` — Wrap with AuthProvider; if !token render LoginPage else existing workspace/app shell and routes.
  - `components/TopBar.tsx` — Accept user + onLogout; show user.name and Logout button; on click call logout().
  - `lib/api.ts` (or in data/) — getApiBase(): import.meta.env.VITE_API_BASE || 'http://localhost:5678'; login(email): fetch POST getApiBase() + '/api/login', body JSON { email }, return { token, user } or throw.

**Main types and interfaces**

- **User** (internal/store): id (string/UUID), email, name, created_at; struct tags for GORM and `json:"snake_case"` for API responses.
- **UserStore** (internal/store): interface with `UserByEmail(ctx context.Context, email string) (*User, error)`.
- **Store** (internal/store): implements UserStore; holds *gorm.DB; constructor `New(ctx, dsn string) (*Store, error)` opens DB and runs AutoMigrate(&User{}).
- **server.Config**: Add `UserStore store.UserStore` and `JWTSecret string`; Addr unchanged. If UserStore is nil, login handler returns 503 or registration is skipped (per task we always pass store when running server).

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|------------|-----------------|
| **Store** | UserByEmail | `(ctx context.Context, email string) (*User, error)` | Look up user by email; return nil, nil when not found. |
| **Store** | Close | `() error` | Close DB connection (optional; server runs until exit). |
| **server** | (handler) | `loginHandler(w, r)` | Parse POST body `{ "email": "..." }`; UserStore.UserByEmail; if nil return 401 JSON; else sign JWT (sub=id, exp=24h), return 200 `{ "token", "user": { id, email, name } }`. |
| **config or cmd** | (pkg func) | `MySQLDSN() string` or build in cmd | Build DSN from MYSQL_HOST, MYSQL_PORT, MYSQL_USER, MYSQL_PASSWORD, MYSQL_DATABASE (defaults: localhost, 3306, buildmax, buildmax, buildmax). |
| **AuthContext** | login | `(token: string, user: UserInfo) => void` | Store token and user in state and localStorage. |
| **AuthContext** | logout | `() => void` | Clear state and localStorage. |
| **api** | login | `(email: string) => Promise<{ token, user }>` | POST apiBase + '/api/login' with { email }; throw on non-2xx. |

## How they work together

**Login flow**

1. User submits email (and password, not used by backend yet) on portal Login page.
2. Portal calls `api.login(email)` → POST /api/login with JSON body.
3. server loginHandler reads body, calls `cfg.UserStore.UserByEmail(ctx, email)`.
4. If no user: respond 401 with JSON error message. If user found: create JWT (claims: sub=user.ID, exp=now+24h), sign with cfg.JWTSecret; respond 200 with `{ "token": "<jwt>", "user": { "id", "email", "name" } }`.
5. Portal stores token and user in AuthContext and localStorage, then renders main app.
6. TopBar reads user from context, shows name and Logout; logout() clears auth state and localStorage, so App re-renders login page.

**Startup (server)**

1. cmd runServer: resolve port (existing), read MYSQL_* env and BUILDMAX_JWT_SECRET, build DSN.
2. store.New(ctx, dsn) opens MySQL, AutoMigrate(&User{}), return Store.
3. server.New(server.Config{ Addr, UserStore: store, JWTSecret }) registers GET /healthz, GET /openapi.json, GET /swagger/*, POST /api/login.
4. s.Run() blocks until shutdown.

**Dependencies**

- internal/server depends on internal/store (UserStore interface and User type for response).
- internal/cmd depends on internal/server and internal/store; cmd builds DSN and opens store.
- internal/store depends only on standard library + GORM (and MySQL driver); no dependency on server or cmd.

**Key data structures**

- **Login request body**: `{ "email": "string" }` — handler decodes into a small struct.
- **Login response**: `{ "token": "string", "user": { "id": "string", "email": "string", "name": "string" } }` — user fields snake_case in JSON per AGENTS.md (or camelCase for portal; task says minimal user info; AGENTS.md snake_case is for “persisted data” — API response can be camelCase for JS. Task says “user: { id, email, name }” — we’ll use snake_case for API consistency with other persisted-style responses).
- **JWT claims**: sub (user id), exp, iat; signing algorithm HS256.

## OpenAPI

Add to server’s openAPISpec paths:

- `POST /api/login`: requestBody application/json `{ "email": "string" }`; responses 200 (description + schema token + user), 401 (description).

## Testing

- **internal/server**: Test loginHandler via server.Handler(): use a mock UserStore (in-memory or interface mock). Case 1: user exists → 200, body contains token and user. Case 2: user not found → 401. Case 3: invalid body → 400.
- **internal/store**: Integration test optional: with test DB (or sqlite in-memory if GORM supports and we abstract driver), ensure UserByEmail returns user when present and nil when absent. If test DB not available, at least table-driven unit test with mock DB is acceptable per task (“with test DB or mock”).

## Changes for review

- **New**: `internal/store/store.go` — User struct, UserStore interface, Store, New(dsn), UserByEmail, AutoMigrate.
- **New**: `internal/store/` — MySQL/GORM dependency; go.mod: add github.com/go-sql-driver/mysql (or use GORM’s recommended driver), gorm.io/gorm, gorm.io/driver/mysql.
- **New**: `internal/server/login.go` — loginHandler; request/response structs; JWT signing (e.g. github.com/golang-jwt/jwt/v5).
- **Modified**: `internal/server/server.go` — Config add UserStore and JWTSecret; New() registers POST /api/login; openAPISpec extended with /api/login.
- **Modified**: `internal/cmd/server.go` — runServer builds MySQL DSN from env, opens store, passes UserStore and JWTSecret into server.Config.
- **New**: `portal/src/lib/api.ts` — getApiBase(), login(email).
- **New**: `portal/src/contexts/AuthContext.tsx` (or `lib/auth.tsx`) — AuthProvider, useAuth(), login/logout, localStorage persistence.
- **New**: `portal/src/pages/LoginPage.tsx` — Form and submit to api.login.
- **Modified**: `portal/src/App.tsx` — AuthProvider wrapper; conditional render LoginPage vs existing app.
- **Modified**: `portal/src/components/TopBar.tsx` — Props: user, onLogout; show user name and Logout.
- **New**: `internal/server/login_test.go` (or server_test.go) — Tests for login handler (mock UserStore, 200/401/400).
