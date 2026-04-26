# Phase 3: Workflow Foundation

## Status

- phase: `3`
- name: `Workflow Foundation`
- status: `done`
- roadmap: [design/010-team-task-workflow-roadmap.md](./010-team-task-workflow-roadmap.md)
- depends_on: [design/012-phase-2-team-foundation.md](./012-phase-2-team-foundation.md)
- started_at: `2026-04-26`
- completed_at: `2026-04-26`

---

## 1. Goal

Introduce `workflow` as a first-class team-scoped concept for reusable execution plans.

After this phase, a team should be able to define a simple reusable workflow, assign an issue to that workflow, and inspect workflow execution progress step by step.

This phase is intentionally about establishing the first durable workflow model and execution path. It is not about advanced visual design, enterprise governance, or a complex general-purpose orchestration engine.

---

## 2. Problem Statement

Phase 1 introduced `Issue` as the user-facing work object.

Phase 2 introduced `Team` as the ownership boundary for issues, agents, conversations, and tasks.

The next missing layer is reusable execution structure.

Today, BuildMax can do the following:

- create team-scoped issues
- create team-scoped agents
- create conversations and tasks
- run background execution through `task` and `task_run`

But it still cannot do the following:

- define a reusable multi-step execution plan
- assign an issue to that plan
- persist workflow-specific execution state
- show progress at the workflow-step level

Without a workflow layer, BuildMax remains limited to one-off execution and direct assignment. That is not enough for repeated team processes such as:

- collect -> analyze -> summarize
- research -> draft -> review
- gather artifacts -> transform -> publish

Phase 3 solves this by adding a small but durable workflow model that reuses the existing runtime foundation.

---

## 3. Core Decisions

### 3.1 Workflow Is A Separate Domain Object

Decision fixed for Phase 3:

- `workflow` is its own team-scoped entity
- `workflow` is not an alias of `agent`
- `workflow` is not a rename of `task`

Reason:

- `agent` is an executor persona
- `workflow` is an execution plan
- `task` / `task_run` are low-level execution records

These roles are related but not interchangeable.

### 3.2 Workflow V1 Uses A Linear Step Model

Decision fixed for Phase 3:

- workflow v1 is step-based
- workflow v1 is linear
- no branching or conditional graph execution in this phase

Reason:

- linear steps are enough to validate the product model
- they map cleanly to the current `task` / `task_run` runtime
- they minimize implementation risk

### 3.3 Workflow Execution Reuses Existing Task Infrastructure

Decision fixed for Phase 3:

- do not build a second unrelated execution runtime
- workflow steps should dispatch work through existing `task` and `task_run` primitives

Reason:

- current scheduler / worker / artifact flow already works
- phase 3 should extend that foundation instead of bypassing it
- this keeps future observability and quota logic more coherent

### 3.4 Workflow Assignment Extends Issue Assignment

Decision fixed for Phase 3:

- `Issue` assignment gains `workflow` as a valid assignee kind
- the issue remains the user-facing work object
- workflow execution is subordinate execution state under the issue

This preserves the product model introduced in earlier phases:

- the user tracks an issue
- the system may execute a workflow for that issue

### 3.5 Keep Workflow Authoring Lightweight

Decision fixed for Phase 3:

- no visual builder
- no nested reusable sub-workflows
- no policy or approval gates
- no general DSL ambitions

The definition format should be structured and durable, but small.

### 3.6 Manual Trigger Is Required In V1

Decision fixed for Phase 3:

- workflow v1 must support a manual trigger
- manual trigger is the primary MVP trigger path
- this manual trigger should be available even when issue assignment to workflow already exists

Reason:

- it gives us a simple and testable execution path during implementation
- it matches the current product habit where agent-backed execution can be started explicitly
- it avoids coupling workflow validation to auto-start behavior too early

---

## 4. Desired Outcome

After Phase 3:

- a `workflow` exists as a first-class team-scoped resource
- a workflow can define a reusable linear sequence of steps
- an issue can be assigned to a workflow
- the backend can create and track workflow executions
- each workflow step has inspectable execution state
- step execution reuses the existing `task` / `task_run` foundation

At the end of this phase, BuildMax should have a stable bridge between:

- issue-level work management
- team-scoped collaboration
- repeated structured execution

### 4.1 MVP Decision

For this phase, the target is:

- one team-scoped workflow entity
- one durable workflow definition format
- one workflow execution model
- issue assignment to workflow
- inspectable workflow run / step state

Concretely, the MVP should be:

- `workflow`
- `workflow_run`
- `workflow_step_run`
- workflow CRUD API
- issue assignment support for `workflow`
- a minimal executor that runs linear steps through existing task creation and task runs

---

## 5. In Scope

This phase includes the following work.

### 5.1 Add Workflow Entity

Add a new team-scoped `workflow` entity.

Recommended MVP fields:

- `workflow_id`
- `team_id`
- `name`
- `description`
- `definition`
- `created_by`
- `created_at`
- `updated_at`

### 5.2 Define Workflow V1 Definition Format

Add a small structured definition format for workflow steps.

Recommended v1 shape:

- ordered list of steps
- each step has stable step id / key
- each step uses one execution type first: `agent_task`

Recommended conceptual example:

```yaml
steps:
  - step_id: collect
    type: agent_task
    target_agent_id: a_xxx
    prompt: collect the source data
  - step_id: summarize
    type: agent_task
    target_agent_id: a_yyy
    prompt: summarize findings and produce the final report
```

The exact persisted encoding can be JSON text in the database even if docs show YAML-like examples.

### 5.3 Add Workflow Execution Records

Add durable execution records for workflow-level progress.

Recommended MVP entities:

- `workflow_run`
- `workflow_step_run`

Recommended semantics:

- `workflow_run` = one execution attempt of a workflow for a specific issue
- `workflow_step_run` = one tracked step execution inside that run

### 5.4 Support Issue Assignment To Workflow

Extend issue assignment so that:

- `assignee_kind` can be `workflow`
- `assignee_id` can refer to a workflow in the same team

This phase only needs one workflow assignee model. It does not need mixed person+workflow assignment or approval routing.

### 5.5 Add Minimal Workflow APIs

Provide a minimal set of team-scoped workflow APIs:

- list workflows
- create workflow
- get workflow
- update workflow

Recommended additional read APIs:

- list workflow runs
- get workflow run detail with step runs

### 5.6 Add Minimal Workflow Execution Trigger

Provide a first execution trigger path for workflows.

Recommended MVP trigger:

- explicit manual "run workflow" action

- Recommended first target:

- manual trigger from workflow detail
- manual trigger from issue detail when the issue is assigned to that workflow

Deferred / optional later:

- auto-start on assignment

This phase should treat manual trigger as the required testable path. The execution model may remain compatible with future auto-start, but auto-start is not required for phase 3.

### 5.7 Add Minimal Workflow Visibility

Portal UX should stay intentionally light.

Minimum target:

- workflow list
- workflow detail
- issue detail shows workflow assignee
- workflow run detail shows step states

---

## 6. Out Of Scope

This phase does **not** include:

- visual workflow builder
- conditional branching / DAG execution
- loop / retry policy authoring
- workflow templates marketplace
- nested workflows / sub-workflows
- cross-team workflow sharing
- approvals, review gates, or enterprise governance
- advanced issue timeline UI beyond minimal execution visibility
- replacing the existing `task` / `task_run` execution runtime

---

## 7. Current Code Touch Points

Phase 3 is additive, but it must integrate with existing issue, team, task, and executor code.

### 7.1 Storage / Entity Layer

Likely changes:

- `internal/storage/entity/models.go`
- `internal/storage/entity/interfaces.go`
- new workflow-related entity files
- `internal/storage/entity/issue.go`
- possibly `internal/storage/entity/task.go` for workflow-linked task creation metadata

### 7.2 Application Layer

Likely changes:

- new workflow service under `internal/app/`
- issue service assignment validation
- task service integration for workflow step dispatch

Likely files:

- new `internal/app/workflow/service.go`
- `internal/app/issue/service.go`
- `internal/app/task/service.go`

### 7.3 Server / Portal Backend

Likely changes:

- portal workflow handlers
- route registration
- issue patch behavior for workflow assignment
- workflow execution read APIs

Likely files:

- new `internal/server/portal/workflows.go`
- `internal/server/portal/register.go`
- `internal/server/portal/issues.go`
- `internal/server/portal/config.go`

### 7.4 Executor / Runtime

Likely changes:

- workflow execution coordinator
- step dispatch through current task creation / run paths
- workflow run status sync

Likely files:

- `internal/executor/*`
- `internal/app/task/service.go`
- possibly new workflow runtime package if separation is cleaner

### 7.5 Portal Frontend

Likely changes:

- workflow list and detail views
- issue assignee selector includes workflows
- workflow run detail surface

Likely files:

- `portal/src/features/*`
- workflow-related pages/components to be added

---

## 8. Data Model Decisions

### 8.1 Workflow Table

Recommended semantics:

- one row per reusable team-scoped execution plan
- definition stored as text / JSON
- owned by `team_id`
- authored by `created_by`

Recommended MVP notes:

- no version table in this phase
- updates replace the current definition
- historical runs keep their own resolved execution state

### 8.2 Workflow Run Table

Recommended semantics:

- one row per workflow execution attempt
- linked to one workflow
- linked to one issue
- owned indirectly by the same team

Recommended MVP fields:

- `workflow_run_id`
- `workflow_id`
- `issue_id`
- `status`
- `created_by`
- `created_at`
- `started_at`
- `ended_at`
- `error_message`

Recommended v1 statuses:

- `pending`
- `running`
- `succeeded`
- `failed`
- `canceled`

### 8.3 Workflow Step Run Table

Recommended semantics:

- one row per step inside a workflow run
- stores durable progress for each step
- may optionally reference a backing `task_id` and latest `task_run_id`

Recommended MVP fields:

- `workflow_step_run_id`
- `workflow_run_id`
- `step_id`
- `step_index`
- `step_type`
- `status`
- `task_id`
- `task_run_id`
- `output_summary`
- `error_message`
- `created_at`
- `started_at`
- `ended_at`

Recommended v1 statuses:

- `pending`
- `running`
- `succeeded`
- `failed`
- `blocked`

### 8.4 Issue Assignment Model

Recommended rule:

- keep `Issue` as the user-facing work object
- extend assignment with `workflow`
- validate that assigned workflow belongs to the same team

This means the assignment vocabulary becomes:

- `person`
- `agent`
- `workflow`

### 8.5 Workflow Definition Versioning

Recommended phase-3 rule:

- a workflow definition is mutable
- each workflow run should snapshot enough step data to remain understandable after edits

This avoids blocking phase 3 on full version-history design while still keeping runs interpretable.

---

## 9. API And Request-Context Decisions

### 9.1 Team Scope

Workflow APIs should follow the same explicit team routing introduced in Phase 2.

Recommended route family:

- `/api/teams/{team_id}/workflows`

This keeps ownership consistent with:

- agents
- issues
- conversations
- tasks

### 9.2 Workflow APIs

Recommended MVP APIs:

- `GET /api/teams/{team_id}/workflows`
- `POST /api/teams/{team_id}/workflows`
- `GET /api/teams/{team_id}/workflows/{workflow_id}`
- `PATCH /api/teams/{team_id}/workflows/{workflow_id}`
- `GET /api/teams/{team_id}/workflows/{workflow_id}/runs`
- `GET /api/teams/{team_id}/workflow-runs/{workflow_run_id}`
- `POST /api/teams/{team_id}/workflows/{workflow_id}/runs`

### 9.3 Issue APIs

Issue APIs should be extended, not replaced.

Recommended change:

- existing issue patch API accepts `assignee_kind=workflow`
- validation checks same-team ownership

### 9.4 Execution APIs

Recommended MVP execution triggers:

- `POST /api/teams/{team_id}/workflows/{workflow_id}/runs`
- `POST /api/teams/{team_id}/issues/{issue_id}/workflow-runs`

Recommended semantics:

- direct workflow trigger is the simplest manual test path
- issue-scoped trigger is the user-facing work-management path once an issue is assigned to a workflow

This keeps phase 3 explicit and observable. If auto-start on assignment is added later, these endpoints still remain useful for reruns and manual testing.

---

## 10. Runtime And Execution Decisions

### 10.1 Execution Strategy

Recommended workflow runtime behavior:

1. create `workflow_run`
2. materialize ordered `workflow_step_run` rows
3. pick the next runnable step
4. dispatch that step through existing task creation primitives
5. wait for backing task/run completion
6. persist step result
7. continue until terminal workflow status

### 10.2 Step Type V1

Recommended first and only step type for phase 3:

- `agent_task`

Semantics:

- workflow step targets a team agent
- system creates a backing task using that agent and prompt
- task execution remains in the current worker pipeline

