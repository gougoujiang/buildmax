---
id: workflow-step-output-binding
title: Bind an upstream step's output into a downstream step's input
roadmap: none
source: docs/design/workflow-runtime.md#9-inputs-outputs-and-trust-boundaries
depends_on: []
verification: [go-unit, workflow-mysql, portal-e2e]
claim:
---

## Outcome

A Workflow can pass one Agent's result to the next: a step declares that it
takes a named earlier step's output as input, and the run feeds that step's
full output to the downstream Agent as labelled, untrusted context. Today every
step runs a static prompt and an upstream result is stored but never reaches a
later step, so multi-step Workflows cannot actually collaborate on data.

## Scope

The minimal text-binding slice over the current linear `steps` format — not the
versioned `nodes`/`bindings` contract.

- `DefinitionStep` gains an optional `bindings` list; each binding names a value
  (`name`) sourced from an earlier step's whole output (`from_step`).
- Publication validation: `from_step` names an existing earlier step, binding
  names are unique within a step, and a step cannot bind itself or a later step
  (the run is linear, so "earlier" is a smaller step index).
- The run snapshots each step's bindings onto its StepRun at start, the same way
  it snapshots the agent, so editing the Workflow mid-run cannot change what an
  in-flight step receives.
- At dispatch, for each binding the service reads the named upstream step's
  TaskRun output (the full output already persisted on the TaskRun, not the
  500-rune display summary) and appends it to the downstream Task's **input** as
  a labelled untrusted block — never merged into the Agent's instructions.
- The bound value is the whole upstream output (no JSON Pointer, no templating).

## Out Of Scope

- The versioned `schema_version: 1` `nodes` + typed `bindings` contract,
  `input_schema`, DAG `needs`, JSON Pointer selection, and `output_schema`
  validation — the full Phase-2/§6 cutover, a later task.
- Structured output (`structured` envelope, `output_schema`): stays null until
  the structured-output work lands. This slice binds the `text` output only.
- The Portal step-editor UI for authoring bindings — the follow-up task
  101-workflow-step-binding-editor.md; this task ships the backend and the JSON
  (advanced-mode) contract with e2e proving execution end to end.

## Acceptance Criteria

- A published two-step Workflow whose second step binds the first step's output
  runs to success, and the second Agent's input contains the first step's full
  output in a labelled untrusted block.
- Publication rejects a binding to a missing step, to a later step, to the step
  itself, and a duplicate binding name, each with a clear validation error.
- A binding is snapshotted at run start: editing the Workflow's definition after
  the run starts does not change what a later in-flight step receives.
- The bound value is the full TaskRun output, not the truncated summary.
- Upstream output reaches `input`, never `instructions`, and is labelled
  untrusted.

## Verification

- `./make test ./internal/service/workflow/...` and `./internal/core/workflow/...`:
  binding validation, snapshot, and the labelled-input assembly.
- `./make test mysql`: the reconcile/restart scope, plus a run that dispatches a
  bound downstream step and asserts its Task input carries the upstream output.
- `./make e2e local` (Portal browser, owned Compose stack): a two-step bound
  Workflow runs and the run view reports both steps' outcomes.

## Notes

- Full upstream output is already on the upstream step's `TaskRun.Output`; the
  fold only derives the 500-rune `OutputSummary` for display. So this slice adds
  no new output column — it reads the bound step's TaskRun at dispatch.
- Labelled-untrusted framing follows workflow-runtime.md §2.3 and §9.1: upstream
  model output is explicit untrusted context, not Agent policy.
- Forward-compatible with §6: bind-prior-output-by-name into input carries over
  when `steps` is later replaced by `nodes`; this is a precursor, not a schema
  version 0.
