# Phase 1: Product And Docs Reset

## Status

- phase: `1`
- name: `Product And Docs Reset`
- status: `done`
- roadmap: [design/018-versioned-workspace-and-outcome-roadmap.md](./018-versioned-workspace-and-outcome-roadmap.md)
- created_at: `2026-04-26`
- completed_at: `2026-04-26`

---

## 1. Goal

Bring top-level product documentation and planning documents into alignment with the actual current BuildMax system.

This phase is intentionally small and high-leverage.

It should make the repository easier to understand before we start deeper work on:

- outcome surfaces
- versioned workspace state
- restore and timeline
- governance 2.0

---

## 2. Problem Statement

The codebase has advanced beyond several of its most visible documents.

Examples of mismatch today:

- [README.md](../README.md) still describes a largely user-scoped model in its "Current Model" section
- [portal/README.md](../portal/README.md) still says the app currently uses synchronous mock data
- [design/001-about-portal.md](./001-about-portal.md) still centers the older `Workspace / Project / Task` product language
- the previous roadmap and adjacent status docs needed manual interpretation to understand what is already complete versus what is still active

This creates unnecessary friction:

- contributors must read code to learn the real product shape
- planning conversations spend time re-explaining current reality
- future phase documents risk inheriting outdated assumptions

---

## 3. Desired Outcome

After this phase:

- the top-level docs describe the actual current system
- the previous roadmap reads as complete for its intended scope
- the new roadmap is the obvious planning entry point
- product language becomes more stable across README, design docs, and portal docs

### 3.1 MVP Decision

This phase should optimize for:

- correctness
- clarity
- minimal churn

It should **not** try to rewrite every historical design doc.

The target is:

- update the current entry-point docs
- add explicit status notes where needed
- document the official current product explanation

---

## 4. In Scope

### 4.1 Update Top-Level README

Refresh [README.md](../README.md) so it accurately describes:

- the current ownership model
- the current main components
- the current portal/backend relationship
- the state of team/issue/workflow/governance work

### 4.2 Update Portal README

Refresh [portal/README.md](../portal/README.md) so it no longer describes the portal as a mock-data UI.

It should instead summarize:

- current real backend integration
- dev/build expectations
- the role of `@buildmax/gui`

### 4.3 Add A Stable Current Product Explanation

Define the short canonical explanation for the post-foundation BuildMax product, suitable for future README and design references.

This explanation should center:

- `Team`
- `Conversation`
- `Issue`
- `Workflow`
- `Task` / `TaskRun` as execution infrastructure

### 4.4 Close Planning/Status Drift

Update planning docs so the repository makes it easy to tell:

- what the previous roadmap completed
- what cleanup landed outside the main phase chain
- which roadmap is current
- which older design docs are now archival references only

### 4.5 Identify Phase 2 Seeds

Capture the exact user-facing outcome/result gaps that should seed Phase 2.

Expected examples:

- issue detail artifact/result aggregation
- conversation-visible output objects
- better result preview surfaces

---

## 5. Out Of Scope

- backend behavior changes unrelated to documentation alignment
- frontend redesign
- workspace versioning implementation
- audit/event implementation
- workflow capability expansion
- large historical doc migration across every archived or superseded document

---

## 6. Current Code Touch Points

Likely touch points for this phase:

- [README.md](../README.md)
- [portal/README.md](../portal/README.md)
- [design/010-team-task-workflow-roadmap.md](./010-team-task-workflow-roadmap.md)
- [design/016-phase-5-enterprise-governance-foundation.md](./016-phase-5-enterprise-governance-foundation.md)
- [design/017-team-scoped-files-upload-alignment.md](./017-team-scoped-files-upload-alignment.md)
- [design/018-versioned-workspace-and-outcome-roadmap.md](./018-versioned-workspace-and-outcome-roadmap.md)

Reference code anchors:

- [internal/storage/entity/models.go](../internal/storage/entity/models.go)
- [internal/app/conversation/service.go](../internal/app/conversation/service.go)
- [internal/app/workflow/service.go](../internal/app/workflow/service.go)
- [internal/server/portal/team_authz.go](../internal/server/portal/team_authz.go)
- [portal/src/pages/IssueDetail.tsx](../portal/src/pages/IssueDetail.tsx)
- [portal/src/pages/WorkflowDetail.tsx](../portal/src/pages/WorkflowDetail.tsx)

---

## 7. Implementation Steps

### Step 1. Reconfirm Current Product Truth

Use the current repository state as source of truth and verify:

- ownership model
- main user-facing objects
- workflow lifecycle/governance status
- file/team alignment status

### Step 2. Refresh Entry Docs

Update:

- `README.md`
- `portal/README.md`

so they describe the actual product and implementation state.

### Step 3. Normalize Planning Status

Ensure old/new roadmap docs clearly communicate:

- prior roadmap completion
- current roadmap entry point
- any major deferred areas

### Step 4. Write The Canonical Product Summary

Add or refine a short current-system summary that future docs can reuse.

### Step 5. Record Phase 2 Candidate Gaps

Write down the exact outcome-surface gaps that should become the next implementation phase.

---

## 8. Validation / Acceptance Checks

This phase is acceptable when:

- a contributor can read `README.md` and understand the current product model without reading old migration history
- `portal/README.md` no longer claims the portal is mock-data-driven
- the previous roadmap is clearly complete for its planned scope
- the new roadmap is clearly the active roadmap
- the remaining outcome-surface gaps are explicit enough to seed Phase 2

Recommended validation:

- manual review of touched docs for consistency
- optional portal build check if README/dev-command wording is updated materially

---

## 9. Open Questions

1. How much of [design/001-about-portal.md](./001-about-portal.md) should be rewritten now versus treated as long-range vision?
2. Should the official short product explanation lead with `Conversation` or `Issue` as the user's primary entry point?
3. Should top-level docs already introduce "versioned workspace" language before Phase 3 implementation exists, or should they frame it as near-term direction?

---

## 10. Recommended Immediate Next Step

The next implementation conversation for this phase should start by updating:

1. [README.md](../README.md)
2. [portal/README.md](../portal/README.md)

using the already-landed codebase as source of truth, then close with a short list of concrete Phase 2 outcome-surface gaps.
