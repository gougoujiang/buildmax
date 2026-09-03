import { test, expect } from '@playwright/test'

// The one golden path this suite proves: the React app boots inside the real
// Wails dev server, the Go bindings it depends on are actually present (not
// the "run this with Wails" fallback a plain browser would show), and the app
// settles into local mode — the state a fresh BUILDMAX_HOME with no
// settings.yaml and nobody signed in always reaches, so this needs no model,
// no account, and no committed scenario to stay deterministic.
test('boots into local mode with real Go bindings', async ({ page }) => {
  await page.goto('/')

  await expect(page).toHaveTitle('BuildMax')

  // window.go is injected by the Wails runtime script the dev server serves;
  // a plain page (or a broken build) never has it.
  await expect
    .poll(() => page.evaluate(() => typeof window.go?.desktop?.App?.GetAuthStatus))
    .toBe('function')

  // The app shows this exact copy only when the bridge is missing, which is
  // the failure mode most worth catching by name.
  await expect(page.locator('body')).not.toContainText('Run this app with Wails')

  // Local mode still passes through a "Loading…" placeholder while
  // GetAuthStatus and the project list resolve.
  await expect(page.getByText('Loading…')).toHaveCount(0, { timeout: 15_000 })
})
