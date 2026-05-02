# Phase 5: Team Quota Foundation

## Status

- phase: `5`
- name: `Team Quota Foundation`
- status: `not_started`
- roadmap: [design/018-versioned-workspace-and-outcome-roadmap.md](./018-versioned-workspace-and-outcome-roadmap.md)
- created_at: `2026-05-02`

---

## 1. Goal

Move quota enforcement and usage display from **user scope** to **team scope**.

This phase is the smallest governance 2.0 slice that brings quota policy back into alignment with the current ownership model:

- work belongs to a `team`
- tasks run in a `team`
- files belong to a `team`
- workflows belong to a `team`

Quota should therefore be governed at the same boundary.

---

## 2. Problem Statement

The product has already moved to team-owned work, but quota has not followed.

Today:

- task creation checks quota by `user_id`
- usage reporting is fetched by `user_id`
- quota tier assignment lives on `user`
- usage aggregation queries usage by task author instead of task team

That creates model mismatch in shared-team operation:

- two members working in the same team do not share one capacity pool
- team-owned workflows are still billed/limited as if they were personal activity
- team settings cannot show the quota that actually governs team work
- governance remains split between team-owned work and user-owned limits

---

## 3. Current State

### 3.1 What Exists Today

- Quota checker exists and blocks task creation with 429 when limits are exceeded.
- Quota tiers are stored in `quota_tier`.
- Usage API exists at `GET /api/usage`.
- Signup assigns a default quota tier to `user.quota_tier`.

### 3.2 Current Code Anchors

- checker logic:
  - [internal/quota/quota.go](../internal/quota/quota.go)
- task service integration:
  - [internal/app/task/service.go](../internal/app/task/service.go)
- portal usage endpoint:
  - [internal/server/portal/usage.go](../internal/server/portal/usage.go)
- usage aggregation:
  - [internal/storage/entity/quota_usage.go](../internal/storage/entity/quota_usage.go)
- quota tier model:
  - [internal/storage/entity/models.go](../internal/storage/entity/models.go)
  - [internal/storage/entity/quota_tier.go](../internal/storage/entity/quota_tier.go)

### 3.3 Key Technical Mismatch

The current interfaces are explicitly user-scoped:

- `QuotaChecker.Check(ctx, userID, ...)`
- `QuotaChecker.GetUsage(ctx, userID)`
- `UsageInWindowReader.UserUsageInWindow(ctx, userID, ...)`

That means a real team quota implementation cannot be done by UI changes alone. The core contract needs to move to `teamID`.

---

## 4. Desired Outcome

After this phase:

- quota enforcement is evaluated for the current `team`
- usage display shows team usage, not per-user usage
- the quota tier used for task creation belongs to the team
- personal usage still works through the default personal team

### 4.1 MVP Decision

The MVP should optimize for:

- model correctness
- low schema risk
- minimal product confusion

The MVP target is:

- one quota tier per team
- one team-scoped usage aggregation path
- task creation/rerun checks team quota
- one team-scoped usage endpoint or equivalent route behavior

The MVP should **not** attempt:

- per-member quota inside a team
- per-workflow quota
- billing console work
- quota history analytics UI

---

## 5. Core Decisions

### 5.1 Team Owns The Quota Tier

Quota tier assignment should move from `user` to `team`.

Reason:

- the team is the ownership boundary for work
- quota should govern the same unit that owns execution

Implication:

- default personal team becomes the private-user quota owner
- invited collaborative teams can have their own quota tier

### 5.2 Usage Must Aggregate By Task Team

Usage should be computed from work owned by the team.

The most direct existing anchor is:

- `task.team_id`

This aligns quota with:

- issue-triggered runs
- workflow-triggered runs
- conversation-created tasks

### 5.3 Keep Quota Tiers Shared As Definitions

The `quota_tier` table should remain the catalog of named tier definitions.

What changes is:

- which entity points to a tier

not:

- how tier definitions themselves are stored

### 5.4 Preserve Backward-Compatible Personal UX

No new special model is needed for personal users.

The default personal team already exists, so:

- a personal user still experiences one simple quota bucket
- internally that bucket is the personal team

### 5.5 Do Not Mix User And Team Enforcement Long-Term

Avoid permanent dual enforcement such as:

- check user quota for some task paths
- check team quota for others

If transitional compatibility is needed, it should be explicitly temporary and kept short-lived.

---

## 6. In Scope

### 6.1 Add Team Quota Tier Ownership

Add quota-tier ownership to `team`.

Recommended shape:

- `team.quota_tier`

Behavior:

- new personal teams default to the configured default tier
- newly created collaborative teams also receive a default tier unless explicitly set later

### 6.2 Add Team-Scoped Usage Aggregation

Extend the usage reader contract so usage can be aggregated for a team in a time window.

Recommended new capability:

- `TeamUsageInWindow(ctx, teamID, sinceUnix, untilUnix)`

Aggregation should count:

- run count from `task_run` joined through `task.team_id`
- run token usage from `task_run`
- title token usage from `task` created inside that team and time window

