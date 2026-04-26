# Phase 4: Issue Flow Visualization

## Status

- phase: `4`
- name: `Issue Flow Visualization`
- status: `paused`
- roadmap: [design/010-team-task-workflow-roadmap.md](./010-team-task-workflow-roadmap.md)
- depends_on: [design/013-phase-3-workflow-foundation.md](./013-phase-3-workflow-foundation.md)
- started_at: `2026-04-26`
- paused_at: `2026-04-26`

---

## 1. Goal

Make issue execution visible and inspectable from the issue itself.

The roadmap goal for this phase is to let users answer "what is happening with this issue?" without reading raw logs.

---

## 2. Current State

Phase 4 is currently paused after completing the workflow-backed issue flow MVP.

Implemented:

- issue detail is now an independent page instead of an edit dialog
- issue list rows navigate to `#/issue/:issueId`
- backend exposes an issue-centric flow endpoint
- issue detail shows editable issue fields, business status, current assignee, execution summary, timeline, latest workflow steps, run history, and links to workflow run detail / conversation trace
- workflow-assigned issues can be inspected through issue-level flow data

Not implemented yet:

- direct artifact/result aggregation on the issue detail page
- agent-assigned issue run sequence and outputs
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

Added workflow run lookup by issue:

- `WorkflowStore.ListWorkflowRunsByIssue`
- `Store.ListWorkflowRunsByIssue`
- mock store support for tests

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
- latest workflow step status view
- timeline derived from issue, workflow run, and workflow step run data
- workflow run history
- navigation to workflow run detail and conversation trace

---

## 5. Validation

Last validation before pause:

- `go test ./internal/server/portal ./internal/app/workflow ./internal/storage/entity`
- `cd portal && npm run build`
- focused frontend lint check for changed files

All passed.

---

## 6. Pause Notes

Work on agent-assigned issue run sequence was started conceptually, then explicitly paused.

The temporary task/issue linkage edits were reverted. Current task persistence does **not** include `issue_id`, and there is no issue-linked agent task/run data chain yet.

Recommended next decision before continuing:

- decide whether agent-assigned issue execution should be modeled by adding `issue_id` to `task`, or by introducing a separate issue execution table
- decide whether agent issue execution should create a dedicated conversation per issue, reuse an existing issue conversation, or remain task-first
- decide how artifacts/results should be surfaced on issue detail

---

## 7. Exit Criteria Remaining

Before marking Phase 4 fully done:

- issue detail directly surfaces artifacts/results, not only links to other pages
- agent-assigned issues show run sequence and outputs
- timeline behavior is stable enough for the product experience, even if a later audit log replaces the derived MVP timeline
