# Design 085: Quota and limit

## Goal

Implement per-user quota: attach a tier to each user, define extensible tier limits (max runs and max tokens per period), aggregate usage from Chat/ChatRun, and enforce limits at create-chat and create-run by returning HTTP 429 with a clear message when the user would exceed their tier quota.

## Modules


| Module                      | Responsibility                                 | Owns                                                                                                                           |
| --------------------------- | ---------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| **internal/model**          | Domain types                                   | `User` gains optional `QuotaTier` (string); new `QuotaTier` (tier row with limits).                                            |
| **internal/config**         | Default tier name only                         | Env key BUILDMAX_DEFAULT_QUOTA_TIER; tier *limits* come from DB.                                                               |
| **internal/storage/entity** | User tier, quota_tier table, usage aggregation | `User` tier column; table `quota_tier`; GetQuotaTier(ctx, tierName); UserUsageInWindow; CreateUser assigns default tier.       |
| **internal/quota**          | Quota check orchestration                      | `Checker` with dependencies; `Check(ctx, userID, addRuns, addTokens)` returns allowed + reason; uses rolling window from tier. |
| **internal/server**         | Enforcement in handlers, 429 response          | Before `CreateChat` / `CreateChatRun`, call quota checker; write 429 + JSON body on reject.                                    |
| **internal/servercmd**      | Wire quota checker into server                 | Build `quota.Checker` from config + entity store; pass into `Server` config.                                                   |


## Structure

### Model

- **internal/model/models.go**
  - On `User`, add `QuotaTier string` with `gorm:"type:varchar(64)"` and `json:"quota_tier,omitempty"`. Empty means use default tier at resolution time.
  - New type **QuotaTier** (row in `quota_tier` table): `TierName string` (PK, e.g. "free_trial", "pro"), `MaxRunsPerPeriod int`, `MaxTokensPerPeriod int`, `PeriodDays int`; gorm and json snake_case (`tier_name`, `max_runs_per_period`, `max_tokens_per_period`, `period_days`). `TableName() string { return "quota_tier" }`.

### Entity (storage)

- **internal/storage/entity/user.go**
  - Add `GetUser(ctx context.Context, userID string) (*User, error)`: look up by `user_id`; return (nil, nil) when not found.
  - In `CreateUser`, set `u.QuotaTier = defaultTier` when defaultTier is non-empty. Caller (servercmd) will pass default tier from config; if no config, leave empty (checker will treat unknown tier as no limit or use a hardcoded default in quota package).
- **internal/storage/entity/store.go** — Add `QuotaTier` to `AutoMigrate` so table `quota_tier` is created; add column `quota_tier` to User via model change. After migrate, run **seed** (or migration step): if no rows in `quota_tier`, insert default rows for `free_trial` (e.g. 10 runs, 100_000 tokens, 30 days) and `pro` (e.g. 1000 runs, 10_000_000 tokens, 30 days) so deployment works without config file.
- **internal/storage/entity/quota_tier.go** (new) — `GetQuotaTier(ctx, tierName string) (*QuotaTier, error)`; look up by `tier_name`. Optional: `SeedDefaultQuotaTiers(ctx)` called from store.New or a migration helper — insert free_trial and pro if table is empty.
- **internal/storage/entity/quota_usage.go** (new)
  - `UserUsageInWindow(ctx context.Context, userID string, sinceUnix, untilUnix int64) (runCount int, totalTokens int, err error)`:
    - Run count: count `chat_run` rows where `chat_run.chat_id` IN (SELECT `chat_id` FROM `chat` WHERE `created_by` = userID) AND `chat_run.created_at` >= sinceUnix AND `chat_run.created_at` <= untilUnix.
    - Token sum: (1) for those runs, sum COALESCE(`chat_run.prompt_tokens`,0) + COALESCE(`chat_run.completion_tokens`,0); (2) for chats where `created_by` = userID AND `created_at` in [sinceUnix, untilUnix], add sum of `title_prompt_tokens` + `title_completion_tokens`.
    - Use two queries or one joined raw query; return (0, 0, nil) on no rows.
