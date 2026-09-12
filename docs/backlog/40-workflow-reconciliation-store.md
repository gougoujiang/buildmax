---
id: workflow-reconciliation-store
title: Make due Workflow runs durably claimable
roadmap: R1
source: docs/design/workflow-runtime.md#10-reconciliation
depends_on: [30-workflow-idempotent-task-admission.md]
verification:
  - ./make test ./internal/core/workflow
  - ./make test mysql -run TestWorkflowReconciliationLease
  - ./make test mysql
  - ./make test
claim: gougoujiang 2026-09-13
---

## Outcome

A Server can discover Workflow runs that need progress and claim a bounded
reconciliation lease from durable state. A crashed or paused owner cannot strand
a run or let two replicas treat an unexpired lease as theirs.

## Scope

- Add the minimum current-linear-run fields for reconciliation ownership, lease
  expiry, and `next_reconcile_at` to the WorkflowRun domain and database row.
- Add intent-oriented store methods to list a bounded batch of due non-terminal
  runs and to claim, renew, and release a lease with guarded writes.
- Define deterministic due ordering, clock inputs suitable for tests, a bounded
  batch, and an index that supports the due query.
- Make lease expiry permit safe takeover and make a stale owner unable to renew
  or release the new owner's lease.
- Keep terminal Workflow runs out of the due set and clear scheduling/ownership
  state when a run becomes terminal.
- Add real-MySQL contention tests and update the factual data-model and
  architecture documentation.

## Out Of Scope

- Implementing the reconciliation state fold or dispatching a Workflow step.
- Starting a background loop in Server bootstrap.
- Treating the lease as the correctness mechanism. Idempotent admission and
  guarded state transitions remain authoritative when a lease expires during a
  live process.
- Operator configuration for lease or polling intervals without a demonstrated
  requirement.
- Phase 2 NodeRun schema, DAGs, retry policy, timeouts, or durable requests.

## Acceptance Criteria

- Due selection returns only non-terminal runs whose `next_reconcile_at` has
  arrived or whose prior lease expired, in stable oldest-due order and within a
  documented batch bound.
- Two concurrent real-MySQL claimers cannot both acquire the same unexpired
  WorkflowRun lease.
- After expiry, another owner can claim the run; the stale owner cannot renew or
  release the replacement lease.
- Marking a run terminal removes it from future due reads even if its previous
  scheduling timestamp is in the past.
- Query and transition tests detect removal of the status filter, lease-owner
  guard, or expiry condition.
- Store errors remain typed above `internal/infra/db`, and no GORM type escapes
  the infrastructure package.

## Verification

Run pure Workflow state tests first. Then run
`./make test mysql -run TestWorkflowReconciliationLease`, followed by the full
MySQL and ordinary suites.

Use fixed UTC timestamps at MySQL microsecond precision. Demonstrate the
critical query/guard mutations locally and revert them before delivery, as
required by the verification program.

## Notes

Follow the intent-oriented store boundary in Workflow runtime §15.2. Existing
scheduler and retention loops are useful operational precedents, but this task
must not make `internal/core/workflow` depend on scheduler, config, or database
packages.
