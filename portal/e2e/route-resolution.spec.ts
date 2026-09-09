import { expect, test } from "@playwright/test"

import { session } from "./fixtures"

/**
 * docs/design/portal-navigation-and-space-context.md requires unknown,
 * forbidden, and missing routes to be distinguishable and actionable --
 * never a silent fallback to Chat. Confirmed against the server
 * (internal/service/{agent,issue}/service.go and friends) that a
 * resource-by-id lookup returns a genuine 404 once the caller has already
 * passed the Space-level guard, so Issue detail's not-found state below is a
 * real 404, not a guess.
 */

// A syntactically plausible but never-issued public id: no account is ever a
// member of it, so every Space-scoped fetch against it hits the same 403 the
// server returns uniformly for "doesn't exist" and "not yours" -- see
// internal/server/access/space.go's SpaceAction.
const NO_SUCH_SPACE = "aaaaaaaaaaaaaaaaaaaa"
const NO_SUCH_ISSUE = "bbbbbbbbbbbbbbbbbbbb"

test("an unrecognized hash renders not-found, never Chat", async ({ page }) => {
  // Ensures the account's Spaces are resolved before navigating to the bad
  // hash, so "Back to Chat" below has a real Space id to land on rather than
  // racing SpaceContext's own first load.
  await session(page)
  await page.goto("/#/this/does/not/exist")
  await expect(page.getByRole("heading", { name: "Page not found" })).toBeVisible()
  await expect(page.getByRole("textbox", { name: "What would you like to do?" })).toHaveCount(0)

  // The one safe action works and lands on a real Space's Chat.
  await page.getByRole("button", { name: "Back to Chat" }).click()
  await expect(page).toHaveURL(/#\/spaces\/[^/]+\/chat$/)
})

test("an incomplete Space-scoped path renders not-found", async ({ page }) => {
  const current = await session(page)
  await page.goto(`/#/spaces/${current.spaceId}/workflow-runs`)
  await expect(page.getByRole("heading", { name: "Page not found" })).toBeVisible()
})

test("a Issue id that never existed renders not-found, not Chat", async ({ page }) => {
  const current = await session(page)
  await page.goto(`/#/spaces/${current.spaceId}/issues/${NO_SUCH_ISSUE}`)
  await expect(page.getByRole("heading", { name: "Issue", exact: true })).toBeVisible()
  await expect(page.getByText(/No issue with this reference exists/)).toBeVisible()

  await page.getByRole("button", { name: "Back to Issues" }).click()
  await expect(page).toHaveURL(new RegExp(`#/spaces/${current.spaceId}/issues$`))
})

test("a Space no account is a member of renders forbidden, not a blank page", async ({ page }) => {
  await session(page)
  await page.goto(`/#/spaces/${NO_SUCH_SPACE}/issues/${NO_SUCH_ISSUE}`)
  await expect(page.getByRole("heading", { name: "Issue", exact: true })).toBeVisible()
  await expect(page.getByText(/you don't have access to/i)).toBeVisible()
})
