# Team / Issue / Workflow Roadmap

## 1. Purpose

This document turns the recent product discussion into an executable roadmap, using the **current codebase as the source of truth**.

It focuses on three feedback themes:

- support workflow
- support team collaboration
- support task management with visible process

The goal is **not** to jump straight into a full enterprise platform. The goal is to evolve BuildMax from a user-scoped agent runtime into a work system with a stable user mental model and a staged implementation plan.

---

## 2. Current Codebase Baseline

As of the current repository state, the main product/runtime model is:

```text
user
├── agents
├── conversations
│   ├── messages
│   └── tasks
│       └── task_runs
└── files / webhook_keys
```

Key facts from the code:

- `agent` is a user-scoped persona resource.
  - `internal/storage/entity/models.go`
  - `internal/storage/entity/agent.go`
- `conversation` is the Tier 1 container for portal turns.
  - `internal/storage/entity/models.go`
  - `internal/app/conversation/service.go`
- `task` already exists as a durable execution-oriented object used by the current runtime and portal.
  - It has `title`, `input`, `created_by`, optional `agent_id`, and denormalized latest status/output.
  - `internal/storage/entity/models.go`
  - `internal/storage/entity/task.go`
- `task_run` is the actual execution unit.
  - It is what scheduler/worker consume and update.
  - `internal/storage/entity/models.go`
  - `internal/storage/entity/task_run.go`
  - `internal/server/worker/handlers.go`
- the portal already supports:
  - conversations
  - task creation under a conversation
  - task follow-up runs
  - agent creation and agent-backed task creation
  - `internal/server/portal/register.go`
  - `internal/server/portal/tasks.go`
  - `internal/server/portal/agents.go`
- the conversation service already knows how to:
  - start a task
  - list recent tasks
  - inspect one task
  - continue a task
  - provide agent summaries to the LLM
  - `internal/app/conversation/service.go`

### 2.1 What This Means

BuildMax already has a low-level execution model:

- `task` = low-level execution container in the current system
- `task_run` = one execution attempt

That is useful infrastructure, but it is **not** the same thing as the user-facing work-management concept we now want to introduce.

The main gaps are:

- no `issue` entity as a first-class work-management object
- no `team` entity or team ownership boundary
- no `workflow` entity or execution-plan abstraction
- no assignment model at the work-management layer for people vs agent vs workflow
- no issue-first progress / process UI

---

## 3. User-Facing Mental Model

User-facing concepts should stay small and stable:

- `Team`: shared work space
- `Issue`: one concrete piece of work
- `Agent`: a digital member that can take work
- `Workflow`: a reusable way to execute repeated work

Recommended user explanation:

> In BuildMax, work happens inside a Team. A Team contains people, Agents, Workflows, and Issues. When something needs to get done, create an Issue and assign it to a person, an Agent, or a Workflow. BuildMax tracks progress, execution, and results.

### 3.1 User Concept vs Internal Type

From this point onward in this document:

- `Issue` = the user-facing concept
- `task` / `task_run` = the current low-level execution types in the codebase

This naming split is intentional. It avoids a collision between:

- product language used in UI/docs
- existing server/storage/runtime code

Important clarification:

- `Issue` is a new concept for managing work
- `task` is an existing low-level concept for agent execution
- these concepts may relate in future architecture, but they should **not** be treated as the same object in this phase

### 3.2 Important Modeling Rule

For the user experience:

- an issue can be assigned to a person, an agent, or a workflow

For the internal model:

- `agent` and `workflow` should **not** be collapsed into one entity yet

Reason:

- `agent` is an autonomous executor
- `workflow` is an explicit execution plan

The UI can unify the assignment experience without forcing the data model to pretend both concepts are the same.

---

## 4. Design Principles

The roadmap should follow these rules:

1. Keep the core visible concepts stable: `Team`, `Issue`, `Agent`, `Workflow`.
2. Keep the current `task` + `task_run` split as the low-level execution model instead of overloading it into the user work-management model.
3. Introduce `team` as a strong backend boundary, but keep early personal UX lightweight.
4. Treat personal space as a default single-member team.
5. Add enterprise complexity gradually; do not front-load RBAC, approvals, or org trees.
6. Prefer additive migrations over broad rewrites where possible.

---

## 5. Strategic Direction

The current product is moving from:

- agent runtime

to:

- work orchestration system

That changes the center of the product:

- before: “run an agent”
- after: “organize work across people and agents”

So the roadmap should prioritize:

1. make `Issue` the user-facing center
2. add assignment and process visibility
3. add `team` as the ownership and collaboration boundary
4. add `workflow` as reusable execution structure

---

## 6. Recommended Delivery Order

Priority order:

1. Issue uplift
2. Team foundation
3. Workflow foundation
4. Issue flow visualization
5. Enterprise governance features

This order intentionally starts with `Issue`, because it is the missing work-management layer between:

- today’s execution model
- tomorrow’s collaborative work system

---

## 7. Implementation Strategy

This roadmap is the top-level planning document.

Execution should follow a strict phase-based workflow so that each implementation step has:

- a clear scope
- an isolated implementation conversation
- an explicit progress record

### 7.1 Phase-Based Execution

We implement this roadmap in phases.

For each phase:

- create a dedicated phase document under `design/`
- use a dedicated implementation conversation for that phase only
- use the roadmap document plus the phase document as the main context for that phase

This keeps each implementation conversation focused while preserving continuity across phases.

### 7.2 Phase Documents

Each phase should have its own document, for example:

- `design/011-phase-0-terminology-and-boundary-alignment.md`
- `design/012-phase-1-issue-uplift.md`
- `design/013-phase-2-team-foundation.md`

The exact numbering can follow the next available design doc number when the phase starts.

Each phase document should contain at least:

- goal
- in-scope items
- out-of-scope items
- current code touch points
- implementation steps
- validation / acceptance checks
- open questions or follow-ups
- current status

### 7.3 Conversation Strategy

Each phase is implemented in a separate conversation.

Inside a phase conversation, the working context should be:

- this roadmap document
- the current phase document
- the current repository state

That means the phase conversation does not need to reconstruct the whole roadmap from memory. It can rely on:

- the top-level roadmap for product direction and ordering
- the phase document for concrete scope and progress

### 7.4 Status Tracking

After a phase is completed, its status must be written back to this roadmap document.

This is required so that a future phase conversation can quickly determine:

- what has already been completed
- what document to read for the completed phase
- what the next phase should start from

Recommended status values:

- `not_started`
- `in_progress`
- `blocked`
- `done`

### 7.5 Phase Status Table

| Phase | Name | Status | Phase Doc | Notes |
|------|------|--------|-----------|-------|
| 0 | Terminology And Boundary Alignment | `not_started` | TBD | |
| 1 | Issue Uplift | `done` | `design/011-phase-1-issue-uplift.md` | MVP landed: user-scoped Issue entity, CRUD API, top-level Issues menu, list view with pagination, shared create/detail modal, basic assignee/status editing. Issue remains separate from low-level task/task_run. |
| 2 | Team Foundation | `done` | `design/012-phase-2-team-foundation.md` | Team is now the ownership boundary for issues, agents, conversations, and tasks. Default personal team (`My Space`) is created automatically, members can be managed from the portal, and APIs use explicit `/api/teams/{team_id}/...` routes instead of current-team header context. |
| 3 | Workflow Foundation | `done` | `design/013-phase-3-workflow-foundation.md` | Workflow v1 is now landed as a lightweight, team-scoped linear step model with explicit manual trigger support, reusable task/task_run-backed execution, issue assignment to `workflow`, a dedicated workflow detail page, and standalone workflow run inspection. |
| 4 | Issue Flow Visualization | `done` | `design/014-phase-4-issue-flow-visualization.md` | Issue detail now has issue-centric flow data, execution summary, timeline, workflow step state, workflow run history, and agent run sequence. Direct artifact/result aggregation remains deferred. |
| 5 | Enterprise Governance | `not_started` | TBD | |