### 6.3 Update Quota Checker Contract

Update checker behavior from:

- `Check(ctx, userID, ...)`
- `GetUsage(ctx, userID)`

to team-oriented equivalents such as:

- `CheckTeam(ctx, teamID, ...)`
- `GetTeamUsage(ctx, teamID)`

Exact naming can vary, but the meaning should be explicit.

### 6.4 Enforce Quota On Team-Owned Task Creation

Task creation and follow-up runs should use the current task team when checking quota.

This affects:

- new conversation tasks
- explicit task run creation
- issue-triggered agent runs
- workflow-triggered task creation through the existing task path

### 6.5 Team-Scoped Usage Display

Usage UI/API should reflect team usage.

Minimum target:

- when the current team is selected, usage display shows that team's usage bucket

This can be:

- by keeping `GET /api/usage` but resolving current team from context
- or by moving to a team route such as `GET /api/teams/{team_id}/usage`

Recommendation:

- prefer the explicit team route for consistency with the rest of the API

---

## 7. Out Of Scope

- billing/subscription system
- team quota admin UI beyond basic display
- per-member sub-limits
- historical usage charts
- approval logic
- audit/event system
- automated quota rollover notifications

---

## 8. Current Code Touch Points

Likely touch points:

- [internal/storage/entity/models.go](../internal/storage/entity/models.go)
- [internal/storage/entity/interfaces.go](../internal/storage/entity/interfaces.go)
- [internal/storage/entity/quota_usage.go](../internal/storage/entity/quota_usage.go)
- [internal/quota/quota.go](../internal/quota/quota.go)
- [internal/app/task/service.go](../internal/app/task/service.go)
- [internal/server/portal/usage.go](../internal/server/portal/usage.go)
- [internal/server/portal/task_service.go](../internal/server/portal/task_service.go)
- [internal/server/portal/config.go](../internal/server/portal/config.go)
- [internal/cmd/server/run.go](../internal/cmd/server/run.go)
- [portal/src/pages/TeamSettings.tsx](../portal/src/pages/TeamSettings.tsx)

Tests likely affected:

- [internal/quota/quota_test.go](../internal/quota/quota_test.go)
- [internal/server/portal/usage_test.go](../internal/server/portal/usage_test.go)
- [internal/server/portal/tasks_test.go](../internal/server/portal/tasks_test.go)

---

## 9. Implementation Steps

### Step 1. Add Team Tier Ownership

Extend `team` with quota tier ownership and apply the default tier when creating teams.

Recommended rule:

- if no explicit tier is provided, use the same default tier currently used for users

### Step 2. Add Team Usage Reader

Implement a team-scoped usage aggregation path based on `task.team_id`.

Recommended initial SQL behavior:

- count `task_run` rows in the window for tasks owned by the team
- sum prompt/completion tokens for those runs
- add title-generation tokens from tasks created in the same window for the team

### Step 3. Update Quota Checker To Team Scope

Refactor the checker so team is the primary subject.

Recommended migration shape:

- introduce team-scoped methods first
- switch task service call sites
- remove user-scoped usage path after transition is complete

### Step 4. Thread TeamID Into Task Quota Checks

Update task service and its callers so quota checks use `teamID`.

This likely means:

- extending the quota-checker interface used by `internal/app/task`
- updating `CreateTask` and `CreateRun` call paths to supply team ownership explicitly

### Step 5. Update Usage API / UI

Expose team-scoped usage to the Portal and present it in the current team context.

Minimum UI target:

- team settings or current team context can show the team's quota tier and usage

### Step 6. Remove User-Quota Wording

Update labels and docs so the product no longer implies quota is user-owned once the backend is switched.

---

## 10. Validation / Acceptance Checks

This phase is acceptable when:

- quota enforcement is evaluated by `team`
- usage display reflects the current team, not the current user
- personal-team usage still works unchanged from the user perspective
- two users working in the same team consume the same quota bucket
- task creation returns 429 when the team quota is exceeded

Recommended validation:

- focused Go tests for team-scoped quota checker behavior
- usage aggregation tests covering:
  - personal team
  - shared team with two users
  - title-token counting
- Portal/API test for team usage endpoint behavior

---

## 11. Open Questions

1. Should the default quota tier remain configured globally by env, or should collaborative team creation allow explicit tier selection later?
2. Should `GET /api/usage` be replaced entirely, or should it become a thin alias for the current personal team?
3. Do we need a temporary compatibility path for existing `user.quota_tier`, or can we move directly because the project is still early-stage?
4. Should workflow-initiated work count only against the owning team, even when a specific member triggered the run?

---

## 12. Recommended Immediate Next Step

The next implementation conversation should start with the smallest viable vertical slice:

1. add `team.quota_tier`
2. add `TeamUsageInWindow`
3. switch quota checker to team scope
4. update task creation to call quota with `teamID`
5. expose team usage in one portal/API path

This gives BuildMax its first correct team-scoped quota model without waiting for the rest of governance 2.0.
