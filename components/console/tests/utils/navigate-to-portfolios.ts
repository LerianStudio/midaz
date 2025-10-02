import { Page, expect } from '@playwright/test'

export async function navigateToPortfolios(page: Page) {
  // Navigate directly to portfolios page since sidebar navigation lacks accessible labels
  // Use 'domcontentloaded' instead of 'load' to avoid waiting for all network requests
  await page.goto('/portfolios', { waitUntil: 'domcontentloaded' })

  // Wait for the page title to be visible - this confirms the page structure is rendered
  // Use a longer timeout since the page makes multiple API calls on mount
  await expect(page.getByTestId('title')).toBeVisible({ timeout: 15000 })

  // Wait for the new portfolio button to be present (even if disabled)
  // This confirms the page's interactive elements have loaded
  await expect(page.getByTestId('new-portfolio')).toBeAttached({
    timeout: 10000
  })
}
