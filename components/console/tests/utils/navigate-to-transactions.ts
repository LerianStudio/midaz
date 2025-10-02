import { Page, expect } from '@playwright/test'

export async function navigateToTransactions(page: Page) {
  // Navigate directly to transactions page since sidebar navigation lacks accessible labels
  // Use 'domcontentloaded' instead of 'load' to avoid waiting for all network requests
  await page.goto('/transactions', { waitUntil: 'domcontentloaded' })

  // Wait for the page title to be visible - this confirms the page structure is rendered
  // Use a longer timeout since the page makes multiple API calls on mount
  await expect(page.getByTestId('title')).toBeVisible({ timeout: 15000 })

  // Wait for the new transaction button to be present (even if disabled)
  // This confirms the page's interactive elements have loaded
  await expect(page.getByTestId('new-transaction')).toBeAttached({
    timeout: 10000
  })
}
