import { Page, expect } from '@playwright/test'

export async function navigateToTransactionRoutes(page: Page) {
  // Navigate directly to transaction routes page since sidebar navigation lacks accessible labels
  // Use 'domcontentloaded' instead of 'load' to avoid waiting for all network requests
  await page.goto('/transaction-routes', { waitUntil: 'domcontentloaded' })

  // Wait for the page heading to be visible - confirms page loaded
  await expect(
    page.getByRole('heading', { name: 'Transaction Routes', level: 1 })
  ).toBeVisible({ timeout: 15000 })

  // Wait for the new transaction route button to be present (even if disabled)
  // This confirms the page's interactive elements have loaded
  await expect(page.getByTestId('new-transaction-route')).toBeAttached({
    timeout: 10000
  })
}
