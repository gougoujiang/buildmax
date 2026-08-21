import { expect, test } from "@playwright/test"

import { MEMBER_STATE } from "./global-setup"

/**
 * Authority over the deployment is not authority inside a space, and the Portal
 * has to act like it for someone who holds neither.
 *
 * The server refuses an ungranted account regardless — this is presentation,
 * not enforcement — but presentation is what a person meets. Only a browser
 * signed in as somebody without the grant can show what they get, and the
 * deployment smoke's own account cannot: it holds `system_admin`.
 */

test.use({ storageState: MEMBER_STATE })

test("an account without a grant is sent away from the admin area", async ({ page }) => {
  await page.goto("/#/admin")

  // Sent home rather than shown a forbidden screen: there is nothing there to
  // tell them about. The heading is the whole area's marker.
  await expect(page.getByRole("heading", { name: "Administration" })).toHaveCount(0)
  await expect(page).toHaveURL(/#\/?$|#\/home/)
})

test("an ungranted account still has a space of its own", async ({ page }) => {
  // The refusal above has to be a boundary holding, not a session that never
  // worked: the same account reaches its own space settings.
  await page.goto("/#/space/overview")
  await expect(page.locator(".login-page__card")).toHaveCount(0)
  await expect(page.getByRole("heading", { name: "Administration" })).toHaveCount(0)
})
