# Design 086: Allow user to see usage at settings page

## Goal

Expose the current user's quota usage and tier limits via a read-only API (`GET /api/usage`) and display them in the Portal Settings modal so the user can see run and token usage for the current period, with tier and limits when available.

## Modules


| Module                 | Responsibility                         | Changes                                                                                        |
| ---------------------- | -------------------------------------- | ---------------------------------------------------------------------------------------------- |
| **internal/quota**     | Provide usage snapshot for a user      | Add `UsageInfo` struct and `Checker.GetUsage(ctx, userID)` using same window logic as `Check`. |
| **internal/server**    | GET /api/usage handler, response shape | New handler; optional `usageHandler` in new file or existing; OpenAPI entry.                   |
| **internal/servercmd** | No change                              | Already wires QuotaChecker; usage uses it.                                                     |
| **portal**             | Settings modal usage section           | `SettingsModal`: fetch usage API, show runs/tokens and limits; loading/error state.            |


## Structure

### Quota package

- **internal/quota/quota.go**
  - **UsageInfo** struct (no JSON tags; server owns API shape): `RunCount`, `TotalTokens int`; `TierName string`; `PeriodDays int`; `MaxRunsPerPeriod`, `MaxTokensPerPeriod *int` (nil when no tier or tier not found).
  - **GetUsage(ctx context.Context, userID string) (UsageInfo, error)* on `Checker`:
    1. Get user: `UserStore.GetUser(ctx, userID)`. If nil or err, return `&UsageInfo{}`, nil (or return zero usage and empty tier).
    2. Resolve tier: `user.QuotaTier` else `c.DefaultTier`.
    3. Get tier: `TierStore.GetQuotaTier(ctx, tierName)`. If nil, return UsageInfo with RunCount/TotalTokens from a default window (e.g. 30 days) so UI can show usage; TierName set; PeriodDays = 30; MaxRunsPerPeriod, MaxTokensPerPeriod = nil.
    4. Compute window: `now := c.clock().Unix()`; `since := now - int64(tier.PeriodDays)*86400`.
    5. `UsageReader.UserUsageInWindow(ctx, userID, since, now)` → runCount, totalTokens.
    6. Return `&UsageInfo{RunCount, TotalTokens, TierName: tierName, PeriodDays: tier.PeriodDays, MaxRunsPerPeriod: &tier.MaxRunsPerPeriod, MaxTokensPerPeriod: &tier.MaxTokensPerPeriod}`.
  - When tier not found (step 3): still compute usage for a default 30-day window so "Runs: X, Tokens: Y" is shown; set PeriodDays = 30, leave limits nil.

### Server

- **internal/server/usage.go** (new)
  - **usageResponse** struct for JSON (snake_case): `RunCount int json:"run_count"`, `TotalTokens int json:"total_tokens"`, `TierName string json:"tier"`, `PeriodDays int json:"period_days"`, `MaxRunsPerPeriod *int json:"max_runs_per_period,omitempty"`, `MaxTokensPerPeriod *int json:"max_tokens_per_period,omitempty"`.
  - **usageHandler(w, r)**:
    1. `userID, ok := requireAuth(w, r, s.cfg.JWTSecret)`; if !ok return.
    2. If `s.cfg.QuotaChecker == nil`: write 503 with message "usage not available" (or 200 with zero/empty usage per product choice; spec says "when limits absent" so 200 with zero usage and no limits is acceptable). Prefer **503** when quota/usage is not configured so the UI can show "Usage not available".
    3. `info, err := s.cfg.QuotaChecker.GetUsage(r.Context(), userID)`; if err != nil write 500 and return.
    4. Build usageResponse from info (copy counts, tier, period_days; copy pointer values for limits).
    5. `writeJSON(w, http.StatusOK, response)`.
  - Register in **server.go**: `mux.HandleFunc("GET /api/usage", s.usageHandler)`.
- **internal/server/static/openapi.json**
  - Add path `**/api/usage`**:
    - **get**: summary "Get current user usage", description "Returns usage and tier limits for the authenticated user (rolling window). Requires Bearer JWT.", security bearerAuth, responses 200 (schema: object with run_count, total_tokens, tier, period_days, optional max_runs_per_period, max_tokens_per_period), 401 Unauthorized, 503 if usage not available.

### Portal

- **portal/src/lib/api/types.ts**
  - Add **ApiUsage** (or inline in index): `run_count: number; total_tokens: number; tier: string; period_days: number; max_runs_per_period?: number; max_tokens_per_period?: number`.
- **portal/src/lib/api/index.ts**
  - Add **getUsage(token: string): Promise****: **`GET ${getApiBase()}/api/usage` with `authHeaders(token)`.
