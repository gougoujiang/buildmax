# Phase 1: Issue Uplift

## Status

- phase: `1`
- name: `Issue Uplift`
- status: `in_progress`
- roadmap: [design/010-team-task-workflow-roadmap.md](./010-team-task-workflow-roadmap.md)

---

## 1. Goal

Introduce `Issue` as a new user-facing work-management concept and corresponding system object.

In this phase, `Issue` should be treated like it is in systems such as Jira or GitHub:

- it describes what should be done
- it shows who is working on it
- it tracks current progress

This phase is intentionally scoped at the work-management layer. It does **not** try to redefine or replace the existing low-level execution model built around `task` and `task_run`.

This version should stay deliberately simple. The goal is to get the basic Issue system running first, not to design the final, complete work-management model.

---

## 2. Problem Statement

BuildMax currently has low-level execution objects:

- `task`
- `task_run`

These are useful for agent execution and runtime tracking, but they are not the right abstraction for user-facing work management.

They are too close to execution mechanics, while users need a higher-level concept for organizing work:

- what needs to be done
- who owns it
- what its current progress is

The new `Issue` concept solves that problem.

### 2.1 What Issue Is

`Issue` is a work-management object.

It should capture:

- title
- description
- assignee
- status
- priority
- creator
- timestamps

It should support a workflow like:

- create an issue
- assign it
- update progress
- review the current state of work

### 2.2 What Issue Is Not

In this phase, `Issue` is **not**:

- a rename of `task`
- a wrapper around `task_run`
- a direct execution record
- a low-level runtime concept

`task` and `task_run` remain separate low-level execution objects.

Any relationship between Issue and low-level execution can be designed later, but it is out of scope for this phase.

### 2.3 Why This Matters

Without Issue, the system lacks a stable business/work object.

That makes it hard to support:

- team collaboration
- workflow-oriented work planning
- visible work progress
- assignment semantics familiar to enterprise users

Issue is the work-management layer that the later phases will build upon.

### 2.4 MVP Principle For This Phase

Phase 1 should optimize for:

- minimal schema
- minimal APIs
- minimal UI
- low rule complexity

If a field or rule is not necessary to make the Issue loop usable, it should be deferred.

---

## 3. Desired Outcome

After Phase 1:

- `Issue` exists as a first-class domain concept
- users can create, list, inspect, and update issues
- issues support basic assignment and progress tracking
- issues use issue-first terminology in user-facing surfaces
- the issue layer is intentionally separate from low-level `task` / `task_run`

At the end of this phase, BuildMax should have a clear work-management surface even if there is not yet a detailed integration between Issue and execution records.

### 3.1 MVP Decision

For this phase, the target is:

- make Issue usable
- keep the schema small
- keep the API small
- keep the UI simple

Concretely, the MVP should be:

- one new `Issue` entity
- four basic APIs: create, list, get, update
- one simple list view
- one simple detail/edit view
- three basic statuses: `todo`, `in_progress`, `done`

---

## 4. In Scope

This phase includes the following work.

### 4.1 Introduce Issue As A New Domain Object

Add a new Issue object in the product/domain model.

Recommended MVP fields:

- `issue_id`
- `title`
- `description`
- `status`
- `assignee_kind`
- `assignee_id`
- `created_by`
- `created_at`
- `updated_at`

Deferred for later unless implementation is nearly free:

- `priority`
- `due_at`
- `completed_at`
- `assigned_by`
- `assigned_at`

### 4.2 Define Issue Status Model

Define issue-level progress states.

Recommended MVP values:

- `todo`
- `in_progress`
- `done`

This status model is purely for work management.

### 4.3 Define Assignment Model

Add issue assignment semantics.

Initial supported kinds:

- `person`
- `agent`

Recommended MVP fields:

- `assignee_kind`
- `assignee_id`

`workflow` assignment is out of scope for implementation in this phase.

### 4.4 Add Issue CRUD Surface

Provide issue-management APIs for:

- create issue
- list issues
- get issue detail
- update issue

Update operations should support at least:

- title
- description
- status
- assignee

### 4.5 Add Issue List / Detail UI

Add or adapt user-facing surfaces so BuildMax can present Issues as work-management objects.

Minimum target:

- issue list
- issue detail

The issue detail view should show:

- title
- description
- status
- assignee
- creator
- timestamps

### 4.6 Issue-First Product Terminology

Move user-facing language toward `Issue` in the relevant product surfaces introduced or touched in this phase.

Internal code names do not need to be renamed globally in this phase.

---

## 5. Out Of Scope

This phase does **not** include:

- changing the meaning of current `task` or `task_run`
- direct binding between Issue and task/task_run
- team entity introduction
- workflow CRUD or workflow execution
- workflow as assignee
- issue boards / kanban / advanced workflow UI
- enterprise RBAC / approvals / audit
- large-scale redesign of the whole portal information architecture

---

## 6. Current Code Touch Points

This phase introduces a new work-management object, so it should expect both additive work and selective reuse of existing patterns.

### 6.1 Storage / Entity Layer

Likely changes:

- `internal/storage/entity/models.go`
- `internal/storage/entity/interfaces.go`
- `internal/storage/entity/store.go`
- new issue-specific storage file(s)

Expected work:

- add `Issue` entity
- add issue store methods
- keep JSON/db field naming in snake_case

### 6.2 Application Layer

Likely changes:

- new issue application service under `internal/app/`

Expected work:

- create/update rules
- assignment validation
- minimal status update rules

### 6.3 Portal/API Layer

Likely changes:

- new issue handler file(s) under `internal/server/portal/`
- route registration updates

Expected work:

- create/list/get/update issue APIs
- issue request/response types

### 6.4 Portal Frontend

Expected work:

- issue list page or section
- issue detail page or section
- issue-first labels and copy in touched flows

### 6.5 Tests

Expected impact:

- entity store tests
- issue service tests
- portal issue handler tests

---

## 7. MVP Definitions

This section defines the concrete MVP shape we should implement in this phase.

### 7.1 Issue Entity Definition

Recommended MVP entity:

```go
type Issue struct {
    ID         uint    `json:"-"`
    IssueID    string  `json:"issue_id"`
    UserID     string  `json:"user_id"`
    Title      string  `json:"title"`
    Description string `json:"description"`
    Status     string  `json:"status"`
    AssigneeKind *string `json:"assignee_kind,omitempty"`
    AssigneeID   *string `json:"assignee_id,omitempty"`
    CreatedBy  string  `json:"created_by"`
    CreatedAt  int64   `json:"created_at"`
    UpdatedAt  int64   `json:"updated_at"`
}
```

Notes:

- keep JSON keys in snake_case
- keep the schema user-scoped in this phase
- `assignee_kind` and `assignee_id` can be nullable for unassigned issues
- `description` should default to empty string if not provided
- `status` should default to `todo`

Decision fixed for Phase 1:

- `Issue` is `user` scoped
- `Issue` is not conversation-scoped in this phase

### 7.2 Status Definition

MVP status values:

- `todo`
- `in_progress`
- `done`

MVP rule:

- keep transitions lightly validated or fully open in this phase
- do not introduce a heavy workflow/state machine yet

### 7.3 Assignment Definition

MVP assignee kinds:

- `person`
- `agent`

MVP rules:

- unassigned is allowed
- if `assignee_kind=person`, `assignee_id` should point to the current user in this phase
- if `assignee_kind=agent`, `assignee_id` should point to an agent owned by the same user
- no workflow assignment in this phase

---

## 8. MVP API Draft

The API should stay small and consistent with the current portal style.

### 8.1 Routes

Recommended routes:

- `POST /api/issues`
- `GET /api/issues`
- `GET /api/issues/{issue_id}`
- `PATCH /api/issues/{issue_id}`

These should be added alongside existing portal routes, not by overloading current task routes.

### 8.2 Create Issue

Route:

```text
POST /api/issues
```

Request body:

```json
{
  "title": "Prepare Q2 hiring plan",
  "description": "Draft the hiring goals and open roles for Q2."
}
```

Response body:

```json
{
  "id": "i_xxxxxxxxxxxxxxxxxxxx",
  "user_id": "u_xxxxxxxxxxxxxxxxxxxx",
  "title": "Prepare Q2 hiring plan",
  "description": "Draft the hiring goals and open roles for Q2.",
  "status": "todo",
  "assignee_kind": null,
  "assignee_id": null,
  "created_by": "u_xxxxxxxxxxxxxxxxxxxx",
  "created_at": 1712345678,
  "updated_at": 1712345678
}
```

