# Design 106 - Token expiry check and login metadata

## Goal

Add client-side JWT expiry detection to `Credentials.IsValid()` and record last login time and platform in the `user` table on each successful login.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/auth** | Client-side credential persistence and auth HTTP client | `Credentials`, `IsValid`, `Save`, `Load`, `Clear`, `Client` (Login, RequestOTP) |
| **internal/infra/db** | User model and UserStore interface + GORM impl | `User` struct, `UserStore` interface, `Store.UpdateLoginMeta` |
| **internal/server/auth** | Unauthenticated auth endpoints (login, OTP) | `loginHandler`, `LoginRequest`, `Config` |
| **internal/testutil** | Mock stores for tests | `MockUserStore` |
| **internal/interface/cli** | CLI login flow | `interactiveLogin` |
| **internal/interface/desktop** | Desktop login flow | `App.DoLogin` |
| **portal/src/features/auth** | Portal login API call | `login()` function |

## Structure

**Directory / files**

- `internal/auth/`
  - `credentials.go` — add `extractJWTExp` helper; update `IsValid` to check exp
- `internal/infra/db/`
  - `models.go` — add `LastLoginAt`, `LastLoginPlatform` fields to `User`
  - `interfaces.go` — add `UpdateLoginMeta` to `UserStore`
  - `user.go` — implement `Store.UpdateLoginMeta`
- `internal/server/auth/`
  - `login.go` — add `Platform` to `LoginRequest`; call `UpdateLoginMeta` after success
- `internal/testutil/`
  - `helpers.go` — add `UpdateLoginMeta` to `MockUserStore`
- `internal/interface/cli/`
  - `login.go` — pass `"cli"` platform to `client.Login`
- `internal/interface/desktop/`
  - `app.go` — pass `"desktop"` platform to `client.Login`
- `portal/src/features/auth/`
  - `api.ts` — add `platform: "portal"` to login request body

**Main types and interfaces**

- **Credentials** (auth): Add expiry-aware `IsValid()` that parses JWT `exp` from the base64 payload. No new fields; reads from existing `Token`.
- **User** (entity): Two new optional fields: `LastLoginAt *int64`, `LastLoginPlatform *string`.
- **UserStore** (entity): New method `UpdateLoginMeta(ctx, userID, loginAt, platform) error`.
- **LoginRequest** (server/auth): New `Platform string` field.

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| — | extractJWTExp | `(tokenStr string) (int64, error)` | Base64-decode JWT payload segment, extract `exp` claim as unix timestamp |
| *Credentials | IsValid | `() bool` | Return false if token is empty, malformed, or `exp` is in the past |
| *Client | Login | `(ctx context.Context, email, otp, platform string) (*LoginResponse, error)` | POST `/api/login` with email, otp, and platform |
| *Store | UpdateLoginMeta | `(ctx context.Context, userID string, loginAt int64, platform string) error` | GORM update of `last_login_at` and `last_login_platform` where `user_id = ?` |
| *Handler | loginHandler | `(w, r)` | (modified) After JWT generation, call `UpdateLoginMeta` with `req.Platform` |
| *MockUserStore | UpdateLoginMeta | `(ctx, userID, loginAt, platform) error` | No-op / record call for assertions |

## How they work together

**Data/control flow — login with metadata**

1. Client (CLI/desktop/portal) sends `POST /api/login` with `{"email":"…","otp":"…","platform":"cli"|"desktop"|"portal"}`.
2. `loginHandler` validates OTP, looks up user, generates JWT (unchanged).
3. `loginHandler` calls `UserStore.UpdateLoginMeta(ctx, user.UserID, now.Unix(), req.Platform)` — best effort; log error but don't fail login.
4. `loginHandler` returns `{token, user}` as before.
5. CLI/desktop save credentials via `auth.Save()` (unchanged).

**Data/control flow — expiry check**

1. CLI/desktop call `auth.Load()` to get `Credentials`.
2. Call `creds.IsValid()` which now parses the JWT payload for `exp`.
3. If expired, caller treats it as not logged in and prompts for re-login.

**extractJWTExp implementation detail**

A JWT is three base64url segments separated by `.`. The middle segment is the payload JSON. We:
1. Split token on `.` — require exactly 3 parts.
2. Base64url-decode part[1] (with padding normalization).
3. Unmarshal into `struct{ Exp *float64 "json:\"exp\"" }`.
4. If `exp` is nil or <= 0, return error.
5. Return `int64(*exp)`.

No external dependency needed — only `encoding/base64`, `encoding/json`, `strings`, `time`.

**Dependencies**

- `internal/auth` — no new dependencies (stdlib only).
- `internal/server/auth` — already depends on `entity.UserStore`; no new dependency.
- `internal/infra/db` — no new dependency.
- `internal/interface/cli` and `internal/interface/desktop` — already depend on `internal/auth`; no new dependency.

**Key data structures**

- `LoginRequest` (server/auth): gains `Platform` field; created by JSON decode from client request, consumed by `loginHandler`.
- `User` (entity): gains `LastLoginAt` and `LastLoginPlatform`; written by `UpdateLoginMeta`, readable by any user query.

## Changes for review

- **Modified**: `internal/auth/credentials.go` — add `extractJWTExp` func; update `IsValid` to check token expiry
- **Modified**: `internal/auth/client.go` — add `platform` parameter to `Login` method
- **Modified**: `internal/auth/auth_test.go` — update `TestIsValid` with real JWT tokens (expired, valid, malformed); update `TestClientLogin` for new `platform` param
- **Modified**: `internal/core/model/db_entities.go` — add `LastLoginAt`, `LastLoginPlatform` to `User`
- **Modified**: `internal/core/model/db_repositories.go` — add `UpdateLoginMeta` to `UserStore`
- **Modified**: `internal/infra/db/user.go` — implement `Store.UpdateLoginMeta`
- **Modified**: `internal/server/auth/login.go` — add `Platform` to `LoginRequest`; call `UpdateLoginMeta` after success
- **Modified**: `internal/testutil/helpers.go` — add `UpdateLoginMeta` to `MockUserStore`
- **Modified**: `internal/testutil/quota_mocks.go` — add `UpdateLoginMeta` to `DenyQuotaUserStore`
- **Modified**: `internal/interface/cli/login.go` — pass `"cli"` to `client.Login`
- **Modified**: `internal/interface/desktop/app.go` — pass `"desktop"` to `client.Login`
- **Modified**: `portal/src/features/auth/api.ts` — add `platform: "portal"` to login body
