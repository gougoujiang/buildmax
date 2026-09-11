---
id: workflow-idempotent-task-admission
title: Admit Workflow Tasks idempotently
roadmap: R1
source: docs/design/workflow-runtime.md#11-dispatch-and-idempotency
depends_on: []
verification:
  - ./make test ./internal/service/task
  - ./make test ./internal/service/workflow
  - ./make test mysql -run TestWorkflowTaskAdmission
  - ./make test mysql
  - ./make test
claim:
---

## Outcome

Repeating Workflow step dispatch after a crash or race resolves to the same Task
and first TaskRun instead of duplicating execution. This closes the
create-before-link side of durable Workflow recovery without adding a second
execution plane.

## Scope

- Add a stable Task admission key scoped to its Space and persist it on the Task
  row with the required uniqueness constraint.
- Add an intent-oriented Task service/store operation for Workflow admission
  that atomically creates the Task and first TaskRun or returns the pair already
  admitted under the same key.
- Reject reuse of a key with a conflicting Task or first-run payload rather than
  silently returning unrelated work.
- Have the current linear Workflow service use
  `workflow/<workflow_run_id>/node/<step_id>` as its Task admission key.
- Preserve existing direct Task, Conversation, Issue, Continue, and Retry
  behavior when no Workflow admission key is supplied.
- Add real-MySQL contention and mutation-sensitive tests, then update the data
  model, current-state description, and relevant architecture documentation.

## Out Of Scope

- The Workflow reconciliation loop, due-run scanner, restart wiring, retries,
  cancellation, timeouts, DAG execution, or typed dataflow.
- Public caller idempotency for ordinary Task creation.
- A transaction spanning Workflow rows and Task rows; recovery comes from the
  stable key and replay, not a cross-service transaction.
- TaskRun retry-attempt admission keys beyond the existing first-run behavior
  needed by the linear precursor.

## Acceptance Criteria

- The first Workflow admission creates exactly one Task and its first TaskRun in
  one transaction and returns both stable public IDs.
- Repeating an identical admission key and payload returns those same IDs before
  and after the first TaskRun becomes terminal.
- Concurrent identical admissions against real MySQL produce one Task and one
  first TaskRun; every successful caller observes the same pair.
- Reusing the key with a different input, Agent revision, provenance, Issue
  relation, creator, or other execution-authority field returns a domain
  conflict and leaves the original rows unchanged.
- Two Spaces may use the same admission-key text without colliding.
- A mutation that removes the uniqueness/locking protection makes the
  contention test fail.
- Existing Task creation paths remain unchanged under focused service and store
  regression tests.

## Verification

Run the focused Task and Workflow service packages first. Exercise the new
store contract with `./make test mysql -run TestWorkflowTaskAdmission`, then run
the complete `./make test mysql` scope and `./make test`.

The real-MySQL scope is mandatory because uniqueness, locking, and transaction
behavior are the feature; an in-memory mock cannot prove them.

## Notes

`internal/infra/db/task.go` already creates a Task and its first TaskRun in one
transaction. `internal/infra/db/task_run.go` already supplies TaskRun-scoped
idempotency for Continue. Reuse those authoritative boundaries instead of
creating Workflow-owned Task rows. The design's future term `node_id` maps to
the current linear `step_id` for this Phase 1 repair.
