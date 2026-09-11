---
id: reconcile-linear-workflow-runs
title: Reconcile linear Workflow runs from durable facts
roadmap: R1
source: docs/design/workflow-runtime.md#10-reconciliation
depends_on:
  - 30-workflow-idempotent-task-admission.md
  - 40-workflow-reconciliation-store.md
verification:
  - ./make test ./internal/service/workflow
  - ./make test mysql -run TestWorkflowReconcile
  - ./make test mysql
  - ./make test
claim:
---

## Outcome

The current linear Workflow can be progressed from persisted WorkflowStepRun,
Task, and TaskRun facts after a lost terminal callback. Repeating or racing the
same reconciliation does not duplicate a Task, accept an outcome twice, or
rewrite a terminal run.

## Scope

- Add the single `Service.Reconcile(ctx, workflowRunID)` progression entry point
  for the existing linear Workflow precursor.
- Under a reconciliation lease, read the run, ordered steps, linked Tasks and
  TaskRuns; fold terminal TaskRun state into guarded step/run transitions;
  idempotently admit the next pending step; and set the next due time while work
  remains.
- Recover the crash window between Task admission and WorkflowStepRun linkage by
  replaying the prerequisite task's stable admission key.
- Make `HandleTaskRunTerminal` a wake-up that invokes reconciliation; the
  callback payload is not durable execution authority.
- Preserve current success summaries, failure/cancellation semantics, Agent
  snapshot behavior, provenance, and Issue relation while replacing callback
  sequencing.
- Add service and real-MySQL tests for lost callbacks, duplicate terminal
  observation, concurrent reconciliation, and create-before-link recovery.
- Update architecture and design implementation status, but leave automatic
  startup/periodic recovery claims to the dependent scheduling task.

## Out Of Scope

- The background due-run scanner and Server startup/shutdown wiring.
- New WorkflowRun cancellation APIs, retries, timeouts, human waits, or
  automatic worker re-dispatch.
- Phase 2 versioned definitions, NodeRun replacement, full outputs/bindings,
  static DAGs, routes, or structured output.
- Changing how ordinary Tasks execute or adding a Workflow-owned model loop.

## Acceptance Criteria

- Reconciling a running step whose TaskRun already succeeded marks that step
  succeeded and admits/links exactly the next linear step; reconciling the last
  success makes the WorkflowRun terminal succeeded.
- Failed and canceled TaskRuns atomically terminalize the current step and run
  and block later pending steps with the existing documented distinctions.
- If Task admission committed but step linkage did not, reconciliation obtains
  the same Task/TaskRun from the stable key and finishes the link without a
  duplicate execution.
- Duplicate callbacks, repeated reconciliation, and two reconcilers racing
  leave one accepted step outcome, one next Task, and one legal run state.
- A missing callback is recoverable by a later explicit `Reconcile` call using
  only persisted state; no callback payload or process memory is required.
- A failed reconciliation records a future due time or leaves the run eligible
  for takeover; it does not silently mark incomplete work successful.
- Real-MySQL tests fail when Task admission idempotency or guarded terminal
  observation is deliberately removed.

## Verification

Run the Workflow service tests first, including an in-memory lost-callback
case. Then run `./make test mysql -run TestWorkflowReconcile`, the complete
MySQL scope, and `./make test`.

The in-memory tests make state folding fast to diagnose. The MySQL tests prove
the cross-transaction crash window and concurrent writers that mocks cannot.

## Notes

The current sequencing is concentrated in
`internal/service/workflow/service.go`: `HandleTaskRunTerminal`,
`dispatchNextStep`, and `createStepTask`. Replace their progression authority
coherently rather than adding reconciliation as a second path beside them.
