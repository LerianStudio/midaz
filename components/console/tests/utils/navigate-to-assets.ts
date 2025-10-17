import { Page, expect } from '@playwright/test'

export async function navigateToAssets(page: Page) {
  await page.goto('/assets', {
    waitUntil: 'networkidle'
  })

  // Wait for the page to stabilize
  await page.waitForLoadState('domcontentloaded')
  await page.waitForTimeout(3000)
}
