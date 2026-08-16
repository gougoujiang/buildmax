import { expect, test } from "@playwright/test"

// What only a browser proves: the published bundle works against a real
// server. The Portal image ships one bundle for every deployment and reads its
// API base at container start, so a wrong or missing runtime config produces a
// page that renders and then fails every request — invisible to a build, and
// invisible to the API-level smoke.

test("the signed-in shell renders against a real deployment", async ({ page }) => {
  await page.goto("/")

  // The login card is what an unauthenticated or misconfigured app falls back
  // to, so its absence is half the assertion.
  await expect(page.locator(".login-page__card")).toHaveCount(0)
  // The other half: the shell actually rendered. Navigation is asserted rather
  // than the wordmark, which is ASCII art and carries no text to match.
  for (const item of ["Issues", "Workflows", "Agents"]) {
    await expect(page.getByRole("link", { name: item }).or(page.getByRole("button", { name: item })).first()).toBeVisible()
  }
})

test("the runtime API base is same-origin, as the ingress serves it", async ({ page }) => {
  const config = await page.request.get("/config.js")
  expect(config.ok()).toBeTruthy()
  // Same-origin is what the deployment reference's single-ingress shape
  // depends on. An absolute URL here means the bundle is talking to a host the
  // browser may not be allowed to reach.
  expect(await config.text()).toContain('apiBase: "/"')
})

test("a reload keeps the session", async ({ page }) => {
  await page.goto("/")
  await page.reload()
  // Session restoration lives entirely in the browser, so nothing below the UI
  // can test it.
  await expect(page.locator(".login-page__card")).toHaveCount(0)
})
