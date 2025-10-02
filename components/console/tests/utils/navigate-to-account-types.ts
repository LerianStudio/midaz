import { Page, expect } from '@playwright/test'

export async function navigateToAccountTypes(page: Page) {
  // Navigate directly to account types page since sidebar navigation lacks accessible labels
  // Use 'domcontentloaded' instead of 'load' to avoid waiting for all network requests
  await page.goto('/account-types', { waitUntil: 'domcontentloaded' })

  // Wait for the page title to be visible - this confirms the page structure is rendered
  // Use a longer timeout since the page makes multiple API calls on mount
  await expect(page.getByTestId('title')).toBeVisible({ timeout: 15000 })

  // Wait for the skeleton to disappear - this indicates initial data loading is complete
  // The skeleton only shows while the data is loading
  await page
    .waitForSelector('[class*="AccountTypesSkeleton"], [class*="skeleton"]', {
      state: 'hidden',
      timeout: 15000
    })
    .catch(() => {
      // Skeleton might not appear if data loads very quickly, continue
    })

  // Wait for the account types table to be visible - this confirms data has loaded
  // The table only renders when data loading is complete
  await expect(page.getByTestId('account-types-table')).toBeVisible({
    timeout: 15000
  })

  // Wait for the new account type button to be present (even if disabled)
  // This confirms all page dependencies have loaded
  await expect(page.getByTestId('new-account-type')).toBeAttached({
    timeout: 10000
  })

  // Additional wait to ensure all JavaScript execution is complete
  // This gives time for any event handlers or state updates to finish
  await page.waitForTimeout(500)
}
