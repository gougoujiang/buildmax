import { expect, test } from "@playwright/test"

import { postJSON, reportLeftovers, session, tagged } from "./fixtures"

/**
 * A schedule is managed on its agent's detail page, so the Schedules tab is
 * where an operator meets one. This is browser-only: the API smoke never renders
 * a schedule, and the handler tests never route to the agent detail view. The
 * dispatcher that fires a due schedule is covered by Go tests; this asserts the
 * surface a person uses to create and read one.
 */
test("a schedule is listed on its agent's Schedules tab", async ({ page }) => {
  const current = await session(page)

  const agent = await postJSON<{ id: string }>(page, `${current.space}/agents`, current, {
    name: tagged("Schedule probe agent"),
    description: "Created by the Portal browser tests.",
    instructions: "Reply with exactly: deployment smoke ok",
  })
  const name = tagged("Nightly probe")
  const schedule = await postJSON<{ id: string }>(page, `${current.space}/schedules`, current, {
    agent_id: agent.id,
    name,
    input: "Summarize the new issues",
    cron_expr: "0 9 * * *",
    timezone: "UTC",
  })
  reportLeftovers(current.spaceId, [`agent ${agent.id}`, `schedule ${schedule.id}`])

  await page.goto(`/#/spaces/${current.spaceId}/agents/${agent.id}`)
  await page.getByRole("button", { name: "Schedules", exact: true }).click()

  const list = page.locator(".agent-schedules__list")
  await expect(list.getByText(name, { exact: true })).toBeVisible()
  // The cron rule and its enabled state are what tell an operator when it runs.
  await expect(list.getByText("0 9 * * *", { exact: true })).toBeVisible()
  await expect(list.getByText("Enabled", { exact: true })).toBeVisible()
})
