import { Page } from '@playwright/test'

/**
 * Navigate to the Segments page
 */
export async function navigateToSegments(page: Page) {
  // Navigate to segments page
  await page.goto('/segments', {
    waitUntil: 'networkidle'
  })

  // Wait for the page to stabilize
  await page.waitForLoadState('domcontentloaded')
  await page.waitForTimeout(1000)
}
