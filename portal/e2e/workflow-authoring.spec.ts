import { expect, test } from "@playwright/test"

import { postJSON, reportLeftovers, session, tagged } from "./fixtures"

/**
 * `agent_task` is the only step type the runtime executes, so the normal
 * Workflow editor presents an Agent step rather than a free-form Type field,
 * and a step's id is generated rather than typed. The advanced JSON view
 * exists for exact inspection, not as a second, unchecked way to build the
 * same workflow -- both paths run the same validation before Save is
 * enabled. None of that is provable without actually rendering the form:
 * a handler test can assert the API rejects a bad definition, not that the
 * Portal form never lets someone type one in the first place.
 */

test("the Agent-step form has no free-form Type field or editable step id, and creates a workflow", async ({
  page,
}) => {
  const current = await session(page)
  const agentName = tagged("Workflow authoring probe agent")
  const agent = await postJSON<{ id: string }>(page, `${current.space}/agents`, current, {
    name: agentName,
    description: "Created by the Portal browser tests.",
    instructions: "Reply with exactly: deployment smoke ok",
  })
  reportLeftovers(current.spaceId, [`agent ${agent.id}`])

  await page.goto("/#/workflows")
  await page.getByRole("button", { name: "New Workflow" }).click()

  const dialog = page.getByRole("dialog", { name: "New Workflow" })
  await expect(dialog).toBeVisible()

  // The step card names what it is -- an Agent step -- and shows its
  // generated id as read-only text, not as an input a person could edit.
  await expect(dialog.getByText("Agent Step 1", { exact: true })).toBeVisible()
  await expect(dialog.getByText(/^id: step_/)).toBeVisible()
  await expect(dialog.getByLabel("Type")).toHaveCount(0)
  await expect(dialog.getByLabel("Step ID")).toHaveCount(0)

  await dialog.getByLabel("Name").fill(tagged("Workflow authoring probe"))
  await dialog.getByLabel("Agent").selectOption({ label: `${agentName} (${agent.id})` })
  await dialog.getByLabel("Prompt").fill("Reply with exactly: deployment smoke ok")

  const submit = dialog.getByRole("button", { name: "Create workflow" })
  await expect(submit).toBeEnabled()
  await submit.click()

  await expect(dialog).toBeHidden()
  await expect(page.getByText(tagged("Workflow authoring probe"), { exact: true })).toBeVisible()
})

test("advanced JSON mode is checked against the same validation as the step form", async ({ page }) => {
  const current = await session(page)
  const agent = await postJSON<{ id: string }>(page, `${current.space}/agents`, current, {
    name: tagged("Workflow authoring validation agent"),
    description: "Created by the Portal browser tests.",
    instructions: "Reply with exactly: deployment smoke ok",
  })
  reportLeftovers(current.spaceId, [`agent ${agent.id}`])

  await page.goto("/#/workflows")
  await page.getByRole("button", { name: "New Workflow" }).click()

  const dialog = page.getByRole("dialog", { name: "New Workflow" })
  await dialog.getByLabel("Name").fill(tagged("Workflow authoring validation"))

  await dialog.getByRole("button", { name: "Advanced: edit raw JSON" }).click()
  const definitionField = dialog.getByLabel(/^Definition \(JSON\)/)
  await expect(definitionField).toBeVisible()

  // A step type this Portal build does not know how to run -- exactly what
  // the normal form makes impossible to type in the first place.
  await definitionField.fill(
    JSON.stringify({
      steps: [{ step_id: "s1", type: "shell_command", target_agent_id: agent.id, prompt: "rm -rf /" }],
    })
  )

  const submit = dialog.getByRole("button", { name: "Create workflow" })
  await expect(submit).toBeDisabled()
  await expect(dialog.getByText(/shell_command.*not.*support/i)).toBeVisible()

  // The same JSON, with the one field the runtime actually supports, is
  // accepted -- proving the block above was the type, not the JSON mode.
  await definitionField.fill(
    JSON.stringify({
      steps: [{ step_id: "s1", type: "agent_task", target_agent_id: agent.id, prompt: "Reply with exactly: deployment smoke ok" }],
    })
  )
  await expect(submit).toBeEnabled()
})
