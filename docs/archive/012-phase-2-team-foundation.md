# Phase 2: Team Foundation

## Status

- phase: `2`
- name: `Team Foundation`
- status: `done`
- roadmap: [design/010-team-task-workflow-roadmap.md](./010-team-task-workflow-roadmap.md)
- depends_on: [design/011-phase-1-issue-uplift.md](./011-phase-1-issue-uplift.md)
- started_at: `2026-04-25`
- completed_at: `2026-04-25`

---

## 1. Goal

Introduce `team` as the real ownership and collaboration boundary for BuildMax without breaking the current personal-user experience.

After this phase, the system should behave as if all working resources live inside a team, even when the user is simply using their own default space.

This phase is intentionally about **ownership and collaboration foundation**, not workflow execution or enterprise governance.

---

## 2. Problem Statement

Phase 1 introduced `Issue` as a first-class work-management object, but the current product and codebase are still fundamentally `user` scoped:

- `issue` is user-scoped
- `agent` is user-scoped
- `conversation` is user-scoped
- portal request handling is mostly `JWT -> user_id -> store query -> ownership check`

That model is too limited for the roadmap direction.

BuildMax is evolving from:

- a user-private agent runtime

to:

- a collaborative work system where people, agents, and later workflows operate in a shared boundary

Without a real `team` boundary:

- issues cannot naturally belong to a shared work space
- agents cannot naturally become shared digital members
- assignment semantics remain artificially user-local
- later workflow ownership and execution become awkward

Phase 2 solves this by making `team` the canonical ownership boundary.

---

## 3. Core Decision

### 3.1 Team Is The Real Ownership Boundary

Decision fixed for Phase 2:

- `team` is the canonical ownership boundary for working resources
- personal usage is not a separate ownership model
- resources should not remain permanently split across `user-owned` and `team-owned` worlds

This means the long-term direction is:

- working resources belong to a `team`
- `created_by` stays as authorship metadata
- `user_id` can remain temporarily on some records for compatibility, but ownership should move to `team_id`

### 3.2 Every User Gets A Default Personal Team

Decision fixed for Phase 2:

- every user gets a default personal team
- this personal team is created automatically
- the personal team is the user’s default work space

This keeps the product simple:

- a new user can still start in a personal context immediately
- the backend still gets a uniform ownership model

### 3.3 `My Space` Is A Team Presentation, Not A Separate Entity

Decision fixed for Phase 2:

- `My Space` is only the default UX label / presentation for a personal team
- `My Space` is **not** a separate domain object
- there is no distinct `space` entity in this phase

This is critical for keeping the model clean.

### 3.4 `My Space` Can Invite Collaborators

Decision fixed for Phase 2:

- a personal team shown as `My Space` can invite other users
- when collaborators join, the same team continues to exist
- the system should not force a migration from “personal space” to “real team”

That means:

- `My Space` starts as a single-member team
- it may later become a multi-member team
- if it becomes collaborative, the UI may encourage renaming it, but the underlying team does not change identity

### 3.5 Keep Phase 2 Lightweight

Phase 2 should establish the ownership boundary and minimal collaboration flow, but should stay deliberately light:

- no advanced RBAC
- no org hierarchy
- no approvals
- no workflow entity yet
- no full workspace migration UI

---

## 4. Desired Outcome

After Phase 2:

- `team` exists as a first-class backend and product concept
- every user has a default personal team
- working resources belong to a team
- the portal can resolve a current team context
- a user can continue to use BuildMax as “My Space”
- the same space can later become collaborative by inviting members

At the end of this phase, BuildMax should have a stable ownership model that later phases can build on:

- Phase 3 workflow ownership
- team-shared assignment
- issue flow visibility in a shared space

### 4.1 MVP Decision

For this phase, the target is:

- establish `team` as the ownership boundary
- preserve a simple personal UX
- support minimal member collaboration
- avoid introducing heavy policy logic

Concretely, the MVP should be:

- one new `team` entity
- one new `team_member` entity
- automatic default personal team creation
- `team_id` added to core working resources
- current-team-aware portal APIs
- a minimal team list / member list surface

---

## 5. In Scope

This phase includes the following work.

### 5.1 Add Team Entities

Add:

- `team`
- `team_member`

Recommended MVP fields for `team`:

- `team_id`
- `name`
- `personal_for_user_id`
- `created_by`
- `created_at`
- `updated_at`

Recommended MVP fields for `team_member`:

- `team_id`
- `user_id`
- `role`
- `created_at`

Recommended MVP roles:

- `owner`
- `member`

### 5.2 Create Default Personal Team Automatically

Each user should get a default personal team automatically.

Recommended implementation rule:

- create the default personal team at user creation time
- do it in the same transaction as `user` creation

Reason:

- avoids half-created users without a team
- keeps downstream team resolution simple
- provides a strong invariant for future code

### 5.3 Move Working Resources Toward Team Ownership

Add `team_id` to the main working resources:

- `issue`
- `agent`
- `conversation`
- `task`

`task_run` does not need its own `team_id` in this phase because it is already owned through `task`.

`created_by` should remain because authorship and ownership are different concepts.

### 5.4 Introduce Team-Aware Request Context

Portal and server-side ownership checks should begin resolving:

- authenticated `user_id`
- current `team_id`

Recommended behavior:

- if no team is explicitly selected, use the user’s default personal team
- if a team is explicitly selected, verify the user is a member

This is the main request-context change for the phase.

### 5.5 Make Issue / Agent / Conversation / Task Access Team-Aware

The first team-aware resources in this phase should be:

- issues
- agents
- conversations
- tasks

That includes:

- list APIs
- detail APIs
- create/update paths
- ownership checks

### 5.6 Add Minimal Team APIs

Provide a minimal set of team APIs:

- list teams for current user
- create team
- list members of a team
- invite / add member to a team

This phase does not need full team administration.

### 5.7 Add Minimal Team UX

Portal UX should stay intentionally light.

Minimum target:

- show `My Space` as the default space label
- add team switcher only when relevant
- show team members for assignment selection
- allow inviting collaborators

Recommended UX behavior:

- when the user only has one team, the app can continue to feel like a personal space
- when the user has multiple teams, a team switcher should appear
- when `My Space` gains more than one member, the UI may suggest renaming it

---

## 6. Out Of Scope

This phase does **not** include:

- workflow CRUD or execution
- workflow assignment
- issue boards / advanced visual collaboration surfaces
- enterprise RBAC beyond `owner` / `member`
- approvals / audit / policy engines
- organization / department hierarchy
- deep migration of all historical product wording from “space” to “team”
- forcing a personal space to convert into a separate new team when collaborators are invited

---

## 7. Current Code Touch Points

Phase 2 is broader than Phase 1 because it changes the ownership model for multiple existing resources.

### 7.1 Storage / Entity Layer

Likely changes:

- `internal/core/model/db_entities.go`
- `internal/core/model/db_repositories.go`
- `internal/infra/db/store.go`
- new files for `team` and `team_member` stores
- `internal/infra/db/user.go`
- existing stores for issue / agent / conversation / task

### 7.2 Application Layer

Likely changes:

- issue service validation and assignment checks
- task service ownership assumptions
- conversation service ownership assumptions

Likely files:

- `internal/core/issue/service.go`
- `internal/core/task/service.go`
- `internal/core/conversation/service.go`

### 7.3 Server / Portal Backend

Likely changes:

- portal auth and current-context resolution
- issue handlers
- agent handlers
- conversation handlers
- task handlers
- auth signup path so user creation also creates default personal team

Likely files:

- `internal/server/auth/otp.go`
- `internal/server/portal/auth.go`
- `internal/server/portal/config.go`
- `internal/server/portal/issues.go`
- `internal/server/portal/agents.go`
- `internal/server/portal/conversation_handlers.go`
- `internal/server/portal/tasks.go`

### 7.4 Portal Frontend

Likely changes:

- team list / switcher
- member list / invite flow
- issue assignee selection
- agent list visibility by current team
- current-team propagation in API requests

Likely files:

- `portal/src/layout/*`
- `portal/src/features/issues/*`
- `portal/src/features/agents/*` or current agent pages
- router / API client / auth context as needed

---

## 8. Data Model Decisions

### 8.1 Team Table

Recommended semantics:

- one row per collaboration boundary
- can represent both personal and collaborative teams

Recommended notes:

- `personal_for_user_id` is nullable
- when non-null, it identifies the default personal team for that user
- this should be unique so one user cannot accidentally get multiple default personal teams

### 8.2 Team Member Table

Recommended semantics:

- one row per user membership in a team
- unique key on `(team_id, user_id)`

Recommended notes:

- `owner` is the initial role for the creator
- invited collaborators can start as `member`

### 8.3 Resource Ownership

Recommended rule:

- `team_id` is the ownership column
- `created_by` is authorship metadata
- compatibility `user_id` fields may temporarily remain where needed, but should not remain the long-term ownership contract

### 8.4 Team Evolution Rule

Recommended rule:

- a team does not change identity just because it changes from single-member to multi-member

This avoids:

- resource migration
- broken references
- confusing “convert my space into a team” flows

---

## 9. API And Request-Context Decisions

### 9.1 Current Team Resolution

Recommended request resolution model:

1. authenticate request to get `user_id`
2. resolve current `team_id`
3. run resource access through the current team boundary

Recommended first implementation:

- support explicit current team selection through request metadata
- fall back to the user’s default personal team when not specified

The exact transport shape can stay simple in this phase.

### 9.2 Team APIs

Recommended MVP APIs:

- `GET /api/teams`
- `POST /api/teams`
- `GET /api/teams/{team_id}/members`
- `POST /api/teams/{team_id}/members`

These should be enough for:

- team listing
- current-team selection in UI
- basic invitation / collaboration setup

### 9.3 Resource APIs

Existing resource APIs do not need a full redesign in this phase.

Recommended rule:

- keep current endpoint shapes where practical
- make them team-aware under the hood
- ensure list/detail access checks are based on current team membership

---

## 10. Product / UX Decisions

### 10.1 `My Space`

Recommended UX explanation:

> Every user starts with My Space. My Space is your default work space in BuildMax. It starts with just you, but you can invite others later if you want to collaborate.

This keeps onboarding light while preserving the real team model underneath.

### 10.2 Team Visibility

Recommended UI behavior:

- if the user only has one team, do not over-emphasize team management
- if the user has multiple teams, show a switcher
- if `My Space` becomes collaborative, encourage naming it more explicitly

### 10.3 Assignee UX

Phase 2 should change issue assignment from:

- self or owned agent

to:

- current team member
- current team agent

This is a major visible benefit of the team foundation.

---

## 11. Implementation Strategy

Recommended implementation order:

### Step 1. Add Team And TeamMember

- add entity models
- add store interfaces and implementations
- add migration via `AutoMigrate`

### Step 2. Create Default Personal Team On User Creation

- update user creation flow
- ensure user + personal team + owner membership are created transactionally

### Step 3. Add `team_id` To Working Resources

- issue
- agent
- conversation
- task

Backfill existing rows through migration logic or controlled bootstrap logic as needed.

### Step 4. Add Team Resolution To Portal Requests

- authenticate user
- resolve current team
- validate membership

### Step 5. Migrate Resource Handlers To Team-Aware Checks

- issues
- agents
- conversations
- tasks

### Step 6. Add Team APIs

- list teams
- create team
- list members
- add / invite member

### Step 7. Add Minimal Portal Team UX

- current team display
- switcher when needed
- member management
- assignee sources from current team

---

## 12. Validation / Acceptance Checks

Phase 2 is complete when all of the following are true:

1. every newly created user automatically gets one default personal team
2. the default personal team is the user’s initial `My Space`
3. `My Space` can add collaborators without becoming a different entity
4. `issue`, `agent`, `conversation`, and `task` belong to a team
5. portal resource access is checked through team membership
6. issue assignment can use current team members and current team agents
7. a user with only one team can still use the product without feeling forced into enterprise/team-heavy UX
8. a user with multiple teams can switch context explicitly

---

## 13. Risks And Mitigations

### 13.1 Risk: Dual Ownership Model Persists Too Long

If `user_id` continues to act as the real ownership key after `team_id` is introduced, the system will become confusing.

Mitigation:

- make `team_id` the intended ownership contract early
- keep `user_id` only as compatibility metadata where necessary

### 13.2 Risk: Personal UX Feels Too Enterprise

If the UI exposes “team” too aggressively on day one, solo users may feel the product has become too heavy.

Mitigation:

- present the default team as `My Space`
- hide or de-emphasize team management when only one team exists

### 13.3 Risk: Personal Team And Collaborative Team Diverge Conceptually

If `My Space` is treated as a fundamentally different object, later collaboration flows will become messy.

Mitigation:

- define `My Space` as presentation only
- keep one `team` model underneath

### 13.4 Risk: Resource Migration Becomes Error-Prone

Adding `team_id` across multiple resource tables can create inconsistent ownership if migrations are partial.

Mitigation:

- backfill from strong rules
- add tests around ownership resolution
- migrate handler groups progressively but deliberately

---

## 14. Open Questions / Follow-Ups

These do not block Phase 2 design, but should be tracked.

1. Should current team selection be carried by request header, query parameter, persisted user preference, or a combination?
2. Do we want team invitations to work only for existing users in this phase, or should we support invite-by-email immediately?
3. When `My Space` becomes collaborative, should the rename prompt be soft guidance only, or should it eventually become required?
4. Which compatibility `user_id` fields should remain after Phase 2, and which should be cleaned up immediately?
5. Do we want the login response or auth/session model to include current-team hints in this phase, or keep that entirely as a portal-side fetch after login?

---

## 15. Current Status

Current phase decision state:

- `team` is the unified ownership boundary: decided
- default personal team per user: decided
- `My Space` is team presentation only: decided
- `My Space` may become collaborative without identity change: decided
- exact request transport for current-team selection: open
- invitation mechanics detail: open

This phase should now proceed to implementation using this document plus the roadmap as the primary context.