- **portal/src/components/SettingsModal.tsx**
  - Use **useAuth()** to get token. When modal is open and token is present, call **getUsage(token)** (e.g. in useEffect or on open).
  - State: loading, error, data (ApiUsage | null).
  - Render a **Usage** section: tier name; "Runs: {run_count} / {max_runs_per_period}" (or "Runs: {run_count}" when max_runs_per_period is undefined); "Tokens: {total_tokens} / {max_tokens_per_period}" (or "Tokens: {total_tokens}" when no limit); period text e.g. "Rolling {period_days} days".
  - On loading: show placeholder (e.g. "Loading usage…"). On error (including 503): show "Usage not available" or error message.

## Method and signature design


| Location          | Method / Type | Signature / Shape                                                                                     | Responsibility                                                       |
| ----------------- | ------------- | ----------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| **quota**         | UsageInfo     | RunCount, TotalTokens int; TierName string; PeriodDays int; MaxRunsPerPeriod, MaxTokensPerPeriod *int | Snapshot for one user.                                               |
| **quota.Checker** | GetUsage      | (ctx context.Context, userID string) (*UsageInfo, error)                                              | Same window as Check; return usage + tier + optional limits.         |
| **server**        | usageResponse | JSON: run_count, total_tokens, tier, period_days, max_runs_per_period?, max_tokens_per_period?        | API response shape.                                                  |
| **server**        | usageHandler  | (w http.ResponseWriter, r *http.Request)                                                              | requireAuth; QuotaChecker.GetUsage; write JSON or 503.               |
| **portal api**    | getUsage      | (token: string) => Promise                                                                            | GET /api/usage with Bearer.                                          |
| **portal**        | SettingsModal | —                                                                                                     | Usage section: fetch, loading/error, display runs/tokens and limits. |


## How they work together

1. User opens Settings from the sidebar → SettingsModal opens.
2. Modal has token from useAuth(); it calls getUsage(token) → GET /api/usage with Bearer.
3. Server usageHandler: requireAuth → userID; if QuotaChecker == nil → 503; else QuotaChecker.GetUsage(ctx, userID) → UsageInfo; map to usageResponse → 200 JSON.
4. GetUsage: same resolution and window as Check (user → tier → tier row → since/until → UserUsageInWindow); when tier missing, still return usage for default 30-day window with limits = nil.
5. Portal receives JSON; displays tier, "Runs: X / Y" (or "Runs: X"), "Tokens: A / B" (or "Tokens: A"), "Rolling N days". Handles 503 and network errors with a clear message.

## Time window

- Same as quota enforcement: rolling window with end = now, start = now - period_days * 86400. When tier is unknown, use 30 days for the usage query so the UI shows current usage even without limits.

## Tests

- **internal/server/usage_test.go** (or usage_test.go next to usage.go): Table-driven: (1) no auth → 401; (2) no QuotaChecker (nil) → 503; (3) valid JWT + mock store returning user, tier, usage → 200 and response body has run_count, total_tokens, tier, period_days and optionally max_runs_per_period, max_tokens_per_period. Use a mock that implements quota dependencies or a small test helper that returns a fixed UsageInfo from Checker (if test-friendly constructor or injectable GetUsage exists).
- **internal/quota/quota_test.go**: Optional: test GetUsage returns correct UsageInfo for known user/tier and for unknown tier (limits nil, usage still computed for default window).

## Changes for review

- **internal/quota/quota.go** — Add `UsageInfo` struct; add `GetUsage(ctx context.Context, userID string) (*UsageInfo, error)` to `Checker` (same window logic as Check; when tier not found return usage for default 30-day window with limits nil).
- **internal/server/usage.go** (new) — Define `usageResponse` (snake_case JSON); `usageHandler` using requireAuth and QuotaChecker.GetUsage; 503 when QuotaChecker is nil.
- **internal/server/server.go** — Register `GET /api/usage` → `s.usageHandler`.
- **internal/server/static/openapi.json** — Add `GET /api/usage` with 200 (usage schema), 401, 503.
- **internal/server/usage_test.go** (new) — Handler tests: 401 without auth, 503 without QuotaChecker, 200 with mock and correct JSON shape.
- **portal/src/lib/api/types.ts** — Add `ApiUsage` type (run_count, total_tokens, tier, period_days, optional max_runs_per_period, max_tokens_per_period).
- **portal/src/lib/api/index.ts** — Export `getUsage(token)`.
- **portal/src/components/SettingsModal.tsx** — Usage section: useAuth, getUsage on open, loading/error/data state, display runs/tokens and limits (or "No limit" when absent).

