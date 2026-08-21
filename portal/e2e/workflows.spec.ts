import { expect, test } from "@playwright/test"

import { postJSON, reportLeftovers, session, tagged } from "./fixtures"

/**
 * A workflow is team-scoped and reusable, so the list and the detail view are
 * where an operator meets one. Both are browser-only: the API smoke never
 * renders a workflow, and the handler tests never route to it.
 */

test("a workflow is listed, and its detail view opens by URL", async ({ page }) => {
  const current = await session(page)

  const agent = await postJSON<{ id: string }>(page, `${current.team}/agents`, current, {
    name: tagged("Workflow probe agent"),
    description: "Created by the Portal browser tests.",
    instructions: "Reply with exactly: deployment smoke ok",
  })
  const name = tagged("Workflow probe")
  const workflow = await postJSON<{ id: string }>(page, `${current.team}/workflows`, current, {
    name,
    description: "Created by the Portal browser tests to exercise the workflow views.",
    definition: JSON.stringify({
      steps: [{ step_id: "only", type: "agent_task", target_agent_id: agent.id, prompt: "Reply with exactly: deployment smoke ok" }],
    }),
  })
  reportLeftovers(current.teamId, [`agent ${agent.id}`, `workflow ${workflow.id}`])

  await page.goto("/#/workflows")
  await expect(page.getByRole("heading", { name: "Workflows", exact: true })).toBeVisible()
  await expect(page.getByText(name, { exact: true })).toBeVisible()

  // The detail route carries the id, so linking to it is the same claim as
  // reaching it by clicking — and it is the one an operator pastes to a
  // colleague.
  await page.goto(`/#/workflow/${workflow.id}`)
  await expect(page.getByRole("heading", { name: "Workflow Detail" })).toBeVisible()
  await expect(page.getByText(workflow.id, { exact: true })).toBeVisible()
  await expect(page.getByText("Workflow not found.")).toHaveCount(0)
})