- **internal/storage/entity/interfaces.go**
  - Extend `UserStore`: add `GetUser(ctx context.Context, userID string) (*User, error)`.
  - Add interface `UsageInWindowReader`: `UserUsageInWindow(ctx context.Context, userID string, sinceUnix, untilUnix int64) (runCount, totalTokens int, err error)`.
  - Add interface **QuotaTierStore**: `GetQuotaTier(ctx context.Context, tierName string) (*QuotaTier, error)` — returns (nil, nil) when tier not found. Implement on `Store`.
  - Implement all on `Store` (quota_usage.go + quota_tier.go).

### Config

- **internal/config/env_spec.go** — Add `EnvKeyBuildmaxDefaultQuotaTier = "BUILDMAX_DEFAULT_QUOTA_TIER"` (e.g. default `"free_trial"`). Add to `EnvVars`. No tier limits file; tier limits are stored in the **quota_tier** table.
- **internal/config/quota.go** (new, minimal) — `DefaultQuotaTier() string`: read from env `BUILDMAX_DEFAULT_QUOTA_TIER`; return "" or "free_trial" if unset. Quota checker gets tier *limits* from **QuotaTierStore.GetQuotaTier(ctx, tierName)** (DB), not from config.

### Quota package

- **internal/quota/quota.go** (new)
  - Dependencies (injected): `UserStore` (GetUser), `UsageInWindowReader` (UserUsageInWindow), **QuotaTierStore** (GetQuotaTier), and default tier name (string or func() string from config).
  - `Checker` struct: holds these dependencies.
  - `Check(ctx context.Context, userID string, addRuns, addTokens int) (allowed bool, reason string)`:
    1. Get user: `store.GetUser(ctx, userID)`; if nil, allow (backward compatibility) or deny—recommend allow.
    2. Resolve tier: if `user.QuotaTier != ""` use it, else use default tier from config.
    3. Get tier limits: **QuotaTierStore.GetQuotaTier(ctx, tierName)**. If nil (unknown tier), allow (no limit)—recommend allow.
    4. Compute window: `periodDays` from limits; sinceUnix = now - periodDays*86400, untilUnix = now.
    5. Call UserUsageInWindow(ctx, userID, sinceUnix, untilUnix) → currentRunCount, currentTokens.
    6. If currentRunCount + addRuns > limits.MaxRunsPerPeriod, return (false, "quota exceeded: run limit").
    7. If currentTokens + addTokens > limits.MaxTokensPerPeriod, return (false, "quota exceeded: token limit").
    8. Return (true, "").
  - Use current time from clock or time.Now() for window end; allow injecting clock for tests.

Design choice: **Tier limits come from DB** via `QuotaTierStore.GetQuotaTier(ctx, tierName)`. Adding or changing a tier = INSERT/UPDATE row in `quota_tier`; no config file or code change. Quota package depends on entity interfaces only (and default tier string from config).

### Server

- **internal/server/server.go**
  - Add to `Config`: `QuotaChecker *quota.Checker` (optional; if nil, no quota check).
  - No change to route registration.
- **internal/server/chats.go**
  - **createWorkspaceChatHandler**: After validating input and before calling ChatTitleGenerator, if s.cfg.QuotaChecker != nil: call Check(ctx, userID, 1, 0); if !allowed, write 429 and JSON `{"error": reason}`, return. After title generation (so we have title tokens): call Check(ctx, userID, 1, titlePromptTokens+titleCompletionTokens); if !allowed, write 429, return. Then create chat as today.
  - **createChatRunHandler**: After validating input and before CreateChatRun, if s.cfg.QuotaChecker != nil: call Check(ctx, userID, 1, 0); if !allowed, write 429 and JSON `{"error": reason}`, return. Then create run as today.
- **internal/server/errors.go** or helpers — Add `writeQuotaExceeded(w http.ResponseWriter, reason string)` that sets status 429 and body `{"error": "quota exceeded: run limit"}` or token variant (snake_case optional: `quota_exceeded`).
- **OpenAPI** — In `static/openapi.json`, for POST create chat and POST create run, add response 429 with description "Quota exceeded" and schema for error body.

### Servercmd wiring

- **internal/servercmd/run.go** (or where server is built): After creating Store, get default tier name from `config.DefaultQuotaTier()`. Build `quota.Checker`: UserStore = store, UsageInWindowReader = store, **QuotaTierStore = store**, DefaultTier = config.DefaultQuotaTier(). Pass Checker into server Config. When creating users (signup), call `CreateUser(ctx, email, defaultTier)` so new users get the default quota tier.

