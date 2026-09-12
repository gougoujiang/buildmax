---
id: workflow-revision-contention
title: Protect Workflow revision advancement under contention
roadmap: R2
source: docs/design/workflow-runtime.md#152-store-capabilities
depends_on: []
verification:
  - ./make test ./internal/service/workflow
  - ./make test mysql -run TestWorkflowRevisionContention
  - ./make test mysql
  - ./make test
claim: gougoujiang 2026-09-12
---

## Outcome

Concurrent Workflow edits cannot silently overwrite a newer definition or leak
a duplicate-key database error. One edit from an expected revision commits, and
a stale edit receives a domain conflict that can be retried after re-reading.

## Scope

- Carry the Workflow revision observed by the service into the store update as
  the expected prior revision.
- Guard the Workflow row update on that revision and append the matching
  WorkflowRevision in the same transaction.
- Return a stable domain conflict when another writer already advanced the row;
  do not expose GORM or MySQL duplicate-key details.
- Apply the same rule to content edits, status transitions, and revision restore
  because all three append through the authoritative update path.
- Add fixed-fixture real-MySQL tests for sequential advancement and concurrent
  writers, including a mutation check that removes the revision guard.
- Update current-state and the verification/design status; add a `fixed`
  changelog entry if the corrected conflict is user-visible through the API.

## Out Of Scope

- Phase 2 separation of draft and published revision pointers.
- Collaborative merge, automatic retry of a stale edit, or Portal conflict-
  resolution UI beyond preserving the existing surfaced API error.
- Workflow run reconciliation, Task admission, or any execution-state change.
- Rewriting historical revisions or adding compatibility around the Alpha
  schema.

## Acceptance Criteria

- Two updates that both start from revision N cannot both commit as revision
  N+1: exactly one wins and the other returns the standard conflict kind.
- The winning Workflow row and appended WorkflowRevision contain the same name,
  description, definition, status, author, and revision number.
- After re-reading the winner, a retried edit advances to the next revision and
  preserves the prior immutable revision.
- Create, no-op update, restore, publish, and archive tests retain their current
  documented behavior.
- No losing path returns a raw duplicate-key or serialization error, and no
  failed transaction changes either the live row or revision history.
- Removing the expected-revision predicate makes the contention test fail.

## Verification

Run the focused Workflow service tests, then
`./make test mysql -run TestWorkflowRevisionContention`, the full MySQL scope,
and `./make test`.

The test must coordinate the writers at the race boundary rather than rely on
timing. Use independent expected values and inspect both the current Workflow
and immutable revision rows after the race.

## Notes

The verification program §4.2 lists Workflow revision advancement under edits
and contention as an open real-database case. `internal/infra/db/workflow.go`
currently reads before its update transaction; keep the expected-revision rule
in the Workflow service/store boundary so handlers do not reimplement it.
