import { Page, expect } from '@playwright/test'

export async function navigateToAccounts(page: Page) {
  // Navigate directly to accounts page with extended timeout
  // Use 'domcontentloaded' to avoid waiting for all network requests
  await page.goto('/accounts', {
    waitUntil: 'domcontentloaded',
    timeout: 60000 // 60 seconds to allow for slow server startup
  })

  // Wait for the page heading to be visible - this confirms the page structure is rendered
  // Use role selector instead of test ID for better resilience
  await expect(
    page.getByRole('heading', { name: 'Accounts', level: 1 })
  ).toBeVisible({ timeout: 15000 })

  // Wait for the skeleton to disappear - this indicates initial data loading is complete
  await page
    .waitForSelector('[class*="AccountsSkeleton"], [class*="skeleton"]', {
      state: 'hidden',
      timeout: 15000
    })
    .catch(() => {
      // Skeleton might not appear if data loads very quickly, continue
    })

  // Wait for either the new account button, table, or empty state to confirm page is ready
  await Promise.race([
    page
      .getByTestId('new-account')
      .waitFor({ state: 'attached', timeout: 15000 }),
    page
      .getByTestId('accounts-table')
      .waitFor({ state: 'visible', timeout: 15000 }),
    page
      .getByText(/You haven't created any Accounts yet/i)
      .waitFor({ state: 'visible', timeout: 15000 })
  ]).catch(() => {
    // If none appears, continue anyway - page is rendered
  })

  // Additional wait to ensure all JavaScript execution is complete
  // This gives time for any event handlers or state updates to finish
  await page.waitForTimeout(500)
}