MVP rules:

- `title` is required
- `description` is optional
- server fills defaults

### 8.3 List Issues

Route:

```text
GET /api/issues
```

Response body:

```json
{
  "issues": [
    {
      "id": "i_xxxxxxxxxxxxxxxxxxxx",
      "user_id": "u_xxxxxxxxxxxxxxxxxxxx",
      "title": "Prepare Q2 hiring plan",
      "description": "Draft the hiring goals and open roles for Q2.",
      "status": "todo",
      "assignee_kind": null,
      "assignee_id": null,
      "created_by": "u_xxxxxxxxxxxxxxxxxxxx",
      "created_at": 1712345678,
      "updated_at": 1712345678
    }
  ]
}
```

MVP rules:

- order by `updated_at DESC`
- pagination can be deferred unless it is nearly free

### 8.4 Get Issue

Route:

```text
GET /api/issues/{issue_id}
```

Response body:

- same shape as create response

### 8.5 Update Issue

Route:

```text
PATCH /api/issues/{issue_id}
```

Request body:

```json
{
  "title": "Prepare Q2 hiring plan",
  "description": "Draft the hiring goals, open roles, and timeline for Q2.",
  "status": "in_progress",
  "assignee_kind": "agent",
  "assignee_id": "a_xxxxxxxxxxxxxxxxxxxx"
}
```

MVP rules:

- partial update is allowed
- validate `status` if provided
- validate assignee if provided
- update `updated_at` on every successful patch

### 8.6 Response Naming

Recommended API response naming:

- use `id` externally in response objects
- persist as `issue_id` in storage

This follows the current portal convention used by agents and tasks.

---

## 9. MVP UI Flow

The UI should stay intentionally small.

### 9.1 Issue List

Minimum elements:

- first-level portal menu entry: `Issues`
- page title: `Issues`
- `New Issue` action
- a vertical list of issues

Each list item should show:

- title
- status
- assignee summary
- updated time

MVP behavior:

- `Issues` appears as a top-level navigation entry
- click list item to open issue detail
- no board view
- no grouping/filtering requirement in the first pass

### 9.2 New Issue Flow

Minimum interaction:

1. click `New Issue`
2. enter title
3. optionally enter description
4. save
5. land on issue detail

### 9.3 Issue Detail / Edit View

Minimum fields shown:

- title
- description
- status
- assignee
- created_at
- updated_at

Minimum actions:

- edit title
- edit description
- change status
- assign to self
- assign to agent

MVP presentation can be a simple form/detail panel. No advanced layout is required.

### 9.4 Assignee UX

For the first pass, keep assignee UX very simple:

- `Unassigned`
- `Me`
- one of my agents

No search UI or complex selector is required unless already convenient.

---

## 10. Storage And Service Draft

This section turns the MVP into concrete backend shapes that fit the current repository style.

### 10.1 Entity Store Interface Draft

Recommended new store interface in `internal/storage/entity/interfaces.go`:

```go
type IssueStore interface {
    CreateIssue(ctx context.Context, userID string, in CreateIssueInput) (*Issue, error)
    ListIssuesByUser(ctx context.Context, userID string, limit, offset int) ([]Issue, int, error)
    GetIssue(ctx context.Context, issueID string) (*Issue, error)
    UpdateIssue(ctx context.Context, issueID, userID string, in UpdateIssueInput) (*Issue, error)
}
```

Recommended supporting input types:

```go
type CreateIssueInput struct {
    Title       string
    Description string
}

type UpdateIssueInput struct {
    Title        *string
    Description  *string
    Status       *string
    AssigneeKind *string
    AssigneeID   *string
}
```

MVP notes:

- keep `UpdateIssue` ownership-aware like current `UpdateAgent`
- return updated entity directly from store for handler convenience
- `ListIssuesByUser` should match current conversation list style and return `([]Issue, total, error)`

### 10.2 Store Behavior

Recommended store behavior:

- `CreateIssue`
  - generate `issue_id`
  - set `status = todo`
  - set `created_by = userID`
  - set `created_at` and `updated_at`
- `ListIssuesByUser`
  - filter by `user_id`
  - order by `updated_at DESC`
- `GetIssue`
  - return `(nil, nil)` when not found
