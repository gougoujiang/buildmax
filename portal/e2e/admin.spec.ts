import { expect, test } from "@playwright/test"

// The administration area is deployment-scoped and reachable only through the
// UI, so this is the first place its wiring runs end to end: hash route,
// /api/admin authorization, and rendering. `./make e2e` grants the test account
// system_admin, which is what makes these reachable at all.

test("an administrator can open the deployment overview by URL", async ({ page }) => {
  await page.goto("/#/admin")

  await expect(page.getByRole("heading", { name: "Administration" })).toBeVisible()
  await expect(page.getByRole("heading", { name: "Health" })).toBeVisible()

  // The status is answered rather than falling into the error branch. Whether
  // the deployment is ready is not this test's business — that it could say is.
  await expect(page.locator(".admin-pill").first()).toBeVisible()
  await expect(page.locator(".settings-section__error")).toHaveCount(0)
})

test("each administration section is linkable and survives a reload", async ({ page }) => {
  for (const [path, heading] of [
    ["/#/admin/accounts", "Accounts"],
    ["/#/admin/teams", "Spaces"],
    ["/#/admin/models", "Models"],
    ["/#/admin/audit", "Audit trail"],
  ] as const) {
    await page.goto(path)
    await expect(page.getByRole("heading", { name: heading })).toBeVisible()
    await page.reload()
    await expect(page.getByRole("heading", { name: heading })).toBeVisible()
    await expect(page.locator(".settings-section__error")).toHaveCount(0)
  }
})

test("the audit search reaches the events that have no space", async ({ page }) => {
  await page.goto("/#/admin/audit")
  await expect(page.getByRole("heading", { name: "Audit trail" })).toBeVisible()

  // Logins and grants are recorded with no team, so the space-scoped trail can
  // never return them. This deployment has at least the test account's own
  // login and grant, so the filter must find something.
  await page.getByRole("button", { name: "Deployment only" }).click()
  await expect(page.locator(".audit-row").first()).toBeVisible()
  await expect(page.locator(".settings-section__error")).toHaveCount(0)
})
