import { expect, test, type Locator, type Page } from "@playwright/test"

import { getJSON, session } from "./fixtures"

/**
 * The space overview is where an operator reads the deployment's own settings
 * back: which quota tier this server was configured with, and how much of it
 * the team has spent.
 *
 * None of it is a Portal decision. `default_quota_tier` is a line in the
 * deployment's `server.yaml`, and the counts are what the server totalled over
 * its own usage window — so a handler test cannot know what the deployment was
 * configured to say, and the API smoke never renders it. What this spec adds is
 * the last link: the number an operator acts on is the number the deployment
 * reported, not a placeholder the UI fell back to.
 *
 * The audit section is covered separately, in space-audit.spec.ts.
 */

interface Usage {
  run_count: number
  total_tokens: number
  tier: string
  period_days: number
  max_runs_per_period?: number
  max_tokens_per_period?: number
}

/** The value cell of one overview row, addressed by its exact term. */
function summaryValue(page: Page, term: string): Locator {
  const rows = page.locator(".team-settings-page__summary > div")
  return rows.filter({ has: page.locator("dt", { hasText: new RegExp(`^${term}$`) }) }).locator("dd")
}

test("the space overview reports the deployment's quota tier and what it counted", async ({ page }) => {
  const current = await session(page)
  const usage = await getJSON<Usage>(page, `${current.team}/usage`, current)

  // Reachable by URL, not only by clicking through the tabs: a section an
  // operator cannot link to is one they cannot send to a colleague.
  await page.goto("/#/space")
  await expect(page.getByRole("heading", { name: "Space", exact: true }).first()).toBeVisible()

  // Compared against what the deployment answered rather than a hard-coded
  // tier. The claim is that Portal reports this server, and pinning the value
  // here would instead assert what the smoke's server.yaml happens to say.
  await expect(summaryValue(page, "Quota tier")).toHaveText(usage.tier)
  await expect(summaryValue(page, "Your role")).toHaveText("owner")

  // The limit half is the part that has to have crossed the wire. Rendered
  // without one, this cell silently drops to a bare count, which reads as a
  // team with no quota rather than as a deployment that failed to report it.
  expect(usage.max_runs_per_period, "the deployment reported no run limit").toBeGreaterThan(0)
  await expect(summaryValue(page, "Runs this period")).toHaveText(
    `${usage.run_count} / ${usage.max_runs_per_period}`
  )
  await expect(page.getByText(`Current usage window: last ${usage.period_days} days.`)).toBeVisible()

  await expect(page.locator(".settings-section__error")).toHaveCount(0)
})

test("the space members section names the signed-in account", async ({ page }) => {
  await page.goto("/#/space/members")

  // Owner-only controls are the point: whether this account may remove members
  // is decided from the membership the deployment returned, and the section is
  // the only place that decision is visible.
  const members = page.locator(".settings-page__section").filter({
    has: page.getByRole("heading", { name: "Members" }),
  })
  await expect(members.getByText("Me", { exact: true })).toBeVisible()
  await expect(page.locator(".settings-section__error")).toHaveCount(0)
})
