import { Page, expect } from '@playwright/test'

export async function navigateToLedgers(page: Page) {
  // Navigate directly to ledgers page since sidebar navigation lacks accessible labels
  // Use 'domcontentloaded' instead of 'load' to avoid waiting for all network requests
  await page.goto('/ledgers', { waitUntil: 'domcontentloaded' })

  // Wait for the page title to be visible - this confirms the page structure is rendered
  // Use a longer timeout since the page makes multiple API calls on mount
  await expect(
    page.getByRole('heading', { name: 'Ledgers', level: 1 })
  ).toBeVisible({ timeout: 30000 })

  // Wait for the new ledger button to be present (even if disabled)
  // This confirms the page's interactive elements have loaded
  await expect(page.getByRole('button', { name: 'New Ledger' })).toBeAttached({
    timeout: 30000
  })
}