### CreateUser signature change

- **internal/storage/entity/interfaces.go** — `CreateUser(ctx context.Context, email string) (*User, error)` → `CreateUser(ctx context.Context, email string, defaultQuotaTier string) (*User, error)`. Implementations set User.QuotaTier when defaultQuotaTier is non-empty.
- **internal/storage/entity/user.go** — Implement new signature; all call sites of CreateUser must pass default tier (server login/signup: pass config default; tests: pass "" or "free_trial").

## Method and signature design


| Location             | Method / Type              | Signature / Shape                                                                                                     | Responsibility                                                     |
| -------------------- | -------------------------- | --------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| **model**            | User                       | Add `QuotaTier string` (gorm, json quota_tier)                                                                        | Per-user tier name.                                                |
| **model**            | QuotaTier                  | `TierName string` (PK), `MaxRunsPerPeriod, MaxTokensPerPeriod, PeriodDays int`; table `quota_tier`                    | One row per tier; limits for that tier.                            |
| **entity.UserStore** | GetUser                    | `(ctx context.Context, userID string) (*User, error)`                                                                 | Look up user by user_id for tier.                                  |
| **entity.UserStore** | CreateUser                 | `(ctx context.Context, email string, defaultQuotaTier string) (*User, error)`                                         | Create user; set QuotaTier when defaultQuotaTier != "".            |
| **entity**           | QuotaTierStore             | Interface: `GetQuotaTier(ctx context.Context, tierName string) (*QuotaTier, error)`                                   | Get tier limits by name from DB; (nil, nil) when not found.        |
| **entity**           | UsageInWindowReader        | Interface: `UserUsageInWindow(ctx, userID string, sinceUnix, untilUnix int64) (runCount, totalTokens int, err error)` | Aggregate usage for quota.                                         |
| **entity.Store**     | GetQuotaTier               | `(ctx, tierName string) (*QuotaTier, error)`                                                                          | Look up by tier_name in `quota_tier` table.                        |
| **entity.Store**     | UserUsageInWindow          | Same as UsageInWindowReader                                                                                           | Implement via JOIN chat + chat_run; sum runs and tokens.           |
| **config**           | DefaultQuotaTier           | `() string`                                                                                                           | Read BUILDMAX_DEFAULT_QUOTA_TIER (e.g. "free_trial").              |
| **quota**            | Checker                    | Struct with UserStore, UsageInWindowReader, **QuotaTierStore**, DefaultTier string                                    | Holds dependencies.                                                |
| **quota.Checker**    | Check                      | `(ctx context.Context, userID string, addRuns, addTokens int) (allowed bool, reason string)`                          | Enforce run and token limits; reason for 429.                      |
| **server**           | Config                     | Add `QuotaChecker *quota.Checker`                                                                                     | Optional; nil = no check.                                          |
| **server**           | createWorkspaceChatHandler | —                                                                                                                     | Before CreateChat: Check(userID, 1, titleTokens); 429 if !allowed. |
| **server**           | createChatRunHandler       | —                                                                                                                     | Before CreateChatRun: Check(userID, 1, 0); 429 if !allowed.        |
| **server**           | writeQuotaExceeded         | `(w http.ResponseWriter, reason string)`                                                                              | Status 429, body `{"error": reason}`.                              |


## How they work together

1. **Startup**
  Servercmd gets default tier name from `config.DefaultQuotaTier()`. Builds `quota.Checker` with store (UserStore + UsageInWindowReader + **QuotaTierStore**) and default tier. Tier limits are read from the **quota_tier** table at check time via `GetQuotaTier(ctx, tierName)`. Seed ensures `free_trial` and `pro` rows exist if table is empty.
2. **Create chat**
  Handler has userID from JWT. Calls ChatTitleGenerator (if set) to get title and title token usage. If QuotaChecker != nil: `allowed, reason := s.cfg.QuotaChecker.Check(ctx, userID, 1, titlePromptTokens+titleCompletionTokens)`. If !allowed, writeQuotaExceeded(w, reason) and return. Else call CreateChat as today.
3. **Create run**
  Handler has userID. If QuotaChecker != nil: `allowed, reason := s.cfg.QuotaChecker.Check(ctx, userID, 1, 0)`. If !allowed, writeQuotaExceeded(w, reason) and return. Else call CreateChatRun as today. Run tokens are not known until run completes; they are counted in the next window for the next Check. So token limit can be exceeded by one run’s usage; the following create will see the updated usage and return 429.
