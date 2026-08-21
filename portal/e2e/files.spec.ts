import { expect, test } from "@playwright/test"

import { RUN_ID, reportLeftovers, session, uploadFile } from "./fixtures"

/**
 * Team storage has a browser half that nothing below the UI exercises.
 *
 * The API smoke uploads a file and reads it back through the API, which proves
 * storage works and says nothing about whether a person can find the file. The
 * tree, the folder selection, and the viewer are only assembled in the browser,
 * and they are the only way a file reaches a reader.
 */

const CONTENT = "explorer probe content\n"

test("an uploaded file is listed and readable in Explore", async ({ page }) => {
  const current = await session(page)
  const name = `explorer-probe-${RUN_ID}.txt`
  await uploadFile(page, current, name, CONTENT)
  reportLeftovers(current.teamId, [`file ${name}`])

  // Straight to the URL: a view reachable only by clicking cannot be linked,
  // and the specs beside this one hold routing to the same rule.
  await page.goto("/#/explore")
  await expect(page.getByRole("heading", { name: "Explore" })).toBeVisible()

  // The root folder is selected on load, which is where an upload lands.
  const entry = page.getByRole("button", { name })
  await expect(entry).toBeVisible()

  await entry.click()
  const viewer = page.getByLabel("File content")
  await expect(viewer.getByRole("heading", { name })).toBeVisible()
  await expect(viewer.locator("pre")).toHaveText(CONTENT.trim())
  await expect(viewer.locator("[role=alert]")).toHaveCount(0)
})
