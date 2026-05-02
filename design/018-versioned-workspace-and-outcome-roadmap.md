# Versioned Workspace And Outcome Roadmap

## 1. Purpose

This document defines the **next product roadmap** after the Team / Issue / Workflow program in [design/010-team-task-workflow-roadmap.md](./010-team-task-workflow-roadmap.md).

The previous roadmap established the collaboration and execution foundation:

- `Team` is the ownership boundary
- `Issue` is the user-facing work object
- `Workflow` is the reusable execution plan
- `Conversation` is the Tier 1 interface
- `Task` / `TaskRun` remain the Tier 2 execution foundation

That program is now effectively complete for its intended scope.

The next roadmap should not spend its first phases reintroducing new top-level nouns. Instead, it should make the existing system feel like the product described in [design/001-about-portal.md](./001-about-portal.md):

- intent first
- workspace as the agent's body
- results visible at the user layer
- state versioned and reversible
- infrastructure hidden behind clear product meaning

---

## 2. Current Codebase Baseline

As of the current repository state, BuildMax already has:

- a shared Go agent runtime reused by CLI, desktop, and worker execution
- Tier 1 conversation orchestration in `internal/core/conversation`
- Tier 2 execution through `task` and `task_run`
- team-scoped `issue`, `agent`, `workflow`, `conversation`, and `task`
- workflow run and step-run persistence
- team-scoped uploaded files and worker runtime materialization
- Portal pages for conversations, issues, workflows, agents, and team settings
- centralized team action authorization and workflow lifecycle governance

Key anchors in code:

- shared runtime: `internal/execution/agentrun`
- Tier 1 orchestration: `internal/core/conversation`
- issue/workflow/task application services: `internal/core/issue`, `internal/core/workflow`, `internal/core/task`
- worker execution pipeline: `internal/execution`
- team authz: `internal/server/portal/team_authz.go`
- workflow lifecycle enforcement: `internal/core/workflow/service.go`
- issue flow UI: `portal/src/pages/IssueDetail.tsx`
- workflow detail UI: `portal/src/pages/WorkflowDetail.tsx`
- team-scoped files UI/API: `portal/src/features/files`, `internal/server/portal/files.go`

### 2.1 What Is Already Good Enough

The current system is **not** missing its basic collaboration model anymore.

The repository already supports:

- shared team ownership
- issue creation and assignment
- workflow definition and execution
- issue-centric execution visibility
- direct Tier 1 conversation as the main user interface
- basic governance over shared automation assets

That means the next roadmap should optimize for:

- product closure
- user-visible meaning
- stronger state model
- safer long-running collaboration

not for another foundational rename or entity introduction cycle.

### 2.2 Main Remaining Gaps

The biggest gaps now are:

1. **Docs and product language lag the codebase.**
   - Some top-level docs still describe older `workspace / project / task` models or older portal assumptions.
2. **Results are still too execution-shaped.**
   - The system exposes runs, steps, and tasks well, but user-facing outcomes and artifacts are not yet the primary surface.
3. **The workspace is not yet the visible product center.**
   - Team files exist, but the user experience is not yet organized around "agent changes versioned workspace state".
4. **Versioned state and restore are still mostly a design intention.**
   - The product vision says the system should support reversible state, semantic change narrative, and hidden Git; this is not yet a first-class product capability.
5. **Governance has a first slice, but not a mature shared-team control layer.**
   - Audit/event history, team quota, approvals, and explicit policy boundaries remain deferred.

---

## 3. Strategic Direction

BuildMax should now move from:

- collaborative execution system

to:

- **versioned AI-native work system**

The product center should shift from:

- "an issue can run an agent or workflow"

to:

- "a team expresses intent, the agent changes workspace state, and the system explains, tracks, and can restore those changes"

This means the next roadmap should prioritize:

1. make outputs and outcomes first-class
2. make workspace state visible and meaningful
3. make change history reversible
4. strengthen human control over shared automation
5. extend workflow capability only after outcome and state layers are stronger

---

## 4. Design Principles

The next roadmap should follow these rules:

1. Keep the existing visible concepts stable: `Team`, `Conversation`, `Issue`, `Agent`, `Workflow`.
2. Do not introduce a second large naming reset while the current model is settling.
3. Make user-facing surfaces describe **outcomes and state changes**, not raw execution internals, whenever possible.
4. Treat the team file space as the beginning of the product's workspace body.
5. Hide infrastructure details like raw Git, run dirs, and storage keys behind user-facing concepts like timeline, snapshot, restore, and result.
6. Keep delivery additive and phase-based; prefer closing product loops over speculative architecture.
7. Defer ambitious graph orchestration or enterprise policy engines until the workspace/state model is stronger.

---

## 5. Recommended Delivery Order

Priority order:

1. Product and documentation reset
2. Outcome surface
3. Versioned workspace foundation
4. Restore and semantic timeline
5. Shared-team governance 2.0
6. Workflow automation 2.0

This order is intentional.

It starts by aligning product meaning, then makes outcomes visible, then adds the versioned-state layer that the long-term product vision depends on.

---

## 6. Phase-Based Execution

Like the previous roadmap, implementation should happen in focused phase documents under `design/`.

Each phase should have:

- goal
- in-scope items
- out-of-scope items
- code touch points
- implementation steps
- validation / acceptance checks
- open questions
- status

Recommended status values:

- `not_started`
- `in_progress`
- `blocked`
- `done`

### 6.1 Phase Status Table

| Phase | Name | Status | Phase Doc | Notes |
|------|------|--------|-----------|-------|
| 1 | Product And Docs Reset | `done` | `design/019-phase-1-product-and-docs-reset.md` | README and portal docs now reflect the current team/conversation/issue/workflow model, the active roadmap is explicit, and outdated design docs were moved under `design/archive/`. |
| 2 | Outcome Surface | `not_started` | `design/020-phase-2-outcome-surface.md` | Make issue/conversation surfaces show direct results and artifacts as first-class outputs, not only links to lower-level runs. |
| 3 | Versioned Workspace Foundation | `not_started` | TBD | Introduce a durable workspace change model, snapshot boundary, and hidden version engine plumbing. |
| 4 | Restore And Semantic Timeline | `not_started` | TBD | Let users inspect meaningful change history and restore prior workspace state without dealing with Git internals. |
| 5 | Shared-Team Governance 2.0 | `not_started` | `design/021-phase-5-team-quota-foundation.md` | Start with team-scoped quota foundation, then extend later toward audit/event visibility and human checkpoints where needed. |
| 6 | Workflow Automation 2.0 | `not_started` | TBD | Extend workflow capability only after the outcome/state/governance layers are stronger. |

---

## 7. Executable Plan

## Phase 1: Product And Docs Reset

### Goal

Bring product documentation, status documents, and top-level explanations into alignment with the actual current system.

### Why First

The codebase has moved faster than the docs.

Today there is visible mismatch between:

- current routes and entities
- older design language
- some README and portal docs

That slows down future implementation because every new phase starts by re-explaining current reality.

### In Scope

- update top-level README product model and current status
- update Portal docs that still describe mock-data or outdated assumptions
- refresh design references that still center old `workspace / project / task` language where that language is no longer authoritative
- close out old roadmap statuses where code is already landed
- define the official short product explanation for the current system

### Out Of Scope

- broad product redesign
- backend or frontend behavior changes beyond doc-alignment support work

### Desired Outcome

After this phase:

- a new contributor can understand the current product from docs without reading history
- the previous roadmap reads as complete for its intended scope
- the new roadmap becomes the clear entry point for planning

---

## Phase 2: Outcome Surface

### Goal

Make BuildMax present results and deliverables at the user-facing layer, not mainly through execution internals.

### Problem

Issue and workflow visibility is already good, but the product still tends to expose:

- run history
- step state
- task records

before it exposes:

- final outputs
- artifact previews
- key conclusions
- recommended next actions

### In Scope

- issue detail artifact/result aggregation
- conversation-facing result summaries linked to concrete output objects
- lightweight output panels for common artifact/result types
- clearer "what happened / what changed / what was produced" sections
- stable linking between issue/workflow/task results and surfaced outputs

