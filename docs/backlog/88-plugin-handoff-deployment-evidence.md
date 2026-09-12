---
id: plugin-handoff-deployment-evidence
title: Prove Task-scoped autonomous install end to end
roadmap: R5
source: docs/design/task-workspace-checkpoints.md#164-runtime-and-deployment-evidence
depends_on: [86-plugin-capability-handoff-orchestration.md]
verification: ["kind", "./make e2e local"]
claim:
---

## Outcome

Closes §16.4 item 7 of the checkpoint design: on a real cluster, a Task-scoped
autonomous Plugin installation continues in a new run with the pinned package,
while Retry reconstructs the original Plugin base. This is the deployment
evidence the unit tests cannot provide.

## Scope

- Add a kind-based end-to-end scenario in which a run installs a Task-scoped
  package, hands off, and a successor run loads the pinned package and uses its
  capability.
- Assert that Retry of the pre-handoff run reconstructs the original base
  environment without the addition.
- Add or extend the deployment smoke and, where it crosses the Portal boundary,
  the Portal browser assertion, per docs/contribute/testing.md.

## Out Of Scope

- The orchestration itself (task 86); this task only proves it on a cluster.

## Acceptance Criteria

- The kind scenario shows install → handoff → successor loads the pinned package.
- Retry reconstructs the original base environment.
- Route or surface changes, if any, keep `tools/mk/deploy_smoke.go` and the
  Portal e2e specs in sync.

## Verification

- `kind`: run the scenario on an ephemeral cluster
  (`BUILDMAX_KIND_EPHEMERAL=1 ./make kind up`), then the deployment smoke and any
  Portal browser suite the change requires; tear down with `./make kind down`.

## Notes

Create the ephemeral cluster on demand only for this task's boundary; use unit
tests for everything task 86 already proves. Removing or adding routes must
update both `tools/mk/deploy_smoke.go` and the Portal `e2e/*.spec.ts` files, a
known source of main-red.
