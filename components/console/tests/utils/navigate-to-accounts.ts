import { Page, expect } from '@playwright/test'

export async function navigateToAccounts(page: Page) {
  // Navigate directly to accounts page with extended timeout
  // Use 'networkidle' to ensure all async operations complete
  await page.goto('/accounts', {
    waitUntil: 'networkidle',
    timeout: 60000 // 60 seconds to allow for slow server startup
  })

  // Wait for the page heading to be visible - this confirms the page structure is rendered
  // Use role selector instead of test ID for better resilience
  await expect(
    page.getByRole('heading', { name: 'Accounts', level: 1 })
  ).toBeVisible({ timeout: 30000 })

  // Wait for the skeleton to disappear - this indicates initial data loading is complete
  // The AccountsSkeleton only shows while isAccountsLoading is true
  await page
    .waitForSelector('[class*="AccountsSkeleton"], [class*="skeleton"]', {
      state: 'hidden',
      timeout: 30000
    })
    .catch(() => {
      // Skeleton might not appear if data loads very quickly, continue
    })

  // Wait for React hydration and stabilization
  await page.waitForTimeout(2000)

  // Wait for the search input to be visible - this confirms form is rendered
  const searchInput = page.getByTestId('search-input')
  await expect(searchInput).toBeVisible({ timeout: 30000 })

  // Wait for the new account button to be present (even if disabled)
  // This confirms all page dependencies (assets, account types) have loaded
  const newAccountButton = page.getByTestId('new-account')
  await expect(newAccountButton).toBeAttached({ timeout: 30000 })

  // Final wait to ensure all JavaScript execution is complete
  // This gives time for any event handlers or state updates to finish
  await page.waitForTimeout(1000)
}