### 7.6 Off-Roadmap Cleanup Log

| Date | Area | Doc | Notes |
|------|------|-----|-------|
| 2026-04-26 | Portal navigation conversation/recent refactor | `design/015-portal-navigation-conversation-recent-refactor.md` | Collapsed the old sidebar `Recent` concept into `Conversations`, removed the top-level Recent menu, reordered primary navigation to `Conversations`, `Issues`, `Workflows`, `Agents`, kept recent conversation history under the Conversations page, and renamed internal `chats` / `Tasks` route/page naming to `conversations` / `Conversations`. |

### 7.7 Update Rule After Each Phase

When a phase finishes:

1. update the phase document status to reflect completion
2. update the phase row in this roadmap
3. add the phase document path into the `Phase Doc` column
4. note any important carry-over items for the next phase in the `Notes` column

This roadmap should remain the single summary view of phase progress across conversations.

---

## 8. Executable Plan

## Phase 0: Terminology And Boundary Alignment

### Goal

Align product language before introducing new persistent entities.

### Why First

The current repo uses `task` for a durable object with `task_run` executions underneath. That is already close to the desired business concept. We should explicitly lean into that instead of replacing it with a second “task” abstraction.

### Deliverables

- document the new user-facing language in product/docs:
  - `Issue` = user-facing work item
  - `task` = current low-level execution record
  - `task_run` = execution attempt
  - `agent` = digital member
  - `workflow` = reusable execution plan
  - `team` = collaboration boundary
- audit internal/server/portal labels that still make `task` look purely execution-scoped
- define naming for assignment targets:
  - `person`
  - `agent`
  - `workflow`

### Current-Code Impact

- mostly docs, API contract review, and UI text changes
- no mandatory schema migration yet

### Exit Criteria

- product and engineering can describe the model consistently
- product and engineering can describe `Issue` and `task` as distinct concepts without conflating them

---

## Phase 1: Issue Uplift

### Goal

Introduce `Issue` as a new user-facing work-management object.

### Why This Is First

This provides immediate value even before team/workflow land:

- users can create issues
- assign them
- track progress
- manage work independent of low-level agent execution

### Scope

#### 1. Add issue-level business state

Issue state should model work-management progress, for example:

- `todo`
- `in_progress`
- `blocked`
- `done`
- `canceled`

This is distinct from low-level execution status on `task` / `task_run`.

#### 2. Add assignment model

Add issue assignment fields on the new Issue object:

- `assignee_kind`: `person` | `agent` | `workflow`
- `assignee_id`
- optional `assigned_by`
- optional `assigned_at`

Phase 1 only needs:

- assign to self
- assign to agent

Workflow assignment can be schema-ready but feature-flagged until Phase 3.

#### 3. Add issue detail fields needed for management

Recommended first batch:

- `description` or richer `goal`
- `status`
- `priority`
- `assignee_kind`
- `assignee_id`

Optional if low-cost:

- `due_at`
- `completed_at`

#### 4. Add issue list/detail management surface

Expose on the issue detail page:

- title
- description / goal
- assignee
- status
- priority
- created_by
- created_at
- updated_at

### API Work

- add Issue CRUD APIs
- add issue update endpoint for:
  - assignment
  - status
  - title/description edits

### UI Work

- add issue list/detail UI
- issue list becomes a work queue / tracker
- issue detail shows:
  - goal
  - assignee
  - status
  - priority

### Code Areas

- `internal/storage/entity/models.go`
- new issue storage/service/portal areas to be introduced in this phase
- portal issue list/detail pages

### Exit Criteria