4. **Checker internals**
  GetUser(ctx, userID) → user. Tier = user.QuotaTier or default. **GetQuotaTier(ctx, tier)** → limits from DB; if nil (unknown tier), allow. Window: since = now - periodDays*86400, until = now. UserUsageInWindow(ctx, userID, since, until) → runCount, tokens. If runCount+addRuns > MaxRunsPerPeriod or tokens+addTokens > MaxTokensPerPeriod, return (false, reason). Else return (true, "").
5. **User registration**
  When a new user is created (e.g. signup), servercmd calls CreateUser(ctx, email, defaultTier) so the user gets the default quota tier. Existing users with empty QuotaTier are treated as default tier when Check runs (resolve tier = default if user.QuotaTier == "").
6. **Extensibility**
  New tier: INSERT a row into `quota_tier` (tier_name, max_runs_per_period, max_tokens_per_period, period_days). Change limits: UPDATE the row. No config file or redeploy. New limit type (future): add column to `quota_tier` and extend Check/aggregation.

## Time window

- Rolling window: `period_days` from tier (e.g. 30). End = now (Unix); start = now - period_days * 86400. All usage with `created_at` in [start, end] is counted. Same window for both run count and token sum.

## DB and migration

- **user**: Add column `quota_tier VARCHAR(64)` nullable or default "". GORM AutoMigrate.
- **quota_tier** (new table, singular per project convention): Columns `tier_name VARCHAR(64) PRIMARY KEY`, `max_runs_per_period INT NOT NULL`, `max_tokens_per_period INT NOT NULL`, `period_days INT NOT NULL`. GORM AutoMigrate with model `QuotaTier`.
- **Seed**: After migrate, if `quota_tier` has no rows, insert default rows: e.g. `free_trial` (10, 100000, 30), `pro` (1000, 10000000, 30). Implement in entity (e.g. `SeedDefaultQuotaTiers(ctx)`) and call from store.New or servercmd after store is created.

## Changes for review

- **internal/model/models.go** — Add `QuotaTier string` to User. Add **QuotaTier** struct (TierName, MaxRunsPerPeriod, MaxTokensPerPeriod, PeriodDays; table `quota_tier`).
- **internal/storage/entity/interfaces.go** — UserStore: add GetUser; CreateUser(ctx, email, defaultQuotaTier string). Add **QuotaTierStore**: GetQuotaTier(ctx, tierName) (*QuotaTier, error). Add UsageInWindowReader.
- **internal/storage/entity/user.go** — Implement GetUser; CreateUser with defaultQuotaTier.
- **internal/storage/entity/quota_tier.go** (new) — GetQuotaTier(ctx, tierName); optional SeedDefaultQuotaTiers(ctx) to insert free_trial and pro if table empty.
- **internal/storage/entity/quota_usage.go** (new) — UserUsageInWindow on Store.
- **internal/storage/entity/store.go** — AutoMigrate(&QuotaTier{}); after migrate call SeedDefaultQuotaTiers if desired.
- **internal/config/env_spec.go** — Add BUILDMAX_DEFAULT_QUOTA_TIER.
- **internal/config/quota.go** (new) — DefaultQuotaTier() string from env.
- **internal/quota/quota.go** (new) — Checker with UserStore, UsageInWindowReader, **QuotaTierStore**, DefaultTier; Check() uses GetQuotaTier for limits.
- **internal/server/server.go** — Config: add QuotaChecker *quota.Checker.
- **internal/server/chats.go** — createWorkspaceChatHandler: quota check after title gen (1 run + title tokens). createChatRunHandler: quota check (1 run, 0 tokens). writeQuotaExceeded helper.
- **internal/server/static/openapi.json** — 429 for create chat and create run.
- **internal/servercmd** — Build Checker with store (UserStore + QuotaTierStore + UsageInWindowReader) and config.DefaultQuotaTier(); pass defaultTier into CreateUser at registration.
- **All CreateUser call sites** — Update to CreateUser(ctx, email, defaultTier).
- **Tests** — quota: Check allowed/deny; entity: UserUsageInWindow, GetQuotaTier, seed; server: 429 when quota exceeded.

