import { Page, expect } from '@playwright/test'

export async function navigateToAccountTypes(page: Page) {
  // Navigate directly to account types page with extended timeout
  await page.goto('/account-types', {
    waitUntil: 'domcontentloaded',
    timeout: 60000
  })

  // Wait for the page heading to be visible - confirms page structure is rendered
  await expect(
    page.getByRole('heading', { name: 'Account Types', level: 1 })
  ).toBeVisible({ timeout: 15000 })

  // Wait for the skeleton to disappear - indicates data loading is complete
  await page
    .waitForSelector('[class*="AccountTypesSkeleton"], [class*="skeleton"]', {
      state: 'hidden',
      timeout: 15000
    })
    .catch(() => {
      // Skeleton might not appear if data loads quickly
    })

  // Wait for either the new-account-type button or empty state to confirm page is ready
  await Promise.race([
    page
      .getByRole('button', { name: /new account type/i })
      .waitFor({ state: 'visible', timeout: 15000 }),
    page
      .getByText(/You haven't created any Account Types yet/i)
      .waitFor({ state: 'visible', timeout: 15000 })
  ]).catch(() => {
    // If neither appears, continue - page might be loading
  })

  // Additional wait to ensure all JavaScript execution is complete
  await page.waitForTimeout(500)
}