- `UpdateIssue`
  - verify issue belongs to `userID`
  - update only provided fields
  - always bump `updated_at`
  - return `(nil, nil)` for not found / wrong owner, following existing store conventions

### 10.3 Issue Application Service Draft

Recommended new service:

- `internal/app/issue/service.go`

Recommended shape:

```go
type Service struct {
    Issues entity.IssueStore
    Agents entity.AgentStore
}

type CreateIssueCmd struct {
    UserID      string
    Title       string
    Description string
}

type UpdateIssueCmd struct {
    UserID       string
    IssueID      string
    Title        *string
    Description  *string
    Status       *string
    AssigneeKind *string
    AssigneeID   *string
}
```

Main responsibilities:

- validate `title` on create
- validate allowed status values on update
- validate assignee combinations on update
- validate that assigned agent belongs to same user

### 10.4 MVP Validation Rules

Keep rules intentionally small:

- create:
  - `title` required
- update:
  - `status` must be one of `todo`, `in_progress`, `done` if provided
  - `assignee_kind` must be `person` or `agent` if provided
  - if `assignee_kind=person`, `assignee_id` must equal current `userID`
  - if `assignee_kind=agent`, `assignee_id` must be an agent owned by current `userID`
  - if clearing assignee, allow both `assignee_kind` and `assignee_id` to become null

Do not add stricter business rules in this phase.

---

## 11. Handler And API Structure Draft

This section aligns the issue API with current portal handler patterns.

### 11.1 Portal Route Registration Draft

Recommended additions in `internal/server/portal/register.go`:

```go
mux.HandleFunc("GET /api/issues", h.listIssuesHandler)
mux.HandleFunc("POST /api/issues", h.createIssueHandler)
mux.HandleFunc("GET /api/issues/{issue_id}", h.getIssueHandler)
mux.HandleFunc("PATCH /api/issues/{issue_id}", h.patchIssueHandler)
```

### 11.2 Response Shape Draft

Recommended issue response:

```go
type IssueResponse struct {
    ID           string  `json:"id"`
    UserID       string  `json:"user_id"`
    Title        string  `json:"title"`
    Description  string  `json:"description"`
    Status       string  `json:"status"`
    AssigneeKind *string `json:"assignee_kind,omitempty"`
    AssigneeID   *string `json:"assignee_id,omitempty"`
    CreatedBy    string  `json:"created_by"`
    CreatedAt    int64   `json:"created_at"`
    UpdatedAt    int64   `json:"updated_at"`
}
```

Recommended list response:

```go
type issueListResponse struct {
    Issues []IssueResponse `json:"issues"`
    Total  int             `json:"total"`
}
```

### 11.3 Request Shape Draft

Recommended request types:

```go
type createIssueRequest struct {
    Title       string `json:"title"`
    Description string `json:"description"`
}

type patchIssueRequest struct {
    Title        *string `json:"title"`
    Description  *string `json:"description"`
    Status       *string `json:"status"`
    AssigneeKind *string `json:"assignee_kind"`
    AssigneeID   *string `json:"assignee_id"`
}
```

### 11.4 Handler Behavior Draft

Recommended handler pattern:

- `listIssuesHandler`
  - authenticate user
  - call issue store/service
  - return `issueListResponse`
- `createIssueHandler`
  - authenticate user
  - decode request
  - call issue service
  - return `201 Created`
- `getIssueHandler`
  - authenticate user
  - load issue
  - ownership-check via `user_id`
  - return `404` if not found
- `patchIssueHandler`
  - authenticate user
  - decode request
  - call issue service
  - return updated issue

### 11.5 Error Behavior Draft

Recommended MVP error behavior:

- `400`
  - missing title
  - invalid status
  - invalid assignee_kind
  - invalid assignee_id for kind
- `404`
  - issue not found
- `500`
  - store/service internal errors

This matches the current portal style and keeps the first pass predictable.

---

## 12. Coding Task Breakdown

Recommended implementation breakdown:

### Task 1: Add Issue Entity And Store

- add `Issue` model to `internal/storage/entity/models.go`
- add `IssueStore` to `internal/storage/entity/interfaces.go`
- implement issue store methods in new entity file(s)
- wire auto-migrate in store/bootstrap

### Task 2: Add Issue Application Service

