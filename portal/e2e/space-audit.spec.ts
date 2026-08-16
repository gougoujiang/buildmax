import { expect, test } from "@playwright/test"

// The audit trail is owner-only and reachable only through the UI, so this is
// the first place its wiring runs end to end: hash route, API call,
// authorization, and rendering.

test("an owner can open the audit trail by URL", async ({ page }) => {
  // Routing is hash-based, and a section reachable only by clicking a tab
  // cannot be linked or survive a reload — so going straight to the URL is
  // part of the assertion.
  await page.goto("/#/space/audit")

  await expect(page.getByRole("heading", { name: "Audit trail" })).toBeVisible()

  // Either state is correct on a fresh deployment. What matters is that the
  // request succeeded rather than falling into the error branch.
  await expect(page.locator(".audit-list, .page-activity__empty").first()).toBeVisible()
  await expect(page.locator(".settings-section__error")).toHaveCount(0)
})

test("the audit section survives a reload", async ({ page }) => {
  await page.goto("/#/space/audit")
  await page.reload()
  await expect(page.getByRole("heading", { name: "Audit trail" })).toBeVisible()
})