This gives phase 3 real value without introducing multiple step executors at once.

### 10.3 Conversation And Task Relationship

Recommended rule:

- workflow execution should reuse existing task creation and run semantics
- workflow runtime may create a dedicated conversation for workflow-owned task traces, or reuse an issue-linked conversation if that becomes cleaner during implementation

Implementation should choose the smaller path, but the design constraint remains:

- do not bypass current task runtime

### 10.4 Failure Handling

Recommended v1 behavior:

- fail-fast by default
- if one step fails, mark the current step failed and the workflow run failed
- downstream steps remain `blocked` or `pending`

Do not add step-level retry policy authoring in this phase.

---

## 11. Product / UX Decisions

### 11.1 Workflow Authoring

Recommended MVP UX:

- simple create/edit form
- name
- description
- structured definition editor

This can be text-area based in the first pass.

### 11.2 Issue Assignee UX

Recommended assignment presentation:

- `Unassigned`
- team member
- team agent
- team workflow

The UI may present one picker, but the backend must preserve distinct kinds.

### 11.3 Workflow Run Visibility

Recommended workflow run detail should show:

- workflow metadata
- issue link
- overall run status
- ordered step list
- each step's status
- backing task/run references when available

This is enough to make workflow execution feel inspectable rather than opaque.

### 11.4 Manual Trigger UX

Recommended MVP UX:

- workflow detail shows a `Run Workflow` action
- issue detail shows a `Run Workflow` action when the issue is assigned to a workflow

This gives phase 3 a straightforward manual testing path before we consider any automation behavior.

---

## 12. Implementation Strategy

Recommended implementation order:

### Step 1. Add Workflow And Execution Entities

- add `workflow`
- add `workflow_run`
- add `workflow_step_run`
- add store interfaces and implementations
- add migration via `AutoMigrate`

### Step 2. Define Workflow Definition Validation

- define v1 schema for linear steps
- validate required fields
- validate allowed step types
- validate referenced agents belong to the same team

### Step 3. Add Workflow Service

- create workflow CRUD service
- add run creation and status transition helpers
- centralize definition parsing and validation

### Step 4. Extend Issue Assignment Validation

- allow `workflow` in issue service
- validate workflow exists in current team

### Step 5. Add Workflow Portal APIs

- CRUD handlers
- run listing/detail handlers
- explicit manual run trigger handlers

### Step 6. Implement Workflow Runtime

- create a minimal coordinator for linear step execution
- dispatch each step through current task service / runtime
- persist workflow run and step run transitions

### Step 7. Add Minimal Portal UX

- workflow list
- workflow detail
- issue workflow assignment
- workflow run detail

---

## 13. Validation / Acceptance Checks

Phase 3 is complete when the following are true:

1. a team can create and update a workflow
2. a workflow definition is validated and stored durably
3. an issue can be assigned to a workflow in the same team
4. the system exposes a manual trigger for workflow execution
5. the system can create a workflow run for that issue
6. the system can also create a workflow run directly from workflow detail for manual testing
7. workflow steps execute in order through the existing task runtime
8. workflow run status becomes inspectable
9. step-level state becomes inspectable
10. workflow assignment does not collapse workflow and agent into one type internally
11. no separate parallel runtime is introduced outside the current task / worker foundation

---

## 14. Open Questions / Follow-Ups

The following questions should be finalized during implementation:

1. Should workflow-created backing tasks live in a dedicated workflow conversation, or in an issue-linked conversation?
2. How much workflow definition snapshot data should be copied onto `workflow_run` / `workflow_step_run` for historical readability?
3. Should phase 3 allow manual rerun from the failed step, or only rerun the whole workflow?
4. In a future phase, should assignment optionally auto-start workflow execution after we validate the manual path?

Recommended bias for MVP:

- explicit run action
- direct workflow-level manual trigger plus issue-level manual trigger
- dedicated workflow execution records
- fail-fast semantics
- full rerun before partial rerun

---

## 15. Current Status

Current repository state before implementation:

- phase 1 issue foundation is complete
- phase 2 team ownership foundation is complete
- issue assignment currently supports `person` and `agent` only
- no workflow entity exists yet
- no workflow execution record exists yet
- current task runtime is available and should be reused

This means phase 3 can start from a solid ownership and execution base, but must still define both workflow persistence and workflow runtime integration.