- create `internal/app/issue/service.go`
- add create/update commands
- add MVP validation

### Task 3: Add Portal Issue Handlers

- add new issue handler file under `internal/server/portal/`
- add request/response types
- register routes in `register.go`

### Task 4: Add Portal Issue UI

- add issue list page/section
- add issue detail/edit page/section
- connect to new issue APIs

### Task 5: Add Tests

- issue store tests
- issue service tests
- issue handler tests

### Task 6: Smoke And Manual Verification

- create issue
- list issues
- update issue status
- assign to self
- assign to agent

---

## 13. Proposed Implementation Plan

Recommended execution order inside this phase:

### Step 1: Define The Issue Entity

Introduce `Issue` as a new persisted object with the minimum fields needed for work management.

The first design pass should keep the schema intentionally small and stable.

Recommended MVP fields only:

- `issue_id`
- `title`
- `description`
- `status`
- `assignee_kind`
- `assignee_id`
- `created_by`
- `created_at`
- `updated_at`

### Step 2: Define Store Interfaces

Add issue store operations for:

- create
- list
- get
- update

Keep the API narrow and explicit.

### Step 3: Add Issue Application Service

Introduce an application-layer service to own:

- creation defaults
- light status validation
- assignment validation

Do not push business rules directly into handlers.

### Step 4: Add Portal Issue APIs

Add endpoints for:

- create issue
- list issues
- get issue
- update issue

Exact route shape can be finalized during implementation, but should stay user-facing and issue-oriented.

Recommended MVP route shape:

- `POST /api/issues`
- `GET /api/issues`
- `GET /api/issues/{issue_id}`
- `PATCH /api/issues/{issue_id}`

### Step 5: Add Issue UI

Expose the new Issue object in the portal with at least:

- issue list
- issue detail

The first version can be simple; it does not need a complex board or workflow UI yet.

Recommended MVP UI:

- a simple issue list
- a simple issue detail/edit view

### Step 6: Align User-Facing Copy

In touched surfaces, use `Issue` consistently in user-facing labels and descriptions.

### Step 7: Validate The Core Loop

Verify the basic work-management loop:

1. create issue
2. assign issue
3. update status
4. inspect issue detail

---

## 14. Acceptance Checks

This phase is complete when all of the following are true:

1. `Issue` exists as a first-class persisted domain object
2. users can create, list, inspect, and update issues
3. issue status supports basic work-management progress
4. issue assignment supports:
   - person
   - agent
5. issue detail clearly shows work-management information
6. the new issue layer is not implemented as a mere rename of `task`
7. the implementation is simple enough to run without requiring team/workflow integration

---

## 15. Validation Plan

Validation should include:

- storage-level tests for issue persistence
- issue service tests for defaults and validation
- portal handler tests for create/list/get/update flows
- manual portal verification for issue list and issue detail

Suggested manual checklist:

1. create an issue
2. verify default status
3. assign the issue to a person
4. assign the issue to an agent
5. update the issue status
6. update title/description
7. reload issue list and detail to confirm persistence

---

## 16. Open Questions

These should be resolved during implementation:

1. What is the minimal issue schema we want to commit to in v1?
2. What exact route shape should issue APIs use?
3. Should status transitions be unrestricted in v1, or lightly validated?

Resolved decisions:

- `Issue` is `user` scoped in Phase 1
- `Issues` is a first-level portal menu entry because it is a top-level model

---

## 17. Recommended First Slice In This Phase

If we want the safest implementation start inside Phase 1, do this first:

1. add Issue entity + store
2. add issue service
3. add create/list/get/update issue APIs
4. add minimal issue list/detail UI
5. align user-facing wording in touched issue flows

This slice creates the work-management layer cleanly before any future linkage to execution.

Recommended MVP defaults:

- default status: `todo`
- default assignee: unassigned
- no priority field in the first pass unless it is very cheap to add
- no strict workflow/state machine rules beyond basic validation

---

## 18. Completion And Handoff

When this phase is done:

- update this document status to `done`
- update the Phase 1 row in [design/010-team-task-workflow-roadmap.md](./010-team-task-workflow-roadmap.md)
- record carry-over items for later phases

Likely carry-over topics:

- Issue ownership model after Team introduction
- future relationship between Issue and low-level execution records
- workflow-driven issue execution in later phases
