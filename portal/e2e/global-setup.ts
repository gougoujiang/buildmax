import { chromium } from "@playwright/test"
import { mkdir } from "node:fs/promises"
import { dirname } from "node:path"

const STATE_PATH = "./e2e/.auth/state.json"

/**
 * Sign in once and save the session for every spec.
 *
 * The login code is single-use, so this is also why the login path is not a
 * separate spec: it runs here, and a break in it fails the whole suite before
 * the first test — loudly, and with a screenshot.
 *
 * `./make e2e` issues the code and passes it in; a login code cannot be
 * fetched from the browser, since the whole point is that it arrives out of
 * band.
 */
async function globalSetup() {
  const baseURL = process.env.BUILDMAX_E2E_BASE_URL ?? "http://localhost:8080"
  const email = process.env.BUILDMAX_E2E_EMAIL
  const code = process.env.BUILDMAX_E2E_LOGIN_CODE
  if (!email || !code) {
    throw new Error(
      "BUILDMAX_E2E_EMAIL and BUILDMAX_E2E_LOGIN_CODE are required. Run these through `./make e2e`, which issues a code against the running deployment."
    )
  }

  await mkdir(dirname(STATE_PATH), { recursive: true })
  const browser = await chromium.launch()
  const page = await browser.newPage({ baseURL })
  try {
    await page.goto("/")
    // The form opens on password. A login code is the recovery path, which is
    // what an out-of-band code is for and the only credential a test can be
    // handed without also owning an account's password.
    await page.getByRole("button", { name: /login code/i }).click()
    await page.getByLabel("Email").fill(email)
    await page.getByLabel("Login code").fill(code)
    // exact: the form also carries a "Sign in with a password" link, and a
    // substring match would find both and refuse to guess.
    await page.getByRole("button", { name: "Sign in", exact: true }).click()
    // Signed in when the login card is gone. Asserting on its absence rather
    // than on a URL keeps this working if the post-login landing route moves.
    await page.locator(".login-page__card").waitFor({ state: "detached", timeout: 15_000 })
    await page.context().storageState({ path: STATE_PATH })
  } finally {
    await browser.close()
  }
}

export default globalSetup
