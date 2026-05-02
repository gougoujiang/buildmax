# Phase 2: Outcome Surface

## Status

- phase: `2`
- name: `Outcome Surface`
- status: `not_started`
- roadmap: [design/018-versioned-workspace-and-outcome-roadmap.md](./018-versioned-workspace-and-outcome-roadmap.md)
- depends_on: [design/019-phase-1-product-and-docs-reset.md](./019-phase-1-product-and-docs-reset.md)
- created_at: `2026-05-01`

---

## 1. Goal

Make BuildMax present **results, deliverables, and produced artifacts** at the user-facing layer, instead of making users primarily navigate execution internals.

This phase should make the product feel more like:

- "I asked for work and received outputs"

and less like:

- "I need to inspect tasks, runs, and step records to understand what happened"

---

## 2. Problem Statement

The current system already has solid execution visibility:

- issue detail shows status, assignee, execution summary, timeline, run history, and step state
- workflow run detail exposes workflow execution internals
- task and artifact APIs exist
- conversation is the main user interface for asking the system to do work

But the current product still emphasizes **execution mechanics** more than **outcomes**.

### 2.1 What Users Can See Today

Users can already inspect:

- latest workflow run status
- latest agent task status
- workflow steps
- run history
- agent run sequence
- conversation traces

### 2.2 What Is Still Missing

Users still do **not** get a strong outcome-first experience for questions like:

- what was produced for this issue?
- what is the latest result I should read?
- what output files are available?
- which result is the most important one?
- what should I do next?

The system has the underlying data, but it is scattered across:

- `workflow_run`
- `workflow_step_run`
- `task`
- `task_run_artifact`
- conversation messages

That makes the product feel more like an execution console than an AI-native work surface.

---

## 3. Desired Outcome

After this phase:

- issue detail feels like the canonical place to receive work results
- conversation detail can point to concrete outputs, not just message text
- artifact/result surfaces become first-class UI sections
- task/run pages become supporting drill-down views rather than the main destination

### 3.1 MVP Decision

This phase should optimize for:

- high user-visible value
- low schema risk
- reuse of existing execution and artifact infrastructure

The MVP should **not** attempt to solve full workspace versioning or generalized document editing.

The target is:

- aggregate results at the issue layer
- expose output objects at the conversation layer where practical
- provide lightweight previews for common outputs
- define a stable "latest result" / "produced outputs" product pattern

---

## 4. In Scope

### 4.1 Issue-Level Output Aggregation

Extend the issue experience so it can directly show:

- latest produced artifacts
- latest output summaries
- links to the conversation or run that produced them
- a clearer distinction between:
  - execution metadata
  - actual deliverables

Minimum target:

- a dedicated "Results" or "Outputs" section on issue detail
- latest output first
- stable linking back to the producing run or conversation when needed

### 4.2 Conversation-Visible Output Objects

Conversation should become a better place to receive outcomes.

This does **not** mean turning conversation into a full document editor.

It means the conversation UI should be able to surface:

- result cards
- artifact links
- lightweight previews
- clear associations between a background task completion and the outputs it produced

### 4.3 Lightweight Output Previews

Support lightweight preview behavior for the most common output types.

Initial target types:

- Markdown / text
- simple structured text outputs where the content can be shown inline

Initial behavior can be modest:

- preview snippet
- "open full content" interaction
- clear filename / relative path / source context

### 4.4 Stable Result Linking

Define one stable way to move between:

- issue
- conversation
- workflow run or agent-backed task
- produced outputs

This should reduce the need for users to infer relationships from IDs or timelines.

### 4.5 Product Wording

Use product language that emphasizes:

- results
- outputs
- produced files
- latest deliverable

instead of leading with:

- task
- run
- step
- artifact internals

---

## 5. Out Of Scope

- hidden Git / versioned workspace implementation
- restore UX
- generalized file editor in Portal
- large artifact gallery system
- custom file-type renderers for every format
- workflow capability expansion
- replacing the current workflow/task execution model

---

## 6. Current Code Touch Points

Likely touch points for this phase:

- [portal/src/pages/IssueDetail.tsx](../portal/src/pages/IssueDetail.tsx)
- [portal/src/features/conversations/components/ConversationDetailView.tsx](../portal/src/features/conversations/components/ConversationDetailView.tsx)
- [portal/src/features/conversations/hooks/useConversationDetail.ts](../portal/src/features/conversations/hooks/useConversationDetail.ts)
- [portal/src/lib/types.ts](../portal/src/lib/types.ts)
- [portal/src/lib/api/types.ts](../portal/src/lib/api/types.ts)
- [portal/src/lib/api/mappers.ts](../portal/src/lib/api/mappers.ts)
- [internal/server/portal/register.go](../internal/server/portal/register.go)
- [internal/server/portal/issues.go](../internal/server/portal/issues.go)
- [internal/server/portal/tasks.go](../internal/server/portal/tasks.go)
- [internal/server/portal/artifacts.go](../internal/server/portal/artifacts.go)
- [internal/core/issue/service.go](../internal/core/issue/service.go)
- [internal/core/model/db_repositories.go](../internal/core/model/db_repositories.go)

Relevant existing capabilities:

- issue-centric flow endpoint:
  - `GET /api/teams/{team_id}/issues/{issue_id}/flow`
- task artifact listing:
  - `GET /api/teams/{team_id}/tasks/{task_id}/artifacts`
- task-run artifact item/content endpoints:
  - `GET /api/teams/{team_id}/task-runs/{task_run_id}/artifacts/items`
  - `GET /api/teams/{team_id}/task-runs/{task_run_id}/artifacts/content`

---

## 7. Implementation Steps

### Step 1. Reconfirm Current Result Surfaces

Audit the current user-facing result path across:

- issue detail
- conversation detail
- workflow run detail
- task artifact APIs

and identify exactly where the user still has to navigate into execution internals to reach an output.

### Step 2. Define The MVP Output Model

Choose the minimum stable product model for surfaced outcomes.

Recommended MVP shape:

- `latest_result`
- `outputs[]`
- each output includes:
  - label/title
  - source kind
  - source id
  - relative path or inline content snippet
  - created_at

This can be an API response shape or an aggregation DTO without changing the underlying persistence model first.

### Step 3. Add Issue-Level Aggregation

Extend issue-facing backend aggregation so issue detail can directly render:

- latest outputs
- output previews/snippets
- links to source conversation or run

### Step 4. Add Conversation-Level Output Surfacing

Add conversation UI/backend support so recent background-task completions can expose:

- output cards
- output links
- inline preview where safe and cheap

### Step 5. Refine Wording And Navigation

Adjust labels, headings, and actions so the product leads with outcomes.

Examples:

- `Results`
- `Outputs`
- `Open latest deliverable`
- `Produced by workflow run`

instead of pushing users first toward raw task/run detail.

### Step 6. Validate The New User Path

Confirm that a user can:

1. create or run work
2. stay on issue or conversation
3. see the produced outputs directly
4. drill down only if they need execution details

---

## 8. Validation / Acceptance Checks

This phase is acceptable when:

- issue detail has a direct, useful output/result section
- users can reach the most relevant produced output without first opening task/run detail pages
- conversation can surface at least lightweight output objects tied to recent work
- output cards/previews use stable product language and clear source context
- lower-level execution pages remain available as drill-downs, not the default path

Recommended validation:

- focused Go tests for any new issue/conversation aggregation behavior
- Portal build validation
- manual UX verification using:
  - one workflow-assigned issue
  - one agent-assigned issue
  - at least one produced artifact/result file

---

## 9. Open Questions

1. Should the first surfaced output model be issue-centric only, or should conversation and issue be updated in the same slice?
2. Should result previews come from artifact files, task output text, or both?
3. For conversation UI, should outputs appear inline as special message cards, or as a side/result panel attached to the conversation view?
4. Should issue detail show only the latest outputs by default, or also expose grouped historical outputs from earlier runs?
5. Which output types deserve inline preview in the MVP beyond plain text / Markdown?

---

## 10. Recommended Immediate Next Step

The next implementation conversation for this phase should start with the smallest vertical slice:

1. extend issue-level aggregation to surface latest outputs
2. add a `Results` section to issue detail
3. reuse existing artifact endpoints or add one small aggregation endpoint if needed
4. validate with one workflow-backed issue and one agent-backed issue

This gives users the first strong outcome-first experience without waiting for conversation-level polish or later workspace-versioning phases.