- an Issue exists as a first-class user-facing object
- an issue can be assigned to self or an agent
- issue management works without depending on low-level task/task_run execution records

---

## Phase 2: Team Foundation

### Goal

Introduce `team` as the real ownership and collaboration boundary without breaking the personal-user experience.

### Why This Comes Before Workflow

Workflow, agent ownership, and issue collaboration all become much cleaner once they live inside a team boundary.

### Scope

#### 1. Add team entities

Add:

- `team`
- `team_member`

Minimum role set:

- `owner`
- `member`

#### 2. Introduce default personal team

Each user gets a default personal team.

This keeps the user model stable:

- personal users continue to feel like they have “My Space”
- internally, resources can begin moving under `team`

#### 3. Move resource ownership to team gradually

Recommended target:

```text
team
├── members
├── agents
├── workflows
├── conversations
└── tasks
```

Migration direction:

- `agent`: add `team_id`, keep `created_by`
- `conversation`: add `team_id`, keep `created_by`
- `task`: add `team_id`, keep `created_by`

Do not try to fully delete `user_id`-based assumptions in one shot. Introduce `team_id`, backfill default personal teams, then migrate handlers and stores progressively.

#### 4. Team-aware APIs

Add team APIs for:

- list/create team
- invite/list members
- switch current team context

Do not add full RBAC yet.

### UI Work

Phase 2 UX should remain intentionally light:

- default landing can still feel like “My Space”
- add team switcher only when multiple teams exist
- allow inviting members
- show issue assignees from team members and team agents

### Code Areas

- `internal/storage/entity/models.go`
- stores for team/team_member
- auth/context resolution in portal/server
- portal team selector and member management

### Exit Criteria

- every working resource belongs to a team
- personal usage still works through an implicit single-member team
- tasks and agents can exist in a shared team context

---

## Phase 3: Workflow Foundation

### Goal

Introduce reusable execution plans for repeated work.

### Important Constraint

Do **not** start with a heavy visual builder.

Start with a small but usable workflow model that the current runtime can execute.

### Scope

#### 1. Add workflow entity

Minimal first version:

- `workflow_id`
- `team_id`
- `name`
- `description`
- `definition`
- `created_by`

The definition can start as structured JSON/YAML-like steps stored in text/JSON.

#### 2. Choose a simple initial execution model

Recommended v1:

- step-based workflow
- linear sequence first
- optional future branch/condition support later

Example conceptual shape:

```yaml
steps:
  - type: agent_task
    target_agent_id: a_xxx
    prompt: collect the source data
  - type: agent_task
    target_agent_id: a_yyy
    prompt: summarize and produce the final report
```

#### 3. Support issue assignment to workflow

When an issue is assigned to a workflow:

- the issue remains the user-facing business object
- workflow execution produces one or more `task_run` or workflow-step runs under the hood

Recommended design choice:

- keep internal `task` as the backing record for the top user object, `Issue`
- introduce workflow execution records under it rather than inventing a sibling business object

#### 4. Keep agent and workflow separate internally

The UI may let the user pick “assignee” from one list, but the backend should still know:

- agent assignment = dynamic executor
- workflow assignment = plan executor

### API Work

- CRUD for workflows
- assign issue to workflow
- fetch workflow definition and recent executions

### Runtime Work

Need a new execution component that can:

- interpret workflow definition
- dispatch step work through existing task/task_run or agent runtime primitives
- persist step state

### Code Areas

- new workflow store/service
- `internal/app/task` integration for workflow assignment
- executor extensions for workflow runs

### Exit Criteria

- a team can define a reusable workflow
- an issue can be assigned to a workflow
- workflow progress can be inspected step by step

---

## Phase 4: Issue Flow Visualization

Status: `done` for current scope; direct artifact/result aggregation is deferred. See [design/014-phase-4-issue-flow-visualization.md](./014-phase-4-issue-flow-visualization.md).

