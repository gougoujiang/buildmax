---
id: schedule-workflow-recovery
title: Recover due Workflow runs after Server restart
roadmap: R1
source: docs/design/workflow-runtime.md#19-delivery-and-migration
depends_on: [50-reconcile-linear-workflow-runs.md]
verification:
  - ./make test ./internal/server/scheduler
  - ./make test ./internal/bootstrap
  - ./make test mysql -run TestWorkflowRestartRecovery
  - ./make test mysql
  - ./make kind smoke
claim: gougoujiang 2026-09-13
---

## Outcome

Lost callbacks and Server restarts no longer strand persisted linear Workflow
runs. Every Server replica performs a bounded due-run sweep, and the durable
lease/reconciliation contract makes their overlap safe.

## Scope

- Add a Workflow reconciliation loop under the existing Server background-work
  ownership and wire it through bootstrap and graceful shutdown.
- Sweep once at startup, then poll due Workflow runs in bounded batches with a
  finite recovery interval and bounded backoff after failures.
- Let multiple Redis-coordinated Server replicas run the loop; WorkflowRun
  leases and idempotent transitions, not process-local election, prevent
  duplicate progress.
- Log claim, recovery, failure, and latency information with WorkflowRun IDs but
  no prompt, output, credentials, or other sensitive content.
- Add a real-MySQL restart test that persists a terminal step TaskRun without
  delivering its callback, starts a fresh loop, and observes the Workflow
  converge without duplicate Task execution.
- Update current-state, Workflow/Server architecture, Roadmap status if its
  completion wording changes, operator-facing recovery documentation, and a
  user-visible changelog entry.

## Out Of Scope

- Redis as Workflow state or a new distributed scheduler dependency; MySQL
  remains the durable authority.
- Automatic re-dispatch of a worker TaskRun lost after claim, which remains an
  accepted first-Beta limit.
- Candidate topology qualification for cross-replica streaming, Redis outage,
  restore, upgrade, or credential rotation.
- Workflow retries, timeouts, new cancellation behavior, DAGs, or typed
  dataflow.

## Acceptance Criteria

- The loop performs an immediate startup sweep and then revisits due work within
  a documented finite interval; one failed sweep is logged and retried without
  terminating Server.
- A persisted Workflow with a terminal TaskRun and no delivered callback
  reaches the same terminal or next-step state after a new Server process starts.
- Two loop instances racing on the same real-MySQL state do not duplicate a
  Task, accept an output twice, or move a terminal run.
- Shutdown stops new sweeps, lets an in-flight bounded pass finish within the
  existing drain budget, and does not leave an ownership claim that prevents
  takeover after expiry.
- The restart test observes persisted rows and stable IDs, not an in-process
  callback or mock-only state.
- Operational logs identify the run and recovery result without including user
  content or secrets.
- Documentation distinguishes Workflow progression recovery from automatic
  retry of a lost worker execution.

## Verification

Run the scheduler and bootstrap packages first, then the focused and full MySQL
scopes. Because this changes Server background work, finish in an isolated kind
cluster:

```text
BUILDMAX_KIND_EPHEMERAL=1 ./make kind up
./make kind smoke
./make kind down
```

The MySQL test is the authoritative restart evidence. The kind smoke proves the
new loop coexists with the deployed two-Server topology and ordinary worker
execution.

## Notes

`internal/server/scheduler` already owns bounded background sweeps and
`internal/bootstrap/server.go` owns their lifecycle. Reuse that operational
shape without merging Workflow progression into the TaskRun dispatch scheduler.
