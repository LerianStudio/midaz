import { Page } from '@playwright/test'

/**
 * Navigate to the Organizations page
 * Organizations is under Settings > Organizations tab
 */
export async function navigateToOrganizations(page: Page) {
  // Navigate to settings page with organizations tab
  await page.goto('/settings?tab=organizations', {
    waitUntil: 'networkidle'
  })

  // Wait for the page to stabilize
  await page.waitForLoadState('domcontentloaded')
  await page.waitForTimeout(1000)
}
