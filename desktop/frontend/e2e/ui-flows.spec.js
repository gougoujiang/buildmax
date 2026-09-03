import { test, expect } from '@playwright/test'

// Two more deterministic, credential-free checks alongside golden-path.spec.js.
// Both stay inside the React app's own state — no bound Go method that could
// hang on a native dialog (OpenFolderDialog opens a real OS picker Playwright
// cannot drive), no login.
test.beforeEach(async ({ page }) => {
  await page.goto('/')
  await expect(page.getByText('Loading…')).toHaveCount(0, { timeout: 15_000 })
})

test('opens and closes the New Project modal', async ({ page }) => {
  // Not getByRole('button', { name: 'New Project' }): the sidebar has its own
  // "New Project" entry point (icon-only when the list is non-empty, a
  // labeled button in the empty state this fresh sandbox always shows), so
  // that name is not unique. This is the "Continue your work" header button.
  await page.locator('.page-home__primary').click()
  const modal = page.locator('.modal-panel')
  await expect(modal).toBeVisible()
  await expect(modal.getByRole('heading', { name: 'New Project' })).toBeVisible()

  await page.keyboard.press('Escape')
  await expect(modal).toBeHidden()
})

test('toggles the theme from the user menu', async ({ page }) => {
  const initial = await page.evaluate(() => document.documentElement.getAttribute('data-theme'))

  await page.locator('.sidebar__user-trigger').click()
  const themeItem = page.locator('.sidebar__user-menu-item--icon')
  await expect(themeItem).toBeVisible()
  await themeItem.click()

  await expect
    .poll(() => page.evaluate(() => document.documentElement.getAttribute('data-theme')))
    .not.toBe(initial)
})
