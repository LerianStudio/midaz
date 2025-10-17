import { Page, expect } from '@playwright/test'

export async function navigateToPortfolios(page: Page) {
  await page.goto('/portfolios', {
    waitUntil: 'networkidle'
  })

  // Wait for the page to stabilize
  await page.waitForLoadState('domcontentloaded')
  await page.waitForTimeout(3000)
}
