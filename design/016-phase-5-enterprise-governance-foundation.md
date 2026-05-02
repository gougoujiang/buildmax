# Phase 5: Enterprise Governance Foundation

## Status

- phase: `5`
- name: `Enterprise Governance Foundation`
- status: `done`
- roadmap: [design/010-team-task-workflow-roadmap.md](./010-team-task-workflow-roadmap.md)
- depends_on: [design/014-phase-4-issue-flow-visualization.md](./014-phase-4-issue-flow-visualization.md)
- started_at: `2026-04-26`
- completed_at: `2026-04-26`

---

## 1. Goal

Add the minimum governance foundation needed so a shared team can safely operate Issues, Agents, and Workflows without introducing a heavy enterprise platform.

This phase is about adding a durable control layer, not about changing the product mental model introduced in Phases 1-4.

The visible concepts remain:

- `Team`
- `Issue`
- `Agent`
- `Workflow`
- `Conversation`

Phase 5 should answer:

- who is allowed to manage shared team resources
- how shared execution should be limited or controlled
- how workflow definitions become safe shared assets instead of free-form mutable records

---

## 2. Current State

After Phases 1-4, the system already has a solid collaboration and execution baseline:

- `team` is the ownership boundary for issues, agents, workflows, conversations, and tasks
- `issue` is the user-facing work object
- `workflow` is a reusable team-scoped execution plan
- issue detail already shows issue-centric process visibility

However, governance is still intentionally thin.

### 2.1 What Exists Today

- Team membership roles only include:
  - `owner`
  - `member`
- Team member management rules are enforced directly in portal handlers.
- Team membership checks are route-level and team-scoped, but not action-scoped.
- Quota enforcement exists, but it is still `per-user`, not `per-team`.
- Workflows are editable team-scoped records with no publishing lifecycle.
- There is no durable audit/event model.
- There is no approval or review checkpoint model.

### 2.2 Key Code Anchors

- Team models and role constants:
  - `internal/core/model/db_entities.go`
  - `internal/infra/db/team.go`
- Team-aware route authorization:
  - `internal/server/portal/auth.go`
- Team member management handler logic:
  - `internal/server/portal/teams.go`
- Issue assignment validation:
  - `internal/core/issue/service.go`
- Workflow CRUD and run behavior:
  - `internal/core/workflow/service.go`
- User-scoped quota enforcement:
  - `internal/core/quota/quota.go`
  - `internal/core/task/service.go`

### 2.3 Main Gap

The current codebase supports collaboration, but it does not yet support governance.

In practice:

- any team member can still create or mutate many shared resources
- action permissions are not modeled centrally
- workflow definitions do not have a lifecycle like draft vs published
- quota and policy do not align with team ownership

That makes the system functional for small teams, but not yet safe enough for larger shared-team operation.

---

## 3. Core Decisions

### 3.1 Governance Must Stay Additive

Phase 5 should extend the current Team / Issue / Workflow model, not replace it.

We should not introduce:

- organization trees
- department hierarchies
- full custom RBAC
- approval DSLs
- policy engines

The first slice must stay small enough to fit the current codebase shape.

### 3.2 Start With Role Clarification Before Audit Or Approval

The first implementation slice should not start with audit log or approvals.

Instead, it should start with:

- richer team roles
- centralized action authorization
- workflow lifecycle constraints

Reason:

- permissions are the base layer that later approval and audit features depend on
- current authorization logic is scattered and partly hard-coded in handlers
- workflow governance is much easier once action authorization exists

### 3.3 Team Is The Governance Boundary

Phase 5 governance should be centered on `team`, not `user`.

This means:

- permissions should be evaluated in the context of a team membership
- quota and policy should be able to evolve toward team scope
- workflow publication and visibility rules should be team-governed

### 3.4 Workflow Needs A Lightweight Lifecycle

Workflows should stop behaving like ungoverned mutable records.

Phase 5 should introduce a lightweight lifecycle, with a minimum target such as:

- `draft`
- `published`
- `archived`

This is not about a marketplace or release management system. It is only about making shared workflows safer to reference and run.

### 3.5 Audit Must Be Deferred Unless It Naturally Falls Out