### Out Of Scope

- full workspace versioning
- general document editor inside portal

### Desired Outcome

After this phase:

- issue and conversation pages feel like the place where users receive work results
- lower-level task/run pages become supporting infrastructure, not the main product destination

---

## Phase 3: Versioned Workspace Foundation

### Goal

Introduce the first durable workspace-state model that supports versioned changes behind the existing team file space.

### Problem

The product vision says:

- the workspace is the agent's body
- text state is primary
- every change is versioned

But the current system mostly has:

- team file upload/browse
- per-run execution directories
- artifacts

without a first-class workspace state transition model.

### In Scope

- define the persistent workspace state boundary for a team
- define snapshot/change-set semantics for agent-produced changes
- choose the hidden version engine boundary
  - Git-backed is the intended direction
- establish minimal metadata for one durable change record
  - actor
  - source issue/conversation/task/workflow
  - summary
  - created_at
  - restore pointer/reference
- connect worker completion to workspace-state recording

### Out Of Scope

- full user-facing restore UI
- branching/merging UX
- generalized code review product

### Desired Outcome

After this phase:

- the system has a clear internal unit of "workspace change"
- future timeline and restore features have a stable base

---

## Phase 4: Restore And Semantic Timeline

### Goal

Turn versioned workspace state into a user-facing timeline and restore experience.

### Problem

A hidden version engine only matters if the user can:

- understand what changed
- trust that history exists
- recover from bad outcomes

### In Scope

- team or conversation scoped activity timeline backed by real workspace change records
- semantic change summaries
- inspectable change detail
- restore a prior workspace state
- product wording that exposes "timeline" and "restore" rather than Git terminology

### Out Of Scope

- advanced branching model
- multi-restore conflict resolution UX
- full diff IDE experience

### Desired Outcome

After this phase:

- users can feel "I can always go back"
- BuildMax meaningfully matches the reversible-state principle in the portal vision

---

## Phase 5: Shared-Team Governance 2.0

### Goal

Add the next control layer needed for a real shared-team AI work system.

### Problem

The current governance slice is enough for:

- role-based management
- workflow lifecycle basics

but not yet enough for:

- durable accountability
- team-level capacity controls
- explicit review or checkpoint semantics when higher-trust actions are introduced

### In Scope

- durable audit/event model for important shared actions
- team-scoped quota direction and product wording
- explicit human checkpoint model where it naturally fits
- authorization hooks that future policy can build on cleanly

### Out Of Scope

- custom RBAC engine
- large enterprise org hierarchy
- generalized compliance suite

### Desired Outcome

After this phase:

- shared-team operation is easier to trust and govern
- future enterprise features can build on a clean base

---

## Phase 6: Workflow Automation 2.0

### Goal

Extend workflows from a useful linear v1 into a stronger automation layer once outcome/state/governance foundations are ready.

### Problem

Workflow v1 is deliberately small and correct for the current codebase, but long-term automation needs more than:

- linear agent-task steps
- manual trigger
- simple status lifecycle

### In Scope

- selected higher-value workflow capabilities such as:
  - retry/replay semantics
  - scheduled triggers
  - webhook/cron adapters where product-ready
  - approval/wait/checkpoint steps
  - limited conditional behavior if justified by real use cases
- stronger workflow run observability tied to surfaced outcomes and workspace changes

### Out Of Scope

- arbitrary workflow DSL ambitions
- visual node-graph builder unless justified later

### Desired Outcome

After this phase:

- workflows become durable operational assets for teams
- automation grows on top of a clearer product center instead of racing ahead of it

---

## 8. Immediate Recommendation

The next implementation conversation should start with **Phase 1: Product And Docs Reset** and keep it narrow.

Recommended first slice:

1. update top-level docs to the current product model
2. mark the previous roadmap complete where code is already landed
3. define the short official product explanation for the post-foundation BuildMax
4. identify the exact issue/conversation artifact/result gaps that will seed Phase 2

This keeps momentum high while creating a cleaner base for the more meaningful product phases that follow.
