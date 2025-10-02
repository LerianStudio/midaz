import { Page, expect } from '@playwright/test'

export async function navigateToOperationRoutes(page: Page) {
  // Navigate directly to operation routes page since sidebar navigation lacks accessible labels
  // Use 'domcontentloaded' instead of 'load' to avoid waiting for all network requests
  await page.goto('/operation-routes', { waitUntil: 'domcontentloaded' })

  // Wait for the page title to be visible - this confirms the page structure is rendered
  // Use a longer timeout since the page makes multiple API calls on mount
  await expect(page.getByTestId('title')).toBeVisible({ timeout: 15000 })

  // Wait for the new operation route button to be present (even if disabled)
  // This confirms the page's interactive elements have loaded
  await expect(page.getByTestId('new-operation-route')).toBeAttached({
    timeout: 10000
  })
}
