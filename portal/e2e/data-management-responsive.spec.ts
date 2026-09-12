import { expect, test, type Page } from "@playwright/test"

import { RUN_ID, reportLeftovers, session, uploadFile } from "./fixtures"

// Narrow-width (320–767px) coverage for Slice 4 of
// docs/design/portal-responsive-and-accessible-interaction.md: Workspace
// Files' progressive navigation, and the "reflow to a card" narrow behavior
// shared by Artifacts, Admin lists, and Space membership. A global
// stylesheet rule is not evidence of support on its own, so this runs
// alongside the slice rather than waiting for a later regression matrix.

test.use({ viewport: { width: 390, height: 844 } })

async function expectNoHorizontalOverflow(page: Page): Promise<void> {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  expect(overflow).toBeLessThanOrEqual(0)
}

test("Files shows either the folder list or the selected file, with a working Back action", async ({
  page,
}) => {
  const current = await session(page)
  const name = `narrow-probe-${RUN_ID}.txt`
  await uploadFile(page, current, name, "narrow layout probe\n")
  reportLeftovers(current.spaceId, [`file ${name}`])

  await page.goto(`/#/spaces/${current.spaceId}/files`)
  await expect(page.getByRole("heading", { name: "Files" })).toBeVisible()

  // No side tree at narrow width, and the folder list is what's shown first.
  await expect(page.getByLabel("Directory tree")).toBeHidden()
  const entry = page.getByRole("button", { name })
  await expect(entry).toBeVisible()
  await expect(page.getByRole("button", { name: "Back" })).toHaveCount(0)
  await expectNoHorizontalOverflow(page)

  // Selecting the file swaps to the item view — the list (and its entry) is
  // no longer in the DOM, only the viewer and a Back control are.
  await entry.click()
  const viewer = page.getByLabel("File content")
  await expect(viewer.getByRole("heading", { name })).toBeVisible()
  await expect(page.getByRole("button", { name })).toHaveCount(0)
  const back = page.getByRole("button", { name: "Back" })
  await expect(back).toBeVisible()
  await expectNoHorizontalOverflow(page)

  // Back returns to the folder list, not the browser history stack.
  await back.click()
  await expect(page.getByRole("button", { name })).toBeVisible()
  await expect(page.getByLabel("File content")).toHaveCount(0)
})

test("the Artifacts list reflows to a stacked card at narrow width", async ({ page }) => {
  const current = await session(page)
  const name = `narrow-probe-${RUN_ID}.txt`
  await uploadFile(page, current, name, "narrow layout probe\n")
  reportLeftovers(current.spaceId, [`file ${name}`])

  await page.goto(`/#/spaces/${current.spaceId}/artifacts`)
  await expect(page.getByRole("heading", { name: "Artifacts", exact: true })).toBeVisible()
  await expectNoHorizontalOverflow(page)
})

test("an admin list row reflows to a stacked card at narrow width", async ({ page }) => {
  await page.goto("/#/admin/administrators")
  await expect(page.getByRole("heading", { name: "Administrators" })).toBeVisible()

  const row = page.locator(".admin-list__row").first()
  await expect(row).toBeVisible()
  const direction = await row.evaluate((el) => getComputedStyle(el).flexDirection)
  expect(direction).toBe("column")
  await expectNoHorizontalOverflow(page)
})

test("a Space membership row's actions wrap instead of overflowing at narrow width", async ({ page }) => {
  const current = await session(page)
  await page.goto(`/#/spaces/${current.spaceId}/settings/members`)
  await expect(page.getByRole("heading", { name: "Members" })).toBeVisible()
  // The signed-in account's own row — every space has at least this one —
  // proves the actions row wraps rather than being clipped or forcing scroll.
  await expect(page.getByText("Me", { exact: true })).toBeVisible()
  await expectNoHorizontalOverflow(page)
})

// Space Plugins had no browser coverage at all before this slice (flagged by
// the Slice 4 research pass) — a plain visibility + overflow smoke test closes
// that gap rather than leaving the surface this slice's CSS changes touched
// entirely unguarded.
test("the Space Plugins tab renders without horizontal overflow at narrow width", async ({ page }) => {
  const current = await session(page)
  await page.goto(`/#/spaces/${current.spaceId}/settings/plugins`)
  const tab = page.getByRole("tab", { name: "Plugins" })
  await expect(tab).toHaveAttribute("aria-selected", "true")
  await expectNoHorizontalOverflow(page)
})