Audit is important, but a full durable audit/event subsystem is a larger step.

Unless implementation becomes nearly free through other changes, this phase should avoid introducing:

- a general audit event table
- broad replay semantics
- complex actor/action history UI

Instead, Phase 5 should prepare the boundaries where audit can be added later.

---

## 4. Desired Outcome

After this phase:

- a shared team can distinguish between people who administer the team and people who only participate in work
- sensitive shared actions use one central authorization model instead of per-handler ad hoc rules
- workflows can be governed as shared assets through a small lifecycle
- the system has a clear path to future team-scoped quota and policy

### 4.1 MVP Decision

The MVP for this phase should focus on governance foundation, not enterprise breadth.

Concretely, the recommended MVP is:

- expand team role model from `owner/member` to `owner/admin/member`
- introduce a central authorization helper/service for portal actions
- gate sensitive shared actions through role checks
- add workflow lifecycle state
- restrict workflow execution to non-archived and preferably published workflows

Deferred from the MVP:

- approvals / review checkpoints
- durable audit log
- team-scoped quota enforcement
- custom roles
- per-workflow reviewer lists

---

## 5. In Scope

### 5.1 Expand Team Roles

Add one intermediate governance role:

- `owner`
- `admin`
- `member`

Recommended semantics:

- `owner`: full control, including team membership and destructive governance actions
- `admin`: manage shared work resources, but not team ownership transfer semantics
- `member`: participate in work, but with reduced governance permissions

### 5.2 Centralize Authorization

Introduce a narrow authorization layer for portal actions.

The first target is not a generic policy framework. It is a small service/helper that answers questions like:

- can manage team members
- can manage agents
- can manage workflows
- can run workflows
- can assign an issue to a workflow

This should replace repeated membership/owner checks spread across handlers.

### 5.3 Add Workflow Lifecycle State

Add a workflow lifecycle field, with recommended values:

- `draft`
- `published`
- `archived`

Recommended behavior:

- newly created workflows start as `draft`
- `draft` workflows are editable
- `published` workflows are runnable and assignable
- `archived` workflows are not runnable and should not be assignable for new work

### 5.4 Apply Governance To Sensitive Actions

First-wave actions that should move under the new authorization model:

- add/remove team member
- create/update/delete agent
- create/update workflow
- publish/archive workflow
- trigger workflow run
- assign issue to workflow

This does not require locking down every read path in the first slice.

### 5.5 Define Team-Scoped Quota Direction

Do not fully implement team-scoped quota in this first slice unless it turns out to be low-cost.

But Phase 5 should explicitly define the intended direction:

- quota is currently checked by user
- future enterprise governance should support quota/policy by team
- new authorization and workflow lifecycle changes should not block that migration

At minimum, this phase should document the migration boundary and avoid deepening user-scoped assumptions.

---

## 6. Out Of Scope

This phase does not include:

- custom RBAC matrix editing UI
- organization / workspace trees
- SSO / SCIM
- workflow approval graphs
- issue approval checkpoints
- durable global audit/event history
- full team-scoped quota implementation
- billing/admin console work

---

## 7. Proposed Permission Model

This permission model is intentionally small.

### 7.1 Team Roles

- `owner`
- `admin`
- `member`

### 7.2 Action Matrix

Recommended first-pass matrix:

| Action | Owner | Admin | Member |
|------|------|------|------|
| View team resources | yes | yes | yes |
| Create/edit issue | yes | yes | yes |
| Assign issue to person/agent | yes | yes | yes |
| Assign issue to workflow | yes | yes | no |
| Create/edit agent | yes | yes | no |
| Create/edit workflow draft | yes | yes | no |
| Publish/archive workflow | yes | yes | no |
| Trigger workflow run | yes | yes | yes |
| Add/remove team member | yes | no | no |

This matrix should be treated as the initial slice, not the final enterprise answer.

### 7.3 Reasoning

- Team member administration stays owner-only to keep social/admin control simple.
- `admin` becomes the operational governance role for shared automation assets.
- `member` can still participate in issue work and run approved workflows, but cannot freely change the team’s automation surface.

---

## 8. Data Model Direction

