# Portal Outcome Surface

## Status

- roadmap_priority: `P2`
- status: `done`
- follows: [024-agent-core-stability.md](./024-agent-core-stability.md), [025-local-agent-experience.md](./025-local-agent-experience.md)
- roadmap: [../ROADMAP.md](../../ROADMAP.md)
- created_at: `2026-05-17`
- completed_at: `2026-05-23`

## 1. Decision

With P0 Agent Core Stability and P1 Local Agent Experience treated as complete,
the next product move is P2: make the Portal an **outcome surface**.

The Portal already exposes issues, workflows, conversations, tasks, runs, and
artifacts. The gap is not execution capability. The gap is that users still need
to inspect execution internals to answer the simplest question:

> What did BuildMax produce for this issue?

The next implementation should make issue detail and conversation detail show
direct results, produced outputs, and lightweight previews before they show raw
task/run mechanics.

## 2. Product Goal

Opening an issue should immediately show:

- the latest deliverable, if one exists
- produced output files
- concise previews for Markdown/text outputs
- where each output came from
- drill-down links to the producing conversation, task, run, or workflow run

Task, task-run, workflow-run, and step pages remain available, but they become
diagnostic drill-downs. The primary surface is the issue or conversation where
the user asked for the work.

## 3. Current Baseline

The current codebase already has most of the raw material.

Backend:

- `GET /api/teams/{team_id}/issues/{issue_id}/flow` returns the issue, assigned
  workflow, workflow runs, workflow step runs, and issue-linked agent tasks.
- `GET /api/teams/{team_id}/tasks/{task_id}/artifacts` lists run outputs for a
  task.
- `GET /api/teams/{team_id}/task-runs/{task_run_id}/artifacts/items` lists
  artifact files for a task run.
- `GET /api/teams/{team_id}/task-runs/{task_run_id}/artifacts/content` returns
  artifact content, defaulting to `result.md`.
- `Task.Output`, `Task.LastRunID`, `WorkflowStepRun.OutputSummary`, and
  `task_run_artifact` rows already carry result-related information.

Frontend:

- `portal/src/pages/issues/IssueDetail.tsx` shows execution summary, flow steps,
  timeline, run history, and agent task sequence.
- `portal/src/pages/conversations/ConversationDetail.tsx` shows the message
  thread.
- `portal/src/features/artifacts/api.ts` already wraps artifact list, item, and
  content endpoints.
- `portal/src/lib/api/types.ts` and `portal/src/lib/types.ts` have task,
  workflow, issue, and artifact DTOs.

The current user path is execution-first:

1. Open issue.
2. Inspect execution summary, flow steps, run history, or agent task sequence.
3. Navigate to a conversation or run.
4. Infer which task/run produced the useful output.
5. Open artifact content from a lower-level surface.

P2 should make that path outcome-first:

1. Open issue.
2. Read latest result or output preview.
3. Open full output if needed.
4. Drill into execution details only for debugging or provenance.

## 4. MVP Scope

### 4.1 In Scope

- Add an issue-level `outputs[]` aggregation.
- Add a single `latest_result` object derived from `outputs[]`.
- Show an issue detail `Results` section above execution internals.
- Surface output cards in conversation detail for background tasks in that
  conversation.
- Preview safe text-like outputs:
  - `result.md`
  - `.md`
  - `.txt`
  - `.json`, `.yaml`, `.yml`, `.csv` as plain text snippets
- Provide clear provenance:
  - source type
  - source ID
  - task ID
  - task run ID
  - workflow run ID / step ID when known
  - conversation link
- Keep task/run/workflow-run pages as drill-downs.

### 4.2 Out Of Scope

- Full workspace versioning.
- Git-backed restore UI.
- Rich document editing.
- Binary/image/PDF previews.
- A broad artifact gallery.
- A workflow engine rewrite.
- New persistent result tables in the first slice.

The first slice should be an aggregation DTO over existing stores and object
storage. Add durable result tables only after the product shape proves useful.

## 5. Product Model

Use product language that matches what users are looking for.

- `Result`: the most important current answer or deliverable for a work object.
- `Output`: a produced file or text object.
- `Preview`: a bounded inline view of an output.
- `Source`: the execution object that produced the output.
- `Drill-down`: task, task run, workflow run, step, and conversation detail.

Avoid leading labels like `artifact`, `task_run`, and `step_run` in primary UI
copy. Keep those labels in metadata, tooltips, URLs, and diagnostics.

## 6. API Shape

Prefer adding this shape to the existing issue flow endpoint first:

```json
{
  "issue": {},
  "workflow": null,
  "runs": [],
  "agent_tasks": [],
  "latest_result": {
    "id": "out_r_abc_result_md",
    "title": "Latest result",
    "kind": "markdown",
    "relative_path": "result.md",
    "preview": "The generated result starts here...",
    "preview_truncated": true,
    "source": {
      "source_type": "task_run",
      "task_id": "t_abc",
      "task_run_id": "r_abc",
      "conversation_id": "c_abc",
      "workflow_run_id": "wr_abc",
      "workflow_step_run_id": "wsr_abc"
    },
    "created_at": 1779010000
  },
  "outputs": [],
  "total": 1
}
```

Recommended Go response DTOs in `internal/server/handlers/issues.go`:

```go
type outputSourceResponse struct {
	SourceType        string  `json:"source_type"`
	TaskID            string  `json:"task_id,omitempty"`
	TaskRunID         string  `json:"task_run_id,omitempty"`
	ConversationID    string  `json:"conversation_id,omitempty"`
	WorkflowRunID     *string `json:"workflow_run_id,omitempty"`
	WorkflowStepRunID *string `json:"workflow_step_run_id,omitempty"`
	WorkflowStepID    *string `json:"workflow_step_id,omitempty"`
}

type issueOutputResponse struct {
	ID               string               `json:"id"`
	Title            string               `json:"title"`
	Kind             string               `json:"kind"`
	RelativePath     string               `json:"relative_path,omitempty"`
	Preview          string               `json:"preview,omitempty"`
	PreviewTruncated bool                 `json:"preview_truncated"`
	Source           outputSourceResponse `json:"source"`
	CreatedAt        int64                `json:"created_at"`
}
```

Recommended TypeScript DTOs in `portal/src/lib/api/types.ts` should mirror the
snake_case API shape. Domain types in `portal/src/lib/types.ts` can use camelCase.

## 7. Aggregation Rules

### 7.1 Candidate Sources

Collect candidates from the existing issue flow data:

- issue agent tasks from `TaskStore.ListTasksByIssue`
- workflow steps from `WorkflowStore.ListWorkflowStepRuns`
- task IDs and task run IDs attached to workflow step runs
- task output and latest run IDs attached to issue-linked tasks
- task-run artifact items from `RunOutputLister`
- result content from `ArtifactStorage.GetResult`

### 7.2 Ordering

Order outputs by `created_at DESC`.

For issue detail:

1. outputs from the latest successful run
2. outputs from other completed runs
3. task output text when no artifact exists
4. failed-run error summaries only in execution sections, not as outputs

For conversation detail:

1. tasks in the current conversation, newest first
2. each task's latest run output first
3. older outputs collapsed behind an "All outputs" affordance later

### 7.3 Preview Rules

- Preview only text-like files in the MVP.
- Default to `result.md` when present.
- Limit previews to a small byte/rune budget, for example 4 KB server-side and
  a visually shorter card client-side.
- Mark `preview_truncated: true` when content is clipped.
- Never fail the entire issue flow response because one preview cannot be read.
  Return the output card without preview and include normal server logs.

### 7.4 Output Identity

The first slice can use deterministic synthetic IDs:

```text
out_<task_run_id>_<clean_relative_path>
```

The ID is a UI/API identifier, not a persisted entity ID. If results become
persisted later, introduce a normal prefixed entity ID through
`internal/util.NewPrefixedID`.

## 8. Backend Plan

### Step 1. Add Aggregation Helpers

Create small helper functions near the issue handler first. If they grow, move
them into `internal/service/issue`.

Responsibilities:

- map workflow step runs by `task_id` and `task_run_id`
- collect issue-linked tasks
- list output files for completed task runs
- choose `result.md` as the primary output
- read bounded previews for safe text paths
- tolerate missing artifact content

Keep the persistence model unchanged.

### Step 2. Extend Issue Flow Response

Extend `issueFlowResponse` with:

- `LatestResult *issueOutputResponse json:"latest_result,omitempty"`
- `Outputs []issueOutputResponse json:"outputs"`

Update `getIssueFlowHandler` to populate those fields when artifact stores are
configured. If artifact stores are not configured, still return the existing
flow response with an empty `outputs` array.

### Step 3. Add Conversation Output Aggregation

Add a conversation-scoped outputs endpoint after issue output cards work:

```text
GET /api/teams/{team_id}/conversations/{conversation_id}/outputs
```

Response:

```json
{
  "latest_result": null,
  "outputs": []
}
```

This keeps conversation messages clean while allowing the UI to render output
cards alongside the thread.

### Step 4. Tests

Add focused handler tests:

- issue flow includes outputs for an issue agent task with `result.md`
- issue flow includes outputs for a workflow step task run
- missing artifact content does not fail issue flow
- outputs are team-scoped and cannot leak across teams
- conversation outputs only include tasks in that conversation

## 9. Frontend Plan

### Step 1. Add Types And Mappers

Add API/domain types:

- `ApiOutputSource`
- `ApiIssueOutput`
- `OutputSource`
- `IssueOutput` or shared `Output`

