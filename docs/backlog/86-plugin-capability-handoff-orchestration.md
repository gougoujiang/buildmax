---
id: plugin-capability-handoff-orchestration
title: Add the immediate capability handoff orchestration
roadmap: R5
source: docs/design/plugin-space-distribution.md#161-the-immediate-capability-handoff-transition
depends_on: [84-plugin-install-staging-and-policy.md]
verification: ["./make test", "./make test mysql", "./make e2e cli"]
claim:
---

## Outcome

When an Agent needs a just-installed capability within the same objective, the
run continues in a successor TaskRun that loads it, presented as one continuous
execution. This is the transition specified in plugin-space-distribution.md
§16.1; it reuses the shipped Task workspace checkpoint machinery.

## Scope

- At a TaskRun boundary, when a staged install requested an immediate handoff:
  quiesce the worker through the graceful-shutdown path; capture, upload, verify,
  and commit a successful result checkpoint via the existing
  task-workspace-checkpoints.md §5.5 protocol without changing it.
- In the same transaction that accepts the terminal report and advances
  `task.workspace_head_checkpoint_id`, freeze the pending revision into an
  immutable Plugin environment revision, set the TaskRun's
  `plugin_environment_result_id` with status `committed`, and advance
  `task.plugin_environment_head_id`. Workspace head and environment head advance
  together or not at all.
- The Server (not the worker) spawns exactly one successor TaskRun whose base is
  the new workspace head, the committed session, and the new environment head; it
  materializes the expanded plugin set as its fail-closed preparation gate.

## Out Of Scope

- Deployment/kind end-to-end evidence (task 88).
- Hot-loading the current process; capability never changes mid-run.

## Acceptance Criteria

- No successor and no head advance on checkpoint or environment commit failure;
  the completed work stands, the pending revision is left unreferenced for GC,
  and the user may Continue from the last durable head (fail-open, never silent).
- A successor whose materialization fails fails closed; it never falls back to the
  prior capability set.
- Retry of the pre-handoff run reconstructs its base environment without the
  addition.
- Exactly one successor is created per committed immediate-handoff report;
  duplicate or late reports cannot create a second.

## Verification

- `go-unit`: the commit is atomic (both heads or neither); fail-open on commit
  failure; fail-closed on successor materialization failure; Retry base
  reconstruction; single-successor idempotency.
- `mysql`: the paired head advance and successor creation against real MySQL.

## Notes

The checkpoint machinery this reuses (seed/restore/result/partial, atomic head
advance, fail-closed restore) already ships on main. This task is the plugin
environment counterpart of that head advance plus the successor spawn. The exact
transition is designed in plugin-space-distribution.md §16.1 and referenced from
task-workspace-checkpoints.md §5.4.