### 8.1 Team Member Role

No new entity is required for the first slice.

Use the existing `team_member.role` field and expand allowed values.

### 8.2 Workflow Lifecycle

Recommended additive change:

- add `workflow.status`

Recommended values:

- `draft`
- `published`
- `archived`

This is enough to support governance without introducing separate version entities yet.

### 8.3 Future Audit Hook

When action authorization and workflow lifecycle updates occur, keep mutation boundaries explicit so a future audit layer can hook into:

- membership changes
- workflow publish/archive actions
- agent creation/deletion
- issue assignment to governed targets

This is a design constraint, not a requirement to implement audit in this phase.

---

## 9. Current Code Touch Points

Likely code areas for the first implementation slice:

- `internal/core/model/db_entities.go`
- `internal/infra/db/team.go`
- `internal/core/model/db_repositories.go`
- `internal/core/issue/service.go`
- `internal/core/workflow/service.go`
- `internal/server/portal/auth.go`
- `internal/server/portal/teams.go`
- `internal/server/portal/agents.go`
- `internal/server/portal/workflows.go`
- `internal/server/portal/issues.go`
- `portal/src/pages/TeamSettings.tsx`
- `portal/src/pages/WorkflowDetail.tsx`
- `portal/src/pages/IssueDetail.tsx`

Potential new package if useful:

- `internal/authz` or `internal/core/teamauth`

The key architectural goal is to avoid burying new governance rules directly inside individual handlers again.

---

## 10. Implementation Plan

### Step 1: Lock Phase 5 Scope

Before coding, lock the first slice to:

- role expansion
- centralized authorization
- workflow lifecycle

Explicitly defer:

- audit log
- approvals
- full team quota

### Step 2: Add Role And Workflow Status Constants

Add explicit constants for:

- `team_member.role`
- `workflow.status`

Avoid string duplication across handlers and services.

### Step 3: Add Central Authorization Helper

Create a narrow authorization layer that takes:

- current user id
- current team id
- team membership role
- requested action

and returns allowed / denied.

This should first be consumed by portal handlers and application services where practical.

### Step 4: Migrate Sensitive Handlers To Authorization Layer

Replace ad hoc owner checks and add role-aware checks for:

- team member add/remove
- agent create/update/delete
- workflow create/update/publish/archive/run
- workflow assignment from issue detail

### Step 5: Add Workflow Lifecycle UI/API

Expose workflow lifecycle in API and Portal.

Recommended first UI behavior:

- show current workflow status on detail page
- allow status change for authorized users
- disable run/assignment behavior when workflow status forbids it

### Step 6: Validate End-To-End Team Safety

Check that a shared team with:

- one owner
- one admin
- one member

behaves predictably across issue, agent, and workflow flows.

---

## 11. Validation / Acceptance Checks

Phase 5 first slice is acceptable when:

- team roles support `owner/admin/member`
- authorization decisions for sensitive actions are no longer hard-coded independently in each handler
- a non-owner admin can manage shared workflow/agent assets
- a member cannot mutate governed workflow/agent assets
- archived workflows cannot be newly run or newly assigned
- published workflows remain usable by the team

Recommended validation:

- focused Go tests for authorization logic
- portal handler tests for allowed/forbidden behavior by role
- workflow service tests for lifecycle enforcement
- portal build validation after any UI changes

---

## 12. Open Questions

These questions should be answered during implementation, not before the phase starts:

1. Should `member` be allowed to manually trigger a `published` workflow, or should that also require `admin`?
2. Should workflow assignment require `published`, or can `draft` workflows still be assigned but not run?
3. Should agent create/edit/delete and workflow create/edit/delete use exactly the same role rules in MVP?
4. Should team quota remain displayed as user usage until team quota really exists, or should UI wording change first?

---

## 13. Recommended Immediate Next Step

The next implementation conversation should start with the smallest viable vertical slice:

1. add `admin` role
2. centralize role-based authorization
3. migrate team member, agent, and workflow mutation checks to the new authorization layer
4. add workflow `status`
5. gate workflow run + issue workflow assignment on `status`

This gives BuildMax its first real governance foundation without prematurely expanding into a full enterprise system.