### Goal

Make work execution visible and inspectable.

This directly answers the feedback about seeing the issue process.

### Scope

Show on the issue detail page:

- business status
- current assignee
- execution timeline
- run history
- workflow step status when applicable
- artifacts/results

Recommended views:

- timeline view
- flow/step view

For agent-assigned issues:

- show run sequence and outputs

For workflow-assigned issues:

- show per-step state:
  - pending
  - running
  - done
  - failed
  - blocked

### Why This Matters

This is where BuildMax starts to feel like a work system instead of a black-box AI runner.

### Exit Criteria

- users can answer “what is happening with this issue?” without reading raw logs

---

## Phase 5: Enterprise Governance

### Goal

Add the minimum governance capabilities needed for larger organizations.

### Scope

Potential additions:

- richer roles and permissions
- audit log
- approvals / review checkpoints
- quotas and policy by team
- workflow governance and publishing rules

### Important Constraint

These should be gated and gradual. They should not shape the first team/workflow versions more than necessary.

### Exit Criteria

- enterprise teams can safely operate shared agents and workflows without changing the base user model

---

## 9. Priority Backlog

This is the recommended priority-ordered work list.

### P0

- lock terminology and update product copy around `Issue`, internal `task`, `task_run`, `agent`, `workflow`, `team`
- decide the exact issue business-status vocabulary
- decide assignment vocabulary and API shape

### P1

- add issue business status
- add issue assignment fields
- add issue update API
- add run history API on issue detail
- update portal UI from `Task` wording toward `Issue` wording where user-facing

### P2

- add `team` and `team_member`
- create default personal teams
- add `team_id` to core resources
- migrate task/agent/conversation ownership checks to team-aware logic
- add minimal team switch/invite UI

### P3

- add workflow entity and CRUD
- define workflow v1 schema
- add issue assignment to workflow
- implement workflow execution engine

### P4

- build issue flow/timeline UI
- surface workflow step state and agent-run trace clearly

### P5

- introduce enterprise governance features selectively

---

## 10. Suggested First Implementation Slice

If we want the smallest high-value next step, start here:

1. keep the current `task` table as the internal backing for `Issue`
2. add issue business status and assignment fields on `task`
3. expose a stronger issue detail API and page
4. allow assignment to:
   - self
   - existing agent
5. delay true team/workflow execution until that first task-management loop feels solid

This slice is intentionally conservative:

- it delivers obvious user value quickly
- it preserves current scheduler/worker assumptions
- it prepares the schema and UI for later team/workflow expansion

---

## 11. Risks And Mitigations

### Risk 1: Overloading `task`

If we keep adding fields without clarifying semantics, `task` can become confusing.

Mitigation:

- treat `Issue` as the business object in product language
- treat `task` as the internal backing record for `Issue`
- treat `task_run` as the execution object
- document that split clearly in code and API

### Risk 2: Team migration churn

Moving from `user` ownership to `team` ownership touches many handlers and stores.

Mitigation:

- add `team_id` first
- create default personal team
- migrate handlers in stages
- keep `created_by` for audit continuity

### Risk 3: Workflow overreach

A big builder too early will slow everything down.

Mitigation:

- start with simple step-based workflow definition
- no drag-and-drop requirement for v1

### Risk 4: User confusion around assignee types

If people, agents, and workflows behave too differently, assignment will feel inconsistent.

Mitigation:

- unify assignment UX
- keep internal engines separate
- always show issue owner, assignee, and latest execution state explicitly

---

## 12. Recommendation

The next milestone should be:

**Issue Uplift**

Reason:

- it directly addresses the feedback about task management
- it uses the strongest existing code foundation
- it creates the cleanest bridge to team and workflow

After that, the recommended path is:

**Team Foundation → Workflow Foundation → Issue Flow Visualization**

That sequence keeps the model stable while gradually opening collaborative and enterprise capabilities.