Map snake_case fields to camelCase in `portal/src/lib/api/mappers.ts`.

### Step 2. Build Reusable Output Card Components

Add presentational components in Portal first:

- `OutputCard`
- `OutputPreview`
- `OutputsList`

A later pass can move them into `gui/` if Desktop or shared surfaces need them.

Card content:

- output title
- kind/path
- preview
- created time
- produced-by metadata
- actions:
  - open full output
  - open conversation
  - open run detail when a workflow run exists

### Step 3. Issue Detail Results Section

In `portal/src/pages/issues/IssueDetail.tsx`, add `Results` before
`Execution Summary`.

Display states:

- latest result card when present
- additional outputs collapsed or listed below
- empty state: "No results produced yet."
- running state can keep using existing execution status

The issue header copy should shift from execution inspection to outcome review.

### Step 4. Full Output Viewer

For the MVP, use an in-page modal or detail panel that loads full content via:

```text
GET /api/teams/{team_id}/task-runs/{task_run_id}/artifacts/content?path=...
```

Do not add a new route until output viewing needs deep-linking.

### Step 5. Conversation Output Cards

After the issue surface lands, show conversation outputs near the message thread:

- a compact `Outputs` strip above the composer, or
- cards interleaved after task-result assistant messages if the association is
  reliable.

Prefer a compact side/strip in the first slice because current messages do not
carry structured output references.

## 10. Milestones

### M1. Issue Results Backend

- Add output DTOs.
- Extend issue flow response.
- Aggregate `result.md` and task output previews.
- Add backend tests.

Acceptance:

- one issue agent run can produce a visible `latest_result`
- one workflow-backed issue can produce step-linked outputs
- existing issue flow clients continue to work

### M2. Issue Results UI

- Add Portal types and mappers.
- Add output cards and preview modal.
- Put `Results` above execution internals on issue detail.
- Keep all existing run and conversation navigation.

Acceptance:

- opening an issue makes the latest deliverable obvious
- users can open the full output without visiting workflow run detail

### M3. Conversation Outputs

- Add conversation output endpoint.
- Add conversation output cards or strip.
- Keep message rendering unchanged except for the new output surface.

Acceptance:

- background task completion in a conversation points to concrete outputs
- users can open produced files from conversation detail

### M4. Language And Navigation Polish

- Rename primary UI copy from execution-first to result-first where appropriate.
- Keep diagnostics available under "Execution Details", "Run History", and
  "Flow Steps".
- Add empty/loading/error states for outputs.

Acceptance:

- issue detail reads as a result page first and an execution page second

## 11. Validation

Run after backend changes:

```sh
go test ./internal/server/handlers ./internal/service/issue ./internal/service/task ./internal/service/workflow
```

Run after frontend changes:

```sh
cd portal && npm run build
```

Run full project validation before merging:

```sh
./make test
```

Manual scenarios:

1. Agent-assigned issue:
   - create issue
   - assign agent
   - run agent
   - wait for task completion
   - confirm latest result appears on issue detail
2. Workflow-assigned issue:
   - assign published workflow
   - run workflow
   - confirm step-produced outputs appear on issue detail
3. Conversation:
   - open producing conversation
   - confirm output cards link to full content
4. Authorization:
   - use a second team
   - confirm outputs from the first team are not visible

## 12. Risks

- **N+1 artifact reads**: keep limits small and read previews only for visible
  issue outputs.
- **Ambiguous source mapping**: workflow step runs have task IDs and task run IDs;
  prefer exact `task_run_id`, then fall back to `task_id`.
- **Missing artifacts**: workers may complete with task output but no artifact
  file; show task output as a text result fallback.
- **UI overloading issue detail**: keep execution panels available but visually
  lower priority.
- **Premature persistence**: avoid new result tables until the aggregation model
  proves stable.

## 13. Open Questions

1. Should the issue flow endpoint include all output previews by default, or
   should previews be opt-in with `?include_outputs=1`?
2. Should output cards include failed-run diagnostics, or should failures stay
   only in execution sections?
3. Should conversation outputs be rendered as a side panel, top strip, or
   message-adjacent cards?
4. When multiple workflow steps produce outputs, should the issue default to the
   final successful step or the newest successful step?
5. Should `result.md` remain the privileged default output name long term?

## 14. Recommended First PR

The first PR should be a narrow vertical slice:

1. Extend `GET /api/teams/{team_id}/issues/{issue_id}/flow` with `outputs[]`
   and `latest_result`.
2. Aggregate `result.md` for issue-linked tasks and workflow step task runs.
3. Add a `Results` section to issue detail.
4. Reuse the existing artifact content endpoint for full output viewing.
5. Add focused backend tests and run the Portal build.

This delivers P2's product shift quickly without blocking on conversation cards,
new persistence, or versioned workspace design.
