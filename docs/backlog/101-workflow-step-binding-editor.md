---
id: workflow-step-binding-editor
title: Author a step's input bindings in the Portal step form
roadmap: none
source: docs/design/workflow-runtime.md#6-workflow-definition-contract
depends_on: []
verification: [portal-unit, portal-e2e]
claim:
---

## Outcome

A Portal user can bind an earlier step's output into a step without hand-editing
JSON. Step output binding ships today, but the only way to author it is advanced
JSON mode; the step form has no binding field, so the feature is effectively
hidden from anyone not editing raw definitions.

## Scope

Add binding authoring to the Agent-step form in `portal/src/features/workflows/`:
a control on each step to add or remove input bindings, each choosing an earlier
step and naming the value. The draft already carries `bindings`
(`WorkflowStepDraft`); this is the form UI and its validation, reusing the same
`validateSteps` gate so the form and advanced JSON stay in agreement.

## Out Of Scope

- The backend, storage, and the advanced-JSON contract — already shipped.
- The versioned `nodes`/typed-`bindings` contract and its editor. If the full
  §6 cutover is scheduled first, fold this into that editor rather than building
  a `steps`-shaped binding form that the cutover would discard; check the
  roadmap before starting.

## Acceptance Criteria

- The step form can add, edit, and remove a step's input bindings, and a step
  can only bind an earlier step.
- Client validation refuses a binding to a missing or later step and a duplicate
  binding name, matching the server, so Save is not enabled for a definition the
  server would reject.
- A workflow authored entirely through the form, with a binding, round-trips and
  runs.

## Verification

- Portal unit tests for the form's binding validation and draft round-trip.
- `portal/e2e/workflow-authoring.spec.ts`: author a two-step workflow with a
  binding through the form and confirm it saves.

## Notes

- The wire shape and the draft field already exist: `bindings` on
  `WorkflowStepDraft`, serialized as `{name, from_step}` by `stepsToDefinition`.
- Server validation and the labelled-untrusted input assembly are in
  `internal/service/workflow`; this task adds no runtime behavior.
