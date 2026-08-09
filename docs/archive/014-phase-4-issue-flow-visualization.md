# Phase 4: Issue Flow Visualization

## Status

- phase: `4`
- name: `Issue Flow Visualization`
- status: `done`
- roadmap: [design/010-team-task-workflow-roadmap.md](./010-team-task-workflow-roadmap.md)
- depends_on: [design/013-phase-3-workflow-foundation.md](./013-phase-3-workflow-foundation.md)
- started_at: `2026-04-26`
- completed_at: `2026-04-26`

---

## 1. Goal

Make issue execution visible and inspectable from the issue itself.

The roadmap goal for this phase is to let users answer "what is happening with this issue?" without reading raw logs.

---

## 2. Current State

Phase 4 is complete for the current scope. Direct artifact/result aggregation is intentionally deferred to a future separate task.

Implemented:

- issue detail is now an independent page instead of an edit dialog
- issue list rows navigate to `#/issue/:issueId`
- backend exposes an issue-centric flow endpoint
- issue detail shows editable issue fields, business status, current assignee, execution summary, timeline, latest workflow steps, run history, and links to workflow run detail / conversation trace
- workflow-assigned issues can be inspected through issue-level flow data
- agent-assigned issues can create issue-linked agent tasks
- agent-assigned issue task sequence and outputs are visible from issue detail

Deferred:

- direct artifact/result aggregation on the issue detail page
- durable audit/event log for timeline data

---

## 3. Backend Work Completed

Added issue-centric flow aggregation:

- `GET /api/teams/{team_id}/issues/{issue_id}/flow`
- validates the issue belongs to the current team
- returns the issue
- returns the assigned workflow when the issue is workflow-assigned
- returns workflow runs linked to the issue
- returns step runs for each workflow run
- returns agent tasks linked to the issue

Added workflow run lookup by issue:

- `WorkflowStore.ListWorkflowRunsByIssue`
- `Store.ListWorkflowRunsByIssue`
- mock store support for tests

Added agent issue execution linkage:

- `task.issue_id`
- `TaskStore.ListTasksByIssue`
- `POST /api/teams/{team_id}/issues/{issue_id}/agent-runs`
- agent-assigned issue runs create a new `task` per run

---

## 4. Frontend Work Completed

Added a standalone issue detail page:

- `portal/src/pages/IssueDetail.tsx`
- route `#/issue/:issueId` now renders `IssueDetail`
- Issues list no longer opens the issue detail modal
- `IssueModal` remains available for issue creation

The detail page currently includes:

- issue title and description editing
- business status editing
- assignee editing
- current assignee summary
- execution summary for the latest workflow run
- execution summary for the latest agent task when applicable
- latest workflow step status view
- timeline derived from issue, workflow run, and workflow step run data
- workflow run history
- agent run sequence
- navigation to workflow run detail and conversation trace

---

## 5. Validation

Last validation:

- `go test ./internal/server/portal ./internal/core/task ./internal/core/workflow ./internal/infra/db`
- `cd portal && npm run build`
- focused frontend lint check for changed files

All passed.

---

## 6. Decisions

Decisions made during this phase:

- agent-assigned issue execution should be modeled by adding `issue_id` to `task`
- each agent issue run should create a new `task`
- artifacts/results aggregation is intentionally deferred to a future separate task

---

## 7. Follow-Up

Future work:

- issue detail directly surfaces artifacts/results, not only links to other pages
- timeline behavior is stable enough for the product experience, even if a later audit log replaces the derived MVP timeline
